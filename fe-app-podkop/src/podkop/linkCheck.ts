import { validateProxyUrl } from '../validators/validateProxyUrl';
import { linkParts, readAddress } from '../validators/linkAddress';
import { PodkopShellMethods } from './methods';
import { logger } from './services';
import type { Podkop } from './types';

// Before the settings are saved, the proxy links of every section are asked of the router:
// the engine of its build takes each as the configuration would (check_links), so that a
// link it cannot use (a transport or a kind of outbound this build has not) is told by its
// field here, and not by the log after the apply, where it would keep the whole
// configuration from running. One request for the links of the form (more for many long
// ones: linkBatches), at the save only: the page checks the form of a link itself as it is
// typed.

// the fields of a section that hold proxy links
const LINK_OPTIONS = [
  'proxy_string',
  'selector_proxy_links',
  'urltest_proxy_links',
  'urltest_fallback_links',
];

export interface LinkPlace {
  section: string;
  option: string;
  field: string;
  // the place of the link in its field, from 1, and how many the field has
  index: number;
  count: number;
  link: string;
}

export interface LinkProblem extends LinkPlace {
  error: string;
}

// what LuCI's form gives of itself (form.Map, form.TypedSection, form.AbstractValue)
interface FormOption {
  option: string;
  title?: string;
  isActive(sectionId: string): boolean;
  formvalue(sectionId: string): unknown;
}

interface FormSection {
  cfgsections(): string[];
  children: FormOption[];
}

export interface FormMap {
  children: FormSection[];
}

// linkPlaces lists the links of the active link fields of every section, as the form holds
// them now
export function linkPlaces(map: FormMap): LinkPlace[] {
  const places: LinkPlace[] = [];
  for (const section of map.children ?? []) {
    if (typeof section.cfgsections !== 'function') continue;
    for (const sectionId of section.cfgsections()) {
      for (const option of section.children ?? []) {
        if (
          !LINK_OPTIONS.includes(option.option) ||
          !option.isActive(sectionId)
        ) {
          continue;
        }
        const value = option.formvalue(sectionId);
        const links = (Array.isArray(value) ? value : [value])
          .map((item) => (typeof item === 'string' ? item.trim() : ''))
          .filter(Boolean);
        links.forEach((link, index) =>
          places.push({
            section: sectionId,
            option: option.option,
            field: (option.title ?? option.option).replace(/<[^>]*>/g, ''),
            index: index + 1,
            count: links.length,
            link,
          }),
        );
      }
    }
  }
  return places;
}

// linkLabel names a link without its secrets: its name, its kind and its server
export function linkLabel(link: string): string {
  const scheme = link.includes('://') ? link.slice(0, link.indexOf('://')) : '';
  let name = '';
  const hash = link.indexOf('#');
  if (hash >= 0) {
    try {
      name = decodeURIComponent(link.slice(hash + 1)).trim();
    } catch (_e) {
      name = link.slice(hash + 1).trim();
    }
  }
  const address = readAddress(linkParts(link).address);
  const server =
    typeof address === 'string'
      ? ''
      : (address.host.includes(':') ? `[${address.host}]` : address.host) +
        (address.port ? `:${address.port}` : '');
  const kind = server ? `${scheme}, ${server}` : scheme;
  return name ? `«${name}» (${kind})` : kind;
}

// checkLinksBeforeSave asks the router about the links of the form and returns those its
// engine refuses. A link of a wrong form is left to the form's own check (LuCI refuses the
// save and marks the field); when the router does not answer, nothing is refused.
export async function checkLinksBeforeSave(
  map: FormMap,
): Promise<LinkProblem[]> {
  const places = linkPlaces(map);
  if (!places.length) return [];
  if (places.some((place) => !validateProxyUrl(place.link).valid)) return [];
  const links = [...new Set(places.map((place) => place.link))];
  try {
    const results = new Map<string, Podkop.LinkCheck>();
    for (const part of linkBatches(links)) {
      const answer = await PodkopShellMethods.checkLinks(part);
      if (
        !answer.success ||
        !Array.isArray(answer.data) ||
        answer.data.length !== part.length
      ) {
        logger.warn('[LINKS]', 'the router did not check the links');
        return [];
      }
      part.forEach((link, index) => results.set(link, answer.data[index]));
    }
    return places
      .filter((place) => !results.get(place.link)?.ok)
      .map((place) => ({
        ...place,
        error:
          results.get(place.link)?.error || _('the engine refuses this link'),
      }));
  } catch (err) {
    logger.warn('[LINKS]', 'the router did not check the links:', err);
    return [];
  }
}

// uhttpd takes no more than 64 KB of a request to /ubus (UH_UBUS_MAX_POST_SIZE) and answers
// a larger one with an error: the links are asked in parts well below that
const PART_BYTES = 40000;

// linkBatches splits the links into parts of PART_BYTES of JSON at most, a link that is longer
// alone
export function linkBatches(links: string[]): string[][] {
  const parts: string[][] = [];
  let part: string[] = [];
  let size = 0;
  for (const link of links) {
    const bytes = JSON.stringify(link).length + 1;
    if (part.length && size + bytes > PART_BYTES) {
      parts.push(part);
      part = [];
      size = 0;
    }
    part.push(link);
    size += bytes;
  }
  if (part.length) parts.push(part);
  return parts;
}

// showLinkProblem opens the tab of the sections and points at the field of a link: the item
// of the link in a list, or the whole field
function showLinkProblem(problem: LinkProblem) {
  ui.hideModal();
  const tab = document.querySelector<HTMLElement>(
    'ul.cbi-tabmenu li[data-tab="section"] a',
  );
  tab?.click();
  const field = document.getElementById(
    `cbi-podkop-${problem.section}-${problem.option}`,
  );
  if (!field) return;
  const item = Array.from(field.querySelectorAll<HTMLElement>('.item')).find(
    (element) => element.textContent?.trim() === problem.link,
  );
  const target = item ?? field;
  target.scrollIntoView({ block: 'center' });
  target.classList.add('pdk-link-problem');
  setTimeout(() => target.classList.remove('pdk-link-problem'), 6000);
}

// showLinkProblems tells which links keep the settings from being saved, and why
export function showLinkProblems(problems: LinkProblem[]) {
  ui.showModal(_('Some proxy links cannot be used'), [
    E('p', {}, [
      _(
        'The settings are not saved. The engine of the router refuses these links:',
      ),
    ]),
    E(
      'ul',
      { class: 'pdk-link-problems' },
      problems.map((problem) =>
        E('li', {}, [
          E('strong', {}, [
            `${problem.section} · ${problem.field}` +
              (problem.count > 1 ? ` · ${problem.index}/${problem.count}` : ''),
          ]),
          E('div', {}, [linkLabel(problem.link)]),
          E('div', { class: 'pdk-link-problem-error' }, [problem.error]),
          E(
            'button',
            {
              class: 'cbi-button cbi-button-action',
              click: () => showLinkProblem(problem),
            },
            [_('Show')],
          ),
        ]),
      ),
    ),
    E('div', { class: 'right' }, [
      E('button', { class: 'cbi-button', click: () => ui.hideModal() }, [
        _('Close'),
      ]),
    ]),
  ]);
}

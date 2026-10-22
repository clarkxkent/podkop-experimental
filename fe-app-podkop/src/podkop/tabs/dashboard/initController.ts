import { onMount, preserveScrollForPage } from '../../../helpers';
import { prettyBytes } from '../../../helpers/prettyBytes';
import { CustomPodkopMethods, PodkopShellMethods } from '../../methods';
import { logger, store, StoreType } from '../../services';
import { renderSections, renderWidget } from './partials';
import { fetchServicesInfo } from '../../fetchers';
import { statsWidgets } from './helpers/statsWidgets';
import type { Podkop } from '../../types';

// Fetchers

async function fetchDashboardSections() {
  const prev = store.get().sectionsWidget;

  store.set({
    sectionsWidget: {
      ...prev,
      failed: false,
    },
  });

  const { data, success } = await CustomPodkopMethods.getDashboardSections();

  if (!success) {
    logger.error('[DASHBOARD]', 'fetchDashboardSections: failed to fetch');
  }

  store.set({
    sectionsWidget: {
      latencyFetching: false,
      loading: false,
      failed: !success,
      data,
    },
  });
}

// The numbers of the dashboard come from the running backend: the method stats of its
// object on ubus, asked every 2 s while the tab is open and the page visible (the first two
// a second apart, for the first rates). The page used to read two websockets of the Clash
// API, one of which sent the whole list of the connections every second (141 KB/s at 300
// connections), and which a page of LuCI served over https cannot open.
const STATS_MS = 2000;
let statsTimers: ReturnType<typeof setTimeout>[] = [];
let lastStats: Podkop.Stats | undefined;
let backendStats: (() => Promise<Podkop.Stats | number>) | undefined;

async function fetchStats() {
  if (document.hidden) {
    // the rates after a pause are of the next two answers
    lastStats = undefined;
    return;
  }
  backendStats ??= rpc.declare<Podkop.Stats | number>({
    object: 'podkop',
    method: 'stats',
    nobatch: true,
  });
  let current: Podkop.Stats | undefined;
  try {
    const reply = await backendStats();
    current = typeof reply === 'object' && reply !== null ? reply : undefined;
  } catch {
    // no object podkop: the backend does not run
  }
  if (!mounted) return;
  store.set(statsWidgets(current, lastStats));
  lastStats = current;
}

function startStats() {
  stopStats();
  fetchStats();
  statsTimers = [
    setTimeout(fetchStats, 1000),
    setInterval(fetchStats, STATS_MS),
  ];
}

function stopStats() {
  statsTimers.forEach((timer) => clearTimeout(timer));
  statsTimers = [];
  lastStats = undefined;
}

// Handlers

async function handleChooseOutbound(selector: string, tag: string) {
  await PodkopShellMethods.setClashApiGroupProxy(selector, tag);
  await fetchDashboardSections();
}

async function handleTestGroupLatency(tag: string) {
  store.set({
    sectionsWidget: {
      ...store.get().sectionsWidget,
      latencyFetching: true,
    },
  });

  await PodkopShellMethods.getClashApiGroupLatency(tag);
  await fetchDashboardSections();

  store.set({
    sectionsWidget: {
      ...store.get().sectionsWidget,
      latencyFetching: false,
    },
  });
}

async function handleTestProxyLatency(tag: string) {
  store.set({
    sectionsWidget: {
      ...store.get().sectionsWidget,
      latencyFetching: true,
    },
  });

  await PodkopShellMethods.getClashApiProxyLatency(tag);
  await fetchDashboardSections();

  store.set({
    sectionsWidget: {
      ...store.get().sectionsWidget,
      latencyFetching: false,
    },
  });
}

// Renderer

async function renderSectionsWidget() {
  logger.debug('[DASHBOARD]', 'renderSectionsWidget');
  const sectionsWidget = store.get().sectionsWidget;
  const container = document.getElementById('dashboard-sections-grid');

  if (sectionsWidget.loading || sectionsWidget.failed) {
    const renderedWidget = renderSections({
      loading: sectionsWidget.loading,
      failed: sectionsWidget.failed,
      section: {
        code: '',
        displayName: '',
        outbounds: [],
        withTagSelect: false,
      },
      onTestLatency: () => {},
      onChooseOutbound: () => {},
      latencyFetching: sectionsWidget.latencyFetching,
    });

    return preserveScrollForPage(() => {
      container!.replaceChildren(renderedWidget);
    });
  }

  const renderedWidgets = sectionsWidget.data.map((section) =>
    renderSections({
      loading: sectionsWidget.loading,
      failed: sectionsWidget.failed,
      section,
      latencyFetching: sectionsWidget.latencyFetching,
      onTestLatency: (tag) => {
        if (section.withTagSelect) {
          return handleTestGroupLatency(tag);
        }

        return handleTestProxyLatency(tag);
      },
      onChooseOutbound: (selector, tag) => {
        handleChooseOutbound(selector, tag);
      },
    }),
  );

  return preserveScrollForPage(() => {
    container!.replaceChildren(...renderedWidgets);
  });
}

async function renderBandwidthWidget() {
  logger.debug('[DASHBOARD]', 'renderBandwidthWidget');
  const traffic = store.get().bandwidthWidget;

  const container = document.getElementById('dashboard-widget-traffic');

  if (traffic.loading || traffic.failed) {
    const renderedWidget = renderWidget({
      loading: traffic.loading,
      failed: traffic.failed,
      title: '',
      items: [],
    });

    return container!.replaceChildren(renderedWidget);
  }

  const renderedWidget = renderWidget({
    loading: traffic.loading,
    failed: traffic.failed,
    title: _('Traffic'),
    items: [
      { key: _('Uplink'), value: `${prettyBytes(traffic.data.up)}/s` },
      { key: _('Downlink'), value: `${prettyBytes(traffic.data.down)}/s` },
    ],
  });

  container!.replaceChildren(renderedWidget);
}

async function renderTrafficTotalWidget() {
  logger.debug('[DASHBOARD]', 'renderTrafficTotalWidget');
  const trafficTotalWidget = store.get().trafficTotalWidget;

  const container = document.getElementById('dashboard-widget-traffic-total');

  if (trafficTotalWidget.loading || trafficTotalWidget.failed) {
    const renderedWidget = renderWidget({
      loading: trafficTotalWidget.loading,
      failed: trafficTotalWidget.failed,
      title: '',
      items: [],
    });

    return container!.replaceChildren(renderedWidget);
  }

  const renderedWidget = renderWidget({
    loading: trafficTotalWidget.loading,
    failed: trafficTotalWidget.failed,
    title: _('Traffic Total'),
    items: [
      {
        key: _('Uplink'),
        value: String(prettyBytes(trafficTotalWidget.data.uploadTotal)),
      },
      {
        key: _('Downlink'),
        value: String(prettyBytes(trafficTotalWidget.data.downloadTotal)),
      },
    ],
  });

  container!.replaceChildren(renderedWidget);
}

async function renderSystemInfoWidget() {
  logger.debug('[DASHBOARD]', 'renderSystemInfoWidget');
  const systemInfoWidget = store.get().systemInfoWidget;

  const container = document.getElementById('dashboard-widget-system-info');

  if (systemInfoWidget.loading || systemInfoWidget.failed) {
    const renderedWidget = renderWidget({
      loading: systemInfoWidget.loading,
      failed: systemInfoWidget.failed,
      title: '',
      items: [],
    });

    return container!.replaceChildren(renderedWidget);
  }

  const renderedWidget = renderWidget({
    loading: systemInfoWidget.loading,
    failed: systemInfoWidget.failed,
    title: _('System info'),
    items: [
      {
        key: _('Active Connections'),
        value: String(systemInfoWidget.data.connections),
      },
      {
        key: _('Memory Usage'),
        value: String(prettyBytes(systemInfoWidget.data.memory)),
      },
    ],
  });

  container!.replaceChildren(renderedWidget);
}

async function renderServicesInfoWidget() {
  logger.debug('[DASHBOARD]', 'renderServicesInfoWidget');
  const servicesInfoWidget = store.get().servicesInfoWidget;

  const container = document.getElementById('dashboard-widget-service-info');

  if (servicesInfoWidget.loading || servicesInfoWidget.failed) {
    const renderedWidget = renderWidget({
      loading: servicesInfoWidget.loading,
      failed: servicesInfoWidget.failed,
      title: '',
      items: [],
    });

    return container!.replaceChildren(renderedWidget);
  }

  const renderedWidget = renderWidget({
    loading: servicesInfoWidget.loading,
    failed: servicesInfoWidget.failed,
    title: _('Services info'),
    items: [
      // the process of podkop, sing-box inside it, and whether the router starts it
      {
        key: _('Podkop'),
        value: servicesInfoWidget.data.singbox
          ? _('✔ Running')
          : _('✘ Stopped'),
        attributes: {
          class: servicesInfoWidget.data.singbox
            ? 'pdk_dashboard-page__widgets-section__item__row--success'
            : 'pdk_dashboard-page__widgets-section__item__row--error',
        },
      },
      {
        key: _('Autostart'),
        value: servicesInfoWidget.data.podkop
          ? _('✔ Enabled')
          : _('✘ Disabled'),
        attributes: {
          class: servicesInfoWidget.data.podkop
            ? 'pdk_dashboard-page__widgets-section__item__row--success'
            : 'pdk_dashboard-page__widgets-section__item__row--error',
        },
      },
    ],
  });

  container!.replaceChildren(renderedWidget);
}

async function onStoreUpdate(
  next: StoreType,
  prev: StoreType,
  diff: Partial<StoreType>,
) {
  if (diff.sectionsWidget) {
    renderSectionsWidget();
  }

  if (diff.bandwidthWidget) {
    renderBandwidthWidget();
  }

  if (diff.trafficTotalWidget) {
    renderTrafficTotalWidget();
  }

  if (diff.systemInfoWidget) {
    renderSystemInfoWidget();
  }

  if (diff.servicesInfoWidget) {
    renderServicesInfoWidget();
  }
}

// The controller starts once a page: LuCI asks the value of the dummy field that starts it
// at every render and after every save, and each start used to add a listener of the tabs that
// was never taken away (each opening of the tab did its work as many times). It mounts at
// every opening of the tab and unmounts when another tab opens; a render of LuCI while the tab
// is open (after a save) mounts it again once, on the new elements.
let started = false;
let mounted = false;
let remounting = false;

async function onPageMount() {
  // Cleanup before mount
  onPageUnmount();
  mounted = true;

  // Add new listener
  store.subscribe(onStoreUpdate);

  // Initial sections fetch
  await fetchDashboardSections();
  await fetchServicesInfo();
  // the tab may have been left while the answers came
  if (!mounted) return;
  startStats();
}

function onPageUnmount() {
  mounted = false;
  // Remove old listener
  store.unsubscribe(onStoreUpdate);
  // Clear store
  store.reset([
    'bandwidthWidget',
    'trafficTotalWidget',
    'systemInfoWidget',
    'servicesInfoWidget',
    'sectionsWidget',
  ]);
  stopStats();
}

function isCurrent() {
  return store.get().tabService.current === 'dashboard';
}

export async function initController(): Promise<void> {
  if (started) {
    if (mounted && !remounting) {
      remounting = true;
      onMount(
        'dashboard-status',
        document.getElementById('dashboard-status'),
      ).then(() => {
        remounting = false;
        if (mounted) onPageMount();
      });
    }
    return;
  }
  started = true;

  store.subscribe((next, prev, diff) => {
    if (
      !diff.tabService ||
      next.tabService.current === prev.tabService.current
    ) {
      return;
    }
    logger.debug('[DASHBOARD]', 'active tab:', next.tabService.current);
    if (isCurrent()) {
      onMount('dashboard-status').then(() => {
        if (isCurrent() && !mounted) onPageMount();
      });
    } else if (mounted) {
      onPageUnmount();
    }
  });

  onMount('dashboard-status').then(() => {
    if (isCurrent() && !mounted) onPageMount();
  });
}

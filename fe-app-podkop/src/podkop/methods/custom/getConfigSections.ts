import { Podkop } from '../../types';

// The sections and the settings of the file: the subscriptions and the groups are what
// the sections of the type group take their servers from, not sections themselves.
export async function getConfigSections(): Promise<Podkop.ConfigSection[]> {
  return uci
    .load('podkop')
    .then(() =>
      (uci.sections('podkop') as unknown as Podkop.ConfigSection[]).filter(
        (section) =>
          section['.type'] !== 'subscription' && section['.type'] !== 'group',
      ),
    );
}

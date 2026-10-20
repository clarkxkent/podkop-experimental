import { DIAGNOSTICS_CHECKS_MAP } from './contstants';
import { PodkopShellMethods } from '../../../methods';
import { updateCheckStore } from './updateCheckStore';
import { getMeta } from '../helpers/getMeta';

export async function runSingBoxCheck() {
  const { order, title, code } = DIAGNOSTICS_CHECKS_MAP.SINGBOX;

  updateCheckStore({
    order,
    code,
    title,
    description: _('Checking, please wait'),
    state: 'loading',
    items: [],
  });

  const singBoxChecks = await PodkopShellMethods.checkSingBox();

  if (!singBoxChecks.success) {
    updateCheckStore({
      order,
      code,
      title,
      description: _('Cannot receive checks result'),
      state: 'error',
      items: [],
    });

    throw new Error('Podkop checks failed');
  }

  const data = singBoxChecks.data;

  // podkop has sing-box inside: what is asked is whether it runs and listens where the DNS
  // of the LAN and the firewall send to (installed, version, service and autostart of a
  // separate sing-box mean nothing here)
  const running = Boolean(data.sing_box_process_running);
  const listening = Boolean(data.sing_box_ports_listening);
  const allGood = running && listening;
  const atLeastOneGood = running || listening;

  const { state, description } = getMeta({ atLeastOneGood, allGood });

  updateCheckStore({
    order,
    code,
    title,
    description,
    state,
    items: [
      {
        state: running ? 'success' : 'error',
        key: _('Podkop is running'),
        value: '',
      },
      {
        state: listening ? 'success' : 'error',
        key: _(
          'Podkop listens for the DNS of the LAN and the transparent proxy',
        ),
        value: '',
      },
    ],
  });

  if (!running) {
    throw new Error('Podkop checks failed');
  }
}

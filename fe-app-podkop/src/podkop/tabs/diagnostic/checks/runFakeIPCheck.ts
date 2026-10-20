import { DIAGNOSTICS_CHECKS_MAP } from './contstants';
import { PodkopShellMethods } from '../../../methods';
import { updateCheckStore } from './updateCheckStore';
import { getMeta } from '../helpers/getMeta';

export async function runFakeIPCheck(label: string) {
  const { order, title, code } = DIAGNOSTICS_CHECKS_MAP.FAKEIP;

  updateCheckStore({
    order,
    code,
    title,
    description: _('Checking, please wait'),
    state: 'loading',
    items: [],
  });

  const [routerFakeIPResponse, browserResponse] = await Promise.all([
    PodkopShellMethods.checkFakeIP(),
    PodkopShellMethods.checkFakeIPClient(label),
  ]);

  const checks = {
    router: routerFakeIPResponse.success && routerFakeIPResponse.data.fakeip,
    browserDNS: browserResponse.success && browserResponse.data.dns,
    browserFakeIP: browserResponse.success && browserResponse.data.connection,
  };

  const allGood = checks.router && checks.browserDNS && checks.browserFakeIP;
  const atLeastOneGood =
    checks.router || checks.browserDNS || checks.browserFakeIP;

  const { state, description } = getMeta({ atLeastOneGood, allGood });

  updateCheckStore({
    order,
    code,
    title,
    description,
    state,
    items: [
      {
        state: checks.router ? 'success' : 'warning',
        key: checks.router
          ? _('Router DNS is routed through podkop')
          : _('Router DNS is not routed through podkop'),
        value: '',
      },
      {
        state: checks.browserDNS ? 'success' : 'error',
        key: checks.browserDNS
          ? _('Browser DNS is routed through podkop')
          : _('Browser DNS is not routed through podkop'),
        value: '',
      },
      {
        state: checks.browserFakeIP ? 'success' : 'error',
        key: checks.browserFakeIP
          ? _('Browser is using FakeIP correctly')
          : _('Browser is not using FakeIP'),
        value: '',
      },
    ],
  });
}

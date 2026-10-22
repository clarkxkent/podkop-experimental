import { TabServiceInstance } from './tab.service';
import { store } from './store.service';
import { logger } from './logger.service';
import { isPodkopError, watchLog } from './logWatch.service';

export function coreService() {
  TabServiceInstance.onChange((activeId, tabs) => {
    logger.info('[TAB]', activeId);
    store.set({
      tabService: {
        current: activeId || '',
        all: tabs.map((tab) => tab.id),
      },
    });
  });

  // the errors of the backend are raised from the system log
  watchLog((line) => {
    if (isPodkopError(line)) {
      ui.addNotification(
        'Podkop Error',
        E('div', {}, [line.msg.replace(/^podkop(\[\d+\])?:\s*/, '')]),
        'error',
      );
    }
  });
}

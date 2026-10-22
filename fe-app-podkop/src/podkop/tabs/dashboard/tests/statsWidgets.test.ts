import { describe, expect, it } from 'vitest';
import { statsWidgets } from '../helpers/statsWidgets';
import type { Podkop } from '../../../types';

function stats(patch: Partial<Podkop.Stats>): Podkop.Stats {
  return {
    running: true,
    at: 0,
    up: 0,
    down: 0,
    connections: 0,
    memory: 0,
    ...patch,
  };
}

describe('statsWidgets', () => {
  it('makes the rates of two answers by the clock of the backend', () => {
    const widgets = statsWidgets(
      stats({ at: 3000, up: 3000, down: 9000, connections: 4, memory: 7 }),
      stats({ at: 1000, up: 1000, down: 1000 }),
    );
    expect(widgets.bandwidthWidget?.data).toEqual({ up: 1000, down: 4000 });
    expect(widgets.trafficTotalWidget?.data).toEqual({
      downloadTotal: 9000,
      uploadTotal: 3000,
    });
    expect(widgets.systemInfoWidget?.data).toEqual({
      connections: 4,
      memory: 7,
    });
  });

  it('leaves the rates for the first answer', () => {
    const widgets = statsWidgets(stats({ up: 5 }), undefined);
    expect(widgets.bandwidthWidget).toBeUndefined();
    expect(widgets.trafficTotalWidget?.failed).toBe(false);
  });

  it('gives no rate when the counts went down (another sing-box)', () => {
    const widgets = statsWidgets(
      stats({ at: 2000, up: 10, down: 10 }),
      stats({ at: 1000, up: 500, down: 500 }),
    );
    expect(widgets.bandwidthWidget?.data).toEqual({ up: 0, down: 0 });
  });

  it('fails every widget when nothing runs', () => {
    for (const current of [undefined, stats({ running: false })]) {
      const widgets = statsWidgets(current, stats({}));
      expect(widgets.bandwidthWidget?.failed).toBe(true);
      expect(widgets.trafficTotalWidget?.failed).toBe(true);
      expect(widgets.systemInfoWidget?.failed).toBe(true);
    }
  });
});

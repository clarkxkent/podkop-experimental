# The dashboard YACD of podkop-exp

podkop-exp brings a small build of [Yacd-meta](https://github.com/MetaCubeX/Yacd-meta) in its package
(`/usr/share/podkop/yacd`). With YACD on, the backend copies it to `/usr/share/podkop/ui`, the directory
sing-box serves as the `external_ui` of the Clash API, before sing-box starts: sing-box downloads a
dashboard itself, from GitHub and within its start, only into an empty directory, and that download is
gone. The copy is what the upgrade writes: the button "Upgrade Dashboard UI" of the dashboard (Config)
makes sing-box download the dashboard of Yacd-meta from GitHub and put it in place of the copy. An
upgrade that fails (no room left on the flash, GitHub out of reach) leaves the dashboard served:
podkop-engine r16 unpacks it into a directory of its own first and replaces the files only when all of
them are there. `podkop yacd_builtin` puts this one back by hand. The files of the package are never
written. `/usr/share/podkop/ui.builtin` tells which dashboard is served (sing-box leaves it alone): the
build copied and the hash of its `index.html`. A newer package replaces an older copy of its own, a
dashboard upgraded from GitHub stays, and one sing-box downloaded before this build was brought is
replaced.

## The build

Yacd-meta at `24151c331027ff176b45ed417180c6f1220474a3` (2026-10-21), with `podkop-minimal.patch` and the
files of `removed.txt` taken out. This build is not developed further: the upgrade goes to Yacd-meta
itself.

| | Yacd-meta | this build |
|---|---|---|
| on disk | 4.6 MB | 0.45 MB |
| gzip | 2.2 MB | 0.14 MB |

What the patch changes:

- Preact (`preact/compat`) in place of React and React DOM.
- English only: a stand-in for i18next and react-i18next (`src/shim/i18n.ts`, the strings of
  `src/i18n/en.ts`); no choice of language.
- The 41 Feather icons the dashboard uses as data (`src/shim/feather.tsx`, made by `feather-shim.py`
  from react-feather 2.0.10), in place of react-feather with prop-types.
- The charts of traffic and memory and the samples of the chart styles drawn on a canvas
  (`src/components/shared/LineChart.tsx`) when their data changes, in place of chart.js redrawing every
  frame.
- No drag and drop: a column of the connection table moves up by its button.
- No switch of the theme: it follows the system (`?theme=` still works for a visit), whatever Yacd-meta
  saved at the address before; no animation of collapsing.
- No service worker: `sw.js` lets go of the one Yacd-meta registers at the address (on https or
  localhost; plain http has none) and of its cache, which would go on serving the old copy.
- No fonts of its own (Inter, Roboto Mono, Twemoji: the system's are used), no logo of Clash Meta
  (105 KB in the styles), one icon of 64×64.
- The charts go through their points (a monotone curve); the first reading of memory, 0 from sing-box,
  is not drawn.
- The button that upgrades the dashboard is shown for sing-box too (Yacd-meta shows it only for a core
  that is not "premium"), and the request has a deadline of 5 minutes; the upgrade of the core stays
  hidden.

Rebuild (git, docker and the network): `sh yacd/build.sh` from the root of the repository. It gives the
same files, byte for byte. To change the build: a working copy of Yacd-meta at the commit above, the
files of `removed.txt` removed, the patch applied, the change made; then
`git diff --binary --diff-filter=AMR <commit> HEAD > podkop-minimal.patch` and
`git diff --name-only --diff-filter=D <commit> HEAD > removed.txt`.

Checked in Chromium, Firefox and WebKit against the sing-box of podkop-exp, the same steps for Yacd-meta
and for this build (rs-lab/pk08/e2e/yacdcheck.py of the lab): the groups, the choice in a selector, the
latency test, the rules, the connections (listed, settings, closed), the logs, the charts, the samples of
the styles, no error of the page or of the console.

The licence: MIT (`LICENSE`, installed with the files).

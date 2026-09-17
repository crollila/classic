# Forever talent window

The talent page now uses three illustrated, four-column trees with icon buttons,
prerequisite arrows, visible ranks, gold maxed states and per-tree / remaining
point counters. Small change markers link visually to the comparison and evidence
inside each tooltip. Other mechanic controls remain available in a disclosure.

Desktop supports click, right-click and Shift+click; keyboard supports Enter,
minus/Backspace and Escape. Touch uses inspect, second-tap add and 500 ms hold to
remove. Pointer cancellation or a drag cancels the hold. Rank changes retain DOM
nodes and open tooltips. Narrow screens scroll the three trees horizontally.

## Data and compatibility

- Engine, mechanics database, reviewed rulesets and shared schemas are unchanged.
  The TypeScript interfaces expose classification, Classic IDs and uncertainty
  fields already present in the checked-in discovery data.
- STRICT retains selected points for row access, but uses the existing effective
  rank suppression. The tooltip distinguishes selected and active ranks. BEST
  GUESS retains existing supported predictions. This is not new promotion logic.
- Presentation metadata uses WoWSims Classic icons and tree artwork where
  available, plus the captured calculator presentation snapshot for new icons and
  comparison text. Icons do not assert an authenticated Forever spell ID. Classic
  IDs are explicitly labeled. Blizzard art remains on the same Wowhead CDN used
  by WoWSims; initials provide a fallback when images cannot load.
- `F1:CLASS:tree-tree-tree` is a new frontend-only talent-string format. F1 fixes
  the current discovery catalog ordering. Future incompatible order/layout changes
  must introduce F2 or explicitly migrate F1; do not reinterpret existing strings.
  Classic positional strings are rejected rather than assigned to Forever nodes.
- URL parameters `ft` and `fm` encode allocation and STRICT/BEST_GUESS. A talent
  link opens the Talents tab after simulator settings initialize. Other simulation
  settings are not part of talent-only URLs; use the existing full-sim exporter
  for those. The existing `forever-v2-builds-<class>` local saved format is retained,
  including race and mechanics options. Clipboard failures expose selectable text.

## Validation (2026-09-13)

Run from `classic/`:

```text
node node_modules/typescript/bin/tsc --noEmit --pretty false
node scripts/test-talent-window.mjs
node scripts/test-forever-ui.mjs
node scripts/vite-forever.mjs
node --test scripts/forever.test.mjs
```

All passed: 56 talent tests (including all 27 trees in both modes), 3 existing
Forever UI tests, 7 existing frontend/release/static-routing tests and the full
production frontend build. Vite reports its existing large-bundle advisory.
An initial release-suite run in the isolated worktree could not resolve the
parent shared release file; the complete suite passed after frontend integration
at the existing `classic/` staging path.

For the browser fixture, run `node scripts/talent-qa-server.mjs`, open
`http://127.0.0.1:5178/forever/talent-qa.html`, and choose **Run all UI checks**.
The fixture is excluded from production build entry points. The test button
passed 54/54 rendered tree/mode scenarios, 81 synthetic pointer/keyboard checks,
and all nine classes' save/load, valid/invalid import and comparison controls.
The fixture uses manufactured players, not captures or inferred combat evidence.

The built simulator was also verified in a browser: shared Feral Druid links
select Talents and restore 0/51/0, STRICT shows 19 active and BEST GUESS 51 active,
copy URL succeeds, reload preserves allocation/mode, and the spell tooltip shows
prediction basis and source. Desktop and 390×844 responsive layouts were visually
inspected. No browser console errors were observed. Touch timing/cancellation was
tested with synthetic browser pointer events; physical phone testing remains a
separate device check. No simulator engine tests were required or claimed.

Implementation was developed on `ux/forever-talents` in `sim/talent-ux`, then only
the owned frontend and test files were copied into `classic/`. Existing edited
files were hash-checked before integration. No public deployment was performed.

Presentation regeneration takes the existing research snapshot as an explicit
argument: `node scripts/build-talent-presentation.mjs ../.research/forever-discovery/talent-data.json`.
It does not scrape the network or alter engine data.

# Forever frontend verification

Verified locally on 2026-09-13 against upstream revision `7779ebbf79dc`, with the current Classic baseline WebAssembly engine. These are interface and integration checks, not evidence that Forever combat mechanics are correct.

## Automated checks

- `npm run build:forever`: generated the production UI, shared assets/database, worker, WASM and release manifest.
- `npm run type-check`: TypeScript check passed.
- `npm run test:forever`: six checks passed, covering metadata completeness, evidence requirements, local versus external URL rewriting, built entry-point assets, analytics removal, deep routes, redirects, WASM content type and real 404 responses.

## Browser checks

All 14 routes listed by the shared launch registry completed actual browser simulations and displayed nonzero results: Balance Druid, Feral DPS Druid, Hunter, Mage, Retribution Paladin, Protection Paladin, Shadow Priest, Rogue, Elemental Shaman, Enhancement Shaman, Warden Shaman, DPS Warlock, DPS Warrior and Tank Warrior. The checks used the existing class presets; no expected DPS was invented.

Warrior interaction checks covered shared item/enchant lists, changing race, changing target level, adding a second target, selecting a saved APL, editing iteration count, resetting a talent tree and allocating a talent point. Results displayed ability damage, crit/miss columns, the DPS histogram, resource usage, buff/proc aura information and a single-iteration rotation timeline.

The launcher and simulator were visually reviewed at the normal desktop viewport. The talent editor and navigation were also reviewed in an iframe with an actual 390px layout viewport (375px content width after its scrollbar); no document-width overflow was present. Character stats collapse on mobile, tabs remain available through horizontal scrolling, and talent navigation retains the upstream carousel. The temporary responsive harness was removed after testing.

## Remaining boundaries

- Live Exaltedcapital.com routing/DNS was not modified; use the deployment instructions after provisioning the independent service.
- Docker/nginx deployment configuration was supplied, but the container was not executed in this Windows environment.
- The in-app test browser did not expose a newly opened separate-results popup. The embedded results view and its timeline were verified; separate-window behavior should also be checked in the production browser.
- Upstream log parsing emitted unmatched aura-stack warnings for Sunder Armor, Improved Scorch and Flurry during single-iteration timeline inspection. The timeline rendered, but those warnings limit stack-trace fidelity. Combat logic and log reconstruction were not changed here.
- The browser still uses the Classic baseline. Forever build/version and last reviewed mechanics update remain visibly UNKNOWN until authoritative data and engine integration are supplied.

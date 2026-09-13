# Forever Simulator frontend

The public entry is `/forever-sim/`. This is the existing WoWSims frontend with a separate presentation/build adapter, not a replacement simulation UI. No Go combat mathematics are changed.

## Develop and verify

From `classic/`, after installing the existing Node/Go/protobuf prerequisites:

```sh
npm ci
npm run build:forever
npm run test:forever
npm run preview:forever
# http://127.0.0.1:4173/forever-sim/
```

For UI iteration after the initial build, `npm run dev:forever` serves the application on port 5173 using the real compiled WASM worker. The build produces `dist/forever-sim/`, including the engine, shared database, all generated class pages, detailed results, and release metadata. It does not replace the original `build` command or write to the main website.

## Data and support boundaries

- Class/spec links use `getLaunchedSimsForClass`, shared protobuf enums and `proto_utils/utils`. The launcher does not maintain a second supported-spec list. Unlaunched specs stay unavailable.
- Talent trees, race eligibility, items, enchants, buffs, consumes and APL use their existing WoWSims sources. Updates belong in the shared simulator data pipeline, not in the presentation adapter.
- Existing settings expose race, buffs, debuffs, consumes, target level, duration and targets. Gear includes enchant selection. Talents retain the interactive trees. Rotation retains the APL editor. The sidebar keeps iterations and Run Sim.
- Detailed Results retains DPS distribution, ability damage, hit/miss/crit metrics, casts, resources, buffs/debuffs and proc/aura uptime. Timeline and Log retain the upstream single-iteration workflow. The upstream engine determines which metrics exist; no frontend estimates or invented proc counts are added.
- Shared release metadata lives at `../shared/simulator/release.json`. The build validates it and emits `release.json` with simulator version, upstream commit, build time and database/WASM SHA-256 hashes. Deploy assets atomically so metadata and engine stay together.
- `foreverBuild`, `mechanicsUpdatedAt` and `rulesetId` are deliberately null until authoritative values are available. Current `engineMode` is `upstream-baseline`: rebranding does **not** turn the Classic engine into validated Forever mechanics. This browser build rejects `forever-ruleset` metadata until explicit ruleset selection is integrated into the worker adapter. Metadata alone must never promote the Classic engine to a validated Forever release.

## Mechanics metadata

Each optional `mechanics` record has `id`, `label`, `category`, `confidence`, `note`, optional `specs` (shared numeric protobuf Spec IDs) and optional `evidence` (HTTPS URLs). Categories are `race`, `talents`, `equipment`, `enchants`, `buffs`, `debuffs`, `consumes`, `encounter`, `rotation`. The note must identify affected options and assumptions; no item, talent or racial database belongs here. Omit `specs` for a global mechanic. All applicable records remain visible even when an option is not currently selected, so uncertainty is not silently removed.

Accepted confidence values are CONFIRMED, HIGH CONFIDENCE, EXPERIMENTAL, UNKNOWN. The first two require evidence. Unlisted mechanics always remain UNKNOWN. These are public presentation annotations, not executable ruleset expressions and not substitutes for the shared reviewed ruleset schema. Missing/unreadable metadata fails to a visible UNKNOWN state; baseline simulations remain available for exploration.

## Independent deployment

Build and run from the workspace root:

```sh
docker compose -f classic/deploy/compose.forever.yml up --build -d
```

The service binds to `127.0.0.1:8088` by default. Configure your existing TLS reverse proxy on Exaltedcapital.com with **only** these additional locations:

```nginx
location = /forever-sim { return 308 /forever-sim/; }
location /forever-sim/ {
    proxy_pass http://127.0.0.1:8088;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

Keep `proxy_pass` without a trailing slash to preserve `/forever-sim/`. The site’s existing routes and assets stay on its current service. A simulator failure affects only this route. For `sim.exaltedcapital.com`, route that host to the same service; `/` redirects to `/forever-sim/`. Add a normal link on the main website to `/forever-sim/` or the subdomain. This checkout contains no main website source or hosting credentials; live DNS/proxy configuration is a separate deployment operation.

Static hosting also works: serve the parent `dist/` directory with directory indexes, real 404 responses and `application/wasm` for `.wasm`. Never rewrite every missing asset to HTML. Content-hashed bundles can be cached immutably; HTML, shared data, metadata, worker and WASM must revalidate. Use atomic releases and retain the previous deployment for rollback. No simulation API, account or user upload is required: simulations run locally in WebAssembly.

## Presentation and attribution

The theme follows the live Exalted Capital portfolio: white surfaces, charcoal type, Arial body text, Georgia headings, restrained green links and fine borders. All overrides are scoped to the Forever build. Original Classic branding, analytics and asset paths are adapted by `forever.config.mts`; external Classic Wowhead references are deliberately unchanged and labeled as references. WoWSims attribution and the MIT license remain visible.

Inherited external dependencies include Wowhead artwork/tooltips and CDN-hosted jQuery, tablesorter and Font Awesome. The engine and shared simulation database are hosted with the app. An offline deployment would need to vendor those presentation dependencies separately.

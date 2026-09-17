# Forever discovery ruleset v2

The public planning simulator uses the 701-record research snapshot, pinned by SHA-256 in `scripts/forever-discovery.mjs`. All 27 trees have semantic talent selection, rank evidence, prerequisites, and a 51-point budget. The Classic request path, item database, talent definitions and upstream regression expectations remain separate.

## Modes and evidence

BEST_GUESS is the planning default. It executes confirmed and provisional effects plus labeled predictions. STRICT disables predicted adapters and uses the highest observed rank at or below each selected rank. Selected points still determine legal tree access, so switching modes does not destroy the saved build. Skyborne requires BEST_GUESS because its base-stat offsets are predicted from Night Elf offsets. Missing Forever interaction details retain the closest Classic behavior; this is provisional and does not confirm undocumented stacking.

Confidence and implementation state are separate. ACTIVE means executable when applicable, not automatically selected in every build. A removed talent is active as an exclusion from selection and from the old talent bonus. Existing baseline rules count only where the ledger explicitly describes the executable fallback. Economic, leveling and other non-combat records are NON-SIM-RELEVANT. BLOCKED is reserved for records without enough identity or behavioral evidence for a reasonable implementation, such as unnamed items with no stats or set-bonus text.

The record-level confidence total uses PREDICTED when any implemented rank is estimated, or the adapter needs an explicit prediction. Observed ranks of that record can still execute in STRICT. Each rank retains its separate source confidence, estimated flag, numerical values and prediction reason. Source estimates are never relabeled confirmed. The original research manifest remains unchanged.

Secondary predictions are listed separately. For example, STRICT permits the known cleansing effect once per encounter while suppressing its predicted reusable cooldown. It treats a pre-applied campsite buff as a Classic snapshot unless the player supplies an explicit remaining duration. BEST_GUESS supplies the disclosed duration/cooldown prediction. Higher Legacy ranks and profession bonus predictions are disabled in STRICT. Scrolls are not treated as elixirs by Mixology.

## Browser configuration

The Forever build replaces Classic talent pickers with all three Forever trees for each class. It exposes STRICT/BEST_GUESS, eligible races, racial effects, class abilities, professions, campsite/Legacy options, and bounded scenario inputs. Saved builds retain semantic IDs. Legal sample builds support exploration and are explicitly not optimization recommendations. Classic presets migrate by talent names/fields and prune invalid moved prerequisites; positional strings are never reinterpreted as Forever trees.

New offensive actions can be prioritized automatically; proc conditions and enemy targets are supplied where needed. Players can disable the overlay and edit the APL. Healing and bear pages have registered factories and usable rotations. Healing output uses the engine's throughput metric, including potential overhealing; it is not a prediction of encounter-specific effective healing. Finite-health, movement, control and resource scenarios must be configured for effects that depend on them.

The equipment picker remains an explicit Classic planning catalogue. The manifest has no authenticated new item identity/stat combinations or complete set-bonus details, so manufacturing new items would create unsupported data. The UI shows those exact evidence gaps. Profession combat predictions are separately configurable and require the corresponding profession/equipment where applicable.

## Engine behavior and identity

The adapters extend spell modifiers, trigger dispatch, target multipliers, channels, absorption, dispels and immunities, control durations, movement, pet/guardian behavior, resource capacities, healing, and finite buff durations. Shared support activates per character/scenario rather than switching a global game mode. Class tests exercise legal builds and real casts/procs; control, healing, costs, cooldowns, iteration resets and Classic isolation have separate regression coverage.

Interrupts lock the interrupted school for the ability's documented or Classic duration; idle targets receive no invented lockout. Spell interception has an explicit single-target harmful classification so non-damaging control effects can be intercepted before application. Unclassified encounter damage retains the disclosed closest engine behavior. Enemy health and mana, incoming control, pre-pull rage waiting, and pet approach distance are bounded scenario inputs, not inferred encounter facts.

Authenticated Classic spell IDs identify reused abilities. New mechanics without authenticated client IDs use internal `OtherActionForever` enum 19 and the pinned manifest record index as a tag. Negative tags identify related secondary effects or alternate healing actions. These are simulator identities, never claimed game IDs. `ui/forever/data/actions.json` resolves names. Every implementation maps back to a manifest record through source comments, adapter declarations and the status ledger.

Cooldowns, coefficients and rank extrapolations that are predictions are stated in adapter metadata or rank overrides. Numerical tooltip effects are applied even when secondary stacking is unknown. Default finite durations and profession bonuses have bounded configuration parameters to support later log calibration. Classic items and consumables retain their known values unless a separately selected Forever prediction changes them.

## Transport and migration

Simulator-private schema v2 adds `ForeverMode`, `mechanic_ranks`, and bounded `parameters` to the existing optional `Player.forever` field 49. Existing Classic field numbers and strings are unchanged. `experimental_estimated_ranks` remains wire-compatible but is deprecated: explicit mode now controls predictions; default enum value means BEST_GUESS. New race enum values are internal identifiers. No root shared schema or original release fixture is overwritten.

V1 discovery requests must update the ruleset ID to `forever-discovery-2026-09-13-v2` and choose a mode. They fail clearly rather than silently changing semantics. Classic callers omit `forever`. The empty imported v1 compatibility catalogue stays fail-closed for unsupported external formulas. The CLI, raw core API, stats and worker paths use the same per-player preparation; requests are cloned and shared database mutation is rejected.

Published release metadata is schema v2 with `engineMode: forever-discovery`, ruleset, manifest hash and research date. It intentionally has no invented beta client build. The original v1 release validator remains supported. Valid and invalid request examples plus schema tests cover compatibility. CLI provenance records selected and effective ranks separately, active/inactive state, prediction basis and confidence.

## Run and verify

From `classic/` in PowerShell:

```powershell
. ./scripts/env.ps1
go run --tags=with_db ./cmd/foreversim -game forever -catalog discovery -infile examples/mage-discovery.json -outfile .artifacts/mage-discovery-result.json
node scripts/forever-discovery.mjs --check
node scripts/forever-status.mjs
npm run type-check
npm test
npm run check:classic
npm run build:forever
npm run test:forever
```

The browser worker embeds the item database through the `with_db` build tag, so Forever requests do not mutate global database state. The boundary check compares unchanged protected files to upstream and pins reviewed extension hashes. It does not substitute for behavioral regression tests. Never regenerate upstream `.results` files to hide failures. Actual validation evidence and remaining blockers are in `sim/forever/discovery-validation.json` and the root implementation status report.

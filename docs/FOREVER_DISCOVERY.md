# Forever discovery ruleset v1

This is a partial implementation of the 701-record research snapshot, separate from the unchanged Classic default and the existing empty Forever compatibility catalog. The manifest is pinned by SHA-256 in `scripts/forever-discovery.mjs`; changes to the evidence require a deliberate ruleset revision. No public calculator IDs are treated as client spell IDs.

## Running a supported build

From `classic/` in PowerShell:

```powershell
. ./scripts/env.ps1
go run --tags=with_db ./cmd/foreversim -game forever -catalog discovery -infile examples/mage-discovery.json -outfile .artifacts/mage-discovery-result.json
```

Each player must have `forever.rulesetId: "forever-discovery-2026-09-13-v1"`, a `talents` map keyed by manifest record ID, and an empty `talentsString`. `mechanics` explicitly selects supported racial/buff record IDs. The engine validates class, race, rank, prerequisites, lower-row investment and the 51-point limit before translating reviewed counterparts into existing typed talents. Removed talents are not selectable. It never interprets a Forever positional tree as a Classic build.

`experimentalEstimatedRanks: true` is required to select any source-estimated rank. The source's actual extrapolated numbers are retained; they are not promoted to observed values. Independently inspected corrections in `implementation_rank_values` take precedence. The CLI result records each selected rank, its value confidence, its source URL, whether it is estimated, and provisional interaction confidence. It also labels the ruleset partial. The machine-readable status includes ranks that are usable only experimentally and adapters that cannot yet be reached through supported prerequisite builds.

Existing spell IDs identify Classic counterparts and preserve existing APL actions. New effects without authenticated IDs use `OtherActionForever` (simulator enum 19) and a pinned manifest-index tag; this is explicitly an internal identity, not a fabricated game ID. `ui/forever/data/actions.json` maps those tags to names. For example, find Rage of the Farseer's `action_tag` in `sim/core/foreverdata/trees.json` when constructing an APL action. It is also registered as a major cooldown.

## Scope and fallbacks

- All nine classes and all 27 trees are represented. Importing a talent does not mean its combat effect is executable. Unreviewed selections fail with a named record. The original browser talent picker remains Classic; `ui/forever/discovery.ts` builds versioned raw worker requests for future UI integration.
- Discovery raids with 10, 20 or 40 casters can register all their spell-rank auras on a target. The internal registration guard scales with discovery player count; Classic and player-unit guards remain unchanged. This does not alter active debuff limits or assert a new combat-system rule.
- Generic damage, stat dependencies, spell costs, resource generation, proc dispatch and exclusive-aura behavior retain the closest existing WoWSims implementation unless an observed tooltip establishes a change. These interactions are PROVISIONAL. Existing engine omissions such as unmodeled dispels, PvP control effects, range constraints and poison charge inventory are not evidence that Forever lacks those mechanics.
- Unselected racial and untalented baseline mechanics retain Classic behavior. This is an explicit partial model, not a claim that they are unchanged in Forever. Source omissions do not remove old resistances, Command, or unrelated weapon skills. Selected Human/Orc weapon specialization replaces the corresponding skill racial with ability/spell crit; other legacy racial omissions remain unresolved. Troll Berserking retains Classic cooldown, cost and class-dependent haste calculation with the observed fixed 10% input. Dwarf Beast damage retains the Classic Beast Slaying critical interaction.
- First Aid Kit is a **preapplied, active-throughout-encounter scenario**: +34 Stamina, excluded by Fortitude. Camping activation, the one-hour shared cooldown, seated requirement and unknown duration are not simulated. No item, set bonus, profession combat bonus or new consume formula is invented.
- Demonic Sacrifice uses the inspected two-hour duration, Imp/Shadow, Sayaad/Fire, Voidwalker/Mana and Felhunter/Health mapping. Any demon summon still cancels it. Demonic Pact and Incubus remain blocked. Regeneration retains Classic four-second scheduling. Bloodthirst retains Classic attack outcomes, cost, cooldown and refund while using the observed 35% AP + 30 and movement aura. Enrage's periodic-hit eligibility and Flurry consumption timing remain provisional.
- New Strider Kick data uses the observed tooltip's 61 mana, 8-second cooldown and 100% main-hand weapon damage, without inventing higher spell ranks. Its tree path still requires unfinished adapters. Rage of the Farseer is executable: self-only 30% melee and casting speed for 25 seconds, three-minute cooldown, with the existing off-GCD haste-cooldown convention provisionally retained.
- Several numerical adapters are present but not reachable because required talent paths are unfinished. They remain BLOCKED in completion counts. Healing-spec factories, undocumented spell coefficients, proc chances, detailed pet additions, new items and set bonuses remain separate work.

## Transport/version decision

This is additive, simulator-private protobuf transport v1: optional `Player.forever` at field 49, `ForeverOptions`, and internal action enum 19. Existing field numbers, Classic talent strings and Classic data files are unchanged. No root `shared/` schema or release metadata changed. Classic callers omit the new field and retain baseline behavior. Discovery cannot run through a Classic or empty-baseline game selection. Old strict readers must reject a discovery payload instead of stripping it. Raw core API, stats, weights and worker paths reach the same `NewAgent` validation and preparation, without a mutable global game flag. Callers using raw core APIs retain the request's profile as provenance; the CLI additionally emits the detailed provenance envelope.

The original v1 imported mechanics catalog remains fail-closed for unsupported external mechanics. `-catalog discovery` selects the built-in adapter explicitly; it does not relax that import contract. Requests are cloned before preparation, so searches, repeated simulations and concurrent requests do not mutate caller builds or the shared item database.

## Regeneration and checks

```powershell
node scripts/forever-discovery.mjs --check
node scripts/forever-status.mjs
npm run type-check
npm test
npm run check:classic
```

The last check verifies unchanged protected baseline files and exact reviewed hashes of explicit extension files. It does not assert the modified files are byte-identical to upstream. The existing complete simulator regression suite, targeted combat tests, request isolation tests and invalid-build tests provide behavioral evidence. Do not regenerate upstream `.results` files to make a regression pass.

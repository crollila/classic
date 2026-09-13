# Forever adaptation: baseline milestone

This fork uses the existing [WoWSims Classic simulator](https://github.com/wowsims/classic).
Upstream commit: `7779ebbf79dc7f1341e6ab939b28a3402c9a730a`.
The Go module path and MIT license remain intact to keep upstream merges small.
The existing UI retains its visible upstream attribution and Classic labeling.

**This milestone runs Classic. It does not claim Forever accuracy.**
No mechanics database was present in the supplied workspace. No Forever gameplay
changes are implemented; `sim/forever/baseline.json` contains zero mechanics.
The GitHub description mentions Season of Discovery; the pinned source is the
baseline, not that description or assumptions about Forever. Passing upstream
tests establishes compatibility, not external validation of every upstream rule.

## Run on Windows

From the `classic` checkout, in PowerShell:

```powershell
./scripts/bootstrap-windows.ps1
npm run build
npm test
npm run check:classic
npm start
```

Open <http://127.0.0.1:8080/classic/mage/>. All existing class pages are retained;
upstream class completeness and known issues still apply. The browser runs the
original Classic WebAssembly engine. It does not select Forever yet.

The bootstrap installs Go 1.23.4 in `.tools/`, verifies its archive SHA-256,
installs `protoc-gen-go` v1.36.6, and runs the locked npm installation. Protoc is
pinned to 3.20.3; `tsx` is pinned to 4.19.3. No machine PATH changes are made.
To run Go directly, source `./scripts/env.ps1`. Node >=20 is required; CI uses
20.13.1. Local validation used Node 26.4.0.

On Linux, install Go 1.23.4 and Node 20, run
`go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.6`, put `$(go env GOPATH)/bin`
on PATH, then run `npm ci`, `npm run build`, and `npm test`. Upstream Make targets
remain available. The new npm commands remove the need for Make on Windows.

## Explicit version selection

The original CLI remains at `.artifacts/wowsimcli.exe`. The new CLI delegates to
the same engine and includes game, ruleset, upstream commit, implemented mechanic
IDs, and a baseline-only label in its result envelope:

```powershell
./.artifacts/foreversim.exe -game classic -infile examples/mage-classic.json -outfile .artifacts/classic-mage.json
./.artifacts/foreversim.exe -game forever -catalog sim/forever/baseline.json -infile examples/mage-classic.json -outfile .artifacts/forever-baseline-mage.json
```

Use the filenames without `.exe` on Linux. The example reuses the upstream Mage
pre-BiS equipment, P1 APL and talents, with Mage Armor, no external buffs, a
60-second encounter, 20 iterations, and seed 101. `isTest` enables upstream's
deterministic test RNG streams. Its DPS is a regression fixture, not a Forever
benchmark. Both commands produce equal metrics; action array order is unspecified
because upstream serializes a Go map.

`sim/game` accepts per-request selections. Classic rejects a Forever catalog.
Forever requires an explicit catalog and labels the empty baseline accordingly.
Unknown versions, unknown catalog fields, nonempty unimplemented catalogs, and
unrecognized ruleset revisions fail closed. Forever baseline rejects custom item
databases because upstream inserts them into a global map. Requests are cloned
before simulation. No global version flag, factory replacement, damage scaling,
or item registry mutation is introduced.

## Source map and extension boundaries

| System | Existing implementation | Preservation and adaptation |
| --- | --- | --- |
| Simulation | `sim/core/sim.go`, `environment.go`, `pending_action.go` | Reuse event scheduling, RNG, reset, presims, metrics and concurrency. Never calibrate formulas to target DPS. |
| Classes | `sim/register_all.go`, `sim/<class>/`, specialization subpackages | Factories create `core.Agent` implementations; retain constructors, pets, resources and rotations. Adapt one class at a time. |
| Lifecycle | `sim/core/environment.go`, `character.go`, `raid.go` | Construction precedes racial/gear/talent/buff/consume effects, then spell initialization, finalization, APL and attack tables. Apply changes at the correct phase. |
| Spells | `sim/core/spell.go`, `cast.go`, `spell_result.go`, `spell_outcome.go`, class spell files | Preserve costs, GCD, cooldowns, travel, coefficients, outcomes and damage processing. New spells use `ActionID.SpellID` and `SpellFlagAPL`. |
| Auras/procs | `sim/core/aura.go`, `dot.go`, `exclusive_effect.go` | Reuse activation, expiration, stacks, snapshots, proc masks and callbacks. Labels serve local uniqueness; external identity uses IDs. |
| Attack tables | `sim/core/target.go`, `environment.go`, `spell_outcome.go`, `armor.go` | Tables depend on attacker, defender, weapon skill and level. Future exceptions need isolated, evidence-backed hooks and analytical tests. |
| Talents | `proto/<class>.proto`, `ui/core/talents/`, class `talents.go` | Preserve Classic protobuf fields and positional strings. Map Forever tree/rank IDs explicitly; never append talents to a Classic positional tree. |
| Equipment | `sim/core/database.go`, `database_load.go`, `item_effects.go`, `item_sets.go`, `sim/common/` | Preserve item/enchant IDs and set counting. Database/effect/set registries are global; never overwrite them with Forever data. |
| Racials/buffs | `sim/core/racials.go`, `buffs.go`, `debuffs.go`, `consumes.go` | Reuse valid effects. Future changes must replace/adjust an instance's effect at its proper phase, preventing double application. |
| APL | `sim/core/apl*.go`, `proto/apl.proto`, `ui/<spec>/apls/` | Keep the interpreter, resource conditions, cooldowns, sequences, prepull and class custom actions. Resolve new abilities by ID. |
| Tests | `sim/core/*_test.go`, `test_suite.go`, `test_generators.go`, class tests and `.results` | Keep expected results and tolerances. Suites cover gear/races/APLs/stats/DPS and single/concurrent agreement. |

For example, Mage constructs mana/stat dependencies and parses Classic talents;
`ApplyTalents` adds effects and `OnSpellRegistered` callbacks; `Initialize`
registers ranked spells. Fireball uses its existing cost/cast configuration,
travel callback, hit result and DoT snapshot. Reuse this pipeline rather than
reconstructing it in a database-driven damage calculator.

## Mechanics database integration contract

`sim/forever/catalog.go` defines a **simulator import contract**, pending the actual
database schema. Metadata is not executable code. An exporter must preserve its
source IDs and revision; handlers will be reviewed Go implementations.
`ValidateExecutable` rejects nonempty catalogs until those handlers and tests exist.

Catalog fields: `schema_version`, `ruleset_id`, pinned `upstream_commit`,
`database_revision` (required for nonempty catalogs), and `mechanics`.

Every record requires a unique `mechanic_id`, `kind`, `change` (`new` or
`changed`), confidence (`low`, `medium`, `high`, `verified`), evidence entries
(`source_id`, `locator`, `note`), a `sim/forever/` implementation path, and automated
test references (`file`, `function`). Confidence is preserved metadata; it does not
by itself authorize execution. CI checks shipped references point to real Go tests;
reviewers must still assess evidence and assertion quality.

| Kind | Entity identity | Additional identity |
| --- | --- | --- |
| `talent` | Talent ID in `entity_id` | WoW `class_id`; rank-specific behavior belongs in the adapter |
| `ability` | Spell ID in `entity_id` | WoW `class_id` when applicable |
| `racial` | Racial spell ID in `entity_id` | WoW `race_id` |
| `item` | Item ID in `entity_id` | Optional class scope |
| `set_bonus` | Item-set ID in `entity_id` | `required_pieces` |
| `combat_mechanic` | `mechanic_id`; optional numeric `entity_id` | Explicit scope in the future implementation |

WoW class/race IDs and WoWSims `proto.Class`/`proto.Race` enum values are different
namespaces. Translate explicitly. Names may be used for display, never as
authoritative identity. The validator does not invent a mapping, talent tree,
spell coefficient, proc rate, or Forever ruleset.

## Class-by-class rollout

1. Select the first class and obtain a versioned database export plus evidence.
2. Classify records as unchanged, supported changes, unresolved, or unsupported.
   Report unresolved coverage; do not silently claim full Forever accuracy.
3. Add the adapter beneath `sim/forever/<class>/`, keeping valid Classic behavior.
   Add only small, per-instance extension points required by demonstrated mechanics.
   Avoid global toggles, item-map swaps and factory overrides.
4. Link each mechanic to real evidence and tests for relevant rank boundaries,
   costs, cooldowns, proc eligibility, aura stacking/snapshots, resets and APL visibility.
5. Extend the executable catalog after handlers and tests exist. Add version
   support to stats, stat weights, bulk/concurrent execution and UI before exposing
   those as Forever. They currently remain original Classic features.
6. Keep upstream regressions and Classic/Forever/Classic isolation checks green.
   Review any baseline-gate change. Never regenerate `.results` or weaken
   tolerances simply to make a mismatch disappear.

## Baseline validation

- Full upstream `go test --tags=with_db ./sim/...` passed before version changes.
- TypeScript type checking, WebAssembly and production browser build passed.
- Browser smoke: Mage UI loaded, calculated stats, and completed 20 iterations.
- New tests cover exact Mage metrics parity (unordered action records compared by
  ID), unchanged input, version rejection, item database isolation, catalog kinds,
  missing evidence/tests, strict decoding and CLI provenance.
- CI runs the build, preservation check and tests on Windows and Linux. Local
  validation alone is not a claim that remote CI has already passed.

Build fixes replace stale Bazel npm commands, pin missing build tools, generate
upstream assets without Make, and normalize Vite entry paths on Windows. Go combat
files, Classic protobufs, item data, APL presets, and expected results are unchanged.

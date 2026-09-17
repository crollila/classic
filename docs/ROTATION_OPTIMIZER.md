# Simulation rotation optimizer

The Forever individual simulator now offers **FIND IDEAL ROTATION** and **USE
THIS ROTATION**. It optimizes the exported player, gear, enchants, talents, race,
consumes, buffs and encounter using the existing WoWSims APL interpreter. No LLM
generates the strategy or its score. Select STRICT/BEST_GUESS in Talents first.

The result is the best statistically validated rotation found in a bounded search,
not a proof of a global maximum. A run with no established improvement retains the
current APL. An invalid current APL is reported with the engine's warnings; it is
not silently repaired into a different comparison baseline.

## Search and validation

Seeds include the current resolved APL and the class UI's existing Classic preset
APLs. The current APL includes enabled Forever additions. Three rounds of beam
search keep three leaders, deduplicate candidates, and cap default evaluation at
90 candidates with 128 common-seed training iterations. Mutations swap priorities,
adjust numeric/time/percentage thresholds, gate on resources, execute phase,
targets, known auras/procs and cooldown readiness, relax existing DoT refresh
guards, and explicitly schedule registered cooldowns. Existing nested sequence,
stance/form and supported item-swap actions remain native APL operations; numeric
conditions within them are searchable. There are no invented spell IDs or combat
formulas. Weapon-swap action synthesis is not enabled: the existing individual UI
currently advertises no item-swap-enabled specs.

Each candidate must pass native APL warning checks before scoring. Developer-only
state injection actions are prohibited. Runtime legality remains the engine's
CanCast checks, including costs, talents, cooldowns, form and equipment restrictions.
Actions missing from both the current APL and known preset families are not all
automatically explored; this is deliberately a bounded neighborhood search.

Exactly one training finalist is frozen. It and the baseline run on 32 fresh seed
blocks, 64 iterations each (2,048 per rotation), paired within blocks. Holdout seeds
never overlap training seeds or each other. The standard error is calculated from
paired block-mean differences; a conservative Student-t multiplier of 2.75 gives
a 99% interval for at least 32 blocks. No holdout reranking or significance-driven
early stopping occurs. Promotion requires the lower bound to exceed the configured
minimum DPS gain (default zero). Otherwise report INCONCLUSIVE. More iterations
can be requested through the tool policy, subject to hard budget bounds.

Confidence measures simulation sampling error, not correctness of predicted game
mechanics. Results retain mode, engine/mechanics fingerprints, mechanics evidence,
training seed, holdout seeds and budget. BEST_GUESS predicted dependencies are
disclosed. STRICT uses the existing engine's suppression rules.

## Cache and apply

The bounded 16-result process/session cache includes the entire simulation setup,
resolved current rotation, templates, search policy, optimizer version and content
fingerprints for engine/mechanics (plus browser item database). Changing any of
these misses the cache. Session reload clears it; live player setups are not
persisted by this feature. RNG seed and ordinary simulation iteration settings
are excluded from identity because optimizer sampling has its own pinned policy.

Apply checks that the setup still matches the measured setup, sets the validated
native APL, and disables the automatic Forever prepend overlay so actions are not
duplicated after optimization. Users may continue editing it in the APL editor.
Cancellation takes effect between engine calls; a currently executing browser
batch finishes first. Each batch is bounded.

## God tool boundary

`scripts/rotation-tool.mjs` exports `tool` (name, description, input schema) and
`findIdealRotation(input, { executable, cache?, signal?, onProgress? })`. This is
the simulator-owned God-callable interface; it does not import God internals.
The trusted executable is supplied by the host, never by natural-language tool
arguments. `node scripts/rotation-tool.mjs --describe` prints its descriptor.

Input has `request` (resolved single-player RaidSimRequest protobuf JSON), optional
`templates` (known APLs), `policy` and `seed`. Output includes `apl`, `finalistAPL`,
DPS, confidence, provenance and `explanation` with opening/priority/cooldowns/execute.
God should retrieve/import the setup, call this tool, and explain the returned
facts. It must not fill missing setup fields by inventing player data. Native
ability names come from the checked-in database/discovery catalogue.

Build and run from `classic/`:

```powershell
./.tools/go/bin/go.exe build -tags=with_db -o .artifacts/rotationworker.exe ./cmd/rotationworker
node scripts/rotation-tool.mjs INPUT.json .artifacts/rotationworker.exe OUTPUT.json
node --test scripts/rotation-optimizer.test.mjs scripts/rotation-native.test.mjs
node node_modules/typescript/bin/tsc --noEmit
node scripts/build-forever.mjs
```

`cmd/rotationworker` is a local stdin/stdout bridge to `game.RunRaidSim` and
`core.ComputeStats`. Build with `with_db`; no network service is opened. It
validates Forever discovery selection and supplies authoritative mechanics
provenance. Runtime panics, invalid references, incomplete and nonfinite results
fail closed. God's `rotation_tool.py`, natural-language `ask`, and configured
Discord `sim` route call this interface through a pinned `god-rotation-1` profile.
See `god/ROTATION_TOOL.md`. Configuring a real authorized player profile and
deploying these changes remain operational setup; no live profile is invented.
This change does not deploy services or modify God storage.

Version decision: new simulator-local `rotation-optimizer-1` result format; existing
WoWSims protobuf and shared capture/learning schemas are unchanged. Consumers must
explicitly accept this version. Legacy simulators ignore this separate tool; they
can load its ordinary APL output without a wire-schema migration. No shared
contract change or inferred-mechanics promotion is involved.

## Verified scope

Manufactured mage case: Scorch before Frostbolt, using the existing example gear
and actual Forever engine. Both STRICT and BEST_GUESS pass native optimization
and 2,048-iteration holdout tests. BEST_GUESS measured 157.69 -> 220.31 DPS,
99% paired improvement interval +61.77 to +63.48 DPS. This is an algorithm fixture,
not a player recommendation or a measurement of the live server.

Algorithm tests cover training-only RNG luck, disjoint paired holdout seeds,
repeatability, illegal candidates, invalid baseline, partial/nonfinite simulation
output, cancellation, cache invalidation and human-readable threshold rendering.
Native tests verify the real engine in both modes and rejection of the existing
mage example's unavailable aura/spell references.

Browser QA also passed find/review/apply using an imported manufactured mage
setup, including named abilities, confidence, predicted dependencies and the
"Validated rotation applied" confirmation. Empty raid slots and protobuf's
omitted zero-DPS scalar fields have regression coverage. An existing invalid
auto APL fails visibly with unavailable spell/aura warnings.

Final checks: 16 frontend/optimizer/native JavaScript tests pass; TypeScript,
the Forever build, and Go `./sim/game ./cmd/rotationworker` checks pass. Together
with the God and Discord component checks this run has 79 passing tests and one
optional Discord SDK test skipped. No shared schema was modified.

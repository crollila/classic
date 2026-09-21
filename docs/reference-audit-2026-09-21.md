# WarriorSim comparison and gear evidence audit

Reference: GuybrushGit/WarriorSim commit `ad5ac8b5dd76db3f0fa7c41de52c0b0b60a5a4d8`, especially `js/classes/player.js`, `weapon.js`, `spell.js`, and `simulation.js`. Forever change descriptions: https://foreverchanges.pro/class/warrior. Reference code is comparison evidence, not proof of Forever behavior. No reference implementation was copied.

## Confirmed corrections

- The high-level rage conversion used Go XOR (`level ^ 2`) instead of squaring. Forever now uses the squared-level polynomial: level 60 = 230.60000004. The existing level-20 fit and level-25/40 measured overrides are preserved. Frozen Classic replays deliberately retain their historical formula; this compatibility branch is explicit and isolated. Incoming and outgoing Forever damage use the correction.
- Set membership now accepts ID-only pieces and rejects conflicting known IDs even when names match. Both activation paths count equipped pieces only. Synthetic tests cover zero through four pieces, 2/4-piece thresholds, replacement and removal. Active modeled bonuses are shown on the character sheet.

## Compared interactions

| Area | Comparison and disposition |
| --- | --- |
| Armor | Both use armor/(armor+400+85*attacker level). Reference caps reduction at 75%; current engine does not explicitly cap here. Needs Forever evidence before changing extreme-armor behavior. |
| Hit / dual wield | Weapon-skill miss and suppression concepts agree. Reference dual-wield base is single-wield miss × 0.8 + 20%; ours adds a versioned 19-point penalty. Not identical; retained pending logs. |
| Glancing | Low/high damage multiplier clamps agree. Reference clamps negative level differences in glance chance; ours needs low-level-target edge-case validation. |
| Crit | Level suppression and extra boss suppression have matching common-case structure; engine already documents a remaining low-buff-crit edge case. |
| Weapon damage / haste | Engine separates normalized AP speeds, actual weapon damage and attack timers. No new speed/AP changes justified by this audit. |
| PPM | Both base ordinary proc chance on weapon speed × PPM / 60. Trigger eligibility, exclusions, extra attacks and each individual proc still need evidence. |
| Heroic Strike / Cleave | Both remove the dual-wield miss penalty while queued. Current queue delay remains configurable. |
| Rage on avoided attacks | Reference uses an average-damage/dodge adjustment; engine uses pre-outcome damage and also handles parry. Kept as an unresolved model difference. |
| Buffs / rotation | Externally supplied Sunder and Battle Shout remove self-casts; Expose takes precedence. Existing regression tests remain. |

This is a targeted structural audit, not an exhaustive validation of every spell, item, enchant, proc or correlation. Rotation search is bounded, not a proof of globally maximal DPS.

## Availability and automatic updates

The existing database policy includes equippable client records. This is NOT proof those items can be obtained. Some old phase/source fields were carried from a previous database. AQ/Naxx availability is not established by those records. “Sanctified” in an item name alone also cannot identify its expansion or availability.

Each release now generates a separate UI evidence index from the exact snapshot, extraction notes and pinned legacy catalog. It is tied to the item database by SHA-256. The browser fails closed on missing/mismatched evidence. No manual whitelist of newly added items is needed for this classification step.

- Default: Forever source-listed items (still not live-confirmed).
- Broader presets: client records, or experimental full catalog.
- Independent legacy-catalog and unclassified/new-to-catalog switches.
- Dynamically discovered legacy phase caps for planning only. They are not Forever phases or an announced content roadmap.
- Saved equipped items remain equipped when filters change. Filters control candidate discovery, not arbitrary deletion of a saved build.

Unclassified is intentionally NOT called Forever-exclusive. No reliable per-item origin/availability feed currently proves that distinction. Future official phase/obtainability evidence needs a separately reviewed import; build detection must not turn client presence into confirmation.

## Remaining coverage limits

Client item-set definitions exist in the snapshot but arbitrary new set-bonus spell effects are not generically executed by the existing registry. This update fixes equipment gating for implemented sets, not every new Forever set effect. Set-effect implementations, special proc triggers, some spell formulas and low-level rage fits remain code-backed. Unknown/new bonuses require adapters and tests; absence from “Active modeled sets” does not establish that a live set has no bonus.

No Oracle behavior or watcher registration is changed in this UI/math update. Existing incoming game-data changes are preserved separately and excluded from the release until validated.

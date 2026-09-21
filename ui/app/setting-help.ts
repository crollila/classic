// Describe the implemented controls, not a promise of live Forever availability.
export const SETTING_HELP: Record<string, string> = {
  durationVariation: 'Randomizes each fight around the sidebar fight length by this many seconds in either direction. Duration stays positive.',
  execute20: 'Percentage of the fight spent below 20% target health. Controls execute-phase conditions; it does not reduce target armor.',
  execute35: 'Percentage spent below 35% target health, including the below-20% phase. Used by supported execute-phase conditions.',
  armor: 'Target armor before armor-reducing debuffs. Higher armor reduces physical damage. Use -1 for the level default; 0 really means no base armor.',
  resistance: 'Sets the same base resistance for all modeled magic schools. This affects spell mitigation, not physical armor.',
  targets: 'Total enemies, including the boss. Extra enemies share these target settings and enable supported multi-target attacks.',
  mobType: 'Creature type used by conditional damage, racials and item effects. Unknown does not imply any particular creature family.',
  inFront: 'Allows frontal avoidance such as parries and blocks where applicable. Leave off to attack from behind.',
  tanking: 'Makes this character receive the configured enemy attacks. Required for incoming-damage, avoidance and reactive mechanics.',
  targetSwingMs: 'Milliseconds between enemy auto-attacks. Only matters when the target attacks this character; 1000 ms is one second.',
  targetMinDamage: 'Lower end of enemy swing damage before mitigation. Zero incoming damage cannot generate incoming-damage rage.',
  targetMaxDamage: 'Upper end of enemy swing damage before mitigation. Used with the minimum to define the incoming damage range.',
  reactionMs: 'Fixed player reaction setting used by supported engine mechanics. It is not a blanket delay after every spell or a randomized reaction range.',
  distance: 'Distance to the target in yards, used by supported travel-time mechanics. -1 keeps this spec’s default distance.',
  seed: 'Starting random seed. Reuse the same seed, build, gear and settings to reproduce a simulation. More iterations reduce random noise.',
  profession1: 'First primary profession. Enables modeled profession effects; some also require their corresponding Forever option or equipped item.',
  profession2: 'Second primary profession. Selecting the same profession twice does not mean two copies of its bonuses.',
  startingRage: 'Rage available at the start of the fight, from 0 to 100. This is not continuous rage generation.',
  queueDelay: 'Delay used by the warrior Heroic Strike/Cleave queue model, in milliseconds. It does not change every spell’s queue behavior.',
  sunder: 'Preset leaves Sunder responsibility to the rotation. None removes self-casts and supplies no stacks. External supplies five max-rank stacks (with an initial ramp) and removes all self-casts. Expose Armor takes precedence.',
  giftOfTheWild: 'Increases primary stats, armor and resistances. Improved selects the supported improved provider, not extra stacks.',
  powerWordFortitude: 'Increases stamina and therefore health. Improved selects the stronger provider version.',
  bloodPact: 'Imp-supplied stamina buff. Regular and improved represent the provider’s talent strength.',
  strengthOfEarthTotem: 'Increases strength, affecting attack power and other class-specific strength conversions.',
  graceOfAirTotem: 'Increases agility, affecting crit, dodge and class-specific attack power.',
  arcaneBrilliance: 'Increases intellect, affecting mana and spell crit through this character’s stat conversions.',
  divineSpirit: 'Increases spirit, affecting supported regeneration mechanics.',
  battleShout: 'Externally supplied attack power. Selecting it removes warrior self-casts so the rotation does not spend rage refreshing it. Improved Battle Shout is unavailable in Forever.',
  trueshotAura: 'Externally supplied attack-power aura for supported melee and ranged attacks.',
  furiousHowl: 'Legacy wolf-buff switch. This external-buff field currently has no application handler; do not assume toggling it increases DPS.',
  leaderOfThePack: 'Increases physical critical-strike chance while the external aura is supplied.',
  moonkinAura: 'Increases spell critical-strike chance while the external aura is supplied.',
  manaSpringTotem: 'Restores mana over time. Improved selects a stronger provider; it is not an extra independent totem.',
  blessingOfWisdom: 'Adds mana regeneration. Selecting both copies of this control does not grant duplicate blessings.',
  shadowProtection: 'Increases shadow resistance. Overlapping resistance buffs follow the engine’s stacking rules.',
  shadowResistanceAura: 'Increases shadow resistance; does not reduce incoming physical damage.',
  natureResistanceTotem: 'Increases nature resistance. Overlapping resistance buffs are handled by the engine.',
  aspectOfTheWild: 'Hunter aura that increases nature resistance, not damage.',
  frostResistanceAura: 'Increases frost resistance while the external paladin aura is supplied.',
  frostResistanceTotem: 'Increases frost resistance while the external shaman totem is supplied.',
  fireResistanceTotem: 'Increases fire resistance while the external shaman totem is supplied.',
  fireResistanceAura: 'Increases fire resistance while the external paladin aura is supplied.',
  scrollOfProtection: 'Adds armor from a protection scroll, subject to supported buff stacking.',
  scrollOfStamina: 'Adds stamina from a scroll, increasing health.',
  scrollOfStrength: 'Adds strength from a scroll, affecting class-specific strength conversions.',
  scrollOfAgility: 'Adds agility from a scroll, affecting class-specific agility conversions.',
  scrollOfIntellect: 'Adds intellect from a scroll, affecting mana and spell crit.',
  scrollOfSpirit: 'Adds spirit from a scroll, affecting supported regeneration.',
  thorns: 'Returns nature damage when qualifying enemy attacks hit you. Usually requires taking attacks to contribute damage.',
  devotionAura: 'Adds armor, reducing incoming physical damage rather than increasing your own attacks.',
  stoneskinTotem: 'Reduces incoming physical hit damage through the supported totem model.',
  retributionAura: 'Returns holy damage on qualifying incoming attacks. It is not a passive bonus to outgoing weapon damage.',
  sanctityAura: 'Increases holy damage dealt through the supported external aura.',
  battleSquawk: 'Number of Battle Squawk stacks. Each stack changes attack speed in the engine; this is an assumed external buff, not a guarantee of obtaining the proc.',
  atieshMage: 'Number of mage Atiesh party auras supplying spell crit. Catalog presence does not confirm Atiesh is obtainable in Forever.',
  atieshWarlock: 'Number of warlock Atiesh party auras supplying spell damage. Availability is unconfirmed by this setting.',
  atieshDruid: 'Number of druid Atiesh party auras supplying mana regeneration. Availability is unconfirmed by this setting.',
  atieshPriest: 'Number of priest Atiesh party auras supplying healing power. Availability is unconfirmed by this setting.',
  manaTideTotems: 'Number of external Mana Tide providers available to the engine’s mana cooldown model, not a flat mana bonus.',
  blessingOfKings: 'Increases primary stats by the modeled Kings multiplier. Class stat conversions then use those increased stats.',
  blessingOfMight: 'Increases melee attack power. Improved represents the stronger provider version.',
  innervates: 'Number of external Innervate providers available to the engine’s mana-management model.',
  powerInfusions: 'Number of external Power Infusion providers available to the engine’s cooldown model. Timing follows its supported scheduling.',
  rallyingCryOfTheDragonslayer: 'World buff affecting attack power and physical/spell crit. This toggle assumes you have it; it does not establish Forever availability.',
  saygesFortune: 'Choose one Darkmoon Faire fortune: damage or a primary-stat bonus. Only the selected fortune applies.',
  spiritOfZandalar: 'World buff increasing primary stats. Selecting it is an encounter assumption, not confirmation that the content is released.',
  songflowerSerenade: 'World buff increasing primary stats and critical-strike chance.',
  warchiefsBlessing: 'World buff affecting health, melee speed and regeneration through the engine model. Faction eligibility still applies.',
  fengusFerocity: 'Dire Maul world buff increasing attack power. It does not change spell power.',
  moldarsMoxie: 'Dire Maul world buff increasing stamina and therefore health.',
  slipkiksSavvy: 'Dire Maul world buff increasing spell critical-strike chance.',
  judgementOfWisdom: 'Allows qualifying attacks and spells against the target to restore mana through the modeled proc.',
  judgementOfTheCrusader: 'Increases holy damage taken by the target through the supported judgement effect.',
  faerieFire: 'Reduces target armor. It belongs to a different armor-reduction category from Sunder/Expose.',
  curseOfElements: 'Reduces fire/frost resistance and increases damage taken from the affected schools under the engine model.',
  curseOfShadow: 'Reduces shadow/arcane resistance and increases damage taken from the affected schools.',
  wintersChill: 'Supplies the modeled stacking frost spell-crit vulnerability on the target, rather than spending your own casts to build it.',
  improvedShadowBolt: 'Supplies the engine’s external shadow-damage vulnerability model. It is not a simulated second warlock’s full rotation.',
  improvedScorch: 'Supplies the modeled fire-damage vulnerability stacks on the target.',
  shadowWeaving: 'Supplies the modeled shadow-damage vulnerability stacks on the target.',
  stormstrike: 'Supplies the engine’s nature-damage vulnerability model. External uptime is an assumption, not another shaman’s simulated rotation.',
  exposeArmor: 'Major armor reduction supplied by another player. Takes precedence over external Sunder and removes Sunder casts from your warrior rotation.',
  curseOfWeakness: 'Reduces the target’s physical damage dealt. Useful when modeling incoming attacks, not as a direct outgoing damage bonus.',
  curseOfRecklessness: 'Reduces target armor but increases its attack power: more outgoing physical damage can come with more incoming damage.',
  demoralizingRoar: 'Reduces target attack power, lowering incoming melee damage. Competing attack-power reductions follow engine exclusivity rules.',
  demoralizingShout: 'Reduces target attack power. Improved Demoralizing Shout is not offered for Forever.',
  thunderClap: 'Slows the target’s attack speed through the supported debuff, affecting incoming swing frequency.',
  thunderfury: 'Supplies the supported Thunderfury attack-speed reduction, not the weapon’s damage proc or ownership.',
  insectSwarm: 'Reduces the target’s chance to hit with qualifying attacks. This toggle does not add another druid’s damage-over-time casts.',
  scorpidSting: 'Coverage gap: this external-debuff switch registers an aura but currently applies no stat reduction. Do not assume it changes incoming damage.',
  huntersMark: 'Adds ranged attack power against this target. It does not grant the same benefit to melee attacks.',
  giftOfArthas: 'Makes qualifying physical hits against the target receive the modeled flat damage bonus.',
  crystalYield: 'Supplies a crystal armor-reduction debuff; competing armor reductions follow engine stacking rules.',
  flask: 'Choose a flask for health, mana, spell power or resistances. Values use supported Forever overrides; catalog availability is not guaranteed.',
  food: 'Choose one food buff for its primary-stat or mana-regeneration effect. Food choices do not stack with each other in this control.',
  agilityElixir: 'Select an agility consumable; Mongoose also adds physical crit in the implemented model.',
  manaRegenElixir: 'Select a consumable that adds mana restored per five seconds.',
  strengthBuff: 'Select a strength consumable. Its benefit depends on the character’s strength-to-attack-power conversion.',
  attackPowerBuff: 'Select an attack-power consumable, separate from strength conversions.',
  spellPowerBuff: 'Select a general spell-power consumable for supported spells.',
  shadowPowerBuff: 'Adds shadow-specific spell power, not power for every magic school.',
  firePowerBuff: 'Adds fire-specific spell power, not power for every magic school.',
  frostPowerBuff: 'Adds frost-specific spell power, not power for every magic school.',
  fillerExplosive: 'Select a repeatable explosive for the engine’s cooldown scheduling. Target caps, cooldowns and profession requirements depend on the implemented item.',
  sapperExplosive: 'Select a sapper explosive for supported cooldown scheduling. Explosive availability and requirements still need Forever coverage review.',
  mainHandImbue: 'Temporary main-hand weapon effect: oil, stone, poison or class imbue. Requires applicable equipment and supported class behavior; does not equip a weapon.',
  offHandImbue: 'Temporary off-hand weapon effect. No off-hand weapon means no off-hand weapon benefit; two-handed builds cannot also use an off-hand weapon.',
  defaultPotion: 'Choose the potion available to the engine’s cooldown logic. It is not automatically consumed at every possible instant. Some listed protection potions have no implemented handler.',
  defaultConjured: 'Choose a conjured resource/healing item for cooldown scheduling. Thistle Tea is listed by the schema but lacks a handler in this engine.',
  petAgilityConsumable: 'Pet agility selection: 0 none, 1 +17, 2 +13, 3 +9, 4 +5 agility. These are selection codes, not a number of stacked consumables.',
  petStrengthConsumable: 'Pet strength selection: 0 none, 1 +30, 2 +17, 3 +13, 4 +9, 5 +5 strength. These are selection codes, not stacks.',
  petAttackPowerConsumable: 'Pet attack-power selection: 0 none, 1 +40 attack power. This does not improve the owner’s attack power.',
  dragonBreathChili: 'Enables the supported melee-triggered fire-damage proc from Dragonbreath Chili.',
  zanzaBuff: 'Choose one Zanza/Blasted Lands/event buff. The engine catalog includes legacy choices; listing is not confirmation of live availability.',
  armorElixir: 'Select an armor consumable to reduce incoming physical damage.',
  healthElixir: 'Select a maximum-health consumable. It does not heal ongoing damage each second.',
  alcohol: 'Select an alcohol buff. Different drinks affect stamina or spirit/intellect; effects follow the implemented item.',
  hitConsumable: 'Choose the supported hit-chance consumable; this is not weapon skill or expertise.',
  boglingRoot: 'Adds the supported flat physical damage bonus. This legacy consumable may not be available in Forever.',
  jujuEmber: 'Increases fire resistance.',
  jujuChill: 'Increases frost resistance.',
  jujuEscape: 'Makes the supported temporary dodge cooldown available; requires incoming attacks to affect outcomes.',
  jujuFlurry: 'Makes the supported attack-speed cooldown available. The nested pet version currently has no application handler and must not be assumed to improve pet DPS.',
  raptorPunch: 'Trades spirit for intellect through the implemented consumable buff.',
  'Deep rotation search before DPS (slower)': 'Searches supported rotation candidates before DPS simulation. Holds your gear, talents and encounter fixed; slower and not a proof of the global maximum.',
  Level: 'Character level determines available ranks, talents and level-dependent conversions. Changing it also updates the default target level.',
  'Target level': 'Enemy level affects hit, avoidance and other combat-table calculations. It is separate from your character level.',
  Race: 'Selects racial base stats and supported racial abilities for this class.',
  Rotation: 'Starting action-priority preset. Rotation search compares supported alternatives for the current setup.',
  'Fight length (s)': 'Average fight duration in seconds; Settings variation randomizes individual fights around it.',
  Iterations: 'Number of fights for equipped DPS. More fights reduce random noise but take longer; they do not improve incomplete mechanics.',
  'Item sim iterations': 'Fights per gear candidate. Higher values improve ranking precision but make full-slot comparisons slower.',
  'Parallel gear simulations': 'Number of gear candidates processed concurrently. Auto estimates CPU capacity and reserves room for responsiveness. High manual values use more memory and can be slower.',
};

export function settingHelp(key: string): string {
  return SETTING_HELP[key] || 'This setting comes from the current engine catalog. Its detailed behavior has not yet been documented; do not assume it is validated for Forever.';
}

let nextHelpId = 0;
export function addSettingHelp(row: HTMLElement, input: HTMLElement, label: string, text: string) {
  const tip = document.createElement('div'); tip.className = 'fa-setting-tooltip';
  tip.id = `fa-setting-help-${++nextHelpId}`; tip.setAttribute('role', 'tooltip'); tip.textContent = text; tip.hidden = true;
  const help = document.createElement('button'); help.type = 'button'; help.className = 'fa-help-button'; help.textContent = '?';
  help.setAttribute('aria-label', `Help: ${label}`); help.setAttribute('aria-expanded', 'false'); help.setAttribute('aria-controls', tip.id);
  input.setAttribute('aria-describedby', tip.id); help.setAttribute('aria-describedby', tip.id);
  for (const control of input.querySelectorAll('input, select, textarea')) control.setAttribute('aria-describedby', tip.id);
  // The native title remains a fallback; the visible panel also works by keyboard/touch.
  input.title = text;
  let pinned = false;
  const show = () => {
    tip.hidden = false;
    const rect = row.getBoundingClientRect();
    const width = Math.min(340, window.innerWidth - 24);
    tip.style.width = `${width}px`;
    tip.style.left = `${Math.max(12, Math.min(rect.left, window.innerWidth - width - 12))}px`;
    tip.style.top = `${Math.max(12, Math.min(rect.bottom + 6, window.innerHeight - tip.offsetHeight - 12))}px`;
  };
  const close = () => { pinned = false; tip.hidden = true; help.setAttribute('aria-expanded', 'false'); };
  row.onmouseenter = show;
  row.onmouseleave = () => { if (!pinned) tip.hidden = true; };
  row.addEventListener('focusin', show);
  row.addEventListener('focusout', event => { if (!row.contains(event.relatedTarget as Node)) close(); });
  row.onkeydown = event => { if (event.key === 'Escape') { close(); event.stopPropagation(); } };
  help.onclick = event => { event.preventDefault(); event.stopPropagation(); pinned = !pinned; help.setAttribute('aria-expanded', String(pinned)); if (pinned) show(); else tip.hidden = true; };
  row.append(help, tip);
}

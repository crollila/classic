package foreverdata

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

// ParameterSpec describes one bounded Forever parameter.
type ParameterSpec struct {
	Key     string  `json:"key"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Label   string  `json:"label,omitempty"`
	Default string  `json:"default,omitempty"` // human description of the Classic default
	Source  string  `json:"source"`            // "trees.json" or "core"
}

// Classes that register each centrally known stat conversion in their class package.
var (
	agilityCritClasses   = []string{"warrior", "paladin", "hunter", "rogue", "shaman", "warlock", "druid"}
	intellectCritClasses = []string{"paladin", "hunter", "priest", "shaman", "mage", "warlock", "druid"}
	allClasses           = []string{"warrior", "paladin", "hunter", "rogue", "priest", "shaman", "mage", "warlock", "druid"}
)

// Core parameter keys. Every hook defaults to the unchanged Classic behaviour and
// is only consulted for Forever-mode players.
const (
	ParamGlancingDamageMultiplier  = "core.combat.glancing_damage_multiplier"
	ParamMeleeCritDamageMultiplier = "core.combat.melee_crit_damage_multiplier"
	ParamSpellCritDamageMultiplier = "core.combat.spell_crit_damage_multiplier"

	// Attack table parameters (player attacking an enemy). The offsets are additive
	// residuals on top of the Classic formula result, in chance units (0.01 = 1%).
	ParamDualWieldMissPenalty       = "core.combat.dual_wield_miss_penalty"
	ParamMeleeMissOffset            = "core.combat.melee_miss_offset"
	ParamDodgeOffset                = "core.combat.dodge_offset"
	ParamParryOffset                = "core.combat.parry_offset"
	ParamGlancingChanceOffset       = "core.combat.glancing_chance_offset"
	ParamMeleeCritSuppressionOffset = "core.combat.melee_crit_suppression_offset"
	ParamSpellMissOffset            = "core.combat.spell_miss_offset"
)

// AttackTableOffsetParams lists the additive attack table residual parameters.
var AttackTableOffsetParams = []string{ParamMeleeMissOffset, ParamDodgeOffset, ParamParryOffset,
	ParamGlancingChanceOffset, ParamMeleeCritSuppressionOffset, ParamSpellMissOffset}

// ConversionParam returns "core.conversion.<class>.<conversion>".
// conversion is one of agility_per_melee_crit, intellect_per_spell_crit,
// attack_power_per_strength, attack_power_per_agility.
func ConversionParam(class, conversion string) string {
	return "core.conversion." + class + "." + conversion
}

var coreParameters = func() []ParameterSpec {
	out := []ParameterSpec{
		{Key: ParamGlancingDamageMultiplier, Min: 0, Max: 1, Source: "core",
			Label:   "Fixed glancing blow damage multiplier (0 = Classic weapon-skill formula)",
			Default: "0 (Classic formula: min=clamp(1.3-0.05*skillDiff,0.01,0.91), max=clamp(1.2-0.03*skillDiff,0.2,0.99))"},
		{Key: ParamMeleeCritDamageMultiplier, Min: 1, Max: 5, Source: "core",
			Label: "Base melee/ranged critical strike damage multiplier", Default: "2.0"},
		{Key: ParamSpellCritDamageMultiplier, Min: 1, Max: 5, Source: "core",
			Label: "Base spell critical strike damage multiplier", Default: "1.5"},
		{Key: ParamDualWieldMissPenalty, Min: 0, Max: 0.5, Source: "core",
			Label: "Miss chance added to dual wielding players' melee attacks", Default: "0.19"},
	}
	for _, o := range []struct{ key, label string }{
		{ParamMeleeMissOffset, "base melee/ranged miss chance"},
		{ParamDodgeOffset, "enemy dodge chance"},
		{ParamParryOffset, "enemy parry chance"},
		{ParamGlancingChanceOffset, "glancing blow chance"},
		{ParamMeleeCritSuppressionOffset, "melee crit suppression"},
		{ParamSpellMissOffset, "base spell miss chance (the 1% floor is kept)"},
	} {
		out = append(out, ParameterSpec{Key: o.key, Min: -0.25, Max: 0.25, Source: "core",
			Label: "Additive offset to the Classic " + o.label + " against enemies; result clamped to >= 0", Default: "0 (Classic formula)"})
	}
	for _, c := range agilityCritClasses {
		out = append(out, ParameterSpec{Key: ConversionParam(c, "agility_per_melee_crit"), Min: 1, Max: 1000, Source: "core",
			Label: "Agility per 1% melee crit", Default: "1 / core.CritPerAgiAtLevel[class]"})
	}
	for _, c := range intellectCritClasses {
		out = append(out, ParameterSpec{Key: ConversionParam(c, "intellect_per_spell_crit"), Min: 1, Max: 1000, Source: "core",
			Label: "Intellect per 1% spell crit", Default: "1 / core.CritPerIntAtLevel[class]"})
	}
	for _, c := range allClasses {
		out = append(out, ParameterSpec{Key: ConversionParam(c, "attack_power_per_strength"), Min: 0, Max: 10, Source: "core",
			Label: "Melee attack power per Strength", Default: "core.APPerStrength[class]"})
		out = append(out, ParameterSpec{Key: ConversionParam(c, "attack_power_per_agility"), Min: 0, Max: 10, Source: "core",
			Label: "Melee attack power per Agility", Default: "1 for hunter and rogue, otherwise 0"})
	}
	return out
}()

// ParameterCatalog lists every registered parameter: trees.json entries plus core hooks.
func ParameterCatalog() []ParameterSpec {
	out := []ParameterSpec{}
	seen := map[string]bool{}
	for _, p := range data.Parameters {
		if !seen[p.Key] {
			seen[p.Key] = true
			out = append(out, ParameterSpec{Key: p.Key, Min: p.Min, Max: p.Max, Source: "trees.json"})
		}
	}
	for _, p := range coreParameters {
		if !seen[p.Key] {
			seen[p.Key] = true
			out = append(out, p)
		}
	}
	return out
}

// ParameterInBounds validates a parameter key and value exactly like request validation.
func ParameterInBounds(key string, value float64) bool {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return false
	}
	if strings.HasPrefix(key, "scenario.enemy_health.") {
		index, err := strconv.Atoi(strings.TrimPrefix(key, "scenario.enemy_health."))
		if err == nil && index >= 0 && index < 40 && value >= 0 && value <= 1e12 {
			return true
		}
	}
	for _, p := range data.Parameters {
		if p.Key == key && value >= p.Min && value <= p.Max {
			return true
		}
	}
	for _, p := range coreParameters {
		if p.Key == key && value >= p.Min && value <= p.Max {
			return true
		}
	}
	return false
}

// SupportedKey documents one override path and whether the simulator applies it automatically.
type SupportedKey struct {
	Path        string   `json:"path"`
	Kinds       []string `json:"kinds,omitempty"`
	AutoApplied bool     `json:"auto_applied"`
	Conditions  string   `json:"conditions"`
}

// SpellValueHook is a (spell, effect) pair consumed by hand-written sim code via
// Unit.ForeverSpellValue. For these pairs spells.<id>.effects.<i>.base (any kind
// except weapon_damage) is applied as a ratio to the Classic constant.
type SpellValueHook struct {
	SpellID     int32  `json:"spell_id"`
	EffectIndex int    `json:"effect_index"`
	Consumer    string `json:"consumer"`
}

// SpellValueHooks is the catalog of retrofitted consumables/buffs. core tests
// check that every Unit.ForeverSpellValue call site is listed here.
var SpellValueHooks = []SpellValueHook{
	// Flasks
	{17626, 0, "Flask of the Titans health"}, {17627, 0, "Flask of Distilled Wisdom mana"},
	{17628, 0, "Flask of Supreme Power spell power"}, {17629, 0, "Flask of Chromatic Resistance resistances"},
	// Food and drink
	{19709, 0, "Hot Wolf Ribs stamina"}, {19709, 1, "Hot Wolf Ribs spirit"},
	{25694, 0, "Smoked Sagefish mp5"}, {25941, 0, "Sagefish Delight mp5"},
	{19710, 0, "Tender Wolf Steak stamina"}, {19710, 1, "Tender Wolf Steak spirit"},
	{18192, 0, "Grilled Squid agility"}, {24799, 0, "Smoked Desert Dumplings strength"},
	{18194, 0, "Nightfin Soup mp5"}, {22730, 0, "Runn Tum Tuber Surprise intellect"},
	{25661, 0, "Dirge's Kickin' Chimaerok Chops stamina"}, {18141, 0, "Blessed Sunfruit Juice spirit"},
	{18125, 0, "Blessed Sunfruit strength"},
	{25804, 0, "Rumsey Rum Black Label stamina"}, {22789, 0, "Gordok Green Grog stamina"},
	{25722, 0, "Rumsey Rum Dark stamina"}, {25037, 0, "Rumsey Rum Light stamina"},
	{22790, 0, "Kreeg's Stout Beatdown spirit"}, {22790, 1, "Kreeg's Stout Beatdown intellect"},
	// Defensive elixirs
	{11348, 0, "Elixir of Superior Defense armor"}, {11349, 0, "Elixir of Greater Defense armor"},
	{3220, 0, "Elixir of Defense armor"}, {673, 0, "Elixir of Minor Defense armor"},
	{3593, 0, "Elixir of Fortitude health"}, {2378, 0, "Elixir of Minor Fortitude health"},
	// Physical
	{16329, 0, "Juju Might attack power"}, {16329, 1, "Juju Might ranged attack power"},
	{17038, 0, "Winterfall Firewater attack power"},
	{17538, 0, "Elixir of the Mongoose agility"}, {17538, 1, "Elixir of the Mongoose crit"},
	{11334, 0, "Elixir of Greater Agility agility"}, {11328, 0, "Elixir of Agility agility"},
	{3160, 0, "Elixir of Lesser Agility agility"},
	{16323, 0, "Juju Power strength"}, {11405, 0, "Elixir of the Giants strength"},
	{3164, 0, "Elixir of Ogre's Strength strength"},
	// Spell
	{11390, 0, "Arcane Elixir spell damage"}, {17539, 0, "Greater Arcane Elixir spell damage"},
	{7844, 0, "Elixir of Firepower fire power"}, {26276, 0, "Elixir of Greater Firepower fire power"},
	{11474, 0, "Elixir of Shadow Power shadow power"}, {21920, 0, "Elixir of Frost Power frost power"},
	{24363, 0, "Mageblood Potion mp5"},
	// Zanza
	{24382, 0, "Spirit of Zanza spirit"}, {24382, 1, "Spirit of Zanza stamina"},
	{10667, 0, "R.O.I.D.S. strength"}, {10669, 0, "Ground Scorpok Assay agility"},
	{10692, 0, "Cerebral Cortex Compound intellect"}, {10693, 0, "Gizzard Gum spirit"},
	{10668, 0, "Lung Juice Cocktail stamina"},
	// Hit
	{29332, 0, "Fire-toasted Bun hit"}, {27723, 0, "Dark Desire hit"},
	// Misc
	{5665, 0, "Bogling Root bonus physical damage"},
	{6114, 0, "Raptor Punch intellect"}, {6114, 1, "Raptor Punch stamina"},
	{16326, 0, "Juju Ember fire resistance"}, {16325, 0, "Juju Chill frost resistance"},
	// Raid buffs (BuffSpellValues)
	{23028, 0, "Arcane Brilliance intellect"}, {27841, 0, "Divine Spirit spirit"},
	{21850, 0, "Gift of the Wild armor"}, {21850, 1, "Gift of the Wild attributes"}, {21850, 2, "Gift of the Wild resistances"},
	{10938, 0, "Power Word: Fortitude stamina"}, {11767, 0, "Blood Pact stamina"},
	{25289, 0, "Battle Shout (AQ) attack power"}, {11551, 0, "Battle Shout attack power"},
	{25291, 0, "Blessing of Might (AQ) attack power"}, {19838, 0, "Blessing of Might attack power"},
	{25290, 0, "Blessing of Wisdom (AQ) mp5"}, {19854, 0, "Blessing of Wisdom mp5"},
	{25362, 0, "Strength of Earth (AQ) strength"}, {10441, 0, "Strength of Earth strength"},
	{25360, 0, "Grace of Air (AQ) agility"}, {10626, 0, "Grace of Air agility"},
	{10494, 0, "Mana Spring mp5"}, {10293, 0, "Devotion Aura armor"},
	{12174, 0, "Scroll of Agility IV agility"}, {12176, 0, "Scroll of Intellect IV intellect"},
	{12177, 0, "Scroll of Spirit IV spirit"}, {12178, 0, "Scroll of Stamina IV stamina"},
	{12179, 0, "Scroll of Strength IV strength"}, {12175, 0, "Scroll of Protection IV armor"},
	{20906, 0, "Trueshot Aura ranged attack power"}, {20906, 1, "Trueshot Aura attack power"},
	{24932, 0, "Leader of the Pack melee crit"}, {24907, 0, "Moonkin Aura spell crit"},
	{20217, 0, "Blessing of Kings attribute percent"},
	// World buffs
	{22888, 0, "Rallying Cry spell crit"}, {22888, 1, "Rallying Cry attack power"},
	{22888, 2, "Rallying Cry melee crit"}, {22888, 3, "Rallying Cry ranged attack power"},
	{24425, 2, "Spirit of Zandalar attribute percent"},
	{15366, 0, "Songflower Serenade melee crit"}, {15366, 1, "Songflower Serenade attributes"}, {15366, 2, "Songflower Serenade spell crit"},
	{16609, 0, "Warchief's Blessing health"}, {16609, 1, "Warchief's Blessing melee haste percent"}, {16609, 2, "Warchief's Blessing mp5"},
	{22817, 0, "Fengus' Ferocity attack power"}, {22817, 1, "Fengus' Ferocity ranged attack power"},
	{22818, 0, "Mol'dar's Moxie stamina percent"}, {22820, 0, "Slip'kik's Savvy spell crit"},
}

// HasSpellValueHook reports whether hand-written sim code consumes this effect.
func HasSpellValueHook(spellID int32, effectIndex int) bool {
	return slices.ContainsFunc(SpellValueHooks, func(h SpellValueHook) bool {
		return h.SpellID == spellID && h.EffectIndex == effectIndex
	})
}

// SupportedOverrideKeys describes exactly which override fields are auto-applicable.
// Anything not auto-applied should be routed to "implementation candidates".
func SupportedOverrideKeys() []SupportedKey {
	magic := "Forever mode only; spell registered with this ActionID.SpellID on a Forever player or its pet"
	return []SupportedKey{
		{Path: "spells.<id>.effects.<i>.coefficient", Kinds: []string{KindSchoolDamage, KindHeal}, AutoApplied: true,
			Conditions: magic + "; exactly one impact effect in the override; registered Spell.BonusCoefficient equals classic within 1e-3; school_damage requires DefenseType magic, heal requires a non melee/ranged DefenseType"},
		{Path: "spells.<id>.effects.<i>.coefficient", Kinds: []string{KindPeriodicDamage, KindPeriodicHeal}, AutoApplied: true,
			Conditions: magic + "; exactly one periodic effect in the override; registered Dot (periodic_damage) or Hot (periodic_heal) BonusCoefficient equals classic within 1e-3"},
		{Path: "spells.<id>.effects.<i>.base (+ optional max)", Kinds: []string{KindSchoolDamage, KindHeal}, AutoApplied: true,
			Conditions: magic + "; classic and forever present (and for max too, when given) with non-zero classic average; multiplies the base passed to CalcDamage/CalcHealing by avg(forever)/avg(classic) before spell power is added; rejected for melee/ranged DefenseType or more than one impact effect"},
		{Path: "spells.<id>.effects.<i>.base (+ optional max)", Kinds: []string{KindPeriodicDamage, KindPeriodicHeal}, AutoApplied: true,
			Conditions: magic + "; same as above applied in CalcPeriodicDamage, Dot.Snapshot and Dot.SnapshotHeal"},
		{Path: "spells.<id>.effects.<i>.base (+ optional max)", Kinds: []string{KindApplyAura, KindOther, KindSchoolDamage, KindHeal, KindPeriodicDamage, KindPeriodicHeal}, AutoApplied: true,
			Conditions: "only for (spell, effect) pairs in SpellValueHooks (consumables, raid and world buffs): ratio avg(forever)/avg(classic) applied to the sim's Classic constant; classic null is rejected"},
		{Path: "spells.<id>.effects.<i>.*", Kinds: []string{KindWeaponDamage}, AutoApplied: false,
			Conditions: "weapon damage effects are never auto-applied (weapon damage is not separable from base damage in the engine)"},
		{Path: "spells.<id>.effects.<i>.coefficient", Kinds: []string{KindApplyAura, KindOther}, AutoApplied: false,
			Conditions: "no generic coefficient slot for auras/other effects"},
		{Path: "spells.<id>.cast_ms", AutoApplied: true,
			Conditions: magic + "; spell uses the default cast-time function; registered DefaultCast.CastTime equals classic within 0.5 ms (talent-modified configs will not match); delta forever-classic added; result must be >= 0"},
		{Path: "spells.<id>.cooldown_ms", AutoApplied: true,
			Conditions: magic + "; spell has its own CD timer; registered CD duration equals classic within 0.5 ms; result must be > 0"},
		{Path: "spells.<id>.cost (power mana)", AutoApplied: true,
			Conditions: magic + "; flat mana cost (ManaCostOptions.FlatCost, no BaseCost percentage) equals classic within 1e-3; result must be > 0"},
		{Path: "spells.<id>.cost (power rage|energy)", AutoApplied: true,
			Conditions: magic + "; RageCost.Cost / EnergyCost.Cost equals classic within 1e-3; result must be > 0"},
		{Path: "spells.<id>.duration_ms", AutoApplied: true,
			Conditions: magic + "; for a spell Dot (else Hot): ticks*tickLength equals classic and forever is a positive multiple of tickLength (tick count changes); otherwise for auras registered on the Forever player/pet units with this SpellID whose Duration equals classic"},
		{Path: "items.<id>.stats.<statKey>", AutoApplied: true,
			Conditions: "Forever mode only; equipped item base stats (not random suffix/enchant) equal classic within 1e-3; delta applied. statKey in StatKeys (db.json stats units)"},
		{Path: "items.<id>.weapon.min|max|speed_ms", AutoApplied: true,
			Conditions: "Forever mode only; equipped item WeaponDamageMin/Max or speed*1000 equals classic (1e-3 / 0.5 ms)"},
		{Path: "items.<id>.new_item", AutoApplied: true,
			Conditions: "Forever requests only; used only when the id is absent from the Classic item database; UIItem JSON as in assets/database/db.json; not available to item swap"},
		{Path: "talents.<recordId>.values", AutoApplied: true,
			Conditions: "rank count == max_rank and per-rank value counts equal the embedded record; replaces Lookup() rank values"},
		{Path: "parameters.<key>.value", AutoApplied: true,
			Conditions: "key in ParameterCatalog() (trees.json or core hooks) and value inside bounds; becomes the default used when the request does not set the parameter"},
		{Path: "*.provenance.status == PREDICTED", AutoApplied: false,
			Conditions: "rejected for players in STRICT Forever mode; applied in DEFAULT/BEST_GUESS"},
	}
}

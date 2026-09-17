package main

// Normalization: everything that is not the spec itself is identical (or
// equivalent by a documented archetype rule) across specs.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
	googleProto "google.golang.org/protobuf/proto"
)

type Normalization struct {
	WorldBuffs    bool   `json:"world_buffs"`
	ConsumesTier  string `json:"consumes_tier"`
	RacePolicy    string `json:"race_policy"`
	TargetLevel   int32  `json:"target_level"`
	Seed          int64  `json:"seed"`
	Iterations    int32  `json:"iterations"`
	AutoRotation  string `json:"forever_auto_rotation"`
	ABIterations  int32  `json:"auto_rotation_ab_iterations"`
	RaceIters     int32  `json:"race_search_iterations,omitempty"`
	TalentPolicy  string `json:"forever_talent_policy"`
	ForeverMode   string `json:"forever_mode"`
	GearFill      string `json:"gear_fill"`
	ReactionMs    int32  `json:"reaction_time_ms"`
	ChannelClipMs int32  `json:"channel_clip_delay_ms"`
}

const (
	reactionTimeMs     = 150
	channelClipDelayMs = 50
)

var worldBuffFields = []string{"rallying_cry_of_the_dragonslayer", "sayges_fortune", "spirit_of_zandalar", "songflower_serenade",
	"warchiefs_blessing", "fengus_ferocity", "moldars_moxie", "slipkiks_savvy"}

// raidBuffs returns the buffs every spec receives: the repository's full raid
// (core.FullBuffs: every blessing, totem, aura and debuff from a mixed-faction
// 40-man raid), with world buffs removed unless worldBuffs is set.
func raidBuffs(worldBuffs bool) (*proto.RaidBuffs, *proto.PartyBuffs, *proto.IndividualBuffs, *proto.Debuffs) {
	ind := googleProto.Clone(core.FullIndividualBuffs).(*proto.IndividualBuffs)
	if !worldBuffs {
		ind.RallyingCryOfTheDragonslayer = false
		ind.SaygesFortune = proto.SaygesFortune_SaygesUnknown
		ind.SpiritOfZandalar = false
		ind.SongflowerSerenade = false
		ind.WarchiefsBlessing = false
		ind.FengusFerocity = false
		ind.MoldarsMoxie = false
		ind.SlipkiksSavvy = false
	}
	return googleProto.Clone(core.FullRaidBuffs).(*proto.RaidBuffs),
		googleProto.Clone(core.FullPartyBuffs).(*proto.PartyBuffs),
		ind,
		googleProto.Clone(core.FullDebuffs).(*proto.Debuffs)
}

var archetypeDoc = map[string]string{
	"melee":           "physical melee (fury warrior, rogues, feral cat)",
	"hybrid_melee":    "melee that also scales with spell power (enhancement, retribution)",
	"ranged_physical": "hunter",
	"caster":          "spell casters (mage, warlock, shadow priest, balance, elemental)",
	"tank_melee":      "physical tanks (protection warrior, bear)",
	"tank_hybrid":     "tanks that scale with spell power (protection paladin, warden)",
}

func isManaUser(c proto.Class) bool {
	return c != proto.Class_ClassWarrior && c != proto.Class_ClassRogue
}

// consumesFor returns the consumables for a spec. tier: standard | max | none.
//
// standard, every DPS archetype: the same damage elixir set (Elixir of the
// Mongoose, Juju Power, Juju Might, Greater Arcane Elixir, Elixir of Greater
// Firepower, Elixir of Frost Power, Elixir of Shadow Power); elixirs have no
// slot limit in Classic, so stats a spec cannot use are simply wasted.
// Slot-exclusive items are the best-in-slot equivalent for the archetype:
//
//	flask:   Supreme Power (caster, hybrid_melee), Titans (melee, hunter, tanks: no physical DPS flask exists)
//	food:    Runn Tum Tuber Surprise (caster), Grilled Squid (hunter), Smoked Desert Dumpling (others)
//	         + Dragonbreath Chili for non-casters
//	weapon:  Brilliant Wizard Oil (caster); Windfury Totem main hand (melee/hybrid/tank physical; shamans use
//	         Windfury Weapon, wardens Rockbiter); Elemental Sharpening Stone off hand (rogues: Instant Poison)
//	potion:  Mighty Rage Potion (warriors), Major Mana Potion (mana users); Demonic Rune + Mageblood Potion
//	         for mana users
//
// tanks additionally get Elixir of Superior Defense and Elixir of Fortitude.
// max adds the Zanza buff (R.O.I.D.S. for non-casters, Cerebral Cortex Compound for casters).
func consumesFor(v Variant, tier string) *proto.Consumes {
	if tier == "none" {
		return &proto.Consumes{}
	}
	c := &proto.Consumes{
		AgilityElixir:   proto.AgilityElixir_ElixirOfTheMongoose,
		StrengthBuff:    proto.StrengthBuff_JujuPower,
		AttackPowerBuff: proto.AttackPowerBuff_JujuMight,
		SpellPowerBuff:  proto.SpellPowerBuff_GreaterArcaneElixir,
		FirePowerBuff:   proto.FirePowerBuff_ElixirOfGreaterFirepower,
		FrostPowerBuff:  proto.FrostPowerBuff_ElixirOfFrostPower,
		ShadowPowerBuff: proto.ShadowPowerBuff_ElixirOfShadowPower,
	}
	caster := v.Archetype == "caster"
	switch v.Archetype {
	case "caster", "hybrid_melee", "tank_hybrid":
		c.Flask = proto.Flask_FlaskOfSupremePower
	default:
		c.Flask = proto.Flask_FlaskOfTheTitans
	}
	switch v.Archetype {
	case "caster":
		c.Food = proto.Food_FoodRunnTumTuberSurprise
		c.MainHandImbue = proto.WeaponImbue_BrilliantWizardOil
	case "ranged_physical":
		c.Food = proto.Food_FoodGrilledSquid
		c.DragonBreathChili = true
		c.MainHandImbue = proto.WeaponImbue_ElementalSharpeningStone
		c.OffHandImbue = proto.WeaponImbue_ElementalSharpeningStone
	default:
		c.Food = proto.Food_FoodSmokedDesertDumpling
		c.DragonBreathChili = true
		c.MainHandImbue = proto.WeaponImbue_Windfury
		c.OffHandImbue = proto.WeaponImbue_ElementalSharpeningStone
	}
	switch {
	case v.ID == "shaman-warden":
		c.MainHandImbue = proto.WeaponImbue_RockbiterWeapon
	case v.Class == proto.Class_ClassShaman && !caster:
		c.MainHandImbue = proto.WeaponImbue_WindfuryWeapon
	case v.Class == proto.Class_ClassRogue:
		c.OffHandImbue = proto.WeaponImbue_InstantPoison
	}
	if strings.HasPrefix(v.Archetype, "tank") {
		c.ArmorElixir = proto.ArmorElixir_ElixirOfSuperiorDefense
		c.HealthElixir = proto.HealthElixir_ElixirOfFortitude
	}
	if v.Class == proto.Class_ClassWarrior {
		c.DefaultPotion = proto.Potions_MightyRagePotion
	}
	if isManaUser(v.Class) {
		c.DefaultPotion = proto.Potions_MajorManaPotion
		c.DefaultConjured = proto.Conjured_ConjuredDemonicRune
		c.ManaRegenElixir = proto.ManaRegenElixir_MagebloodPotion
	}
	if tier == "max" {
		if caster {
			c.ZanzaBuff = proto.ZanzaBuff_CerebralCortexCompound
		} else {
			c.ZanzaBuff = proto.ZanzaBuff_ROIDS
		}
	}
	return c
}

// ---------------------------------------------------------------- races

// Classic (1.12) race/class legality. Forever legality comes from
// ui/forever/data/talents.json race_classes.
var classicRaces = map[proto.Class][]proto.Race{
	proto.Class_ClassWarrior: {proto.Race_RaceHuman, proto.Race_RaceDwarf, proto.Race_RaceNightElf, proto.Race_RaceGnome, proto.Race_RaceOrc, proto.Race_RaceUndead, proto.Race_RaceTauren, proto.Race_RaceTroll},
	proto.Class_ClassPaladin: {proto.Race_RaceHuman, proto.Race_RaceDwarf},
	proto.Class_ClassHunter:  {proto.Race_RaceDwarf, proto.Race_RaceNightElf, proto.Race_RaceOrc, proto.Race_RaceTauren, proto.Race_RaceTroll},
	proto.Class_ClassRogue:   {proto.Race_RaceHuman, proto.Race_RaceDwarf, proto.Race_RaceNightElf, proto.Race_RaceGnome, proto.Race_RaceOrc, proto.Race_RaceUndead, proto.Race_RaceTroll},
	proto.Class_ClassPriest:  {proto.Race_RaceHuman, proto.Race_RaceDwarf, proto.Race_RaceNightElf, proto.Race_RaceUndead, proto.Race_RaceTroll},
	proto.Class_ClassShaman:  {proto.Race_RaceOrc, proto.Race_RaceTauren, proto.Race_RaceTroll},
	proto.Class_ClassMage:    {proto.Race_RaceHuman, proto.Race_RaceGnome, proto.Race_RaceUndead, proto.Race_RaceTroll},
	proto.Class_ClassWarlock: {proto.Race_RaceHuman, proto.Race_RaceGnome, proto.Race_RaceOrc, proto.Race_RaceUndead},
	proto.Class_ClassDruid:   {proto.Race_RaceNightElf, proto.Race_RaceTauren},
}

func raceName(r proto.Race) string { return strings.TrimPrefix(r.String(), "Race") }

// raceCandidates returns the races allowed in Forever for the class, restricted
// to Classic-legal races when the Classic game is also simmed.
func (d *fvDataset) raceCandidates(c proto.Class, needClassic bool) []proto.Race {
	className := strings.ToUpper(strings.TrimPrefix(c.String(), "Class"))
	out := []proto.Race{}
	for race, key := range foreverRaceKeys {
		allowed := false
		for _, cn := range d.RaceClasses[key] {
			allowed = allowed || cn == className
		}
		if !allowed {
			continue
		}
		if needClassic {
			legal := false
			for _, r := range classicRaces[c] {
				legal = legal || r == race
			}
			if !legal {
				continue
			}
		}
		out = append(out, race)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (d *fvDataset) raceAllowed(c proto.Class, r proto.Race, needClassic bool) bool {
	for _, x := range d.raceCandidates(c, needClassic) {
		if x == r {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- encounters

type EncounterSpec struct {
	Name     string  `json:"name"`
	Weight   float64 `json:"weight"`
	Targets  int     `json:"targets"`
	Duration float64 `json:"duration_s"`
}

var encounterMixes = map[string][]EncounterSpec{
	"first-raid": {
		{Name: "single-target", Weight: 0.60, Targets: 1, Duration: 180},
		{Name: "cleave-3", Weight: 0.25, Targets: 3, Duration: 120},
		{Name: "aoe-5", Weight: 0.15, Targets: 5, Duration: 60},
	},
	"single-target": {{Name: "single-target", Weight: 1, Targets: 1, Duration: 180}},
	"cleave":        {{Name: "cleave-3", Weight: 1, Targets: 3, Duration: 120}},
	"aoe":           {{Name: "aoe-5", Weight: 1, Targets: 5, Duration: 60}},
}

var encounterMixDocs = map[string]string{
	"first-raid":    "projected first raid (Molten Core style): 60% single-target bosses (180 s), 25% 2-3 target pulls/adds (3 targets, 120 s), 15% trash packs (5 targets, 60 s)",
	"single-target": "100% single target, 180 s",
	"cleave":        "100% three targets, 120 s",
	"aoe":           "100% five targets, 60 s",
}

// parseEncounterMix accepts a preset name, an inline JSON array, or a path to a JSON file.
func parseEncounterMix(value string, readFile func(string) ([]byte, error)) (string, []EncounterSpec, error) {
	if mix, ok := encounterMixes[value]; ok {
		return value, mix, nil
	}
	raw := []byte(value)
	name := "custom"
	if !strings.HasPrefix(strings.TrimSpace(value), "[") {
		data, err := readFile(value)
		if err != nil {
			return "", nil, fmt.Errorf("-encounters %q is not a preset (%s), JSON array, or readable file: %w", value, strings.Join(mixNames(), ", "), err)
		}
		raw, name = data, "file:"+value
	}
	var mix []EncounterSpec
	if err := json.Unmarshal(raw, &mix); err != nil {
		return "", nil, fmt.Errorf("-encounters: %w", err)
	}
	if len(mix) == 0 {
		return "", nil, fmt.Errorf("-encounters: empty mix")
	}
	total := 0.0
	for i := range mix {
		e := &mix[i]
		if e.Targets < 1 || e.Targets > 40 || e.Duration <= 0 || e.Duration > 3600 || e.Weight <= 0 {
			return "", nil, fmt.Errorf("-encounters entry %d: need weight>0, targets 1..40, duration_s in (0,3600]", i)
		}
		if e.Name == "" {
			e.Name = fmt.Sprintf("encounter-%d", i+1)
		}
		total += e.Weight
	}
	for i := range mix {
		mix[i].Weight /= total
	}
	return name, mix, nil
}

func mixNames() []string {
	out := []string{}
	for k := range encounterMixes {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// normalizationHash identifies the settings a talent build was optimized under:
// everything in the normalization except seed and iteration counts (the
// optimizer uses its own), plus the preset snapshot (gear, APLs).
func normalizationHash(n Normalization) string {
	key := map[string]interface{}{
		"world_buffs": n.WorldBuffs, "consumes": n.ConsumesTier, "race_policy": n.RacePolicy, "race_iterations": n.RaceIters,
		"target_level": n.TargetLevel, "auto_rotation": n.AutoRotation, "forever_mode": n.ForeverMode, "gear_fill": n.GearFill,
		"reaction_ms": n.ReactionMs, "channel_clip_ms": n.ChannelClipMs, "preset_snapshot": presetSnapshotSHA(), "apls": embeddedAPLsSHA(),
	}
	raw, _ := json.Marshal(key)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

// normalizationDoc renders the normalization for output metadata.
func normalizationDoc(n Normalization, role string) map[string]interface{} {
	raid, party, ind, debuffs := raidBuffs(n.WorldBuffs)
	pj := func(m googleProto.Message) json.RawMessage {
		b, _ := protojson.Marshal(m)
		return normalizeJSON(b)
	}
	consumes := map[string]json.RawMessage{}
	for _, v := range variants {
		if v.Unavailable != "" || (role != "all" && v.Role != role) {
			continue
		}
		consumes[v.ID] = pj(consumesFor(v, n.ConsumesTier))
	}
	worldBuffs := "off: " + strings.Join(worldBuffFields, ", ") + " removed from every spec"
	if n.WorldBuffs {
		worldBuffs = "on: " + strings.Join(worldBuffFields, ", ") + " applied to every spec"
	}
	return map[string]interface{}{
		"settings":             n,
		"raid_buffs":           pj(raid),
		"party_buffs":          pj(party),
		"individual_buffs":     pj(ind),
		"debuffs":              pj(debuffs),
		"buff_policy":          "core.FullBuffs (mixed-faction 40-man raid: all blessings, totems, auras, debuffs) for every spec; party buffs empty",
		"world_buffs":          worldBuffs,
		"consumes_policy":      "identical damage elixir set for all DPS; flask/food/weapon/potion slot filled with the archetype equivalent (see consumes_by_spec); tier " + n.ConsumesTier,
		"consumes_archetypes":  archetypeDoc,
		"consumes_by_spec":     consumes,
		"professions":          "profession1 Engineering, profession2 none for every spec",
		"player":               fmt.Sprintf("reaction_time_ms %d, channel_clip_delay_ms %d, distance_from_target from the UI spec defaults (melee 5)", reactionTimeMs, channelClipDelayMs),
		"encounter":            fmt.Sprintf("level %d boss target(s) from core.DefaultTargetProtoLvl60 (repo test target: base armor 1104 before debuffs, demon, 2.0 s swing) with level overridden, execute phases 20/25/35%%; same targets/duration for every spec", n.TargetLevel),
		"random_seed_policy":   fmt.Sprintf("seed %d reused for every spec and both games (common random numbers)", n.Seed),
		"race_policy":          racePolicyDoc[n.RacePolicy],
		"gear_policy":          "same content phase => the spec's UI BiS set for that phase; without a complete own set the spec borrows a same-armor, same-role spec's set for the phase (gear_status borrowed:<spec>; two-handed specs get a phase two-hander) and is ranked; otherwise it is excluded (gear_status missing / incomplete) unless -gear-fill previous fills an incomplete set from the previous phase set (gear_status patched); blank gear never used",
		"talent_policy":        "Classic: the UI talent preset, else the optimized Classic build (presets/classic_builds.json, -classic-builds). Forever: the optimized/genuine Forever build (presets/forever_builds.json, -forever-builds; exact phase first, else nearest phase), else the -forever-talent-policy derivation from the Classic talents",
		"normalization_hash":   normalizationHash(n),
		"auto_rotation_policy": autoRotationDoc[n.AutoRotation],
	}
}

var racePolicyDoc = map[string]string{
	"fixed": "fixed: one documented race per spec (the conventional best throughput race, legal for the class in both Forever and Classic; see `foreverbench list`)",
	"best":  "best: every race legal for the class in Forever (and in Classic when Classic is simmed) is simmed single-target in the primary game at race_search_iterations; the highest-DPS race is used for all runs of that spec",
}

var autoRotationDoc = map[string]string{
	"conservative": "conservative: each new Forever action the UI would prepend to the APL is A/B tested (same seed, auto_rotation_ab_iterations) in priority order and kept only if DPS does not drop",
	"ui":           "ui: every new Forever action the UI would prepend is added (Forever web UI behaviour)",
	"off":          "off: the UI Classic APL is used unchanged in Forever",
}

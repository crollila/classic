package main

// Preset matrix: spec variant x content phase, built from the UI presets
// (ui/<spec>/presets.ts) snapshotted by extract_presets.mjs into
// presets/ui_presets.json and embedded in the binary.

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/wowsims/classic/assets/database"
	"github.com/wowsims/classic/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

//go:embed presets/ui_presets.json
var uiPresetsJSON []byte

//go:embed presets/forever_builds.json
var foreverBuildsJSON []byte

//go:embed presets/classic_builds.json
var classicBuildsJSON []byte

//go:embed presets/apls/*.apl.json
var embeddedAPLs embed.FS

// Phase is a Classic content phase. P0 is pre-raid BiS (dungeon/crafted gear).
type Phase int

var phaseLabels = []string{
	"P0 pre-raid BiS (dungeons, crafted, quest)",
	"P1 Molten Core / Onyxia",
	"P2 Dire Maul / world bosses",
	"P3 Blackwing Lair",
	"P4 Zul'Gurub",
	"P5 Ahn'Qiraj",
	"P6 Naxxramas",
}

func (p Phase) String() string { return fmt.Sprintf("P%d", int(p)) }
func (p Phase) Label() string  { return phaseLabels[p] }

func allPhases() []Phase {
	out := []Phase{}
	for i := range phaseLabels {
		out = append(out, Phase(i))
	}
	return out
}

func parsePhase(s string) (Phase, error) {
	t := strings.ToLower(strings.TrimSpace(s))
	switch t {
	case "prebis", "pre-bis", "pre-raid", "preraid":
		return 0, nil
	}
	t = strings.TrimPrefix(strings.TrimPrefix(t, "phase"), "p")
	n, err := strconv.Atoi(strings.TrimSpace(t))
	if err != nil || n < 0 || n >= len(phaseLabels) {
		return 0, fmt.Errorf("unknown phase %q (use P0..P%d)", s, len(phaseLabels)-1)
	}
	return Phase(n), nil
}

// ---------------------------------------------------------------- snapshot

type uiSnapshot struct {
	Schema int `json:"schema"`
	Specs  map[string]struct {
		Exports      map[string]json.RawMessage `json:"exports"`
		GearSetFiles []string                   `json:"gear_set_files"`
		AplFiles     []string                   `json:"apl_files"`
	} `json:"specs"`
	Files map[string]json.RawMessage `json:"files"`
}

var (
	snapOnce sync.Once
	snap     *uiSnapshot
	snapErr  error
)

func uiPresets() (*uiSnapshot, error) {
	snapOnce.Do(func() {
		snap = &uiSnapshot{}
		snapErr = json.Unmarshal(uiPresetsJSON, snap)
	})
	return snap, snapErr
}

func presetSnapshotSHA() string {
	sum := sha256.Sum256(uiPresetsJSON)
	return hex.EncodeToString(sum[:])
}

// embeddedAPLsSHA hashes the embedded foreverbench APL files (name + content).
func embeddedAPLsSHA() string {
	h := sha256.New()
	entries, _ := embeddedAPLs.ReadDir("presets/apls")
	for _, e := range entries {
		data, _ := embeddedAPLs.ReadFile("presets/apls/" + e.Name())
		h.Write([]byte(e.Name()))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

type presetRef struct {
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	File          string `json:"file"`
	TalentsString string `json:"talentsString"`
	Data          *struct {
		TalentsString string `json:"talentsString"`
	} `json:"data"`
}

func (p presetRef) talents() string {
	if p.TalentsString != "" {
		return p.TalentsString
	}
	if p.Data != nil {
		return p.Data.TalentsString
	}
	return ""
}

// collectRefs walks a preset export (object keyed by phase, array, or single
// preset) and returns every preset object with its phase key ("" if none).
func collectRefs(raw json.RawMessage) []presetRef {
	out := []presetRef{}
	var one presetRef
	if json.Unmarshal(raw, &one) == nil && one.Name != "" {
		return append(out, one)
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil {
		for _, item := range list {
			out = append(out, collectRefs(item)...)
		}
		return out
	}
	var byKey map[string]json.RawMessage
	if json.Unmarshal(raw, &byKey) == nil {
		keys := make([]string, 0, len(byKey))
		for k := range byKey {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = append(out, collectRefs(byKey[k])...)
		}
	}
	return out
}

// ---------------------------------------------------------------- variants

// Variant is one rankable spec configuration. Everything except the spec
// identity (talents, APL, spec options, gear by phase) comes from the UI presets,
// optimized talent builds (presets/*_builds.json) or documented fallbacks.
type Variant struct {
	ID         string
	Class      proto.Class
	Spec       string
	Role       string // dps | tank
	UIDir      string
	OneofField string // proto Player.spec oneof field name
	Talents    string // UI talent preset name ("" = none: optimized Classic build from presets/classic_builds.json)
	APL        string // UI APL preset name, or a label for APLFile
	APLFile    string // APL used instead of a UI APL preset: "<ui dir>/apls/x.apl.json" or "foreverbench/apls/x.apl.json" (embedded)
	GearFilter string // substring the gear file name must contain (variants sharing a UI dir)
	Archetype  string // consumables archetype, see normalize.go
	FixedRace  proto.Race
	RaceWhy    string
	// GearFrom borrows every phase's gear from another spec (gear_status
	// borrowed:<spec>); GearDonors are only used for phases without a complete
	// own set. TwoHander replaces the donor's main hand and clears the off hand.
	GearFrom   string
	GearDonors []string
	TwoHander  map[Phase]int32
	// Talent search identity (optimize-talents): at least FocusMin points in
	// tree FocusTree (tree index; same tree order in both games), plus required
	// and forbidden talents (Forever record ids / Classic field names).
	FocusTree      int
	FocusMin       int32
	RequireForever []string
	RequireClassic []string
	ForbidForever  []string
	ForbidClassic  []string
	// Unavailable is set when the spec cannot be simmed; such variants are
	// listed but never simmed.
	Unavailable string
	Notes       []string
}

// twoHanders is the main hand used when a two-handed spec borrows the Fury
// warrior dual-wield set (swords/maces for the Human racial where possible).
var twoHanders = map[Phase]int32{
	0: 12784, // Arcanite Reaper (crafted)
	1: 17076, // Bonereaver's Edge (Molten Core)
	2: 17076,
	3: 19364, // Ashkandi, Greatsword of the Brotherhood (BWL)
	4: 19364,
	5: 21134, // Dark Edge of Insanity (AQ40)
	6: 22798, // Might of Menethil (Naxxramas)
}

// Embedded APLs (presets/apls) for specs without a fitting UI APL:
//   - warrior_arms.apl.json: the UI Fury "DPS (With Reck)" APL with Bloodthirst replaced by
//     Mortal Strike (21553), plus Sweeping Strikes before Whirlwind on multi-target pulls.
//   - rogue_subtlety.apl.json: the UI combat APL (Slice and Dice, cooldowns, Eviscerate) with
//     Hemorrhage (17348) as builder; Sinister Strike only when Hemorrhage is not known.
//   - mage_arcane.apl.json: autocast cooldowns (Arcane Power, Presence of Mind), Arcane
//     Missiles channel, Frostbolt fallback.
var variants = []Variant{
	{ID: "warrior-fury", Class: proto.Class_ClassWarrior, Spec: "Fury", Role: "dps", UIDir: "warrior", OneofField: "warrior",
		Talents: "DPS", APL: "DPS (With Reck)", Archetype: "melee", FixedRace: proto.Race_RaceHuman, FocusTree: 1, FocusMin: 31,
		RaceWhy: "UI default race; sword/mace weapon skill with the sword-based BiS lists",
		Notes:   []string{"UI 'DPS' talent preset (Fury); 'DPS (With Reck)' APL (the UI default 'No Reck' APL never uses Recklessness)."}},
	{ID: "warrior-arms", Class: proto.Class_ClassWarrior, Spec: "Arms", Role: "dps", UIDir: "warrior", OneofField: "warrior",
		APL: "foreverbench Arms (Fury APL with Mortal Strike)", APLFile: "foreverbench/apls/warrior_arms.apl.json", Archetype: "melee",
		FixedRace: proto.Race_RaceHuman, RaceWhy: "sword/mace weapon skill with the borrowed two-handers", FocusTree: 0, FocusMin: 31,
		GearFrom: "warrior-fury", TwoHander: twoHanders,
		Notes: []string{"No Arms talent preset or APL in the UI: talents are optimized (presets/classic_builds.json), the APL is presets/apls/warrior_arms.apl.json, gear is the Fury set with a two-hander."}},
	{ID: "rogue-combat-swords", Class: proto.Class_ClassRogue, Spec: "Combat (Swords)", Role: "dps", UIDir: "rogue", OneofField: "rogue",
		Talents: "Sinister Strike", APL: "Sinister Strike", GearFilter: "combat_sinister_strike", Archetype: "melee", FixedRace: proto.Race_RaceHuman,
		RaceWhy: "sword specialization racial with Sinister Strike sword gear", FocusTree: 1, FocusMin: 31},
	{ID: "rogue-combat-daggers", Class: proto.Class_ClassRogue, Spec: "Combat (Daggers)", Role: "dps", UIDir: "rogue", OneofField: "rogue",
		Talents: "Backstab", APL: "Backstab", GearFilter: "combat_backstab", Archetype: "melee", FixedRace: proto.Race_RaceOrc,
		RaceWhy: "no dagger racial exists; Blood Fury is the strongest generic melee racial", FocusTree: 1, FocusMin: 31},
	{ID: "rogue-assassination", Class: proto.Class_ClassRogue, Spec: "Assassination", Role: "dps", UIDir: "rogue", OneofField: "rogue",
		APL: "Backstab", APLFile: "rogue/apls/combat_backstab.apl.json", Archetype: "melee", FixedRace: proto.Race_RaceOrc,
		RaceWhy: "dagger build; Blood Fury is the strongest generic melee racial", FocusTree: 0, FocusMin: 31, GearFrom: "rogue-combat-daggers",
		Notes: []string{"No Assassination talent preset in the UI: talents are optimized; UI Backstab APL (Cold Blood is autocast); Backstab dagger gear."}},
	{ID: "rogue-subtlety", Class: proto.Class_ClassRogue, Spec: "Subtlety", Role: "dps", UIDir: "rogue", OneofField: "rogue",
		APL: "foreverbench Hemorrhage", APLFile: "foreverbench/apls/rogue_subtlety.apl.json", Archetype: "melee", FixedRace: proto.Race_RaceOrc,
		RaceWhy: "Blood Fury is the strongest generic melee racial", FocusTree: 2, FocusMin: 31, GearFrom: "rogue-combat-daggers",
		Notes: []string{"No Subtlety talent preset or APL in the UI: talents are optimized, the APL is presets/apls/rogue_subtlety.apl.json (Hemorrhage builder); Backstab dagger gear."}},
	{ID: "hunter-marksmanship", Class: proto.Class_ClassHunter, Spec: "Marksmanship", Role: "dps", UIDir: "hunter", OneofField: "hunter",
		Talents: "Marksmanship", APL: "Marksmanship", Archetype: "ranged_physical", FixedRace: proto.Race_RaceTroll,
		RaceWhy: "UI default race (bow specialization, Berserking)", FocusTree: 1, FocusMin: 31},
	{ID: "hunter-beast-mastery", Class: proto.Class_ClassHunter, Spec: "Beast Mastery", Role: "dps", UIDir: "hunter", OneofField: "hunter",
		APL: "Marksmanship", APLFile: "hunter/apls/p1.apl.json", Archetype: "ranged_physical", FixedRace: proto.Race_RaceTroll,
		RaceWhy: "UI default hunter race (bow specialization, Berserking)", FocusTree: 0, FocusMin: 31, GearFrom: "hunter-marksmanship",
		Notes: []string{"No Beast Mastery talent preset in the UI: talents are optimized; the UI hunter APL (shared shot rotation) and Marksmanship gear are used."}},
	{ID: "hunter-survival", Class: proto.Class_ClassHunter, Spec: "Survival", Role: "dps", UIDir: "hunter", OneofField: "hunter",
		APL: "Marksmanship", APLFile: "hunter/apls/p1.apl.json", Archetype: "ranged_physical", FixedRace: proto.Race_RaceTroll,
		RaceWhy: "UI default hunter race (bow specialization, Berserking)", FocusTree: 2, FocusMin: 31, GearFrom: "hunter-marksmanship",
		Notes: []string{"No Survival talent preset in the UI: talents are optimized; the UI hunter APL (shared shot rotation) and Marksmanship gear are used."}},
	{ID: "mage-frost", Class: proto.Class_ClassMage, Spec: "Frost", Role: "dps", UIDir: "mage", OneofField: "mage",
		Talents: "Frost DPS", APL: "DPS", Archetype: "caster", FixedRace: proto.Race_RaceTroll,
		RaceWhy: "Berserking is the only throughput racial available to mages", FocusTree: 2, FocusMin: 31},
	{ID: "mage-fire", Class: proto.Class_ClassMage, Spec: "Fire", Role: "dps", UIDir: "mage", OneofField: "mage",
		APL: "DPS", APLFile: "mage/apls/p1.apl.json", Archetype: "caster", FixedRace: proto.Race_RaceTroll,
		RaceWhy: "Berserking is the only throughput racial available to mages", FocusTree: 1, FocusMin: 31, GearFrom: "mage-frost",
		Notes: []string{"No Fire talent preset in the UI: talents are optimized; the UI mage APL casts Scorch/Fireball/Combustion once Combustion is known; Frost gear."}},
	{ID: "mage-arcane", Class: proto.Class_ClassMage, Spec: "Arcane", Role: "dps", UIDir: "mage", OneofField: "mage",
		APL: "foreverbench Arcane Missiles", APLFile: "foreverbench/apls/mage_arcane.apl.json", Archetype: "caster", FixedRace: proto.Race_RaceTroll,
		RaceWhy: "Berserking is the only throughput racial available to mages", FocusTree: 0, FocusMin: 31, GearFrom: "mage-frost",
		Notes: []string{"No Arcane talent preset or APL in the UI: talents are optimized, the APL is presets/apls/mage_arcane.apl.json; Frost gear."}},
	{ID: "warlock-affliction", Class: proto.Class_ClassWarlock, Spec: "Affliction (SM/Ruin)", Role: "dps", UIDir: "warlock", OneofField: "warlock",
		Talents: "SM/Ruin", APL: "Destruction", Archetype: "caster", FixedRace: proto.Race_RaceOrc,
		RaceWhy: "Blood Fury / Command (pet damage); no caster racial is strictly better in Classic", FocusTree: 0, FocusMin: 30,
		Notes: []string{"The UI has one warlock APL (named 'Destruction', a Shadow Bolt rotation) shared by all warlock specs."}},
	{ID: "warlock-demonology", Class: proto.Class_ClassWarlock, Spec: "Demonology (DS/Ruin)", Role: "dps", UIDir: "warlock", OneofField: "warlock",
		Talents: "DS/Ruin", APL: "Destruction", Archetype: "caster", FixedRace: proto.Race_RaceOrc,
		RaceWhy: "Blood Fury / Command (pet damage); no caster racial is strictly better in Classic", FocusTree: 1, FocusMin: 21,
		RequireForever: []string{"warlock.talent.demonic-sacrifice"}, RequireClassic: []string{"demonicSacrifice"}},
	{ID: "warlock-destruction", Class: proto.Class_ClassWarlock, Spec: "Destruction", Role: "dps", UIDir: "warlock", OneofField: "warlock",
		APL: "Destruction", APLFile: "warlock/apls/rotation.apl.json", Archetype: "caster", FixedRace: proto.Race_RaceOrc,
		RaceWhy: "Blood Fury / Command (pet damage); no caster racial is strictly better in Classic", FocusTree: 2, FocusMin: 31, GearFrom: "warlock-affliction",
		ForbidForever: []string{"warlock.talent.demonic-sacrifice"}, ForbidClassic: []string{"demonicSacrifice"},
		Notes: []string{"No deep-Destruction talent preset in the UI: talents are optimized with 31+ Destruction and without Demonic Sacrifice (DS builds are ranked as demonology); UI warlock APL and gear."}},
	{ID: "priest-shadow", Class: proto.Class_ClassPriest, Spec: "Shadow", Role: "dps", UIDir: "shadow_priest", OneofField: "shadow_priest",
		Talents: "Shadow", APL: "Shadow", Archetype: "caster", FixedRace: proto.Race_RaceTroll,
		RaceWhy: "Berserking is the only throughput racial available to priests", FocusTree: 2, FocusMin: 31, GearDonors: []string{"warlock-affliction"}},
	{ID: "druid-balance", Class: proto.Class_ClassDruid, Spec: "Balance", Role: "dps", UIDir: "balance_druid", OneofField: "balance_druid",
		Talents: "Balance", APL: "Default Balance", Archetype: "caster", FixedRace: proto.Race_RaceTauren,
		RaceWhy: "neither druid race has a throughput racial; Tauren matches the repo tests", FocusTree: 0, FocusMin: 31},
	{ID: "druid-feral-cat", Class: proto.Class_ClassDruid, Spec: "Feral (Cat)", Role: "dps", UIDir: "feral_druid", OneofField: "feral_druid",
		Talents: "Feral", APL: "Feral", Archetype: "melee", FixedRace: proto.Race_RaceTauren,
		RaceWhy: "neither druid race has a throughput racial; Tauren matches the repo tests", FocusTree: 1, FocusMin: 31},
	{ID: "shaman-elemental", Class: proto.Class_ClassShaman, Spec: "Elemental", Role: "dps", UIDir: "elemental_shaman", OneofField: "elemental_shaman",
		Talents: "Level 60", APL: "Default", Archetype: "caster", FixedRace: proto.Race_RaceTroll,
		RaceWhy: "Berserking is the only caster throughput racial available to shamans", FocusTree: 0, FocusMin: 31},
	{ID: "shaman-enhancement", Class: proto.Class_ClassShaman, Spec: "Enhancement", Role: "dps", UIDir: "enhancement_shaman", OneofField: "enhancement_shaman",
		Talents: "Level 60", APL: "Default", Archetype: "hybrid_melee", FixedRace: proto.Race_RaceOrc,
		RaceWhy: "UI default race (axe specialization, Blood Fury)", FocusTree: 1, FocusMin: 31},
	{ID: "paladin-retribution", Class: proto.Class_ClassPaladin, Spec: "Retribution", Role: "dps", UIDir: "retribution_paladin", OneofField: "retribution_paladin",
		Talents: "P4/P5 Ret", APL: "Basic Ret", Archetype: "hybrid_melee", FixedRace: proto.Race_RaceHuman,
		RaceWhy: "sword/mace weapon skill", FocusTree: 2, FocusMin: 31, GearDonors: []string{"warrior-fury"}, TwoHander: twoHanders,
		Notes: []string{"The UI ships only blank retribution gear: the Fury warrior strength plate set is borrowed with a two-hander (gear_status borrowed:warrior-fury)."}},
	{ID: "warrior-protection", Class: proto.Class_ClassWarrior, Spec: "Protection", Role: "tank", UIDir: "tank_warrior", OneofField: "tank_warrior",
		Talents: "Protection", APL: "DPS (With Reck)", Archetype: "tank_melee", FixedRace: proto.Race_RaceHuman,
		RaceWhy: "UI default race", FocusTree: 2, FocusMin: 31,
		Notes: []string{"The tank warrior UI only ships DPS-style APLs; tank rankings compare threat-agnostic damage."}},
	{ID: "paladin-protection", Class: proto.Class_ClassPaladin, Spec: "Protection", Role: "tank", UIDir: "protection_paladin", OneofField: "protection_paladin",
		Talents: "P5 Prot", APL: "P5 Prot", Archetype: "tank_hybrid", FixedRace: proto.Race_RaceHuman, RaceWhy: "sword/mace weapon skill", FocusTree: 1, FocusMin: 31},
	{ID: "shaman-warden", Class: proto.Class_ClassShaman, Spec: "Warden", Role: "tank", UIDir: "warden_shaman", OneofField: "warden_shaman",
		Talents: "Level 60", APL: "Default", Archetype: "tank_hybrid", FixedRace: proto.Race_RaceTroll, RaceWhy: "matches the repo tests", FocusTree: 1, FocusMin: 24},
	{ID: "druid-feral-tank", Class: proto.Class_ClassDruid, Spec: "Feral (Bear)", Role: "tank", UIDir: "feral_tank_druid", OneofField: "feral_tank_druid",
		Talents: "Standard", APL: "Default", Archetype: "tank_melee", FixedRace: proto.Race_RaceTauren, RaceWhy: "Endurance (+5% health)", FocusTree: 1, FocusMin: 31},
}

func findVariant(id string) (Variant, bool) {
	for _, v := range variants {
		if v.ID == id {
			return v, true
		}
	}
	return Variant{}, false
}

// ---------------------------------------------------------------- matrix

type PresetEntry struct {
	Spec          string   `json:"spec"`
	Class         string   `json:"class"`
	SpecLabel     string   `json:"spec_label"`
	Role          string   `json:"role"`
	Phase         string   `json:"phase"`
	Status        string   `json:"status"`      // ok | missing_gear | incomplete_gear | unavailable (| no_talents in rankings)
	GearStatus    string   `json:"gear_status"` // ok | borrowed:<spec> | patched | missing | incomplete | n/a
	GearLabel     string   `json:"gear_label,omitempty"`
	GearFile      string   `json:"gear_file,omitempty"`
	GearSource    string   `json:"gear_source,omitempty"`
	AvgIlvl       float64  `json:"avg_ilvl,omitempty"`
	EpicSlots     int      `json:"epic_slots,omitempty"`
	FilledSlots   int      `json:"filled_slots,omitempty"`
	TalentPreset  string   `json:"talent_preset,omitempty"`
	TalentsString string   `json:"classic_talents,omitempty"`
	TalentSource  string   `json:"classic_talent_source,omitempty"` // ui:<preset> | optimized:<date> | none
	APLPreset     string   `json:"apl_preset,omitempty"`
	APLFile       string   `json:"apl_file,omitempty"`
	Reason        string   `json:"reason,omitempty"`
	Alternatives  []string `json:"other_gear_sets_this_phase,omitempty"`
	BorrowedFrom  string   `json:"gear_borrowed_from,omitempty"`
	OwnGearStatus string   `json:"own_gear_status,omitempty"`

	PatchedFrom  string   `json:"patched_from,omitempty"`
	PatchedSlots []string `json:"patched_slots,omitempty"`

	variant     Variant
	specOptions json.RawMessage
	distance    float64
	gearJSON    json.RawMessage // set for patched/borrowed gear; otherwise the snapshot file is used
	patch       *gearPatch
}

// borrowGear returns the donor gear set, with the main hand replaced by a
// two-hander (keeping the donor main-hand enchant) and the off hand cleared
// when twoHander is non-zero.
func borrowGear(raw json.RawMessage, twoHander int32) (json.RawMessage, error) {
	if twoHander == 0 {
		return raw, nil
	}
	type item = map[string]interface{}
	var g map[string]interface{}
	if err := json.Unmarshal(raw, &g); err != nil {
		return nil, err
	}
	items, _ := g["items"].([]interface{})
	for len(items) <= int(proto.ItemSlot_ItemSlotOffHand) {
		items = append(items, item{})
	}
	mh := item{"id": twoHander}
	if old, ok := items[proto.ItemSlot_ItemSlotMainHand].(map[string]interface{}); ok && old["enchant"] != nil {
		mh["enchant"] = old["enchant"]
	}
	items[proto.ItemSlot_ItemSlotMainHand] = mh
	items[proto.ItemSlot_ItemSlotOffHand] = item{}
	g["items"] = items
	return json.Marshal(g)
}

// presetFile returns a gear/APL file: embedded foreverbench APLs
// ("foreverbench/apls/...") or a UI snapshot file ("<ui dir>/...").
func presetFile(path string) (json.RawMessage, bool) {
	if rest, ok := strings.CutPrefix(path, "foreverbench/"); ok {
		data, err := embeddedAPLs.ReadFile("presets/" + rest)
		return data, err == nil
	}
	s, err := uiPresets()
	if err != nil {
		return nil, false
	}
	data, ok := s.Files[path]
	return data, ok
}

// gearPatch is an incomplete phase set whose empty slots are filled from the
// spec's previous complete phase set (only used with -gear-fill previous).
type gearPatch struct {
	label, file, from string
	slots             []string
	json              json.RawMessage
}

func patchGear(partial, base json.RawMessage) (json.RawMessage, []string, error) {
	type item = map[string]interface{}
	var p, b struct {
		Items []item `json:"items"`
	}
	if err := json.Unmarshal(partial, &p); err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(base, &b); err != nil {
		return nil, nil, err
	}
	filled := []string{}
	for i := range slotNames {
		for len(p.Items) <= i {
			p.Items = append(p.Items, item{})
		}
		if id, _ := p.Items[i]["id"].(float64); id == 0 && i < len(b.Items) {
			if bid, _ := b.Items[i]["id"].(float64); bid != 0 {
				p.Items[i] = b.Items[i]
				filled = append(filled, slotNames[i])
			}
		}
	}
	out, err := json.Marshal(p)
	return out, filled, err
}

// withGearFill returns the entry with patched gear applied when fill is
// "previous" and a patch exists; otherwise the entry unchanged.
func (e PresetEntry) withGearFill(fill string) PresetEntry {
	if fill != "previous" || e.patch == nil || e.Status != "incomplete_gear" {
		return e
	}
	e.Status, e.GearStatus = "ok", "patched"
	e.GearLabel, e.GearFile, e.GearSource = e.patch.label, e.patch.file, "incomplete UI set, empty slots filled from "+e.patch.from
	e.PatchedFrom, e.PatchedSlots, e.gearJSON = e.patch.from, e.patch.slots, e.patch.json
	e.AvgIlvl, e.EpicSlots, e.FilledSlots, _ = gearStats(e.patch.json, e.variant.Class)
	e.Reason = ""
	return e
}

var (
	gearPreBisRe   = regexp.MustCompile(`(?i)\bp(\d)[ ._-]*pre-?bis`)
	gearPhaseRe    = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:p|phase[ _]?)(\d)(?:[ ._-]*bis|[^a-z0-9]|$)`)
	gearPreRaidRe  = regexp.MustCompile(`(?i)pre-?bis`)
	gearMoltenCore = regexp.MustCompile(`(?i)^(mc|molten core)(\.gear\.json)?$`)
)

// classifyGear maps a gear preset name or file name to a phase. alt=true for
// "Pn Pre-BiS" style sets (a lower tier inside a phase) which are not used.
func classifyGear(name string) (phase Phase, ok bool, alt bool) {
	base := name[strings.LastIndex(name, "/")+1:]
	switch {
	case strings.Contains(strings.ToLower(base), "blank"):
		return 0, false, false
	case gearPreBisRe.MatchString(base):
		return 0, false, true
	case gearMoltenCore.MatchString(base):
		return 1, true, false
	}
	if m := gearPhaseRe.FindStringSubmatch(base); m != nil {
		n, _ := strconv.Atoi(m[1])
		if n < len(phaseLabels) {
			return Phase(n), true, false
		}
	}
	if gearPreRaidRe.MatchString(base) {
		return 0, true, false
	}
	return 0, false, false
}

type ilvlInfo struct {
	ilvl    int32
	quality proto.ItemQuality
}

var (
	ilvlOnce sync.Once
	ilvls    map[int32]ilvlInfo
)

func itemLevels() map[int32]ilvlInfo {
	ilvlOnce.Do(func() {
		ilvls = map[int32]ilvlInfo{}
		defer func() { recover() }()
		for _, it := range database.Load().Items {
			ilvls[it.Id] = ilvlInfo{it.Ilvl, it.Quality}
		}
	})
	return ilvls
}

var slotNames = []string{"head", "neck", "shoulder", "back", "chest", "wrist", "hands", "waist", "legs", "feet",
	"finger1", "finger2", "trinket1", "trinket2", "main_hand", "off_hand", "ranged"}

// gearStats returns the mean item level, epic count and filled slot count of a
// gear set, plus the required slots that are empty (head..main hand for every
// spec, ranged for hunters; off hand, wand/relic slots are optional).
func gearStats(raw json.RawMessage, c proto.Class) (avg float64, epics, filled int, emptyRequired []string) {
	var g struct {
		Items []struct {
			ID int32 `json:"id"`
		} `json:"items"`
	}
	if json.Unmarshal(raw, &g) != nil {
		return 0, 0, 0, []string{"unparseable gear file"}
	}
	db := itemLevels()
	sum := 0.0
	for i, name := range slotNames {
		id := int32(0)
		if i < len(g.Items) {
			id = g.Items[i].ID
		}
		required := i <= int(proto.ItemSlot_ItemSlotMainHand) || (i == int(proto.ItemSlot_ItemSlotRanged) && c == proto.Class_ClassHunter)
		if id == 0 {
			if required {
				emptyRequired = append(emptyRequired, name)
			}
			continue
		}
		info, ok := db[id]
		if !ok {
			continue
		}
		filled++
		sum += float64(info.ilvl)
		if info.quality >= proto.ItemQuality_ItemQualityEpic {
			epics++
		}
	}
	if filled > 0 {
		avg = math.Round(sum/float64(filled)*10) / 10
	}
	return avg, epics, filled, emptyRequired
}

type gearCandidate struct {
	label, file, source string
	phase               Phase
}

func (s *uiSnapshot) gearCandidates(v Variant) ([]gearCandidate, []string) {
	spec := s.Specs[v.UIDir]
	out := []gearCandidate{}
	skipped := []string{}
	registered := map[string]bool{}
	names := make([]string, 0, len(spec.Exports))
	for k := range spec.Exports {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, exp := range names {
		for _, r := range collectRefs(spec.Exports[exp]) {
			if r.Kind != "gear" || r.File == "" || registered[r.File] {
				continue
			}
			registered[r.File] = true
			if v.GearFilter != "" && !strings.Contains(r.File, v.GearFilter) {
				continue
			}
			p, ok, alt := classifyGear(r.Name)
			if !ok && !alt {
				p, ok, alt = classifyGear(r.File)
			}
			if alt {
				skipped = append(skipped, fmt.Sprintf("%s (%s): phase-internal pre-BiS set, not used", r.Name, r.File))
			}
			if ok {
				out = append(out, gearCandidate{r.Name, r.File, "ui presets.ts", p})
			}
		}
	}
	for _, f := range spec.GearSetFiles {
		rel := v.UIDir + "/gear_sets/" + f
		if registered[rel] || (v.GearFilter != "" && !strings.Contains(f, v.GearFilter)) {
			continue
		}
		if p, ok, _ := classifyGear(f); ok {
			out = append(out, gearCandidate{strings.TrimSuffix(f, ".gear.json"), rel, "gear_sets file not registered in presets.ts", p})
		}
	}
	return out, skipped
}

func (s *uiSnapshot) findRef(v Variant, kind, name string) (presetRef, bool) {
	spec := s.Specs[v.UIDir]
	names := make([]string, 0, len(spec.Exports))
	for k := range spec.Exports {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, exp := range names {
		for _, r := range collectRefs(spec.Exports[exp]) {
			if r.Name != name {
				continue
			}
			if kind == "talents" && r.talents() != "" {
				return r, true
			}
			if kind == "apl" && r.Kind == "apl" && r.File != "" {
				return r, true
			}
		}
	}
	return presetRef{}, false
}

func (s *uiSnapshot) otherDefaults(v Variant) (distance float64) {
	raw := s.Specs[v.UIDir].Exports["OtherDefaults"]
	var od struct {
		Distance *float64 `json:"distanceFromTarget"`
	}
	if json.Unmarshal(raw, &od) == nil && od.Distance != nil {
		return *od.Distance
	}
	return 5
}

var (
	matrixOnce sync.Once
	matrix     []PresetEntry
	matrixErr  error
)

// presetMatrix returns one entry per (variant, phase), in variant order.
func presetMatrix() ([]PresetEntry, error) {
	matrixOnce.Do(func() { matrix, matrixErr = buildMatrix() })
	return matrix, matrixErr
}

func buildMatrix() ([]PresetEntry, error) {
	s, err := uiPresets()
	if err != nil {
		return nil, fmt.Errorf("embedded UI presets: %w", err)
	}
	out := []PresetEntry{}
	for _, v := range variants {
		base := PresetEntry{Spec: v.ID, Class: strings.TrimPrefix(v.Class.String(), "Class"), SpecLabel: v.Spec, Role: v.Role, variant: v}
		if _, ok := s.Specs[v.UIDir]; !ok && v.Unavailable == "" {
			return nil, fmt.Errorf("%s: ui/%s not in the preset snapshot; rerun extract_presets.mjs", v.ID, v.UIDir)
		}
		if v.Unavailable == "" {
			if v.Talents != "" {
				t, ok := s.findRef(v, "talents", v.Talents)
				if !ok {
					return nil, fmt.Errorf("%s: talent preset %q not found in ui/%s/presets.ts", v.ID, v.Talents, v.UIDir)
				}
				base.TalentPreset, base.TalentsString, base.TalentSource = t.Name, t.talents(), "ui:"+t.Name
			}
			if v.APLFile != "" {
				if _, ok := presetFile(v.APLFile); !ok {
					return nil, fmt.Errorf("%s: APL file %s not found", v.ID, v.APLFile)
				}
				base.APLPreset, base.APLFile = v.APL, v.APLFile
			} else {
				a, ok := s.findRef(v, "apl", v.APL)
				if !ok {
					return nil, fmt.Errorf("%s: APL preset %q not found in ui/%s/presets.ts", v.ID, v.APL, v.UIDir)
				}
				base.APLPreset, base.APLFile = a.Name, a.File
			}
			base.specOptions = s.Specs[v.UIDir].Exports["DefaultOptions"]
			base.distance = s.otherDefaults(v)
		}
		cands, skipped := s.gearCandidates(v)
		if v.GearFrom != "" {
			cands, skipped = nil, nil
		}
		var lastComplete *gearCandidate
		for _, p := range allPhases() {
			e := base
			e.Phase = p.String()
			if v.Unavailable != "" {
				e.Status, e.GearStatus, e.Reason = "unavailable", "n/a", v.Unavailable
				out = append(out, e)
				continue
			}
			var chosen, partial *gearCandidate
			var incomplete []string
			for i := range cands {
				if cands[i].phase != p {
					continue
				}
				_, _, _, empty := gearStats(s.Files[cands[i].file], v.Class)
				switch {
				case len(empty) > 0:
					incomplete = append(incomplete, fmt.Sprintf("%s (%s) has empty required slots: %s", cands[i].label, cands[i].file, strings.Join(empty, ", ")))
					if partial == nil {
						partial = &cands[i]
					}
				case chosen == nil:
					chosen = &cands[i]
				default:
					e.Alternatives = append(e.Alternatives, cands[i].file)
				}
			}
			switch {
			case chosen != nil:
				e.Status, e.GearStatus = "ok", "ok"
				e.GearLabel, e.GearFile, e.GearSource = chosen.label, chosen.file, chosen.source
				e.AvgIlvl, e.EpicSlots, e.FilledSlots, _ = gearStats(s.Files[chosen.file], v.Class)
				lastComplete = chosen
			case v.GearFrom != "":
				e.Status, e.GearStatus = "missing_gear", "missing"
				e.Reason = fmt.Sprintf("gear is borrowed from %s, which has no complete %s set", v.GearFrom, p)
			case len(incomplete) > 0:
				e.Status, e.GearStatus = "incomplete_gear", "incomplete"
				e.Reason = strings.Join(incomplete, "; ") + " (partial sets such as a lone tier set would understate the spec, so it is not ranked"
				if lastComplete != nil {
					patched, slots, err := patchGear(s.Files[partial.file], s.Files[lastComplete.file])
					if err != nil {
						return nil, fmt.Errorf("%s %s: patching gear: %w", v.ID, p, err)
					}
					from := fmt.Sprintf("%s %s (%s)", lastComplete.phase, lastComplete.label, lastComplete.file)
					e.patch = &gearPatch{label: partial.label + " + " + lastComplete.phase.String() + " fill", file: partial.file, from: from, slots: slots, json: patched}
					e.Reason += "; -gear-fill previous fills it from " + from
				}
				e.Reason += ")"
			default:
				e.Status, e.GearStatus = "missing_gear", "missing"
				e.Reason = fmt.Sprintf("no %s gear set for this spec in ui/%s (blank gear is never used)", p, v.UIDir)
				if len(skipped) > 0 && p > 0 {
					e.Reason += "; skipped: " + strings.Join(skipped, "; ")
				}
			}
			out = append(out, e)
		}
	}
	if err := applyGearBorrowing(out); err != nil {
		return nil, err
	}
	return out, nil
}

// applyGearBorrowing gives entries without their own complete set the donor's
// own (not itself borrowed) set for the same phase: always for GearFrom, and for
// GearDonors only when the spec has no complete set of its own.
func applyGearBorrowing(entries []PresetEntry) error {
	own := map[string]PresetEntry{}
	for _, e := range entries {
		if e.Status == "ok" && e.GearStatus == "ok" {
			own[e.Spec+"|"+e.Phase] = e
		}
	}
	for i := range entries {
		e := &entries[i]
		v := e.variant
		if v.Unavailable != "" || e.Status == "ok" {
			continue
		}
		donors := v.GearDonors
		if v.GearFrom != "" {
			donors = []string{v.GearFrom}
		}
		for _, donor := range donors {
			if _, ok := findVariant(donor); !ok {
				return fmt.Errorf("%s: unknown gear donor %q", v.ID, donor)
			}
			d, ok := own[donor+"|"+e.Phase]
			if !ok {
				continue
			}
			raw, _ := presetFile(d.GearFile)
			phase, _ := parsePhase(e.Phase)
			th := v.TwoHander[phase]
			gear, err := borrowGear(raw, th)
			if err != nil {
				return fmt.Errorf("%s %s: borrowing gear from %s: %w", v.ID, e.Phase, donor, err)
			}
			if e.GearStatus != "missing" || v.GearFrom == "" {
				e.OwnGearStatus = e.GearStatus
			}
			e.Status, e.GearStatus, e.BorrowedFrom = "ok", "borrowed:"+donor, donor
			e.GearLabel, e.GearFile = d.GearLabel+" ("+donor+")", d.GearFile
			e.GearSource = "borrowed from " + donor + " (same armor type and role; no " + e.Phase + " set for this spec in the UI)"
			if v.GearFrom != "" {
				e.GearSource = "borrowed from " + donor + " (same armor type and role; the UI has no set for this spec)"
			}
			if th != 0 {
				e.gearJSON = gear
				e.GearLabel += fmt.Sprintf(" + two-hander %d", th)
				e.GearSource += fmt.Sprintf("; main hand replaced by two-hander item %d, off hand empty", th)
			}
			e.AvgIlvl, e.EpicSlots, e.FilledSlots, _ = gearStats(gear, v.Class)
			e.Reason, e.patch, e.Alternatives = "", nil, nil
			break
		}
	}
	return nil
}

func matrixEntry(specID string, phase Phase) (PresetEntry, error) {
	m, err := presetMatrix()
	if err != nil {
		return PresetEntry{}, err
	}
	for _, e := range m {
		if e.Spec == specID && e.Phase == phase.String() {
			return e, nil
		}
	}
	return PresetEntry{}, fmt.Errorf("unknown spec %q (see `foreverbench list`)", specID)
}

// specOneof builds the Player.spec oneof from the UI DefaultOptions JSON.
func setSpecOptions(player *proto.Player, field string, optionsJSON json.RawMessage) error {
	msg := player.ProtoReflect()
	fd := msg.Descriptor().Fields().ByName(protoreflect.Name(field))
	if fd == nil || fd.ContainingOneof() == nil {
		return fmt.Errorf("proto Player has no spec field %q", field)
	}
	specMsg := msg.NewField(fd).Message()
	if optFd := specMsg.Descriptor().Fields().ByName("options"); optFd != nil {
		opt := specMsg.NewField(optFd).Message()
		if len(optionsJSON) > 0 && string(optionsJSON) != "null" {
			if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(optionsJSON, opt.Interface()); err != nil {
				return fmt.Errorf("UI DefaultOptions for %s: %w", field, err)
			}
		}
		specMsg.Set(optFd, protoreflect.ValueOfMessage(opt))
	}
	msg.Set(fd, protoreflect.ValueOfMessage(specMsg))
	return nil
}

// ---------------------------------------------------------------- talent builds

// TalentBuild is a talent build for a spec: a Forever F1 string
// (presets/forever_builds.json, -forever-builds FILE) or a Classic talent
// string (presets/classic_builds.json, -classic-builds FILE). Builds written
// by optimize-all carry provenance; hand-curated builds only need spec, name,
// talents and source.
type TalentBuild struct {
	Spec    string `json:"spec"`
	Name    string `json:"name"`
	Talents string `json:"talents"`
	Source  string `json:"source"`
	Game    string `json:"game,omitempty"`  // forever | classic
	Phase   string `json:"phase,omitempty"` // phase the build was optimized for ("" = any)
	Date    string `json:"date,omitempty"`

	RulesetID            string          `json:"ruleset_id,omitempty"`
	ManifestSHA256       string          `json:"manifest_sha256,omitempty"`
	PresetSnapshotSHA256 string          `json:"preset_snapshot_sha256,omitempty"`
	NormalizationHash    string          `json:"normalization_hash,omitempty"`
	Normalization        json.RawMessage `json:"normalization,omitempty"`
	Targets              int             `json:"targets,omitempty"`
	DurationS            float64         `json:"duration_s,omitempty"`
	Seed                 int64           `json:"seed,omitempty"`
	Iterations           []int32         `json:"iterations,omitempty"`
	DPS                  float64         `json:"dps,omitempty"`
	StdErr               float64         `json:"stderr,omitempty"`
	CI95                 *[2]float64     `json:"ci95,omitempty"`
	Baseline             *BuildBaseline  `json:"baseline,omitempty"`
	Points               int32           `json:"points,omitempty"`
	Sims                 int64           `json:"sims,omitempty"`
	WallSeconds          float64         `json:"wall_seconds,omitempty"`
	BudgetExhausted      bool            `json:"budget_exhausted,omitempty"`
}

type BuildBaseline struct {
	Kind    string  `json:"kind"` // repair | ui | none
	Talents string  `json:"talents,omitempty"`
	DPS     float64 `json:"dps,omitempty"`
	StdErr  float64 `json:"stderr,omitempty"`
	Error   string  `json:"error,omitempty"`
}

func (b TalentBuild) optimized() bool { return strings.HasPrefix(b.Source, "optimize") }

// label is the talent_source reported by rank/delta/meta.
func (b TalentBuild) label() string {
	if b.optimized() {
		return "optimized:" + b.Date
	}
	return fmt.Sprintf("%s-build: %s (%s)", b.gameName(), b.Name, b.Source)
}

func (b TalentBuild) gameName() string {
	if b.Game == "" {
		return "forever"
	}
	return b.Game
}

type buildsFile struct {
	Note   string        `json:"note,omitempty"`
	Builds []TalentBuild `json:"builds"`
}

// loadBuilds parses builds files in order (later files win ties) for game.
func loadBuilds(game string, files ...[]byte) ([]TalentBuild, error) {
	out := []TalentBuild{}
	for i, raw := range files {
		if len(raw) == 0 {
			continue
		}
		var file buildsFile
		if err := json.Unmarshal(raw, &file); err != nil {
			return nil, fmt.Errorf("%s builds file %d: %w", game, i, err)
		}
		for _, b := range file.Builds {
			if _, ok := findVariant(b.Spec); !ok {
				return nil, fmt.Errorf("%s build %q: unknown spec %q", game, b.Name, b.Spec)
			}
			if b.Game != "" && b.Game != game {
				return nil, fmt.Errorf("%s build %q for %s is marked game %q", game, b.Name, b.Spec, b.Game)
			}
			if b.Phase != "" {
				if _, err := parsePhase(b.Phase); err != nil {
					return nil, fmt.Errorf("%s build %q: %w", game, b.Name, err)
				}
			}
			if b.Talents == "" {
				return nil, fmt.Errorf("%s build %q for %s has no talents", game, b.Name, b.Spec)
			}
			b.Game = game
			out = append(out, b)
		}
	}
	return out, nil
}

func loadForeverBuilds(extra []byte) ([]TalentBuild, error) {
	return loadBuilds("forever", foreverBuildsJSON, extra)
}

func loadClassicBuilds(extra []byte) ([]TalentBuild, error) {
	return loadBuilds("classic", classicBuildsJSON, extra)
}

// pickBuild selects the build for spec and phase: a build for exactly that
// phase, else a phase-less (hand-curated) build, else the build from the
// nearest phase (earlier phase on ties). Later entries win ties. Forever builds
// for a different ruleset are ignored.
func pickBuild(list []TalentBuild, spec string, phase Phase, rulesetID string) (TalentBuild, bool) {
	best, bestScore := TalentBuild{}, -1
	for _, b := range list {
		if b.Spec != spec || (rulesetID != "" && b.RulesetID != "" && b.RulesetID != rulesetID) {
			continue
		}
		score := 1000
		if b.Phase != "" {
			p, _ := parsePhase(b.Phase)
			dist := int(p) - int(phase)
			if dist < 0 {
				dist = -dist*2 - 1
			} else if dist > 0 {
				dist *= 2
			}
			score = 2000 - dist
			if dist == 0 {
				score = 3000
			}
		}
		if score >= bestScore {
			best, bestScore = b, score
		}
	}
	return best, bestScore >= 0
}

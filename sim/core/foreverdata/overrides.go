package foreverdata

// Best-known Forever override layer.
//
// overrides.json is produced by the external knowledge pipeline. It lets that
// pipeline change simulator numbers without Go changes. Nothing in this file
// mutates Classic data: overrides are only consulted by core when a player runs
// in Forever mode (Player.Forever != nil), and every application or rejection
// is recorded as an OverrideEvent (see diagnostics.go).
//
// Document shape (version "forever-overrides-1"):
//
//	{
//	  "version": "forever-overrides-1",
//	  "generated_at": "<RFC3339>",
//	  "source_hash": "<sha256 hex of the knowledge snapshot, or empty>",
//	  "spells": { "<spellId>": {
//	      "effects": { "<effectIndex>": {
//	          "base":        {"classic": <num|null>, "forever": <num>},
//	          "max":         {"classic": <num|null>, "forever": <num>},
//	          "coefficient": {"classic": <num|null>, "forever": <num>},
//	          "kind": "school_damage|periodic_damage|heal|periodic_heal|apply_aura|weapon_damage|other" } },
//	      "cast_ms":     {"classic": <num|null>, "forever": <num>},
//	      "cooldown_ms": {"classic": <num|null>, "forever": <num>},
//	      "duration_ms": {"classic": <num|null>, "forever": <num>},
//	      "cost":        {"classic": <num|null>, "forever": <num>, "power": "mana|rage|energy"},
//	      "provenance":  {"beliefs": [...], "status": "...", "sources": [...], "reason": "..."} } },
//	  "items": { "<itemId>": {
//	      "stats":  { "<statKey>": {"classic": <num|null>, "forever": <num>} },
//	      "weapon": {"min": {...}, "max": {...}, "speed_ms": {...}},
//	      "new_item": null | <UIItem JSON exactly as one entry of assets/database/db.json "items">,
//	      "provenance": {...} } },
//	  "talents": { "<record id>": { "values": [[rank1...], [rank2...]], "provenance": {...} } },
//	  "parameters": { "<parameter key>": {"value": <num>, "provenance": {...}} }
//	}
//
// Any unknown field (at any level), unknown version, malformed number, unknown
// effect kind, unknown power type or unknown stat key makes the document invalid
// and the package panics at init. Semantic problems that depend on other data
// (talent shape, parameter bounds) reject only that override and are reported by
// OverridesInfo().InitRejected.

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wowsims/classic/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
)

const OverridesVersion = "forever-overrides-1"

// Effect kinds.
const (
	KindSchoolDamage   = "school_damage"
	KindPeriodicDamage = "periodic_damage"
	KindHeal           = "heal"
	KindPeriodicHeal   = "periodic_heal"
	KindApplyAura      = "apply_aura"
	KindWeaponDamage   = "weapon_damage"
	KindOther          = "other"
)

var effectKinds = []string{KindSchoolDamage, KindPeriodicDamage, KindHeal, KindPeriodicHeal, KindApplyAura, KindWeaponDamage, KindOther}

// Power types accepted by spells.*.cost.power.
var costPowers = []string{"mana", "rage", "energy"}

// StatKeys lists the accepted items.*.stats keys, indexed by proto.Stat value.
// Values use the units of the sim item database (the "stats" array of
// assets/database/db.json), e.g. melee_crit/spell_crit/melee_hit/spell_hit are
// percentage points and armor is the item's base armor.
var StatKeys = []string{
	"strength", "agility", "stamina", "intellect", "spirit",
	"spell_power", "arcane_power", "fire_power", "frost_power", "holy_power", "nature_power", "shadow_power",
	"mp5", "spell_hit", "spell_crit", "spell_haste", "spell_penetration",
	"attack_power", "melee_hit", "melee_crit", "melee_haste", "armor_penetration", "expertise",
	"mana", "energy", "rage", "armor", "ranged_attack_power",
	"defense", "block", "block_value", "dodge", "parry", "resilience", "health",
	"arcane_resistance", "fire_resistance", "frost_resistance", "nature_resistance", "shadow_resistance",
	"bonus_armor", "healing_power", "spell_damage", "feral_attack_power",
}

// StatKeyIndex returns the proto.Stat for a stats key.
func StatKeyIndex(key string) (proto.Stat, bool) {
	i := slices.Index(StatKeys, key)
	return proto.Stat(i), i >= 0
}

type Provenance struct {
	Beliefs []string `json:"beliefs,omitempty"`
	Status  string   `json:"status,omitempty"`
	Sources []string `json:"sources,omitempty"`
	Reason  string   `json:"reason,omitempty"`
}

// Predicted reports whether the knowledge status marks this value as a prediction.
// STRICT Forever mode refuses predicted overrides.
func (p Provenance) Predicted() bool {
	return strings.EqualFold(strings.TrimSpace(p.Status), "PREDICTED")
}

type ValuePair struct {
	Classic *float64 `json:"classic"`
	Forever *float64 `json:"forever"`
}

type CostPair struct {
	Classic *float64 `json:"classic"`
	Forever *float64 `json:"forever"`
	Power   string   `json:"power"`
}

type EffectOverride struct {
	Base        *ValuePair `json:"base,omitempty"`
	Max         *ValuePair `json:"max,omitempty"`
	Coefficient *ValuePair `json:"coefficient,omitempty"`
	Kind        string     `json:"kind"`
}

type SpellOverride struct {
	Effects    map[string]EffectOverride `json:"effects,omitempty"`
	CastMs     *ValuePair                `json:"cast_ms,omitempty"`
	CooldownMs *ValuePair                `json:"cooldown_ms,omitempty"`
	DurationMs *ValuePair                `json:"duration_ms,omitempty"`
	Cost       *CostPair                 `json:"cost,omitempty"`
	Provenance Provenance                `json:"provenance"`
}

// Effect returns the override for an effect index.
func (s *SpellOverride) Effect(index int) (EffectOverride, bool) {
	e, ok := s.Effects[strconv.Itoa(index)]
	return e, ok
}

// EffectIndices returns the effect indices in ascending order.
func (s *SpellOverride) EffectIndices() []int {
	out := make([]int, 0, len(s.Effects))
	for k := range s.Effects {
		i, _ := strconv.Atoi(k)
		out = append(out, i)
	}
	sort.Ints(out)
	return out
}

type WeaponOverride struct {
	Min     *ValuePair `json:"min,omitempty"`
	Max     *ValuePair `json:"max,omitempty"`
	SpeedMs *ValuePair `json:"speed_ms,omitempty"`
}

type ItemOverride struct {
	Stats      map[string]ValuePair `json:"stats,omitempty"`
	Weapon     *WeaponOverride      `json:"weapon,omitempty"`
	NewItem    json.RawMessage      `json:"new_item,omitempty"`
	Provenance Provenance           `json:"provenance"`

	newItem *proto.UIItem
}

// NewItemProto returns a copy-safe pointer to the parsed new_item definition (nil when absent).
func (i *ItemOverride) NewItemProto() *proto.UIItem { return i.newItem }

type TalentOverride struct {
	Values     [][]float64 `json:"values"`
	Provenance Provenance  `json:"provenance"`
}

type ParameterOverride struct {
	Value      *float64   `json:"value"`
	Provenance Provenance `json:"provenance"`
}

type OverrideDocument struct {
	Version     string                       `json:"version"`
	GeneratedAt string                       `json:"generated_at"`
	SourceHash  string                       `json:"source_hash"`
	Spells      map[string]SpellOverride     `json:"spells"`
	Items       map[string]ItemOverride      `json:"items"`
	Talents     map[string]TalentOverride    `json:"talents"`
	Parameters  map[string]ParameterOverride `json:"parameters"`
}

// Overrides is a parsed, validated and indexed override document. It is immutable.
type Overrides struct {
	doc        OverrideDocument
	spells     map[int32]*SpellOverride
	items      map[int32]*ItemOverride
	talents    map[string]TalentOverride
	parameters map[string]ParameterOverride
	rejected   []OverrideEvent
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func checkPair(path string, p *ValuePair) error {
	if p == nil {
		return nil
	}
	if p.Forever == nil {
		return fmt.Errorf("%s: forever value is required", path)
	}
	if !finite(*p.Forever) || (p.Classic != nil && !finite(*p.Classic)) {
		return fmt.Errorf("%s: non-finite value", path)
	}
	return nil
}

func parseID(path, key string) (int32, error) {
	id, err := strconv.ParseInt(key, 10, 32)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != key {
		return 0, fmt.Errorf("%s: %q is not a positive integer id", path, key)
	}
	return int32(id), nil
}

// ParseOverrides strictly parses and validates an override document.
// It returns an error for any structural problem. Talent and parameter overrides
// that do not fit the embedded ruleset are dropped and listed in Rejected().
func ParseOverrides(raw []byte) (*Overrides, error) {
	// Top-level keys and version are checked explicitly for a clear message.
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("overrides: invalid JSON: %w", err)
	}
	allowed := []string{"version", "generated_at", "source_hash", "spells", "items", "talents", "parameters"}
	for key := range top {
		if !slices.Contains(allowed, key) {
			return nil, fmt.Errorf("overrides: unknown top-level key %q", key)
		}
	}
	var version string
	if err := json.Unmarshal(top["version"], &version); err != nil || version != OverridesVersion {
		return nil, fmt.Errorf("overrides: unsupported version %s (want %q)", string(top["version"]), OverridesVersion)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var doc OverrideDocument
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("overrides: %w", err)
	}
	if _, err := time.Parse(time.RFC3339, doc.GeneratedAt); err != nil {
		return nil, fmt.Errorf("overrides: generated_at must be RFC3339: %w", err)
	}
	if doc.SourceHash != "" {
		if b, err := hex.DecodeString(doc.SourceHash); err != nil || len(b) != 32 {
			return nil, fmt.Errorf("overrides: source_hash must be a sha256 hex digest")
		}
	}

	o := &Overrides{doc: doc, spells: map[int32]*SpellOverride{}, items: map[int32]*ItemOverride{},
		talents: map[string]TalentOverride{}, parameters: map[string]ParameterOverride{}}

	for key, spell := range doc.Spells {
		path := "spells." + key
		id, err := parseID(path, key)
		if err != nil {
			return nil, err
		}
		for idx, effect := range spell.Effects {
			ep := path + ".effects." + idx
			i, err := strconv.Atoi(idx)
			if err != nil || i < 0 || i > 31 || strconv.Itoa(i) != idx {
				return nil, fmt.Errorf("%s: effect index must be an integer 0..31", ep)
			}
			if !slices.Contains(effectKinds, effect.Kind) {
				return nil, fmt.Errorf("%s.kind: unknown kind %q", ep, effect.Kind)
			}
			for name, p := range map[string]*ValuePair{"base": effect.Base, "max": effect.Max, "coefficient": effect.Coefficient} {
				if err := checkPair(ep+"."+name, p); err != nil {
					return nil, err
				}
			}
		}
		for name, p := range map[string]*ValuePair{"cast_ms": spell.CastMs, "cooldown_ms": spell.CooldownMs, "duration_ms": spell.DurationMs} {
			if err := checkPair(path+"."+name, p); err != nil {
				return nil, err
			}
		}
		if spell.Cost != nil {
			if err := checkPair(path+".cost", &ValuePair{Classic: spell.Cost.Classic, Forever: spell.Cost.Forever}); err != nil {
				return nil, err
			}
			if !slices.Contains(costPowers, spell.Cost.Power) {
				return nil, fmt.Errorf("%s.cost.power: unknown power %q", path, spell.Cost.Power)
			}
		}
		s := spell
		o.spells[id] = &s
	}

	for key, item := range doc.Items {
		path := "items." + key
		id, err := parseID(path, key)
		if err != nil {
			return nil, err
		}
		for stat, p := range item.Stats {
			if _, ok := StatKeyIndex(stat); !ok {
				return nil, fmt.Errorf("%s.stats: unknown stat key %q", path, stat)
			}
			pair := p
			if err := checkPair(path+".stats."+stat, &pair); err != nil {
				return nil, err
			}
		}
		if item.Weapon != nil {
			for name, p := range map[string]*ValuePair{"min": item.Weapon.Min, "max": item.Weapon.Max, "speed_ms": item.Weapon.SpeedMs} {
				if err := checkPair(path+".weapon."+name, p); err != nil {
					return nil, err
				}
			}
		}
		it := item
		if trimmed := bytes.TrimSpace(item.NewItem); len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
			ui := &proto.UIItem{}
			if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(trimmed, ui); err != nil {
				return nil, fmt.Errorf("%s.new_item: %w", path, err)
			}
			if ui.Id == 0 {
				ui.Id = id
			}
			if ui.Id != id {
				return nil, fmt.Errorf("%s.new_item: id %d does not match key", path, ui.Id)
			}
			if ui.Name == "" || ui.Type == proto.ItemType_ItemTypeUnknown {
				return nil, fmt.Errorf("%s.new_item: name and type are required", path)
			}
			if len(ui.Stats) > len(StatKeys) {
				return nil, fmt.Errorf("%s.new_item: stats array longer than %d", path, len(StatKeys))
			}
			it.newItem = ui
		}
		o.items[id] = &it
	}

	talentIDs := make([]string, 0, len(doc.Talents))
	for id := range doc.Talents {
		talentIDs = append(talentIDs, id)
	}
	sort.Strings(talentIDs)
	for _, id := range talentIDs {
		t := doc.Talents[id]
		reject := func(reason string) {
			o.rejected = append(o.rejected, OverrideEvent{Scope: ScopeTalent, ID: id, Field: "values", Reason: reason,
				Beliefs: t.Provenance.Beliefs, Status: t.Provenance.Status})
		}
		r, ok := lookupBase(id)
		if !ok {
			reject("unknown talent record")
			continue
		}
		if len(t.Values) != int(r.MaxRank) || len(r.Ranks) != int(r.MaxRank) {
			reject(fmt.Sprintf("rank count %d != max_rank %d", len(t.Values), r.MaxRank))
			continue
		}
		bad := ""
		for i, values := range t.Values {
			if len(values) != len(r.Ranks[i].Values) {
				bad = fmt.Sprintf("rank %d has %d values, embedded record has %d", i+1, len(values), len(r.Ranks[i].Values))
				break
			}
			for _, v := range values {
				if !finite(v) {
					bad = fmt.Sprintf("rank %d has a non-finite value", i+1)
				}
			}
		}
		if bad != "" {
			reject(bad)
			continue
		}
		o.talents[id] = t
	}

	paramKeys := make([]string, 0, len(doc.Parameters))
	for key := range doc.Parameters {
		paramKeys = append(paramKeys, key)
	}
	sort.Strings(paramKeys)
	for _, key := range paramKeys {
		p := doc.Parameters[key]
		if p.Value == nil {
			return nil, fmt.Errorf("parameters.%s: value is required", key)
		}
		if !ParameterInBounds(key, *p.Value) {
			v := *p.Value
			o.rejected = append(o.rejected, OverrideEvent{Scope: ScopeParameter, ID: key, Field: "value", Forever: &v,
				Reason: "unknown parameter or value outside registered bounds", Beliefs: p.Provenance.Beliefs, Status: p.Provenance.Status})
			continue
		}
		o.parameters[key] = p
	}
	return o, nil
}

func mustParseOverrides(raw []byte) *Overrides {
	o, err := ParseOverrides(raw)
	if err != nil {
		panic("foreverdata: invalid embedded overrides.json: " + err.Error())
	}
	for _, e := range o.rejected {
		log.Printf("foreverdata: rejected %s override %s: %s", e.Scope, e.ID, e.Reason)
	}
	return o
}

var activeOverrides atomic.Pointer[Overrides]

func init() {
	raw, err := files.ReadFile("overrides.json")
	if err != nil {
		panic(err)
	}
	activeOverrides.Store(mustParseOverrides(raw))
}

// ActiveOverrides returns the process-wide override document.
func ActiveOverrides() *Overrides { return activeOverrides.Load() }

// SetOverridesForTesting swaps the process-wide overrides and returns a restore func.
// Only for tests; simulations snapshot the pointer when a Forever character is built.
func SetOverridesForTesting(o *Overrides) (restore func()) {
	old := activeOverrides.Swap(o)
	return func() { activeOverrides.Store(old) }
}

func (o *Overrides) Spell(id int32) (*SpellOverride, bool) {
	s, ok := o.spells[id]
	return s, ok
}
func (o *Overrides) Item(id int32) (*ItemOverride, bool) {
	i, ok := o.items[id]
	return i, ok
}
func (o *Overrides) Talent(id string) (TalentOverride, bool) {
	t, ok := o.talents[id]
	return t, ok
}
func (o *Overrides) Parameter(key string) (ParameterOverride, bool) {
	p, ok := o.parameters[key]
	return p, ok
}
func (o *Overrides) HasParameters() bool { return len(o.parameters) > 0 }

// ItemIDs returns overridden item ids in ascending order.
func (o *Overrides) ItemIDs() []int32 {
	ids := make([]int32, 0, len(o.items))
	for id := range o.items {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// Rejected lists overrides dropped while loading (talent shape, parameter bounds).
func (o *Overrides) Rejected() []OverrideEvent { return slices.Clone(o.rejected) }

type OverrideCounts struct {
	Spells       int `json:"spells"`
	SpellEffects int `json:"spell_effects"`
	Items        int `json:"items"`
	NewItems     int `json:"new_items"`
	Talents      int `json:"talents"`
	Parameters   int `json:"parameters"`
}

type OverridesSummary struct {
	Version     string          `json:"version"`
	SourceHash  string          `json:"source_hash"`
	GeneratedAt string          `json:"generated_at"`
	Counts      OverrideCounts  `json:"counts"`
	Rejected    OverrideCounts  `json:"rejected_counts"`
	InitRejects []OverrideEvent `json:"init_rejected"`
}

func (o *Overrides) Summary() OverridesSummary {
	s := OverridesSummary{Version: o.doc.Version, SourceHash: o.doc.SourceHash, GeneratedAt: o.doc.GeneratedAt, InitRejects: o.Rejected()}
	s.Counts.Spells = len(o.spells)
	for _, sp := range o.spells {
		s.Counts.SpellEffects += len(sp.Effects)
	}
	s.Counts.Items = len(o.items)
	for _, it := range o.items {
		if it.newItem != nil {
			s.Counts.NewItems++
		}
	}
	s.Counts.Talents = len(o.talents)
	s.Counts.Parameters = len(o.parameters)
	for _, e := range o.rejected {
		switch e.Scope {
		case ScopeTalent:
			s.Rejected.Talents++
		case ScopeParameter:
			s.Rejected.Parameters++
		}
	}
	return s
}

// OverridesInfo summarizes the active override document.
func OverridesInfo() OverridesSummary { return ActiveOverrides().Summary() }

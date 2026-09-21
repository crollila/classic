// Package gamedata is the Forever client's game data for one build, as the simulator reads it.
//
// current.json.gz is generated from a build-versioned snapshot (god/forever_god/gamedata.py,
// history in assets/gamedata/) and embedded: spells with every effect, cost, cooldown, cast
// time, duration and proc option; Forever Trait talents with per-rank effect values; item sets;
// enchants; and per-level game tables. Every value is client data for Build.
//
// The package is a leaf: it imports nothing from core, so core and every class package can use
// it. Class code asks for a spell by client id and reads numbers from it instead of carrying its
// own copy; Fallback records the places that still use a non-client value, so none are silent.
package gamedata

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
)

//go:embed current.json.gz
var embedded []byte

type Effect struct {
	Index          int      `json:"index"`
	Effect         int      `json:"effect"`
	EffectName     string   `json:"effect_name"`
	Aura           int      `json:"aura"`
	AuraName       string   `json:"aura_name"`
	Base           float64  `json:"base"`
	Variance       float64  `json:"variance"`
	Min            float64  `json:"min"`
	Max            float64  `json:"max"`
	PerLevel       float64  `json:"per_level"`
	SPCoefficient  *float64 `json:"sp_coefficient"`
	APCoefficient  float64  `json:"ap_coefficient"`
	PeriodMs       int      `json:"period_ms"`
	Amplitude      float64  `json:"amplitude"`
	ChainTargets   int      `json:"chain_targets"`
	ChainAmplitude float64  `json:"chain_amplitude"`
	TriggerSpell   int32    `json:"trigger_spell"`
	Mechanic       int      `json:"mechanic"`
	Misc           []int32  `json:"misc"`
	ClassMask      []int64  `json:"class_mask"`
	PointsPerCombo float64  `json:"points_per_resource"`
	Targets        []int    `json:"targets"`
}

// Range is the effect's value range at the spell's level: Min..Max, or Base when the client
// stores a single value.
func (e *Effect) Range() (float64, float64) {
	if e == nil {
		return 0, 0
	}
	if e.Max == 0 && e.Min == 0 {
		return e.Base, e.Base
	}
	if e.Max == 0 {
		return e.Min, e.Min
	}
	return e.Min, e.Max
}

// Coefficient is the spell power coefficient; the client's default of 1 means "no bonus
// coefficient set" only for effects that do not scale, so callers decide what 1 means.
func (e *Effect) Coefficient() float64 {
	if e == nil || e.SPCoefficient == nil {
		return 0
	}
	return *e.SPCoefficient
}

type Cost struct {
	Power    string  `json:"power"`
	Cost     float64 `json:"cost"`
	CostPct  float64 `json:"cost_pct"`
	PerLevel float64 `json:"per_level"`
}

type Spell struct {
	ID                 int32
	Name               string    `json:"name"`
	Rank               string    `json:"rank"`
	School             []string  `json:"school"`
	CastMs             int       `json:"cast_ms"`
	DurationMs         int       `json:"duration_ms"`
	RangeYd            float64   `json:"range_yd"`
	CooldownMs         int       `json:"cooldown_ms"`
	CategoryCooldownMs int       `json:"category_cooldown_ms"`
	GCDMs              int       `json:"gcd_ms"`
	Costs              []Cost    `json:"costs"`
	BaseLevel          int32     `json:"base_level"`
	SpellLevel         int32     `json:"spell_level"`
	MaxLevel           int32     `json:"max_level"`
	SpellClass         string    `json:"spell_class"`
	SpellClassMask     []int64   `json:"spell_class_mask"`
	RequiresItemClass  *int      `json:"requires_item_class"`
	ProcChance         float64   `json:"proc_chance"`
	ProcCharges        int       `json:"proc_charges"`
	ProcsPerMinute     float64   `json:"procs_per_minute"`
	ProcICDMs          int       `json:"proc_icd_ms"`
	MaxStacks          int       `json:"max_stacks"`
	MaxTargets         int       `json:"max_targets"`
	Attributes         []int64   `json:"attributes"`
	Effects            []*Effect `json:"effects"`
}

// Effect returns the effect with the given index, or nil.
func (s *Spell) Effect(index int) *Effect {
	if s == nil {
		return nil
	}
	for _, e := range s.Effects {
		if e.Index == index {
			return e
		}
	}
	return nil
}

// FirstEffect returns the first effect of the given client effect type, or nil.
func (s *Spell) FirstEffect(effectType int) *Effect {
	if s == nil {
		return nil
	}
	for _, e := range s.Effects {
		if e.Effect == effectType {
			return e
		}
	}
	return nil
}

// Cost returns the spell's cost for a power type ("mana", "rage", "energy", ...), or nil.
func (s *Spell) Cost(power string) *Cost {
	if s == nil {
		return nil
	}
	for i := range s.Costs {
		if s.Costs[i].Power == power {
			return &s.Costs[i]
		}
	}
	return nil
}

type Talent struct {
	Key          string
	Name         string               `json:"name"`
	Class        string               `json:"character_class"`
	Tree         string               `json:"tree"`
	SpellID      int32                `json:"spell_id"`
	MaxRanks     int                  `json:"max_ranks"`
	RankPoints   []map[string]float64 `json:"rank_points"`
	RequiresNode []int                `json:"requires_nodes"`
}

// Points is the client value of effect `effect` at `rank` (1-based), and whether the client has
// a per-rank value for it. Talents without curves take their value from the spell's effect.
func (t *Talent) Points(rank int, effect int) (float64, bool) {
	if t == nil || rank < 1 || rank > len(t.RankPoints) {
		return 0, false
	}
	v, ok := t.RankPoints[rank-1][fmt.Sprint(effect)]
	return v, ok
}

type ItemSet struct {
	Name    string  `json:"name"`
	Items   []int32 `json:"items"`
	Bonuses []struct {
		Pieces  int   `json:"pieces"`
		SpellID int32 `json:"spell_id"`
	} `json:"bonuses"`
}

type Enchant struct {
	Name     string    `json:"name"`
	Effects  []int     `json:"effects"`
	Points   []float64 `json:"points"`
	Args     []int32   `json:"args"`
	MinLevel int32     `json:"min_level"`
}

type GameTable struct {
	Columns []string                      `json:"columns"`
	Rows    map[string]map[string]float64 `json:"rows"`
}

type Snapshot struct {
	Schema         int                    `json:"schema"`
	Build          string                 `json:"build"`
	SnapshotSHA256 string                 `json:"snapshot_sha256"`
	RawSpells      map[string]*Spell      `json:"spells"`
	RawTalents     map[string]*Talent     `json:"talents"`
	RecordTalents  map[string]string      `json:"record_talents"`
	RawItemSets    map[string]*ItemSet    `json:"item_sets"`
	RawEnchants    map[string]*Enchant    `json:"enchants"`
	GameTables     map[string]*GameTable  `json:"gametables"`
	Source         map[string]interface{} `json:"source"`
	// The Classic Era client's values for the same spells: what the simulator's Classic code
	// was written against. Only the fields used to carry Forever values into that code.
	ClassicBuild     string            `json:"classic_build"`
	RawClassicSpells map[string]*Spell `json:"classic_spells"`

	spells        map[int32]*Spell
	classicSpells map[int32]*Spell
	itemSets      map[int32]*ItemSet
	enchants      map[int32]*Enchant
}

// Spell returns the client spell, or nil when this build has no such spell.
func (s *Snapshot) Spell(id int32) *Spell {
	if s == nil {
		return nil
	}
	return s.spells[id]
}

// ClassicSpell is the Classic Era client's version of a spell, or nil for Forever-only spells.
func (s *Snapshot) ClassicSpell(id int32) *Spell {
	if s == nil {
		return nil
	}
	return s.classicSpells[id]
}

// TalentForRecord returns the client Trait talent behind a simulator talent record id.
func (s *Snapshot) TalentForRecord(recordID string) *Talent {
	if s == nil {
		return nil
	}
	return s.RawTalents[s.RecordTalents[recordID]]
}

func (s *Snapshot) ItemSet(id int32) *ItemSet { return s.itemSets[id] }
func (s *Snapshot) Enchant(id int32) *Enchant { return s.enchants[id] }

// Value reads a game table cell, e.g. Value("combatratings", "60", "Crit - Melee").
func (s *Snapshot) Value(table, row, column string) (float64, bool) {
	t := s.GameTables[table]
	if t == nil {
		return 0, false
	}
	v, ok := t.Rows[row][column]
	return v, ok
}

func decode(raw []byte) (*Snapshot, error) {
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	snapshot := &Snapshot{}
	if err := json.Unmarshal(body, snapshot); err != nil {
		return nil, err
	}
	snapshot.index()
	return snapshot, nil
}

func (s *Snapshot) index() {
	s.spells = make(map[int32]*Spell, len(s.RawSpells))
	for key, spell := range s.RawSpells {
		var id int32
		fmt.Sscan(key, &id)
		spell.ID = id
		s.spells[id] = spell
	}
	s.classicSpells = make(map[int32]*Spell, len(s.RawClassicSpells))
	for key, spell := range s.RawClassicSpells {
		var id int32
		fmt.Sscan(key, &id)
		spell.ID = id
		s.classicSpells[id] = spell
	}
	for key, talent := range s.RawTalents {
		talent.Key = key
	}
	s.itemSets = map[int32]*ItemSet{}
	for key, set := range s.RawItemSets {
		var id int32
		fmt.Sscan(key, &id)
		s.itemSets[id] = set
	}
	s.enchants = map[int32]*Enchant{}
	for key, enchant := range s.RawEnchants {
		var id int32
		fmt.Sscan(key, &id)
		s.enchants[id] = enchant
	}
}

var (
	mu       sync.RWMutex
	current  *Snapshot
	loaded   = map[string]*Snapshot{}
	loadOnce sync.Once
)

// Current is the embedded snapshot (the build this simulator was published with).
func Current() *Snapshot {
	loadOnce.Do(func() {
		snapshot, err := decode(embedded)
		if err != nil {
			panic("gamedata: embedded snapshot is invalid: " + err.Error())
		}
		mu.Lock()
		current = snapshot
		loaded[snapshot.Build] = snapshot
		mu.Unlock()
	})
	mu.RLock()
	defer mu.RUnlock()
	return current
}

var disabled bool

// DisableForTesting makes ForBuild("") return no data, so a test can exercise the simulator's
// Classic code paths and the override document on their own. It returns the restore function.
func DisableForTesting() func() {
	mu.Lock()
	previous := disabled
	disabled = true
	mu.Unlock()
	return func() {
		mu.Lock()
		disabled = previous
		mu.Unlock()
	}
}

// ForBuild returns a loaded snapshot by build label; "" means Current.
func ForBuild(build string) (*Snapshot, error) {
	mu.RLock()
	off := disabled
	mu.RUnlock()
	if build == "" && off {
		return nil, nil
	}
	if build == "" {
		return Current(), nil
	}
	Current()
	mu.RLock()
	snapshot, ok := loaded[build]
	mu.RUnlock()
	if ok {
		return snapshot, nil
	}
	return nil, fmt.Errorf("gamedata: build %q is not loaded (have %v)", build, Builds())
}

// LoadFile registers an engine export (a *.json.gz written by gamedata.engine_export) so
// requests can name its build. Used to reproduce results on an older build.
func LoadFile(path string) (*Snapshot, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	snapshot, err := decode(raw)
	if err != nil {
		return nil, err
	}
	Register(snapshot)
	return snapshot, nil
}

// Register makes a snapshot selectable by its build label.
func Register(snapshot *Snapshot) {
	if snapshot.spells == nil {
		snapshot.index()
	}
	mu.Lock()
	loaded[snapshot.Build] = snapshot
	mu.Unlock()
}

// Builds lists the loaded build labels.
func Builds() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(loaded))
	for build := range loaded {
		out = append(out, build)
	}
	sort.Strings(out)
	return out
}

// Clone returns a deep copy, for tests that change data and check the simulator follows.
func (s *Snapshot) Clone(build string) *Snapshot {
	body, _ := json.Marshal(s)
	copy := &Snapshot{}
	_ = json.Unmarshal(body, copy)
	copy.Build = build
	copy.index()
	return copy
}

// RegisterVariant registers a copy of the current build under label with mutate applied, and
// returns it. Requests select it with ForeverOptions.game_data_build. Tests use it to prove a value
// reaches the simulator from data alone; the build pipeline uses it to sim a candidate build.
func RegisterVariant(label string, mutate func(*Snapshot)) *Snapshot {
	variant := Current().Clone(label)
	if mutate != nil {
		mutate(variant)
	}
	// Mutations may add or replace records, not just edit existing pointers.
	variant.index()
	Register(variant)
	return variant
}

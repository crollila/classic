// Package foreverdata is immutable discovery data and request validation.
// It does not import core, change factories, or mutate process-wide game selection.
package foreverdata

import (
	"embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/wowsims/classic/sim/core/proto"
	googleproto "google.golang.org/protobuf/proto"
)

const RulesetID = "forever-discovery-2026-09-13-v1"

//go:embed trees.json
var files embed.FS

type Rank struct {
	Rank       int32
	Effect     string
	Estimated  bool
	Confidence string
	SourceURL  string `json:"source_url"`
	Values     []float64
}
type Prerequisite struct {
	ID   string
	Rank int32
}
type Record struct {
	ID, Class, Tree, Name string
	MaxRank               int32 `json:"max_rank"`
	Row, Column           int32
	RequiredPoints        int32 `json:"required_points"`
	Prerequisites         []Prerequisite
	ClassicField          string `json:"classic_field"`
	ClassicMaxRank        int32  `json:"classic_max_rank"`
	Mode                  string
	Ranks                 []Rank
	ActionTag             int32 `json:"action_tag"`
}
type Mechanic struct{ ID, Mode string }
type dataset struct {
	ManifestSHA256 string                `json:"manifest_sha256"`
	ClassicLayout  map[string][][]string `json:"classic_layout"`
	RaceClasses    map[string][]string   `json:"race_classes"`
	Records        []Record
	Mechanics      []Mechanic
}

var data = func() dataset {
	raw, err := files.ReadFile("trees.json")
	if err != nil {
		panic(err)
	}
	var d dataset
	if err = json.Unmarshal(raw, &d); err != nil {
		panic(err)
	}
	return d
}()

func ManifestSHA256() string { return data.ManifestSHA256 }

// Lookup returns a copy of all mutable slices, keeping embedded data immutable.
func Lookup(id string) (Record, bool) {
	for _, r := range data.Records {
		if r.ID == id {
			r.Prerequisites = slices.Clone(r.Prerequisites)
			r.Ranks = slices.Clone(r.Ranks)
			for i := range r.Ranks {
				r.Ranks[i].Values = slices.Clone(r.Ranks[i].Values)
			}
			return r, true
		}
	}
	return Record{}, false
}
func KnownMechanic(id string) bool {
	for _, m := range data.Mechanics {
		if m.ID == id {
			return true
		}
	}
	return false
}
func ClassName(c proto.Class) string { return strings.ToUpper(strings.TrimPrefix(c.String(), "Class")) }
func Validate(p *proto.Player) error {
	f := p.GetForever()
	if f == nil {
		return nil
	}
	if f.RulesetId != RulesetID {
		return fmt.Errorf("unsupported Forever ruleset %q", f.RulesetId)
	}
	if p.TalentsString != "" {
		return fmt.Errorf("Forever requires record-keyed talents; Classic talents_string must be empty")
	}
	if p.Database != nil {
		return fmt.Errorf("Forever cannot mutate the shared Classic item database")
	}
	var specClass proto.Class
	switch p.GetSpec().(type) {
	case *proto.Player_Warrior, *proto.Player_TankWarrior:
		specClass = proto.Class_ClassWarrior
	case *proto.Player_RetributionPaladin, *proto.Player_ProtectionPaladin:
		specClass = proto.Class_ClassPaladin
	case *proto.Player_Hunter:
		specClass = proto.Class_ClassHunter
	case *proto.Player_Rogue:
		specClass = proto.Class_ClassRogue
	case *proto.Player_ShadowPriest:
		specClass = proto.Class_ClassPriest
	case *proto.Player_ElementalShaman, *proto.Player_EnhancementShaman, *proto.Player_WardenShaman:
		specClass = proto.Class_ClassShaman
	case *proto.Player_Mage:
		specClass = proto.Class_ClassMage
	case *proto.Player_Warlock:
		specClass = proto.Class_ClassWarlock
	case *proto.Player_BalanceDruid, *proto.Player_FeralDruid:
		specClass = proto.Class_ClassDruid
	default:
		return fmt.Errorf("missing or unsupported Forever specialization (healing factories remain unavailable)")
	}
	if specClass != p.Class {
		return fmt.Errorf("Forever class does not match specialization")
	}
	total := int32(0)
	for id, n := range f.Talents {
		r, ok := Lookup(id)
		if !ok || r.Class != ClassName(p.Class) {
			return fmt.Errorf("unknown, removed, or wrong-class talent %s", id)
		}
		if n < 0 || n > r.MaxRank {
			return fmt.Errorf("invalid rank %d for %s", n, id)
		}
		if n == 0 {
			continue
		}
		total += n
		if r.Mode == "blocked" {
			return fmt.Errorf("BLOCKED Forever mechanic %s: no reviewed combat adapter", id)
		}
		if r.Ranks[n-1].Estimated && !f.ExperimentalEstimatedRanks {
			return fmt.Errorf("estimated rank %d of %s requires experimental_estimated_ranks", n, id)
		}
		for _, pre := range r.Prerequisites {
			if f.Talents[pre.ID] < pre.Rank {
				return fmt.Errorf("%s requires %s rank %d", id, pre.ID, pre.Rank)
			}
		}
		below := int32(0)
		for other, points := range f.Talents {
			o, exists := Lookup(other)
			if exists && o.Tree == r.Tree && o.Row < r.Row {
				below += points
			}
		}
		if below < r.RequiredPoints {
			return fmt.Errorf("%s needs %d points in lower rows of %s", id, r.RequiredPoints, r.Tree)
		}
	}
	if total > 51 {
		return fmt.Errorf("Forever talent point budget exceeded: %d > 51", total)
	}
	seen := map[string]bool{}
	raceNames := map[proto.Race]string{proto.Race_RaceHuman: "human", proto.Race_RaceDwarf: "dwarf", proto.Race_RaceNightElf: "night-elf", proto.Race_RaceGnome: "gnome", proto.Race_RaceOrc: "orc", proto.Race_RaceUndead: "undead", proto.Race_RaceTauren: "tauren", proto.Race_RaceTroll: "troll"}
	if !slices.Contains(data.RaceClasses[raceNames[p.Race]], ClassName(p.Class)) {
		return fmt.Errorf("unsupported Forever race/class combination (Skyborne base stats remain unknown)")
	}
	for _, id := range f.Mechanics {
		if !KnownMechanic(id) || seen[id] {
			return fmt.Errorf("unknown, unsupported, or duplicate Forever mechanic %s", id)
		}
		seen[id] = true
		if strings.HasPrefix(id, "racials.") && !strings.HasPrefix(id, "racials."+raceNames[p.Race]+".") {
			return fmt.Errorf("wrong-race Forever mechanic %s", id)
		}
	}
	return nil
}

func PrimaryTree(p *proto.Player) uint8 {
	names := []string{}
	points := map[string]int32{}
	for _, r := range data.Records {
		if r.Class == ClassName(p.Class) {
			if !slices.Contains(names, r.Tree) {
				names = append(names, r.Tree)
			}
			points[r.Tree] += p.Forever.Talents[r.ID]
		}
	}
	best := uint8(0)
	for i, name := range names {
		if points[name] > points[names[best]] {
			best = uint8(i)
		}
	}
	return best
}

// Prepare clones before translating semantic record IDs to the existing typed
// talent fields. Removed talents remain zero. Replacements do not set old fields.
func Prepare(p *proto.Player) (*proto.Player, error) {
	if p.GetForever() == nil {
		return p, nil
	}
	if err := Validate(p); err != nil {
		return nil, err
	}
	out := googleproto.Clone(p).(*proto.Player)
	fields := map[string]int32{}
	for id, n := range p.Forever.Talents {
		r, _ := Lookup(id)
		if r.Mode == "classic" || r.Mode == "class" {
			fields[r.ClassicField] = min(n, r.ClassicMaxRank)
		}
	}
	trees := []string{}
	for _, tree := range data.ClassicLayout[ClassName(p.Class)] {
		var s strings.Builder
		for _, field := range tree {
			s.WriteByte(byte('0' + fields[field]))
		}
		trees = append(trees, s.String())
	}
	out.TalentsString = strings.Join(trees, "-")
	return out, nil
}

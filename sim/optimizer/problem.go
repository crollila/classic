package optimizer

import (
	"fmt"
	"slices"
	"strings"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
	googleProto "google.golang.org/protobuf/proto"
)

// Problem is the question "what is the highest-DPS <spec> setup at level L for this build":
// the character, the fight the app sims (target level L+2 humanoid with the level's mob armor,
// fight length with 10 s variation, no buffs or consumables) and the item filters.
type Problem struct {
	Data        *Data
	Spec        *SpecDef
	Level       int32
	TargetLevel int32   // 0 = min(63, level + 2), as the app
	Duration    float64 // seconds; 0 = 120
	Build       string  // ForeverOptions.GameDataBuild ("" = the embedded client build)
	AboveLevel  bool    // allow items above the character's level (off by default)
	Qualities   []proto.ItemQuality
	Professions bool // allow items/enchants that need a profession

	races       []proto.Race
	records     []TalentRecord
	recByID     map[string]TalentRecord
	enchantByID map[int32]*proto.UIEnchant
}

// NewProblem prepares a problem for a spec key of the app at a level.
func NewProblem(d *Data, specKey string, level int32) (*Problem, error) {
	spec := SpecByKey(specKey)
	if spec == nil {
		keys := []string{}
		for _, s := range Specs {
			keys = append(keys, s.Key)
		}
		return nil, fmt.Errorf("unknown spec %q (have %s)", specKey, strings.Join(keys, ", "))
	}
	if level < 10 || level > 60 {
		return nil, fmt.Errorf("level %d outside 10..60", level)
	}
	p := &Problem{Data: d, Spec: spec, Level: level, races: d.Races(spec.Class), records: d.ClassRecords(spec.Class),
		recByID: map[string]TalentRecord{}, enchantByID: map[int32]*proto.UIEnchant{}}
	for _, r := range p.records {
		p.recByID[r.ID] = r
	}
	for _, e := range d.Enchants {
		if _, dup := p.enchantByID[e.EffectId]; !dup {
			p.enchantByID[e.EffectId] = e
		}
	}
	if len(p.races) == 0 {
		return nil, fmt.Errorf("no Forever race can be a %s", spec.Label)
	}
	return p, nil
}

func (p *Problem) targetLevel() int32 {
	if p.TargetLevel > 0 {
		return p.TargetLevel
	}
	return min(63, p.Level+2)
}

func (p *Problem) duration() float64 {
	if p.Duration > 0 {
		return p.Duration
	}
	return 120
}

// Races are the races the problem may choose.
func (p *Problem) Races() []proto.Race { return slices.Clone(p.races) }

// Config is one complete setup. Gear and Enchants are indexed by proto.ItemSlot.
type Config struct {
	Race     proto.Race
	Rotation int
	Gear     [numSlots]int32
	Enchants [numSlots]int32
	Talents  map[string]int32
}

// Clone deep-copies the configuration.
func (c Config) Clone() Config {
	t := make(map[string]int32, len(c.Talents))
	for k, v := range c.Talents {
		if v > 0 {
			t[k] = v
		}
	}
	c.Talents = t
	return c
}

// Key identifies the configuration for the result cache.
func (c *Config) Key() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d|%d|", c.Race, c.Rotation)
	for i := 0; i < numSlots; i++ {
		fmt.Fprintf(&b, "%d:%d,", c.Gear[i], c.Enchants[i])
	}
	for _, id := range sortedTalentIDs(c.Talents) {
		fmt.Fprintf(&b, "%s=%d;", id, c.Talents[id])
	}
	return b.String()
}

// Player builds the proto player exactly as the Forever app does for this configuration.
func (p *Problem) Player(c *Config) (*proto.Player, error) {
	rot, err := p.Data.Rotation(p.Spec.Rotations[c.Rotation].Path)
	if err != nil {
		return nil, err
	}
	eq := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, numSlots)}
	for i := range eq.Items {
		eq.Items[i] = &proto.ItemSpec{Id: c.Gear[i], Enchant: c.Enchants[i]}
	}
	talents := map[string]int32{}
	for id, n := range c.Talents {
		if n > 0 {
			talents[id] = n
		}
	}
	distance := float64(25)
	if p.Spec.Role == "melee" {
		distance = 5
	}
	player := &proto.Player{
		Name: "Forever", Class: p.Spec.Class, Race: c.Race, Level: p.Level,
		Equipment: eq, Consumes: &proto.Consumes{}, Buffs: &proto.IndividualBuffs{}, Cooldowns: &proto.Cooldowns{},
		Forever: &proto.ForeverOptions{RulesetId: p.Data.RulesetID, Mode: proto.ForeverMode_BEST_GUESS, Talents: talents,
			Mechanics: p.Data.mechanicsFor(p.Spec.Class, c.Race), GameDataBuild: p.Build},
		Rotation:           googleProto.Clone(rot).(*proto.APLRotation),
		ReactionTimeMs:     100,
		DistanceFromTarget: distance,
	}
	return core.WithSpec(player, p.Spec.Spec(p.Level)), nil
}

// Request is the raid sim request of the app for the configuration.
func (p *Problem) Request(c *Config, iterations int32, seed int64) (*proto.RaidSimRequest, error) {
	player, err := p.Player(c)
	if err != nil {
		return nil, err
	}
	tl := p.targetLevel()
	target := &proto.Target{Level: tl, MobType: proto.MobType_MobTypeHumanoid,
		Stats: stats.Stats{stats.Armor: p.Data.targetArmor(tl)}.ToFloatArray()}
	return &proto.RaidSimRequest{
		Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{player}}}, Buffs: &proto.RaidBuffs{}, Debuffs: &proto.Debuffs{}},
		Encounter: &proto.Encounter{Duration: p.duration(), DurationVariation: 10, ExecuteProportion_20: 0.2, ExecuteProportion_35: 0.35,
			Targets: []*proto.Target{target}},
		SimOptions: &proto.SimOptions{Iterations: iterations, RandomSeed: seed, SaveAllValues: true},
	}, nil
}

// NakedConfig is the starting point: no gear, no talents, the first legal race, rotation 0.
func (p *Problem) NakedConfig() Config {
	return Config{Race: p.races[0], Talents: map[string]int32{}}
}

// PresetTalents fills a popular build in its own order until the level's points run out,
// skipping any point that would be illegal (the app's "Load a build" behavior).
func (p *Problem) PresetTalents(pr Preset) map[string]int32 {
	t := map[string]int32{}
	for _, id := range pr.order {
		if _, ok := p.recByID[id]; !ok {
			continue
		}
		for n := int32(1); n <= pr.Talents[id]; n++ {
			next := copyTalents(t)
			next[id] = n
			if p.TalentError(next) != "" {
				break
			}
			t = next
		}
	}
	return t
}

// Presets are the popular builds of the spec.
func (p *Problem) Presets() []Preset { return p.Data.Presets[p.Spec.Key] }

func copyTalents(t map[string]int32) map[string]int32 {
	out := make(map[string]int32, len(t)+1)
	for k, v := range t {
		if v > 0 {
			out[k] = v
		}
	}
	return out
}

func spent(t map[string]int32) int32 {
	s := int32(0)
	for _, v := range t {
		s += v
	}
	return s
}

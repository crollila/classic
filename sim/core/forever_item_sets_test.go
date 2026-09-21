package core

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core/gamedata"
	"github.com/wowsims/classic/sim/core/stats"
)

func syntheticSetCharacter(t *testing.T) *Character {
	t.Helper()
	// All identifiers and values in this fixture are manufactured.
	var data gamedata.Snapshot
	if err := json.Unmarshal([]byte(`{"build":"synthetic-sets","item_sets":{"900":{"name":"Synthetic","items":[901,902,903,904],"bonuses":[{"pieces":2,"spell_id":910},{"pieces":2,"spell_id":911},{"pieces":4,"spell_id":912}]}},"spells":{"910":{"effects":[{"index":0,"effect":6,"aura":29,"base":12,"misc":[0],"targets":[1]}]},"911":{"effects":[{"index":0,"effect":6,"aura":54,"base":1,"targets":[1]}]},"912":{"spell_class":"warrior","effects":[{"index":0,"effect":6,"aura":107,"base":-3000,"misc":[11],"class_mask":[16],"targets":[1]}]},"920":{"spell_class":"warrior","spell_class_mask":[16]},"921":{"spell_class":"mage","spell_class_mask":[16]}}}`), &data); err != nil {
		t.Fatal(err)
	}
	return &Character{GameData: data.Clone("synthetic-sets"), Unit: Unit{Level: 60, PseudoStats: stats.NewPseudoStats(), auraTracker: newAuraTracker()}}
}

func TestClientSetMembershipThresholdsAndDuplicateThreshold(t *testing.T) {
	c := syntheticSetCharacter(t)
	c.Equipment[0] = Item{ID: 901, SetName: "wrong inherited name"}
	if len(c.GetActiveSetBonuses()) != 0 {
		t.Fatal("one piece activated a two-piece bonus")
	}
	c.Equipment[1] = Item{ID: 902}
	if got := len(c.GetActiveSetBonuses()); got != 2 {
		t.Fatalf("both two-piece spells must activate: got %d", got)
	}
	for _, b := range c.GetActiveSetBonuses() {
		if b.NumPieces != 2 {
			t.Fatal("wrong threshold")
		}
	}
	apply, gaps := c.planClientSetSpell(c.ClientSpell(910))
	if len(gaps) != 0 {
		t.Fatal(gaps)
	}
	for _, fn := range apply {
		fn(c)
	}
	if c.GetStat(stats.Strength) != 12 {
		t.Fatal("client strength not applied")
	}
	c.Equipment[1] = Item{ID: 901}
	if len(c.GetActiveSetBonuses()) != 0 {
		t.Fatal("duplicate item IDs inflated set count")
	}
	c.Equipment[1] = Item{ID: 999, SetID: 900, SetName: "Synthetic"}
	if len(c.GetActiveSetBonuses()) != 0 {
		t.Fatal("display metadata overrode client membership")
	}
}

func TestClientSetCooldownFamilyAndBuildChanges(t *testing.T) {
	c := syntheticSetCharacter(t)
	apply, gaps := c.planClientSetSpell(c.ClientSpell(912))
	if len(gaps) != 0 {
		t.Fatal(gaps)
	}
	for _, fn := range apply {
		fn(c)
	}
	config := func(id int32) SpellConfig {
		return SpellConfig{ActionID: ActionID{SpellID: id}, Cast: CastConfig{CD: Cooldown{Timer: c.NewTimer(), Duration: 10 * time.Second}}}
	}
	if got := c.RegisterSpell(config(920)).CD.Duration; got != 7*time.Second {
		t.Fatal("matching spell cooldown", got)
	}
	if got := c.RegisterSpell(config(921)).CD.Duration; got != 10*time.Second {
		t.Fatal("unrelated spell family changed", got)
	}
	variant := syntheticSetCharacter(t)
	variant.GameData.Spell(910).Effects[0].Base = 23
	apply, _ = variant.planClientSetSpell(variant.ClientSpell(910))
	for _, fn := range apply {
		fn(variant)
	}
	if variant.GetStat(stats.Strength) != 23 || c.ClientSpell(910).Effects[0].Base != 12 {
		t.Fatal("build isolation/value update failed")
	}
}

func TestClientSetUnknownEffectsAreExplicit(t *testing.T) {
	c := syntheticSetCharacter(t)
	c.Equipment[0] = Item{ID: 901}
	c.Equipment[1] = Item{ID: 902}
	c.GameData.Spell(910).Effects[0].Aura = 4
	if !strings.Contains(c.GetActiveSetBonuses()[0].Name, "incomplete") {
		t.Fatal("dummy script silently accepted")
	}
	if c.HasSetBonus(&ItemSet{Name: "Synthetic", Bonuses: map[int32]ApplyEffect{2: func(Agent) {}}}, 2) {
		t.Fatal("Classic callback leaked into Forever")
	}
}

func TestClientSetUnsafeConditionsAndScalingAreRejected(t *testing.T) {
	for _, kind := range []string{"item restriction", "temporary", "level scaling", "variable", "cast haste", "nil effect", "split rating"} {
		t.Run(kind, func(t *testing.T) {
			c := syntheticSetCharacter(t)
			s := c.ClientSpell(910)
			switch kind {
			case "item restriction":
				v := 2
				s.RequiresItemClass = &v
			case "temporary":
				s.DurationMs = 10000
			case "level scaling":
				s.Effects[0].PerLevel = 1
			case "variable":
				s.Effects[0].Variance = 1
			case "cast haste":
				s.Effects[0].Aura = 65
			case "nil effect":
				s.Effects[0] = nil
			case "split rating":
				s.Effects[0].Aura = 189
				s.Effects[0].Misc = []int32{32}
			}
			actions, gaps := c.planClientSetSpell(s)
			if len(actions) != 0 || len(gaps) == 0 {
				t.Fatalf("unsafe bonus treated as unconditional: actions=%d gaps=%v", len(actions), gaps)
			}
		})
	}
}

func TestClientSetPieceRemovalAndAllThresholds(t *testing.T) {
	c := syntheticSetCharacter(t)
	for i, id := range []int32{901, 902, 903, 904} {
		c.Equipment[i] = Item{ID: id}
	}
	if got := len(c.GetActiveSetBonuses()); got != 3 {
		t.Fatalf("four pieces: got %d bonuses", got)
	}
	c.Equipment[3] = Item{}
	if got := len(c.GetActiveSetBonuses()); got != 2 {
		t.Fatalf("removing fourth piece did not remove four-piece bonus: %d", got)
	}
	c.Equipment[2], c.Equipment[1] = Item{}, Item{}
	if len(c.GetActiveSetBonuses()) != 0 {
		t.Fatal("removing set pieces left a bonus eligible")
	}
}

func TestClientSetSnapshotCoverage(t *testing.T) {
	c := &Character{GameData: gamedata.Current(), Unit: Unit{Level: 60}}
	full, partial, missing := 0, 0, 0
	type row struct {
		SetID            string   `json:"set_id"`
		Name             string   `json:"name"`
		Pieces           int      `json:"pieces"`
		SpellID          int32    `json:"spell_id"`
		SupportedEffects int      `json:"supported_effects"`
		Gaps             []string `json:"gaps"`
	}
	var rows []row
	for id, set := range c.GameData.RawItemSets {
		for _, bonus := range set.Bonuses {
			actions, gaps := c.planClientSetSpell(c.ClientSpell(bonus.SpellID))
			rows = append(rows, row{id, set.Name, bonus.Pieces, bonus.SpellID, len(actions), gaps})
			if len(gaps) == 0 {
				full++
			} else if len(actions) > 0 {
				partial++
			} else {
				missing++
			}
		}
	}
	t.Logf("Client set bonus spell rows: complete=%d partial=%d unsupported=%d", full, partial, missing)
	if full == 0 {
		t.Fatal("no set bonus coverage")
	}
	if path := os.Getenv("FOREVER_SET_COVERAGE_PATH"); path != "" {
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].SetID != rows[j].SetID {
				return rows[i].SetID < rows[j].SetID
			}
			if rows[i].Pieces != rows[j].Pieces {
				return rows[i].Pieces < rows[j].Pieces
			}
			return rows[i].SpellID < rows[j].SpellID
		})
		body, err := json.MarshalIndent(struct {
			Build string `json:"build"`
			Note  string `json:"note"`
			Rows  []row  `json:"rows"`
		}{c.GameData.Build, "Handler coverage, not proof of live availability or server behavior. Legacy and unconfirmed sets remain in client data.", rows}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(body, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

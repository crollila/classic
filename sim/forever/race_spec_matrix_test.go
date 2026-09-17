package forever_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
)

// Exercise the same default racial selections exposed by the Forever browser.
// Construction must succeed before the UI can display stats or configure an APL.
func TestForeverEveryRaceAndSpecBothModes(t *testing.T) {
	raw, err := os.ReadFile("../core/foreverdata/trees.json")
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		RaceClasses map[string][]string `json:"race_classes"`
		Mechanics   []struct{ ID, Kind, Category, Mode string }
	}
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	races := map[string]string{"orc": "Orc", "undead": "Undead", "tauren": "Tauren", "troll": "Troll", "human": "Human", "dwarf": "Dwarf", "night-elf": "NightElf", "gnome": "Gnome", "skyborne-windshaper": "SkyborneWindshaper", "skyborne-high-order": "SkyborneHighOrder"}
	specs := map[string][]string{"Warrior": {"warrior", "tankWarrior"}, "Paladin": {"retributionPaladin", "protectionPaladin", "holyPaladin"}, "Hunter": {"hunter"}, "Rogue": {"rogue"}, "Priest": {"shadowPriest", "healingPriest"}, "Shaman": {"elementalShaman", "enhancementShaman", "restorationShaman", "wardenShaman"}, "Mage": {"mage"}, "Warlock": {"warlock"}, "Druid": {"balanceDruid", "feralDruid", "feralTankDruid", "restorationDruid"}}
	for class, variants := range specs {
		for raceKey, race := range races {
			eligible := false
			for _, c := range data.RaceClasses[raceKey] {
				eligible = eligible || c == strings.ToUpper(class)
			}
			if !eligible {
				continue
			}
			for _, spec := range variants {
				for _, mode := range []proto.ForeverMode{proto.ForeverMode_BEST_GUESS, proto.ForeverMode_STRICT} {
					if mode == proto.ForeverMode_STRICT && strings.HasPrefix(raceKey, "skyborne-") {
						continue
					}
					t.Run(spec+"/"+race+"/"+mode.String(), func(t *testing.T) {
						p := discoveryPlayer(t, class, race, spec, map[string]int32{})
						p.Forever.Mode = mode
						for _, m := range data.Mechanics {
							if foreverdata.KnownMechanic(m.ID) && ((m.Kind == "racial" && strings.HasPrefix(m.ID, "racials."+raceKey+".")) || (m.Category == strings.ToUpper(class) && m.Kind != "removed_talent")) {
								p.Forever.Mechanics = append(p.Forever.Mechanics, m.ID)
							}
						}
						if err := foreverdata.Validate(p); err != nil {
							t.Fatal(err)
						}
						request := &proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}}, Encounter: &proto.Encounter{Duration: 3, Targets: []*proto.Target{{Level: 63}}}, SimOptions: &proto.SimOptions{Iterations: 2, RandomSeed: 1, IsTest: true}}
						result := core.RunRaidSim(request)
						if result.Error != nil {
							t.Fatal(result.Error.Message)
						}
						if len(result.RaidMetrics.Parties) != 1 || len(result.RaidMetrics.Parties[0].Players) != 1 {
							t.Fatal("missing class result")
						}
					})
				}
			}
		}
	}
}

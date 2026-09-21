package core

import (
	"fmt"
	"strings"

	"github.com/wowsims/classic/sim/core/gamedata"
	"github.com/wowsims/classic/sim/core/proto"
)

// foreverGameData selects the client build a Forever character simulates with.
func foreverGameData(player *proto.Player) *gamedata.Snapshot {
	if player.Forever == nil {
		return nil
	}
	snapshot, err := gamedata.ForBuild(player.Forever.GameDataBuild)
	if err != nil {
		panic(err.Error())
	}
	return snapshot
}

// ClientSpell is the Forever client's spell for id in this character's build, or nil (Classic
// mode, or a spell the build does not have).
func (character *Character) ClientSpell(id int32) *gamedata.Spell {
	if character.GameData == nil {
		return nil
	}
	return character.GameData.Spell(id)
}

// ClientEffectRange is effect `index` of client spell `id` as a (min, max) range. Without client
// data it returns the given Classic range and records the fallback, so it is never silent.
func (character *Character) ClientEffectRange(id int32, index int, classicMin, classicMax float64) (float64, float64) {
	if character.GameData == nil {
		return classicMin, classicMax
	}
	if effect := character.GameData.Spell(id).Effect(index); effect != nil {
		return effect.Range()
	}
	owner := fmt.Sprintf("spell %d effect %d", id, index)
	gamedata.Use(owner, "min", classicMin, "UNKNOWN", "client build has no such spell/effect; Classic value used")
	gamedata.Use(owner, "max", classicMax, "UNKNOWN", "client build has no such spell/effect; Classic value used")
	return classicMin, classicMax
}

// ClientEffectValue is ClientEffectRange's single value (the client's base points).
func (character *Character) ClientEffectValue(id int32, index int, classic float64) float64 {
	if character.GameData == nil {
		return classic
	}
	if effect := character.GameData.Spell(id).Effect(index); effect != nil {
		return effect.Base
	}
	return gamedata.Use(fmt.Sprintf("spell %d effect %d", id, index), "value", classic, "UNKNOWN",
		"client build has no such spell/effect; Classic value used")
}

// ClientTalentValue is a Forever talent's client value at the character's rank for `effect`
// (per-rank curve points, else the talent spell's effect base). `classic` is returned, and
// recorded, only when the build has no client twin for the record.
func (character *Character) ClientTalentValue(recordID string, effect int, classic float64) float64 {
	rank := int(character.ForeverRank(recordID))
	if character.GameData == nil || rank == 0 {
		return classic
	}
	talent := character.GameData.TalentForRecord(recordID)
	if talent != nil {
		if v, ok := talent.Points(rank, effect); ok {
			return v
		}
		if e := character.GameData.Spell(talent.SpellID).Effect(effect); e != nil {
			return e.Base
		}
	}
	return gamedata.Use("talent "+recordID, fmt.Sprintf("effect %d", effect), classic, "UNKNOWN",
		"no client talent value for this record; value from the talent's research record used")
}

// provenance records what a result was computed from: the client game data of the first Forever
// player, the seed actually used, and every non-client value the process has used.
func (sim *Simulation) provenance() *proto.SimProvenance {
	out := &proto.SimProvenance{RandomSeed: sim.rseed}
	foundBuild := false
	for _, party := range sim.Raid.Parties {
		for _, agent := range party.Players {
			if data := agent.GetCharacter().GameData; data != nil {
				if !foundBuild {
					out.GameDataBuild, out.GameDataSha256, out.ClassicReferenceBuild = data.Build, data.SnapshotSHA256, data.ClassicBuild
					foundBuild = true
				} else if data.Build != out.GameDataBuild || data.SnapshotSHA256 != out.GameDataSha256 {
					out.NonClientValues = append(out.NonClientValues, fmt.Sprintf("Additional player build: %s | %s | %s", agent.GetCharacter().Label, data.Build, data.SnapshotSHA256))
				}
				for _, bonus := range agent.GetCharacter().GetActiveSetBonuses() {
					if strings.Contains(bonus.Name, "[incomplete:") {
						out.NonClientValues = append(out.NonClientValues, fmt.Sprintf("Set coverage: %s | %s (%dpc); unsupported effects are NOT applied", agent.GetCharacter().Label, bonus.Name, bonus.NumPieces))
					}
				}
			}
		}
	}
	if foundBuild {
		for _, f := range gamedata.Fallbacks() {
			out.NonClientValues = append(out.NonClientValues, fmt.Sprintf("%s | %s = %g (%s)", f.Owner, f.What, f.Value, f.Confidence))
		}
	}
	return out
}

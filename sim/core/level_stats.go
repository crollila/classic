package core

import (
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

// PlayerLevel is the level a request simulates: the player's own level when set, else the cap.
func PlayerLevel(player *proto.Player) int32 {
	if player == nil || player.Level < 1 || player.Level > CharacterMaxLevel {
		return CharacterMaxLevel
	}
	return player.Level
}

// getBaseStatsAtLevel is getBaseStatsCombo for any level; the cap keeps the engine's own table.
func getBaseStatsAtLevel(r proto.Race, c proto.Class, level int32) stats.Stats {
	table, ok := ClassLevelStats[c]
	if level >= CharacterMaxLevel || level < 1 || !ok {
		return getBaseStatsCombo(r, c)
	}
	row := table[level]
	base := stats.Stats{}
	base[stats.Health] = row[0]
	base[stats.Mana] = row[1]
	base[stats.Strength] = row[2]
	base[stats.Agility] = row[3]
	base[stats.Stamina] = row[4]
	base[stats.Intellect] = row[5]
	base[stats.Spirit] = row[6]
	// Same shape as the level-60 table: melee classes gain base attack power per level.
	l := float64(level)
	switch c {
	case proto.Class_ClassWarrior, proto.Class_ClassPaladin:
		base[stats.AttackPower] = l*3 - 20
	case proto.Class_ClassHunter:
		base[stats.AttackPower] = l*2 - 20
		base[stats.RangedAttackPower] = l*2 - 20
	case proto.Class_ClassRogue, proto.Class_ClassShaman:
		base[stats.AttackPower] = l*2 - 20
	case proto.Class_ClassDruid:
		base[stats.AttackPower] = -20
	default:
		base[stats.AttackPower] = -10
	}
	return base.Add(RaceOffsets[r]).Add(ClassBaseCrit[c])
}

func levelCurve(curve map[proto.Class][61]float64, class proto.Class, level int32) float64 {
	row, ok := curve[class]
	if !ok || level < 1 || level >= CharacterMaxLevel {
		return 1
	}
	return row[level]
}

// CritPerAgiAt is CritPerAgiAtLevel for a unit of the given level.
func CritPerAgiAt(class proto.Class, level int32) float64 {
	return CritPerAgiAtLevel[class] * levelCurve(meleeCritCurve, class, level)
}

// DodgePerAgiAt follows the melee crit curve, as dodge from agility does in the client tables.
func DodgePerAgiAt(class proto.Class, level int32) float64 {
	return DodgePerAgiAtLevel[class] * levelCurve(meleeCritCurve, class, level)
}

// CritPerIntAt is CritPerIntAtLevel for a unit of the given level.
func CritPerIntAt(class proto.Class, level int32) float64 {
	return CritPerIntAtLevel[class] * levelCurve(spellCritCurve, class, level)
}

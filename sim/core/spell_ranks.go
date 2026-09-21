package core

// A rotation names one rank of a spell; a character below the level that learns it
// knows a lower rank instead. These resolve a spell or aura id to the highest rank the
// unit actually registered, so a rotation written at the level cap runs at any level.

// Classic ranks the Forever client folds into fewer ranks. Rotations written for Classic still
// name the Classic ids, so they resolve through these to whatever rank the unit registered.
var classicRankFamilies = map[string][][2]int32{
	"Classic|Multi-Shot":   {{2643, 18}, {14288, 30}, {14289, 42}, {14290, 54}, {25294, 60}},
	"Classic|Tiger's Fury": {{5217, 24}, {6793, 36}, {9845, 48}, {9846, 60}},
}

var spellRankFamily = func() map[int32][][2]int32 {
	byID := make(map[int32][][2]int32)
	for _, ranks := range classicRankFamilies {
		for _, rank := range ranks {
			byID[rank[0]] = ranks
		}
	}
	for _, ranks := range spellRankFamilies {
		for _, rank := range ranks {
			byID[rank[0]] = ranks
		}
	}
	return byID
}()

// SpellLearnedLevel is the level a trainer spell rank is learned at, 0 when unknown.
func SpellLearnedLevel(spellID int32) int32 {
	for _, rank := range spellRankFamily[spellID] {
		if rank[0] == spellID {
			return rank[1]
		}
	}
	return 0
}

// lowerRanks returns the other ranks of spellID's family, highest first.
func lowerRanks(spellID int32) []int32 {
	ranks := spellRankFamily[spellID]
	ids := make([]int32, 0, len(ranks))
	for i := len(ranks) - 1; i >= 0; i-- {
		if ranks[i][0] != spellID {
			ids = append(ids, ranks[i][0])
		}
	}
	return ids
}

func (unit *Unit) getSpellAnyRank(actionID ActionID) *Spell {
	if spell := unit.GetSpell(actionID); spell != nil || actionID.SpellID == 0 {
		return spell
	}
	for _, id := range lowerRanks(actionID.SpellID) {
		if spell := unit.GetSpell(ActionID{SpellID: id, Tag: actionID.Tag}); spell != nil {
			return spell
		}
	}
	return nil
}

func (unit *Unit) getAuraAnyRank(actionID ActionID, get func(*Unit, ActionID) *Aura) *Aura {
	if aura := get(unit, actionID); aura != nil || actionID.SpellID == 0 {
		return aura
	}
	for _, id := range lowerRanks(actionID.SpellID) {
		if aura := get(unit, ActionID{SpellID: id, Tag: actionID.Tag}); aura != nil {
			return aura
		}
	}
	return nil
}

// TrainerRanks returns every rank of a trainer spell family, e.g. "Rogue|Sinister Strike",
// as {spell id, learned level}, lowest rank first. Nil when the family is unknown.
func TrainerRanks(family string) [][2]int32 {
	return spellRankFamilies[family]
}

// TrainerRankAt is the highest rank of a trainer spell family a unit of the given level knows:
// its 1-based rank number and spell id. rank is 0 when the level is below the first rank.
func TrainerRankAt(family string, level int32) (rank int, spellID int32) {
	for i, r := range spellRankFamilies[family] {
		if r[1] <= level {
			rank, spellID = i+1, r[0]
		}
	}
	return rank, spellID
}

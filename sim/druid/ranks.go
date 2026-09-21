package druid

// catRank is one trainer rank of a feral ability: spell id, learned level and its
// rank-dependent value (flat damage bonus).
type catRank struct {
	id    int32
	level int32
	value float64
}

// catRankAt returns the highest rank learned at the given level; ok is false when
// the first rank is not learned yet.
func catRankAt(ranks []catRank, level int32) (rank catRank, ok bool) {
	for _, r := range ranks {
		if r.level <= level {
			rank, ok = r, true
		}
	}
	return rank, ok
}

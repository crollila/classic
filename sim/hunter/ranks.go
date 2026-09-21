package hunter

// rankForLevel returns the highest rank whose learned level (levels[rank], index 0
// unused) is at most level, or 0 when the first rank is not learned yet.
func rankForLevel(levels []int, level int32) int {
	rank := 0
	for r := 1; r < len(levels); r++ {
		if int32(levels[r]) <= level {
			rank = r
		}
	}
	return rank
}

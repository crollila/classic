package warrior

import (
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/gamedata"
)

// Forever values read from the client build the warrior simulates with (character.GameData).
//
// Every helper takes the value the code carries as its fallback: in Classic mode (no client data)
// that is the Classic value and is returned unchanged, so Classic results do not move. In Forever
// mode the client's value is returned; where the client does not state the number, the code value
// is returned and recorded with gamedata.Use, so no Forever fallback is silent.

// clientTalent is a Forever talent's value at the warrior's rank, in the units the code uses: the
// client's per-rank value of `effect` (Trait curve points, else the talent spell's effect base)
// times `scale` (e.g. 0.1 for rage the client stores in tenths, 0.001 for milliseconds, a negative
// scale for reductions the client stores as negative modifiers). `fallback` is what the code used
// before (research record or Classic value); it is returned when the talent is not taken, and when
// the build has no client data.
func clientTalent(c *core.Character, record string, effect int, scale, fallback float64) float64 {
	if c.ForeverRank(record) == 0 {
		return fallback
	}
	return c.ClientTalentValue(record, effect, fallback/scale) * scale
}

// clientRankValue is effect `index` of a learned rank's client spell (its base points), else the
// rank table's value.
func clientRankValue(c *core.Character, spellID int32, index int, table float64) float64 {
	return c.ClientEffectValue(spellID, index, table)
}

// clientDuration is a client spell's duration, or `fallback` (recorded) when the client has none.
func clientDuration(c *core.Character, spellID int32, owner string, fallback time.Duration) time.Duration {
	if c.GameData == nil {
		return fallback
	}
	if s := c.ClientSpell(spellID); s != nil && s.DurationMs > 0 {
		return time.Duration(s.DurationMs) * time.Millisecond
	}
	gamedata.Use(owner, "duration_s", fallback.Seconds(), "PROVISIONAL", "the client spell states no duration; the code's value is used")
	return fallback
}

// clientProcChance is a client spell's proc chance as a fraction, or `fallback` (recorded).
func clientProcChance(c *core.Character, spellID int32, owner string, fallback float64) float64 {
	if c.GameData == nil {
		return fallback
	}
	if s := c.ClientSpell(spellID); s != nil && s.ProcChance > 0 {
		return s.ProcChance / 100
	}
	gamedata.Use(owner, "proc chance", fallback, "PROVISIONAL", "the client spell states no proc chance; the code's value is used")
	return fallback
}

// codeValue returns a number the client does not state and, in a Forever run with client data,
// records it (confidence PROVISIONAL: carried from Classic/research; PREDICTED: an analogue).
func codeValue(c *core.Character, owner, what string, value float64, confidence, reason string) float64 {
	if c.GameData == nil {
		return value
	}
	return gamedata.Use(owner, what, value, confidence, reason)
}

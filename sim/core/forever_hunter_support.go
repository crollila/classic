package core

import (
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
	"math"
)

// ForeverEnableManaPool supplies a finite, explicit encounter mana pool without
// inventing a NPC intellect/regen formula. Call during Initialize, before finalize.
// The caller exposes the amount as a scenario parameter; zero leaves it absent.
func (u *Unit) ForeverEnableManaPool(amount float64) {
	if u.HasManaBar() || amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return
	}
	u.AddStat(stats.Mana, amount-u.GetStat(stats.Mana))
	u.manaBar.unit = u
	u.BaseMana = amount
	u.SpiritManaRegenPerSecond = func() float64 { return 0 }
	u.manaCastingMetrics = u.NewManaMetrics(ActionID{OtherID: proto.OtherAction_OtherActionManaRegen, Tag: 1})
	u.manaNotCastingMetrics = u.NewManaMetrics(ActionID{OtherID: proto.OtherAction_OtherActionManaRegen, Tag: 2})
	u.GetOrRegisterSpell(SpellConfig{ActionID: ActionID{OtherID: proto.OtherAction_OtherActionManaGain}})
}

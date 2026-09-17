package druid

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
)

type foreverDruid struct {
	*Druid
	bear bool
}

func (d *Druid) GetDruid() *Druid { return d }
func (d *foreverDruid) Initialize() {
	d.Druid.Initialize()
	d.RegisterBalanceSpells()
	d.RegisterFeralCatSpells()
}
func (d *foreverDruid) Reset(sim *core.Simulation) {
	d.Druid.Reset(sim)
	d.CancelShapeshift(sim)
	if d.bear {
		d.BearFormAura.Activate(sim)
	}
}
func newForeverDruid(c *core.Character, p *proto.Player, bear bool) core.Agent {
	if c.Forever == nil {
		panic("Restoration/Bear Druid requires the Forever ruleset")
	}
	form := Humanoid
	if bear {
		form = Bear
	}
	d := &foreverDruid{Druid: New(c, form, SelfBuffs{}, p.TalentsString), bear: bear}
	d.EnableAutoAttacks(d, core.AutoAttackOptions{MainHand: d.WeaponFromMainHand(), AutoSwingMelee: true})
	return d
}
func RegisterForeverDruidSpecs() {
	core.RegisterAgentFactory(proto.Player_RestorationDruid{}, proto.Spec_SpecRestorationDruid, func(c *core.Character, p *proto.Player) core.Agent { return newForeverDruid(c, p, false) }, func(p *proto.Player, s interface{}) { p.Spec = s.(*proto.Player_RestorationDruid) })
	core.RegisterAgentFactory(proto.Player_FeralTankDruid{}, proto.Spec_SpecFeralTankDruid, func(c *core.Character, p *proto.Player) core.Agent { return newForeverDruid(c, p, true) }, func(p *proto.Player, s interface{}) { p.Spec = s.(*proto.Player_FeralTankDruid) })
}

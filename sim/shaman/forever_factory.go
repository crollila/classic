package shaman

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
)

func (s *Shaman) GetShaman() *Shaman { return s }
func RegisterForeverRestorationShaman() {
	core.RegisterAgentFactory(proto.Player_RestorationShaman{}, proto.Spec_SpecRestorationShaman, func(c *core.Character, p *proto.Player) core.Agent {
		if c.Forever == nil {
			panic("Restoration Shaman requires the Forever ruleset")
		}
		s := NewShaman(c, p.TalentsString)
		s.EnableAutoAttacks(s, core.AutoAttackOptions{MainHand: s.WeaponFromMainHand(), AutoSwingMelee: true})
		return s
	}, func(p *proto.Player, s interface{}) { p.Spec = s.(*proto.Player_RestorationShaman) })
}

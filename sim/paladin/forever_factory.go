package paladin

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
)

func RegisterForeverHolyPaladin() {
	core.RegisterAgentFactory(proto.Player_HolyPaladin{}, proto.Spec_SpecHolyPaladin, func(c *core.Character, p *proto.Player) core.Agent {
		if c.Forever == nil {
			panic("Holy Paladin requires the Forever ruleset")
		}
		options := p.GetHolyPaladin().GetOptions()
		if options == nil {
			options = &proto.PaladinOptions{}
		}
		pal := NewPaladin(c, p, options)
		pal.EnableAutoAttacks(pal, core.AutoAttackOptions{MainHand: pal.WeaponFromMainHand(), AutoSwingMelee: true})
		return pal
	}, func(p *proto.Player, s interface{}) { p.Spec = s.(*proto.Player_HolyPaladin) })
}

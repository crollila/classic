package feral

import (
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/druid"
)

func RegisterFeralDruid() {
	core.RegisterAgentFactory(
		proto.Player_FeralDruid{},
		proto.Spec_SpecFeralDruid,
		func(character *core.Character, options *proto.Player) core.Agent {
			return NewFeralDruid(character, options)
		},
		func(player *proto.Player, spec interface{}) {
			playerSpec, ok := spec.(*proto.Player_FeralDruid)
			if !ok {
				panic("Invalid spec value for Feral Druid!")
			}
			player.Spec = playerSpec
		},
	)
}

func NewFeralDruid(character *core.Character, options *proto.Player) *FeralDruid {
	feralOptions := options.GetFeralDruid()
	selfBuffs := druid.SelfBuffs{}

	cat := &FeralDruid{
		Druid:   druid.New(character, druid.Cat, selfBuffs, options.TalentsString),
		latency: time.Duration(max(feralOptions.Options.LatencyMs, 1)) * time.Millisecond,
	}

	cat.SelfBuffs.InnervateTarget = &proto.UnitReference{}
	if feralOptions.Options.InnervateTarget == nil || feralOptions.Options.InnervateTarget.Type == proto.UnitReference_Unknown {
		cat.SelfBuffs.InnervateTarget = &proto.UnitReference{
			Type: proto.UnitReference_Self,
		}
	} else {
		cat.SelfBuffs.InnervateTarget = feralOptions.Options.InnervateTarget
	}

	cat.maxRipTicks = druid.RipTicks

	if !cat.HasEnergyBar() {
		cat.EnableEnergyBar(100.0)
	}
	if !cat.HasRageBar() {
		cat.EnableRageBar(core.RageBarOptions{DamageDealtMultiplier: 1, DamageTakenMultiplier: 1})
	}

	cat.EnableAutoAttacks(cat, core.AutoAttackOptions{
		// Base paw weapon.
		MainHand:       cat.GetCatWeapon(),
		AutoSwingMelee: true,
	})
	/*
		cat.ReplaceBearMHFunc = func(sim *core.Simulation, mhSwingSpell *core.Spell) *core.Spell {
			return cat.checkReplaceMaul(sim, mhSwingSpell)
		}*/

	cat.PseudoStats.FeralCombatEnabled = true

	return cat
}

type FeralDruid struct {
	*druid.Druid

	Rotation FeralDruidRotation

	missChance    float64
	readyToShift  bool
	latency       time.Duration
	maxRipTicks   int32
	bleedAura     *core.Aura
	lastShift     time.Duration
	poolingMana   bool
	poolStartTime time.Duration

	rotationAction *core.PendingAction
}

func (cat *FeralDruid) GetDruid() *druid.Druid {
	return cat.Druid
}

func (cat *FeralDruid) MissChance() float64 {
	at := cat.AttackTables[cat.CurrentTarget.UnitIndex][proto.CastType_CastTypeMainHand]
	builder := cat.Shred
	if builder == nil {
		builder = cat.Claw // Shred is learned at level 22
	}
	if builder == nil {
		return at.BaseMissChance + at.BaseDodgeChance
	}
	miss := at.BaseMissChance - builder.PhysicalHitChance(at)
	dodge := at.BaseDodgeChance
	return miss + dodge
}

func (cat *FeralDruid) Initialize() {
	cat.Druid.Initialize()
	cat.RegisterBalanceSpells()
	cat.RegisterFeralCatSpells()
}

func (cat *FeralDruid) Reset(sim *core.Simulation) {
	cat.Druid.Reset(sim)
	cat.Druid.CancelShapeshift(sim)
	cat.CatFormAura.Activate(sim)
	cat.readyToShift = false
	//cat.berserkUsed = false
	cat.poolingMana = false
	cat.rotationAction = nil
}

// furorRank is the Furor rank the rotation plans powershifts with. Forever Furor is a
// class-new talent, so the Classic talent field stays 0 there.
func (cat *FeralDruid) furorRank() int32 {
	if cat.Forever != nil {
		return cat.ForeverRank("druid.talent.furor")
	}
	return cat.Talents.Furor
}

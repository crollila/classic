package sim

import (
	_ "github.com/wowsims/classic/sim/common"
	"github.com/wowsims/classic/sim/druid"
	"github.com/wowsims/classic/sim/druid/balance"
	"github.com/wowsims/classic/sim/paladin"
	"github.com/wowsims/classic/sim/paladin/retribution"
	dpsrogue "github.com/wowsims/classic/sim/rogue/dps_rogue"
	"github.com/wowsims/classic/sim/shaman"
	"github.com/wowsims/classic/sim/shaman/elemental"
	"github.com/wowsims/classic/sim/shaman/enhancement"
	"github.com/wowsims/classic/sim/shaman/warden"

	"github.com/wowsims/classic/sim/druid/feral"
	// restoDruid "github.com/wowsims/classic/sim/druid/restoration"
	// feralTank "github.com/wowsims/classic/sim/druid/tank"
	_ "github.com/wowsims/classic/sim/encounters"
	"github.com/wowsims/classic/sim/hunter"
	"github.com/wowsims/classic/sim/mage"

	// holyPaladin "github.com/wowsims/classic/sim/paladin/holy"
	"github.com/wowsims/classic/sim/paladin/protection"
	// "github.com/wowsims/classic/sim/paladin/retribution"
	healingPriest "github.com/wowsims/classic/sim/priest/healing"
	"github.com/wowsims/classic/sim/priest/shadow"

	// restoShaman "github.com/wowsims/classic/sim/shaman/restoration"
	dpsWarlock "github.com/wowsims/classic/sim/warlock/dps"
	dpsWarrior "github.com/wowsims/classic/sim/warrior/dps_warrior"
	tankWarrior "github.com/wowsims/classic/sim/warrior/tank_warrior"
)

var registered = false

func RegisterAll() {
	if registered {
		return
	}
	registered = true

	balance.RegisterBalanceDruid()
	feral.RegisterFeralDruid()
	// feralTank.RegisterFeralTankDruid()
	druid.RegisterForeverDruidSpecs()
	elemental.RegisterElementalShaman()
	enhancement.RegisterEnhancementShaman()
	warden.RegisterWardenShaman()
	shaman.RegisterForeverRestorationShaman()
	hunter.RegisterHunter()
	mage.RegisterMage()
	healingPriest.RegisterHealingPriest()
	shadow.RegisterShadowPriest()
	dpsrogue.RegisterDpsRogue()
	dpsWarrior.RegisterDpsWarrior()
	tankWarrior.RegisterTankWarrior()
	paladin.RegisterForeverHolyPaladin()
	protection.RegisterProtectionPaladin()
	retribution.RegisterRetributionPaladin()
	dpsWarlock.RegisterDpsWarlock()
}

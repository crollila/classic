package mage

import (
	"github.com/wowsims/classic/sim/core"
	"time"
)

// foreverMageRank is one rank of a Forever baseline spell: its spell id, learned level,
// mana cost and damage (or absorb amount in low). The last rank of each table keeps the
// values this file used before ranks were level-aware, so level 60 is unchanged; lower
// ranks come from the Forever beta client tooltips.
type foreverMageRank struct {
	id        int32
	level     int32
	mana      float64
	low, high float64
}

var foreverFrostNovaRanks = []foreverMageRank{
	{122, 10, 55, 21, 23}, {865, 26, 85, 34, 38}, {6131, 40, 115, 52, 58}, {10230, 54, 205, 71, 80},
}
var foreverConeOfColdRanks = []foreverMageRank{
	{120, 26, 210, 96, 106}, {8492, 34, 290, 142, 156}, {10159, 42, 380, 200, 220}, {10160, 50, 465, 260, 286}, {10161, 58, 555, 335, 365},
}
var foreverFireWardRanks = []foreverMageRank{
	{543, 20, 85, 162, 0}, {8457, 30, 135, 285, 0}, {8458, 40, 195, 463, 0}, {10223, 50, 255, 668, 0}, {10225, 60, 320, 920, 0},
}
var foreverFrostWardRanks = []foreverMageRank{
	{6143, 22, 85, 162, 0}, {8461, 32, 135, 284, 0}, {8462, 42, 195, 463, 0}, {10177, 52, 255, 668, 0}, {28609, 60, 320, 920, 0},
}
var foreverManaShieldRanks = []foreverMageRank{
	{1463, 20, 40, 120, 0}, {8494, 28, 60, 210, 0}, {8495, 36, 80, 300, 0}, {10191, 44, 100, 390, 0}, {10192, 52, 120, 480, 0}, {10193, 60, 180, 570, 0},
}

// Frostfire Bolt, Arcane Blast and Ice Lance rank ids and learned levels. In Forever mode
// every number is read from the client spell of the rank (forever_client.go); the values
// typed here (client tooltips of 1.60.1.69876 at the rank's top level) are the fallback
// used only when a build lacks the spell. The top Frostfire Bolt rank (1237313) keeps the
// Forever action so the rotations that name it keep working.
var foreverFrostfireBoltRanks = []foreverMageRank{
	{401502, 40, 205, 102, 118}, {1237312, 50, 285, 181, 209}, {1237313, 60, 370, 270, 314},
}

// foreverFrostfireBoltDot is each Frostfire Bolt rank's periodic damage in total over
// 9 sec (3 ticks), keyed by the rank's learned level. The client lists no spell power
// coefficient for the periodic part.
var foreverFrostfireBoltDot = map[int32]float64{40: 27, 50: 39, 60: 57}

// Arcane Blast costs 15% of base mana at every rank, so mana is unused here.
var foreverArcaneBlastRanks = []foreverMageRank{
	{400574, 20, 0, 57, 65}, {1239696, 30, 0, 131, 151}, {1239697, 40, 0, 168, 194}, {1239699, 50, 0, 269, 311}, {1239700, 60, 0, 364, 424},
}
var foreverIceLanceRanks = []foreverMageRank{
	{1312002, 20, 45, 28, 32}, {400640, 28, 55, 34, 40}, {1240044, 34, 70, 44, 52}, {1240045, 42, 105, 76, 90}, {1240046, 48, 120, 95, 111}, {1240047, 56, 160, 136, 160},
}

// foreverArcaneMissilesTick is the Forever damage of one missile per rank (index = rank),
// from the client tooltips; the tick counts and mana costs are Classic's.
var foreverArcaneMissilesTick = [ArcaneMissilesRanks + 1]float64{0, 25, 32, 46, 68, 97, 133, 174, 209}

// foreverArcaneMissilesCoeff is the per-missile coefficient ElliotWood/Forever reads from
// the client's missile spell (the tooltip prints none): 1/3.5 at every rank.
const foreverArcaneMissilesCoeff = .286

// foreverTalentRankAt is the rank a talent-taught spell uses at a level: the highest rank
// learned by then, or rank 1 when the talent is taken before rank 1's listed level.
func foreverTalentRankAt(level int32, ranks []foreverMageRank) foreverMageRank {
	if r, ok := foreverRankAt(level, ranks); ok {
		return r
	}
	return ranks[0]
}

// foreverRankAt is the highest rank learned by the given level; false when none is.
func foreverRankAt(level int32, ranks []foreverMageRank) (foreverMageRank, bool) {
	var best foreverMageRank
	ok := false
	for _, r := range ranks {
		if r.level <= level {
			best, ok = r, true
		}
	}
	return best, ok
}

func (m *Mage) registerForeverDefenses() {
	if m.Forever == nil {
		return
	}
	f := m.foreverState
	// Talent values by client effect index (signs and units are the client's).
	val := func(id string, index int) float64 { return m.clientTalent(id, index, 0) }
	// Ice Barrier absorbs: the client rank's absorb effect (0) at the character's level;
	// the typed amounts are the fallback.
	amounts := []float64{0, 431, 549, 678, 818}
	for i, s := range m.IceBarrier {
		if s != nil && i < len(amounts) {
			amounts[i], _ = foreverClientRange(m.GetCharacter(), s.SpellID, 0, amounts[i], amounts[i])
		}
	}
	if m.ForeverRank("mage.talent.ice-barrier") > 0 {
		shield := core.NewForeverAbsorb(&m.Unit, "Forever Ice Barrier", m.ForeverAction("mage.talent.ice-barrier"), time.Minute, 0)
		for i, s := range m.IceBarrier {
			if s == nil {
				continue
			}
			i, s := i, s
			old := s.ApplyEffects
			s.ApplyEffects = func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
				old(sim, t, sp)
				shield.Apply(sim, amounts[i], false)
			}
		}
		a := shield.Aura
		oldGain, oldExpire := a.OnGain, a.OnExpire
		a.OnGain = func(a *core.Aura, sim *core.Simulation) {
			if oldGain != nil {
				oldGain(a, sim)
			}
			for _, sp := range m.Spellbook {
				sp.PushbackReduction += 1
			}
		}
		a.OnExpire = func(a *core.Aura, sim *core.Simulation) {
			if oldExpire != nil {
				oldExpire(a, sim)
			}
			for _, sp := range m.Spellbook {
				sp.PushbackReduction -= 1
			}
		}
	}
	if m.ForeverRank("mage.talent.ice-block") > 0 {
		a := m.ForeverImmobileAura("Forever Ice Block", m.ForeverAction("mage.talent.ice-block"), 10*time.Second)
		m.OnSpellRegistered(func(s *core.Spell) {
			if !s.Flags.Matches(core.SpellFlagAPL) {
				return
			}
			old := s.ExtraCastCondition
			s.ExtraCastCondition = func(sim *core.Simulation, t *core.Unit) bool { return !a.IsActive() && (old == nil || old(sim, t)) }
		})
		immunity := m.ForeverControlImmunityAura("Forever Ice Block immunity", m.ForeverAction("mage.talent.ice-block"), []core.ForeverControlKind{core.ForeverRoot, core.ForeverFear, core.ForeverSilence, core.ForeverStun, core.ForeverCharm, core.ForeverSleep, core.ForeverIncapacitate}, 10*time.Second)
		m.AddDynamicDamageTakenModifier(func(sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if a.IsActive() {
				r.Damage = 0
			}
		})
		m.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: m.ForeverAction("mage.talent.ice-block"), SpellSchool: core.SpellSchoolFrost, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, ManaCost: core.ManaCostOptions{FlatCost: 15}, Cast: core.CastConfig{CD: core.Cooldown{Timer: m.NewTimer(), Duration: 5 * time.Minute}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			m.ForeverInterrupt(sim)
			a.Activate(sim)
			immunity.Activate(sim)
		}})
	}
	// Baseline Frost Nova / Cone of Cold make all root and chill talents playable.
	// Their known Classic highest-rank values are retained; prediction concerns the
	// otherwise undocumented Forever baseline, not fabricated spell identifiers.
	// Lower ranks follow the Forever client tooltips; the top rank keeps the values above.
	if nr, ok := foreverRankAt(m.Level, foreverFrostNovaRanks); ok {
		nova := m.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			return t.ForeverControlAura("Forever Frost Nova-"+m.Label, core.ActionID{SpellID: 10230}, core.ForeverRoot, 8*time.Second)
		})
		m.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: nr.id}, SpellSchool: core.SpellSchoolFrost, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: SpellFlagMage | SpellFlagChillSpell | core.SpellFlagAPL,
			ManaCost: core.ManaCostOptions{FlatCost: nr.mana}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: m.NewTimer(), Duration: 25*time.Second + time.Duration(val("improved-frost-nova", 0))*time.Millisecond}}, DamageMultiplier: 1, ThreatMultiplier: 1, BonusCoefficient: .135,
			ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
				return m.DistanceFromTarget <= 10*(1+val("arctic-reach", 0)/100)
			},
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
				for _, t := range sim.Encounter.TargetUnits {
					r := s.CalcAndDealDamage(sim, t, sim.Roll(nr.low, nr.high), s.OutcomeMagicHitAndCrit)
					if r.Landed() {
						nova.Get(t).Activate(sim)
						if nova.Get(t).IsActive() {
							a := f.frozen.Get(t)
							a.Activate(sim)
							a.UpdateExpires(sim, sim.CurrentTime+8*time.Second)
						}
					}
				}
			},
		})
	}
	if cr, ok := foreverRankAt(m.Level, foreverConeOfColdRanks); ok {
		m.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: cr.id}, SpellSchool: core.SpellSchoolFrost, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: SpellFlagMage | SpellFlagChillSpell | core.SpellFlagAPL,
			ManaCost: core.ManaCostOptions{FlatCost: cr.mana}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: m.NewTimer(), Duration: 10 * time.Second}}, DamageMultiplier: 1 + val("improved-cone-of-cold", 0)/100, ThreatMultiplier: 1, BonusCoefficient: .135,
			ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
				return m.DistanceFromTarget <= 10*(1+val("arctic-reach", 0)/100)
			},
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
				for _, t := range sim.Encounter.TargetUnits {
					s.CalcAndDealDamage(sim, t, sim.Roll(cr.low, cr.high), s.OutcomeMagicHitAndCrit)
				}
			},
		})
	}
	// Counterspell's Classic interruption now operates on encounter spellcasts.
	silences := m.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.ForeverControlAura("Forever Improved Counterspell-"+m.Label, m.ForeverAction("mage.talent.improved-counterspell"), core.ForeverSilence, time.Duration(val("improved-counterspell", 0))*time.Millisecond)
	})
	if m.Counterspell != nil {
		oldCounter := m.Counterspell.ApplyEffects
		m.Counterspell.ForeverSingleTargetHarmful = true
		m.Counterspell.ApplyEffects = func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			oldCounter(sim, t, s)
			t.ForeverInterruptSchool(sim, 10*time.Second)
			if val("improved-counterspell", 0) > 0 {
				silences.Get(t).Activate(sim)
			}
		}
	}
	for _, ward := range []struct {
		ranks  []foreverMageRank
		name   string
		school core.SpellSchool
		talent string
	}{
		{foreverFireWardRanks, "Fire Ward", core.SpellSchoolFire, "improved-fire-ward"}, {foreverFrostWardRanks, "Frost Ward", core.SpellSchoolFrost, "frost-warding"},
	} {
		ward := ward
		wr, ok := foreverRankAt(m.Level, ward.ranks)
		if !ok {
			continue
		}
		action := core.ActionID{SpellID: wr.id}
		// The absorb amount is the client rank's effect 0 at the character's level.
		absorb, _ := foreverClientRange(m.GetCharacter(), wr.id, 0, wr.low, wr.low)
		var shield *core.ForeverAbsorb
		reflect := m.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: action.WithTag(1), SpellSchool: ward.school, Flags: core.SpellFlagPassiveSpell, DamageMultiplier: 1, ThreatMultiplier: 1})
		m.AddDynamicDamageTakenModifier(func(sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			chance := val(ward.talent, 0)
			if ward.talent == "frost-warding" {
				chance = val(ward.talent, 1)
			}
			if shield.Aura.IsActive() && s.SpellSchool.Matches(ward.school) && sim.Proc(chance/100, ward.name) {
				damage := r.Damage
				r.Damage = 0
				reflect.CalcAndDealDamage(sim, s.Unit, damage, reflect.OutcomeAlwaysHit)
			}
		})
		shield = core.NewForeverAbsorb(&m.Unit, "Forever "+ward.name, action, 30*time.Second, ward.school)
		m.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: action, SpellSchool: ward.school, Flags: SpellFlagMage | core.SpellFlagAPL | core.SpellFlagHelpful, ManaCost: core.ManaCostOptions{FlatCost: wr.mana}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: m.NewTimer(), Duration: 30 * time.Second}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { shield.Apply(sim, absorb, false) }})
	}
	// Mana Shield converts absorbed Physical damage into mana spending.
	if msr, ok := foreverRankAt(m.Level, foreverManaShieldRanks); ok {
		shieldAmount, _ := foreverClientRange(m.GetCharacter(), msr.id, 0, msr.low, msr.low)
		// "Drains $e mana per damage absorbed": the absorb effect's amplitude.
		manaPerDamage := 2.0
		if e := m.ClientSpell(msr.id).Effect(0); e != nil && e.Amplitude > 0 {
			manaPerDamage = e.Amplitude
		}
		manaShield := m.RegisterAura(core.Aura{Label: "Forever Mana Shield", ActionID: core.ActionID{SpellID: msr.id}, Duration: time.Minute})
		remaining := 0.0
		manaMetrics := m.NewManaMetrics(core.ActionID{SpellID: msr.id})
		m.AddDynamicDamageTakenModifier(func(sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if !manaShield.IsActive() || !s.SpellSchool.Matches(core.SpellSchoolPhysical) {
				return
			}
			ratio := manaPerDamage * (1 + val("arcane-shielding", 0)/100)
			absorb := min(r.Damage, min(remaining, m.CurrentMana()/ratio))
			r.Damage -= absorb
			remaining -= absorb
			m.SpendMana(sim, absorb*ratio, manaMetrics)
			if remaining <= 0 || m.CurrentMana() <= 0 {
				manaShield.Deactivate(sim)
			}
		})
		m.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: core.ActionID{SpellID: msr.id}, SpellSchool: core.SpellSchoolArcane, Flags: SpellFlagMage | core.SpellFlagAPL | core.SpellFlagHelpful, ManaCost: core.ManaCostOptions{FlatCost: msr.mana}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			remaining = shieldAmount
			manaShield.Activate(sim)
		}})
	}
	// A kill is detected only with health-backed targets; fixed-duration bosses do
	// not fabricate kills. The next Fire Blast consumes its own 20-second buff.
	wake := m.RegisterAura(core.Aura{Label: "Forever Wake of Fire", ActionID: m.ForeverAction("mage.talent.wake-of-fire"), Duration: 20 * time.Second, OnGain: func(a *core.Aura, sim *core.Simulation) {
		for _, s := range m.FireBlast {
			if s != nil {
				s.BonusCritRating += val("wake-of-fire", 1) * core.SpellCritRatingPerCritChance
			}
		}
	}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
		for _, s := range m.FireBlast {
			if s != nil {
				s.BonusCritRating -= val("wake-of-fire", 1) * core.SpellCritRatingPerCritChance
			}
		}
	}, OnCastComplete: func(a *core.Aura, sim *core.Simulation, s *core.Spell) {
		if s.SpellCode == SpellCode_MageFireBlast && a.RemainingDuration(sim) != a.Duration {
			a.Deactivate(sim)
		}
	}})
	killTrigger := func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if m.ForeverRank("mage.talent.wake-of-fire") > 0 && r.Target.HasHealthBar() && r.Target.CurrentHealth() <= 0 && r.Damage > 0 && r.Target.Level >= m.Level-8 {
			wake.Activate(sim)
		}
	}
	core.MakePermanent(m.RegisterAura(core.Aura{Label: "Forever Wake of Fire trigger", OnSpellHitDealt: killTrigger, OnPeriodicDamageDealt: killTrigger}))
}

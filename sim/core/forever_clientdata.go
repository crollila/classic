package core

import (
	"math"
	"strconv"
	"time"

	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/gamedata"
)

// Client-data layer: every spell a Forever unit registers takes its cost, cooldown and cast time
// from the Forever client build it simulates, keyed by the spell's client id. Class code no longer
// has to carry those numbers for Forever.
//
// Class code was written against Classic, and some of it adjusts a value for a talent before
// registering (Classic cost minus Improved X). So a registered value is compared with the Classic
// Era client's value for the same spell id:
//   - equal to Classic: the Forever value replaces it;
//   - different from Classic: it is a Classic value adjusted in code, and the Forever-minus-
//     Classic difference is applied on top, keeping the adjustment;
//   - a spell Classic does not have (Forever-only): the Forever value replaces the registered one
//     only when nothing adjusted it; a disagreement is reported, not overwritten.
// Every decision is recorded in the unit's override diagnostics with scope "client_data".

const clientDataScope = "client_data"

func (st *foreverOverrideState) clientEvent(unit *Unit, id int32, field string, registered, classic, forever float64, applied bool, result float64, reason string) {
	ev := foreverdata.OverrideEvent{Scope: clientDataScope, ID: strconv.Itoa(int(id)), Field: field, Unit: unit.Label,
		Registered: foreverdata.Ptr(registered), Forever: foreverdata.Ptr(forever), Status: "client_data"}
	if !math.IsNaN(classic) {
		ev.Classic = foreverdata.Ptr(classic)
	}
	if applied {
		ev.Applied = true
		ev.Result = foreverdata.Ptr(result)
	} else {
		ev.Reason = reason
	}
	st.diag.Record(ev)
}

// clientValue decides one field. hasClassic is false for Forever-only spells.
func (st *foreverOverrideState) clientValue(unit *Unit, id int32, field string, registered, classic, forever float64, hasClassic bool) float64 {
	const tol = 1e-6
	switch {
	case math.Abs(registered-forever) <= tol:
		return registered
	case !hasClassic:
		st.clientEvent(unit, id, field, registered, math.NaN(), forever, false, 0,
			"Forever-only spell: the registered value disagrees with the client and may carry a talent adjustment; left for class code")
		return registered
	case math.Abs(registered-classic) <= tol:
		st.clientEvent(unit, id, field, registered, classic, forever, true, forever, "")
		return forever
	default:
		result := math.Max(0, registered+forever-classic)
		st.clientEvent(unit, id, field, registered, classic, forever, true, result, "")
		return result
	}
}

func costOf(spell *gamedata.Spell, power string) (flat, pct float64, ok bool) {
	c := spell.Cost(power)
	if c == nil {
		return 0, 0, false
	}
	return c.Cost, c.CostPct, true
}

func cooldownOf(spell *gamedata.Spell) float64 {
	if spell.CooldownMs > 0 {
		return float64(spell.CooldownMs)
	}
	return float64(spell.CategoryCooldownMs)
}

// applyClientData patches a config with the unit's client build before the spell is built.
func (st *foreverOverrideState) applyClientData(unit *Unit, config *SpellConfig) {
	if st.gameData == nil || config.ActionID.SpellID == 0 || config.ActionID.Tag != 0 {
		return
	}
	id := config.ActionID.SpellID
	forever := st.gameData.Spell(id)
	if forever == nil {
		return
	}
	classic := st.gameData.ClassicSpell(id)
	hasClassic := classic != nil

	// Costs. The client stores rage in tenths; mana as a flat amount or a percent of base mana.
	if config.RageCost.Cost > 0 {
		if f, _, ok := costOf(forever, "rage"); ok {
			c, _, _ := costOf(classic, "rage")
			config.RageCost.Cost = st.clientValue(unit, id, "cost.rage", config.RageCost.Cost, c/10, f/10, hasClassic && c > 0)
		}
	}
	if config.EnergyCost.Cost > 0 {
		if f, _, ok := costOf(forever, "energy"); ok {
			c, _, _ := costOf(classic, "energy")
			config.EnergyCost.Cost = st.clientValue(unit, id, "cost.energy", config.EnergyCost.Cost, c, f, hasClassic && c > 0)
		}
	}
	if config.ManaCost.FlatCost > 0 {
		if f, _, ok := costOf(forever, "mana"); ok && f > 0 {
			c, _, _ := costOf(classic, "mana")
			config.ManaCost.FlatCost = st.clientValue(unit, id, "cost.mana", config.ManaCost.FlatCost, c, f, hasClassic && c > 0)
		}
	} else if config.ManaCost.BaseCost > 0 {
		if _, f, ok := costOf(forever, "mana"); ok && f > 0 {
			_, c, _ := costOf(classic, "mana")
			config.ManaCost.BaseCost = st.clientValue(unit, id, "cost.mana_pct", config.ManaCost.BaseCost*100, c, f, hasClassic && c > 0) / 100
		}
	}

	// Cooldown, when the class registered one.
	if config.Cast.CD.Timer != nil && config.Cast.CD.Duration > 0 {
		if f := cooldownOf(forever); f > 0 {
			c := 0.0
			if hasClassic {
				c = cooldownOf(classic)
			}
			ms := st.clientValue(unit, id, "cooldown_ms", msOf(config.Cast.CD.Duration), c, f, hasClassic && c > 0)
			config.Cast.CD.Duration = time.Duration(ms * float64(time.Millisecond))
		}
	}

	// Cast time, for spells with one.
	if config.Cast.DefaultCast.CastTime > 0 && forever.CastMs > 0 {
		c := 0.0
		if hasClassic {
			c = float64(classic.CastMs)
		}
		ms := st.clientValue(unit, id, "cast_ms", msOf(config.Cast.DefaultCast.CastTime), c, float64(forever.CastMs), hasClassic && c > 0)
		config.Cast.DefaultCast.CastTime = time.Duration(ms * float64(time.Millisecond))
	}
}

// clientEffectKind names a client effect the way the override document does.
func clientEffectKind(e *gamedata.Effect) string {
	switch {
	case e.Effect == 2:
		return foreverdata.KindSchoolDamage
	case e.Effect == 10:
		return foreverdata.KindHeal
	case e.Effect == 6 && e.Aura == 3:
		return foreverdata.KindPeriodicDamage
	case e.Effect == 6 && e.Aura == 8:
		return foreverdata.KindPeriodicHeal
	case e.Effect == 6:
		return foreverdata.KindApplyAura
	case e.Effect == 17 || e.Effect == 31 || e.Effect == 58 || e.Effect == 121:
		return foreverdata.KindWeaponDamage
	}
	return foreverdata.KindOther
}

func clientRange(e *gamedata.Effect) (float64, float64) {
	lo, hi := e.Range()
	return lo, hi
}

// clientSpellOverride derives a spell's Forever-vs-Classic effect changes straight from the
// client build and the Classic Era reference, in the override document's shape, so the existing
// registration-time machinery applies them. Costs, cooldowns and cast times are handled by
// applyClientData and are left out here. ok is false when there is nothing to compare.
func clientSpellOverride(data *gamedata.Snapshot, id int32) (*foreverdata.SpellOverride, bool) {
	forever, classic := data.Spell(id), data.ClassicSpell(id)
	if forever == nil || classic == nil {
		return nil, false
	}
	out := &foreverdata.SpellOverride{Effects: map[string]foreverdata.EffectOverride{},
		Provenance: foreverdata.Provenance{Status: "client_data",
			Sources: []string{"Forever client build " + data.Build + " compared with Classic Era " + data.ClassicBuild}}}
	pair := func(c, f float64) *foreverdata.ValuePair {
		return &foreverdata.ValuePair{Classic: foreverdata.Ptr(c), Forever: foreverdata.Ptr(f)}
	}
	for _, fe := range forever.Effects {
		ce := classic.Effect(fe.Index)
		if ce == nil || ce.Effect != fe.Effect || ce.Aura != fe.Aura {
			continue
		}
		override := foreverdata.EffectOverride{Kind: clientEffectKind(fe)}
		flo, fhi := clientRange(fe)
		clo, chi := clientRange(ce)
		if flo != clo || fhi != chi {
			override.Base = pair(clo, flo)
			if fhi != flo || chi != clo {
				override.Max = pair(chi, fhi)
			}
		}
		if fe.Coefficient() != ce.Coefficient() && fe.SPCoefficient != nil && ce.SPCoefficient != nil {
			override.Coefficient = pair(ce.Coefficient(), fe.Coefficient())
		}
		if override.Base != nil || override.Coefficient != nil {
			out.Effects[strconv.Itoa(fe.Index)] = override
		}
	}
	return out, len(out.Effects) > 0
}

// clientStatuses are override-document statuses that only restate client data; the unit's own
// client build supersedes them. Any other status (log observations, official statements) is
// evidence beyond the client and wins.
var clientStatuses = map[string]bool{"supported_by_data": true, "client_data": true, "": true}

// spellOverride is the override a Forever unit applies to a spell. The unit's client build is the
// baseline; a published document entry wins only when it carries non-client evidence.
func (st *foreverOverrideState) spellOverride(id int32) (*foreverdata.SpellOverride, bool) {
	published, ok := st.overrides.Spell(id)
	if ok && !clientStatuses[published.Provenance.Status] {
		return published, true
	}
	if st.gameData != nil && st.gameData.Spell(id) != nil && st.gameData.ClassicSpell(id) != nil {
		return clientSpellOverride(st.gameData, id)
	}
	return published, ok
}

package core

// Application of foreverdata overrides (overrides.json) for Forever-mode players.
//
// Every entry point is guarded by Unit.foreverOverrides, which is only set for
// characters constructed with Player.Forever != nil (and their pets). Classic
// units never reach any code in this file, so Classic results are unchanged.
// Each evaluated override is recorded as an applied or rejected
// foreverdata.OverrideEvent in the owning character's diagnostics.

import (
	"fmt"
	"github.com/wowsims/classic/sim/core/gamedata"
	"math"
	"strconv"
	"time"

	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"github.com/wowsims/classic/sim/core/stats"
)

const (
	foreverCoefficientTolerance = 1e-3
	foreverValueTolerance       = 1e-3
	foreverMsTolerance          = 0.5
)

type foreverOverrideState struct {
	gameData  *gamedata.Snapshot
	overrides *foreverdata.Overrides
	diag      *foreverdata.Diagnostics
	strict    bool
}

func (character *Character) initForeverOverrides(player *proto.Player) {
	o := foreverdata.ActiveOverrides()
	st := &foreverOverrideState{gameData: character.GameData, overrides: o, diag: foreverdata.NewDiagnostics(), strict: foreverdata.IsStrict(player.Forever)}
	character.Unit.foreverOverrides = st

	// Talent overrides are applied inside foreverdata.Lookup; record the ones this build uses.
	for id, n := range player.Forever.Talents {
		if n <= 0 {
			continue
		}
		if t, ok := o.Talent(id); ok {
			ev := foreverdata.OverrideEvent{Scope: foreverdata.ScopeTalent, ID: id, Field: "values", Unit: player.Name,
				Detail: "rank " + strconv.Itoa(int(n)), Beliefs: t.Provenance.Beliefs, Status: t.Provenance.Status, Applied: true}
			if int(n) <= len(t.Values) && len(t.Values[n-1]) > 0 {
				ev.Forever = foreverdata.Ptr(t.Values[n-1][0])
			}
			st.diag.Record(ev)
		}
	}

	// Combat hooks (Classic defaults when the parameter is absent).
	if v, ok := character.foreverParameterValue(foreverdata.ParamGlancingDamageMultiplier); ok && v > 0 {
		character.Unit.foreverGlanceMultiplier = v
	}
	if v, ok := character.foreverParameterValue(foreverdata.ParamMeleeCritDamageMultiplier); ok && v != 2.0 {
		character.Unit.foreverMeleeCritBase = v
	}
	if v, ok := character.foreverParameterValue(foreverdata.ParamSpellCritDamageMultiplier); ok && v != 1.5 {
		character.Unit.foreverSpellCritBase = v
	}
	character.initForeverAttackTableParams()
}

// ClassicDualWieldMissPenalty is the miss chance added to dual wielding players' attacks.
const ClassicDualWieldMissPenalty = 0.19

// foreverAttackTableParams holds the core.combat.* attack table parameters of a Forever
// character. active is false for Classic units and when every value is the Classic default.
type foreverAttackTableParams struct {
	active               bool
	dualWieldMissPenalty float64
	meleeMiss            float64
	dodge                float64
	parry                float64
	glanceChance         float64
	meleeCritSuppression float64
	spellMiss            float64
}

func (character *Character) initForeverAttackTableParams() {
	params := foreverAttackTableParams{dualWieldMissPenalty: ClassicDualWieldMissPenalty}
	for _, entry := range []struct {
		key    string
		target *float64
	}{
		{foreverdata.ParamDualWieldMissPenalty, &params.dualWieldMissPenalty},
		{foreverdata.ParamMeleeMissOffset, &params.meleeMiss},
		{foreverdata.ParamDodgeOffset, &params.dodge},
		{foreverdata.ParamParryOffset, &params.parry},
		{foreverdata.ParamGlancingChanceOffset, &params.glanceChance},
		{foreverdata.ParamMeleeCritSuppressionOffset, &params.meleeCritSuppression},
		{foreverdata.ParamSpellMissOffset, &params.spellMiss},
	} {
		// Only keys registered in the catalog pass request/override validation.
		if v, ok := character.foreverParameterValue(entry.key); ok && v != *entry.target && foreverdata.ParameterInBounds(entry.key, v) {
			*entry.target = v
			params.active = true
		}
	}
	character.Unit.foreverAttackTable = params
}

// apply adjusts a player-vs-enemy attack table once, when it is built. Chances are
// clamped to >= 0; the 1% spell miss floor is kept by Spell.SpellChanceToMiss.
func (params foreverAttackTableParams) apply(table *AttackTable) {
	table.DualWieldMissPenalty = params.dualWieldMissPenalty
	table.BaseMissChance = max(0, table.BaseMissChance+params.meleeMiss)
	table.BaseDodgeChance = max(0, table.BaseDodgeChance+params.dodge)
	table.BaseParryChance = max(0, table.BaseParryChance+params.parry)
	table.BaseGlanceChance = max(0, table.BaseGlanceChance+params.glanceChance)
	table.MeleeCritSuppression = max(0, table.MeleeCritSuppression+params.meleeCritSuppression)
	table.BaseSpellMissChance = max(0, table.BaseSpellMissChance+params.spellMiss)
}

// foreverParameterValue returns the request value or the override default, if either exists.
func (character *Character) foreverParameterValue(key string) (float64, bool) {
	if character.Forever == nil {
		return 0, false
	}
	if v, ok := character.Forever.Parameters[key]; ok {
		return v, true
	}
	return character.foreverParameterDefault(key)
}

func (character *Character) foreverParameterDefault(key string) (float64, bool) {
	st := character.Unit.foreverOverrides
	if st == nil || !st.overrides.HasParameters() {
		return 0, false
	}
	p, ok := st.overrides.Parameter(key)
	if !ok {
		return 0, false
	}
	if st.diag.Seen(foreverdata.ScopeParameter, key, "value", character.Name, "") {
		return *p.Value, !(st.strict && p.Provenance.Predicted())
	}
	ev := foreverdata.OverrideEvent{Scope: foreverdata.ScopeParameter, ID: key, Field: "value", Unit: character.Name,
		Forever: foreverdata.Ptr(*p.Value), Beliefs: p.Provenance.Beliefs, Status: p.Provenance.Status}
	if st.strict && p.Provenance.Predicted() {
		ev.Reason = "PREDICTED override refused in STRICT mode"
		st.diag.Record(ev)
		return 0, false
	}
	ev.Applied = true
	ev.Result = foreverdata.Ptr(*p.Value)
	st.diag.Record(ev)
	return *p.Value, true
}

// ForeverOverrideReport returns the applied/rejected overrides for this character
// (including its pets). Empty for Classic characters.
func (character *Character) ForeverOverrideReport() foreverdata.DiagnosticsReport {
	if character.Unit.foreverOverrides == nil {
		return foreverdata.DiagnosticsReport{Applied: []foreverdata.OverrideEvent{}, Rejected: []foreverdata.OverrideEvent{}}
	}
	return character.Unit.foreverOverrides.diag.Report()
}

// ForeverPlayerOverrideReport is the per-player section of a run report.
type ForeverPlayerOverrideReport struct {
	Name      string `json:"name"`
	RaidIndex int32  `json:"raid_index"`
	foreverdata.DiagnosticsReport
}

// ForeverOverrideRunReport describes the override document and what each Forever player used.
type ForeverOverrideRunReport struct {
	Overrides foreverdata.OverridesSummary  `json:"overrides"`
	Players   []ForeverPlayerOverrideReport `json:"players"`
}

// ForeverOverrideReport collects the report for every Forever player in the environment.
func (env *Environment) ForeverOverrideReport() *ForeverOverrideRunReport {
	report := &ForeverOverrideRunReport{Overrides: foreverdata.OverridesInfo(), Players: []ForeverPlayerOverrideReport{}}
	if env == nil || env.Raid == nil {
		return report
	}
	for _, party := range env.Raid.Parties {
		for _, agent := range party.Players {
			c := agent.GetCharacter()
			if c.Unit.foreverOverrides == nil {
				continue
			}
			report.Overrides = c.Unit.foreverOverrides.overrides.Summary()
			report.Players = append(report.Players, ForeverPlayerOverrideReport{Name: c.Name, RaidIndex: c.Index, DiagnosticsReport: c.ForeverOverrideReport()})
		}
	}
	return report
}

// RunRaidSimWithForeverOverrideReport runs exactly like RunRaidSim and also returns
// the override diagnostics of the main (non-presim) simulation environment.
// The report is nil only if the environment could not be constructed.
func RunRaidSimWithForeverOverrideReport(request *proto.RaidSimRequest) (*proto.RaidSimResult, *ForeverOverrideRunReport) {
	var env *Environment
	result := runSimObserved(request, nil, false, simsignals.CreateSignals(), func(sim *Simulation) { env = sim.Environment })
	if env == nil {
		return result, nil
	}
	return result, env.ForeverOverrideReport()
}

func (st *foreverOverrideState) refuse(ev *foreverdata.OverrideEvent, prov foreverdata.Provenance) bool {
	ev.Beliefs = prov.Beliefs
	ev.Status = prov.Status
	if st.strict && prov.Predicted() {
		ev.Reason = "PREDICTED override refused in STRICT mode"
		st.diag.Record(*ev)
		return true
	}
	return false
}

// ForeverSpellValue scales a Classic constant that hand-written sim code grants for
// a spell effect (consumables, buffs) by forever/classic from spells.<id>.effects.<i>.base
// (averaged with max when present). Classic units return classicValue unchanged.
func (unit *Unit) ForeverSpellValue(spellID int32, effectIndex int, classicValue float64) float64 {
	st := unit.foreverOverrides
	if st == nil {
		return classicValue
	}
	spell, ok := st.spellOverride(spellID)
	if !ok {
		return classicValue
	}
	effect, ok := spell.Effect(effectIndex)
	if !ok || (effect.Base == nil && effect.Max == nil) {
		return classicValue
	}
	ev := foreverdata.OverrideEvent{Scope: foreverdata.ScopeSpellValue, ID: strconv.Itoa(int(spellID)),
		Field: fmt.Sprintf("effects.%d.base", effectIndex), Unit: unit.Label, Detail: strconv.FormatFloat(classicValue, 'g', -1, 64),
		Registered: foreverdata.Ptr(classicValue)}
	if st.refuse(&ev, spell.Provenance) {
		return classicValue
	}
	reject := func(reason string) float64 {
		ev.Reason = reason
		st.diag.Record(ev)
		return classicValue
	}
	if effect.Kind == foreverdata.KindWeaponDamage {
		return reject("weapon_damage effects are not applied to hand-written values")
	}
	classic, forever, err := foreverEffectAverage(effect)
	if err != "" {
		return reject(err)
	}
	ev.Classic, ev.Forever = foreverdata.Ptr(classic), foreverdata.Ptr(forever)
	result := classicValue
	switch {
	case forever == classic:
	case classic == 0 && classicValue == 0:
		result = forever
	case classic == 0:
		return reject("override classic value is 0 but the simulator constant is not")
	default:
		result = classicValue * forever / classic
	}
	ev.Applied = true
	ev.Result = foreverdata.Ptr(result)
	st.diag.Record(ev)
	return result
}

// foreverEffectAverage returns avg(classic) and avg(forever) of base (and max when present).
func foreverEffectAverage(effect foreverdata.EffectOverride) (float64, float64, string) {
	if effect.Base == nil || effect.Base.Classic == nil {
		return 0, 0, "base.classic is null or base is missing"
	}
	classic, forever := *effect.Base.Classic, *effect.Base.Forever
	if effect.Max != nil {
		if effect.Max.Classic == nil {
			return 0, 0, "max.classic is null"
		}
		classic = (classic + *effect.Max.Classic) / 2
		forever = (forever + *effect.Max.Forever) / 2
	}
	return classic, forever, ""
}

func foreverClose(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func msOf(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// foreverSpellPatch carries per-spell values that must be set after the Spell is constructed.
type foreverSpellPatch struct {
	impactBaseRatio   float64
	periodicBaseRatio float64
}

func isImpactKind(kind string) bool {
	return kind == foreverdata.KindSchoolDamage || kind == foreverdata.KindHeal
}
func isPeriodicKind(kind string) bool {
	return kind == foreverdata.KindPeriodicDamage || kind == foreverdata.KindPeriodicHeal
}

// patchSpellConfig applies spell overrides to the config before RegisterSpell uses it.
func (st *foreverOverrideState) patchSpellConfig(unit *Unit, config *SpellConfig) *foreverSpellPatch {
	if config.ActionID.SpellID == 0 {
		return nil
	}
	spell, ok := st.spellOverride(config.ActionID.SpellID)
	if !ok {
		return nil
	}
	id := strconv.Itoa(int(config.ActionID.SpellID))
	detail := ""
	if config.ActionID.Tag != 0 {
		detail = "tag " + strconv.Itoa(int(config.ActionID.Tag))
	}
	newEvent := func(field string, pair *foreverdata.ValuePair) foreverdata.OverrideEvent {
		ev := foreverdata.OverrideEvent{Scope: foreverdata.ScopeSpell, ID: id, Field: field, Unit: unit.Label, Detail: detail,
			Beliefs: spell.Provenance.Beliefs, Status: spell.Provenance.Status}
		if pair != nil {
			ev.Classic, ev.Forever = pair.Classic, pair.Forever
		}
		return ev
	}
	if st.strict && spell.Provenance.Predicted() {
		ev := newEvent("*", nil)
		ev.Reason = "PREDICTED override refused in STRICT mode"
		st.diag.Record(ev)
		return nil
	}
	record := func(ev foreverdata.OverrideEvent, registered float64, result *float64, reason string) {
		ev.Registered = foreverdata.Ptr(registered)
		if reason == "" {
			ev.Applied = true
			ev.Result = result
		} else {
			ev.Reason = reason
		}
		st.diag.Record(ev)
	}
	physical := config.DefenseType == DefenseTypeMelee || config.DefenseType == DefenseTypeRanged
	patch := &foreverSpellPatch{}

	// Ambiguity is judged per field: two impact effects that both carry a coefficient
	// (or both carry base values) cannot be mapped onto the single Spell slot.
	impactCoef, periodicCoef, impactBase, periodicBase := 0, 0, 0, 0
	for _, i := range spell.EffectIndices() {
		e, _ := spell.Effect(i)
		hasBase := e.Base != nil || e.Max != nil
		if isImpactKind(e.Kind) {
			impactCoef += b2i(e.Coefficient != nil)
			impactBase += b2i(hasBase)
		} else if isPeriodicKind(e.Kind) {
			periodicCoef += b2i(e.Coefficient != nil)
			periodicBase += b2i(hasBase)
		}
	}

	for _, i := range spell.EffectIndices() {
		e, _ := spell.Effect(i)
		prefix := fmt.Sprintf("effects.%d.", i)

		if e.Coefficient != nil {
			ev := newEvent(prefix+"coefficient", e.Coefficient)
			var target *float64
			reason := ""
			switch {
			case e.Kind == foreverdata.KindSchoolDamage || e.Kind == foreverdata.KindHeal:
				target = &config.BonusCoefficient
				if impactCoef > 1 {
					reason = "more than one impact effect in override; ambiguous"
				} else if e.Kind == foreverdata.KindSchoolDamage && config.DefenseType != DefenseTypeMagic {
					reason = "school_damage coefficient requires DefenseType magic"
				} else if e.Kind == foreverdata.KindHeal && physical {
					reason = "heal coefficient on a melee/ranged spell"
				}
			case e.Kind == foreverdata.KindPeriodicDamage:
				target = &config.Dot.BonusCoefficient
				if periodicCoef > 1 {
					reason = "more than one periodic effect in override; ambiguous"
				} else if config.Dot.NumberOfTicks == 0 && config.Dot.TickLength == 0 {
					reason = "spell registers no Dot"
				} else if config.DefenseType != DefenseTypeMagic {
					reason = "periodic_damage coefficient requires DefenseType magic"
				}
			case e.Kind == foreverdata.KindPeriodicHeal:
				target = &config.Hot.BonusCoefficient
				if periodicCoef > 1 {
					reason = "more than one periodic effect in override; ambiguous"
				} else if config.Hot.NumberOfTicks == 0 && config.Hot.TickLength == 0 {
					reason = "spell registers no Hot"
				}
			default:
				reason = "coefficient is not auto-applicable for kind " + e.Kind
			}
			registered := 0.0
			if target != nil {
				registered = *target
			}
			if reason == "" && e.Coefficient.Classic == nil {
				reason = "coefficient.classic is null"
			}
			if reason == "" && !foreverClose(registered, *e.Coefficient.Classic, foreverCoefficientTolerance) {
				reason = fmt.Sprintf("registered coefficient %g does not match classic %g", registered, *e.Coefficient.Classic)
			}
			if reason == "" {
				*target = *e.Coefficient.Forever
			}
			record(ev, registered, e.Coefficient.Forever, reason)
		}

		if e.Base != nil || e.Max != nil {
			pair := e.Base
			if pair == nil {
				pair = e.Max
			}
			ev := newEvent(prefix+"base", pair)
			reason := ""
			var ratio float64
			switch {
			case !isImpactKind(e.Kind) && !isPeriodicKind(e.Kind):
				if foreverdata.HasSpellValueHook(config.ActionID.SpellID, i) {
					// Consumed by hand-written code through Unit.ForeverSpellValue; not a registration-time change.
					break
				}
				reason = "base is not auto-applicable at spell registration for kind " + e.Kind
			case physical:
				reason = "base damage on a melee/ranged spell cannot be separated from weapon/attack power damage"
			case isImpactKind(e.Kind) && impactBase > 1, isPeriodicKind(e.Kind) && periodicBase > 1:
				reason = "more than one effect of this category in override; ambiguous"
			case e.Kind == foreverdata.KindPeriodicDamage && config.Dot.NumberOfTicks == 0 && config.Dot.TickLength == 0:
				reason = "spell registers no Dot"
			case e.Kind == foreverdata.KindPeriodicHeal && config.Hot.NumberOfTicks == 0 && config.Hot.TickLength == 0:
				reason = "spell registers no Hot"
			default:
				classic, forever, err := foreverEffectAverage(e)
				if err != "" {
					reason = err
				} else if classic == 0 {
					reason = "classic base average is 0"
				} else {
					ratio = forever / classic
				}
			}
			if !isImpactKind(e.Kind) && !isPeriodicKind(e.Kind) && reason == "" {
				continue
			}
			if reason == "" {
				if isImpactKind(e.Kind) {
					patch.impactBaseRatio = ratio
				} else {
					patch.periodicBaseRatio = ratio
				}
				ev.Detail = detailJoin(detail, "ratio "+strconv.FormatFloat(ratio, 'g', 6, 64))
			}
			ev.Applied = reason == ""
			ev.Reason = reason
			if ev.Applied {
				ev.Result = foreverdata.Ptr(ratio)
			}
			st.diag.Record(ev)
		}
	}

	if p := spell.CastMs; p != nil {
		ev := newEvent("cast_ms", p)
		registered := msOf(config.Cast.DefaultCast.CastTime)
		reason := ""
		var result float64
		switch {
		case p.Classic == nil:
			reason = "cast_ms.classic is null"
		case config.Cast.CastTime != nil:
			reason = "spell uses a custom cast time function"
		case !foreverClose(registered, *p.Classic, foreverMsTolerance):
			reason = fmt.Sprintf("registered cast time %gms does not match classic %gms", registered, *p.Classic)
		default:
			result = registered + *p.Forever - *p.Classic
			var empty Cast
			if result < 0 {
				reason = "resulting cast time is negative"
			} else if config.Cast.DefaultCast == empty && result > 0 {
				reason = "spell has no default cast (auto/proc spell)"
			} else {
				config.Cast.DefaultCast.CastTime = time.Duration(math.Round(result * float64(time.Millisecond)))
			}
		}
		record(ev, registered, &result, reason)
	}

	if p := spell.CooldownMs; p != nil {
		ev := newEvent("cooldown_ms", p)
		registered := msOf(config.Cast.CD.Duration)
		reason := ""
		var result float64
		switch {
		case p.Classic == nil:
			reason = "cooldown_ms.classic is null"
		case config.Cast.CD.Timer == nil:
			reason = "spell has no own cooldown timer"
		case !foreverClose(registered, *p.Classic, foreverMsTolerance):
			reason = fmt.Sprintf("registered cooldown %gms does not match classic %gms", registered, *p.Classic)
		default:
			result = registered + *p.Forever - *p.Classic
			if result <= 0 {
				reason = "resulting cooldown is not positive"
			} else {
				config.Cast.CD.Duration = time.Duration(math.Round(result * float64(time.Millisecond)))
			}
		}
		record(ev, registered, &result, reason)
	}

	if c := spell.Cost; c != nil {
		ev := newEvent("cost", &foreverdata.ValuePair{Classic: c.Classic, Forever: c.Forever})
		ev.Detail = detailJoin(detail, c.Power)
		var target *float64
		reason := ""
		switch c.Power {
		case "mana":
			if config.ManaCost.BaseCost != 0 {
				reason = "mana cost is a percentage of base mana"
			} else if config.ManaCost.FlatCost == 0 {
				reason = "spell has no flat mana cost"
			} else {
				target = &config.ManaCost.FlatCost
			}
		case "rage":
			if config.RageCost.Cost == 0 || config.ManaCost.BaseCost != 0 || config.ManaCost.FlatCost != 0 {
				reason = "spell has no rage cost"
			} else {
				target = &config.RageCost.Cost
			}
		case "energy":
			if config.EnergyCost.Cost == 0 || config.ManaCost.BaseCost != 0 || config.ManaCost.FlatCost != 0 {
				reason = "spell has no energy cost"
			} else {
				target = &config.EnergyCost.Cost
			}
		}
		registered, result := 0.0, 0.0
		if target != nil {
			registered = *target
		}
		if reason == "" && c.Classic == nil {
			reason = "cost.classic is null"
		}
		if reason == "" && !foreverClose(registered, *c.Classic, foreverValueTolerance) {
			reason = fmt.Sprintf("registered cost %g does not match classic %g", registered, *c.Classic)
		}
		if reason == "" {
			result = registered + *c.Forever - *c.Classic
			if result <= 0 {
				reason = "resulting cost is not positive"
			} else {
				*target = result
			}
		}
		record(ev, registered, &result, reason)
	}

	if p := spell.DurationMs; p != nil {
		dot := &config.Dot
		if dot.NumberOfTicks == 0 && dot.TickLength == 0 {
			dot = &config.Hot
		}
		if dot.NumberOfTicks != 0 || dot.TickLength != 0 {
			ev := newEvent("duration_ms", p)
			registered := msOf(dot.TickLength) * float64(dot.NumberOfTicks)
			reason := ""
			var result float64
			switch {
			case p.Classic == nil:
				reason = "duration_ms.classic is null"
			case dot.TickLength <= 0:
				reason = "dot has no tick length"
			case !foreverClose(registered, *p.Classic, foreverMsTolerance):
				reason = fmt.Sprintf("registered dot duration %gms does not match classic %gms", registered, *p.Classic)
			default:
				tick := msOf(dot.TickLength)
				ticks := *p.Forever / tick
				if *p.Forever <= 0 || math.Abs(ticks-math.Round(ticks)) > 1e-9 {
					reason = fmt.Sprintf("forever duration %gms is not a positive multiple of the %gms tick", *p.Forever, tick)
				} else {
					dot.NumberOfTicks = int32(math.Round(ticks))
					result = *p.Forever
				}
			}
			record(ev, registered, &result, reason)
		}
		// Spells without a Dot/Hot: durations are handled when auras with this SpellID register.
	}

	if patch.impactBaseRatio == 0 && patch.periodicBaseRatio == 0 {
		return nil
	}
	return patch
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func detailJoin(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

// patchAuraDuration applies spells.<id>.duration_ms to an aura registered on a Forever unit.
func (st *foreverOverrideState) patchAuraDuration(unit *Unit, aura *Aura) {
	if aura.ActionID.SpellID == 0 || aura.Duration <= 0 {
		return
	}
	spell, ok := st.spellOverride(aura.ActionID.SpellID)
	if !ok || spell.DurationMs == nil {
		return
	}
	p := spell.DurationMs
	ev := foreverdata.OverrideEvent{Scope: foreverdata.ScopeSpell, ID: strconv.Itoa(int(aura.ActionID.SpellID)), Field: "duration_ms",
		Unit: unit.Label, Detail: "aura " + aura.Label, Classic: p.Classic, Forever: p.Forever}
	if st.refuse(&ev, spell.Provenance) {
		return
	}
	registered := msOf(aura.Duration)
	ev.Registered = foreverdata.Ptr(registered)
	switch {
	case p.Classic == nil:
		ev.Reason = "duration_ms.classic is null"
	case !foreverClose(registered, *p.Classic, foreverMsTolerance):
		ev.Reason = fmt.Sprintf("registered aura duration %gms does not match classic %gms", registered, *p.Classic)
	case *p.Forever <= 0:
		ev.Reason = "forever duration is not positive"
	default:
		aura.Duration = time.Duration(math.Round(*p.Forever * float64(time.Millisecond)))
		ev.Applied = true
		ev.Result = foreverdata.Ptr(*p.Forever)
	}
	st.diag.Record(ev)
}

// classicEquipmentOrEmpty keeps the Classic construction path byte-for-byte; Forever
// players build their equipment after the override state exists.
func classicEquipmentOrEmpty(player *proto.Player) Equipment {
	if player.Forever != nil {
		return Equipment{}
	}
	return ProtoToEquipment(player.Equipment)
}

// foreverEquipment builds equipment for a Forever player: Classic items first,
// then new_item definitions for ids absent from the Classic database, then
// item stat/weapon deltas.
func (character *Character) foreverEquipment(es *proto.EquipmentSpec) Equipment {
	st := character.Unit.foreverOverrides
	equipment := Equipment{}
	for _, spec := range ProtoToEquipmentSpec(es) {
		if spec.ID == 0 {
			continue
		}
		equipment.EquipItem(character.foreverNewItem(spec))
	}
	for slot := range equipment {
		item := &equipment[slot]
		if item.ID == 0 {
			continue
		}
		override, ok := st.overrides.Item(item.ID)
		if !ok {
			continue
		}
		id := strconv.Itoa(int(item.ID))
		slotName := proto.ItemSlot(slot).String()
		base := foreverdata.OverrideEvent{Scope: foreverdata.ScopeItem, ID: id, Unit: character.Name, Detail: slotName}
		if st.refuse(&base, override.Provenance) {
			continue
		}
		apply := func(field string, pair foreverdata.ValuePair, current *float64, scale float64, tol float64, positive bool) {
			ev := base
			ev.Field = field
			ev.Classic, ev.Forever = pair.Classic, pair.Forever
			registered := *current * scale
			ev.Registered = foreverdata.Ptr(registered)
			switch {
			case pair.Classic == nil:
				ev.Reason = field + ".classic is null"
			case !foreverClose(registered, *pair.Classic, tol):
				ev.Reason = fmt.Sprintf("item value %g does not match classic %g", registered, *pair.Classic)
			case positive && registered+*pair.Forever-*pair.Classic <= 0:
				ev.Reason = "resulting value is not positive"
			default:
				result := registered + *pair.Forever - *pair.Classic
				*current = result / scale
				ev.Applied = true
				ev.Result = foreverdata.Ptr(result)
			}
			st.diag.Record(ev)
		}
		for _, key := range foreverdata.StatKeys {
			pair, ok := override.Stats[key]
			if !ok {
				continue
			}
			s, _ := foreverdata.StatKeyIndex(key)
			apply("stats."+key, pair, &item.Stats[stats.Stat(s)], 1, foreverValueTolerance, false)
		}
		if w := override.Weapon; w != nil {
			if w.Min != nil {
				apply("weapon.min", *w.Min, &item.WeaponDamageMin, 1, foreverValueTolerance, false)
			}
			if w.Max != nil {
				apply("weapon.max", *w.Max, &item.WeaponDamageMax, 1, foreverValueTolerance, false)
			}
			if w.SpeedMs != nil {
				apply("weapon.speed_ms", *w.SpeedMs, &item.SwingSpeed, 1000, foreverMsTolerance, true)
			}
		}
	}
	return equipment
}

// foreverNewItem resolves an item for a Forever player. Classic database items always win.
func (character *Character) foreverNewItem(spec ItemSpec) Item {
	if _, ok := ItemsByID[spec.ID]; ok {
		return NewItem(spec)
	}
	st := character.Unit.foreverOverrides
	override, ok := st.overrides.Item(spec.ID)
	if !ok || override.NewItemProto() == nil {
		return NewItem(spec) // panics with the Classic message
	}
	ev := foreverdata.OverrideEvent{Scope: foreverdata.ScopeNewItem, ID: strconv.Itoa(int(spec.ID)), Field: "new_item", Unit: character.Name}
	if st.refuse(&ev, override.Provenance) {
		panic(fmt.Sprintf("No item with id: %d (Forever new_item refused in STRICT mode: PREDICTED)", spec.ID))
	}
	ui := override.NewItemProto()
	item := ItemFromProto(&proto.SimItem{
		Id: ui.Id, ClassAllowlist: ui.ClassAllowlist, Name: ui.Name, Type: ui.Type, ArmorType: ui.ArmorType,
		WeaponType: ui.WeaponType, HandType: ui.HandType, RangedWeaponType: ui.RangedWeaponType, Stats: ui.Stats,
		BonusPhysicalDamage: ui.BonusPhysicalDamage, WeaponDamageMin: ui.WeaponDamageMin, WeaponDamageMax: ui.WeaponDamageMax,
		WeaponSpeed: ui.WeaponSpeed, SetName: ui.SetName, SetId: ui.SetId, WeaponSkills: ui.WeaponSkills,
	})
	item.Quality = ui.Quality
	if spec.RandomSuffix != 0 {
		if randomSuffix, ok := RandomSuffixesByID[spec.RandomSuffix]; ok {
			item.RandomSuffix = randomSuffix
		} else {
			panic(fmt.Sprintf("No random suffix with id: %d", spec.RandomSuffix))
		}
	}
	if spec.Enchant != 0 {
		if enchant, ok := EnchantsByEffectID[spec.Enchant]; ok {
			item.Enchant = enchant
		}
	}
	ev.Applied = true
	ev.Detail = item.Name
	st.diag.Record(ev)
	return item
}

// Classic stat conversion values registered by the class packages. The Forever
// hook adds (forever - classic) as an extra static dependency when a
// core.conversion.* parameter is supplied by the request or the override defaults.
func classicConversion(class proto.Class, conversion string) (float64, bool) {
	switch conversion {
	case "agility_per_melee_crit":
		if v := CritPerAgiAtLevel[class]; v > 0 {
			return 1 / v, true
		}
	case "intellect_per_spell_crit":
		if v := CritPerIntAtLevel[class]; v > 0 {
			return 1 / v, true
		}
	case "attack_power_per_strength":
		v, ok := APPerStrength[class]
		return v, ok
	case "attack_power_per_agility":
		if class == proto.Class_ClassHunter || class == proto.Class_ClassRogue {
			return 1, true
		}
		if _, ok := APPerStrength[class]; ok {
			return 0, true
		}
	}
	return 0, false
}

func (character *Character) applyForeverStatConversions() {
	if character.Unit.foreverOverrides == nil {
		return
	}
	class := foreverClassKey(character.Class)
	for _, conversion := range []string{"agility_per_melee_crit", "intellect_per_spell_crit", "attack_power_per_strength", "attack_power_per_agility"} {
		key := foreverdata.ConversionParam(class, conversion)
		value, ok := character.foreverParameterValue(key)
		if !ok {
			continue
		}
		classic, known := classicConversion(character.Class, conversion)
		// Only keys registered in the catalog pass request/override validation.
		if !known || value == classic || !foreverdata.ParameterInBounds(key, value) {
			continue
		}
		switch conversion {
		case "agility_per_melee_crit":
			character.AddStatDependency(stats.Agility, stats.MeleeCrit, (1/value-1/classic)*CritRatingPerCritChance)
		case "intellect_per_spell_crit":
			character.AddStatDependency(stats.Intellect, stats.SpellCrit, (1/value-1/classic)*SpellCritRatingPerCritChance)
		case "attack_power_per_strength":
			character.AddStatDependency(stats.Strength, stats.AttackPower, value-classic)
		case "attack_power_per_agility":
			character.AddStatDependency(stats.Agility, stats.AttackPower, value-classic)
		}
	}
}

func foreverClassKey(class proto.Class) string {
	switch class {
	case proto.Class_ClassWarrior:
		return "warrior"
	case proto.Class_ClassPaladin:
		return "paladin"
	case proto.Class_ClassHunter:
		return "hunter"
	case proto.Class_ClassRogue:
		return "rogue"
	case proto.Class_ClassPriest:
		return "priest"
	case proto.Class_ClassShaman:
		return "shaman"
	case proto.Class_ClassMage:
		return "mage"
	case proto.Class_ClassWarlock:
		return "warlock"
	case proto.Class_ClassDruid:
		return "druid"
	}
	return "unknown"
}

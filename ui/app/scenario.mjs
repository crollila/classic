export function defaultScenario() {
  return { durationVariation: 10, execute20: 20, execute35: 35, armor: -1, resistance: 0,
    targets: 1, mobType: 6, reactionMs: 100, distance: -1, inFront: false, tanking: false,
    targetSwingMs: 2000, targetMinDamage: 0, targetMaxDamage: 0, startingRage: 0, queueDelay: 250,
    sunder: 'preset', raidBuffs: {}, partyBuffs: {}, buffs: {}, debuffs: {}, consumes: {miscConsumes: {}, petMiscConsumes: {}},
    profession1: 0, profession2: 0, mechanicRanks: /** @type {Record<string, number>} */ ({}), parameters: /** @type {Record<string, number>} */ ({}), seed: 20260921 };
}

const SUNDER = [7386, 7405, 8380, 11596, 11597];
const SHOUT = [6673, 5242, 6192, 11549, 11550, 11551, 25289];
// Preserve unrelated actions and conditions. External maintenance never costs this player's GCD/rage.
export function scenarioRotation(apl, scenario, isWarrior) {
  const result = structuredClone(apl);
  if (!isWarrior) return result;
  const excluded = new Set([
    ...(scenario.sunder !== 'preset' || scenario.debuffs?.exposeArmor ? SUNDER : []),
    ...(scenario.raidBuffs?.battleShout ? SHOUT : []),
  ]);
  const clean = action => {
    if (!action || excluded.has(action.castSpell?.spellId?.spellId)) return null;
    for (const key of ['sequence', 'strictSequence']) {
      if (action[key]) {
        action[key].actions = (action[key].actions || []).map(clean).filter(Boolean);
        if (!action[key].actions.length) return null;
      }
    }
    return action;
  };
  for (const section of ['prepullActions', 'priorityList']) {
    result[section] = (result[section] || []).filter(row => clean(row.action));
  }
  return result;
}

export function scenarioDebuffs(scenario) {
  return { ...scenario.debuffs, sunderArmor: scenario.sunder === 'external' && !scenario.debuffs?.exposeArmor };
}

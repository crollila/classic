// Zero is the UI's Off state, never an engine mechanic rank. Rebuild requests
// from the current manifest so old localStorage cannot retain obsolete ranks.
export function scenarioMechanics(scenario, mechanics) {
  const ranks = {}, parameters = {}, corrections = [];
  for (const [id, saved] of Object.entries(scenario.mechanicRanks || {})) {
    const m = mechanics.find(m => m.id === id);
    if (!saved) continue;
    if (!m || ['blocked', 'non-sim', 'baseline'].includes(m.mode) || !Number.isFinite(saved)) {
      corrections.push(`${m?.name || id}: unavailable selection omitted`); continue;
    }
    const rank = Math.max(1, Math.min(m.max_rank, Math.floor(saved)));
    if (rank !== saved) corrections.push(`${m.name}: rank adjusted to ${rank}`);
    ranks[id] = rank;
    for (const p of m.adapter?.parameters || []) {
      const savedValue = scenario.parameters?.[p.key];
      if (savedValue === undefined) continue;
      const value = Number.isFinite(savedValue) ? Math.max(p.min, Math.min(p.max, savedValue)) : p.default;
      if (value !== savedValue) corrections.push(`${p.label}: adjusted to ${value}`);
      parameters[p.key] = value;
    }
  }
  return {ranks, parameters, corrections};
}

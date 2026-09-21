// Omit only actions the engine explicitly identifies as unavailable to this character.
// Other warnings must reach the optimizer's validation gate.
export function learnedRotation(apl, stats) {
  const result = structuredClone(apl);
  for (const section of ['prepullActions', 'priorityList']) {
    result[section] = (result[section] || []).filter((_, index) => {
      const warnings = stats?.[section]?.[index]?.warnings || [];
      return !warnings.length || !warnings.every(w => /does not know spell|No aura found .+ for:/.test(w));
    });
  }
  return result;
}

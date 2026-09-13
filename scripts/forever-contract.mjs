export const CONFIDENCE = ['CONFIRMED', 'HIGH CONFIDENCE', 'EXPERIMENTAL', 'UNKNOWN'];
export const CATEGORIES = ['race', 'talents', 'equipment', 'enchants', 'buffs', 'debuffs', 'consumes', 'encounter', 'rotation'];

export function validateRelease(data) {
  if (!data || data.schemaVersion !== 1 || !['upstream-baseline', 'forever-ruleset'].includes(data.engineMode)) throw new Error('Invalid Forever release metadata');
  for (const field of ['foreverBuild', 'rulesetId', 'mechanicsUpdatedAt']) {
    if (data[field] !== null && (typeof data[field] !== 'string' || !data[field].trim())) throw new Error(`Invalid ${field}`);
  }
  if (data.mechanicsUpdatedAt !== null && !Number.isFinite(Date.parse(data.mechanicsUpdatedAt))) throw new Error('Invalid mechanics update date');
  if (data.engineMode === 'forever-ruleset' && (!data.foreverBuild || !data.rulesetId || !data.mechanicsUpdatedAt)) throw new Error('A Forever ruleset requires a build, ruleset ID, and mechanics update date');
  if (!Array.isArray(data.mechanics)) throw new Error('Mechanics must be an array');
  const ids = new Set();
  for (const mechanic of data.mechanics) {
    if (!mechanic.id || ids.has(mechanic.id) || !mechanic.label || !mechanic.note || !CATEGORIES.includes(mechanic.category) || !CONFIDENCE.includes(mechanic.confidence)) throw new Error('Invalid or duplicate mechanic');
    ids.add(mechanic.id);
    if (mechanic.specs && (!Array.isArray(mechanic.specs) || mechanic.specs.some(s => !Number.isInteger(s)))) throw new Error('Mechanic specs must be shared protobuf spec IDs');
    if (['CONFIRMED', 'HIGH CONFIDENCE'].includes(mechanic.confidence) && !mechanic.evidence?.length) throw new Error('Confidence claims require evidence');
    if (mechanic.evidence && (!Array.isArray(mechanic.evidence) || mechanic.evidence.some(url => typeof url !== 'string' || !/^https:\/\//.test(url)))) throw new Error('Evidence must use HTTPS URLs');
  }
  return data;
}

export function rewriteLocalPaths(source) {
  return source.replaceAll('/classic/assets/', '/forever-sim/assets/');
}

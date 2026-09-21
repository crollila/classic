// Display labels only: these do not grant an ability or change its mechanics.
// Spell effects whose labels are absent from the talent catalogue, verified in
// sim/core/racials.go, sim/warrior/items.go and sim/common/item_effects.go.
const EFFECT_NAMES = { 20572: 'Blood Fury', 24427: 'Diamond Flask', 29602: 'Jom Gabbar' };

export function createActionNames(spellNames = {}, database = {}) {
  const spells = new Map(Object.entries({ ...EFFECT_NAMES, ...spellNames }).map(([id, name]) => [Number(id), name]));
  const items = new Map();
  for (const entry of database.spellIcons || []) if (entry.name) spells.set(entry.id, entry.name);
  for (const entry of [...(database.itemIcons || []), ...(database.items || [])]) if (entry.name) items.set(entry.id, entry.name);
  return raw => raw?.oneofKind === 'spellId'
    ? spells.get(raw.spellId) || 'Unidentified ability (see technical details)'
    : raw?.oneofKind === 'itemId'
      ? items.get(raw.itemId) || 'Unidentified item (see technical details)'
      : undefined;
}

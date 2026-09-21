// Slot 17 is a UI-only two-hand view. The engine always receives 17 slots:
// a two-hander occupies main hand (14), never a third weapon slot.
export function normalizeWeapons(gear, itemFor) {
  const next = Array.from({length:17}, (_, i) => gear[i] || 0);
  if (itemFor(next[14])?.handType === 4) next[15] = 0;
  return next;
}
export function weaponViewId(gear, view, itemFor, separate) {
  const twoHand = itemFor(gear[14])?.handType === 4;
  if (view === 17) return twoHand ? gear[14] : 0;
  if (separate && view === 14 && twoHand) return 0;
  return gear[view] || 0;
}
export function replaceWeapon(gear, view, id, itemFor) {
  const next = normalizeWeapons(gear, itemFor);
  if (view === 17) {
    if (id && itemFor(id)?.handType !== 4) throw Error('Two-hand view requires a two-handed weapon');
    if (id || itemFor(next[14])?.handType === 4) next[14] = id;
    if (id) next[15] = 0;
  } else {
    next[view] = id;
    if (view === 15 && id && itemFor(next[14])?.handType === 4) next[14] = 0;
  }
  return normalizeWeapons(next, itemFor);
}

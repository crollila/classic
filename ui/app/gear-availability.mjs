// Client presence, catalog ancestry and obtainability are separate claims.
export function classifyGear(item, snapshot, notes, legacyIds) {
  const note = notes[String(item.id)] || {};
  const listed = String(note.carried?.sources || '').startsWith('foreverchanges.pro') || /Forever loot table/.test(note.reason_present || '');
  return {
    evidence: listed ? 'listed' : snapshot.items?.[String(item.id)] ? 'client' : 'unknown',
    origin: legacyIds.has(item.id) ? 'legacy' : 'unclassified',
    legacyPhase: legacyIds.has(item.id) ? Number(item.phase || 0) : 0,
  };
}
export function gearAllowed(meta, filter) {
  if (!meta) return filter.evidence === 'all' && filter.unclassified;
  if (filter.evidence === 'listed' && meta.evidence !== 'listed') return false;
  if (filter.evidence === 'client' && !['listed', 'client'].includes(meta.evidence)) return false;
  if (!filter[meta.origin]) return false;
  // Legacy phase is a planning filter, never a Forever release schedule.
  if (meta.origin === 'legacy' && filter.maxPhase > 0 && (!meta.legacyPhase || meta.legacyPhase > filter.maxPhase)) return false;
  return true;
}
export const defaultGearFilter = () => ({ evidence: 'listed', legacy: true, unclassified: true, maxPhase: 0 });
export const evidenceLabel = meta => meta?.evidence === 'listed' ? 'Forever source-listed (not live-confirmed)' : meta?.evidence === 'client' ? 'Client record only · availability unknown' : 'No current Forever evidence';

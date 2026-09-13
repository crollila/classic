export const confidenceLevels = ['CONFIRMED', 'HIGH CONFIDENCE', 'EXPERIMENTAL', 'UNKNOWN'] as const;
export type Confidence = typeof confidenceLevels[number];
export type Mechanic = { id: string; label: string; category: string; confidence: Confidence; note: string; specs?: number[]; evidence?: string[] };
export type Release = {
  schemaVersion: number;
  simulatorVersion: string;
  foreverBuild: string | null;
  mechanicsUpdatedAt: string | null;
  rulesetId: string | null;
  engineMode: 'upstream-baseline' | 'forever-ruleset';
  notice?: string;
  mechanics: Mechanic[];
  databaseSha256?: string;
  engineSha256?: string;
  upstreamCommit?: string;
  builtAt?: string;
};

const unknown: Release = { schemaVersion: 1, simulatorVersion: 'UNKNOWN', foreverBuild: null, mechanicsUpdatedAt: null, rulesetId: null, engineMode: 'upstream-baseline', mechanics: [], notice: 'Release metadata is unavailable. Forever build and mechanics confidence are UNKNOWN. Results cannot be treated as verified Forever predictions.' };
let pending: Promise<Release> | undefined;
export function getRelease(): Promise<Release> {
  return pending ||= fetch(`${import.meta.env.BASE_URL}release.json`, { cache: 'no-cache' })
    .then(async response => {
      if (!response.ok) throw new Error('Release unavailable');
      const release = await response.json();
      validateRelease(release);
      if (release.schemaVersion !== 1 || !Array.isArray(release.mechanics) || !release.simulatorVersion ||
        !['upstream-baseline', 'forever-ruleset'].includes(release.engineMode) ||
        release.mechanics.some((m: Mechanic) => !confidenceLevels.includes(m.confidence) || !m.label || !m.note)) throw new Error('Invalid release');
      return release as Release;
    }).catch(() => unknown);
}

export function el<K extends keyof HTMLElementTagNameMap>(tag: K, className = '', text = ''): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag); node.className = className; node.textContent = text; return node;
}
export function badge(status: Confidence) {
  return el('span', `forever-confidence forever-${status.toLowerCase().replaceAll(' ', '-')}`, status);
}
export function releaseSummary(release: Release) {
  const wrapper = el('section', 'forever-release-summary');
  const grid = el('dl', 'forever-release-grid');
  for (const [label, value] of [
    ['Simulator version', release.simulatorVersion],
    ['Forever beta / build', release.foreverBuild || 'UNKNOWN · not supplied'],
    ['Mechanics data updated', release.mechanicsUpdatedAt || 'UNKNOWN · not supplied'],
    ['Active ruleset', release.rulesetId || 'WoWSims Classic baseline'],
  ]) {
    const pair = el('div'); pair.append(el('dt', '', label), el('dd', '', value)); grid.append(pair);
  }
  wrapper.append(grid);
  if (release.notice || release.engineMode === 'upstream-baseline') {
    const warning = el('p', 'forever-disclosure');
    warning.append(badge('UNKNOWN'), el('span', '', release.notice || 'Forever mechanics are not validated in this baseline build.'));
    wrapper.append(warning);
  }
  return wrapper;
}

export function confidencePanel(release: Release, spec?: number | null) {
  const details = el('details', 'forever-mechanics'); details.id = 'forever-mechanics';
  const summary = el('summary', '', 'Mechanics confidence & assumptions');
  const legend = el('div', 'forever-confidence-legend');
  const definitions: Record<Confidence, string> = { CONFIRMED: 'Supported by reviewed evidence for this build.', 'HIGH CONFIDENCE': 'Strong evidence; some validation remains.', EXPERIMENTAL: 'Provisional implementation or assumption.', UNKNOWN: 'No validated evidence supplied. Do not assume Classic behavior matches Forever.' };
  for (const status of confidenceLevels) { const entry = el('div'); entry.append(badge(status), el('span', '', definitions[status])); legend.append(entry); }
  details.append(summary, legend);
  const mechanics = release.mechanics.filter(m => !m.specs?.length || spec == null || m.specs.includes(spec));
  for (const mechanic of mechanics) {
    const row = el('article', 'forever-mechanic'); row.append(badge(mechanic.confidence), el('strong', '', `${mechanic.label} · ${mechanic.category}`), el('p', '', mechanic.note));
    for (const url of mechanic.evidence || []) {
      if (!url.startsWith('https://')) continue;
      const link = el('a', '', 'View evidence ↗'); link.href = url; link.target = '_blank'; link.rel = 'noopener noreferrer'; row.append(link);
    }
    details.append(row);
  }
  if (!mechanics.length) details.append(el('p', '', 'No reviewed Forever mechanics have been published for this configuration. Races, talents, equipment, enchants, buffs, debuffs, consumes, encounter behavior, and rotation assumptions are UNKNOWN.'));
  details.append(el('p', 'forever-footnote', 'Unlisted mechanics remain UNKNOWN. Classic presets and external Wowhead tooltips are references, not evidence of Forever behavior.'));
  return details;
}
import { validateRelease } from '../../scripts/forever-contract.mjs';

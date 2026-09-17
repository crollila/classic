export const confidenceLevels = ['CONFIRMED', 'PROVISIONAL', 'PREDICTED', 'UNKNOWN'] as const;
export type Confidence = typeof confidenceLevels[number];
export type Mechanic = { id: string; label: string; category: string; confidence: Confidence; note: string; specs?: number[]; evidence?: string[] };
export type Release = {
  schemaVersion: number;
  simulatorVersion: string;
  foreverBuild: string | null;
  mechanicsUpdatedAt: string | null;
  rulesetId: string | null;
  engineMode: 'upstream-baseline' | 'forever-ruleset' | 'forever-discovery';
  notice?: string;
  mechanics: Mechanic[];
  databaseSha256?: string;
  engineSha256?: string;
  upstreamCommit?: string;
  builtAt?: string;
  liveDataUrl?: string;
  embeddedOverridesSha256?: string | null;
};

const unknown: Release = { schemaVersion: 1, simulatorVersion: 'UNKNOWN', foreverBuild: null, mechanicsUpdatedAt: null, rulesetId: null, engineMode: 'upstream-baseline', mechanics: [], notice: 'Release metadata is unavailable. Forever build and mechanics confidence are UNKNOWN. Results cannot be treated as verified Forever predictions.' };
let pending: Promise<Release> | undefined;
export function getRelease(): Promise<Release> {
  return pending ||= fetch(`${import.meta.env.BASE_URL}release.json`, { cache: 'no-cache' })
    .then(async response => {
      if (!response.ok) throw new Error('Release unavailable');
      const release = await response.json();
      validateRelease(release);
      if (![1,2].includes(release.schemaVersion) || !Array.isArray(release.mechanics) || !release.simulatorVersion ||
        !['upstream-baseline', 'forever-ruleset','forever-discovery'].includes(release.engineMode) ||
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
    ['Forever beta / build', release.foreverBuild || 'Pre-beta discovery · no client build'],
    ['Mechanics data updated', release.mechanicsUpdatedAt || 'UNKNOWN · not supplied'],
    ['Active ruleset', release.rulesetId || 'WoWSims Classic baseline'],
  ]) {
    const pair = el('div'); pair.append(el('dt', '', label), el('dd', '', value)); grid.append(pair);
  }
  wrapper.append(grid, liveDataStatus(release));
  if (release.notice || release.engineMode === 'upstream-baseline') {
    const warning = el('p', 'forever-disclosure');
    warning.append(badge(release.engineMode==='forever-discovery'?'PROVISIONAL':'UNKNOWN'), el('span', '', release.notice || 'Forever mechanics are not validated in this baseline build.'));
    wrapper.append(warning);
  }
  return wrapper;
}

// Which overrides document the engine runs: the published live one, or the embedded fallback.
function liveDataStatus(release: Release) {
  const status = el('div', 'forever-live-data');
  const line = el('p', 'forever-live-data-line', 'Forever data: checking for live data…');
  const warning = el('p', 'forever-disclosure forever-live-data-warning');
  warning.hidden = true; warning.setAttribute('role', 'status');
  status.append(line, warning);
  watchLiveData(data => {
    const described = describeLiveData(data, release);
    line.textContent = `Forever data: ${described.text}`;
    status.dataset.liveData = described.live ? 'live' : 'embedded';
    const messages = [described.live ? '' : 'This page is running the data embedded in this build, which may be older than the Oracle.', described.warning || ''].filter(Boolean);
    warning.replaceChildren(...(messages.length ? [el('strong', '', 'Warning'), el('span', '', messages.join(' '))] : []));
    warning.hidden = !messages.length;
  });
  return status;
}

export function confidencePanel(release: Release, spec?: number | null) {
  const details = el('details', 'forever-mechanics'); details.id = 'forever-mechanics';
  const summary = el('summary', '', 'Mechanics confidence & assumptions');
  const legend = el('div', 'forever-confidence-legend');
  const definitions: Record<Confidence, string> = { CONFIRMED: 'Direct evidence; only the stated scope is confirmed.', PROVISIONAL: 'Known effect with Classic-equivalent interactions where unknown.', PREDICTED: 'Explicitly extrapolated value or analogous behavior. BEST_GUESS only.', UNKNOWN: 'Insufficient evidence for a specific mechanic.' };
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
import { describeLiveData } from './live-data.mjs';
import { watchLiveData } from './live-data-client';

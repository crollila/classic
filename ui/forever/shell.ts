import type { SimUI } from '../core/sim_ui';
import { Spec } from '../core/proto/common';
import { ForeverMode } from '../core/proto/api';
import { el, getRelease, releaseSummary, confidencePanel } from './metadata';
import { mountOptimizer } from './optimizer-ui';

export async function mountForever(simUI: SimUI, spec: Spec | null) {
  document.body.classList.add('forever-page');
  const root = simUI.rootElem;
  const header = el('header', 'forever-masthead forever-sim-masthead');
  const home = el('a', 'forever-wordmark', 'Forever Simulator'); home.href = import.meta.env.BASE_URL;
  home.append(el('span', '', 'Exalted Capital · World of Warcraft: Forever'));
  const navigation = el('nav'); navigation.setAttribute('aria-label', 'Simulator navigation');
  const allClasses = el('a', '', 'All classes'); allClasses.href = import.meta.env.BASE_URL;
  const website = el('a', '', 'Exalted Capital ↗'); website.href = 'https://www.exaltedcapital.com/';
  navigation.append(allClasses, website); header.append(home, navigation); root.prepend(header);
  simUI.simMain.id = 'forever-main';
  const skip = el('a', 'forever-skip', 'Skip to simulator'); skip.href = '#forever-main'; root.prepend(skip);

  const release = await getRelease();
  mountOptimizer(simUI, release);
  const status = el('div', 'forever-sim-status');
  status.append(releaseSummary(release), confidencePanel(release, spec));
  simUI.simContentContainer.insertBefore(status, simUI.simHeader.rootElem);

  const categories: Record<string, string> = { 'gear-tab': 'Equipment and enchants', 'talents-tab': 'Talents', 'settings-tab': 'Races, buffs, debuffs, consumes, and encounter mechanics', 'rotation-tab': 'Rotation and APL assumptions' };
  const annotate = () => {
    for (const [id, label] of Object.entries(categories)) {
      const panel = root.querySelector<HTMLElement>(`#${id}`);
      if (!panel || panel.querySelector('.forever-option-note')) continue;
      const note = el('p', 'forever-option-note');
      const notes:Record<string,string>={'gear-tab':'This catalogue is Forever: items are the Forever client’s, plus the ones the client only receives in game. Combat ratings have no confirmed conversion yet, so they are listed rather than applied.','talents-tab':'These are Forever trees. Each rank shows its evidence and confidence; STRICT suppresses predictions.','settings-tab':'Forever racial and profession options are available in Talents. Existing encounter and consumable rules provide the Classic fallback.','rotation-tab':'New Forever offensive actions are added when enabled in Talents. The APL editor exposes registered abilities for custom rotations.'};
      note.append(document.createTextNode(`${notes[id]} `));
      const link = el('a', '', 'Review Forever mechanics confidence'); link.href = '#forever-mechanics';
      link.addEventListener('click', () => { status.querySelector('details')!.open = true; });
      note.append(link); panel.prepend(note);
    }
  };
  annotate();
  simUI.sim.waitForInit().then(annotate);
  const stats = root.querySelector<HTMLElement>('.sim-sidebar-stats');
  if (stats) {
    const disclosure = el('details', 'forever-character-stats');
    disclosure.append(el('summary', '', 'Character stats'));
    stats.before(disclosure); disclosure.append(stats);
    const desktop = matchMedia('(min-width: 992px)');
    const syncStats = () => { disclosure.open = desktop.matches; };
    syncStats(); desktop.addEventListener('change', syncStats);
    simUI.addOnDisposeCallback(() => desktop.removeEventListener('change', syncStats));
  }
  // Results retain provenance even after the user changes the current configuration.
  const provenance = el('p', 'forever-result-provenance');
  provenance.hidden = true;
  root.querySelector('.sim-sidebar-results')?.append(provenance);
  simUI.sim.simResultEmitter.on((_eventID,result) => {
    provenance.hidden = false;
    const modes=[...new Set(result.request.raid?.parties.flatMap(p=>p.players.filter(p=>p.forever).map(p=>p.forever!.mode===ForeverMode.STRICT?'STRICT':'BEST_GUESS'))||[])];
    provenance.textContent = `Run ${modes.join(' / ')} · ${release.simulatorVersion} · ${release.rulesetId || 'Classic baseline'} · Mechanics ${release.mechanicsUpdatedAt || 'UNKNOWN'}. Pre-beta result; see rank evidence and interaction assumptions.`;
  });
  const footer = el('footer', 'forever-footer');
  footer.append(el('span', '', 'Exalted Capital · Forever Simulator'));
  const upstream = el('a', '', 'Built on WoWSims Classic · MIT'); upstream.href = 'https://github.com/wowsims/classic'; footer.append(upstream);
  root.append(footer);
}

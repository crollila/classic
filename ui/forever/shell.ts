import type { SimUI } from '../core/sim_ui';
import { Spec } from '../core/proto/common';
import { el, getRelease } from './metadata';
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
  const back = el('a', 'forever-app-link', '← Back to Forever Sim'); back.href = `${import.meta.env.BASE_URL}app/`;
  simUI.simContentContainer.insertBefore(back, simUI.simHeader.rootElem);
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
  const footer = el('footer', 'forever-footer');
  footer.append(el('span', '', 'Exalted Capital · Forever Simulator'));
  const upstream = el('a', '', 'Built on WoWSims Classic · MIT'); upstream.href = 'https://github.com/wowsims/classic'; footer.append(upstream);
  root.append(footer);
}

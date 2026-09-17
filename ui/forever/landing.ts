import { getLaunchedSimsForClass } from '../core/launched_sims';
import { classNames, classIcons, specNames, naturalClassOrder, getSpecSiteUrl } from '../core/proto_utils/utils';
import { el, getRelease, releaseSummary, confidencePanel } from './metadata';

const grid = document.querySelector<HTMLElement>('#forever-classes')!;
let count = 0;
for (const klass of naturalClassOrder) {
  const specs = getLaunchedSimsForClass(klass);
  if (!specs.length) continue;
  count += specs.length;
  const card = el('article', 'forever-class-card');
  const heading = el('div', 'forever-class-heading');
  const icon = el('img'); icon.src = classIcons[klass]; icon.alt = ''; icon.width = 40; icon.height = 40;
  heading.append(icon, el('h3', '', classNames[klass])); card.append(heading);
  for (const spec of specs) {
    const link = el('a', 'forever-spec-link', specNames[spec]); link.href = getSpecSiteUrl(spec);
    link.append(el('span', '', '→')); card.append(link);
  }
  grid.append(card);
}
document.querySelector('#forever-count')!.textContent = `${count} supported simulators`;
getRelease().then(release => document.querySelector('#forever-release')!.append(releaseSummary(release), confidencePanel(release)));

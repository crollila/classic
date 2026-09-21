import { getLaunchedSimsForClass } from '../core/launched_sims';
import { classNames, classIcons, specNames, naturalClassOrder, getSpecSiteUrl } from '../core/proto_utils/utils';
import { SPECS } from '../app/specs';
import { el } from './metadata';

// Every DPS spec opens the Forever Sim; the full WoWSims page stays one click away as "Advanced".
const grid = document.querySelector<HTMLElement>('#forever-classes')!;
let count = 0;
for (const klass of naturalClassOrder) {
  const specs = getLaunchedSimsForClass(klass);
  if (!specs.length) continue;
  const card = el('article', 'forever-class-card');
  const heading = el('div', 'forever-class-heading');
  const icon = el('img'); icon.src = classIcons[klass]; icon.alt = ''; icon.width = 40; icon.height = 40;
  heading.append(icon, el('h3', '', classNames[klass])); card.append(heading);
  for (const spec of specs) {
    const advanced = getSpecSiteUrl(spec);
    const app = SPECS.find(s => advanced.replace(/\/+$/, '').endsWith('/' + s.key));
    if (!app) continue;
    count++;
    const link = el('a', 'forever-spec-link', specNames[spec]); link.href = `./app/#${app.key}`;
    link.append(el('span', '', '→'));
    const extra = el('a', 'forever-advanced-link', 'Advanced'); extra.href = advanced;
    card.append(link, extra);
  }
  if (card.children.length > 1) grid.append(card);
}
document.querySelector('#forever-count')!.textContent = `${count} DPS specs`;

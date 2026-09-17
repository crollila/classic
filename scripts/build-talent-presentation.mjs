// Presentation only. Never imports effects into the simulator.
import fs from 'node:fs';
import path from 'node:path';
const snapshot = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const data = JSON.parse(fs.readFileSync('ui/forever/data/talents.json', 'utf8'));
const db = JSON.parse(fs.readFileSync('assets/database/db.json', 'utf8'));
const tooltips = new Map(fs.readFileSync('assets/db_inputs/wowhead_spell_tooltips.csv', 'utf8').trim().split(/\r?\n/).map(line => {
 const comma = line.indexOf(','); return [Number(line.slice(0,comma)), JSON.parse(line.slice(comma+1))];
}));
const result = {source:'WoWSims Classic assets/database and talent layouts; captured forevertalents.up.railway.app presentation metadata (2026-09-13). Blizzard artwork via Wowhead. Icons are presentation, not authenticated Forever spell identities.', trees:{}, talents:{}};
for (const [name, cls] of Object.entries(snapshot)) {
 const classic = JSON.parse(fs.readFileSync(path.join('ui/core/talents/trees',name.toLowerCase()+'.json'),'utf8'));
 result.trees[name.toUpperCase()] = cls.trees.map(tree => ({name:tree.name,icon:tree.icon,background:classic.find(t=>t.name===tree.name)?.backgroundUrl || `https://wow.zamimg.com/images/wow/talents/backgrounds/classic/${tree.bg}.jpg`}));
 for (const tree of cls.trees) for (const talent of tree.talents) {
  const record=data.records.find(r=>r.class===name.toUpperCase()&&r.tree===tree.name&&r.name===talent.name);
  if(!record)throw Error('Unmatched presentation talent: '+talent.name);
  const id=record.classic_spell_ids[0], icon=db.spellIcons.find(s=>s.id===id)?.icon || tooltips.get(id)?.icon || talent.icon;
  result.talents[record.id]={icon,classic:talent.classic||null};
 }
}
if(Object.keys(result.talents).length!==data.records.length)throw Error('Incomplete presentation');
fs.writeFileSync('ui/forever/data/talent-presentation.json',JSON.stringify(result,null,2)+'\n');

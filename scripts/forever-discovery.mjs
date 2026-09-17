// Deterministic import of the complete research manifest, never a network scraper.
// Run from classic/: node scripts/forever-discovery.mjs [--check]
import { readFileSync, writeFileSync, mkdirSync, readdirSync } from 'node:fs';
import { createHash } from 'node:crypto';
import assert from 'node:assert/strict';
const raw = readFileSync('../forever_changes.json');
const manifest = JSON.parse(raw);
const sha = createHash('sha256').update(raw).digest('hex');
assert.equal(sha,'c5a8742a1f33415ab45df28f541e83bd51e7cb90c4e58b35f7e58809c9c5d11f','Manifest changed: review the evidence and bump the ruleset before regenerating.');
const classes = ['WARRIOR','PALADIN','HUNTER','ROGUE','PRIEST','SHAMAN','MAGE','WARLOCK','DRUID'];
const support = {};
function add(cls, mode, names) { for (const n of names.split(' ')) support[`${cls}.talent.${n}`] = mode; }
// Individually reviewed bridges to existing engine effects. This is an allowlist,
// not a heuristic based on the source's sometimes inaccurate "unchanged" badge.
add('warrior','classic','improved-heroic-strike deflection improved-overpower deep-wounds sweeping-strikes cruelty toughness improved-sunder-armor');
add('paladin','classic','divine-strength divine-intellect toughness deflection conviction improved-judgement');
add('hunter','classic','frenzy ranged-weapon-specialization survivalist');
add('rogue','classic','ruthlessness improved-slice-and-dice relentless-strikes lightning-reflexes seal-fate blade-flurry adrenaline-rush');
add('priest','classic','meditation holy-specialization spell-warding divine-fury improved-shadow-word-pain improved-mind-blast darkness');
add('shaman','classic','elemental-warding elemental-devastation elemental-focus purification');
add('mage','classic','arcane-concentration presence-of-mind arcane-power ignite improved-flamestrike master-of-elements critical-mass fire-power improved-frostbolt ice-shards piercing-ice frost-channeling cold-snap');
add('warlock','classic','improved-life-tap master-summoner fel-domination');
add('druid','classic','improved-moonfire vengeance natural-shapeshifter');
// Replacements leave the old Classic field at zero, preventing double application.
add('warrior','replace','anticipation precision boundless-rage vitality bastion master-of-defense');
add('paladin','replace','reverence divine-precision champion-of-the-light crusade precision anticipation one-handed-weapon-specialization two-handed-weapon-specialization');
add('hunter','replace','careful-aim lightning-reflexes deflection lethal-attacks endurance-training ferocity unleashed-fury bestial-discipline focused-fire lone-wolf');
add('rogue','replace','malice precision deflection');
add('priest','replace','holy-precision mental-strength spiritual-guidance shadow-focus shadow-affinity improved-mind-flay');
add('shaman','replace','mental-dexterity mental-quickness mindfulness thundering-strikes ancestral-knowledge anticipation toughness natural-grace rage-of-the-farseer');
add('mage','replace','arcane-focus elemental-precision arcane-resilience arcane-impact arcane-meditation arcane-mind');
add('warlock','replace','suppression demonic-embrace shadow-mastery molten-skin malevolence');
add('druid','replace','nature-s-majesty reflection living-spirit moonfury');
add('priest','replace','meditation');
add('warlock','replace','unholy-power fel-vitality demonic-knowledge');
// Class-owned implementations retain the Classic field only where its type/ranks match.
add('warrior','class','bloodthirst improved-rend improved-cleave improved-execute improved-revenge improved-thunder-clap flurry shield-specialization improved-bloodrage death-wish last-stand unbridled-wrath');
add('rogue','class','dual-wield-specialization improved-eviscerate improved-sinister-strike murder vigor');
add('warlock','class','demonic-sacrifice');
add('mage','class','combustion');
add('shaman','class','flurry');
add('hunter','replace','strider-kick');
add('hunter','class','efficiency');
add('warrior','class','dual-wield-specialization enrage');
add('druid','class','improved-wrath');
add('druid','replace','moonglow');
add('shaman','classic','improved-lightning-shield convection reverberation');
add('rogue','classic','lethality vile-poisons');
add('warrior','classic','two-handed-weapon-specialization impale');
const extras = {
 'racials.human.the-human-spirit': 'retain',
 'racials.troll.beast-slaying': 'retain',
 'racials.tauren.endurance': 'racial',
 'racials.gnome.expansive-mind': 'racial',
 'racials.human.sword-specialization': 'racial',
 'racials.orc.axe-specialization': 'racial',
 'racials.dwarf.mace-specialization': 'racial',
 'racials.dwarf.big-game-hunter': 'racial',
 'racials.troll.berserking': 'racial',
 'buffs.first-aid-kit': 'preapplied_buff',
};
const all = Object.values(manifest.sections).flat();
assert.equal(all.length,701);
const adapters = {};
for (const file of readdirSync('scripts').filter(f=>/^forever-.+-adapters\.json$/.test(f)).sort()) {
 for (const [id, adapter] of Object.entries(JSON.parse(readFileSync(`scripts/${file}`)))) {
	if(adapter.mode==='non_sim_relevant')adapter.mode='non-sim';
  assert(all.some(r=>r.id===id),`unknown adapter record ${id}`);
  assert(!adapters[id],`duplicate adapter ${id}`);
  adapters[id]=adapter;
  if(adapter.mode) support[id]=adapter.mode;
 }
}
const records = [];
const layout = {};
for (const cls of classes) {
 const old = JSON.parse(readFileSync(`ui/core/talents/trees/${cls.toLowerCase()}.json`));
 layout[cls] = old.map(t=>t.talents.map(x=>x.fieldName));
 for (const r of manifest.sections[cls].filter(r=>r.kind==='talent')) {
  const adapter=adapters[r.id];
  const mode=support[r.id]||'blocked';
  const oldField=r.current_classic_behavior.field_name||'';
  if (mode==='classic'||mode==='class') assert(oldField, r.id);
  const ranks=r.forever_behavior.tooltip_values_by_rank.map(t=>{
   const v=r.forever_behavior.implementation_rank_values.find(x=>x.rank===t.rank);
   const estimated=!v?.effect;
   const override=adapter?.rank_overrides?.find(o=>o.rank===t.rank);
   const effect=override?.effect||v?.effect||t.tooltip;
   return {rank:t.rank, effect, estimated, confidence:estimated?'PREDICTED':v.confidence,
    confidence_value:estimated?0.5:(v.confidence==='CONFIRMED'?0.95:0.8),
    prediction_reason:estimated?(override?.reason||override?.basis||'Calculator estimated progression from observed ranks; enabled only in BEST_GUESS. Fixed durations and trigger counts retain source values.') : null,
    source_url:v?.source_url||r.source_url,
    values:override?.values||[...effect.matchAll(/(?<![\w])\d+(?:\.\d+)?/g)].map(x=>Number(x[0]))};
  });
  records.push({id:r.id, class:cls, tree:r.tree, name:r.talent_name, max_rank:r.ranks,
   row:r.position.row, column:r.position.column, required_points:r.position.points_in_tree_required,
   prerequisites:r.prerequisites.map(p=>({id:manifest.sections[cls].find(x=>x.kind==='talent'&&x.talent_name===p.talent_name)?.id,rank:p.required_ranks})),
   source_url:r.source_url, classification:r.classification, classic_field:oldField, classic_max_rank:r.current_classic_behavior.max_ranks||0,
   classic_spell_ids:(r.current_classic_behavior.ranks||[]).map(x=>x.classic_spell_id),
   mode, confidence:adapter?.confidence||'PROVISIONAL', adapter:adapter||null,
   activation_text:r.forever_behavior.activation_text||'',
   ranks, action_tag:all.findIndex(x=>x.id===r.id)+1});
 }
}
for(const id of Object.keys(support)) assert(all.some(r=>r.id===id),id);
for(const r of records) for(const p of r.prerequisites) assert(p.id&&p.rank,JSON.stringify(r));
const raceClasses=Object.fromEntries(manifest.sections.RACIALS.filter(r=>r.kind==='race_class_combinations').map(r=>[r.id.split('.')[1],r.forever_behavior.classes.map(c=>c.toUpperCase())]));
const mechanics=all.filter(r=>r.kind!=='talent').map(r=>{
 const adapter=adapters[r.id];
 return {id:r.id,category:Object.entries(manifest.sections).find(([,rs])=>rs.includes(r))[0],kind:r.kind,
  name:r.talent_name||r.name||r.id.split('.').at(-1).replaceAll('-',' '),
  effect:typeof r.forever_behavior==='string'?r.forever_behavior:r.forever_behavior?.effect||'',
  max_rank:r.forever_behavior?.reported_max_ranks||1,
  mode:adapter?.mode||extras[r.id]||'blocked',confidence:adapter?.confidence||(r.confidence==='UNVERIFIED'?'PREDICTED':'PROVISIONAL'),
  action_tag:all.findIndex(x=>x.id===r.id)+1,source_url:r.source_url,adapter:adapter||null};
});
const data={schema_version:2,ruleset_id:'forever-discovery-2026-09-13-v2',manifest_sha256:sha,race_classes:raceClasses,
 classic_layout:layout,records,mechanics,parameters:[{key:'rotation.forever_auto',min:0,max:1},...Object.values(adapters).flatMap(a=>a.parameters||[])]};
function emit(file, value) {const text=JSON.stringify(value,null,2)+'\n'; if(process.argv.includes('--check')) assert.equal(readFileSync(file,'utf8'),text,file); else writeFileSync(file,text);}
mkdirSync('sim/core/foreverdata',{recursive:true});
emit('sim/core/foreverdata/trees.json',data);
// The UI consumes the same versioned records; Classic's talent JSON is untouched.
mkdirSync('ui/forever/data',{recursive:true});
emit('ui/forever/data/talents.json',data);
emit('ui/forever/data/actions.json',Object.fromEntries([...records,...mechanics].map(r=>[r.action_tag,r.name])));
console.log(`Imported ${records.length} talents / 27 trees; ${Object.keys(support).length} audited adapter declarations (including non-combat and blocked scopes); ${sha}`);

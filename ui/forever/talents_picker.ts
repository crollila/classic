import type { Player } from '../core/player';
import { ForeverMode, ForeverOptions } from '../core/proto/api';
import { Spec } from '../core/proto/common';
import { TypedEvent } from '../core/typed_event';
import { el } from './metadata';
import { foreverDiscoveryTalents as data, foreverClassName, foreverRaceKeys, isExecutable } from './discovery';
import { ForeverTalentWindow } from './talent-window';

export class ForeverTalentsPicker {
 readonly root:HTMLElement;
 constructor(parent:HTMLElement,private player:Player<Spec>){
  const window=new ForeverTalentWindow(parent,player);
  this.root=el('section','forever-build-picker');window.root.append(this.root);
  let last='';const render=()=>{const f=player.getForever()!,signature=JSON.stringify({...f,talents:{},race:player.getRace()});if(signature===last)return;last=signature;this.root.replaceChildren();this.renderOptions(f);};
  player.talentsChangeEmitter.on(render);render();
 }
 private save(f:ForeverOptions){this.player.setForever(TypedEvent.nextEventID(),f);}
 private records(){return data.records.filter(r=>r.class===foreverClassName(this.player.getClass()));}
 private renderOptions(f:ForeverOptions){
  const options=el('details','forever-extra-options');options.open=false;options.append(el('summary','','Forever abilities, racials, buffs, consumes & professions'));
  const autoLabel=el('label','','Prioritize new Forever offensive abilities in the rotation '),auto=el('input');auto.type='checkbox';auto.checked=f.parameters['rotation.forever_auto']!==0;auto.addEventListener('change',()=>{f.parameters['rotation.forever_auto']=auto.checked?1:0;this.save(f)});autoLabel.append(auto);options.append(autoLabel);
  options.append(el('p','','Racial switches select Forever overrides. An unchecked racial keeps its Classic baseline behavior. Baseline rules are configured through the usual Settings controls.'));
  const cls=foreverClassName(this.player.getClass()),raceKey=foreverRaceKeys[this.player.getRace()];
  const rows=data.mechanics.filter(m=>isExecutable(m.mode)&&m.kind!=='removed_talent'&&m.kind!=='race_class_combinations'&&((m.kind==='racial'&&m.id.startsWith(`racials.${raceKey}.`))||m.category===cls||['BUFFS','CONSUMES','PROFESSIONS'].includes(m.category)));
  for(const m of rows){
   const row=el('div','forever-extra-row'),label=el('label'),input=el('input');input.type='checkbox';input.checked=f.mechanics.includes(m.id);input.setAttribute('aria-label',m.name);input.addEventListener('change',()=>{f.mechanics=f.mechanics.filter(id=>id!==m.id);if(input.checked)f.mechanics.push(m.id);else delete f.mechanicRanks[m.id];this.save(f)});
   if(['baseline','retain'].includes(m.mode)){input.checked=true;input.disabled=true}
   label.append(input,el('strong','',m.name));row.append(label,el('small','',`${m.confidence}${f.mode===ForeverMode.STRICT&&m.confidence==='PREDICTED'?' · disabled in STRICT':''}`),el('p','',m.effect));
   if(m.max_rank>1&&input.checked){const n=el('input');n.type='number';n.min='1';n.max=String(m.max_rank);n.value=String(f.mechanicRanks[m.id]||1);n.setAttribute('aria-label',`${m.name} rank`);n.addEventListener('change',()=>{f.mechanicRanks[m.id]=Math.max(1,Math.min(m.max_rank,Number(n.value)));this.save(f)});row.append(n)}
   if(m.adapter?.reason)row.append(el('p','forever-assumption',m.adapter.reason));
   for(const prediction of m.adapter?.predicted_components||[])row.append(el('p','predicted',`PREDICTED component · ${prediction}`));
   const source=el('a','','Evidence');source.href=m.source_url;source.target='_blank';source.rel='noopener noreferrer';row.append(source);options.append(row);
  }
  const descriptors=[...this.records(),...rows].flatMap(r=>r.adapter?.parameters||[]);
  for(const parameter of descriptors.filter((p,i,all)=>all.findIndex(x=>x.key===p.key)===i)){
   const label=el('label','forever-parameter',parameter.label+' '),input=el('input');input.type='number';input.min=String(parameter.min);input.max=String(parameter.max);input.value=String(f.parameters[parameter.key]??parameter.default);input.addEventListener('change',()=>{f.parameters[parameter.key]=Math.min(parameter.max,Math.max(parameter.min,Number(input.value)));this.save(f)});label.append(input,el('small','',`${parameter.confidence}: ${parameter.reason}`));options.append(label);
  }
  const items=el('details','forever-item-evidence');items.append(el('summary','','Item and set evidence'));
  for(const m of data.mechanics.filter(m=>m.category==='ITEMS')){items.append(el('p','',`${m.name}: ${m.effect} ${m.adapter?.reason||'No authenticated item identity/stat/bonus data is available; existing items remain explicit Classic planning assumptions.'}`))}
  options.append(items);this.root.append(options);
 }
}

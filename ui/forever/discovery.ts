import { Player, ForeverOptions, ForeverMode } from '../core/proto/api';
import { Class, Race } from '../core/proto/common';
import importedData from './data/talents.json';

export interface ForeverRank {rank:number;effect:string;estimated:boolean;confidence:string;source_url:string;values:number[];prediction_reason?:string|null}
export interface ForeverAdapter {confidence?:string;reason?:string;scope?:string;remaining_unknowns?:string[];predicted_components?:string[];auto_priority?:number;parameters?:Array<{key:string;label:string;default:number;min:number;max:number;confidence:string;reason:string}>}
export interface ForeverTalent {id:string;class:string;tree:string;name:string;max_rank:number;row:number;column:number;required_points:number;prerequisites:Array<{id:string;rank:number}>;classic_field:string;classic_max_rank:number;classic_spell_ids?:number[];classification?:{new_in_forever:boolean;changed_from_classic:boolean;moved:boolean;unchanged?:boolean;removed_from_classic?:boolean};mode:string;confidence:string;source_url:string;activation_text:string;ranks:ForeverRank[];action_tag:number;adapter:ForeverAdapter|null}
export interface ForeverMechanic {id:string;category:string;kind:string;name:string;effect:string;max_rank:number;mode:string;confidence:string;source_url:string;action_tag:number;adapter:ForeverAdapter|null}
export interface ForeverDataset {schema_version:number;ruleset_id:string;manifest_sha256:string;race_classes:Record<string,string[]>;classic_layout:Record<string,string[][]>;records:ForeverTalent[];mechanics:ForeverMechanic[]}
export const foreverDiscoveryTalents=importedData as unknown as ForeverDataset;
export const foreverRaceKeys:Record<number,string>={1:'dwarf',2:'gnome',3:'human',4:'night-elf',5:'orc',6:'tauren',7:'troll',8:'undead',9:'skyborne-windshaper',10:'skyborne-high-order'};
export function foreverClassName(c:Class):string {return Class[c].replace('Class','').toUpperCase()}
export function foreverRaces(c:Class):Race[]{return Object.entries(foreverRaceKeys).filter(([,k])=>foreverDiscoveryTalents.race_classes[k]?.includes(foreverClassName(c))).map(([id])=>Number(id) as Race)}
export function isExecutable(mode:string){return mode!=='blocked'&&mode!=='non-sim'}
export function effectiveForeverRank(f:ForeverOptions,r:ForeverTalent):number{
 let n=f.talents[r.id]||0;
 if(!isExecutable(r.mode))return 0;
 if(f.mode!==ForeverMode.STRICT)return n;
 if(r.confidence==='PREDICTED')return 0;
 while(n>0&&(r.ranks[n-1].estimated||r.ranks[n-1].confidence==='PREDICTED'))n--;
 return n;
}
export function foreverBuildErrors(c:Class,talents:Record<string,number>):string[]{
 const records=foreverDiscoveryTalents.records.filter(r=>r.class===foreverClassName(c));
 const errors:string[]=[];
 if(Object.values(talents).reduce((a,b)=>a+b,0)>51)errors.push('Maximum 51 talent points.');
 for(const [id,n] of Object.entries(talents)){
  const r=records.find(r=>r.id===id);if(!r||n<0||n>r.max_rank||!Number.isInteger(n)){errors.push(`Invalid talent rank: ${id}`);continue}
  if(!n)continue;
  if(r.prerequisites.some(p=>(talents[p.id]||0)<p.rank))errors.push(`${r.name}: prerequisite required.`);
  const lower=records.filter(t=>t.tree===r.tree&&t.row<r.row).reduce((a,t)=>a+(talents[t.id]||0),0);
  if(lower<r.required_points)errors.push(`${r.name}: invest ${r.required_points} points in earlier rows.`);
 }
 return errors;
}
export function foreverDefaultOptions(c:Class,race:Race):ForeverOptions{
 const raceKey=foreverRaceKeys[race];
 return ForeverOptions.create({rulesetId:foreverDiscoveryTalents.ruleset_id,mode:ForeverMode.BEST_GUESS,
  mechanics:foreverDiscoveryTalents.mechanics.filter(m=>isExecutable(m.mode)&&((m.kind==='racial'&&m.id.startsWith(`racials.${raceKey}.`))||(m.category===foreverClassName(c)&&m.mode==='ability'))).map(m=>m.id)});
}
// Semantic migration of saved Classic presets. Invalid moved-node prerequisites
// are pruned rather than interpreting positional digits as Forever positions.
export function migrateClassicTalents(c:Class,text:string):Record<string,number>{
 const cls=foreverClassName(c),oldFields:Record<string,number>={};
 text.split('-').forEach((tree,i)=>[...tree].forEach((n,j)=>oldFields[foreverDiscoveryTalents.classic_layout[cls]?.[i]?.[j]||'']=Number(n)||0));
 const talents=Object.fromEntries(foreverDiscoveryTalents.records.filter(r=>r.class===cls&&oldFields[r.classic_field]).map(r=>[r.id,Math.min(r.max_rank,oldFields[r.classic_field])]));
 let changed=true;while(changed){changed=false;for(const r of foreverDiscoveryTalents.records.filter(r=>r.class===cls)){
  if(!talents[r.id])continue;
  const lower=foreverDiscoveryTalents.records.filter(t=>t.class===cls&&t.tree===r.tree&&t.row<r.row).reduce((a,t)=>a+(talents[t.id]||0),0);
  if(lower<r.required_points||r.prerequisites.some(p=>(talents[p.id]||0)<p.rank)){delete talents[r.id];changed=true}
 }}
 return talents;
}
export function foreverClassicBridge(c:Class,f:ForeverOptions):string{
 const fields:Record<string,number>={};for(const r of foreverDiscoveryTalents.records.filter(r=>r.class===foreverClassName(c)))fields[r.classic_field]=Math.min(r.classic_max_rank,effectiveForeverRank(f,r));
 return foreverDiscoveryTalents.classic_layout[foreverClassName(c)].map(tree=>tree.map(field=>fields[field]||0).join('')).join('-');
}
// A legal exploration build, not a claim of an optimized raid allocation.
export function sampleForeverBuild(c:Class,tree:string):Record<string,number>{
 const rows=foreverDiscoveryTalents.records.filter(r=>r.class===foreverClassName(c)),chosen:Record<string,number>={};
 const total=()=>Object.values(chosen).reduce((a,b)=>a+b,0);
 const available=(r:ForeverTalent)=>!r.prerequisites.some(p=>(chosen[p.id]||0)<p.rank)&&rows.filter(x=>x.tree===r.tree&&x.row<r.row).reduce((n,x)=>n+(chosen[x.id]||0),0)>=r.required_points;
 const ensure=(r:ForeverTalent,n:number):void=>{
  for(const p of r.prerequisites){const parent=rows.find(x=>x.id===p.id)!;ensure(parent,p.rank)}
  for(let guard=0;guard<51&&!available(r)&&total()<51;guard++){
   const lower=rows.filter(x=>x.tree===r.tree&&x.row<r.row&&(chosen[x.id]||0)<x.max_rank&&available(x)).sort((a,b)=>(isExecutable(a.mode)?0:1)-(isExecutable(b.mode)?0:1)||a.row-b.row);
   if(!lower.length)break;const x=lower[0];chosen[x.id]=(chosen[x.id]||0)+1;
  }
  if(available(r))chosen[r.id]=Math.min(n,(chosen[r.id]||0)+51-total());
 };
 const cap=rows.filter(r=>r.tree===tree).sort((a,b)=>b.row-a.row||(a.adapter?.auto_priority??999)-(b.adapter?.auto_priority??999))[0];if(cap)ensure(cap,cap.max_rank);
 for(let guard=0;guard<100&&total()<51;guard++){
  const next=rows.filter(r=>(chosen[r.id]||0)<r.max_rank&&available(r)&&isExecutable(r.mode)).sort((a,b)=>(a.tree===tree?0:1)-(b.tree===tree?0:1)||(a.adapter?.auto_priority??999)-(b.adapter?.auto_priority??999)||b.row-a.row)[0];
  if(!next)break;chosen[next.id]=(chosen[next.id]||0)+1;
 }
 return chosen;
}
export function withForeverDiscovery(player:Player,options:Partial<ForeverOptions>):Player{
 const result=Player.clone(player);result.talentsString='';result.database=undefined;
 result.forever=ForeverOptions.create({...options,rulesetId:foreverDiscoveryTalents.ruleset_id});return result;
}
// Knowledge state, shared with the God knowledge model: confidence describes how
// well we know a mechanic and never means the mechanic is missing. A talent the
// sources report as unchanged keeps its WoWSims Classic behavior, which is real
// knowledge and is labeled BASELINE_ASSUMED rather than left uncertain.
export type ForeverKnowledgeState='CONFIRMED'|'PROVISIONAL'|'PREDICTED'|'BASELINE_ASSUMED';
export const foreverStateMeaning:Record<ForeverKnowledgeState,string>={
 CONFIRMED:'Direct strong Forever evidence.',
 PROVISIONAL:'Documented Forever information with some uncertain behavior.',
 PREDICTED:'Educated estimate; modeled, but the numbers are not confirmed.',
 BASELINE_ASSUMED:'We know the Classic rule and have no evidence Forever changed it, so the Classic behavior still applies.',
};
export function foreverKnowledgeState(r:ForeverTalent,predicted:boolean):ForeverKnowledgeState{
 if(r.confidence==='CONFIRMED')return 'CONFIRMED';
 const k=r.classification;
 if(k?.unchanged&&!k.changed_from_classic&&!k.new_in_forever&&!k.removed_from_classic)return 'BASELINE_ASSUMED';
 if(predicted||r.confidence==='PREDICTED')return 'PREDICTED';
 return r.confidence==='UNVERIFIED'?'PREDICTED':'PROVISIONAL';
}

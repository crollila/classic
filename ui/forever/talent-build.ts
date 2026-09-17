import { Class } from '../core/proto/common';
import { ForeverMode } from '../core/proto/api';
import { foreverDiscoveryTalents as data, foreverBuildErrors, foreverClassName, ForeverTalent } from './discovery';

export function classRecords(cls:Class) { return data.records.filter(r=>r.class===foreverClassName(cls)); }
export function treeNames(cls:Class) { return [...new Set(classRecords(cls).map(r=>r.tree))]; }
export function rankChange(cls:Class, talents:Record<string,number>, r:ForeverTalent, delta:number) {
 const rank=(talents[r.id]||0)+delta;
 if(rank<0||rank>r.max_rank) return {talents,errors:[rank<0?'No points to remove.':'Talent is already maxed.']};
 const next={...talents,[r.id]:rank};
 return {talents:next,errors:foreverBuildErrors(cls,next)};
}
// Versioned Forever string, never confused with the positional Classic format.
// Full rows retain the pinned data order. Existing saved builds/protobuf stay unchanged.
export function encodeTalents(cls:Class,talents:Record<string,number>) {
 return `F1:${foreverClassName(cls)}:${treeNames(cls).map(tree=>classRecords(cls).filter(r=>r.tree===tree).map(r=>talents[r.id]||0).join('')).join('-')}`;
}
export function decodeTalents(cls:Class,text:string):Record<string,number> {
 const prefix=`F1:${foreverClassName(cls)}:`;
 if(!text.startsWith(prefix))throw Error('Use a Forever F1 talent string for this class. Classic strings use a different layout.');
 const parts=text.slice(prefix.length).split('-'), names=treeNames(cls), talents:Record<string,number>={};
 if(parts.length!==3)throw Error('A build must contain all three trees.');
 names.forEach((tree,i)=>{
  const records=classRecords(cls).filter(r=>r.tree===tree);
  if(parts[i].length!==records.length||!/^[0-5]+$/.test(parts[i]))throw Error('Talent string has an invalid length or rank.');
  records.forEach((r,j)=>{if(Number(parts[i][j]))talents[r.id]=Number(parts[i][j]);});
 });
 const errors=foreverBuildErrors(cls,talents);if(errors.length)throw Error(errors.join(' '));
 return talents;
}
export function talentURL(url:string,cls:Class,talents:Record<string,number>,mode:ForeverMode) {
 const result=new URL(url);result.searchParams.set('ft',encodeTalents(cls,talents));
 result.searchParams.set('fm',mode===ForeverMode.STRICT?'STRICT':'BEST_GUESS');
 // The simulator hash can contain unrelated gear/settings. Talent links are talent-only.
 result.hash='';return result.href;
}
export function readTalentURL(url:string,cls:Class) {
 const parsed=new URL(url), text=parsed.searchParams.get('ft');if(!text)return null;
 const mode=parsed.searchParams.get('fm');
 if(mode!=='STRICT'&&mode!=='BEST_GUESS')throw Error('Shared build has an unsupported simulation mode.');
 return {talents:decodeTalents(cls,text),mode:mode==='STRICT'?ForeverMode.STRICT:ForeverMode.BEST_GUESS};
}

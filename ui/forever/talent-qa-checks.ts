// Synthetic browser interaction checks, invoked explicitly from the QA fixture.
import {ForeverTalentWindow} from './talent-window';
import {Class,Spec} from '../core/proto/common';
import {ForeverMode,ForeverOptions} from '../core/proto/api';
import {TypedEvent} from '../core/typed_event';
import type {Player} from '../core/player';
import {classRecords,treeNames,encodeTalents} from './talent-build';
import {foreverDefaultOptions,foreverRaces,sampleForeverBuild,effectiveForeverRank} from './discovery';
const check=(ok:unknown,message:string)=>{if(!ok)throw Error(message);};
export async function runBrowserChecks(host:HTMLElement){
 let scenarios=0,gestures=0;
 const savedURL=location.href;
 for(const cls of Object.values(Class).filter((v):v is Class=>typeof v==='number'&&v!==Class.ClassUnknown)){
  let race=foreverRaces(cls)[0],f=foreverDefaultOptions(cls,race);
  const talentsChangeEmitter=new TypedEvent<void>(),raceChangeEmitter=new TypedEvent<void>();
  // Keep init pending to isolate this fixture from the real page URL.
  const player={getClass:()=>cls,getRace:()=>race,getForever:()=>ForeverOptions.clone(f),setForever:(id:number,next:ForeverOptions)=>{f=ForeverOptions.clone(next);talentsChangeEmitter.emit(id);},setRace:(id:number,next:number)=>{race=next;raceChangeEmitter.emit(id);},talentsChangeEmitter,raceChangeEmitter,sim:{waitForInit:()=>new Promise(()=>{})}} as unknown as Player<Spec>;
  const mount=document.createElement('div');host.append(mount);const window=new ForeverTalentWindow(mount,player);
  const node=(id:string)=>mount.querySelector<HTMLButtonElement>(`[data-record-id="${id}"]`)!;
  const click=(id:string,shiftKey=false)=>node(id).dispatchEvent(new MouseEvent('click',{bubbles:true,shiftKey}));
  const set=(next:ForeverOptions)=>player.setForever(TypedEvent.nextEventID(),next);
  const records=classRecords(cls);
  check(mount.querySelectorAll('.ft-node').length===records.length,'Missing class icons');
  check(mount.querySelectorAll('.ft-tree').length===3,'Expected three trees');
  for(const mode of [ForeverMode.STRICT,ForeverMode.BEST_GUESS])for(const tree of treeNames(cls)){
   set(ForeverOptions.create({...f,mode,talents:sampleForeverBuild(cls,tree)}));
   const selected=records.reduce((n,r)=>n+(f.talents[r.id]||0),0),active=records.reduce((n,r)=>n+effectiveForeverRank(f,r),0);
   check(selected===51&&mount.querySelector('.ft-summary strong')?.textContent==='0','Points remaining');
   check(mount.querySelector('.ft-summary small')?.textContent?.includes(`${active} active`),'Active count');
   for(const r of records){const b=node(r.id),n=f.talents[r.id]||0;check(b.querySelector('.ft-rank')?.textContent===`${n}/${r.max_rank}`,'Rank display');check(b.classList.contains('ft-maxed')===(n===r.max_rank),'Maxed state');check(b.classList.contains('ft-inactive')===(effectiveForeverRank(f,r)<n),'Inactive state');}
   scenarios++;
  }
  set(ForeverOptions.create({...f,talents:{}}));
  const first=records.find(r=>r.row===1&&!r.prerequisites.length)!;
  click(first.id);check(f.talents[first.id]===1,'Left click');
  node(first.id).dispatchEvent(new MouseEvent('contextmenu',{bubbles:true,cancelable:true}));check(f.talents[first.id]===0,'Right click');
  click(first.id);click(first.id,true);check(f.talents[first.id]===0,'Shift click');
  click(first.id);node(first.id).dispatchEvent(new KeyboardEvent('keydown',{key:'-',bubbles:true,cancelable:true}));check(f.talents[first.id]===0,'Keyboard minus');
  const locked=records.find(r=>r.required_points>0)!;click(locked.id);check(!f.talents[locked.id],'Locked click spent point');
  const touch=(id:string,type:string)=>node(id).dispatchEvent(new PointerEvent(type,{bubbles:true,pointerType:'touch',clientX:20,clientY:20}));
  touch(first.id,'pointerdown');touch(first.id,'pointerup');click(first.id);check(!f.talents[first.id],'First tap must inspect');
  touch(first.id,'pointerdown');touch(first.id,'pointerup');click(first.id);check(f.talents[first.id]===1,'Second tap must add');
  touch(first.id,'pointerdown');node(first.id).dispatchEvent(new MouseEvent('contextmenu',{bubbles:true,cancelable:true}));check(f.talents[first.id]===1,'Native touch context menu must not remove early');await new Promise(resolve=>setTimeout(resolve,550));touch(first.id,'pointerup');click(first.id);check(f.talents[first.id]===0,'Hold must remove exactly once');
  touch(first.id,'pointerdown');touch(first.id,'pointercancel');click(first.id);check(f.talents[first.id]===0,'Cancelled touch must not add');
  check(mount.textContent?.includes('MECHANIC STATUS'),'Tooltip evidence');
  check(mount.textContent?.includes('COMPARE TO CLASSIC'),'Tooltip comparison');
  gestures+=9;
  // Use the real controls for persistence and import/export.
  const buttons=()=>Array.from(mount.querySelectorAll<HTMLButtonElement>('button'));
  const byText=(text:string)=>buttons().find(b=>b.textContent===text)!;
  click(first.id);const code=mount.querySelector<HTMLTextAreaElement>('textarea')!;const exported=encodeTalents(cls,f.talents);
  byText('Reset').click();code.value=exported;byText('Import string').click();check(f.talents[first.id]===1,'Import roundtrip');
  const before=JSON.stringify(f.talents);code.value='invalid';byText('Import string').click();check(JSON.stringify(f.talents)===before,'Invalid import changed build');
  const key=`forever-v2-builds-${cls}`,old=localStorage.getItem(key);
  const input=mount.querySelector<HTMLInputElement>('[aria-label="Build name"]')!;input.value='Manufactured QA build';byText('Save build').click();byText('Reset').click();
  const saved=mount.querySelector<HTMLSelectElement>('[aria-label="Saved Forever builds"]')!;saved.value=input.value;saved.dispatchEvent(new Event('change'));check(f.talents[first.id]===1,'Saved build reload');
  if(old===null)localStorage.removeItem(key);else localStorage.setItem(key,old);
  byText('Compare to Classic').click();check(window.root.classList.contains('ft-comparing'),'Comparison toggle');
  mount.remove();
 }
 history.replaceState(history.state,'',savedURL);
 return `${scenarios}/54 tree/mode UI scenarios passed; ${gestures} synthetic pointer/keyboard-path checks; all nine class import, save/load and comparison controls passed.`;
}

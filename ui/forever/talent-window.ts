import type { Player } from '../core/player';
import { Spec } from '../core/proto/common';
import { ForeverMode, ForeverOptions } from '../core/proto/api';
import { raceNames } from '../core/proto_utils/names';
import { TypedEvent } from '../core/typed_event';
import tippy, { Instance } from 'tippy.js';
import { el } from './metadata';
import { ForeverTalent, effectiveForeverRank, foreverDefaultOptions, foreverRaces, foreverRaceKeys, foreverClassName, foreverBuildErrors, sampleForeverBuild, foreverKnowledgeState, foreverStateMeaning } from './discovery';
import { classRecords, treeNames, rankChange, encodeTalents, decodeTalents, talentURL, readTalentURL } from './talent-build';
import importedPresentation from './data/talent-presentation.json';
import clientText from '../app/data/talent-text.json';
import './talent-window.css';

type Presentation={icon:string;classic:null|{status?:string;tree?:string;row?:number;col?:number;max?:number;text?:string}};
const art=importedPresentation as unknown as {trees:Record<string,Array<{name:string;icon:string;background:string}>>;talents:Record<string,Presentation>};
// Rank text as the Forever beta client writes it (talentsforever.com export, CC BY 4.0).
const clientRanks=(cls:string,name:string):string[]|undefined=>(clientText as unknown as {talents:Record<string,Record<string,{desc:string[]}>>}).talents[cls]?.[name]?.desc;
const iconURL=(icon:string)=>`https://wow.zamimg.com/images/wow/icons/large/${icon}.jpg`;
function image(icon:string){const img=el('img');img.src=iconURL(icon);img.alt='';img.draggable=false;img.addEventListener('error',()=>{img.hidden=true},{once:true});return img;}
function button(label:string,action:()=>void){const b=el('button','ft-button',label);b.type='button';b.addEventListener('click',action);return b;}

export class ForeverTalentWindow {
 readonly root=el('section','ft-window');
 private records:ForeverTalent[];
 private nodes=new Map<string,HTMLButtonElement>();
 private tips=new Map<string,Instance>();
 private totals=new Map<string,HTMLElement>();
 private arrows:Array<{path:SVGPathElement;parent:string;rank:number}>=[];
 private notice=el('div','ft-notice');
 private summary=el('div','ft-summary');
 private mode=el('select');
 private race=el('select');
 private code=el('textarea');
 private link=el('input');
 private saved=el('select');
 private selectedTouch:string|null=null;
 private compare=false;
 private ready=false;
 private copyCount=0;
 constructor(parent:HTMLElement,private player:Player<Spec>){
  this.records=classRecords(player.getClass());parent.append(this.root);
  this.notice.setAttribute('role','status');this.notice.setAttribute('aria-live','polite');
  if(!player.getForever())player.setForever(TypedEvent.nextEventID(),foreverDefaultOptions(player.getClass(),player.getRace()));
  this.build();
  player.talentsChangeEmitter.on(()=>this.refresh());
  player.raceChangeEmitter.on(()=>{
   const f=player.getForever()!;f.mechanics=f.mechanics.filter(id=>!id.startsWith('racials.'));
   f.mechanics.push(...foreverDefaultOptions(player.getClass(),player.getRace()).mechanics.filter(id=>id.startsWith('racials.')));
   player.setForever(TypedEvent.nextEventID(),f);this.refresh();
  });
  // Apply after the simulator has restored local settings, so shared links win.
  player.sim.waitForInit().then(()=>{const shared=new URL(location.href).searchParams.has('ft');this.restoreURL();this.ready=true;this.refresh();if(shared)document.querySelector<HTMLElement>('[data-bs-target="#talents-tab"]')?.click();});
  window.addEventListener('popstate',()=>this.restoreURL());
  this.refresh();
 }
 private options(){return this.player.getForever()!;}
 private save(f:ForeverOptions){this.player.setForever(TypedEvent.nextEventID(),f);}
 private message(text:string){this.notice.textContent=text;}
 private restoreURL(){try{const shared=readTalentURL(location.href,this.player.getClass());if(shared){this.save({...this.options(),...shared});if(shared.mode===ForeverMode.STRICT&&this.player.getRace()>=9)this.player.setRace(TypedEvent.nextEventID(),foreverRaces(this.player.getClass()).find(r=>r<9)!);this.message('Shared talent build loaded.');}}catch(error){this.message(String((error as Error).message));}}
 private change(r:ForeverTalent,delta:number){
  const f=this.options(),next=rankChange(this.player.getClass(),f.talents,r,delta);
  if(next.errors.length){this.message(next.errors.join(' '));return;}
  f.talents=next.talents;this.save(f);this.message(`${r.name}: ${f.talents[r.id]}/${r.max_rank}.`);
 }
 private build(){
  const top=el('header','ft-header'),title=el('div');
  title.append(el('span','ft-eyebrow','FOREVER SIM'),el('h2','',`${foreverClassName(this.player.getClass()).toLowerCase()} talents`));
  top.append(title,this.summary);this.root.append(top);
  const toolbar=el('div','ft-toolbar');
  const modeLabel=el('label','','Mode ');this.mode.setAttribute('aria-label','Forever simulation mode');
  for(const [value,label]of [[ForeverMode.BEST_GUESS,'BEST GUESS'],[ForeverMode.STRICT,'STRICT']] as const){const option=el('option','',label);option.value=String(value);this.mode.append(option);}
  this.mode.addEventListener('change',()=>{const f=this.options();f.mode=Number(this.mode.value);this.save(f);if(f.mode===ForeverMode.STRICT&&this.player.getRace()>=9)this.player.setRace(TypedEvent.nextEventID(),foreverRaces(this.player.getClass()).find(r=>r<9)!);});modeLabel.append(this.mode);
  const raceLabel=el('label','','Race ');this.race.setAttribute('aria-label','Forever race');this.race.addEventListener('change',()=>this.player.setRace(TypedEvent.nextEventID(),Number(this.race.value)));raceLabel.append(this.race);
  const compare=button('Compare to Classic',()=>{this.compare=!this.compare;compare.setAttribute('aria-pressed',String(this.compare));this.root.classList.toggle('ft-comparing',this.compare);this.refresh();});compare.setAttribute('aria-pressed','false');
  toolbar.append(raceLabel,button('Reset',()=>{const f=this.options();f.talents={};this.save(f);this.message('All talent points refunded.');}),button('Copy URL',()=>void this.copy(this.link.value)),button('Share build',()=>void this.share()),compare);
  this.root.append(toolbar,this.notice);
  this.root.append(el('p','ft-mobile-hint','Swipe across the three trees. Tap an icon to inspect it.'));
  const trees=el('div','ft-trees');trees.setAttribute('aria-label','Three talent trees');
  const maxRow=Math.max(...this.records.map(r=>r.row));
  for(const [i,tree]of treeNames(this.player.getClass()).entries()){
   const presentation=art.trees[foreverClassName(this.player.getClass())][i];
   const section=el('section','ft-tree');section.dataset.tree=tree;section.setAttribute('aria-label',tree);
   const heading=el('header','ft-tree-heading'),count=el('span','ft-tree-count');this.totals.set(tree,count);
   heading.append(image(presentation.icon),el('h3','',tree),count,button('↺',()=>{const f=this.options();for(const r of this.records.filter(r=>r.tree===tree))delete f.talents[r.id];this.save(f);this.message(`${tree} reset.`);}));
   heading.querySelector('button')!.setAttribute('aria-label',`Reset ${tree}`);
   const grid=el('div','ft-grid');grid.style.setProperty('--rows',String(maxRow));grid.style.backgroundImage=`linear-gradient(180deg,#05090b26,#05090b66),url("${presentation.background}")`;
   const svg=document.createElementNS('http://www.w3.org/2000/svg','svg');svg.setAttribute('viewBox',`0 0 400 ${maxRow*76}`);svg.setAttribute('preserveAspectRatio','none');svg.classList.add('ft-arrows');svg.setAttribute('aria-hidden','true');
   for(const r of this.records.filter(r=>r.tree===tree)){
    for(const p of r.prerequisites){
     const source=this.records.find(x=>x.id===p.id)!;
     const sx=(source.column-.5)*100,sy=(source.row-.5)*76,tx=(r.column-.5)*100,ty=(r.row-.5)*76;
     const path=document.createElementNS(svg.namespaceURI,'path') as SVGPathElement;
     // Terminate outside the icon; arrows never cover rank labels.
     let d:string;
     if(sy===ty){const sign=tx>sx?1:-1,end=tx-sign*43;d=`M ${sx+sign*43} ${sy} H ${end} m ${-sign*8} -5 l ${sign*8} 5 l ${-sign*8} 5`;}
     else {const end=ty-28;d=`M ${sx} ${sy+27} V ${sy+38} H ${tx} V ${end} m -5 -8 l 5 8 l 5 -8`;}
     path.setAttribute('d',d);svg.append(path);this.arrows.push({path,parent:p.id,rank:p.rank});
    }
   }
   grid.append(svg);
   for(const r of this.records.filter(r=>r.tree===tree)){
    const node=el('button','ft-node');node.type='button';node.dataset.recordId=r.id;node.style.gridColumn=String(r.column);node.style.gridRow=String(r.row);
    const fallback=el('span','ft-icon-fallback',r.name.split(' ').map(s=>s[0]).slice(0,3).join(''));fallback.setAttribute('aria-hidden','true');
    node.append(fallback,image(art.talents[r.id].icon),el('span','ft-rank'),el('span','ft-mark'),el('span','ft-prediction'));
    const tip=tippy(node,{content:()=>this.tooltip(r),theme:'forever-talent',allowHTML:false,interactive:true,appendTo:()=>this.root,placement:'right',maxWidth:350,delay:[80,100],duration:0,touch:false,hideOnClick:false,onShow:instance=>{instance.setContent(this.tooltip(r));}});
    this.tips.set(r.id,tip);
    let touch=false,held=false,timer:ReturnType<typeof setTimeout>|undefined,startX=0,startY=0,moved=false;
    const cancel=()=>{clearTimeout(timer);timer=undefined;};
    node.addEventListener('pointerdown',event=>{
     if(event.pointerType==='mouse'){touch=false;held=false;return;}touch=true;held=false;moved=false;startX=event.clientX;startY=event.clientY;
     timer=setTimeout(()=>{held=true;this.selectedTouch=r.id;this.change(r,-1);tip.show();},500);
    });
    node.addEventListener('pointermove',event=>{if(Math.hypot(event.clientX-startX,event.clientY-startY)>10){moved=true;cancel();}});
    node.addEventListener('pointercancel',()=>{moved=true;cancel();});node.addEventListener('pointerleave',cancel);node.addEventListener('pointerup',cancel);
    node.addEventListener('click',event=>{
     if(touch){touch=false;if(held||moved)return;if(this.selectedTouch!==r.id){for(const instance of this.tips.values())instance.hide();this.selectedTouch=r.id;tip.show();this.refresh();return;}}
     this.change(r,event.shiftKey?-1:1);
    });
    node.addEventListener('contextmenu',event=>{event.preventDefault();if(!touch&&!held)this.change(r,-1);});
    node.addEventListener('keydown',event=>{if(event.key==='Escape'){tip.hide();this.selectedTouch=null;this.refresh();}if(event.key==='-'||event.key==='Backspace'){event.preventDefault();this.change(r,-1);}});
    this.nodes.set(r.id,node);grid.append(node);
   }
   section.append(heading,grid);trees.append(section);
  }
  this.root.append(trees);
  this.root.append(el('p','ft-help','Click to add · Right-click or Shift+click to remove · Hover for details. Touch: tap to inspect, tap again to add, hold to remove. Keyboard: Enter to add, − to remove, Esc to close.'));
  this.root.append(el('p','ft-legend','✦ New in Forever   ·   ◆ Changed   ·   ↗ Moved. Talent text is the Forever beta client\'s.'));
  const samples=el('details','ft-disclosure');samples.append(el('summary','','Explore sample builds'));
  samples.append(el('p','','Legal 51-point examples, not optimized builds.'));
  for(const tree of treeNames(this.player.getClass()))samples.append(button(tree,()=>{this.save({...this.options(),talents:sampleForeverBuild(this.player.getClass(),tree)});this.message(`${tree} sample loaded.`);}));
  this.root.append(samples);this.buildLibrary();
 }
 private tooltip(r:ForeverTalent){
  const f=this.options(),n=f.talents[r.id]||0,active=effectiveForeverRank(f,r),box=el('div','ft-tooltip');
  box.append(el('h4','',r.name),el('div','ft-tooltip-rank',`Rank ${n}/${r.max_rank}`));
  if(r.activation_text)box.append(el('p','',r.activation_text));
  const ranks=n===0?[r.ranks[0]]:r.ranks.slice(n-1,Math.min(n+1,r.max_rank));
  const client=clientRanks(foreverClassName(this.player.getClass()),r.name);
  for(const rank of ranks)box.append(el('p','ft-effect',`${rank.rank===n?'Current rank':'Next rank'} ${rank.rank}\n${client?.[rank.rank-1]||rank.effect}`));
  if(!n)box.append(el('small','','Not learned.'));
  const lower=this.records.filter(t=>t.tree===r.tree&&t.row<r.row).reduce((a,t)=>a+(f.talents[t.id]||0),0);
  if(r.required_points)box.append(el('p',lower<r.required_points?'ft-unmet':'ft-met',`Requires ${r.required_points} points in earlier ${r.tree} rows (${lower} spent).`));
  for(const p of r.prerequisites)box.append(el('p',(f.talents[p.id]||0)<p.rank?'ft-unmet':'ft-met',`Requires ${p.rank} ranks in ${this.records.find(t=>t.id===p.id)?.name}.`));
  const classic=art.talents[r.id].classic,comparison=el('section','ft-comparison');comparison.append(el('h5','','COMPARE TO CLASSIC'));
  const labels=this.labels(r);if(labels.length)comparison.append(el('p','',labels.join(' · ')));
  if(classic?.tree)comparison.append(el('p','',`Classic: ${classic.tree}, row ${classic.row}, column ${classic.col}, ${classic.max} ranks.\nForever: ${r.tree}, row ${r.row}, column ${r.column}, ${r.max_rank} ranks.`));
  comparison.append(el('p','',classic?.text||'No Classic counterpart text in the captured reference.'));
  if(r.classic_spell_ids?.length)comparison.append(el('small','',`Classic spell IDs (not verified Forever IDs): ${r.classic_spell_ids.join(', ')}`));
  comparison.append(el('small','',`Catalog talent: ${r.id}`));box.append(comparison);
  const close=button('Close',()=>{this.tips.get(r.id)?.hide();this.selectedTouch=null;this.refresh();});close.classList.add('ft-tooltip-close');box.append(close);
  return box;
 }
 private labels(r:ForeverTalent){return [r.classification?.new_in_forever?'NEW IN FOREVER':'',r.classification?.changed_from_classic?'CHANGED':'',r.classification?.moved?'MOVED':''].filter(Boolean);}
 private refresh(){
  const f=this.options(),total=Object.values(f.talents).reduce((a,b)=>a+b,0),active=this.records.reduce((a,r)=>a+effectiveForeverRank(f,r),0);
  this.summary.replaceChildren(el('strong','',`${51-total}`),el('span','','POINTS REMAINING'),el('small','',`${treeNames(this.player.getClass()).map(tree=>this.records.filter(r=>r.tree===tree).reduce((n,r)=>n+(f.talents[r.id]||0),0)).join(' / ')} · ${active} active`));
  this.mode.value=String(f.mode||ForeverMode.BEST_GUESS);
  const races=foreverRaces(this.player.getClass()).filter(r=>f.mode!==ForeverMode.STRICT||r<9);
  this.race.replaceChildren(...races.map(r=>{const option=el('option','',raceNames.get(r)||foreverRaceKeys[r]);option.value=String(r);return option;}));this.race.value=String(this.player.getRace());
  for(const r of this.records){
   const node=this.nodes.get(r.id)!,n=f.talents[r.id]||0,rank=r.ranks[Math.max(0,n-1)],locked=rankChange(this.player.getClass(),f.talents,r,1).errors.length>0&&n<r.max_rank;
   const predicted=rank.estimated||rank.confidence==='PREDICTED'||r.confidence==='PREDICTED';
   node.classList.toggle('ft-locked',locked);node.classList.toggle('ft-learned',n>0);node.classList.toggle('ft-maxed',n===r.max_rank);node.classList.toggle('ft-inactive',effectiveForeverRank(f,r)<n);node.classList.toggle('ft-inspected',this.selectedTouch===r.id);
   node.classList.toggle('ft-changed',this.labels(r).length>0);node.setAttribute('aria-label',`${r.name}, rank ${n} of ${r.max_rank}${locked?', locked':''}${n===r.max_rank?', maxed':''}`);
   node.querySelector('.ft-rank')!.textContent=`${n}/${r.max_rank}`;
   node.querySelector('.ft-mark')!.textContent=r.classification?.new_in_forever?'✦':r.classification?.moved?'↗':r.classification?.changed_from_classic?'◆':predicted?'~':'';
   node.querySelector('.ft-prediction')!.textContent=predicted&&this.labels(r).length?'~':'';
   const tip=this.tips.get(r.id)!;if(tip.state.isVisible)tip.setContent(this.tooltip(r));
  }
  for(const [tree,count]of this.totals){const n=this.records.filter(r=>r.tree===tree).reduce((a,r)=>a+(f.talents[r.id]||0),0);count.textContent=String(n);count.setAttribute('aria-label',`${n} points in ${tree}`);}
  for(const arrow of this.arrows)arrow.path.classList.toggle('ft-arrow-active',(f.talents[arrow.parent]||0)>=arrow.rank);
  if(document.activeElement!==this.code)this.code.value=encodeTalents(this.player.getClass(),f.talents);
  this.link.value=talentURL(location.href,this.player.getClass(),f.talents,f.mode);
  // Avoid replacing a simulator share hash. Talent URLs use independent parameters.
  if(this.ready){const next=new URL(location.href),shared=new URL(this.link.value);next.searchParams.set('ft',shared.searchParams.get('ft')!);next.searchParams.set('fm',shared.searchParams.get('fm')!);history.replaceState(history.state,'',next);}
 }
 private storageKey(){return `forever-v2-builds-${this.player.getClass()}`;}
 private readBuilds():Record<string,unknown>{try{const value=JSON.parse(localStorage.getItem(this.storageKey())||'{}');return value&&typeof value==='object'&&!Array.isArray(value)?value:{};}catch{return {};}}
 private refreshSaved(){this.saved.replaceChildren(el('option','','Select a saved build'));for(const name of Object.keys(this.readBuilds())){const option=el('option','',name);option.value=name;this.saved.append(option);}}
 private buildLibrary(){
  const library=el('details','ft-disclosure ft-library');library.append(el('summary','','Saved builds · Import / export'));
  const name=el('input');name.placeholder='Build name';name.maxLength=80;name.setAttribute('aria-label','Build name');
  this.saved.setAttribute('aria-label','Saved Forever builds');this.refreshSaved();
  this.saved.addEventListener('change',()=>{if(!this.saved.value)return;try{const stored=this.readBuilds()[this.saved.value] as {race?:number;forever?:unknown};const f=ForeverOptions.fromJson((stored.forever||stored) as any);const errors=foreverBuildErrors(this.player.getClass(),f.talents);if(errors.length)throw Error(errors.join(' '));if(![ForeverMode.STRICT,ForeverMode.BEST_GUESS].includes(f.mode))throw Error('Unsupported saved mode.');const races=foreverRaces(this.player.getClass()).filter(r=>f.mode!==ForeverMode.STRICT||r<9);this.player.setRace(TypedEvent.nextEventID(),races.includes(stored.race!)?stored.race!:races[0]);this.save(f);this.message('Saved build loaded.');}catch{this.message('Saved build could not be loaded. Its data is invalid.');}});
  library.append(name,button('Save build',()=>{if(!name.value.trim()){this.message('Enter a build name.');name.focus();return;}try{const builds=this.readBuilds();Object.defineProperty(builds,name.value.trim(),{value:{race:this.player.getRace(),forever:ForeverOptions.toJson(this.options())},enumerable:true,configurable:true});localStorage.setItem(this.storageKey(),JSON.stringify(builds));this.refreshSaved();this.message('Build saved on this device.');}catch{this.message('Storage is unavailable. Copy your talent string to keep this build.');}}),this.saved);
  const codeLabel=el('label','','Forever talent string');this.code.rows=3;this.code.spellcheck=false;this.code.setAttribute('aria-label','Forever talent string');codeLabel.append(this.code);
  library.append(codeLabel,button('Import string',()=>{try{this.save({...this.options(),talents:decodeTalents(this.player.getClass(),this.code.value.trim())});this.message('Talent string imported.');this.code.blur();this.refresh();}catch(error){this.message((error as Error).message);}}),button('Export string',()=>{this.code.value=encodeTalents(this.player.getClass(),this.options().talents);void this.copy(this.code.value); }));
  const linkLabel=el('label','','Shareable talent URL');this.link.readOnly=true;this.link.setAttribute('aria-label','Shareable talent URL');linkLabel.append(this.link);library.append(linkLabel,el('small','','Talent URLs include class, allocation and mode. Saved builds on this device also retain race and mechanic options.'));
  this.root.append(library);
 }
 private async copy(text:string){const attempt=++this.copyCount;try{await navigator.clipboard.writeText(text);if(attempt===this.copyCount)this.message('Copied to clipboard.');}catch{this.message('Clipboard unavailable. Open Saved builds · Import / export and copy the string or URL.');(this.root.querySelector('.ft-library') as HTMLDetailsElement).open=true;this.link.focus();this.link.select();}}
 private async share(){const url=talentURL(location.href,this.player.getClass(),this.options().talents,this.options().mode);if(navigator.share){try{await navigator.share({title:'Forever talent build',url});this.message('Build shared.');}catch(error){if((error as Error).name!=='AbortError')await this.copy(url);}}else await this.copy(url);}
}

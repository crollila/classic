import {readFileSync,writeFileSync} from 'node:fs';
const m=JSON.parse(readFileSync('../forever_changes.json'));
const categories=['CORE','RACIALS','ITEMS','CONSUMES','BUFFS','PROFESSIONS'];
const out={};
function put(id,mode,confidence,reason,files=[],extra={}){out[id]={mode,confidence,reason,scope:reason,files,...extra};}
for(const id of ['core.level-cap','core.raid-sizes','core.talent-point-budget','core.classic-compatibility'])put(id,'baseline','CONFIRMED',{'core.level-cap':'Level 60 class construction; no level scaling.','core.raid-sizes':'10/20/40-player raid construction and target aura capacity.','core.talent-point-budget':'All 27 Forever topologies, 51 points, max ranks and prerequisites validated; removed Classic nodes rejected.','core.classic-compatibility':'Per-request v2 mode, semantic IDs, isolated class adapters and browser build persistence.'}[id],['sim/core/foreverdata/data.go','sim/core/character.go','ui/forever/talents_picker.ts']);
for(const r of m.sections.CORE.filter(r=>r.id.startsWith('core.unresolved.')))put(r.id,'baseline','PROVISIONAL','No global replacement is evidenced: retain the executable Classic '+r.id.split('.').at(-1).replaceAll('-',' ')+' rules, with separately implemented Forever overrides. This is a fallback, not confirmation of Forever interactions.',['sim/core/']);
put('core.skyborne-base-stats','baseline','PREDICTED','Use the Classic class table plus Night Elf racial offsets for both Skyborne factions. Replace this isolated prediction when a client racial table exists.',['sim/core/character.go']);
for(const r of m.sections.RACIALS.filter(r=>r.kind==='race_class_combinations'))put(r.id,'baseline',r.id.includes('skyborne')?'PREDICTED':'PROVISIONAL','All documented race/class combinations are selectable in the browser and validated by the engine. '+(r.id.includes('skyborne')?'Base stats use the disclosed Night Elf-offset prediction.':'Existing race offsets and Classic class base stats are retained.'),['sim/core/foreverdata/data.go','ui/forever/discovery.ts']);
const racialKnown=['orc.blood-fury','orc.axe-specialization','orc.hardiness','tauren.war-stomp','tauren.endurance','troll.berserking','troll.beast-slaying','human.sword-specialization','human.the-human-spirit','dwarf.mace-specialization','dwarf.big-game-hunter','night-elf.quickness','gnome.expansive-mind','skyborne-windshaper.wind-blessed','skyborne-windshaper.elemental-insight','skyborne-high-order.wind-blessed','skyborne-high-order.elemental-insight'];
for(const suffix of racialKnown)put('racials.'+suffix,'racial','PROVISIONAL','Observed numerical racial effect executes with the nearest Classic stacking, tick, weapon and cooldown conventions.',['sim/core/racials.go','sim/core/forever_racials.go','sim/core/forever_racials_expanded.go']);
const racialPred={
 'orc.shatter-curse':'Curse/Bane cleanse and immunity for known 8 seconds. Predict 10% magic damage reduction and 3-minute cooldown from analogous defensive racials.',
 'undead.will-of-the-forsaken':'Remove fear immediately; Classic 2-minute cooldown. No post-cast immunity beyond the instantaneous removal. Charm/sleep share the control engine when encountered.',
 'undead.cannibalize':'Known 35% Health/Mana restored over Classic 10 seconds, with predicted 3-minute cooldown. Requires the explicit corpse-available encounter option; damage interrupts recovery.',
 'undead.touch-of-the-grave':'Predict 5% attack/spell-hit proc, 10-second internal cooldown, drain 5% of maximum health as Shadow damage and heal actual damage. These are explicit exploratory values, not observed facts.',
 'tauren.plainsrunning':'Predict +5% run speed per continuous moving second, capped at +30% after 6 seconds; stop resets the ramp. Travel time responds to the ramp.',
 'troll.rapid-regeneration':'Known 50% maximum-health recovery; predict a 10-second duration and 3-minute cooldown.',
 'troll.regeneration':'Known 10% combat health regeneration; use an explicit spirit/4 + 6 per 2-second baseline prediction until racial health-regen formulas are logged.',
 'human.will-to-survive':'Remove stuns; predict 2-minute cooldown using the analogous racial ability. No sustained immunity.',
 'dwarf.stoneform':'Known 8-second bleed/poison/disease cleanse and immunity. Predict 10% physical reduction, keep Classic 3-minute cooldown, replace the Classic armor increase.',
 'night-elf.elune-s-light':'Known +10% melee/spell crit for 15 seconds; predict 3-minute cooldown using analogous offensive racials.',
 'gnome.escape-artist':'Root/snare cleanse and immunity; predict 5 seconds of immunity and retain the Classic 1-minute cooldown.',
 'gnome.eureka':'Known next 3 eligible abilities gain +10% damage/healing. Predict 20% cost reduction, a 30-second charge window and 3-minute cooldown.',
 'skyborne-windshaper.skysight':'Known +10% run speed; model a preapplied persistent Elemental Blessing until its duration is observed.',
 'skyborne-high-order.read-ley-line':'Known +100% Health/Mana regeneration; predict 15-second duration and 3-minute cooldown. Duplicate actual mana regen ticks, with a disclosed spirit-based health-regen baseline.'
};
for(const [suffix,reason]of Object.entries(racialPred))put('racials.'+suffix,'racial','PREDICTED',reason,['sim/core/forever_racials_expanded.go','sim/core/forever_control.go','sim/core/forever_dispel.go']);
out['racials.undead.cannibalize'].parameters=[{key:'scenario.corpse_available',label:'Usable corpse available (0/1)',default:0,min:0,max:1,confidence:'PREDICTED',reason:'Explicit encounter scenario; no corpse is invented by the simulation.'}];
const nonSim={
 'core.demo-context':'Evidence context about level-38 screenshots; not a combat modifier.',
 'legacy.well-rested':'Rested experience has no effect at the level-60 raid cap.',
 'legacy.high-alert':'Ambient stealth detection is outside the raid target model.',
 'legacy.thrill-of-adventure':'Tooltip explicitly disables the recovery inside dungeons and raids.',
 'legacy.the-quick-and-the-dead':'Movement while dead and resource discounts end upon entering combat.',
 'legacy.reinforce':'Durability loss after death does not change the equipped starting raid gear.',
 'legacy.diplomat':'Reputation rewards are outside combat.',
 'legacy.for-great-honor':'Honor rewards are outside raid combat.',
 'legacy.frequent-flier':'Flight-path cost and speed are travel economy.',
 'legacy.source-discrepancy':'Evidence-source discrepancy, not a separate mechanic to execute.',
 'racials.undead.underwater-breathing':'Underwater breath duration is outside the configured raid encounters.',
 'racials.tauren.cultivation':'Herb gathering yield is outside combat.',
 'racials.human.perception':'Ambient stealth detection is outside the raid target model.',
 'racials.dwarf.find-treasure':'Treasure tracking does not modify raid combat.',
 'racials.night-elf.shadowmeld':'Stationary world stealth has no asserted combat threat-drop effect in the evidence; ambient detection is outside the raid target model.',
 'racials.night-elf.wisp-spirit':'Movement after death does not affect an alive raid character.',
 'racials.skyborne-windshaper.walk-on-air':'Gliding/fall travel is outside the raid encounter geometry.',
 'racials.skyborne-high-order.walk-on-air':'Gliding/fall travel is outside the raid encounter geometry.',
 'legacy.field-medicine':'Tooltip explicitly disables the recently-bandaged reduction in dungeons and raids.',
 'consumes.first-aid-healing-potion-recipes':'Crafting access changes; existing named healing potions keep their executable Classic effects.',
 'legacy.working-overtime':'Skill-up probability is a crafting/progression reward.',
 'legacy.performance-bonus':'Merchant Favor on crate turn-in is outside combat.',
 'legacy.luremaster':'Extra fishing yield is outside combat.',
 'legacy.master-chef':'Extra cooking yield changes crafting quantity, not consumed food strength.',
 'legacy.dedicated-study':'Skill gains and Elemental Essence rewards do not modify combat stats.',
 'legacy.bountiful-harvest':'Gathering materials is outside combat.',
 'legacy.reagent-economy':'Vendor reagent and campsite crafting costs affect inventory economy; the raid model starts with required reagents.',
 'profession.blacksmithing.repair-all':'Repair convenience does not change fully repaired raid equipment.',
 'profession.enchanting.shard-transmutes':'Crafting material conversion has no direct combat effect.',
 'profession.engineering.powerful-profession':'A qualitative recommendation, not an executable bonus.',
 'profession.engineering.goblin-or-gnome':'Existing specialization recipe access is preparation, without a new quantified combat recipe.',
 'profession.herbalism.lotus-affinity':'Rare herb discovery probability is outside combat.',
 'profession.herbalism.brutal-pickings':'Gathering yield is outside combat.',
 'profession.leatherworking.must-go-faster':'Mounted travel speed is outside raid combat.',
 'profession.mining.a-lode-of-ore':'Ore yield is outside combat.',
 'profession.mining.malicious-mining':'Gathering rewards lack a specified usable combat item.',
 'profession.skinning.rare-hides':'Skinning yield is outside combat.',
 'profession.skinning.a-classic-starter-profession':'A preparation recommendation, not an executable effect.',
 'profession.tailoring.cloth-scavenging':'Cloth loot yield is outside combat.'
};
for(const[id,reason]of Object.entries(nonSim))put(id,'non-sim','UNKNOWN',reason);
put('legacy.talented','baseline','PROVISIONAL','Earlier leveling unlock retains the same executable 51-point cap at level 60.',['sim/core/foreverdata/data.go']);
put('racials.legacy-omissions','baseline','PROVISIONAL','Unproven omissions retain the existing Classic resistances/legacy effects; explicitly replaced weapon racials disable their old skill effects.',['sim/core/racials.go']);
const blocked={
 'racials.gnome.engineering-specialization':'No affected device, baseline malfunction event, or reduction magnitude is identified. Applying an arbitrary combat bonus would not represent the stated reliability mechanic.',
 'items.new-catalogue':'No identifiable new item with a stat line, proc or authenticated item ID is available in the manifest; a catalogue announcement cannot determine a playable item.',
 'items.tier-sets':'No individual set name/identity, piece threshold, or bonus text is available. There is no effect to extrapolate from.',
 'items.legendary':'The reward identity, stats and effect are explicitly unrevealed; no numerical or behavioral anchor exists.',
 'items.enchants':'Specific profession predictions are implemented separately; no distinct authenticated Forever enchant catalogue or effect IDs exist to equip as real enchants.',
 'items.feral-gear':'No changed Pummeler/Wolfshead effect or feral-AP rule is evidenced. Current items remain usable, but inventing a specific replacement is unsupported.',
 'profession.alchemy.philosopher-s-stone':'No upgrade levels, stat line or combat effect is given for the claimed new trinket; there is no specific upgrade curve to extrapolate.',
 'profession.alchemy.new-recipes':'No new recipe output identity or numerical consume effect is provided.',
 'profession.blacksmithing.new-recipes':'No specific new crafted item stats or effects are provided.',
 'profession.enchanting.stave-crafting-and-more':'No named staff/stat line or crafting-only combat effect is provided.',
 'profession.engineering.but-wait-there-s-more':'No particular gadget, bomb or trinket and no effect specification are provided.',
 'profession.leatherworking.dragons-and-elements':'No named new set, piece stats or set bonus is provided.',
 'profession.tailoring.new-and-reworked-recipes':'No particular new cloth item or changed stat line is provided.'
};
for(const[id,reason]of Object.entries(blocked))put(id,'blocked','UNKNOWN',reason);
put('consumes.unchanged-baseline','baseline','PROVISIONAL','Existing Classic consumes execute unchanged when selected. Availability remains a planning assumption.',['sim/core/consumes.go']);
for(const[id,kind]of [['consumes.woolen-tourniquet','bleed'],['consumes.simple-poultice','disease'],['consumes.anti-venom','poison']])put(id,'consume','PROVISIONAL',`Remove active ${kind} effects. BEST_GUESS predicts a 1-minute cooldown using Classic anti-venom conventions; STRICT permits one use per encounter.`,['sim/core/forever_professions.go','sim/core/forever_dispel.go'],{predicted_components:['Repeat-use cooldown of 60 seconds; disabled in STRICT.']});
put('buffs.camping','buff','PREDICTED','Prepull campsite feature action requires stationary preparation. Model 5-second setup, shared 1-hour feature timer, and First Aid Kit; unspecified camp contributions remain unavailable.',['sim/core/forever.go']);
put('buffs.first-aid-kit','buff','PROVISIONAL','Actual +34 Stamina, exclusive with Fortitude. Preapplied campsite scenario; configurable time remaining, default30 minutes is a prediction.',['sim/core/forever.go','sim/core/forever_professions.go'],{parameters:[{key:'scenario.campsite_seconds_remaining',label:'Campsite buff seconds remaining at pull',default:1800,min:1,max:14400,confidence:'PREDICTED',reason:'Duration unobserved; 30-minute class-buff analogy, directly editable.'}]});
put('buffs.world-buffs','baseline','PREDICTED','BEST_GUESS executes selected Classic world buffs as availability assumptions. STRICT suppresses unconfirmed world buffs without mutating the saved selection.',['sim/core/forever_professions.go','sim/core/buffs.go']);
put('buffs.party-scope','baseline','PROVISIONAL','Class implementations apply their recorded personal/party buff scopes; this is an aggregate reference to those executing mechanics.',['sim/hunter/','sim/druid/','sim/paladin/','sim/shaman/']);
put('legacy.permanence','buff','PROVISIONAL','Known rank1 +50% class-stat/campsite duration; higher ranks extrapolate linearly only in BEST_GUESS.',['sim/core/forever.go','sim/core/forever_professions.go']);
put('legacy.field-guide','buff','PROVISIONAL','Known rank1 campsite cooldown -8%; higher ranks extrapolate linearly only in BEST_GUESS. Actual feature cooldown duration changes.',['sim/core/forever.go']);
put('legacy.gourmand','buff','PROVISIONAL','Known rank1 food duration +33%; higher ranks extrapolate linearly only in BEST_GUESS. Actual food stats expire according to the configured time at pull.',['sim/core/forever_professions.go','sim/core/consumes.go'],{parameters:[{key:'scenario.food_seconds_remaining',label:'Food seconds remaining at pull',default:900,min:1,max:14400,confidence:'PREDICTED',reason:'15-minute Classic food scenario; explicitly editable.'}]});
const professions={
 'alchemy.mixology':['profession.alchemy.effect_bonus','Elixir/flask effect bonus',.25,0,1,'Predict +25% stats using analogous profession consume bonuses; actual flask/elixir stats change.'],
 'blacksmithing.belt-buckles':['profession.blacksmithing.socket_stat','Belt socket primary stat',8,0,40,'Predict a +8 primary-stat socket using early expansion gem magnitude; requires equipped belt and Blacksmithing.'],
 'enchanting.ring-enchants':['profession.enchanting.spell_power_per_ring','Spell power per enchanted ring',12,0,100,'Predict +12 spell power per equipped ring from analogous ring enchants; profession required.'],
 'herbalism.natural-talent':['profession.herbalism.resistance','Herbalism resistance to each school',10,0,100,'Predict +10 to five magic resistances using Classic racial resistance magnitude.'],
 'tailoring.embroider-your-work':['profession.tailoring.embroidery_spell_power','Cloak embroidery spell power',15,0,100,'Predict passive +15 spell power on an equipped cloak using a Classic enchant-scale bonus; profession required.']
};
for(const[suffix,[key,label,value,min,max,reason]]of Object.entries(professions))put('profession.'+suffix,'profession','PREDICTED',reason,['sim/core/forever_professions.go','sim/core/consumes.go'],{parameters:[{key,label,default:value,min,max,confidence:'PREDICTED',reason}]});
put('profession.leatherworking.armor-kits','profession','PREDICTED','Use Classic +40 armor kit magnitude on equipped leather/mail chest, legs, gloves and boots; future Forever slot/value overrides remain easy to replace.',['sim/core/forever_professions.go']);
put('profession.mining.made-of-metal','profession','PREDICTED','Execute the community claim of +5% total health only with Mining selected; the claim itself is unverified.',['sim/core/forever_professions.go']);
put('profession.skinning.beasts-of-the-wild','profession','PREDICTED','Execute the community claim of +5% Beast/Dragonkin damage only with Skinning selected; the claim itself is unverified.',['sim/core/forever_professions.go']);
put('professions.slot-count','baseline','CONFIRMED','The engine and browser retain two primary profession selections; no unsupported third slot is added.',['sim/core/character.go','ui/core/player.ts']);
out['legacy.permanence'].parameters=[{key:'scenario.class_buff_seconds_remaining',label:'Class stat-buff seconds remaining at pull',default:1800,min:1,max:14400,confidence:'PROVISIONAL',reason:'Classic 30-minute buff scenario; editable remaining lifetime.'}];
out['profession.alchemy.mixology'].parameters.push(...[['flask',7200],['elixir',3600]].map(([kind,duration])=>({key:`scenario.${kind}_seconds_remaining`,label:`${kind} seconds remaining at pull`,default:duration,min:1,max:14400,confidence:'PROVISIONAL',reason:'Classic duration fallback; selected Mixology predicts doubled lifetime in BEST_GUESS.'})));
out['profession.alchemy.mixology'].reason+=' Predict doubled duration using the expansion Mixology analogue; both stat expiration and magnitude execute.';
out['profession.alchemy.mixology'].scope=out['profession.alchemy.mixology'].reason;
for(const id of ['legacy.permanence','legacy.field-guide','legacy.gourmand'])out[id].predicted_components=['Ranks above observed rank 1 extrapolate linearly in BEST_GUESS only.'];
out['buffs.first-aid-kit'].predicted_components=['Default 30-minute remaining lifetime in BEST_GUESS. STRICT retains the Classic preapplied-buff snapshot unless a remaining lifetime is explicitly supplied.'];
out['buffs.first-aid-kit'].scope+=' STRICT keeps a preapplied snapshot unless a remaining lifetime is explicitly supplied as an encounter input.';
out['buffs.first-aid-kit'].parameters[0].reason='Explicit remaining-lifetime scenario. Unspecified BEST_GUESS lifetime predicts 30 minutes; STRICT uses the Classic preapplied-buff snapshot.';
out['legacy.gourmand'].parameters[0].confidence='PROVISIONAL';
for(const c of categories)for(const r of m.sections[c])if(!out[r.id])throw Error('Missing individual audit '+r.id);
writeFileSync('scripts/forever-core-adapters.json',JSON.stringify(out,null,2)+'\n');
console.log('Audited '+Object.keys(out).length+' cross-class records');

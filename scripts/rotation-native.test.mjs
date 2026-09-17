import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync,existsSync } from 'node:fs';
import path from 'node:path';
import { findIdealRotation,tool } from './rotation-tool.mjs';
const executable=path.resolve('.artifacts/rotationworker.exe');
test('God tool performs actual WoWSims optimization in both modes', {skip:!existsSync(executable)},async()=>{
 const request=JSON.parse(readFileSync('examples/mage-discovery.json','utf8'));
 // Manufactured strategy deliberately prioritizes Scorch over Frostbolt.
 request.raid.parties[0].players[0].rotation={type:'TypeAPL',priorityList:[{action:{castSpell:{spellId:{spellId:10207}}}},{action:{castSpell:{spellId:{spellId:25304}}}}]};
 for(const mode of ['STRICT','BEST_GUESS']){
  request.raid.parties[0].players[0].forever.mode=mode;
  const result=await findIdealRotation({request,seed:12345,policy:{candidates:24}},{executable,cache:new Map()});
  assert.equal(result.mode,mode);assert.equal(result.status,'VALIDATED_IMPROVEMENT');assert.ok(result.confidence.lower>0);
  assert.equal(result.search.iterationsPerRotation,2048);assert.match(result.explanation.priority.join(' '),/Frostbolt/);
  assert.ok(result.mechanics.manifest_sha256);assert.ok(result.mechanics.modes.includes(mode));
 }
 assert.equal(tool.name,'find_ideal_rotation');
});
test('native parser rejects invalid APL references', {skip:!existsSync(executable)},async()=>{
 const request=JSON.parse(readFileSync('examples/mage-discovery.json','utf8'));
 await assert.rejects(findIdealRotation({request,seed:123},{executable}),/Current APL is invalid/);
});

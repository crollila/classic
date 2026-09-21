import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, unlinkSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';

test('boundary checks retained files, raw binary bytes, missing files and untracked engine files',()=>{
 const root=mkdtempSync(path.join(tmpdir(),'forever-boundary-test-'));
 const put=(file,data)=>{mkdirSync(path.dirname(path.join(root,file)),{recursive:true});writeFileSync(path.join(root,file),data);};
 const git=(...args)=>{const r=spawnSync('git',args,{cwd:root,encoding:'utf8'});assert.equal(r.status,0,r.stderr);return r.stdout.trim();};
 const sha=data=>createHash('sha256').update(data).digest('hex');
 try{
  put('scripts/check-classic.mjs',readFileSync(new URL('./check-classic.mjs',import.meta.url)));
  put('sim/core/synthetic.go','package core\n');put('assets/database/synthetic.bin',Buffer.from([0xff,0]));
  git('init','--quiet');git('add','.');git('-c','user.name=Fixture','-c','user.email=fixture@example.invalid','commit','--quiet','-m','Synthetic baseline');
  const upstream=git('rev-parse','HEAD');
  put('sim/forever/baseline.json',JSON.stringify({upstream_commit:upstream}));
  put('sim/forever/reviewed-boundary.json',JSON.stringify({upstream_commit:upstream,files:{'sim/core/synthetic.go':sha('package core\n')},binary_files:{'assets/database/synthetic.bin':sha(Buffer.from([0xff,0]))}}));
  const check=()=>{const r=spawnSync(process.execPath,['scripts/check-classic.mjs','--json'],{cwd:root,encoding:'utf8'});return {status:r.status,...JSON.parse(r.stdout)};};
  assert.equal(check().status,0);
  put('sim/core/synthetic.go','package changed\n');
  put('assets/database/synthetic.bin',Buffer.from([0xfe,0]));
  put('sim/core/untracked.go','package core\n');
  const changed=check();assert.equal(changed.status,1);assert.equal(changed.issues.length,3);
  assert(changed.issues.some(i=>i.kind==='untracked-protected-file'));
  assert(changed.issues.some(i=>i.file.endsWith('.bin')&&i.kind==='changed-review-pin'));
  unlinkSync(path.join(root,'sim/core/synthetic.go'));
  assert(check().issues.some(i=>i.kind==='missing-reviewed-file'));
 }finally{
  assert.equal(path.dirname(root),path.resolve(tmpdir()));
  assert(path.basename(root).startsWith('forever-boundary-test-'));
  rmSync(root,{recursive:true,force:true});
 }
});

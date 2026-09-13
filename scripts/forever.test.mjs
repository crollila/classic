import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync, readdirSync } from 'node:fs';
import http from 'node:http';
import { validateRelease, rewriteLocalPaths, CONFIDENCE } from './forever-contract.mjs';
import { serve } from './serve-forever.mjs';

const baseline = () => JSON.parse(readFileSync(new URL('../../shared/simulator/release.json', import.meta.url), 'utf8'));
test('baseline never claims a validated Forever build', () => {
  const data = validateRelease(baseline());
  assert.equal(data.engineMode, 'upstream-baseline');
  assert.equal(data.foreverBuild, null);
  assert.equal(data.mechanicsUpdatedAt, null);
});
test('a published ruleset needs explicit build and date provenance', () => {
  assert.throws(() => validateRelease({ ...baseline(), engineMode:'forever-ruleset' }));
  assert.throws(() => validateRelease({ ...baseline(), mechanicsUpdatedAt:'not a date' }));
});
test('all four confidence levels are supported, with evidence for strong claims', () => {
  for (const confidence of CONFIDENCE) {
    const mechanic = { id:'test', label:'Test mechanic', category:'talents', note:'Test-only metadata', confidence, specs:[0], evidence:['https://example.com/evidence'] };
    assert.doesNotThrow(() => validateRelease({ ...baseline(), mechanics:[mechanic] }));
  }
  const mechanic = { id:'test', label:'Test', category:'race', note:'Test', confidence:'CONFIRMED' };
  assert.throws(() => validateRelease({ ...baseline(), mechanics:[mechanic] }));
  assert.throws(() => validateRelease({ ...baseline(), mechanics:[{ ...mechanic, confidence:'probably' }] }));
});
test('path adaptation preserves external Classic references', () => {
  const source = "'/classic/assets/database/db.json' https://wowhead.com/classic/spell=123 https://github.com/wowsims/classic";
  const result = rewriteLocalPaths(source);
  assert.match(result, /\/forever-sim\/assets\/database\/db.json/);
  assert.match(result, /https:\/\/wowhead.com\/classic\/spell=123/);
  assert.match(result, /https:\/\/github.com\/wowsims\/classic/);
});
test('built entry points load only existing local bundles and keep analytics out', t => {
  const dist = new URL('../dist/forever-sim/', import.meta.url);
  if (!existsSync(new URL('index.html', dist))) { t.skip('Run build:forever first'); return; }
  const pages = [new URL('index.html', dist), ...readdirSync(dist, { withFileTypes:true }).filter(e => e.isDirectory() && existsSync(new URL(`${e.name}/index.html`, dist))).map(e => new URL(`${e.name}/index.html`, dist))];
  assert.ok(pages.length >= 15, 'Launcher plus shared spec pages must be built');
  for (const page of pages) {
    const html = readFileSync(page, 'utf8');
    assert.doesNotMatch(html, /googletagmanager|\/classic\/assets\//);
    for (const match of html.matchAll(/(?:src|href)="(\/forever-sim\/[^"#]+)"/g)) {
      assert.ok(existsSync(new URL(match[1].replace('/forever-sim/', ''), dist)), `${page.pathname}: missing ${match[1]}`);
    }
  }
  for (const file of readdirSync(new URL('bundle/', dist)).filter(f => /\.css$/.test(f))) {
    // Check only current CSS referenced by entry points; old immutable bundles may remain between local builds.
    if (pages.some(page => readFileSync(page, 'utf8').includes(file))) assert.doesNotMatch(readFileSync(new URL(`bundle/${file}`, dist), 'utf8'), /\/classic\/assets\//);
  }
});
test('static application preserves deep routes, WASM MIME, and real 404s', async t => {
  if (!existsSync(new URL('../dist/forever-sim/warrior/index.html', import.meta.url))) { t.skip('Run build:forever first'); return; }
  const server = http.createServer(serve);
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  t.after(() => new Promise(resolve => server.close(resolve)));
  const base = `http://127.0.0.1:${server.address().port}`;
  for (const route of ['/forever-sim/', '/forever-sim/warrior/', '/forever-sim/mage/', '/forever-sim/release.json', '/forever-sim/assets/database/db.json']) {
    const response = await fetch(base + route); assert.equal(response.status, 200, route); await response.arrayBuffer();
  }
  const wasm = await fetch(base + '/forever-sim/lib.wasm', { method:'HEAD' });
  assert.equal(wasm.headers.get('content-type'), 'application/wasm');
  assert.equal((await fetch(base + '/forever-sim/missing.js')).status, 404);
  assert.equal((await fetch(base + '/forever-sim/warrior', { redirect:'manual' })).headers.get('location'), '/forever-sim/warrior/');
  assert.equal((await fetch(base + '/forever-sim', { redirect:'manual' })).headers.get('location'), '/forever-sim/');
  assert.equal((await fetch(base + '/main-site-route')).status, 404);
});

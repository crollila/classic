// Live Forever data: manifest + hash verification, and fallback to the embedded document.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createHash, webcrypto } from 'node:crypto';
import { validateRelease } from './forever-contract.mjs';
import { loadLiveOverrides, applyWorkerResult, describeLiveData, resolveLiveDataUrl, validateManifest, engineCommitDiffers, DEFAULT_LIVE_DATA_URL } from '../ui/forever/live-data.mjs';

const release = () => JSON.parse(readFileSync(new URL('../examples/release-discovery-v2.json', import.meta.url), 'utf8'));
const liveDocument = readFileSync(new URL('../sim/core/foreverdata/testdata/overrides_valid.json', import.meta.url));
const liveManifest = (overrides = {}) => ({ schema:'forever-sim-publication-1', overrides_sha256:createHash('sha256').update(liveDocument).digest('hex'),
  overrides_source_hash:JSON.parse(liveDocument).source_hash, engine_commit:'e7398b5eaaaabbbbccccddddeeeeffff00001111', published_at:'2026-09-16T12:30:00Z',
  counts:{ spells:3, items:2, talents:1, parameters:1 }, ...overrides });
function liveFetch(files, calls = []) {
  return async (url, init) => {
    calls.push({ url, init });
    const body = files[url.split('/').pop()];
    if (body instanceof Error) throw body;
    const bytes = typeof body === 'function' ? body() : body;
    return { ok:body !== undefined, status:body === undefined ? 404 : 200, arrayBuffer:async () => new Uint8Array(Buffer.from(bytes ?? '')).buffer };
  };
}
const live = (files, extra = {}) => loadLiveOverrides({ baseUrl:'https://data.example/forever/sim/', fetch:liveFetch(files, extra.calls), subtle:webcrypto.subtle, retryDelayMs:0, ...extra });

test('live data is accepted only when it matches the manifest hash', async () => {
  const calls = [];
  const data = await live({ 'manifest.json':JSON.stringify(liveManifest()), 'overrides.json':liveDocument }, { calls });
  assert.equal(data.status, 'live');
  assert.equal(data.text, liveDocument.toString('utf8'));
  assert.deepEqual(calls.map(c => c.url), ['https://data.example/forever/sim/manifest.json', 'https://data.example/forever/sim/overrides.json']);
  assert.ok(calls.every(c => c.init.cache === 'no-cache' && c.init.credentials === 'omit'));
  assert.deepEqual(describeLiveData(data, { upstreamCommit:'e7398b5eaaaa' }), { text:'live 9f86d081884c, published 2026-09-16 12:30 UTC', live:true, warning:null });
  assert.match(describeLiveData(data, { upstreamCommit:'0123456789ab' }).warning, /engine update pending/);
});

test('live data falls back to the embedded document on any failure', async () => {
  const manifest = JSON.stringify(liveManifest());
  const tampered = Buffer.from(liveDocument.toString('utf8').replace('0.5', '0.9'));
  const cases = [
    [{ 'manifest.json':new TypeError('Failed to fetch') }, /manifest unavailable: Failed to fetch/],
    [{ 'overrides.json':liveDocument }, /manifest unavailable: manifest.json returned HTTP 404/],
    [{ 'manifest.json':'<html>', 'overrides.json':liveDocument }, /manifest unavailable/],
    [{ 'manifest.json':JSON.stringify(liveManifest({ schema:'forever-sim-publication-2' })), 'overrides.json':liveDocument }, /schema/],
    [{ 'manifest.json':manifest }, /overrides.json returned HTTP 404/],
    [{ 'manifest.json':manifest, 'overrides.json':tampered }, /hash mismatch/],
    [{ 'manifest.json':JSON.stringify(liveManifest({ overrides_source_hash:'0'.repeat(64) })), 'overrides.json':liveDocument }, /source_hash differs/],
  ];
  for (const [files, reason] of cases) {
    const data = await live(files);
    assert.equal(data.status, 'embedded');
    assert.equal(data.text, undefined);
    assert.match(data.reason, reason);
    assert.ok(describeLiveData(data).text.startsWith('embedded (live data unavailable: '));
  }
  assert.equal((await loadLiveOverrides({ baseUrl:'', fetch:liveFetch({}), subtle:webcrypto.subtle })).status, 'embedded');
  assert.match((await loadLiveOverrides({ baseUrl:'https://data.example/', fetch:liveFetch({}) })).reason, /WebCrypto/);
  // The publisher replaces two files: one hash mismatch is retried before giving up.
  let reads = 0;
  const retried = await live({ 'manifest.json':manifest, 'overrides.json':() => (reads++ ? liveDocument : tampered) });
  assert.equal(retried.status, 'live');
  // The engine-pending warning is still shown when a valid manifest was read.
  const mismatch = await live({ 'manifest.json':manifest, 'overrides.json':tampered });
  assert.match(describeLiveData(mismatch, { upstreamCommit:'0123456789ab' }).warning, /engine update pending/);
});

test('an engine rejection of the live document is surfaced as embedded', async () => {
  const data = await live({ 'manifest.json':JSON.stringify(liveManifest()), 'overrides.json':liveDocument });
  const summary = { source_hash:liveManifest().overrides_source_hash };
  assert.equal(applyWorkerResult(data, JSON.stringify({ ok:true, summary })), data);
  assert.match(applyWorkerResult(data, JSON.stringify({ ok:false, error:'overrides: unknown top-level key "auras"' })).reason, /engine rejected the live document: overrides: unknown top-level key/);
  assert.equal(applyWorkerResult(data, JSON.stringify({ ok:true, summary:{ source_hash:'f'.repeat(64) } })).status, 'embedded');
  assert.equal(applyWorkerResult(data, 'not json').status, 'embedded');
});

test('live data URL, manifest and release contract', () => {
  assert.equal(resolveLiveDataUrl(undefined), DEFAULT_LIVE_DATA_URL);
  assert.equal(resolveLiveDataUrl(''), '');
  assert.equal(resolveLiveDataUrl('https://data.example/x'), 'https://data.example/x/');
  assert.equal(resolveLiveDataUrl('http://127.0.0.1:8099/live/'), 'http://127.0.0.1:8099/live/');
  assert.throws(() => resolveLiveDataUrl('http://data.example/'));
  assert.throws(() => resolveLiveDataUrl('https://data.example/?x=1'));
  assert.doesNotThrow(() => validateManifest(liveManifest()));
  for (const bad of [{ overrides_sha256:'abc' }, { engine_commit:'HEAD' }, { published_at:'yesterday' }, { counts:{ spells:1 } }]) assert.throws(() => validateManifest(liveManifest(bad)));
  assert.equal(engineCommitDiffers('e7398b5eaaaabbbb', 'e7398b5eaaaa'), false);
  assert.equal(engineCommitDiffers('e7398b5eaaaabbbb', 'unavailable'), false);
  assert.equal(engineCommitDiffers('e7398b5eaaaabbbb', 'fa5511039abc'), true);
  assert.doesNotThrow(() => validateRelease({ ...release(), liveDataUrl:DEFAULT_LIVE_DATA_URL, embeddedOverridesSha256:'a'.repeat(64) }));
  assert.doesNotThrow(() => validateRelease({ ...release(), liveDataUrl:'', embeddedOverridesSha256:null }));
  assert.throws(() => validateRelease({ ...release(), liveDataUrl:'ftp://x' }));
  assert.throws(() => validateRelease({ ...release(), embeddedOverridesSha256:'nope' }));
});

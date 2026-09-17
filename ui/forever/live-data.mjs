// Live Forever data: the validated overrides document published by the Oracle pipeline.
// Pure logic (no DOM, no globals) so it runs under node --test; the browser wiring is in
// live-data-client.ts. Any failure resolves to the embedded document with a reason:
// the website never runs a document it could not verify.

export const PUBLICATION_SCHEMA = 'forever-sim-publication-1';
export const OVERRIDES_VERSION = 'forever-overrides-1';
export const DEFAULT_LIVE_DATA_URL = 'https://api.exaltedcapital.com/forever/sim/';

/**
 * @typedef {{schema:string, overrides_sha256:string, overrides_source_hash:string, engine_commit:string, published_at:string,
 *   counts:{spells:number, items:number, talents:number, parameters:number}}} LiveManifest
 * @typedef {{status:'live', text:string, manifest:LiveManifest} | {status:'embedded', reason:string, manifest?:LiveManifest}} LiveData
 */

/**
 * Build-time URL: undefined/null means the default, an empty value disables live data.
 * @param {string|undefined|null} value
 * @returns {string} base URL ending in "/", or '' when disabled
 */
export function resolveLiveDataUrl(value) {
	if (value === undefined || value === null) return DEFAULT_LIVE_DATA_URL;
	const trimmed = String(value).trim();
	if (!trimmed) return '';
	const url = new URL(trimmed);
	const local = ['localhost', '127.0.0.1'].includes(url.hostname);
	if (url.protocol !== 'https:' && !(url.protocol === 'http:' && local)) throw new Error('FOREVER_LIVE_DATA_URL must be https (http is allowed for localhost only)');
	if (url.search || url.hash) throw new Error('FOREVER_LIVE_DATA_URL must not have a query or fragment');
	return url.href.endsWith('/') ? url.href : url.href + '/';
}

const SHA256 = /^[a-f0-9]{64}$/;

/** @param {any} m @returns {LiveManifest} */
export function validateManifest(m) {
	if (!m || typeof m !== 'object' || m.schema !== PUBLICATION_SCHEMA) throw new Error(`manifest schema is not ${PUBLICATION_SCHEMA}`);
	if (!SHA256.test(m.overrides_sha256 || '')) throw new Error('manifest overrides_sha256 is not a sha256 hex digest');
	if (typeof m.overrides_source_hash !== 'string' || (m.overrides_source_hash && !SHA256.test(m.overrides_source_hash))) throw new Error('manifest overrides_source_hash is invalid');
	if (typeof m.engine_commit !== 'string' || !/^[a-f0-9]{7,40}$/.test(m.engine_commit)) throw new Error('manifest engine_commit is not a git commit');
	if (typeof m.published_at !== 'string' || !Number.isFinite(Date.parse(m.published_at))) throw new Error('manifest published_at is not a date');
	for (const key of ['spells', 'items', 'talents', 'parameters']) {
		if (!Number.isInteger(m.counts?.[key]) || m.counts[key] < 0) throw new Error(`manifest counts.${key} is invalid`);
	}
	return m;
}

/** @param {ArrayBuffer|Uint8Array} bytes @param {SubtleCrypto} subtle */
export async function sha256Hex(bytes, subtle) {
	const digest = new Uint8Array(await subtle.digest('SHA-256', bytes));
	return Array.from(digest, b => b.toString(16).padStart(2, '0')).join('');
}

const message = error => (error instanceof Error ? error.message : String(error)) || 'unknown error';

async function fetchBytes(fetchFn, url, timeoutMs) {
	const controller = typeof AbortController === 'function' ? new AbortController() : null;
	const timer = controller ? setTimeout(() => controller.abort(), timeoutMs) : null;
	try {
		const response = await fetchFn(url, { cache: 'no-cache', credentials: 'omit', signal: controller?.signal });
		if (!response.ok) throw new Error(`${url.split('/').pop()} returned HTTP ${response.status}`);
		return await response.arrayBuffer();
	} catch (error) {
		throw new Error(controller?.signal.aborted ? `${url.split('/').pop()} timed out` : message(error));
	} finally {
		if (timer) clearTimeout(timer);
	}
}

async function attempt(baseUrl, fetchFn, subtle, timeoutMs) {
	const decoder = new TextDecoder('utf-8', { fatal: true });
	let manifest;
	try {
		manifest = JSON.parse(decoder.decode(await fetchBytes(fetchFn, baseUrl + 'manifest.json', timeoutMs)));
	} catch (error) {
		throw new Error(`manifest unavailable: ${message(error)}`);
	}
	validateManifest(manifest);
	const bytes = await fetchBytes(fetchFn, baseUrl + 'overrides.json', timeoutMs);
	const actual = await sha256Hex(bytes, subtle);
	if (actual !== manifest.overrides_sha256) {
		const error = new Error(`overrides.json hash mismatch (manifest ${manifest.overrides_sha256.slice(0, 12)}, received ${actual.slice(0, 12)})`);
		error.hashMismatch = true;
		error.manifest = manifest;
		throw error;
	}
	const text = decoder.decode(bytes);
	const doc = JSON.parse(text);
	if (doc?.version !== OVERRIDES_VERSION) throw new Error(`overrides.json version is not ${OVERRIDES_VERSION}`);
	if ((doc.source_hash || '') !== manifest.overrides_source_hash) throw new Error('overrides.json source_hash differs from the manifest');
	return { status: 'live', text, manifest };
}

/**
 * Fetch manifest.json + overrides.json and verify the document against the manifest hash.
 * Never rejects. A hash mismatch is retried once (the publisher replaces two files).
 * @param {{baseUrl:string, fetch:typeof fetch, subtle?:SubtleCrypto, timeoutMs?:number, retryDelayMs?:number}} options
 * @returns {Promise<LiveData>}
 */
export async function loadLiveOverrides({ baseUrl, fetch: fetchFn, subtle, timeoutMs = 10000, retryDelayMs = 1500 }) {
	if (!baseUrl) return { status: 'embedded', reason: 'live data is disabled in this build' };
	if (!subtle) return { status: 'embedded', reason: 'WebCrypto is unavailable, so the live document cannot be verified' };
	for (let tries = 0; ; tries++) {
		try {
			return await attempt(baseUrl, fetchFn, subtle, timeoutMs);
		} catch (error) {
			if (error?.hashMismatch && tries === 0) {
				await new Promise(resolve => setTimeout(resolve, retryDelayMs));
				continue;
			}
			return { status: 'embedded', reason: message(error), ...(error?.manifest ? { manifest: error.manifest } : {}) };
		}
	}
}

/**
 * Fold a worker's setForeverOverrides reply (JSON string {ok,error?,summary}) into the page state.
 * A rejected or different document means the engine kept the embedded document.
 * @param {LiveData} data @param {string} resultJson @returns {LiveData}
 */
export function applyWorkerResult(data, resultJson) {
	if (data.status !== 'live') return data;
	let result;
	try {
		result = JSON.parse(resultJson);
	} catch {
		return { status: 'embedded', reason: 'the engine returned an unreadable reply', manifest: data.manifest };
	}
	if (!result?.ok) return { status: 'embedded', reason: `the engine rejected the live document: ${result?.error || 'unknown error'}`, manifest: data.manifest };
	if ((result.summary?.source_hash || '') !== data.manifest.overrides_source_hash) return { status: 'embedded', reason: 'the engine reports a different document than the manifest', manifest: data.manifest };
	return data;
}

/** True when both commits are known and neither is a prefix of the other. */
export function engineCommitDiffers(manifestCommit, buildCommit) {
	const a = (manifestCommit || '').toLowerCase(), b = (buildCommit || '').toLowerCase();
	if (!/^[a-f0-9]{7,40}$/.test(a) || !/^[a-f0-9]{7,40}$/.test(b)) return false;
	return !(a.startsWith(b) || b.startsWith(a));
}

/**
 * Text for the release panel.
 * @param {LiveData} data @param {{upstreamCommit?:string}} [release]
 * @returns {{text:string, live:boolean, warning:string|null}}
 */
export function describeLiveData(data, release = {}) {
	const warning = data.manifest && engineCommitDiffers(data.manifest.engine_commit, release.upstreamCommit)
		? 'Simulator engine update pending; some newer mechanics may be missing.' : null;
	if (data.status !== 'live') return { text: `embedded (live data unavailable: ${data.reason})`, live: false, warning };
	const hash = data.manifest.overrides_source_hash || data.manifest.overrides_sha256;
	return { text: `live ${hash.slice(0, 12)}, published ${new Date(data.manifest.published_at).toISOString().slice(0, 16).replace('T', ' ')} UTC`, live: true, warning };
}

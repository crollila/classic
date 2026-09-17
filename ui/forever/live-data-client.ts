// Browser wiring for live Forever data (Forever builds only; see live-data.mjs for the logic).
// The main thread downloads and verifies the published overrides document once per page and
// hands the text to every sim worker before that worker accepts its first request.
import { applyWorkerResult, loadLiveOverrides } from './live-data.mjs';

export type LiveData = Awaited<ReturnType<typeof loadLiveOverrides>>;

let pending: Promise<LiveData> | undefined;
let current: LiveData | undefined;
const listeners = new Set<(data: LiveData) => void>();

/** Live document (or the reason the embedded one is used). Fetched once per page; never rejects. */
export function getLiveData(): Promise<LiveData> {
	pending ||= loadLiveOverrides({
		baseUrl: import.meta.env.VITE_FOREVER_LIVE_DATA_URL || '',
		fetch: (input, init) => fetch(input, init),
		subtle: globalThis.crypto?.subtle,
	}).then(data => {
		if (data.status !== 'live') console.warn(`Forever data: embedded (live data unavailable: ${data.reason})`);
		return (current = data);
	});
	// A worker may have rejected the document after the download succeeded.
	return pending.then(data => current || data);
}

/** Record a worker's setForeverOverrides reply; returns the resulting page state. */
export function reportWorkerResult(resultJson: string): LiveData | undefined {
	if (!current) return current;
	const next = applyWorkerResult(current, resultJson);
	if (next !== current) {
		current = next;
		if (next.status !== 'live') console.warn(`Forever data: embedded (live data unavailable: ${next.reason})`);
		listeners.forEach(listener => listener(next));
	}
	return current;
}

/** Calls listener with the state once known and again whenever it changes. */
export function watchLiveData(listener: (data: LiveData) => void) {
	listeners.add(listener);
	void getLiveData().then(listener);
}

import { WorkerInterface } from './worker_interface';
import { ungzip } from 'pako';

type SimRequestAsync = (data: Uint8Array, progress: (result: Uint8Array) => void, id: string) => Uint8Array;
type SimRequestSync = (data: Uint8Array) => Uint8Array;

// Functions provided or used by the wasm lib.
declare global {
	function wasmready(): void;
	const bulkSimAsync: SimRequestAsync;
	const bulkSimCombos: SimRequestSync;
	const computeStats: SimRequestSync;
	const computeStatsJson: SimRequestSync;
	const raidSim: SimRequestSync;
	const raidSimJson: SimRequestSync;
	const raidSimAsync: SimRequestAsync;
	const statWeights: SimRequestSync;
	const statWeightsAsync: SimRequestAsync;
	const statWeightRequests: SimRequestSync;
	const statWeightCompute: SimRequestSync;
	const raidSimResultCombination: SimRequestSync;
	const raidSimRequestSplit: SimRequestSync;
	const abortById: SimRequestSync;
	const setForeverOverrides: ((json: string) => string) | undefined;
	const foreverOverridesInfo: (() => string) | undefined;
}

// Wasm binary calls this function when its done loading.
// eslint-disable-next-line @typescript-eslint/no-unused-vars
globalThis.wasmready = function () {
	new WorkerInterface({
		bulkSimAsync: bulkSimAsync,
		//bulkSimCombos: bulkSimCombos,
		computeStats: computeStats,
		computeStatsJson: computeStatsJson,
		raidSim: raidSim,
		raidSimJson: raidSimJson,
		raidSimAsync: raidSimAsync,
		statWeights: statWeights,
		statWeightsAsync: statWeightsAsync,
		statWeightRequests: statWeightRequests,
		statWeightCompute: statWeightCompute,
		raidSimRequestSplit: raidSimRequestSplit,
		raidSimResultCombination: raidSimResultCombination,
		abortById: abortById,
	}, typeof setForeverOverrides === 'function' ? setForeverOverrides : undefined).ready(true);
};

const go = new Go();
let inst: WebAssembly.Instance | null = null;

const instantiate = import.meta.env.VITE_FOREVER
	? fetch(`lib.wasm.gz${self.location.search}`).then(r=>{if(!r.ok)throw new Error(`Forever engine download failed: ${r.status}`);return r.arrayBuffer()}).then(bytes=>WebAssembly.instantiate(ungzip(new Uint8Array(bytes)),go.importObject))
	: WebAssembly.instantiateStreaming(fetch('lib.wasm'), go.importObject);
instantiate.then(async result => {
	inst = result.instance;
	// console.log("loading wasm...")
	await go.run(inst);
});

export {};

import type { SimRequest, WorkerReceiveMessage, WorkerSendMessage } from './types';

export type HandlerProgressCallback = (outputData: Uint8Array) => void;
export type HandlerFunction = (data: Uint8Array, progress: HandlerProgressCallback, id: string, msg: SimRequest) => Uint8Array | Promise<Uint8Array>;
export type Handlers = Record<SimRequest, HandlerFunction>;

/**
 * Communication with the UI.
 */
export class WorkerInterface {
	private _workerId = '';
	private readonly handlers: Handlers;

	/** @param setForeverOverrides wasm export of Forever builds; returns a JSON string {ok, error?, summary}. */
	constructor(handlers: Handlers, setForeverOverrides?: (json: string) => string) {
		this.handlers = handlers;

		addEventListener('message', async ({ data }: MessageEvent<WorkerReceiveMessage>) => {
			if (data.msg === 'setForeverOverrides') {
				let result: string;
				try {
					if (!setForeverOverrides) throw new Error('this engine cannot load live Forever data');
					result = setForeverOverrides(data.text);
				} catch (error) {
					result = JSON.stringify({ ok: false, error: error instanceof Error ? error.message : String(error) });
				}
				this.postMessage({ msg: 'foreverOverrides', id: data.id, result });
				return;
			}

			const { id, msg, inputData } = data;

			if (msg === 'setID') {
				this._workerId = id;
				this.postMessage({ msg: 'idConfirm' });
				return;
			}

			const handlerFunc = this.handlers?.[msg];

			if (!handlerFunc) {
				console.error(`Request msg: ${msg}, id: ${id}, is not handled!`);
				return;
			}

			const progressCallback: HandlerProgressCallback = prog => {
				this.postMessage({
					msg: 'progress',
					id: `${id}progress`,
					outputData: prog,
				});
			};

			const outputData = await handlerFunc(inputData, progressCallback, id, msg);
			this.postMessage({ msg, id, outputData });
		});
	}

	private postMessage(m: WorkerSendMessage) {
		postMessage(m);
	}

	get workerId() {
		return this._workerId;
	}

	/**
	 * Tell UI that the worker is ready.
	 * @param isWasm true if worker is using wasm.
	 */
	ready(isWasm: boolean) {
		this.postMessage({ msg: 'ready', outputData: new Uint8Array([+isWasm]) });
	}
}

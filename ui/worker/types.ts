/**
 * API endpoints and exposed wasm function names. Also used as request identifier.
 */
export enum SimRequest {
	bulkSimAsync = 'bulkSimAsync',
	//bulkSimCombos = 'bulkSimCombos',
	computeStats = 'computeStats',
	computeStatsJson = 'computeStatsJson',
	raidSim = 'raidSim',
	raidSimJson = 'raidSimJson',
	raidSimAsync = 'raidSimAsync',
	statWeights = 'statWeights',
	statWeightsAsync = 'statWeightsAsync',
	statWeightRequests = 'statWeightRequests',
	statWeightCompute = 'statWeightCompute',
	raidSimRequestSplit = 'raidSimRequestSplit',
	raidSimResultCombination = 'raidSimResultCombination',
	abortById = 'abortById',
}

/**
 * What the Worker receives from the UI
 */
export type WorkerReceiveMessageType = keyof typeof SimRequest | 'setID' | 'setForeverOverrides';

export interface WorkerReceiveMessageBodyBase {
	id: string;
	msg: WorkerReceiveMessageType;
	inputData?: Uint8Array;
}

export interface WorkerReceiveMessageSetId extends WorkerReceiveMessageBodyBase {
	msg: 'setID';
}

export interface WorkerReceiveMessageSimRequest extends Required<WorkerReceiveMessageBodyBase> {
	msg: SimRequest;
}

/** Forever builds only: the verified live overrides document, sent before the first sim request. */
export interface WorkerReceiveMessageSetForeverOverrides extends WorkerReceiveMessageBodyBase {
	msg: 'setForeverOverrides';
	text: string;
}

export type WorkerReceiveMessage = WorkerReceiveMessageSetId | WorkerReceiveMessageSetForeverOverrides | WorkerReceiveMessageSimRequest;

/**
 * What the Worker sends to the UI
 */
export type WorkerSendMessageType = 'ready' | 'idConfirm' | 'progress' | 'foreverOverrides' | keyof typeof SimRequest;

export interface WorkerSendMessageBodyBase {
	id?: string;
	msg: WorkerSendMessageType;
	outputData?: Uint8Array;
}

export interface WorkerSendMessageIdConfirm extends WorkerSendMessageBodyBase {
	msg: 'idConfirm';
}

export interface WorkerSendMessageReady extends WorkerSendMessageBodyBase {
	msg: 'ready';
}

export interface WorkerSendMessageProgress extends Required<WorkerSendMessageBodyBase> {
	msg: 'progress';
}

export interface WorkerSendMessageSimRequest extends Required<WorkerSendMessageBodyBase>, Required<Omit<WorkerReceiveMessageSimRequest, 'inputData'>> {
	msg: SimRequest;
}

/** Reply to setForeverOverrides: the engine's JSON string {ok, error?, summary}. */
export interface WorkerSendMessageForeverOverrides extends WorkerSendMessageBodyBase {
	msg: 'foreverOverrides';
	result: string;
}

export type WorkerSendMessage =
	| WorkerSendMessageReady
	| WorkerSendMessageIdConfirm
	| WorkerSendMessageProgress
	| WorkerSendMessageForeverOverrides
	| WorkerSendMessageSimRequest;

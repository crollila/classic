import { Player, ForeverOptions } from '../core/proto/api';
import data from './data/talents.json';

// UI consumers must display rank.estimated and rank.confidence from this data.
// Positions are one-based and record IDs are stable research keys, not spell IDs.
export const foreverDiscoveryTalents = data;

// Explicit conversion for clients constructing raw worker/API requests. The
// existing Classic talent picker continues to construct Classic requests.
// Validation of supported ranks, points and prerequisites is authoritative in Go.
export function withForeverDiscovery(player: Player, options: Omit<ForeverOptions, 'rulesetId'>): Player {
	const result = Player.clone(player);
	result.talentsString = '';
	result.forever = ForeverOptions.create({ ...options, rulesetId: data.ruleset_id });
	return result;
}

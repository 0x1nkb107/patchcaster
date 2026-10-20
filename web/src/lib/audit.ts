// How the interface reads and words the audit log.
import type { AuditEntry } from '#lib/api/generated/client.js';

// auditPageSize is how many entries the log reads at a time.
export const auditPageSize = 50;

// actorOf words who made a change, and with which API token.
export function actorOf(entry: AuditEntry): string {
	if (entry.actor_token === undefined) {
		return entry.actor;
	}
	return `${entry.actor}, with the token ${entry.actor_token}`;
}

// hasState reports whether a change recorded the target before or after
// it.
export function hasState(entry: AuditEntry): boolean {
	return isRecorded(entry.before) || isRecorded(entry.after);
}

// isRecorded reports whether a state of the target was recorded.
function isRecorded(state: unknown): boolean {
	return state !== undefined && state !== null;
}

// stateOf shows the target before or after a change as indented JSON, or
// says that there was none, as before a creation.
export function stateOf(state: unknown): string {
	return isRecorded(state) ? JSON.stringify(state, null, 2) : 'nothing';
}

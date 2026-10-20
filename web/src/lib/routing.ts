// How the interface words the routing configuration: what a source
// selects, and how its events reach PatchCaster.
import type { SourceHook, SourceScope } from '#lib/api/generated/client.js';

// scopeOf words what a source selects.
export function scopeOf(scope: SourceScope): string {
	switch (scope.kind) {
		case 'instance':
			return 'the whole instance';
		case 'group':
			return `the group ${scope.path ?? ''}`;
		case 'repository':
			return `the repository ${scope.path ?? ''}`;
	}
}

// hookOf words how a source's events reach PatchCaster.
export function hookOf(hook: SourceHook): string {
	switch (hook.state) {
		case 'registered':
			return 'PatchCaster registered the hook.';
		case 'pending':
			return 'PatchCaster is about to register the hook.';
		case 'failed':
			return `Registering the hook failed${hook.last_error ? `: ${hook.last_error}` : '.'} PatchCaster tries again every five minutes.`;
		case 'covered':
			return `The hook of ${hook.covered_by ? scopeOf(hook.covered_by) : 'a broader source'} takes the events.`;
		case 'manual':
			return "Register a hook by hand at the connection's webhook address.";
	}
}

// namesById maps the id of each item to its name.
export function namesById(items: { id: number; name: string }[]): Map<number, string> {
	return new Map(items.map((item) => [item.id, item.name]));
}

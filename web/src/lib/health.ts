// How the interface words the health that the API reports (SYSREQ-064).
import type { ConnectionHealth, ConnectionKind, SinkHealth } from '#lib/api/generated/client.js';

// platformNames names each kind of connection as its platform names itself.
export const platformNames: Record<ConnectionKind, string> = {
	gitlab: 'GitLab',
	mattermost: 'Mattermost'
};

// Delivery is where a sink or chat connection stands: failing while its
// latest attempt failed, delivering once anything got through, idle before
// any attempt.
export type Delivery = 'failing' | 'delivering' | 'idle';

// deliveryOf compares the last failure with the last success.
export function deliveryOf(health: SinkHealth | ConnectionHealth): Delivery {
	const failed = health.last_failure_at === undefined ? 0 : Date.parse(health.last_failure_at);
	const succeeded = health.last_success_at === undefined ? 0 : Date.parse(health.last_success_at);
	if (failed > succeeded) {
		return 'failing';
	}
	return succeeded > 0 ? 'delivering' : 'idle';
}

// rejectionOf words why a forge connection's delivery was rejected.
export function rejectionOf(cause: ConnectionHealth['last_rejection']): string {
	switch (cause) {
		case 'signature':
			return 'its signature did not match any signing token';
		case 'parse':
			return 'its payload could not be read';
		default:
			return '';
	}
}

// moment words a time from the API, or never.
export function moment(timestamp: string | undefined): string {
	return timestamp === undefined ? 'never' : new Date(timestamp).toLocaleString();
}

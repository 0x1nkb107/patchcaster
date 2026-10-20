import { redirect } from '@sveltejs/kit';

import { resolve } from '$app/paths';
import { listConnections, listSinks } from '#lib/api/generated/client.js';
import { everyItem } from '#lib/api/pages.js';

import type { PageLoad } from './$types';

// The sinks, and the connections they belong to. Only Instance Admins
// list connections, and the API refuses anyone else.
export const load: PageLoad = async ({ parent }) => {
	const { person } = await parent();
	if (person === null) {
		redirect(307, resolve('/'));
	}
	const [sinks, connections] = await Promise.all([
		everyItem((cursor) => listSinks({ limit: 200, cursor })),
		everyItem((cursor) => listConnections({ limit: 200, cursor }))
	]);
	return { sinks, connections };
};

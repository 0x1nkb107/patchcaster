import { redirect } from '@sveltejs/kit';

import { resolve } from '$app/paths';
import { listConnections, listSinks } from '#lib/api/generated/client.js';
import { everyItem } from '#lib/api/pages.js';

import type { PageLoad } from './$types';

// The health of what the person may see (SYSREQ-064): the sinks of their
// teams, and for an administrator every sink and connection.
export const load: PageLoad = async ({ parent }) => {
	const { person } = await parent();
	if (person === null) {
		redirect(307, resolve('/'));
	}
	const [sinks, connections] = await Promise.all([
		everyItem((cursor) => listSinks({ limit: 200, cursor })),
		person.administrator
			? everyItem((cursor) => listConnections({ limit: 200, cursor }))
			: Promise.resolve([])
	]);
	return { sinks, connections };
};

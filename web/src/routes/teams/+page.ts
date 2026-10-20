import { redirect } from '@sveltejs/kit';

import { resolve } from '$app/paths';
import { listTeams } from '#lib/api/generated/client.js';
import { everyItem } from '#lib/api/pages.js';

import type { PageLoad } from './$types';

// The teams the person may see: every team for an Instance Admin, the
// teams they manage for a Routing Manager.
export const load: PageLoad = async ({ parent }) => {
	const { person } = await parent();
	if (person === null) {
		redirect(307, resolve('/'));
	}
	return { teams: await everyItem((cursor) => listTeams({ limit: 200, cursor })) };
};

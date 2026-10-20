import { redirect } from '@sveltejs/kit';

import { resolve } from '$app/paths';
import { listPersons, listTeams } from '#lib/api/generated/client.js';
import { everyItem } from '#lib/api/pages.js';

import type { PageLoad } from './$types';

// The people who logged in, and the teams that tell who manages one. Only
// Instance Admins list people, and the API refuses anyone else.
export const load: PageLoad = async ({ parent }) => {
	const { person } = await parent();
	if (person === null) {
		redirect(307, resolve('/'));
	}
	const [persons, teams] = await Promise.all([
		everyItem((cursor) => listPersons({ limit: 200, cursor })),
		everyItem((cursor) => listTeams({ limit: 200, cursor }))
	]);
	return { persons, teams };
};

import { error, redirect } from '@sveltejs/kit';

import { resolve } from '$app/paths';
import { getTeam, listPersons, listSinks, listSources } from '#lib/api/generated/client.js';
import { everyItem } from '#lib/api/pages.js';
import { describe } from '#lib/api/problem.js';

import type { PageLoad } from './$types';

// A team, with the people, sources and sinks it may be given. Only
// Instance Admins list people, and the API refuses anyone else.
export const load: PageLoad = async ({ parent, params }) => {
	const { person } = await parent();
	if (person === null) {
		redirect(307, resolve('/'));
	}
	const id = Number(params.id);
	if (!Number.isSafeInteger(id)) {
		error(404, 'No team has this address');
	}
	const [answer, persons, sources, sinks] = await Promise.all([
		getTeam(id),
		everyItem((cursor) => listPersons({ limit: 200, cursor })),
		everyItem((cursor) => listSources({ limit: 200, cursor })),
		everyItem((cursor) => listSinks({ limit: 200, cursor }))
	]);
	if (answer.status !== 200) {
		error(answer.status, describe(answer.data));
	}
	return { team: answer.data, persons, sources, sinks };
};

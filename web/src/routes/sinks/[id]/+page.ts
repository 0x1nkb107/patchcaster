import { error, redirect } from '@sveltejs/kit';

import { resolve } from '$app/paths';
import { getSink, listTeams } from '#lib/api/generated/client.js';
import { everyItem } from '#lib/api/pages.js';
import { describe } from '#lib/api/problem.js';

import type { PageLoad } from './$types';

// A sink, with the teams it belongs to.
export const load: PageLoad = async ({ parent, params }) => {
	const { person } = await parent();
	if (person === null) {
		redirect(307, resolve('/'));
	}
	const id = Number(params.id);
	if (!Number.isSafeInteger(id)) {
		error(404, 'No sink has this address');
	}
	const [answer, teams] = await Promise.all([
		getSink(id),
		everyItem((cursor) => listTeams({ limit: 200, cursor }))
	]);
	if (answer.status !== 200) {
		error(answer.status, describe(answer.data));
	}
	return { sink: answer.data, teams };
};

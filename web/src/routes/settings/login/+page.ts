import { error, redirect } from '@sveltejs/kit';

import { resolve } from '$app/paths';
import { getLoginSettings } from '#lib/api/generated/client.js';
import { describe } from '#lib/api/problem.js';
import { etagOf } from '#lib/api/versions.js';

import type { PageLoad } from './$types';

// The identity provider settings, with the version a change must name.
// Only Instance Admins read them; the API refuses anyone else.
export const load: PageLoad = async ({ parent }) => {
	const { person } = await parent();
	if (person === null) {
		redirect(307, resolve('/'));
	}
	const answer = await getLoginSettings();
	if (answer.status !== 200) {
		error(answer.status, describe(answer.data));
	}
	return { settings: answer.data, etag: etagOf(answer.headers) };
};

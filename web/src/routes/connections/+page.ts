import { redirect } from '@sveltejs/kit';

import { resolve } from '$app/paths';
import { listConnections } from '#lib/api/generated/client.js';
import { everyItem } from '#lib/api/pages.js';

import type { PageLoad } from './$types';

// Only Instance Admins manage connections. The API refuses anyone else,
// and the error page then says why.
export const load: PageLoad = async ({ parent }) => {
	const { person } = await parent();
	if (person === null) {
		redirect(307, resolve('/'));
	}
	return { connections: await everyItem((cursor) => listConnections({ limit: 200, cursor })) };
};

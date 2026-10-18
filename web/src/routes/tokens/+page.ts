import { redirect } from '@sveltejs/kit';

import { resolve } from '$app/paths';
import { listTokens } from '#lib/api/generated/client.js';
import { everyItem } from '#lib/api/pages.js';

import type { PageLoad } from './$types';

// Only a login manages its tokens (ADR-033), so the page needs one.
export const load: PageLoad = async ({ parent }) => {
	const { person } = await parent();
	if (person === null) {
		redirect(307, resolve('/'));
	}
	return { tokens: await everyItem((cursor) => listTokens({ limit: 200, cursor })) };
};

import { roleOf } from '#lib/roles.js';

import type { PageLoad } from './$types';

// The start page names the role of whoever is logged in.
export const load: PageLoad = async ({ parent }) => {
	const { person } = await parent();
	if (person === null) {
		return { role: null };
	}
	return { role: await roleOf(person) };
};

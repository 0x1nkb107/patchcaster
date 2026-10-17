import { error } from '@sveltejs/kit';

import { getInstance, getSession } from '#lib/api/generated/client.js';
import { describe } from '#lib/api/problem.js';

import type { LayoutLoad } from './$types';

// A single-page application: nothing renders on a server, and no page is
// prerendered, because every page depends on the session (ADR-032).
export const ssr = false;
export const prerender = false;

// Every page knows the instance and whoever is logged in. Without a
// session, the API answers 401 and person stays null.
export const load: LayoutLoad = async () => {
	const [instance, session] = await Promise.all([getInstance(), getSession()]);
	if (instance.status !== 200) {
		error(instance.status, describe(instance.data));
	}
	if (session.status === 401) {
		return { instance: instance.data, person: null };
	}
	if (session.status !== 200) {
		error(session.status, describe(session.data));
	}
	return { instance: instance.data, person: session.data };
};

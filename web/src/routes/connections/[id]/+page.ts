import { error, redirect } from '@sveltejs/kit';

import { resolve } from '$app/paths';
import { getConnection, listSigningTokens } from '#lib/api/generated/client.js';
import { everyItem } from '#lib/api/pages.js';
import { describe } from '#lib/api/problem.js';

import type { PageLoad } from './$types';

// A connection with its signing tokens, which only a forge has.
export const load: PageLoad = async ({ parent, params }) => {
	const { person } = await parent();
	if (person === null) {
		redirect(307, resolve('/'));
	}
	const id = Number(params.id);
	if (!Number.isSafeInteger(id)) {
		error(404, 'No connection has this address');
	}
	const answer = await getConnection(id);
	if (answer.status !== 200) {
		error(answer.status, describe(answer.data));
	}
	const connection = answer.data;
	const isForge = connection.webhook_url !== undefined;
	const signingTokens = isForge
		? await everyItem((cursor) => listSigningTokens(id, { limit: 200, cursor }))
		: [];
	return { connection, signingTokens };
};

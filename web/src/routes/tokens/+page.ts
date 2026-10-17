import { error, redirect } from '@sveltejs/kit';

import { resolve } from '$app/paths';
import { listTokens, type ApiToken } from '#lib/api/generated/client.js';
import { describe } from '#lib/api/problem.js';

import type { PageLoad } from './$types';

// Only a login manages its tokens (ADR-033), so the page needs one.
export const load: PageLoad = async ({ parent }) => {
	const { person } = await parent();
	if (person === null) {
		redirect(307, resolve('/'));
	}
	return { tokens: await everyToken() };
};

// everyToken reads the person's tokens page by page, up to the last one.
async function everyToken(): Promise<ApiToken[]> {
	const tokens: ApiToken[] = [];
	let cursor: string | undefined;
	do {
		const answer = await listTokens({ limit: 200, cursor });
		if (answer.status !== 200) {
			error(answer.status, describe(answer.data));
		}
		tokens.push(...answer.data.items);
		cursor = answer.data.next_cursor;
	} while (cursor !== undefined);
	return tokens;
}

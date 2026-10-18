import { error } from '@sveltejs/kit';

import type { HTTPStatusCodes, Problem } from '#lib/api/generated/client.js';
import { describe } from '#lib/api/problem.js';

// Page is one page of a list the API answers.
interface Page<T> {
	items: T[];
	next_cursor?: string;
}

// PageAnswer is how the generated client answers a list request: with a
// page, or with a problem.
type PageAnswer<T> =
	{ status: 200; data: Page<T> } | { status: Exclude<HTTPStatusCodes, 200>; data: Problem };

// everyItem reads a list page by page, up to the last page. A problem ends
// the load, and the error page shows it.
export async function everyItem<T>(
	readPage: (cursor: string | undefined) => Promise<PageAnswer<T>>
): Promise<T[]> {
	const items: T[] = [];
	let cursor: string | undefined;
	do {
		const answer = await readPage(cursor);
		if (answer.status !== 200) {
			error(answer.status, describe(answer.data));
		}
		items.push(...answer.data.items);
		cursor = answer.data.next_cursor;
	} while (cursor !== undefined);
	return items;
}

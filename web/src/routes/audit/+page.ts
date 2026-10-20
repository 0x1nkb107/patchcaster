import { error, redirect } from '@sveltejs/kit';

import { resolve } from '$app/paths';
import { listAudit } from '#lib/api/generated/client.js';
import { describe } from '#lib/api/problem.js';
import { auditPageSize } from '#lib/audit.js';

import type { PageLoad } from './$types';

// The newest entries of the audit log. Only Instance Admins read it, and
// the API refuses anyone else.
export const load: PageLoad = async ({ parent }) => {
	const { person } = await parent();
	if (person === null) {
		redirect(307, resolve('/'));
	}
	const answer = await listAudit({ limit: auditPageSize });
	if (answer.status !== 200) {
		error(answer.status, describe(answer.data));
	}
	return { entries: answer.data.items, next: answer.data.next_cursor };
};

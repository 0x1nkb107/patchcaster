// Forms submit to the API and show what it answers. The API reports a
// refusal as an RFC 9457 problem whose field errors name each field by the
// path form libraries use (ADR-043), so one helper serves every form.
import { setError, type FormPathLeavesWithErrors, type SuperValidated } from 'sveltekit-superforms';

import type { Problem } from '#lib/api/generated/client.js';
import { describe } from '#lib/api/problem.js';

// withPath names the field a message is about. The API words a field's
// messages to follow its path, such as "is required".
function withPath(path: string, message: string): string {
	return path === '' ? message : `${path} ${message}`;
}

// hasField reports whether the form's data holds the field that path
// starts with.
function hasField(form: SuperValidated<Record<string, unknown>, string>, path: string): boolean {
	const name = path.split(/[.[]/, 1)[0] ?? '';
	return name !== '' && Object.hasOwn(form.data, name);
}

// showProblem attaches a problem to a form: each field error to the field
// its path names, and the problem's detail as the form's message. A form
// whose fields differ from the request body translates the paths with
// fieldOf. Messages for a path that names no field of the form join the
// form's message, so that none goes unseen.
export function showProblem<T extends Record<string, unknown>>(
	form: SuperValidated<T, string>,
	problem: Problem,
	fieldOf: (path: string) => string = (path) => path
): void {
	const unplaced: string[] = [];
	for (const error of problem.errors ?? []) {
		const field = error.path === '' ? '' : fieldOf(error.path);
		if (!hasField(form, field)) {
			unplaced.push(...error.messages.map((message) => withPath(error.path, message)));
			continue;
		}
		setError(form, field as FormPathLeavesWithErrors<T>, error.messages);
	}
	const detail = describe(problem);
	form.message = unplaced.length === 0 ? detail : `${detail}: ${unplaced.join(', ')}`;
}

// unlessEmpty leaves out a secret field left empty. The API never shows a
// stored secret, and it keeps the stored one when a change leaves it out.
export function unlessEmpty(value: string): string | undefined {
	return value === '' ? undefined : value;
}

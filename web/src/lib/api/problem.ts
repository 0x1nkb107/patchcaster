import type { Problem } from '#lib/api/generated/client.js';

// describe returns what a problem tells a person: its detail, or its title
// when it has none. The API words details as Go words errors, in lower
// case, so the first letter is raised for display.
export function describe(problem: Problem): string {
	const text = problem.detail ?? problem.title;
	return text.charAt(0).toUpperCase() + text.slice(1);
}

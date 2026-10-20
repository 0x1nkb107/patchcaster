// How the interface names the role of a person (SYSREQ-039).
import { error } from '@sveltejs/kit';

import { listTeams, type Person, type Team } from '#lib/api/generated/client.js';
import { describe } from '#lib/api/problem.js';

// Role is a person's role by its code name.
export type Role = 'administrator' | 'manager' | 'regular';

// roleNames names each role as people read it.
export const roleNames: Record<Role, string> = {
	administrator: 'Instance Admin',
	manager: 'Routing Manager',
	regular: 'Regular User'
};

// roleOf finds the role of a person. An Instance Admin carries a flag.
// Anyone else sees only the teams they manage, so one listed team makes
// them a Routing Manager. A problem ends the load, and the error page
// shows it.
export async function roleOf(person: Person): Promise<Role> {
	if (person.administrator) {
		return 'administrator';
	}
	const teams = await listTeams({ limit: 1 });
	if (teams.status !== 200) {
		error(teams.status, describe(teams.data));
	}
	return teams.data.items.length > 0 ? 'manager' : 'regular';
}

// roleAmong finds the role of a person from every team, as an Instance
// Admin sees them: a person a team names among its managers is a Routing
// Manager.
export function roleAmong(person: Person, teams: Team[]): Role {
	if (person.administrator) {
		return 'administrator';
	}
	const manages = teams.some((team) => team.managers.includes(person.id));
	return manages ? 'manager' : 'regular';
}

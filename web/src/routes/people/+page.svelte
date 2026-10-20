<script lang="ts">
	import { refreshAll } from '$app/navigation';
	import { revokeAccess, updatePerson, type Person } from '#lib/api/generated/client.js';
	import { describe } from '#lib/api/problem.js';
	import { ifMatch } from '#lib/api/versions.js';
	import ConfirmButton from '#lib/components/confirm-button.svelte';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { roleAmong, roleNames } from '#lib/roles.js';

	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// What the last action did, or why it failed.
	let outcome: { failed: boolean; text: string } | null = $state(null);

	// nameOf names a person the way the identity provider does.
	function nameOf(person: Person): string {
		return person.display_name === '' ? person.subject : person.display_name;
	}

	// tokensOf words how many tokens a revocation revoked.
	function tokensOf(count: number): string {
		return count === 1 ? '1 token' : `${String(count)} tokens`;
	}

	async function setAdministrator(person: Person, administrator: boolean): Promise<void> {
		const answer = await updatePerson(person.id, { administrator }, ifMatch(person.etag));
		if (answer.status !== 200) {
			outcome = { failed: true, text: describe(answer.data) };
			return;
		}
		// The message waits for the reload, so that it never contradicts the
		// table.
		await refreshAll();
		const now = administrator ? 'an Instance Admin now' : 'no longer an Instance Admin';
		outcome = { failed: false, text: `${nameOf(person)} is ${now}.` };
	}

	async function revoke(person: Person): Promise<void> {
		const answer = await revokeAccess(person.id);
		if (answer.status !== 200) {
			outcome = { failed: true, text: describe(answer.data) };
			return;
		}
		await refreshAll();
		const revoked = tokensOf(answer.data.revoked_tokens);
		outcome = {
			failed: false,
			text: `PatchCaster revoked ${revoked} of ${nameOf(person)} and ended their sessions.`
		};
	}
</script>

<svelte:head>
	<title>People · PatchCaster</title>
</svelte:head>

<Card.Root>
	<Card.Header>
		<Card.Title>People</Card.Title>
		<Card.Description>
			Everyone who logged in at least once. Instance Admins configure the instance and decide who
			may do what. A team's page names its Routing Managers.
		</Card.Description>
	</Card.Header>
	<Card.Content class="grid gap-6">
		{#if outcome}
			<Alert.Root variant={outcome.failed ? 'destructive' : 'default'}>
				<Alert.Description>{outcome.text}</Alert.Description>
			</Alert.Root>
		{/if}
		<table class="w-full text-sm">
			<thead class="text-left text-muted-foreground">
				<tr>
					<th class="py-2 pr-4 font-medium">Name</th>
					<th class="py-2 pr-4 font-medium">Role</th>
					<th></th>
				</tr>
			</thead>
			<tbody>
				{#each data.persons as person (person.id)}
					<tr class="border-t align-top">
						<td class="py-2 pr-4">
							{nameOf(person)}
							<span class="block text-muted-foreground">
								{person.email === '' ? person.subject : person.email}
							</span>
						</td>
						<td class="py-2 pr-4">
							<Badge variant="secondary">{roleNames[roleAmong(person, data.teams)]}</Badge>
						</td>
						<td class="py-2">
							<div class="flex flex-wrap justify-end gap-2">
								{#if person.administrator}
									<ConfirmButton
										label="Remove Instance Admin"
										question="Take the Instance Admin role from {nameOf(person)}?"
										consequence="They keep the teams they manage and their own tokens. The instance needs at least one Instance Admin."
										variant="outline"
										size="sm"
										onConfirm={() => setAdministrator(person, false)}
									/>
								{:else}
									<ConfirmButton
										label="Make Instance Admin"
										question="Make {nameOf(person)} an Instance Admin?"
										consequence="They can then change the whole configuration, the login and everyone's roles."
										variant="outline"
										size="sm"
										onConfirm={() => setAdministrator(person, true)}
									/>
								{/if}
								<ConfirmButton
									label="Revoke access"
									question="Revoke the access of {nameOf(person)}?"
									consequence="PatchCaster revokes all their API tokens and ends all their sessions at once. They can log in again unless the identity provider refuses them, so remove them there too."
									variant="destructive"
									size="sm"
									onConfirm={() => revoke(person)}
								/>
							</div>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</Card.Content>
</Card.Root>

<script lang="ts">
	import { defaults, superForm } from 'sveltekit-superforms';
	import { zod4, zod4Client } from 'sveltekit-superforms/adapters';

	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { createTeam } from '#lib/api/generated/client.js';
	import { CreateTeamBody } from '#lib/api/generated/schemas.js';
	import FormField from '#lib/components/form-field.svelte';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { showProblem } from '#lib/forms.js';

	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const form = superForm(defaults(zod4(CreateTeamBody)), {
		SPA: true,
		validators: zod4Client(CreateTeamBody),
		resetForm: false,
		async onUpdate({ form }) {
			if (!form.valid) {
				return;
			}
			const answer = await createTeam(form.data);
			if (answer.status !== 201) {
				showProblem(form, answer.data);
				return;
			}
			await goto(resolve('/teams/[id]', { id: String(answer.data.id) }));
		}
	});
	const { form: fields, errors, message, enhance, submitting } = form;

	// countOf words a count with its noun.
	function countOf(n: number, noun: string): string {
		return n === 1 ? `1 ${noun}` : `${String(n)} ${noun}s`;
	}
</script>

<svelte:head>
	<title>Teams · PatchCaster</title>
</svelte:head>

<div class="grid gap-6">
	<Card.Root>
		<Card.Header>
			<Card.Title>Create a team</Card.Title>
			<Card.Description>
				A team's Routing Managers route its sources into its sinks. Give the team its managers,
				sources and sinks on its page.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			<form method="POST" use:enhance class="grid gap-6 sm:grid-cols-[1fr_auto] sm:items-end">
				{#if $message}
					<Alert.Root variant="destructive" class="sm:col-span-2">
						<Alert.Description>{$message}</Alert.Description>
					</Alert.Root>
				{/if}
				<FormField id="name" label="Name" errors={$errors.name}>
					<Input
						id="name"
						name="name"
						placeholder="Payments"
						bind:value={$fields.name}
						aria-invalid={$errors.name ? 'true' : undefined}
					/>
				</FormField>
				<Button type="submit" disabled={$submitting}>Create</Button>
			</form>
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header>
			<Card.Title>Teams</Card.Title>
		</Card.Header>
		<Card.Content>
			{#if data.teams.length === 0}
				<p class="text-sm text-muted-foreground">There are no teams yet.</p>
			{:else}
				<table class="w-full text-sm">
					<thead class="text-left text-muted-foreground">
						<tr>
							<th class="py-2 pr-4 font-medium">Name</th>
							<th class="py-2 pr-4 font-medium">Routing Managers</th>
							<th class="py-2 pr-4 font-medium">Sources</th>
							<th class="py-2 font-medium">Sinks</th>
						</tr>
					</thead>
					<tbody>
						{#each data.teams as team (team.id)}
							<tr class="border-t">
								<td class="py-2 pr-4">
									<a
										href={resolve('/teams/[id]', { id: String(team.id) })}
										class="font-medium underline-offset-4 hover:underline"
									>
										{team.name}
									</a>
								</td>
								<td class="py-2 pr-4">{countOf(team.managers.length, 'manager')}</td>
								<td class="py-2 pr-4">{countOf(team.sources.length, 'source')}</td>
								<td class="py-2">{countOf(team.sinks.length, 'sink')}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
		</Card.Content>
	</Card.Root>
</div>

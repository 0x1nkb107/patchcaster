<script lang="ts">
	import { untrack } from 'svelte';
	import { defaults, superForm } from 'sveltekit-superforms';
	import { zod4, zod4Client } from 'sveltekit-superforms/adapters';

	import { goto, refreshAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { deleteTeam, updateTeam } from '#lib/api/generated/client.js';
	import { UpdateTeamBody } from '#lib/api/generated/schemas.js';
	import { describe } from '#lib/api/problem.js';
	import { ifMatch } from '#lib/api/versions.js';
	import CheckboxList from '#lib/components/checkbox-list.svelte';
	import FormField from '#lib/components/form-field.svelte';
	import FormStatus from '#lib/components/form-status.svelte';
	import PageHeader from '#lib/components/page-header.svelte';
	import RemoveCard from '#lib/components/remove-card.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { showProblem } from '#lib/forms.js';
	import { scopeOf } from '#lib/routing.js';

	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const team = $derived(data.team);

	// The choices, named the way people tell them apart.
	const people = $derived(
		data.persons.map((person) => ({
			id: person.id,
			name: person.display_name === '' ? person.subject : person.display_name,
			detail: person.email === '' ? person.subject : person.email
		}))
	);
	const sources = $derived(
		data.sources.map((source) => ({
			id: source.id,
			name: source.name,
			detail: `Selects ${scopeOf(source.scope)}`
		}))
	);
	const sinks = $derived(
		data.sinks.map((sink) => ({
			id: sink.id,
			name: sink.name,
			detail: `Posts into ${sink.channel_name}`
		}))
	);

	// The API names a chosen item by its place in a list, such as
	// managers[1]. The form has one field for each list.
	function fieldOf(path: string): string {
		return path.replace(/\[\d+\]$/, '');
	}

	// Whether the last change was saved, until the form changes again.
	let saved = $state(false);

	// The form starts with the team the page loaded, and keeps what the
	// person chose when the page reloads after a save.
	const initial = untrack(() => ({
		name: data.team.name,
		managers: [...data.team.managers],
		sources: [...data.team.sources],
		sinks: [...data.team.sinks]
	}));

	const form = superForm(defaults(initial, zod4(UpdateTeamBody)), {
		SPA: true,
		dataType: 'json',
		validators: zod4Client(UpdateTeamBody),
		resetForm: false,
		onChange() {
			saved = false;
		},
		async onUpdate({ form }) {
			if (!form.valid) {
				return;
			}
			const answer = await updateTeam(team.id, form.data, ifMatch(team.etag));
			if (answer.status !== 200) {
				showProblem(form, answer.data, fieldOf);
				return;
			}
			saved = true;
			await refreshAll();
		}
	});
	const { form: fields, errors, message, enhance, submitting } = form;

	// removeTeam returns why the removal failed, or leaves the page.
	async function removeTeam(): Promise<string | null> {
		const answer = await deleteTeam(team.id, ifMatch(team.etag));
		if (answer.status !== 204) {
			return describe(answer.data);
		}
		await goto(resolve('/teams'));
		return null;
	}
</script>

<svelte:head>
	<title>{team.name} · Teams · PatchCaster</title>
</svelte:head>

<div class="grid gap-6">
	<PageHeader back={{ label: 'Teams', route: '/teams' }} title={team.name} />

	<Card.Root>
		<Card.Header>
			<Card.Title>Change the team</Card.Title>
			<Card.Description>
				The team's Routing Managers see its sources and sinks, and route the sources into the sinks.
				A source or sink that the team's routes use stays with the team.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			<form method="POST" use:enhance class="grid gap-8">
				<FormStatus message={$message} {saved} savedText="The team is saved." />
				<FormField id="name" label="Name" errors={$errors.name}>
					<Input
						id="name"
						name="name"
						class="max-w-sm"
						bind:value={$fields.name}
						aria-invalid={$errors.name ? 'true' : undefined}
					/>
				</FormField>
				<CheckboxList
					id="managers"
					legend="Routing Managers"
					hint="Only people who logged in at least once appear here."
					empty="Nobody has logged in yet."
					items={people}
					bind:selected={$fields.managers}
					errors={$errors.managers?._errors}
				/>
				<div class="grid gap-8 sm:grid-cols-2">
					<CheckboxList
						id="sources"
						legend="Sources"
						empty="There are no sources yet."
						items={sources}
						bind:selected={$fields.sources}
						errors={$errors.sources?._errors}
					/>
					<CheckboxList
						id="sinks"
						legend="Sinks"
						empty="There are no sinks yet."
						items={sinks}
						bind:selected={$fields.sinks}
						errors={$errors.sinks?._errors}
					/>
				</div>
				<div>
					<Button type="submit" disabled={$submitting}>Save</Button>
				</div>
			</form>
		</Card.Content>
	</Card.Root>

	<RemoveCard
		title="Remove the team"
		description="A team with routes stays until they are removed."
		question="Remove {team.name}?"
		consequence="PatchCaster forgets the team. Its sources and sinks stay, for other teams to use."
		onRemove={removeTeam}
	/>
</div>

<script lang="ts">
	import { untrack } from 'svelte';
	import { defaults, superForm } from 'sveltekit-superforms';
	import { zod4, zod4Client } from 'sveltekit-superforms/adapters';

	import { goto, refreshAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { deleteSink, updateSink } from '#lib/api/generated/client.js';
	import { UpdateSinkBody } from '#lib/api/generated/schemas.js';
	import { describe } from '#lib/api/problem.js';
	import { ifMatch } from '#lib/api/versions.js';
	import FormField from '#lib/components/form-field.svelte';
	import FormStatus from '#lib/components/form-status.svelte';
	import PageHeader from '#lib/components/page-header.svelte';
	import RemoveCard from '#lib/components/remove-card.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { showProblem } from '#lib/forms.js';
	import { deliveryOf, moment, waitingOf } from '#lib/health.js';

	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const sink = $derived(data.sink);
	const owners = $derived(data.teams.filter((team) => sink.teams.includes(team.id)));

	// Whether the last change was saved, until the form changes again.
	let saved = $state(false);

	// The form starts with the values the page loaded, and keeps what the
	// person typed when the page reloads after a save.
	const initial = untrack(() => ({ name: data.sink.name, channel_id: data.sink.channel_id }));

	const form = superForm(defaults(initial, zod4(UpdateSinkBody)), {
		SPA: true,
		validators: zod4Client(UpdateSinkBody),
		resetForm: false,
		onChange() {
			saved = false;
		},
		async onUpdate({ form }) {
			if (!form.valid) {
				return;
			}
			const answer = await updateSink(sink.id, form.data, ifMatch(sink.etag));
			if (answer.status !== 200) {
				showProblem(form, answer.data);
				return;
			}
			saved = true;
			await refreshAll();
		}
	});
	const { form: fields, errors, message, enhance, submitting } = form;

	// removeSink returns why the removal failed, or leaves the page.
	async function removeSink(): Promise<string | null> {
		const answer = await deleteSink(sink.id, ifMatch(sink.etag));
		if (answer.status !== 204) {
			return describe(answer.data);
		}
		await goto(resolve('/sinks'));
		return null;
	}
</script>

<svelte:head>
	<title>{sink.name} · Sinks · PatchCaster</title>
</svelte:head>

<div class="grid gap-6">
	<PageHeader
		back={{ label: 'Sinks', route: '/sinks' }}
		title={sink.name}
		subtitle="Posts into the channel {sink.channel_name}."
	/>

	<Card.Root>
		<Card.Header>
			<Card.Title>Delivery</Card.Title>
			<Card.Description>How messages reach the channel.</Card.Description>
		</Card.Header>
		<Card.Content class="grid gap-2 text-sm">
			<p>{deliveryOf(sink.health)}: {waitingOf(sink.health)}.</p>
			<p class="text-muted-foreground">Last success: {moment(sink.health.last_success_at)}.</p>
			<p class="text-muted-foreground">Last failure: {moment(sink.health.last_failure_at)}.</p>
			{#if sink.health.last_error}
				<p class="break-words text-destructive">{sink.health.last_error}</p>
			{/if}
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header>
			<Card.Title>Teams</Card.Title>
			<Card.Description>
				The teams that may route into the sink. Teams are given sinks on their own pages.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			{#if owners.length === 0}
				<p class="text-sm text-muted-foreground">No team owns the sink yet.</p>
			{:else}
				<ul class="grid gap-1 text-sm">
					{#each owners as team (team.id)}
						<li>
							<a
								href={resolve('/teams/[id]', { id: String(team.id) })}
								class="underline-offset-4 hover:underline">{team.name}</a
							>
						</li>
					{/each}
				</ul>
			{/if}
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header>
			<Card.Title>Change the sink</Card.Title>
			<Card.Description>
				PatchCaster checks that the bot can post into a changed channel before it saves it.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			<form method="POST" use:enhance class="grid gap-6 sm:grid-cols-2">
				<FormStatus
					message={$message}
					{saved}
					savedText="The sink is saved."
					class="sm:col-span-2"
				/>
				<FormField id="name" label="Name" errors={$errors.name}>
					<Input
						id="name"
						name="name"
						bind:value={$fields.name}
						aria-invalid={$errors.name ? 'true' : undefined}
					/>
				</FormField>
				<FormField id="channel_id" label="Channel ID" errors={$errors.channel_id}>
					<Input
						id="channel_id"
						name="channel_id"
						bind:value={$fields.channel_id}
						aria-invalid={$errors.channel_id ? 'true' : undefined}
					/>
				</FormField>
				<div class="sm:col-span-2">
					<Button type="submit" disabled={$submitting}>Save</Button>
				</div>
			</form>
		</Card.Content>
	</Card.Root>

	<RemoveCard
		title="Remove the sink"
		description="A sink that routes use stays until they are removed."
		question="Remove {sink.name}?"
		consequence="PatchCaster forgets the sink and which teams owned it. Its posts stay in the channel, but PatchCaster no longer replies to them."
		onRemove={removeSink}
	/>
</div>

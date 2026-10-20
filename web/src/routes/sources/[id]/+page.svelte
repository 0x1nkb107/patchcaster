<script lang="ts">
	import { untrack } from 'svelte';
	import { defaults, superForm } from 'sveltekit-superforms';
	import { zod4, zod4Client } from 'sveltekit-superforms/adapters';
	import { z } from 'zod';

	import { goto, refreshAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { deleteSource, updateSource, type SourceScopeKind } from '#lib/api/generated/client.js';
	import { describe } from '#lib/api/problem.js';
	import { ifMatch } from '#lib/api/versions.js';
	import FormField from '#lib/components/form-field.svelte';
	import RemoveCard from '#lib/components/remove-card.svelte';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import * as Select from '#lib/components/ui/select/index.js';
	import { showProblem } from '#lib/forms.js';
	import { moment } from '#lib/health.js';
	import { hookOf, scopeOf } from '#lib/routing.js';

	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const source = $derived(data.source);
	const owners = $derived(data.teams.filter((team) => source.teams.includes(team.id)));

	const scopeKinds: Record<SourceScopeKind, string> = {
		instance: 'The whole instance',
		group: 'A group, with everything below it',
		repository: 'One repository'
	};

	const schema = z.object({
		name: z.string().min(1).max(100),
		scope_kind: z.enum(['instance', 'group', 'repository']),
		scope_path: z.string()
	});

	function fieldOf(path: string): string {
		return path.startsWith('scope.') ? `scope_${path.slice('scope.'.length)}` : path;
	}

	// Whether the last change was saved, until the form changes again.
	let saved = $state(false);

	// The form starts with the values the page loaded, and keeps what the
	// person typed when the page reloads after a save.
	const initial = untrack(() => ({
		name: data.source.name,
		scope_kind: data.source.scope.kind,
		scope_path: data.source.scope.path ?? ''
	}));

	const form = superForm(defaults(initial, zod4(schema)), {
		SPA: true,
		validators: zod4Client(schema),
		resetForm: false,
		onChange() {
			saved = false;
		},
		async onUpdate({ form }) {
			if (!form.valid) {
				return;
			}
			const isInstance = form.data.scope_kind === 'instance';
			const answer = await updateSource(
				source.id,
				{
					name: form.data.name,
					scope: { kind: form.data.scope_kind, path: isInstance ? undefined : form.data.scope_path }
				},
				ifMatch(source.etag)
			);
			if (answer.status !== 200) {
				showProblem(form, answer.data, fieldOf);
				return;
			}
			saved = true;
			await refreshAll();
		}
	});
	const { form: fields, errors, message, enhance, submitting } = form;

	// Why the removal failed.
	let removalProblem: string | null = $state(null);

	async function removeSource(): Promise<void> {
		const answer = await deleteSource(source.id, ifMatch(source.etag));
		if (answer.status !== 204) {
			removalProblem = describe(answer.data);
			return;
		}
		await goto(resolve('/sources'));
	}
</script>

<svelte:head>
	<title>{source.name} · Sources · PatchCaster</title>
</svelte:head>

<div class="grid gap-6">
	<div>
		<a href={resolve('/sources')} class="text-sm text-muted-foreground hover:text-foreground">
			Sources
		</a>
		<h1 class="text-lg font-semibold">{source.name}</h1>
		<p class="text-sm text-muted-foreground">Selects {scopeOf(source.scope)}.</p>
	</div>

	{#if removalProblem}
		<Alert.Root variant="destructive">
			<Alert.Title>The source was not removed</Alert.Title>
			<Alert.Description>{removalProblem}</Alert.Description>
		</Alert.Root>
	{/if}

	<Card.Root>
		<Card.Header>
			<Card.Title>Hook</Card.Title>
			<Card.Description>How the source's events reach PatchCaster.</Card.Description>
		</Card.Header>
		<Card.Content class="grid gap-2 text-sm">
			<p class={source.hook.state === 'failed' ? 'text-destructive' : ''}>
				{hookOf(source.hook)}
			</p>
			{#if source.hook.registered_at}
				<p class="text-muted-foreground">Registered {moment(source.hook.registered_at)}.</p>
			{/if}
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header>
			<Card.Title>Teams</Card.Title>
			<Card.Description
				>The teams that may route the source. Teams are given sources on their own pages.</Card.Description
			>
		</Card.Header>
		<Card.Content>
			{#if owners.length === 0}
				<p class="text-sm text-muted-foreground">No team owns the source yet.</p>
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
			<Card.Title>Change the source</Card.Title>
			<Card.Description>
				A changed scope applies to the next event, and PatchCaster registers the hooks anew.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			<form method="POST" use:enhance class="grid gap-6 sm:grid-cols-2">
				{#if $message}
					<Alert.Root variant="destructive" class="sm:col-span-2">
						<Alert.Description>{$message}</Alert.Description>
					</Alert.Root>
				{:else if saved}
					<Alert.Root class="sm:col-span-2">
						<Alert.Description>The source is saved.</Alert.Description>
					</Alert.Root>
				{/if}
				<FormField id="name" label="Name" errors={$errors.name}>
					<Input
						id="name"
						name="name"
						bind:value={$fields.name}
						aria-invalid={$errors.name ? 'true' : undefined}
					/>
				</FormField>
				<FormField id="scope_kind" label="Selects" errors={$errors.scope_kind}>
					<Select.Root type="single" name="scope_kind" bind:value={$fields.scope_kind}>
						<Select.Trigger id="scope_kind" class="w-full">
							{scopeKinds[$fields.scope_kind]}
						</Select.Trigger>
						<Select.Content>
							{#each Object.entries(scopeKinds) as [kind, label] (kind)}
								<Select.Item value={kind} {label} />
							{/each}
						</Select.Content>
					</Select.Root>
				</FormField>
				{#if $fields.scope_kind !== 'instance'}
					<FormField id="scope_path" label="Path" errors={$errors.scope_path}>
						<Input
							id="scope_path"
							name="scope_path"
							bind:value={$fields.scope_path}
							aria-invalid={$errors.scope_path ? 'true' : undefined}
						/>
					</FormField>
				{/if}
				<div class="sm:col-span-2">
					<Button type="submit" disabled={$submitting}>Save</Button>
				</div>
			</form>
		</Card.Content>
	</Card.Root>

	<RemoveCard
		title="Remove the source"
		description="A source that routes use stays until they are removed."
		question="Remove {source.name}?"
		consequence="PatchCaster forgets the source and which teams owned it. A hook it registered for the source goes too."
		onRemove={removeSource}
	/>
</div>

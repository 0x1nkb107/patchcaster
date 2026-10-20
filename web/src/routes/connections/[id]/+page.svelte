<script lang="ts">
	import { untrack } from 'svelte';
	import { defaults, superForm } from 'sveltekit-superforms';
	import { zod4, zod4Client } from 'sveltekit-superforms/adapters';
	import { z } from 'zod';

	import { goto, refreshAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { deleteConnection, updateConnection } from '#lib/api/generated/client.js';
	import { UpdateConnectionBody } from '#lib/api/generated/schemas.js';
	import { describe } from '#lib/api/problem.js';
	import { ifMatch } from '#lib/api/versions.js';
	import FormField from '#lib/components/form-field.svelte';
	import FormStatus from '#lib/components/form-status.svelte';
	import PageHeader from '#lib/components/page-header.svelte';
	import RemoveCard from '#lib/components/remove-card.svelte';
	import SigningTokens from '#lib/components/signing-tokens.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { showProblem, unlessEmpty } from '#lib/forms.js';
	import { platformNames } from '#lib/health.js';

	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const connection = $derived(data.connection);
	const isForge = $derived(connection.webhook_url !== undefined);

	// The page's wording depends on the connection: only a forge has
	// sources, signing tokens and hooks, and only a credential acts as
	// someone.
	const actingAs = $derived(
		connection.account === '' ? 'nobody: it has no credential' : connection.account
	);
	const hookRegistration = $derived(
		connection.has_credential
			? 'PatchCaster registers its hooks at this address itself.'
			: 'Register the hooks by hand at this address, and add the signing token of each hook below.'
	);

	// removalTexts says what removing a connection does, for a forge and
	// for a chat platform.
	const removalTexts = {
		forge: {
			description: 'A connection that sources use stays until they are removed.',
			consequence: 'PatchCaster forgets the connection, its credential and its signing tokens.'
		},
		chat: {
			description: 'A connection that sinks use stays until they are removed.',
			consequence: 'PatchCaster forgets the connection and its credential.'
		}
	};
	const removal = $derived(isForge ? removalTexts.forge : removalTexts.chat);

	// The form takes a new credential as text that may stay empty, which
	// keeps the stored one.
	const changeSchema = UpdateConnectionBody.omit({ credential: true }).extend({
		credential: z.string()
	});

	// Whether the last change was saved, until the form changes again.
	let saved = $state(false);

	// The form starts with the values the page loaded, and keeps what the
	// person typed when the page reloads after a save.
	const initialChange = untrack(() => ({
		name: data.connection.name,
		base_url: data.connection.base_url,
		credential: ''
	}));

	const change = superForm(defaults(initialChange, zod4(changeSchema)), {
		SPA: true,
		validators: zod4Client(changeSchema),
		resetForm: false,
		onChange() {
			saved = false;
		},
		async onUpdate({ form }) {
			if (!form.valid) {
				return;
			}
			const answer = await updateConnection(
				connection.id,
				{ ...form.data, credential: unlessEmpty(form.data.credential) },
				ifMatch(connection.etag)
			);
			if (answer.status !== 200) {
				showProblem(form, answer.data);
				return;
			}
			form.data.credential = '';
			saved = true;
			await refreshAll();
		}
	});
	const { form: fields, errors, message, enhance, submitting } = change;

	// removeConnection returns why the removal failed, or leaves the page.
	async function removeConnection(): Promise<string | null> {
		const answer = await deleteConnection(connection.id, ifMatch(connection.etag));
		if (answer.status !== 204) {
			return describe(answer.data);
		}
		await goto(resolve('/connections'));
		return null;
	}
</script>

<svelte:head>
	<title>{connection.name} · Connections · PatchCaster</title>
</svelte:head>

<div class="grid gap-6">
	<PageHeader
		back={{ label: 'Connections', route: '/connections' }}
		title={connection.name}
		subtitle="{platformNames[connection.kind]} at {connection.base_url}, acting as {actingAs}"
	/>

	{#if connection.webhook_url !== undefined}
		<Card.Root>
			<Card.Header>
				<Card.Title>Webhook address</Card.Title>
				<Card.Description>
					{hookRegistration}
				</Card.Description>
			</Card.Header>
			<Card.Content>
				<code class="block break-all select-all">{connection.webhook_url}</code>
			</Card.Content>
		</Card.Root>
	{/if}

	<Card.Root>
		<Card.Header>
			<Card.Title>Change the connection</Card.Title>
			<Card.Description>
				A new credential is checked with the platform before it replaces the stored one. Leave it
				empty to keep the stored one.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			<form method="POST" use:enhance class="grid gap-6 sm:grid-cols-2">
				<FormStatus
					message={$message}
					{saved}
					savedText="The connection is saved."
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
				<FormField id="base_url" label="Address" errors={$errors.base_url}>
					<Input
						id="base_url"
						name="base_url"
						type="url"
						bind:value={$fields.base_url}
						aria-invalid={$errors.base_url ? 'true' : undefined}
					/>
				</FormField>
				<FormField id="credential" label="New credential" errors={$errors.credential}>
					<Input
						id="credential"
						name="credential"
						type="password"
						autocomplete="off"
						bind:value={$fields.credential}
						aria-invalid={$errors.credential ? 'true' : undefined}
					/>
				</FormField>
				<div class="sm:col-span-2">
					<Button type="submit" disabled={$submitting}>Save</Button>
				</div>
			</form>
		</Card.Content>
	</Card.Root>

	{#if isForge}
		<SigningTokens connectionID={connection.id} tokens={data.signingTokens} />
	{/if}

	<RemoveCard
		title="Remove the connection"
		description={removal.description}
		question="Remove {connection.name}?"
		consequence={removal.consequence}
		onRemove={removeConnection}
	/>
</div>

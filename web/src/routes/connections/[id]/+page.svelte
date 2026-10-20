<script lang="ts">
	import { untrack } from 'svelte';
	import { defaults, superForm } from 'sveltekit-superforms';
	import { zod4, zod4Client } from 'sveltekit-superforms/adapters';
	import { z } from 'zod';

	import { goto, refreshAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import {
		addSigningToken,
		deleteConnection,
		removeSigningToken,
		updateConnection,
		type SigningToken
	} from '#lib/api/generated/client.js';
	import { AddSigningTokenBody, UpdateConnectionBody } from '#lib/api/generated/schemas.js';
	import { describe } from '#lib/api/problem.js';
	import { ifMatch } from '#lib/api/versions.js';
	import FormField from '#lib/components/form-field.svelte';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import * as AlertDialog from '#lib/components/ui/alert-dialog/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { showProblem, unlessEmpty } from '#lib/forms.js';
	import { platformNames } from '#lib/health.js';

	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const connection = $derived(data.connection);
	const isForge = $derived(connection.webhook_url !== undefined);

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
		id: 'change',
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
	const {
		form: changeFields,
		errors: changeErrors,
		message: changeMessage,
		enhance: changeEnhance,
		submitting: changing
	} = change;

	const addToken = superForm(defaults(zod4(AddSigningTokenBody)), {
		id: 'add-token',
		SPA: true,
		validators: zod4Client(AddSigningTokenBody),
		async onUpdate({ form }) {
			if (!form.valid) {
				return;
			}
			const answer = await addSigningToken(connection.id, form.data);
			if (answer.status !== 201) {
				showProblem(form, answer.data);
				return;
			}
			await refreshAll();
		}
	});
	const {
		form: tokenFields,
		errors: tokenErrors,
		message: tokenMessage,
		enhance: tokenEnhance,
		submitting: adding
	} = addToken;

	// Why the last removal failed, until one succeeds.
	let removalProblem: string | null = $state(null);

	async function removeToken(token: SigningToken): Promise<void> {
		const answer = await removeSigningToken(connection.id, token.id);
		removalProblem = answer.status === 204 ? null : describe(answer.data);
		await refreshAll();
	}

	async function removeConnection(): Promise<void> {
		const answer = await deleteConnection(connection.id, ifMatch(connection.etag));
		if (answer.status !== 204) {
			removalProblem = describe(answer.data);
			return;
		}
		await goto(resolve('/connections'));
	}

	function day(timestamp: string): string {
		return new Date(timestamp).toLocaleDateString();
	}
</script>

<svelte:head>
	<title>{connection.name} · Connections · PatchCaster</title>
</svelte:head>

<div class="grid gap-6">
	<div>
		<a href={resolve('/connections')} class="text-sm text-muted-foreground hover:text-foreground">
			Connections
		</a>
		<h1 class="text-lg font-semibold">{connection.name}</h1>
		<p class="text-sm text-muted-foreground">
			{platformNames[connection.kind]} at {connection.base_url}, acting as {connection.account ===
			''
				? 'nobody: it has no credential'
				: connection.account}
		</p>
	</div>

	{#if removalProblem}
		<Alert.Root variant="destructive">
			<Alert.Title>Nothing was removed</Alert.Title>
			<Alert.Description>{removalProblem}</Alert.Description>
		</Alert.Root>
	{/if}

	{#if connection.webhook_url !== undefined}
		<Card.Root>
			<Card.Header>
				<Card.Title>Webhook address</Card.Title>
				<Card.Description>
					{connection.has_credential
						? 'PatchCaster registers its hooks at this address itself.'
						: 'Register the hooks by hand at this address, and add the signing token of each hook below.'}
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
			<form method="POST" use:changeEnhance class="grid gap-6 sm:grid-cols-2">
				{#if $changeMessage}
					<Alert.Root variant="destructive" class="sm:col-span-2">
						<Alert.Description>{$changeMessage}</Alert.Description>
					</Alert.Root>
				{:else if saved}
					<Alert.Root class="sm:col-span-2">
						<Alert.Description>The connection is saved.</Alert.Description>
					</Alert.Root>
				{/if}
				<FormField id="name" label="Name" errors={$changeErrors.name}>
					<Input
						id="name"
						name="name"
						bind:value={$changeFields.name}
						aria-invalid={$changeErrors.name ? 'true' : undefined}
					/>
				</FormField>
				<FormField id="base_url" label="Address" errors={$changeErrors.base_url}>
					<Input
						id="base_url"
						name="base_url"
						type="url"
						bind:value={$changeFields.base_url}
						aria-invalid={$changeErrors.base_url ? 'true' : undefined}
					/>
				</FormField>
				<FormField id="credential" label="New credential" errors={$changeErrors.credential}>
					<Input
						id="credential"
						name="credential"
						type="password"
						autocomplete="off"
						bind:value={$changeFields.credential}
						aria-invalid={$changeErrors.credential ? 'true' : undefined}
					/>
				</FormField>
				<div class="sm:col-span-2">
					<Button type="submit" disabled={$changing}>Save</Button>
				</div>
			</form>
		</Card.Content>
	</Card.Root>

	{#if isForge}
		<Card.Root>
			<Card.Header>
				<Card.Title>Signing tokens</Card.Title>
				<Card.Description>
					Any of these tokens may sign a delivery. To rotate a token, add the new one, switch the
					hook over, then remove the old one. PatchCaster manages the tokens of the hooks it
					registers itself.
				</Card.Description>
			</Card.Header>
			<Card.Content class="grid gap-6">
				{#if data.signingTokens.length === 0}
					<p class="text-sm text-muted-foreground">The connection has no signing tokens.</p>
				{:else}
					<table class="w-full text-sm">
						<thead class="text-left text-muted-foreground">
							<tr>
								<th class="py-2 pr-4 font-medium">Label</th>
								<th class="py-2 pr-4 font-medium">Added</th>
								<th></th>
							</tr>
						</thead>
						<tbody>
							{#each data.signingTokens as token (token.id)}
								<tr class="border-t">
									<td class="py-2 pr-4">{token.label}</td>
									<td class="py-2 pr-4">{day(token.created_at)}</td>
									<td class="py-2 text-right">
										<Button variant="destructive" size="sm" onclick={() => removeToken(token)}>
											Remove
										</Button>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				{/if}
				<form
					method="POST"
					use:tokenEnhance
					class="grid gap-6 sm:grid-cols-[1fr_1fr_auto] sm:items-end"
				>
					{#if $tokenMessage}
						<Alert.Root variant="destructive" class="sm:col-span-3">
							<Alert.Description>{$tokenMessage}</Alert.Description>
						</Alert.Root>
					{/if}
					<FormField id="label" label="Label" errors={$tokenErrors.label}>
						<Input
							id="label"
							name="label"
							placeholder="group payments"
							bind:value={$tokenFields.label}
							aria-invalid={$tokenErrors.label ? 'true' : undefined}
						/>
					</FormField>
					<FormField id="token" label="Token" errors={$tokenErrors.token}>
						<Input
							id="token"
							name="token"
							type="password"
							autocomplete="off"
							placeholder="whsec_…"
							bind:value={$tokenFields.token}
							aria-invalid={$tokenErrors.token ? 'true' : undefined}
						/>
					</FormField>
					<Button type="submit" disabled={$adding}>Add</Button>
				</form>
			</Card.Content>
		</Card.Root>
	{/if}

	<Card.Root>
		<Card.Header>
			<Card.Title>Remove the connection</Card.Title>
			<Card.Description>
				A connection that {isForge ? 'sources' : 'sinks'} use stays until they are removed.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			<AlertDialog.Root>
				<AlertDialog.Trigger>
					{#snippet child({ props })}
						<Button variant="destructive" {...props}>Remove</Button>
					{/snippet}
				</AlertDialog.Trigger>
				<AlertDialog.Content>
					<AlertDialog.Header>
						<AlertDialog.Title>Remove {connection.name}?</AlertDialog.Title>
						<AlertDialog.Description>
							PatchCaster forgets the connection and its credential{isForge
								? ', and its signing tokens'
								: ''}.
						</AlertDialog.Description>
					</AlertDialog.Header>
					<AlertDialog.Footer>
						<AlertDialog.Cancel>Keep it</AlertDialog.Cancel>
						<AlertDialog.Action variant="destructive" onclick={removeConnection}>
							Remove
						</AlertDialog.Action>
					</AlertDialog.Footer>
				</AlertDialog.Content>
			</AlertDialog.Root>
		</Card.Content>
	</Card.Root>
</div>

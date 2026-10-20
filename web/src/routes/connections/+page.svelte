<script lang="ts">
	import { defaults, superForm } from 'sveltekit-superforms';
	import { zod4, zod4Client } from 'sveltekit-superforms/adapters';
	import { z } from 'zod';

	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { createConnection, type ConnectionKind } from '#lib/api/generated/client.js';
	import { CreateConnectionBody } from '#lib/api/generated/schemas.js';
	import FormField from '#lib/components/form-field.svelte';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import * as Select from '#lib/components/ui/select/index.js';
	import { showProblem, unlessEmpty } from '#lib/forms.js';
	import { platformNames } from '#lib/health.js';

	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// The form takes the credential as text that may stay empty: a GitLab
	// connection works without one.
	const schema = CreateConnectionBody.omit({ credential: true }).extend({
		credential: z.string()
	});

	// credentialHints says what each platform's credential is.
	const credentialHints: Record<ConnectionKind, string> = {
		gitlab:
			'An access token, with which PatchCaster registers the hooks itself. Leave it empty to register them by hand.',
		mattermost: "The bot account's access token."
	};

	const form = superForm(
		defaults({ kind: 'gitlab', name: '', base_url: '', credential: '' }, zod4(schema)),
		{
			SPA: true,
			validators: zod4Client(schema),
			resetForm: false,
			async onUpdate({ form }) {
				if (!form.valid) {
					return;
				}
				const answer = await createConnection({
					...form.data,
					credential: unlessEmpty(form.data.credential)
				});
				if (answer.status !== 201) {
					showProblem(form, answer.data);
					return;
				}
				await goto(resolve('/connections/[id]', { id: String(answer.data.id) }));
			}
		}
	);
	const { form: fields, errors, message, enhance, submitting } = form;
</script>

<svelte:head>
	<title>Connections · PatchCaster</title>
</svelte:head>

<div class="grid gap-6">
	<Card.Root>
		<Card.Header>
			<Card.Title>Connect a platform</Card.Title>
			<Card.Description>
				A connection holds what PatchCaster acts with on one platform: GitLab as a forge, Mattermost
				as a chat platform. PatchCaster checks the credential with the platform before it stores it,
				and never shows it again.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			<form method="POST" use:enhance class="grid gap-6 sm:grid-cols-2">
				{#if $message}
					<Alert.Root variant="destructive" class="sm:col-span-2">
						<Alert.Description>{$message}</Alert.Description>
					</Alert.Root>
				{/if}
				<FormField id="kind" label="Platform" errors={$errors.kind}>
					<Select.Root type="single" name="kind" bind:value={$fields.kind}>
						<Select.Trigger id="kind" class="w-full">{platformNames[$fields.kind]}</Select.Trigger>
						<Select.Content>
							<Select.Item value="gitlab" label="GitLab" />
							<Select.Item value="mattermost" label="Mattermost" />
						</Select.Content>
					</Select.Root>
				</FormField>
				<FormField id="name" label="Name" errors={$errors.name}>
					<Input
						id="name"
						name="name"
						placeholder="gitlab"
						bind:value={$fields.name}
						aria-invalid={$errors.name ? 'true' : undefined}
					/>
				</FormField>
				<FormField id="base_url" label="Address" errors={$errors.base_url}>
					<Input
						id="base_url"
						name="base_url"
						type="url"
						placeholder="https://gitlab.example.org"
						bind:value={$fields.base_url}
						aria-invalid={$errors.base_url ? 'true' : undefined}
					/>
				</FormField>
				<FormField
					id="credential"
					label="Credential"
					hint={credentialHints[$fields.kind]}
					errors={$errors.credential}
				>
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
					<Button type="submit" disabled={$submitting}>Connect</Button>
				</div>
			</form>
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header>
			<Card.Title>Connections</Card.Title>
		</Card.Header>
		<Card.Content>
			{#if data.connections.length === 0}
				<p class="text-sm text-muted-foreground">No platform is connected yet.</p>
			{:else}
				<table class="w-full text-sm">
					<thead class="text-left text-muted-foreground">
						<tr>
							<th class="py-2 pr-4 font-medium">Name</th>
							<th class="py-2 pr-4 font-medium">Platform</th>
							<th class="py-2 pr-4 font-medium">Address</th>
							<th class="py-2 font-medium">Acts as</th>
						</tr>
					</thead>
					<tbody>
						{#each data.connections as connection (connection.id)}
							<tr class="border-t">
								<td class="py-2 pr-4">
									<a
										href={resolve('/connections/[id]', { id: String(connection.id) })}
										class="font-medium underline-offset-4 hover:underline"
									>
										{connection.name}
									</a>
								</td>
								<td class="py-2 pr-4">{platformNames[connection.kind]}</td>
								<td class="py-2 pr-4 break-all">{connection.base_url}</td>
								<td class="py-2">
									{connection.account === '' ? 'no credential' : connection.account}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
		</Card.Content>
	</Card.Root>
</div>

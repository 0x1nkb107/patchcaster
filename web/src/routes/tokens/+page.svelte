<script lang="ts">
	import { defaults, superForm } from 'sveltekit-superforms';
	import { zod4, zod4Client } from 'sveltekit-superforms/adapters';

	import { refreshAll } from '$app/navigation';
	import { createToken, revokeToken, type ApiToken } from '#lib/api/generated/client.js';
	import { CreateTokenBody } from '#lib/api/generated/schemas.js';
	import FormField from '#lib/components/form-field.svelte';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { describe } from '#lib/api/problem.js';
	import { showProblem } from '#lib/forms.js';

	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// The token itself is shown once, right after it is created.
	let created: { name: string; secret: string } | null = $state(null);

	const form = superForm(defaults(zod4(CreateTokenBody)), {
		SPA: true,
		validators: zod4Client(CreateTokenBody),
		async onUpdate({ form }) {
			if (!form.valid) {
				return;
			}
			const answer = await createToken(form.data);
			if (answer.status !== 201) {
				showProblem(form, answer.data);
				return;
			}
			created = { name: answer.data.token.name, secret: answer.data.secret };
			await refreshAll();
		}
	});
	const { form: fields, errors, message, enhance, submitting } = form;

	// Why the last revocation failed, until one succeeds.
	let revokeProblem: string | null = $state(null);

	async function revoke(token: ApiToken): Promise<void> {
		const answer = await revokeToken(token.id);
		revokeProblem = answer.status === 204 ? null : describe(answer.data);
		await refreshAll();
	}

	function stateOf(token: ApiToken): string {
		if (token.revoked_at !== undefined) {
			return 'revoked';
		}
		return new Date(token.expires_at) <= new Date() ? 'expired' : 'active';
	}

	function day(timestamp: string): string {
		return new Date(timestamp).toLocaleDateString();
	}
</script>

<svelte:head>
	<title>API tokens · PatchCaster</title>
</svelte:head>

<div class="grid gap-6">
	<Card.Root>
		<Card.Header>
			<Card.Title>Create an API token</Card.Title>
			<Card.Description>
				Automation sends a token as <code>Authorization: Bearer</code>. It acts with your
				permissions, and lives at most 90 days.
			</Card.Description>
		</Card.Header>
		<Card.Content class="grid gap-6">
			{#if created}
				<Alert.Root>
					<Alert.Title>Copy the token {created.name} now</Alert.Title>
					<Alert.Description>
						It is shown only this once.
						<code class="mt-2 block break-all select-all">{created.secret}</code>
					</Alert.Description>
				</Alert.Root>
			{/if}
			<form method="POST" use:enhance class="grid gap-6 sm:grid-cols-[1fr_10rem_auto] sm:items-end">
				{#if $message}
					<Alert.Root variant="destructive" class="sm:col-span-3">
						<Alert.Description>{$message}</Alert.Description>
					</Alert.Root>
				{/if}
				<FormField id="name" label="Name" errors={$errors.name}>
					<Input
						id="name"
						name="name"
						placeholder="routes from git"
						bind:value={$fields.name}
						aria-invalid={$errors.name ? 'true' : undefined}
					/>
				</FormField>
				<FormField id="lifetime_days" label="Days" errors={$errors.lifetime_days}>
					<Input
						id="lifetime_days"
						name="lifetime_days"
						type="number"
						min="1"
						max="90"
						bind:value={$fields.lifetime_days}
						aria-invalid={$errors.lifetime_days ? 'true' : undefined}
					/>
				</FormField>
				<Button type="submit" disabled={$submitting}>Create</Button>
			</form>
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header>
			<Card.Title>Your tokens</Card.Title>
		</Card.Header>
		<Card.Content class="grid gap-6">
			{#if revokeProblem}
				<Alert.Root variant="destructive">
					<Alert.Title>The token was not revoked</Alert.Title>
					<Alert.Description>{revokeProblem}</Alert.Description>
				</Alert.Root>
			{/if}
			{#if data.tokens.length === 0}
				<p class="text-sm text-muted-foreground">You have no API tokens.</p>
			{:else}
				<table class="w-full text-sm">
					<thead class="text-left text-muted-foreground">
						<tr>
							<th class="py-2 font-medium">Name</th>
							<th class="py-2 font-medium">Created</th>
							<th class="py-2 font-medium">Expires</th>
							<th class="py-2 font-medium">State</th>
							<th></th>
						</tr>
					</thead>
					<tbody>
						{#each data.tokens as token (token.id)}
							<tr class="border-t">
								<td class="py-2">{token.name}</td>
								<td class="py-2">{day(token.created_at)}</td>
								<td class="py-2">{day(token.expires_at)}</td>
								<td class="py-2">
									<Badge variant={stateOf(token) === 'active' ? 'secondary' : 'outline'}>
										{stateOf(token)}
									</Badge>
								</td>
								<td class="py-2 text-right">
									{#if stateOf(token) === 'active'}
										<Button variant="destructive" size="sm" onclick={() => revoke(token)}>
											Revoke
										</Button>
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
		</Card.Content>
	</Card.Root>
</div>

<!--
	The signing tokens of a forge connection: the list with a removal for
	each, and a form that adds one. A change reloads the page's data.
-->
<script lang="ts">
	import { defaults, superForm } from 'sveltekit-superforms';
	import { zod4, zod4Client } from 'sveltekit-superforms/adapters';

	import { refreshAll } from '$app/navigation';
	import {
		addSigningToken,
		removeSigningToken,
		type SigningToken
	} from '#lib/api/generated/client.js';
	import { AddSigningTokenBody } from '#lib/api/generated/schemas.js';
	import { describe } from '#lib/api/problem.js';
	import FormField from '#lib/components/form-field.svelte';
	import FormStatus from '#lib/components/form-status.svelte';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { showProblem } from '#lib/forms.js';

	let { connectionID, tokens }: { connectionID: number; tokens: SigningToken[] } = $props();

	const form = superForm(defaults(zod4(AddSigningTokenBody)), {
		id: 'add-token',
		SPA: true,
		validators: zod4Client(AddSigningTokenBody),
		async onUpdate({ form }) {
			if (!form.valid) {
				return;
			}
			const answer = await addSigningToken(connectionID, form.data);
			if (answer.status !== 201) {
				showProblem(form, answer.data);
				return;
			}
			await refreshAll();
		}
	});
	const { form: fields, errors, message, enhance, submitting } = form;

	// Why the last removal failed, until one succeeds.
	let removalProblem: string | null = $state(null);

	async function remove(token: SigningToken): Promise<void> {
		const answer = await removeSigningToken(connectionID, token.id);
		removalProblem = answer.status === 204 ? null : describe(answer.data);
		await refreshAll();
	}

	function day(timestamp: string): string {
		return new Date(timestamp).toLocaleDateString();
	}
</script>

<Card.Root>
	<Card.Header>
		<Card.Title>Signing tokens</Card.Title>
		<Card.Description>
			Any of these tokens may sign a delivery. To rotate a token, add the new one, switch the hook
			over, then remove the old one. PatchCaster manages the tokens of the hooks it registers
			itself.
		</Card.Description>
	</Card.Header>
	<Card.Content class="grid gap-6">
		{#if removalProblem}
			<Alert.Root variant="destructive">
				<Alert.Title>The token was not removed</Alert.Title>
				<Alert.Description>{removalProblem}</Alert.Description>
			</Alert.Root>
		{/if}
		{#if tokens.length === 0}
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
					{#each tokens as token (token.id)}
						<tr class="border-t">
							<td class="py-2 pr-4">{token.label}</td>
							<td class="py-2 pr-4">{day(token.created_at)}</td>
							<td class="py-2 text-right">
								<Button variant="destructive" size="sm" onclick={() => remove(token)}>
									Remove
								</Button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
		<form method="POST" use:enhance class="grid gap-6 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
			<FormStatus message={$message} class="sm:col-span-3" />
			<FormField id="label" label="Label" errors={$errors.label}>
				<Input
					id="label"
					name="label"
					placeholder="group payments"
					bind:value={$fields.label}
					aria-invalid={$errors.label ? 'true' : undefined}
				/>
			</FormField>
			<FormField id="token" label="Token" errors={$errors.token}>
				<Input
					id="token"
					name="token"
					type="password"
					autocomplete="off"
					placeholder="whsec_…"
					bind:value={$fields.token}
					aria-invalid={$errors.token ? 'true' : undefined}
				/>
			</FormField>
			<Button type="submit" disabled={$submitting}>Add</Button>
		</form>
	</Card.Content>
</Card.Root>

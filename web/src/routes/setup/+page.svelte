<script lang="ts">
	import { defaults, superForm } from 'sveltekit-superforms';
	import { zod4, zod4Client } from 'sveltekit-superforms/adapters';
	import { z } from 'zod';

	import { page } from '$app/state';
	import { setUp } from '#lib/api/generated/client.js';
	import { SetUpBody } from '#lib/api/generated/schemas.js';
	import FormField from '#lib/components/form-field.svelte';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { showProblem } from '#lib/forms.js';

	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// The setup link carries its token in the URL fragment, which browsers
	// never send to a server (ADR-033).
	const setupToken = page.url.hash.slice(1);

	// The form takes the allowed groups as text, one group per line.
	const schema = SetUpBody.omit({ setup_token: true, allowed_groups: true }).extend({
		allowed_groups: z.string()
	});

	// Group names may hold commas, as distinguished names do, so only line
	// breaks separate them.
	function groupsOf(text: string): string[] {
		return text
			.split('\n')
			.map((group) => group.trim())
			.filter((group) => group !== '');
	}

	// The API names a group by its place in the list, such as
	// allowed_groups[1]. The form has one field for all of them.
	function fieldOf(path: string): string {
		return path.startsWith('allowed_groups[') ? 'allowed_groups' : path;
	}

	const form = superForm(defaults(zod4(schema)), {
		SPA: true,
		validators: zod4Client(schema),
		resetForm: false,
		async onUpdate({ form }) {
			if (!form.valid) {
				return;
			}
			const answer = await setUp({
				...form.data,
				setup_token: setupToken,
				groups_claim: form.data.groups_claim ?? '',
				allowed_groups: groupsOf(form.data.allowed_groups)
			});
			if (answer.status !== 200) {
				showProblem(form, answer.data, fieldOf);
				return;
			}
			// The login that follows makes the person logging in the first
			// administrator, in this same browser.
			window.location.assign(answer.data.login_url);
		}
	});
	const { form: fields, errors, message, enhance, submitting } = form;
</script>

<svelte:head>
	<title>Set up PatchCaster</title>
</svelte:head>

<Card.Root class="max-w-xl">
	<Card.Header>
		<Card.Title>Set up PatchCaster</Card.Title>
		<Card.Description>
			Connect the identity provider your organization logs in with. Register PatchCaster there
			first, with the redirect address
			<code>{page.url.origin}/auth/callback</code>.
		</Card.Description>
	</Card.Header>
	<Card.Content>
		{#if !data.instance.setup_required}
			<Alert.Root>
				<Alert.Title>The setup is complete</Alert.Title>
				<Alert.Description>An administrator exists. Log in from the start page.</Alert.Description>
			</Alert.Root>
		{:else if setupToken === ''}
			<Alert.Root variant="destructive">
				<Alert.Title>This is no setup link</Alert.Title>
				<Alert.Description>
					Open the link that <code>patchcaster bootstrap</code> printed, in full.
				</Alert.Description>
			</Alert.Root>
		{:else}
			<form method="POST" use:enhance class="grid gap-6">
				{#if $message}
					<Alert.Root variant="destructive">
						<Alert.Description>{$message}</Alert.Description>
					</Alert.Root>
				{/if}
				<FormField
					id="issuer"
					label="Issuer"
					hint="The provider's issuer URL, such as https://login.example.org/realms/main."
					errors={$errors.issuer}
				>
					<Input
						id="issuer"
						name="issuer"
						type="url"
						bind:value={$fields.issuer}
						aria-invalid={$errors.issuer ? 'true' : undefined}
					/>
				</FormField>
				<FormField id="client_id" label="Client ID" errors={$errors.client_id}>
					<Input
						id="client_id"
						name="client_id"
						bind:value={$fields.client_id}
						aria-invalid={$errors.client_id ? 'true' : undefined}
					/>
				</FormField>
				<FormField
					id="client_secret"
					label="Client secret"
					hint="Stored encrypted, and never shown again."
					errors={$errors.client_secret}
				>
					<Input
						id="client_secret"
						name="client_secret"
						type="password"
						autocomplete="off"
						bind:value={$fields.client_secret}
						aria-invalid={$errors.client_secret ? 'true' : undefined}
					/>
				</FormField>
				<FormField
					id="groups_claim"
					label="Groups claim"
					hint="The ID token claim that lists a person's groups. Leave it empty to admit everyone the provider authenticates."
					errors={$errors.groups_claim}
				>
					<Input id="groups_claim" name="groups_claim" bind:value={$fields.groups_claim} />
				</FormField>
				<FormField
					id="allowed_groups"
					label="Allowed groups"
					hint="One group per line. Only their members may log in."
					errors={$errors.allowed_groups}
				>
					<textarea
						id="allowed_groups"
						name="allowed_groups"
						rows="3"
						class="rounded-lg border bg-transparent px-2.5 py-1.5 text-sm"
						bind:value={$fields.allowed_groups}></textarea>
				</FormField>
				<div>
					<Button type="submit" disabled={$submitting}>Save and log in</Button>
				</div>
			</form>
		{/if}
	</Card.Content>
</Card.Root>

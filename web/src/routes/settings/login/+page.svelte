<script lang="ts">
	import { untrack } from 'svelte';
	import { defaults, superForm } from 'sveltekit-superforms';
	import { zod4, zod4Client } from 'sveltekit-superforms/adapters';
	import { z } from 'zod';

	import { refreshAll } from '$app/navigation';
	import { putLoginSettings } from '#lib/api/generated/client.js';
	import { PutLoginSettingsBody } from '#lib/api/generated/schemas.js';
	import { ifMatch } from '#lib/api/versions.js';
	import FormField from '#lib/components/form-field.svelte';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import { showProblem, unlessEmpty } from '#lib/forms.js';

	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// The form takes the allowed groups as text, one group per line, and a
	// new client secret as text that may stay empty, which keeps the stored
	// one.
	const schema = PutLoginSettingsBody.omit({ client_secret: true, allowed_groups: true }).extend({
		client_secret: z.string(),
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

	// Whether the last change was saved, until the form changes again.
	let saved = $state(false);

	// The form starts with the settings the page loaded, and keeps what the
	// person typed when the page reloads after a save.
	const initial = untrack(() => ({
		issuer: data.settings.issuer,
		client_id: data.settings.client_id,
		client_secret: '',
		groups_claim: data.settings.groups_claim,
		allowed_groups: data.settings.allowed_groups.join('\n')
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
			const answer = await putLoginSettings(
				{
					...form.data,
					client_secret: unlessEmpty(form.data.client_secret),
					allowed_groups: groupsOf(form.data.allowed_groups)
				},
				ifMatch(data.etag)
			);
			if (answer.status !== 200) {
				showProblem(form, answer.data, fieldOf);
				return;
			}
			form.data.client_secret = '';
			saved = true;
			await refreshAll();
		}
	});
	const { form: fields, errors, message, enhance, submitting } = form;
</script>

<svelte:head>
	<title>Login · PatchCaster</title>
</svelte:head>

<Card.Root class="max-w-xl">
	<Card.Header>
		<Card.Title>Login</Card.Title>
		<Card.Description>
			People log in through this OpenID Connect provider. PatchCaster checks that a provider answers
			at the issuer before it saves a change.
		</Card.Description>
	</Card.Header>
	<Card.Content>
		<form method="POST" use:enhance class="grid gap-6">
			{#if $message}
				<Alert.Root variant="destructive">
					<Alert.Description>{$message}</Alert.Description>
				</Alert.Root>
			{:else if saved}
				<Alert.Root>
					<Alert.Description>The login settings are saved.</Alert.Description>
				</Alert.Root>
			{/if}
			<FormField id="issuer" label="Issuer" errors={$errors.issuer}>
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
				label="New client secret"
				hint="Leave it empty to keep the stored one."
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
				hint="The ID token claim that lists a person's groups. Leave it empty to admit everyone the provider knows."
				errors={$errors.groups_claim}
			>
				<Input
					id="groups_claim"
					name="groups_claim"
					placeholder="groups"
					bind:value={$fields.groups_claim}
					aria-invalid={$errors.groups_claim ? 'true' : undefined}
				/>
			</FormField>
			<FormField
				id="allowed_groups"
				label="Allowed groups"
				hint="One group per line. People in none of them cannot log in."
				errors={$errors.allowed_groups}
			>
				<Textarea
					id="allowed_groups"
					name="allowed_groups"
					rows={4}
					bind:value={$fields.allowed_groups}
					aria-invalid={$errors.allowed_groups ? 'true' : undefined}
				/>
			</FormField>
			<div>
				<Button type="submit" disabled={$submitting}>Save</Button>
			</div>
		</form>
	</Card.Content>
</Card.Root>

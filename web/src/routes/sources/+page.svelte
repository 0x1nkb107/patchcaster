<script lang="ts">
	import { defaults, superForm } from 'sveltekit-superforms';
	import { zod4, zod4Client } from 'sveltekit-superforms/adapters';
	import { z } from 'zod';

	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { createSource, type SourceScopeKind } from '#lib/api/generated/client.js';
	import FormField from '#lib/components/form-field.svelte';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import * as Select from '#lib/components/ui/select/index.js';
	import { showProblem } from '#lib/forms.js';
	import { namesById, scopeOf } from '#lib/routing.js';

	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const connectionNames = $derived(namesById(data.connections));
	// Only a forge connection has a webhook address, and sources.
	const forges = $derived(data.connections.filter((c) => c.webhook_url !== undefined));

	// scopeKinds names each kind of scope for the select.
	const scopeKinds: Record<SourceScopeKind, string> = {
		instance: 'The whole instance',
		group: 'A group, with everything below it',
		repository: 'One repository'
	};

	// The form keeps the scope's kind and path as fields of their own, and
	// the connection as the select's text.
	const schema = z.object({
		connection_id: z.string().min(1, 'Choose a forge connection'),
		name: z.string().min(1).max(100),
		scope_kind: z.enum(['instance', 'group', 'repository']),
		scope_path: z.string()
	});

	// The API names the scope's fields by their path in the request.
	function fieldOf(path: string): string {
		return path.startsWith('scope.') ? `scope_${path.slice('scope.'.length)}` : path;
	}

	const form = superForm(
		defaults({ connection_id: '', name: '', scope_kind: 'group', scope_path: '' }, zod4(schema)),
		{
			SPA: true,
			validators: zod4Client(schema),
			resetForm: false,
			async onUpdate({ form }) {
				if (!form.valid) {
					return;
				}
				const isInstance = form.data.scope_kind === 'instance';
				const answer = await createSource({
					connection_id: Number(form.data.connection_id),
					name: form.data.name,
					scope: {
						kind: form.data.scope_kind,
						path: isInstance ? undefined : form.data.scope_path
					}
				});
				if (answer.status !== 201) {
					showProblem(form, answer.data, fieldOf);
					return;
				}
				await goto(resolve('/sources/[id]', { id: String(answer.data.id) }));
			}
		}
	);
	const { form: fields, errors, message, enhance, submitting } = form;
</script>

<svelte:head>
	<title>Sources · PatchCaster</title>
</svelte:head>

<div class="grid gap-6">
	<Card.Root>
		<Card.Header>
			<Card.Title>Add a source</Card.Title>
			<Card.Description>
				A source selects the repositories of a forge connection whose merge requests a team may
				route: the whole instance, a group with everything below it, or one repository.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			{#if forges.length === 0}
				<p class="text-sm text-muted-foreground">
					Connect a forge first, on the <a
						href={resolve('/connections')}
						class="underline underline-offset-4">Connections</a
					> page.
				</p>
			{:else}
				<form method="POST" use:enhance class="grid gap-6 sm:grid-cols-2">
					{#if $message}
						<Alert.Root variant="destructive" class="sm:col-span-2">
							<Alert.Description>{$message}</Alert.Description>
						</Alert.Root>
					{/if}
					<FormField id="connection_id" label="Forge" errors={$errors.connection_id}>
						<Select.Root type="single" name="connection_id" bind:value={$fields.connection_id}>
							<Select.Trigger id="connection_id" class="w-full">
								{connectionNames.get(Number($fields.connection_id)) ?? 'Choose a connection'}
							</Select.Trigger>
							<Select.Content>
								{#each forges as forge (forge.id)}
									<Select.Item value={String(forge.id)} label={forge.name} />
								{/each}
							</Select.Content>
						</Select.Root>
					</FormField>
					<FormField id="name" label="Name" errors={$errors.name}>
						<Input
							id="name"
							name="name"
							placeholder="payments"
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
						<FormField
							id="scope_path"
							label="Path"
							hint="The full path as the forge shows it."
							errors={$errors.scope_path}
						>
							<Input
								id="scope_path"
								name="scope_path"
								placeholder={$fields.scope_kind === 'group' ? 'payments' : 'payments/api'}
								bind:value={$fields.scope_path}
								aria-invalid={$errors.scope_path ? 'true' : undefined}
							/>
						</FormField>
					{/if}
					<div class="sm:col-span-2">
						<Button type="submit" disabled={$submitting}>Add</Button>
					</div>
				</form>
			{/if}
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header>
			<Card.Title>Sources</Card.Title>
		</Card.Header>
		<Card.Content>
			{#if data.sources.length === 0}
				<p class="text-sm text-muted-foreground">There are no sources yet.</p>
			{:else}
				<table class="w-full text-sm">
					<thead class="text-left text-muted-foreground">
						<tr>
							<th class="py-2 pr-4 font-medium">Name</th>
							<th class="py-2 pr-4 font-medium">Selects</th>
							<th class="py-2 pr-4 font-medium">Forge</th>
							<th class="py-2 font-medium">Hook</th>
						</tr>
					</thead>
					<tbody>
						{#each data.sources as source (source.id)}
							<tr class="border-t">
								<td class="py-2 pr-4">
									<a
										href={resolve('/sources/[id]', { id: String(source.id) })}
										class="font-medium underline-offset-4 hover:underline"
									>
										{source.name}
									</a>
								</td>
								<td class="py-2 pr-4 break-all">{scopeOf(source.scope)}</td>
								<td class="py-2 pr-4">{connectionNames.get(source.connection_id)}</td>
								<td class="py-2">
									<Badge variant={source.hook.state === 'failed' ? 'destructive' : 'secondary'}>
										{source.hook.state}
									</Badge>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
		</Card.Content>
	</Card.Root>
</div>

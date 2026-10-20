<script lang="ts">
	import { defaults, superForm } from 'sveltekit-superforms';
	import { zod4, zod4Client } from 'sveltekit-superforms/adapters';
	import { z } from 'zod';

	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { createSink } from '#lib/api/generated/client.js';
	import FormField from '#lib/components/form-field.svelte';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import * as Select from '#lib/components/ui/select/index.js';
	import { showProblem } from '#lib/forms.js';
	import { deliveryOf } from '#lib/health.js';
	import { namesById } from '#lib/routing.js';

	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const connectionNames = $derived(namesById(data.connections));
	// A connection without a webhook address is a chat connection.
	const chats = $derived(data.connections.filter((c) => c.webhook_url === undefined));

	// The form takes the connection as the select's text.
	const schema = z.object({
		connection_id: z.string().min(1, 'Choose a chat connection'),
		name: z.string().min(1).max(100),
		channel_id: z.string().min(1)
	});

	const form = superForm(defaults({ connection_id: '', name: '', channel_id: '' }, zod4(schema)), {
		SPA: true,
		validators: zod4Client(schema),
		resetForm: false,
		async onUpdate({ form }) {
			if (!form.valid) {
				return;
			}
			const answer = await createSink({
				...form.data,
				connection_id: Number(form.data.connection_id)
			});
			if (answer.status !== 201) {
				showProblem(form, answer.data);
				return;
			}
			await goto(resolve('/sinks/[id]', { id: String(answer.data.id) }));
		}
	});
	const { form: fields, errors, message, enhance, submitting } = form;
</script>

<svelte:head>
	<title>Sinks · PatchCaster</title>
</svelte:head>

<div class="grid gap-6">
	<Card.Root>
		<Card.Header>
			<Card.Title>Add a sink</Card.Title>
			<Card.Description>
				A sink is a channel that a team may route merge requests into. PatchCaster checks that the
				connection's bot can post there before it stores the sink.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			{#if chats.length === 0}
				<p class="text-sm text-muted-foreground">
					Connect a chat platform first, on the <a
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
					<FormField id="connection_id" label="Chat platform" errors={$errors.connection_id}>
						<Select.Root type="single" name="connection_id" bind:value={$fields.connection_id}>
							<Select.Trigger id="connection_id" class="w-full">
								{connectionNames.get(Number($fields.connection_id)) ?? 'Choose a connection'}
							</Select.Trigger>
							<Select.Content>
								{#each chats as chat (chat.id)}
									<Select.Item value={String(chat.id)} label={chat.name} />
								{/each}
							</Select.Content>
						</Select.Root>
					</FormField>
					<FormField id="name" label="Name" errors={$errors.name}>
						<Input
							id="name"
							name="name"
							placeholder="payments room"
							bind:value={$fields.name}
							aria-invalid={$errors.name ? 'true' : undefined}
						/>
					</FormField>
					<FormField
						id="channel_id"
						label="Channel ID"
						hint="Mattermost shows a channel's ID in the channel's info."
						errors={$errors.channel_id}
					>
						<Input
							id="channel_id"
							name="channel_id"
							bind:value={$fields.channel_id}
							aria-invalid={$errors.channel_id ? 'true' : undefined}
						/>
					</FormField>
					<div class="sm:col-span-2">
						<Button type="submit" disabled={$submitting}>Add</Button>
					</div>
				</form>
			{/if}
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header>
			<Card.Title>Sinks</Card.Title>
		</Card.Header>
		<Card.Content>
			{#if data.sinks.length === 0}
				<p class="text-sm text-muted-foreground">There are no sinks yet.</p>
			{:else}
				<table class="w-full text-sm">
					<thead class="text-left text-muted-foreground">
						<tr>
							<th class="py-2 pr-4 font-medium">Name</th>
							<th class="py-2 pr-4 font-medium">Channel</th>
							<th class="py-2 pr-4 font-medium">Chat platform</th>
							<th class="py-2 font-medium">Delivery</th>
						</tr>
					</thead>
					<tbody>
						{#each data.sinks as sink (sink.id)}
							<tr class="border-t">
								<td class="py-2 pr-4">
									<a
										href={resolve('/sinks/[id]', { id: String(sink.id) })}
										class="font-medium underline-offset-4 hover:underline"
									>
										{sink.name}
									</a>
								</td>
								<td class="py-2 pr-4">{sink.channel_name}</td>
								<td class="py-2 pr-4">{connectionNames.get(sink.connection_id)}</td>
								<td class="py-2">
									<Badge
										variant={deliveryOf(sink.health) === 'failing' ? 'destructive' : 'secondary'}
									>
										{deliveryOf(sink.health)}
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

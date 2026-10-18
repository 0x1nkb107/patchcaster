<script lang="ts">
	import type { Connection } from '#lib/api/generated/client.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { deliveryOf, moment, platformNames, rejectionOf, type Delivery } from '#lib/health.js';

	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const connectionNames = $derived(new Map(data.connections.map((c) => [c.id, c.name])));

	const badgeVariant: Record<Delivery, 'destructive' | 'secondary' | 'outline'> = {
		failing: 'destructive',
		delivering: 'secondary',
		idle: 'outline'
	};

	// The API gives a forge connection, and only a forge connection, a
	// webhook address.
	function isForge(connection: Connection): boolean {
		return connection.webhook_url !== undefined;
	}
</script>

<svelte:head>
	<title>Health · PatchCaster</title>
</svelte:head>

<div class="grid gap-6">
	<div>
		<h1 class="text-lg font-semibold">Health</h1>
		<p class="text-sm text-muted-foreground">
			How deliveries reach PatchCaster and its chat platforms. A connection or sink in trouble never
			takes PatchCaster out of service, and the others keep delivering.
		</p>
	</div>

	{#if data.connections.length > 0}
		<Card.Root>
			<Card.Header>
				<Card.Title>Connections</Card.Title>
			</Card.Header>
			<Card.Content>
				<table class="w-full text-sm">
					<thead class="text-left text-muted-foreground">
						<tr>
							<th class="py-2 pr-4 font-medium">Name</th>
							<th class="py-2 pr-4 font-medium">Platform</th>
							<th class="py-2 pr-4 font-medium">Health</th>
						</tr>
					</thead>
					<tbody>
						{#each data.connections as connection (connection.id)}
							<tr class="border-t align-top">
								<td class="py-2 pr-4">{connection.name}</td>
								<td class="py-2 pr-4">{platformNames[connection.kind]}</td>
								<td class="grid gap-1 py-2">
									{#if isForge(connection)}
										<span>Last delivery: {moment(connection.health.last_delivery_at)}</span>
										{#if connection.health.last_rejection_at}
											<span class="text-destructive">
												Last rejection: {moment(connection.health.last_rejection_at)}, because
												{rejectionOf(connection.health.last_rejection)}
											</span>
										{/if}
									{:else}
										<span>
											<Badge variant={badgeVariant[deliveryOf(connection.health)]}>
												{deliveryOf(connection.health)}
											</Badge>
											{connection.health.waiting_messages ?? 0} waiting
										</span>
										<span>Last success: {moment(connection.health.last_success_at)}</span>
										{#if connection.health.last_failure_at}
											<span class="break-words text-destructive">
												Last failure: {moment(connection.health.last_failure_at)}:
												{connection.health.last_error}
											</span>
										{/if}
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</Card.Content>
		</Card.Root>
	{/if}

	<Card.Root>
		<Card.Header>
			<Card.Title>Sinks</Card.Title>
		</Card.Header>
		<Card.Content>
			{#if data.sinks.length === 0}
				<p class="text-sm text-muted-foreground">You see no sinks.</p>
			{:else}
				<table class="w-full text-sm">
					<thead class="text-left text-muted-foreground">
						<tr>
							<th class="py-2 pr-4 font-medium">Sink</th>
							<th class="py-2 pr-4 font-medium">State</th>
							<th class="py-2 pr-4 font-medium">Waiting</th>
							<th class="py-2 pr-4 font-medium">Last success</th>
							<th class="py-2 pr-4 font-medium">Last failure</th>
						</tr>
					</thead>
					<tbody>
						{#each data.sinks as sink (sink.id)}
							<tr class="border-t align-top">
								<td class="py-2 pr-4">
									{sink.name}
									<span class="block text-muted-foreground">
										{sink.channel_name}
										{#if connectionNames.has(sink.connection_id)}
											on {connectionNames.get(sink.connection_id)}
										{/if}
									</span>
								</td>
								<td class="py-2 pr-4">
									<Badge variant={badgeVariant[deliveryOf(sink.health)]}>
										{deliveryOf(sink.health)}
									</Badge>
								</td>
								<td class="py-2 pr-4">
									{sink.health.waiting_messages}
									{#if sink.health.oldest_waiting_since}
										<span class="block text-muted-foreground">
											since {moment(sink.health.oldest_waiting_since)}
										</span>
									{/if}
								</td>
								<td class="py-2 pr-4">{moment(sink.health.last_success_at)}</td>
								<td class="py-2 pr-4">
									{moment(sink.health.last_failure_at)}
									{#if sink.health.last_error}
										<span class="block break-words text-destructive">{sink.health.last_error}</span>
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

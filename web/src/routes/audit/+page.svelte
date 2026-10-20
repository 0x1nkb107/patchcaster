<script lang="ts">
	import { untrack } from 'svelte';

	import { listAudit } from '#lib/api/generated/client.js';
	import { describe } from '#lib/api/problem.js';
	import { actorOf, auditPageSize, hasState, stateOf } from '#lib/audit.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { moment } from '#lib/health.js';

	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// The entries read so far, newest first, and where the older ones
	// start. The page reads more on request.
	let entries = $state(untrack(() => data.entries));
	let next = $state(untrack(() => data.next));
	let reading = $state(false);

	// Why reading older entries failed.
	let problem: string | null = $state(null);

	async function readOlder(): Promise<void> {
		reading = true;
		const answer = await listAudit({ limit: auditPageSize, cursor: next });
		reading = false;
		if (answer.status !== 200) {
			problem = describe(answer.data);
			return;
		}
		problem = null;
		entries = [...entries, ...answer.data.items];
		next = answer.data.next_cursor;
	}
</script>

<svelte:head>
	<title>Audit log · PatchCaster</title>
</svelte:head>

<Card.Root>
	<Card.Header>
		<Card.Title>Audit log</Card.Title>
		<Card.Description>
			Every change to the configuration, every setup step, every grant and every Instance Admin's
			login, newest first. Secrets never appear, and nobody can change or remove an entry.
		</Card.Description>
	</Card.Header>
	<Card.Content class="grid gap-4">
		{#if entries.length === 0}
			<p class="text-sm text-muted-foreground">The log is empty.</p>
		{:else}
			<ol class="grid text-sm">
				{#each entries as entry (entry.id)}
					<li class="grid gap-1 border-t py-3">
						<div class="flex flex-wrap items-baseline justify-between gap-x-4">
							<code class="font-medium">{entry.action}</code>
							<span class="text-muted-foreground">{moment(entry.at)}</span>
						</div>
						<p class="text-muted-foreground">
							{actorOf(entry)} · {entry.target_kind}
							{entry.target_id}
						</p>
						{#if hasState(entry)}
							<details>
								<summary class="cursor-pointer text-muted-foreground hover:text-foreground">
									Change
								</summary>
								<div class="mt-2 grid gap-2 sm:grid-cols-2">
									<div>
										<p class="text-muted-foreground">Before</p>
										<pre class="overflow-x-auto rounded-md bg-muted p-2 text-xs">{stateOf(
												entry.before
											)}</pre>
									</div>
									<div>
										<p class="text-muted-foreground">After</p>
										<pre class="overflow-x-auto rounded-md bg-muted p-2 text-xs">{stateOf(
												entry.after
											)}</pre>
									</div>
								</div>
							</details>
						{/if}
					</li>
				{/each}
			</ol>
		{/if}
		{#if problem}
			<Alert.Root variant="destructive">
				<Alert.Description>{problem}</Alert.Description>
			</Alert.Root>
		{/if}
		{#if next !== undefined}
			<Button variant="outline" class="w-fit" disabled={reading} onclick={readOlder}>
				Show older entries
			</Button>
		{/if}
	</Card.Content>
</Card.Root>

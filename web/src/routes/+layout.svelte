<script lang="ts">
	import type { Snippet } from 'svelte';

	import { refreshAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { endSession } from '#lib/api/generated/client.js';
	import { Button } from '#lib/components/ui/button/index.js';

	import type { LayoutData } from './$types';

	import '../app.css';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	// A failed logout leaves the session as it was, and the refresh then
	// shows the person still logged in.
	async function logOut(): Promise<void> {
		await endSession();
		await refreshAll();
	}
</script>

<div class="min-h-screen">
	<header class="border-b">
		<nav class="mx-auto flex max-w-4xl items-center gap-6 px-4 py-3">
			<a href={resolve('/')} class="font-semibold">PatchCaster</a>
			{#if data.person}
				<a href={resolve('/tokens')} class="text-sm text-muted-foreground hover:text-foreground">
					API tokens
				</a>
				<span class="ml-auto text-sm text-muted-foreground">{data.person.display_name}</span>
				<Button variant="outline" size="sm" onclick={logOut}>Log out</Button>
			{/if}
		</nav>
	</header>
	<main class="mx-auto max-w-4xl px-4 py-8">
		{@render children()}
	</main>
</div>

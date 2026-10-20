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
		<nav class="mx-auto flex max-w-4xl flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3">
			<a href={resolve('/')} class="font-semibold">PatchCaster</a>
			{#if data.person}
				<a href={resolve('/health')} class="text-sm text-muted-foreground hover:text-foreground">
					Health
				</a>
				{#if data.person.administrator}
					<a href={resolve('/teams')} class="text-sm text-muted-foreground hover:text-foreground">
						Teams
					</a>
					<a href={resolve('/sources')} class="text-sm text-muted-foreground hover:text-foreground">
						Sources
					</a>
					<a href={resolve('/sinks')} class="text-sm text-muted-foreground hover:text-foreground">
						Sinks
					</a>
					<a
						href={resolve('/connections')}
						class="text-sm text-muted-foreground hover:text-foreground"
					>
						Connections
					</a>
					<a
						href={resolve('/settings/login')}
						class="text-sm text-muted-foreground hover:text-foreground"
					>
						Login
					</a>
				{/if}
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
	<footer class="mx-auto max-w-4xl px-4 pb-8 text-sm text-muted-foreground">
		<!-- The build writes the notices next to the bundle, outside the router. -->
		<a href="/third-party-licences.txt" rel="external" class="hover:text-foreground">
			Third-party licences
		</a>
	</footer>
</div>

<script lang="ts">
	import type { Snippet } from 'svelte';

	import MainNav from '#lib/components/main-nav.svelte';

	import type { LayoutData } from './$types';

	import '../app.css';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();
</script>

<div class="min-h-screen">
	<header class="border-b">
		<MainNav person={data.person} />
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

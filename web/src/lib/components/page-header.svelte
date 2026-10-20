<!--
	The head of an item's page: a link back to the list it belongs to, the
	item's name, and a line about it.
-->
<script lang="ts">
	import { resolve } from '$app/paths';

	// ListRoute is a page that lists items.
	type ListRoute = '/connections' | '/sources' | '/sinks' | '/teams';

	let {
		back,
		title,
		subtitle
	}: {
		back: { label: string; route: ListRoute };
		title: string;
		subtitle?: string;
	} = $props();
</script>

<div>
	<a href={resolve(back.route)} class="text-sm text-muted-foreground hover:text-foreground">
		{back.label}
	</a>
	<h1 class="text-lg font-semibold">{title}</h1>
	{#if subtitle}
		<p class="text-sm text-muted-foreground">{subtitle}</p>
	{/if}
</div>

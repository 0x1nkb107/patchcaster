<!--
	A labelled field with its hint and the errors the form or the API found
	for it.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	import { Label } from '#lib/components/ui/label/index.js';

	let {
		id,
		label,
		hint,
		errors,
		children
	}: {
		id: string;
		label: string;
		hint?: string;
		errors?: string[] | undefined;
		children: Snippet;
	} = $props();
</script>

<div class="grid gap-2">
	<Label for={id}>{label}</Label>
	{@render children()}
	{#if hint}
		<p class="text-sm text-muted-foreground">{hint}</p>
	{/if}
	{#each errors ?? [] as error (error)}
		<p class="text-sm text-destructive">{error}</p>
	{/each}
</div>

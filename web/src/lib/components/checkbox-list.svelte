<!--
	A group of checkboxes that chooses items by their ids, with its legend,
	its hint and the errors the form or the API found for it.
-->
<script lang="ts">
	import { Checkbox } from '#lib/components/ui/checkbox/index.js';
	import { Label } from '#lib/components/ui/label/index.js';

	// Item is one choice: an id, a name and what else tells it apart.
	interface Item {
		id: number;
		name: string;
		detail?: string;
	}

	let {
		id,
		legend,
		hint,
		empty,
		items,
		selected = $bindable([]),
		errors
	}: {
		id: string;
		legend: string;
		hint?: string;
		empty: string;
		items: Item[];
		selected?: number[];
		errors?: string[] | undefined;
	} = $props();

	// toggle chooses an item or lets it go, and keeps the ids in order, so
	// that an unchanged choice reads the same.
	function toggle(itemID: number, chosen: boolean): void {
		const others = selected.filter((other) => other !== itemID);
		selected = chosen ? [...others, itemID].sort((a, b) => a - b) : others;
	}
</script>

<fieldset class="grid gap-2">
	<legend class="mb-2 text-sm leading-none font-medium">{legend}</legend>
	{#if items.length === 0}
		<p class="text-sm text-muted-foreground">{empty}</p>
	{:else}
		<ul class="grid gap-2">
			{#each items as item (item.id)}
				<li class="flex items-start gap-2">
					<Checkbox
						id="{id}-{item.id}"
						checked={selected.includes(item.id)}
						onCheckedChange={(checked: boolean) => {
							toggle(item.id, checked);
						}}
						aria-invalid={errors ? 'true' : undefined}
					/>
					<Label for="{id}-{item.id}" class="grid gap-0.5 font-normal">
						<span>{item.name}</span>
						{#if item.detail}
							<span class="text-muted-foreground">{item.detail}</span>
						{/if}
					</Label>
				</li>
			{/each}
		</ul>
	{/if}
	{#if hint}
		<p class="text-sm text-muted-foreground">{hint}</p>
	{/if}
	{#each errors ?? [] as error (error)}
		<p class="text-sm text-destructive">{error}</p>
	{/each}
</fieldset>

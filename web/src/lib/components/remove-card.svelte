<!--
	A card that removes an item after a confirmation dialog. The page does
	the removal, and it returns why the removal failed, or null once the
	item is gone. The card then shows the failure.
-->
<script lang="ts">
	import ConfirmButton from '#lib/components/confirm-button.svelte';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import * as Card from '#lib/components/ui/card/index.js';

	let {
		title,
		description,
		question,
		consequence,
		onRemove
	}: {
		title: string;
		description: string;
		question: string;
		consequence: string;
		onRemove: () => Promise<string | null>;
	} = $props();

	// Why the last removal failed, until one succeeds.
	let problem: string | null = $state(null);

	async function remove(): Promise<void> {
		problem = await onRemove();
	}
</script>

<Card.Root>
	<Card.Header>
		<Card.Title>{title}</Card.Title>
		<Card.Description>{description}</Card.Description>
	</Card.Header>
	<Card.Content class="grid gap-4">
		{#if problem}
			<Alert.Root variant="destructive">
				<Alert.Title>Nothing was removed</Alert.Title>
				<Alert.Description>{problem}</Alert.Description>
			</Alert.Root>
		{/if}
		<ConfirmButton
			label="Remove"
			{question}
			{consequence}
			cancelLabel="Keep it"
			variant="destructive"
			onConfirm={remove}
		/>
	</Card.Content>
</Card.Root>

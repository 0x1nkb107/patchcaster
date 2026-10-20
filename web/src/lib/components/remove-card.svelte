<!--
	A card that removes an item after a confirmation dialog. The page does
	the removal and shows why it failed.
-->
<script lang="ts">
	import * as AlertDialog from '#lib/components/ui/alert-dialog/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
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
		onRemove: () => Promise<void>;
	} = $props();
</script>

<Card.Root>
	<Card.Header>
		<Card.Title>{title}</Card.Title>
		<Card.Description>{description}</Card.Description>
	</Card.Header>
	<Card.Content>
		<AlertDialog.Root>
			<AlertDialog.Trigger>
				{#snippet child({ props })}
					<Button variant="destructive" {...props}>Remove</Button>
				{/snippet}
			</AlertDialog.Trigger>
			<AlertDialog.Content>
				<AlertDialog.Header>
					<AlertDialog.Title>{question}</AlertDialog.Title>
					<AlertDialog.Description>{consequence}</AlertDialog.Description>
				</AlertDialog.Header>
				<AlertDialog.Footer>
					<AlertDialog.Cancel>Keep it</AlertDialog.Cancel>
					<AlertDialog.Action variant="destructive" onclick={onRemove}>Remove</AlertDialog.Action>
				</AlertDialog.Footer>
			</AlertDialog.Content>
		</AlertDialog.Root>
	</Card.Content>
</Card.Root>

<!--
	A button that asks for confirmation in a dialog before it acts. The
	dialog closes before the action starts, so that what the action shows
	afterwards is not hidden behind it. Only the dialog's Cancel closes it
	by itself.
-->
<script lang="ts">
	import * as AlertDialog from '#lib/components/ui/alert-dialog/index.js';
	import { Button } from '#lib/components/ui/button/index.js';

	let {
		label,
		question,
		consequence,
		confirmLabel,
		cancelLabel = 'Cancel',
		variant = 'default',
		size = 'default',
		onConfirm
	}: {
		label: string;
		question: string;
		consequence: string;
		confirmLabel?: string;
		cancelLabel?: string;
		variant?: 'default' | 'outline' | 'destructive';
		size?: 'default' | 'sm';
		onConfirm: () => Promise<void>;
	} = $props();

	// Whether the dialog asks for confirmation.
	let confirming = $state(false);

	async function confirm(): Promise<void> {
		confirming = false;
		await onConfirm();
	}
</script>

<AlertDialog.Root bind:open={confirming}>
	<AlertDialog.Trigger>
		{#snippet child({ props })}
			<Button {variant} {size} class="w-fit" {...props}>{label}</Button>
		{/snippet}
	</AlertDialog.Trigger>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{question}</AlertDialog.Title>
			<AlertDialog.Description>{consequence}</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{cancelLabel}</AlertDialog.Cancel>
			<AlertDialog.Action {variant} onclick={confirm}>{confirmLabel ?? label}</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

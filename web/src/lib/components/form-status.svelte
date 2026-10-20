<!--
	What became of a form's last submission: why it was refused, or that it
	was saved. It shows nothing before the first submission.
-->
<script lang="ts">
	import * as Alert from '#lib/components/ui/alert/index.js';

	let {
		message,
		saved = false,
		savedText = '',
		class: className
	}: {
		message: string | undefined;
		saved?: boolean;
		savedText?: string;
		class?: string;
	} = $props();
</script>

{#if message}
	<Alert.Root variant="destructive" class={className}>
		<Alert.Description>{message}</Alert.Description>
	</Alert.Root>
{:else if saved}
	<Alert.Root class={className}>
		<Alert.Description>{savedText}</Alert.Description>
	</Alert.Root>
{/if}

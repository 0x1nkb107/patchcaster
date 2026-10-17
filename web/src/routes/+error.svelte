<script lang="ts">
	import { page } from '$app/state';
	import * as Alert from '#lib/components/ui/alert/index.js';
</script>

<svelte:head>
	<title>{page.status} · PatchCaster</title>
</svelte:head>

<Alert.Root variant="destructive" class="max-w-xl">
	<Alert.Title>{page.status}</Alert.Title>
	<Alert.Description>{page.error?.message}</Alert.Description>
</Alert.Root>

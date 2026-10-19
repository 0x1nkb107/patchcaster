<script lang="ts">
	import { resolve } from '$app/paths';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { roleNames } from '#lib/roles.js';

	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

<svelte:head>
	<title>PatchCaster</title>
</svelte:head>

<Card.Root class="max-w-lg">
	{#if data.person}
		<Card.Header>
			<Card.Title>Welcome, {data.person.display_name}</Card.Title>
			<Card.Description>
				Logged in as {data.person.subject}
				{#if data.role !== null}
					<Badge variant="secondary" class="ml-2">{roleNames[data.role]}</Badge>
				{/if}
			</Card.Description>
		</Card.Header>
		<Card.Content>
			<Button href={resolve('/tokens')} variant="outline">Manage your API tokens</Button>
		</Card.Content>
	{:else if data.instance.setup_required}
		<Card.Header>
			<Card.Title>Not set up yet</Card.Title>
			<Card.Description>
				Run <code>patchcaster bootstrap</code> and open the setup link it prints.
			</Card.Description>
		</Card.Header>
	{:else}
		<Card.Header>
			<Card.Title>Log in</Card.Title>
			<Card.Description>
				PatchCaster logs you in through your organization's identity provider.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			<!-- The login runs on the server, outside the application's router. -->
			<Button href="/auth/login" rel="external" data-sveltekit-reload>Log in</Button>
		</Card.Content>
	{/if}
</Card.Root>

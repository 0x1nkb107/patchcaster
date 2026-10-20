<!--
	The main navigation: the pages the person who is logged in may open, and
	their logout. Pages for Instance Admins show to them alone.
-->
<script lang="ts">
	import { refreshAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { endSession, type Person } from '#lib/api/generated/client.js';
	import { Button } from '#lib/components/ui/button/index.js';

	let { person }: { person: Person | null } = $props();

	// Route is a page the navigation links to.
	type Route =
		'/health' | '/teams' | '/sources' | '/sinks' | '/connections' | '/settings/login' | '/tokens';

	// links lists the pages in the order the navigation shows them.
	const links: { label: string; route: Route; adminOnly?: boolean }[] = [
		{ label: 'Health', route: '/health' },
		{ label: 'Teams', route: '/teams', adminOnly: true },
		{ label: 'Sources', route: '/sources', adminOnly: true },
		{ label: 'Sinks', route: '/sinks', adminOnly: true },
		{ label: 'Connections', route: '/connections', adminOnly: true },
		{ label: 'Login', route: '/settings/login', adminOnly: true },
		{ label: 'API tokens', route: '/tokens' }
	];

	// isCurrent reports whether the page shown is route or lies below it,
	// as a connection's page lies below the connections.
	function isCurrent(route: Route): boolean {
		const href = resolve(route);
		const path = page.url.pathname;
		return path === href || path.startsWith(`${href}/`);
	}

	// A failed logout leaves the session as it was, and the refresh then
	// shows the person still logged in.
	async function logOut(): Promise<void> {
		await endSession();
		await refreshAll();
	}
</script>

<nav
	aria-label="Main navigation"
	class="mx-auto flex max-w-4xl flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3"
>
	<a href={resolve('/')} class="font-semibold">PatchCaster</a>

	{#if person}
		{#each links as link (link.route)}
			{#if !link.adminOnly || person.administrator}
				<a
					href={resolve(link.route)}
					aria-current={isCurrent(link.route) ? 'page' : undefined}
					class="text-sm text-muted-foreground hover:text-foreground aria-[current=page]:text-foreground"
				>
					{link.label}
				</a>
			{/if}
		{/each}

		<span class="ml-auto text-sm text-muted-foreground">{person.display_name}</span>

		<Button variant="outline" size="sm" onclick={logOut}>Log out</Button>
	{/if}
</nav>

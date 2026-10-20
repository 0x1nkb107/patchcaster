<!--
	The main navigation: the pages the person who is logged in may open, and
	their logout. Pages for Instance Admins show to them alone, and the
	settings of the instance gather in one menu.
-->
<script lang="ts">
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';

	import { refreshAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { endSession, type Person } from '#lib/api/generated/client.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as DropdownMenu from '#lib/components/ui/dropdown-menu/index.js';

	let { person }: { person: Person | null } = $props();

	// Route is a page the navigation links to.
	type Route =
		| '/health'
		| '/teams'
		| '/sources'
		| '/sinks'
		| '/connections'
		| '/settings/login'
		| '/people'
		| '/audit'
		| '/tokens';

	// Link is a page.
	interface Link {
		label: string;
		route: Route;
	}

	// Entry is what the bar shows: a link, or a menu of links. An entry for
	// Instance Admins shows to them alone.
	type Entry = (Link | { label: string; menu: Link[] }) & { adminOnly?: boolean };

	// entries lists the bar in the order it shows them. The settings of the
	// instance gather in one menu.
	const entries: Entry[] = [
		{ label: 'Health', route: '/health' },
		{ label: 'Teams', route: '/teams', adminOnly: true },
		{ label: 'Sources', route: '/sources', adminOnly: true },
		{ label: 'Sinks', route: '/sinks', adminOnly: true },
		{
			label: 'Instance',
			adminOnly: true,
			menu: [
				{ label: 'Connections', route: '/connections' },
				{ label: 'Login', route: '/settings/login' },
				{ label: 'People', route: '/people' },
				{ label: 'Audit log', route: '/audit' }
			]
		},
		{ label: 'API tokens', route: '/tokens' }
	];

	// isCurrent reports whether the page shown is route or lies below it,
	// as a connection's page lies below the connections.
	function isCurrent(route: Route): boolean {
		const href = resolve(route);
		const path = page.url.pathname;
		return path === href || path.startsWith(`${href}/`);
	}

	// holdsCurrent reports whether a menu links to the page shown.
	function holdsCurrent(menu: Link[]): boolean {
		return menu.some((link) => isCurrent(link.route));
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
		{#each entries as entry (entry.label)}
			{#if !entry.adminOnly || person.administrator}
				{#if 'menu' in entry}
					<DropdownMenu.Root>
						<DropdownMenu.Trigger
							data-current={holdsCurrent(entry.menu) ? 'true' : undefined}
							class="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground data-current:text-foreground"
						>
							{entry.label}
							<ChevronDownIcon class="size-4" />
						</DropdownMenu.Trigger>
						<DropdownMenu.Content align="start">
							{#each entry.menu as link (link.route)}
								<DropdownMenu.Item>
									{#snippet child({ props })}
										<a
											href={resolve(link.route)}
											aria-current={isCurrent(link.route) ? 'page' : undefined}
											{...props}
										>
											{link.label}
										</a>
									{/snippet}
								</DropdownMenu.Item>
							{/each}
						</DropdownMenu.Content>
					</DropdownMenu.Root>
				{:else}
					<a
						href={resolve(entry.route)}
						aria-current={isCurrent(entry.route) ? 'page' : undefined}
						class="text-sm text-muted-foreground hover:text-foreground aria-[current=page]:text-foreground"
					>
						{entry.label}
					</a>
				{/if}
			{/if}
		{/each}

		<span class="ml-auto text-sm text-muted-foreground">{person.display_name}</span>

		<Button variant="outline" size="sm" onclick={logOut}>Log out</Button>
	{/if}
</nav>

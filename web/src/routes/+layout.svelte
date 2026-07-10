<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { api } from '$lib/api';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';

	let { children } = $props();
	// Gate every route except /login behind a valid session (requirements.md
	// FR-32). /login handles its own status check (setup vs. login form) and
	// its own redirect back to "/" once authenticated, so it's excluded here
	// to avoid the two checks racing each other.
	let checked = $state(false);

	onMount(async () => {
		if ($page.url.pathname === '/login') {
			checked = true;
			return;
		}
		try {
			const status = await api.authStatus();
			// setup_required always needs the /login page's setup form,
			// regardless of network -- the admin account has to exist before
			// WAN exposure can ever be turned on. Otherwise, lan_bypass means
			// the backend won't actually demand a session for this client
			// (see requireAuth in internal/api/router.go), so there's
			// nothing to gate even if authenticated is false.
			const needsLogin = status.setup_required || (!status.lan_bypass && !status.authenticated);
			if (needsLogin) {
				await goto('/login');
				return;
			}
		} catch {
			// Status check itself failed (backend unreachable) -- let the
			// page render and surface its own error instead of looping here.
		}
		checked = true;
	});
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>
{#if checked}
	{@render children()}
{/if}

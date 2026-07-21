<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { api } from '$lib/api';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { locale } from '$lib/i18n';

	let { children } = $props();

	// app.html hardcodes lang="ko" as the pre-hydration default (matches this
	// project's original all-Korean UI) -- once the locale store picks up a
	// browser-detected or saved override, keep the actual <html lang> in
	// sync with it.
	$effect(() => {
		document.documentElement.lang = $locale;
	});
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
				// This layout mounts once and never remounts on client-side
				// navigation -- `goto` swaps `children` in place, it doesn't
				// re-run this onMount. The old code returned here without
				// ever setting `checked`, so after landing on /login the
				// `{#if checked}` gate below stayed closed forever: the URL
				// bar showed /login but nothing rendered but the dark-mode
				// background (confirmed on real hardware -- exactly the
				// "first load is a black screen, only a manual refresh
				// fixes it" report, since a hard refresh is the only thing
				// that re-runs onMount with pathname already /login). Just
				// fall through to `checked = true` below instead of
				// returning early.
				await goto('/login');
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

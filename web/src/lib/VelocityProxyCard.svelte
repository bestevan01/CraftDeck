<script lang="ts">
	import type { ProxyStatus } from '$lib/api';
	import { t } from '$lib/i18n';

	// The always-on Velocity proxy's version is picked once, at creation,
	// and never re-checked afterward (see ensureProxyInstance) -- so it can
	// silently fall behind newer Velocity releases, including ones that add
	// support for a new Minecraft protocol version entirely. This panel
	// surfaces that gap and lets the operator apply the update.
	let {
		proxyStatus,
		upgrading,
		upgradeError,
		onUpgrade
	}: {
		proxyStatus: ProxyStatus;
		upgrading: boolean;
		upgradeError: string;
		onUpgrade: () => void;
	} = $props();
</script>

<div class="border-border bg-card rounded-lg border p-4">
	<h2 class="font-medium">{$t('velocityProxyCard.title')}</h2>
	<p class="text-muted-foreground mt-2 text-xs">
		{$t('velocityProxyCard.currentVersion', { version: proxyStatus.current_version ?? '' })}
	</p>
	{#if proxyStatus.update_available}
		<p class="mt-1 text-xs text-yellow-500">
			{$t('velocityProxyCard.updateAvailable', { latest: proxyStatus.latest_version ?? '' })}
		</p>
		<button
			disabled={upgrading}
			onclick={onUpgrade}
			class="border-border mt-2 rounded-md border px-3 py-1.5 text-sm disabled:opacity-50"
		>
			{upgrading ? $t('velocityProxyCard.upgrading') : $t('velocityProxyCard.upgrade')}
		</button>
	{:else}
		<p class="text-muted-foreground mt-1 text-xs">{$t('velocityProxyCard.upToDate')}</p>
	{/if}
	{#if upgradeError}
		<p class="text-destructive mt-2 text-xs">{upgradeError}</p>
	{/if}
</div>

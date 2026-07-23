<script lang="ts">
	import type { Instance, ProxyBackend, ProxyStatus } from '$lib/api';
	import { api } from '$lib/api';
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
		onUpgrade,
		proxyId,
		instances
	}: {
		proxyStatus: ProxyStatus;
		upgrading: boolean;
		upgradeError: string;
		onUpgrade: () => void;
		proxyId: string | undefined;
		instances: Instance[];
	} = $props();

	// Independently-exposed servers never show up here -- they're not in
	// proxy_backends at all, so there's nothing for this list to reorder for
	// them (see ReconcileProxyMode/addServerToProxy).
	let backends = $state<ProxyBackend[]>([]);
	let backendsError = $state('');
	let savedFlash = $state(false);
	let dragFromID = $state<string | null>(null);
	let savedFlashTimer: ReturnType<typeof setTimeout> | undefined;

	async function loadBackends() {
		if (!proxyId) return;
		try {
			backends = await api.getProxyBackends(proxyId);
			backendsError = '';
		} catch (err) {
			backendsError = err instanceof Error ? err.message : String(err);
		}
	}
	$effect(() => {
		loadBackends();
	});

	function instanceName(id: string) {
		return instances.find((i) => i.id === id)?.name ?? id;
	}

	function flashSaved() {
		savedFlash = true;
		clearTimeout(savedFlashTimer);
		savedFlashTimer = setTimeout(() => (savedFlash = false), 1200);
	}

	// Drops (not every drag-over swap) are what actually save -- each save
	// restarts the live proxy to apply the new order (see
	// applyProxyBackends), which briefly disconnects everyone connected
	// through it, so this only fires once per completed reorder rather than
	// on every intermediate position the row passes over.
	async function handleDrop(targetID: string) {
		const fromID = dragFromID;
		dragFromID = null;
		if (!proxyId || !fromID || fromID === targetID) return;

		const fromIdx = backends.findIndex((b) => b.backend_instance_id === fromID);
		const toIdx = backends.findIndex((b) => b.backend_instance_id === targetID);
		if (fromIdx === -1 || toIdx === -1) return;

		const reordered = [...backends];
		const [moved] = reordered.splice(fromIdx, 1);
		reordered.splice(toIdx, 0, moved);
		reordered.forEach((b, i) => (b.priority = i));
		backends = reordered;

		try {
			backends = await api.setProxyBackends(proxyId, reordered);
			backendsError = '';
			flashSaved();
		} catch (err) {
			backendsError = err instanceof Error ? err.message : String(err);
			await loadBackends();
		}
	}
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

	{#if backends.length > 0}
		<div class="border-border mt-3 border-t pt-3">
			<div class="flex items-center justify-between">
				<span class="text-muted-foreground text-xs">{$t('velocityProxyCard.priorityTitle')}</span>
				<span class="text-xs text-green-500 transition-opacity {savedFlash ? 'opacity-100' : 'opacity-0'}"
					>{$t('velocityProxyCard.saved')}</span
				>
			</div>
			<p class="text-muted-foreground mt-1 mb-2 text-xs">{$t('velocityProxyCard.priorityHint')}</p>
			{#if backendsError}
				<p class="text-destructive mb-2 text-xs">{backendsError}</p>
			{/if}
			<div class="flex flex-col gap-1.5">
				{#each backends as b, i (b.backend_instance_id)}
					<div
						role="listitem"
						draggable="true"
						ondragstart={() => (dragFromID = b.backend_instance_id)}
						ondragover={(e) => e.preventDefault()}
						ondrop={() => handleDrop(b.backend_instance_id)}
						class="border-border bg-background flex cursor-grab items-center gap-2 rounded-md border px-2.5 py-1.5 active:cursor-grabbing"
					>
						<span class="text-muted-foreground text-xs">⠿</span>
						<span class="text-muted-foreground w-3.5 text-xs">{i + 1}</span>
						<div class="min-w-0 flex-1">
							<div class="truncate text-xs">{instanceName(b.backend_instance_id)}</div>
							{#if b.forced_host}
								<div class="text-muted-foreground truncate text-[10px]">{b.forced_host}</div>
							{:else}
								<div class="text-muted-foreground text-[10px]">
									{$t('velocityProxyCard.noSubdomain')}
								</div>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		</div>
	{/if}
</div>

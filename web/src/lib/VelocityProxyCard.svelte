<script lang="ts">
	import type { ProxyStatus } from '$lib/api';

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
	<h2 class="font-medium">Velocity 프록시</h2>
	<p class="text-muted-foreground mt-2 text-xs">현재 버전: {proxyStatus.current_version}</p>
	{#if proxyStatus.update_available}
		<p class="mt-1 text-xs text-yellow-500">
			최신 버전 {proxyStatus.latest_version} 사용 가능 (새 마인크래프트 프로토콜 지원이 추가됐을 수
			있습니다)
		</p>
		<button
			disabled={upgrading}
			onclick={onUpgrade}
			class="border-border mt-2 rounded-md border px-3 py-1.5 text-sm disabled:opacity-50"
		>
			{upgrading ? '업데이트 중... (프록시가 잠시 재시작됩니다)' : '프록시 업데이트'}
		</button>
	{:else}
		<p class="text-muted-foreground mt-1 text-xs">최신 버전입니다.</p>
	{/if}
	{#if upgradeError}
		<p class="text-destructive mt-2 text-xs">{upgradeError}</p>
	{/if}
</div>

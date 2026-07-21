<script lang="ts">
	import type { SwapInfo } from '$lib/api';
	import { t } from '$lib/i18n';

	// 가상 메모리(디스크 스왑파일) -- 라즈베리파이 OS의 zram(RAM 압축 스왑)과
	// 별개로 동작하는 CraftDeck 전용 디스크 기반 스왑. SD카드/eMMC 부팅
	// 환경(swapInfo.supported === false)에서는 이 카드 자체를 아예 숨긴다 --
	// 랜덤 쓰기 성능/수명이 나빠서 켜라고 권할 이유가 없음. 다만 이건
	// "확인해보니 지원 안 함"으로 확정된 경우만이고, 조회 자체가 실패한
	// 경우(swapFetchError)는 구분해서 에러로 보여준다 -- 안 그러면 일시적
	// 네트워크 오류와 "이 하드웨어는 지원 안 함"이 똑같이 카드가 사라지는
	// 걸로 보여서 구분이 안 됐다.
	let {
		swapInfo,
		swapFetchError,
		swapSizeInput = $bindable(),
		swapSaving,
		swapError,
		onSave,
		onDisable
	}: {
		swapInfo: SwapInfo | null;
		swapFetchError: string;
		swapSizeInput: string;
		swapSaving: boolean;
		swapError: string;
		onSave: () => void;
		onDisable: () => void;
	} = $props();
</script>

<div class="border-border bg-card rounded-lg border p-4">
	<h2 class="font-medium">{$t('swapCard.title')}</h2>
	<p class="text-muted-foreground mt-1 text-xs">
		{$t('swapCard.description')}
	</p>
	{#if swapFetchError}
		<p class="text-destructive mt-2 text-xs">
			{$t('swapCard.fetchError', { error: swapFetchError })}
		</p>
	{:else if swapInfo}
		<p class="mt-2 text-xs">
			{#if swapInfo.enabled}
				<span class="text-green-500">{$t('swapCard.statusOn')}</span>{$t('swapCard.statusOnUsage', {
					sizeGb: (swapInfo.size_mb / 1024).toFixed(1),
					usedGb: (swapInfo.used_mb / 1024).toFixed(1)
				})}
			{:else if swapInfo.size_mb > 0}
				<span class="text-muted-foreground">{$t('swapCard.statusOff')}</span>{$t(
					'swapCard.statusOffRemaining',
					{ sizeGb: (swapInfo.size_mb / 1024).toFixed(1) }
				)}
			{:else}
				<span class="text-muted-foreground">{$t('swapCard.statusNotSet')}</span>
			{/if}
		</p>
		<p class="text-muted-foreground mt-1 text-xs">
			{$t('swapCard.freeDisk', { freeGb: (swapInfo.free_disk_mb / 1024).toFixed(1) })}
		</p>
		<div class="mt-2 flex gap-2">
			<div
				class="border-input bg-background flex min-w-0 flex-1 items-center rounded-md border px-2 py-1.5"
			>
				<input
					type="number"
					min="0.1"
					step="0.1"
					bind:value={swapSizeInput}
					placeholder={$t('swapCard.sizePlaceholder')}
					class="min-w-0 flex-1 bg-transparent text-sm outline-none"
				/>
				<span class="text-muted-foreground shrink-0 text-sm">{$t('swapCard.sizeUnit')}</span>
			</div>
			<button
				class="bg-primary text-primary-foreground shrink-0 rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50"
				disabled={swapSaving}
				onclick={onSave}
			>
				{swapSaving ? $t('swapCard.applying') : $t('swapCard.apply')}
			</button>
		</div>
		{#if swapInfo.enabled}
			<button
				class="border-border text-destructive mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
				disabled={swapSaving}
				onclick={onDisable}
			>
				{$t('swapCard.disable')}
			</button>
		{/if}
		{#if swapError}
			<p class="text-destructive mt-2 text-xs">{swapError}</p>
		{/if}
	{:else}
		<p class="text-muted-foreground mt-2 text-xs">{$t('swapCard.loading')}</p>
	{/if}
</div>

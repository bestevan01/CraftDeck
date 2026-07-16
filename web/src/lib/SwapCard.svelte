<script lang="ts">
	import type { SwapInfo } from '$lib/api';

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
	<h2 class="font-medium">가상 메모리 (스왑)</h2>
	<p class="text-muted-foreground mt-1 text-xs">
		라즈베리파이 OS의 zram(RAM 내 압축 스왑)과는 별개로, 실제 디스크 공간을 추가 여유분으로 씁니다.
		커널은 항상 실제 RAM과 zram을 먼저 쓰고, 그걸로도 부족할 때만 이 스왑파일을 사용합니다.
	</p>
	{#if swapFetchError}
		<p class="text-destructive mt-2 text-xs">상태를 불러오지 못했습니다: {swapFetchError}</p>
	{:else if swapInfo}
		<p class="mt-2 text-xs">
			{#if swapInfo.enabled}
				<span class="text-green-500">켜짐</span> -- {(swapInfo.size_mb / 1024).toFixed(1)}GB 중 {(
					swapInfo.used_mb / 1024
				).toFixed(1)}GB 사용 중
			{:else if swapInfo.size_mb > 0}
				<span class="text-muted-foreground">꺼짐</span> (파일은 {(swapInfo.size_mb / 1024).toFixed(
					1
				)}GB로 남아있음)
			{:else}
				<span class="text-muted-foreground">설정 안 됨</span>
			{/if}
		</p>
		<p class="text-muted-foreground mt-1 text-xs">
			여유 공간: {(swapInfo.free_disk_mb / 1024).toFixed(1)}GB (스왑파일 자체 크기 포함)
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
					placeholder="예: 4"
					class="min-w-0 flex-1 bg-transparent text-sm outline-none"
				/>
				<span class="text-muted-foreground shrink-0 text-sm">GB</span>
			</div>
			<button
				class="bg-primary text-primary-foreground shrink-0 rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50"
				disabled={swapSaving}
				onclick={onSave}
			>
				{swapSaving ? '적용 중...' : '적용'}
			</button>
		</div>
		{#if swapInfo.enabled}
			<button
				class="border-border text-destructive mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
				disabled={swapSaving}
				onclick={onDisable}
			>
				끄고 삭제
			</button>
		{/if}
		{#if swapError}
			<p class="text-destructive mt-2 text-xs">{swapError}</p>
		{/if}
	{:else}
		<p class="text-muted-foreground mt-2 text-xs">불러오는 중...</p>
	{/if}
</div>

<script lang="ts">
	import type { SwapInfo, SystemResources } from '$lib/api';

	let {
		resources,
		resourceError,
		swapInfo
	}: {
		resources: SystemResources | null;
		resourceError: string;
		swapInfo: SwapInfo | null;
	} = $props();

	function usagePercent(used: number, total: number) {
		if (total <= 0) return 0;
		return Math.min(100, (used / total) * 100);
	}

	function barClass(percent: number) {
		if (percent >= 90) return 'bg-destructive';
		if (percent >= 75) return 'bg-yellow-500';
		return 'bg-primary';
	}

	// Raspberry Pi SoCs start throttling clock speed around 80-85degC, so
	// that's the destructive threshold here -- 70 is just an early warning.
	function tempTextClass(tempC: number) {
		if (tempC >= 80) return 'text-destructive';
		if (tempC >= 70) return 'text-yellow-500';
		return '';
	}
</script>

<div class="border-border bg-card rounded-lg border p-4">
	<h2 class="font-medium">라즈베리파이 리소스</h2>
	{#if resources}
		{@const swapTotalMB = swapInfo?.enabled ? swapInfo.size_mb : 0}
		{@const swapUsedMB = swapInfo?.enabled ? swapInfo.used_mb : 0}
		{@const memCombinedTotalMB = resources.total_memory_mb + swapTotalMB}
		{@const memRAMPercentOfBar = usagePercent(resources.used_memory_mb, memCombinedTotalMB)}
		{@const memSwapPercentOfBar = usagePercent(swapUsedMB, memCombinedTotalMB)}
		{@const memRAMOwnPercent = usagePercent(resources.used_memory_mb, resources.total_memory_mb)}
		{@const diskPercent = usagePercent(resources.used_disk_mb, resources.total_disk_mb)}
		<div class="mt-3 space-y-4">
			<div>
				<div class="mb-1 flex justify-between text-xs">
					<span class="text-muted-foreground">CPU 사용률</span>
					<span
						>{resources.cpu_percent.toFixed(0)}% ({resources.cpu_count}코어){#if resources.cpu_temp_c !== undefined}<span
								class={tempTextClass(resources.cpu_temp_c)}
								>
								· {resources.cpu_temp_c.toFixed(1)}°C</span
							>{/if}</span
					>
				</div>
				<div class="bg-background h-2 overflow-hidden rounded-full">
					<div
						class="h-full {barClass(resources.cpu_percent)}"
						style="width: {Math.min(100, resources.cpu_percent)}%"
					></div>
				</div>
			</div>
			<div>
				<div class="mb-1 flex justify-between text-xs">
					<span class="text-muted-foreground">
						메모리{#if swapTotalMB > 0}<span class="text-yellow-500"> (+스왑)</span>{/if}
					</span>
					<span
						>{(resources.used_memory_mb / 1024).toFixed(1)}{#if swapUsedMB > 0}<span
								class="text-yellow-500">+{(swapUsedMB / 1024).toFixed(1)}</span
							>{/if}GB / {(memCombinedTotalMB / 1024).toFixed(1)}GB</span
					>
				</div>
				<!-- 라즈베리파이 OS의 zram(RAM 압축 스왑)과 별개인 CraftDeck 자체
					디스크 스왑(FR-46)이 켜져 있으면, 막대 전체 길이를 물리 RAM+스왑
					합산 용량 기준으로 놓고 두 구간으로 나눠 표시한다: 물리 RAM 사용량은
					기존과 같은 임계값 색(barClass), 스왑 사용량 구간은 항상 노란색으로
					구분해서 "지금 스왑까지 파고들었다"는 걸 한눈에 보이게 한다. -->
				<div class="bg-background flex h-2 overflow-hidden rounded-full">
					<div class="h-full {barClass(memRAMOwnPercent)}" style="width: {memRAMPercentOfBar}%"></div>
					{#if memSwapPercentOfBar > 0}
						<div class="h-full bg-yellow-500" style="width: {memSwapPercentOfBar}%"></div>
					{/if}
				</div>
			</div>
			<div>
				<div class="mb-1 flex justify-between text-xs">
					<span class="text-muted-foreground">디스크</span>
					<span
						>{(resources.used_disk_mb / 1024).toFixed(1)}GB / {(
							resources.total_disk_mb / 1024
						).toFixed(1)}GB</span
					>
				</div>
				<div class="bg-background h-2 overflow-hidden rounded-full">
					<div class="h-full {barClass(diskPercent)}" style="width: {diskPercent}%"></div>
				</div>
			</div>
		</div>
	{:else if resourceError}
		<p class="text-destructive mt-3 text-xs">{resourceError}</p>
	{:else}
		<p class="text-muted-foreground mt-3 text-xs">불러오는 중...</p>
	{/if}
</div>

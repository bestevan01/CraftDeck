<script lang="ts">
	import { OVERCLOCK_PRESETS, type BenchmarkStatus, type HardwareInfo } from '$lib/api';

	// Active Cooler가 실제로 붙어있는지 확인된 경우에만 오버클럭 조작이
	// 가능하다 (internal/hardware.DetectActiveCooler) -- 감지 안 된 경우에도
	// 카드 자체는 보여주되(스왑카드와 달리 "나중에 쿨러를 달았을 때 다시
	// 확인"할 방법이 있어야 해서) 조작부는 숨기고 "다시 감지" 버튼만 노출.
	let {
		hardwareInfo,
		hardwareFetchError,
		redetecting,
		onRedetect,
		overclockForm = $bindable(),
		overclockSaving,
		overclockError,
		onApplyOverclock,
		rebooting,
		benchmarkStatus,
		benchmarkStarting,
		onStartBenchmark,
		onRevertOverclock
	}: {
		hardwareInfo: HardwareInfo | null;
		hardwareFetchError: string;
		redetecting: boolean;
		onRedetect: () => void;
		overclockForm: { preset: string; armFreq: string; overVoltageDeltaUV: string };
		overclockSaving: boolean;
		overclockError: string;
		onApplyOverclock: () => void;
		rebooting: boolean;
		benchmarkStatus: BenchmarkStatus | null;
		benchmarkStarting: boolean;
		onStartBenchmark: () => void;
		onRevertOverclock: () => void;
	} = $props();
</script>

<div class="border-border bg-card rounded-lg border p-4">
	<h2 class="font-medium">오버클럭</h2>
	<p class="text-muted-foreground mt-1 text-xs">
		공식 Active Cooler가 실제로 장착된 게 확인된 경우에만 사용할 수 있습니다. 적용하면 곧바로
		재부팅까지 진행되며(실행 중인 서버는 먼저 안전하게 종료됩니다), 이후 안정성 테스트로
		언더볼트/쓰로틀링 없이 동작하는지 직접 확인할 수 있습니다.
	</p>

	{#if hardwareFetchError}
		<p class="text-destructive mt-2 text-xs">상태를 불러오지 못했습니다: {hardwareFetchError}</p>
	{:else if !hardwareInfo}
		<p class="text-muted-foreground mt-2 text-xs">불러오는 중...</p>
	{:else if !hardwareInfo.cooler_detected}
		<p class="text-muted-foreground mt-2 text-xs">
			Active Cooler가 감지되지 않았습니다. 쿨러를 장착한 뒤 다시 감지해보세요.
		</p>
		<button
			class="border-border mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
			disabled={redetecting}
			onclick={onRedetect}
		>
			{redetecting ? '감지 중...' : '다시 감지'}
		</button>
	{:else}
		<div class="mt-2 flex gap-2">
			<div class="relative flex-1">
				<select
					bind:value={overclockForm.preset}
					class="border-input bg-background w-full appearance-none rounded-md border py-1.5 pl-3 pr-8 text-sm"
				>
					<option value="__none__">사용 안 함 (기본값)</option>
					{#each OVERCLOCK_PRESETS.filter((p) => p.name !== 'default') as p (p.name)}
						<option value={p.name}>{p.label} ({p.arm_freq_mhz}MHz)</option>
					{/each}
					<option value="custom">커스텀</option>
				</select>
				<svg
					class="text-muted-foreground pointer-events-none absolute top-1/2 right-3 h-4 w-4 -translate-y-1/2"
					viewBox="0 0 20 20"
					fill="none"
					stroke="currentColor"
					stroke-width="1.5"
					><path d="M5 7l5 5 5-5" stroke-linecap="round" stroke-linejoin="round" /></svg
				>
			</div>
		</div>

		{#if overclockForm.preset === 'custom'}
			<div class="mt-2 grid grid-cols-2 gap-2">
				<div>
					<label class="text-muted-foreground mb-1 block text-xs" for="oc-arm-freq">arm_freq (MHz)</label>
					<input
						id="oc-arm-freq"
						type="number"
						min="2400"
						max="3200"
						step="50"
						bind:value={overclockForm.armFreq}
						class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-sm"
					/>
				</div>
				<div>
					<label class="text-muted-foreground mb-1 block text-xs" for="oc-over-voltage"
						>over_voltage_delta (µV)</label
					>
					<input
						id="oc-over-voltage"
						type="number"
						min="0"
						max="100000"
						step="5000"
						bind:value={overclockForm.overVoltageDeltaUV}
						class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-sm"
					/>
				</div>
			</div>
		{/if}

		<button
			class="bg-primary text-primary-foreground mt-2 rounded-md px-3 py-1.5 text-xs font-medium disabled:opacity-50"
			disabled={overclockSaving || rebooting}
			onclick={onApplyOverclock}
		>
			{rebooting ? '재부팅 중...' : overclockSaving ? '적용 중...' : '적용 후 재부팅'}
		</button>
		{#if rebooting}
			<p class="text-muted-foreground mt-2 text-xs">
				재부팅 중입니다. 완료되면 자동으로 새로고침됩니다.
			</p>
		{/if}
		{#if overclockError}
			<p class="text-destructive mt-2 text-xs">{overclockError}</p>
		{/if}

		{#if hardwareInfo.overclock_enabled}
			<p class="text-muted-foreground mt-2 text-xs">
				현재 적용됨: arm_freq={hardwareInfo.overclock_arm_freq}MHz, over_voltage_delta={hardwareInfo.overclock_over_voltage_delta}µV
			</p>
		{/if}

		<div class="border-border mt-3 border-t pt-3">
			<h3 class="text-sm font-medium">안정성 테스트</h3>
			{#if benchmarkStatus?.running}
				<p class="text-muted-foreground mt-1 text-xs">
					진행 중... {benchmarkStatus.elapsed_sec}/{benchmarkStatus.total_sec}초, 현재 온도 {benchmarkStatus.temp_c.toFixed(
						1
					)}°C
				</p>
			{:else}
				<button
					class="border-border mt-1 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
					disabled={benchmarkStarting}
					onclick={onStartBenchmark}
				>
					{benchmarkStarting ? '시작 중...' : '테스트 시작 (약 90초)'}
				</button>
				{#if benchmarkStatus?.result === 'pass'}
					<p class="mt-2 text-xs text-green-500">
						통과 — 언더볼트/쓰로틀링 없이 안정적으로 동작했습니다.
					</p>
				{:else if benchmarkStatus?.result === 'fail'}
					<p class="text-destructive mt-2 text-xs">
						실패 — {benchmarkStatus.under_voltage_detected ? '언더볼트' : ''}
						{benchmarkStatus.under_voltage_detected && benchmarkStatus.throttled_detected ? ' / ' : ''}
						{benchmarkStatus.throttled_detected ? '쓰로틀링' : ''}이 감지됐습니다. 값을 낮추거나
						되돌리는 걸 권장합니다.
					</p>
					<button
						class="border-border text-destructive mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
						disabled={overclockSaving || rebooting}
						onclick={onRevertOverclock}
					>
						안전한 값으로 되돌리기
					</button>
				{/if}
			{/if}
		</div>
	{/if}
</div>

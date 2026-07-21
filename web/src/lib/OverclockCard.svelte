<script lang="ts">
	import { OVERCLOCK_PRESETS, type BenchmarkStatus, type HardwareInfo } from '$lib/api';
	import { t } from '$lib/i18n';

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
	<h2 class="font-medium">{$t('overclockCard.title')}</h2>
	<p class="text-muted-foreground mt-1 text-xs">
		{$t('overclockCard.description')}
	</p>

	{#if hardwareFetchError}
		<p class="text-destructive mt-2 text-xs">{$t('overclockCard.fetchError', { error: hardwareFetchError })}</p>
	{:else if !hardwareInfo}
		<p class="text-muted-foreground mt-2 text-xs">{$t('overclockCard.loading')}</p>
	{:else if !hardwareInfo.cooler_detected}
		<p class="text-muted-foreground mt-2 text-xs">
			{$t('overclockCard.coolerNotDetected')}
		</p>
		<button
			class="border-border mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
			disabled={redetecting}
			onclick={onRedetect}
		>
			{redetecting ? $t('overclockCard.redetecting') : $t('overclockCard.redetect')}
		</button>
	{:else}
		<div class="mt-2 flex gap-2">
			<div class="relative flex-1">
				<select
					bind:value={overclockForm.preset}
					class="border-input bg-background w-full appearance-none rounded-md border py-1.5 pl-3 pr-8 text-sm"
				>
					<option value="__none__">{$t('overclockCard.presetNone')}</option>
					{#each OVERCLOCK_PRESETS.filter((p) => p.name !== 'default') as p (p.name)}
						<option value={p.name}>{p.label} ({p.arm_freq_mhz}MHz)</option>
					{/each}
					<option value="custom">{$t('overclockCard.presetCustom')}</option>
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
					<label class="text-muted-foreground mb-1 block text-xs" for="oc-arm-freq">{$t('overclockCard.clockLabel')}</label>
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
						>{$t('overclockCard.voltageOffsetLabel')}</label
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
			{rebooting ? $t('overclockCard.rebooting') : overclockSaving ? $t('overclockCard.applying') : $t('overclockCard.applyAndReboot')}
		</button>
		{#if rebooting}
			<p class="text-muted-foreground mt-2 text-xs">
				{$t('overclockCard.rebootingNotice')}
			</p>
		{/if}
		{#if overclockError}
			<p class="text-destructive mt-2 text-xs">{overclockError}</p>
		{/if}

		{#if hardwareInfo.overclock_enabled}
			<p class="text-muted-foreground mt-2 text-xs">
				{$t('overclockCard.currentApplied', {
					armFreq: hardwareInfo.overclock_arm_freq ?? 0,
					voltageDelta: hardwareInfo.overclock_over_voltage_delta ?? 0
				})}
			</p>
		{/if}

		<div class="border-border mt-3 border-t pt-3">
			<h3 class="text-sm font-medium">{$t('overclockCard.stabilityTest')}</h3>
			{#if benchmarkStatus?.running}
				<p class="text-muted-foreground mt-1 text-xs">
					{$t('overclockCard.testInProgress', {
						elapsed: benchmarkStatus.elapsed_sec,
						total: benchmarkStatus.total_sec,
						temp: benchmarkStatus.current_temp_c.toFixed(1)
					})}
				</p>
			{:else}
				<button
					class="border-border mt-1 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
					disabled={benchmarkStarting}
					onclick={onStartBenchmark}
				>
					{benchmarkStarting ? $t('overclockCard.startingTest') : $t('overclockCard.startTest')}
				</button>
				{#if benchmarkStatus?.result === 'pass' || benchmarkStatus?.result === 'fail'}
					<p class="text-muted-foreground mt-2 text-xs">
						{$t('overclockCard.tempSummary', {
							avg: benchmarkStatus.avg_temp_c.toFixed(1),
							min: benchmarkStatus.min_temp_c.toFixed(1),
							max: benchmarkStatus.max_temp_c.toFixed(1)
						})}
					</p>
				{/if}
				{#if benchmarkStatus?.result === 'pass'}
					<p class="mt-2 text-xs text-green-500">
						{$t('overclockCard.testPass')}
					</p>
				{:else if benchmarkStatus?.result === 'fail'}
					<p class="text-destructive mt-2 text-xs">
						{$t('overclockCard.testFailMessage', {
							issues: [
								benchmarkStatus.under_voltage_detected ? $t('overclockCard.testFailUnderVoltage') : '',
								benchmarkStatus.throttled_detected ? $t('overclockCard.testFailThrottled') : ''
							]
								.filter(Boolean)
								.join(' / ')
						})}
					</p>
					<button
						class="border-border text-destructive mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
						disabled={overclockSaving || rebooting}
						onclick={onRevertOverclock}
					>
						{$t('overclockCard.revertToSafe')}
					</button>
				{/if}
			{/if}
		</div>
	{/if}
</div>

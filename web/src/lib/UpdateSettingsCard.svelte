<script lang="ts">
	import type { UpdateSettings } from '$lib/api';

	// stable/beta/canary 구독 채널 + 업데이트 확인 주기. 채널을 바꾸면
	// 백엔드가 /etc/apt/sources.list.d/craftdeck.list를 재작성하고
	// apt-get update까지 실행하므로(update.ApplySourcesList), 적용 후에는
	// 부모가 systemVersion을 다시 불러 최신 버전 표시를 갱신해야 한다.
	let {
		settings,
		fetchError,
		form = $bindable(),
		saving,
		error,
		onSave
	}: {
		settings: UpdateSettings | null;
		fetchError: string;
		form: { channel: string; check_frequency: string };
		saving: boolean;
		error: string;
		onSave: () => void;
	} = $props();

	const checkFrequencyLabels: Record<string, string> = {
		every_visit: '접속마다',
		daily: '매일',
		weekly: '매주',
		monthly: '매달'
	};
</script>

<div class="border-border bg-card mt-6 rounded-lg border p-4">
	<h2 class="font-medium">업데이트 설정</h2>
	<p class="text-muted-foreground mt-1 text-xs">
		구독할 채널과 업데이트 확인 주기를 정합니다. beta/canary는 stable보다 먼저 새 기능을 받아볼 수
		있지만 덜 검증된 빌드일 수 있습니다.
	</p>

	{#if fetchError}
		<p class="text-destructive mt-2 text-xs">불러오지 못했습니다: {fetchError}</p>
	{:else if !settings}
		<p class="text-muted-foreground mt-2 text-xs">불러오는 중...</p>
	{:else}
		<div class="mt-2 grid grid-cols-2 gap-2">
			<div>
				<label class="text-muted-foreground mb-1 block text-xs" for="update-channel">채널</label>
				<select
					id="update-channel"
					bind:value={form.channel}
					class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-sm"
				>
					<option value="stable">stable</option>
					<option value="beta">beta</option>
					<option value="canary">canary</option>
				</select>
			</div>
			<div>
				<label class="text-muted-foreground mb-1 block text-xs" for="update-frequency"
					>확인 주기</label
				>
				<select
					id="update-frequency"
					bind:value={form.check_frequency}
					class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-sm"
				>
					<option value="every_visit">접속마다</option>
					<option value="daily">매일</option>
					<option value="weekly">매주</option>
					<option value="monthly">매달</option>
				</select>
			</div>
		</div>

		<button
			class="bg-primary text-primary-foreground mt-2 rounded-md px-3 py-1.5 text-xs font-medium disabled:opacity-50"
			disabled={saving}
			onclick={onSave}
		>
			{saving ? '적용 중...' : '적용'}
		</button>
		{#if error}
			<p class="text-destructive mt-2 text-xs">{error}</p>
		{/if}

		<p class="text-muted-foreground mt-2 text-xs">
			현재 채널: {settings.channel} · 확인 주기: {checkFrequencyLabels[settings.check_frequency] ??
				settings.check_frequency}
		</p>
	{/if}
</div>

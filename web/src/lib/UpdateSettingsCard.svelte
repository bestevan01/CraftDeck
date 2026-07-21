<script lang="ts">
	import type { UpdateSettings } from '$lib/api';
	import { t } from '$lib/i18n';

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
		onSave,
		checkingNow,
		checkNowMessage,
		onCheckNow
	}: {
		settings: UpdateSettings | null;
		fetchError: string;
		form: { channel: string; check_frequency: string };
		saving: boolean;
		error: string;
		onSave: () => void;
		checkingNow: boolean;
		checkNowMessage: string;
		onCheckNow: () => void;
	} = $props();

	const checkFrequencyLabels = $derived<Record<string, string>>({
		every_visit: $t('updateSettingsCard.frequencyEveryVisit'),
		daily: $t('updateSettingsCard.frequencyDaily'),
		weekly: $t('updateSettingsCard.frequencyWeekly'),
		monthly: $t('updateSettingsCard.frequencyMonthly')
	});
</script>

<div class="border-border bg-card mt-6 rounded-lg border p-4">
	<h2 class="font-medium">{$t('updateSettingsCard.title')}</h2>
	<p class="text-muted-foreground mt-1 text-xs">
		{$t('updateSettingsCard.description')}
	</p>

	{#if fetchError}
		<p class="text-destructive mt-2 text-xs">
			{$t('updateSettingsCard.fetchError', { error: fetchError })}
		</p>
	{:else if !settings}
		<p class="text-muted-foreground mt-2 text-xs">{$t('updateSettingsCard.loading')}</p>
	{:else}
		<div class="mt-2 grid grid-cols-2 gap-2">
			<div>
				<label class="text-muted-foreground mb-1 block text-xs" for="update-channel"
					>{$t('updateSettingsCard.channelLabel')}</label
				>
				<div class="relative">
					<select
						id="update-channel"
						bind:value={form.channel}
						class="border-input bg-background w-full appearance-none rounded-md border py-1.5 pl-3 pr-8 text-sm"
					>
						<option value="stable">stable</option>
						<option value="beta">beta</option>
						<option value="canary">canary</option>
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
			<div>
				<label class="text-muted-foreground mb-1 block text-xs" for="update-frequency"
					>{$t('updateSettingsCard.frequencyLabel')}</label
				>
				<div class="relative">
					<select
						id="update-frequency"
						bind:value={form.check_frequency}
						class="border-input bg-background w-full appearance-none rounded-md border py-1.5 pl-3 pr-8 text-sm"
					>
						<option value="every_visit">{$t('updateSettingsCard.frequencyEveryVisit')}</option>
						<option value="daily">{$t('updateSettingsCard.frequencyDaily')}</option>
						<option value="weekly">{$t('updateSettingsCard.frequencyWeekly')}</option>
						<option value="monthly">{$t('updateSettingsCard.frequencyMonthly')}</option>
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
		</div>

		<button
			class="bg-primary text-primary-foreground mt-2 rounded-md px-3 py-1.5 text-xs font-medium disabled:opacity-50"
			disabled={saving}
			onclick={onSave}
		>
			{saving ? $t('updateSettingsCard.applying') : $t('updateSettingsCard.apply')}
		</button>
		{#if error}
			<p class="text-destructive mt-2 text-xs">{error}</p>
		{/if}

		<p class="text-muted-foreground mt-2 text-xs">
			{$t('updateSettingsCard.currentStatus', {
				channel: settings.channel,
				frequency: checkFrequencyLabels[settings.check_frequency] ?? settings.check_frequency
			})}
		</p>

		<div class="border-border mt-3 border-t pt-3">
			<button
				class="border-border rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
				disabled={checkingNow}
				onclick={onCheckNow}
			>
				{checkingNow ? $t('updateSettingsCard.checkingNow') : $t('updateSettingsCard.checkNow')}
			</button>
			{#if checkNowMessage}
				<p class="text-muted-foreground mt-2 text-xs">{checkNowMessage}</p>
			{/if}
		</div>
	{/if}
</div>

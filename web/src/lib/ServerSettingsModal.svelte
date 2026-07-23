<script lang="ts">
	import type { Instance } from '$lib/api';
	import MemorySlider from '$lib/MemorySlider.svelte';
	import { t } from '$lib/i18n';

	let {
		open,
		inst,
		settingsMemoryGB = $bindable(1),
		settingsCpu = $bindable(0),
		settingsGamePort = $bindable(0),
		canEditGamePort,
		maxMemoryGB,
		ramBoundaryGB,
		settingsError,
		settingsSaving,
		onSave,
		onClose
	}: {
		open: boolean;
		inst: Instance;
		settingsMemoryGB: number;
		settingsCpu: number;
		settingsGamePort: number;
		canEditGamePort: boolean;
		maxMemoryGB: number;
		ramBoundaryGB: number;
		settingsError: string;
		settingsSaving: boolean;
		onSave: () => void;
		onClose: () => void;
	} = $props();

	let pressedBackdrop = false;
</script>

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-8"
		onmousedown={(e) => (pressedBackdrop = e.target === e.currentTarget)}
		onclick={(e) => {
			if (pressedBackdrop && e.target === e.currentTarget) onClose();
		}}
		onkeydown={(e) => {
			if (e.key === 'Escape') onClose();
		}}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="bg-card border-border w-full max-w-md rounded-lg border p-4 shadow-lg">
			<div class="mb-1 flex items-center justify-between">
				<h2 class="font-medium">{$t('manageTab.serverSettings.title')}</h2>
				<button type="button" class="text-muted-foreground text-sm" onclick={onClose}>&times;</button>
			</div>

			<div class="mt-3 grid grid-cols-1 gap-3">
				{#if inst.kind === 'proxy'}
					<div>
						<span class="text-muted-foreground mb-1 block text-xs"
							>{$t('manageTab.serverSettings.memoryAllocLabel')}</span
						>
						<p class="mt-1.5 text-sm">{$t('manageTab.serverSettings.memoryFixed')}</p>
					</div>
				{:else}
					<div>
						<label class="text-muted-foreground mb-1 block text-xs" for="settings-memory">
							{$t('manageTab.serverSettings.memoryLabelPrefix')}{$t('manageTab.serverSettings.memoryLabelValue', {
								current: settingsMemoryGB,
								max: maxMemoryGB
							})}{#if ramBoundaryGB < maxMemoryGB}<span class="text-yellow-500"
									>{$t('manageTab.serverSettings.swapIncluded', { swap: maxMemoryGB - ramBoundaryGB })}</span
								>{/if})
						</label>
						<MemorySlider id="settings-memory" bind:value={settingsMemoryGB} maxGB={maxMemoryGB} {ramBoundaryGB} />
					</div>
				{/if}
				<div>
					<label class="text-muted-foreground mb-1 block text-xs" for="settings-cpu">
						{$t('manageTab.serverSettings.cpuLabelPrefix')}{settingsCpu > 0
							? `${settingsCpu}%`
							: $t('manageTab.common.unlimited')})
					</label>
					<input
						id="settings-cpu"
						type="range"
						min="0"
						max="100"
						step="5"
						bind:value={settingsCpu}
						class="w-full"
					/>
				</div>
				{#if inst.kind === 'server'}
					<div>
						<label class="text-muted-foreground mb-1 block text-xs" for="settings-port">
							{$t('manageTab.serverSettings.gamePortLabel')}
						</label>
						{#if canEditGamePort}
							<input
								id="settings-port"
								type="number"
								min="1024"
								max="65535"
								bind:value={settingsGamePort}
								class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-sm"
							/>
						{:else}
							<p class="mt-1.5 text-sm">{inst.game_port}</p>
							<p class="text-muted-foreground mt-1 text-xs">
								{$t('manageTab.serverSettings.gamePortLockedNote')}
							</p>
						{/if}
					</div>
				{/if}
			</div>

			{#if settingsError}
				<p class="text-destructive mt-2 text-xs">{settingsError}</p>
			{/if}

			<div class="mt-3 flex gap-2">
				<button
					class="bg-primary text-primary-foreground rounded-md px-3 py-1.5 text-xs font-medium disabled:opacity-50"
					disabled={settingsSaving}
					onclick={onSave}>{settingsSaving ? $t('manageTab.serverSettings.saving') : $t('manageTab.serverSettings.saveButton')}</button
				>
				<button class="border-border rounded-md border px-3 py-1.5 text-xs" onclick={onClose}
					>{$t('manageTab.serverSettings.cancelButton')}</button
				>
			</div>
		</div>
	</div>
{/if}

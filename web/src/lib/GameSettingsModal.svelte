<script lang="ts">
	import type { ServerSetting } from '$lib/api';
	import { t } from '$lib/i18n';

	let {
		open,
		settings,
		edits = $bindable({}),
		loading,
		error,
		saving,
		saved,
		onSave,
		onClose
	}: {
		open: boolean;
		settings: ServerSetting[];
		edits: Record<string, string>;
		loading: boolean;
		error: string;
		saving: boolean;
		saved: boolean;
		onSave: () => void;
		onClose: () => void;
	} = $props();

	const enumOptionKeys: Record<string, Record<string, string>> = {
		difficulty: {
			peaceful: 'gameSettingsModal.difficulty.peaceful',
			easy: 'gameSettingsModal.difficulty.easy',
			normal: 'gameSettingsModal.difficulty.normal',
			hard: 'gameSettingsModal.difficulty.hard'
		},
		gamemode: {
			survival: 'gameSettingsModal.gamemode.survival',
			creative: 'gameSettingsModal.gamemode.creative',
			adventure: 'gameSettingsModal.gamemode.adventure',
			spectator: 'gameSettingsModal.gamemode.spectator'
		}
	};
	function enumOptionLabel(settingKey: string, value: string) {
		const key = enumOptionKeys[settingKey]?.[value];
		return key ? $t(key) : value;
	}

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
		<div
			class="bg-card border-border flex max-h-[80vh] w-full max-w-2xl flex-col rounded-lg border p-4 shadow-lg"
		>
			<div class="mb-1 flex shrink-0 items-center justify-between">
				<h2 class="font-medium">{$t('gameSettingsModal.title')}</h2>
				<button type="button" class="text-muted-foreground text-sm" onclick={onClose}>&times;</button>
			</div>
			<p class="text-muted-foreground mb-3 shrink-0 text-xs">
				{$t('gameSettingsModal.descriptionPre')}
				<code>server.properties</code>{$t('gameSettingsModal.descriptionPost')}
			</p>
			{#if loading}
				<p class="text-muted-foreground text-xs">{$t('gameSettingsModal.loading')}</p>
			{:else if error && settings.length === 0}
				<p class="text-destructive text-xs">{$t('gameSettingsModal.loadError', { error })}</p>
			{:else}
				<div class="min-h-0 flex-1 overflow-y-auto">
					<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
						{#each settings as setting (setting.key)}
							<div>
								<label
									class="text-muted-foreground mb-1 flex items-center gap-1 text-xs"
									for="gs-{setting.key}"
								>
									<span>{setting.label}</span>
									{#if setting.description}
										<span class="group relative inline-flex">
											<span
												class="border-muted-foreground text-muted-foreground inline-flex h-3.5 w-3.5 cursor-help items-center justify-center rounded-full border text-[9px] leading-none"
												>?</span
											>
											<span
												class="bg-popover text-popover-foreground border-border pointer-events-none absolute bottom-full left-1/2 z-10 mb-1.5 w-56 -translate-x-1/2 rounded-md border p-2 text-xs opacity-0 shadow-lg transition-opacity group-hover:opacity-100"
												>{setting.description}</span
											>
										</span>
									{/if}
								</label>
								{#if setting.type === 'bool'}
									<div class="relative">
										<select
											id="gs-{setting.key}"
											bind:value={edits[setting.key]}
											class="border-input bg-background w-full appearance-none rounded-md border py-1.5 pl-3 pr-8 text-sm"
										>
											<option value="true">{$t('gameSettingsModal.boolOn')}</option>
											<option value="false">{$t('gameSettingsModal.boolOff')}</option>
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
								{:else if setting.type === 'enum'}
									<div class="relative">
										<select
											id="gs-{setting.key}"
											bind:value={edits[setting.key]}
											class="border-input bg-background w-full appearance-none rounded-md border py-1.5 pl-3 pr-8 text-sm"
										>
											{#each setting.options ?? [] as opt (opt)}
												<option value={opt}>{enumOptionLabel(setting.key, opt)}</option>
											{/each}
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
								{:else if setting.type === 'int'}
									<input
										id="gs-{setting.key}"
										type="number"
										bind:value={edits[setting.key]}
										class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-sm"
									/>
								{:else}
									<input
										id="gs-{setting.key}"
										type="text"
										bind:value={edits[setting.key]}
										class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-sm"
									/>
								{/if}
							</div>
						{/each}
					</div>
				</div>
				{#if error}
					<p class="text-destructive mt-2 shrink-0 text-xs">{error}</p>
				{/if}
				{#if saved}
					<p class="mt-2 shrink-0 text-xs text-green-500">{$t('gameSettingsModal.saved')}</p>
				{/if}
				<button
					class="border-border mt-3 shrink-0 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
					disabled={saving}
					onclick={onSave}
				>
					{saving ? $t('gameSettingsModal.saving') : $t('gameSettingsModal.save')}
				</button>
			{/if}
		</div>
	</div>
{/if}

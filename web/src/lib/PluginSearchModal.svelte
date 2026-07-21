<script lang="ts">
	import type { PluginSearchHit } from '$lib/api';
	import { t } from '$lib/i18n';

	let {
		open = $bindable(false),
		loaderLabel,
		query = $bindable(''),
		results,
		error,
		searching,
		installingProjectId,
		onSearch,
		onInstall
	}: {
		open: boolean;
		loaderLabel: string;
		query: string;
		results: PluginSearchHit[];
		error: string;
		searching: boolean;
		installingProjectId: string | null;
		onSearch: (e: SubmitEvent) => void;
		onInstall: (projectId: string) => void;
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
			if (pressedBackdrop && e.target === e.currentTarget) open = false;
		}}
		onkeydown={(e) => {
			if (e.key === 'Escape') open = false;
		}}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="bg-card border-border flex max-h-[80vh] w-full max-w-lg flex-col rounded-lg border p-4 shadow-lg"
		>
			<div class="mb-3 flex items-center justify-between">
				<h2 class="font-medium">{$t('pluginSearchModal.title', { loaderLabel })}</h2>
				<button type="button" class="text-muted-foreground text-sm" onclick={() => (open = false)}
					>&times;</button
				>
			</div>
			<form class="flex gap-2" onsubmit={onSearch}>
				<input
					bind:value={query}
					placeholder={$t('pluginSearchModal.searchPlaceholder', { loaderLabel })}
					class="border-input bg-background w-full min-w-0 flex-1 rounded-md border px-3 py-2 text-sm"
				/>
				<button
					type="submit"
					disabled={searching}
					class="border-border shrink-0 rounded-md border px-3 py-1.5 text-sm"
				>
					{searching ? $t('pluginSearchModal.searching') : $t('pluginSearchModal.search')}
				</button>
			</form>
			{#if error}
				<p class="text-destructive mt-2 text-xs">{error}</p>
			{/if}
			<div class="mt-2 flex-1 space-y-1.5 overflow-y-auto">
				{#each results as hit (hit.project_id)}
					<div
						class="border-border flex items-center justify-between rounded-md border px-2 py-1.5 text-xs"
					>
						<div class="min-w-0">
							<span class="font-medium">{hit.title}</span>
							<span class="text-muted-foreground ml-2">
								{$t('pluginSearchModal.downloads', { count: hit.downloads.toLocaleString() })}
							</span>
							<p class="text-muted-foreground truncate">{hit.description}</p>
						</div>
						<button
							class="border-border ml-2 shrink-0 rounded-md border px-2 py-1 text-xs"
							disabled={installingProjectId === hit.project_id}
							onclick={() => onInstall(hit.project_id)}
						>
							{installingProjectId === hit.project_id
								? $t('pluginSearchModal.installing')
								: $t('pluginSearchModal.install')}
						</button>
					</div>
				{/each}
			</div>
		</div>
	</div>
{/if}

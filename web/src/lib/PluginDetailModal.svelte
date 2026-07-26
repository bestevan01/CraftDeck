<script lang="ts">
	import type { PluginProject, PluginVersion } from '$lib/api';
	import { t } from '$lib/i18n';

	let {
		open = $bindable(false),
		project,
		versions,
		loading,
		error,
		installingVersionId,
		installError,
		onInstallVersion,
		onClose
	}: {
		open: boolean;
		project: PluginProject | null;
		versions: PluginVersion[];
		loading: boolean;
		error: string;
		installingVersionId: string | null;
		installError: string;
		onInstallVersion: (versionId: string) => void;
		onClose: () => void;
	} = $props();

	let pressedBackdrop = false;

	// Release first, then Beta, then Alpha -- each already newest-first
	// within its own group since the backend passes versions through in
	// Modrinth's own (date_published descending) order.
	const channelOrder: PluginVersion['version_type'][] = ['release', 'beta', 'alpha'];
	let grouped = $derived(
		channelOrder
			.map((type) => ({ type, versions: versions.filter((v) => v.version_type === type) }))
			.filter((g) => g.versions.length > 0)
	);

	function channelLabel(type: PluginVersion['version_type']) {
		return type === 'release'
			? $t('pluginDetailModal.channelRelease')
			: type === 'beta'
				? $t('pluginDetailModal.channelBeta')
				: $t('pluginDetailModal.channelAlpha');
	}
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
				<h2 class="font-medium">{project?.title ?? $t('pluginDetailModal.title')}</h2>
				<button type="button" class="text-muted-foreground text-sm" onclick={onClose}>&times;</button>
			</div>

			{#if loading}
				<p class="text-muted-foreground text-xs">{$t('pluginDetailModal.loading')}</p>
			{:else if error}
				<p class="text-destructive text-xs">{error}</p>
			{:else if project}
				<div class="min-h-0 flex-1 overflow-y-auto">
					{#if project.categories.length > 0}
						<div class="mt-2 flex flex-wrap gap-1.5">
							{#each project.categories as category (category)}
								<span class="bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-[10px]"
									>{category}</span
								>
							{/each}
						</div>
					{/if}
					<p class="text-muted-foreground mt-2 text-xs">{project.description}</p>

					{#if installError}
						<p class="text-destructive mt-2 text-xs">{installError}</p>
					{/if}

					<div class="mt-3 space-y-3">
						{#each grouped as group (group.type)}
							<div>
								<span class="text-muted-foreground mb-1.5 block text-xs font-medium"
									>{channelLabel(group.type)}</span
								>
								<div class="space-y-1.5">
									{#each group.versions as version (version.id)}
										<div
											class="border-border flex items-center justify-between rounded-md border px-2 py-1.5 text-xs"
										>
											<div class="min-w-0">
												<span title={version.name}>{version.version_number}</span>
												<span class="text-muted-foreground ml-2 truncate">
													{version.game_versions.join(', ')}
												</span>
											</div>
											<button
												class="border-border ml-2 shrink-0 rounded-md border px-2 py-1 text-xs disabled:opacity-50"
												disabled={installingVersionId === version.id}
												onclick={() => onInstallVersion(version.id)}
											>
												{installingVersionId === version.id
													? $t('pluginDetailModal.installing')
													: $t('pluginDetailModal.install')}
											</button>
										</div>
									{/each}
								</div>
							</div>
						{:else}
							<p class="text-muted-foreground text-xs">{$t('pluginDetailModal.noVersions')}</p>
						{/each}
					</div>
				</div>
			{/if}
		</div>
	</div>
{/if}

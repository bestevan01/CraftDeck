<script lang="ts">
	import type { PluginProject, PluginVersion } from '$lib/api';
	import MarkdownContent from '$lib/MarkdownContent.svelte';
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
	let activeTab = $state<'description' | 'changelog' | 'versions'>('description');

	// This component instance stays mounted (just hidden) whenever `open`
	// goes false, so activeTab would otherwise keep whatever tab it was on
	// the last time the modal closed -- confirmed: reopening it on a
	// different mod still showed whatever tab was open before. Always land
	// on 설명 first regardless of where the operator left off.
	$effect(() => {
		if (open) activeTab = 'description';
	});

	// Release first, then Beta, then Alpha -- each already newest-first
	// within its own group since the backend passes versions through in
	// Modrinth's own (date_published descending) order.
	const channelOrder: PluginVersion['version_type'][] = ['release', 'beta', 'alpha'];
	let grouped = $derived(
		channelOrder
			.map((type) => ({ type, versions: versions.filter((v) => v.version_type === type) }))
			.filter((g) => g.versions.length > 0)
	);

	// Newest first, same ordering the backend already returns -- the
	// changelog timeline reads top-to-bottom like Modrinth's own version
	// history rather than needing its own sort.
	let changelogVersions = $derived(versions.filter((v) => v.changelog));

	function channelLabel(type: PluginVersion['version_type']) {
		return type === 'release'
			? $t('pluginDetailModal.channelRelease')
			: type === 'beta'
				? $t('pluginDetailModal.channelBeta')
				: $t('pluginDetailModal.channelAlpha');
	}

	function channelDotClass(type: PluginVersion['version_type']) {
		return type === 'release' ? 'bg-green-500' : type === 'beta' ? 'bg-yellow-500' : 'bg-red-500';
	}

	function channelTextClass(type: PluginVersion['version_type']) {
		return type === 'release'
			? 'text-green-500'
			: type === 'beta'
				? 'text-yellow-500'
				: 'text-red-500';
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
				{#if installError}
					<p class="text-destructive mt-1 shrink-0 text-xs">{installError}</p>
				{/if}

				<div class="border-border mt-2 flex shrink-0 gap-1 border-b">
					<button
						class="border-b-2 px-3 py-1.5 text-xs {activeTab === 'description'
							? 'border-primary font-medium'
							: 'text-muted-foreground border-transparent'}"
						onclick={() => (activeTab = 'description')}
						>{$t('pluginDetailModal.tabDescription')}</button
					>
					<button
						class="border-b-2 px-3 py-1.5 text-xs {activeTab === 'changelog'
							? 'border-primary font-medium'
							: 'text-muted-foreground border-transparent'}"
						onclick={() => (activeTab = 'changelog')}
						>{$t('pluginDetailModal.tabChangelog')}</button
					>
					<button
						class="border-b-2 px-3 py-1.5 text-xs {activeTab === 'versions'
							? 'border-primary font-medium'
							: 'text-muted-foreground border-transparent'}"
						onclick={() => (activeTab = 'versions')}
						>{$t('pluginDetailModal.tabVersions')}</button
					>
				</div>

				<div class="mt-2 min-h-0 flex-1 overflow-y-auto">
					{#if activeTab === 'description'}
						{#if project.categories.length > 0}
							<div class="flex flex-wrap gap-1.5">
								{#each project.categories as category (category)}
									<span class="bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-[10px]"
										>{category}</span
									>
								{/each}
							</div>
						{/if}
						<p class="text-muted-foreground mt-2 text-xs">{project.description}</p>
						<MarkdownContent markdown={project.body} class="text-xs" />
					{:else if activeTab === 'changelog'}
						{#if changelogVersions.length === 0}
							<p class="text-muted-foreground text-xs">{$t('pluginDetailModal.noChangelog')}</p>
						{:else}
							{#each changelogVersions as version, i (version.id)}
								<div
									class="relative pl-4 {i < changelogVersions.length - 1
										? 'border-border border-l pb-3.5'
										: ''}"
								>
									<span
										class="absolute top-0.5 -left-[4.5px] h-2 w-2 rounded-full {channelDotClass(
											version.version_type
										)}"
									></span>
									<div class="flex items-baseline justify-between gap-2">
										<span class="text-xs font-medium">{version.version_number}</span>
										<span class="text-muted-foreground shrink-0 text-[10px]"
											>{version.date_published.slice(0, 10)}</span
										>
									</div>
									<span class="text-[10px] {channelTextClass(version.version_type)}"
										>{channelLabel(version.version_type)}</span
									>
									<MarkdownContent markdown={version.changelog} class="text-xs" />
								</div>
							{/each}
						{/if}
					{:else}
						<div class="space-y-3">
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
					{/if}
				</div>
			{/if}
		</div>
	</div>
{/if}

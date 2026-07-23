<script lang="ts">
	import type { Instance, Plugin } from '$lib/api';
	import { t } from '$lib/i18n';

	let {
		inst,
		pluginTabLabel,
		searchCapableLoaders,
		uploadingPlugin,
		onPluginFileChange,
		pluginsError,
		plugins,
		busyPluginId,
		onOpenPluginSearchModal,
		onTogglePlugin,
		onDeletePlugin
	}: {
		inst: Instance;
		pluginTabLabel: (loader: string | undefined) => string;
		searchCapableLoaders: string[];
		uploadingPlugin: boolean;
		onPluginFileChange: (e: Event) => void;
		pluginsError: string;
		plugins: Plugin[];
		busyPluginId: string | null;
		onOpenPluginSearchModal: () => void;
		onTogglePlugin: (p: Plugin) => void;
		onDeletePlugin: (p: Plugin) => void;
	} = $props();

	// 검색으로 직접 설치한(또는 업로드한) 모드를 부모로, 그때 같이 딸려온
	// 종속성은 그 아래 들여쓰기로 묶어서 보여준다 -- 부모가 나중에
	// 삭제되면(0012_plugin_parent.sql, ON DELETE SET NULL) 종속성 자체는
	// 안 지워지고 parent_plugin_id만 비므로, 그런 것들은 "기타 종속성"으로
	// 따로 모은다.
	let topLevelPlugins = $derived(plugins.filter((p) => !p.parent_plugin_id));
	let orphanDependencies = $derived(
		plugins.filter((p) => p.parent_plugin_id && !plugins.some((x) => x.id === p.parent_plugin_id))
	);
	function childrenOf(parentId: string) {
		return plugins.filter((p) => p.parent_plugin_id === parentId);
	}
</script>

{#snippet pluginRow(p: Plugin, dependencyOfLabel?: string)}
	<div class="border-border flex items-center justify-between rounded-md border px-2 py-1.5 text-xs">
		<span>
			{p.filename}
			{#if !p.enabled}<span class="text-muted-foreground">{$t('pluginsTab.disabledTag')}</span>{/if}
			{#if dependencyOfLabel}<span class="text-muted-foreground">{dependencyOfLabel}</span>{/if}
		</span>
		<div class="flex shrink-0 gap-1.5">
			<button
				class="border-border rounded-md border px-2 py-1 text-xs"
				disabled={busyPluginId === p.id}
				onclick={() => onTogglePlugin(p)}
			>
				{p.enabled ? $t('pluginsTab.disable') : $t('pluginsTab.enable')}
			</button>
			<button
				class="border-border text-destructive rounded-md border px-2 py-1 text-xs"
				disabled={busyPluginId === p.id}
				onclick={() => onDeletePlugin(p)}>{$t('pluginsTab.delete')}</button
			>
		</div>
	</div>
{/snippet}

<div class="border-border bg-card rounded-lg border p-4">
	<div class="flex items-center justify-between">
		<h2 class="font-medium">{pluginTabLabel(inst.loader)}</h2>
		{#if searchCapableLoaders.includes(inst.loader)}
			<button
				class="border-border rounded-md border px-3 py-1.5 text-xs"
				onclick={onOpenPluginSearchModal}>{$t('pluginsTab.searchOnModrinth')}</button
			>
		{/if}
	</div>
	<p class="text-muted-foreground mt-1 text-xs">{$t('pluginsTab.restartNotice')}</p>

	<div class="mt-4">
		<span class="text-muted-foreground mb-1 block text-xs">{$t('pluginsTab.uploadLabel')}</span>
		<input
			type="file"
			accept=".jar"
			disabled={uploadingPlugin}
			onchange={onPluginFileChange}
			class="text-muted-foreground file:border-border file:bg-background file:text-foreground file:mr-2 file:rounded-md file:border file:px-3 file:py-1.5 file:text-xs file:font-medium file:cursor-pointer text-xs"
		/>
		{#if uploadingPlugin}
			<span class="text-muted-foreground ml-2 text-xs">{$t('pluginsTab.uploading')}</span>
		{/if}
	</div>

	{#if pluginsError}
		<p class="text-destructive mt-2 text-xs">{pluginsError}</p>
	{/if}
	<div class="mt-3">
		<span class="text-muted-foreground mb-1 block text-xs"
			>{$t('pluginsTab.installedLabel', { label: pluginTabLabel(inst.loader) })}</span
		>
		{#if plugins.length === 0}
			<p class="text-muted-foreground text-xs">
				{$t('pluginsTab.installedEmpty', { label: pluginTabLabel(inst.loader) })}
			</p>
		{:else}
			<div class="space-y-2.5">
				{#each topLevelPlugins as p (p.id)}
					<div>
						{@render pluginRow(p)}
						{#if childrenOf(p.id).length > 0}
							<div class="border-border mt-1.5 ml-4 space-y-1.5 border-l pl-2.5">
								{#each childrenOf(p.id) as dep (dep.id)}
									{@render pluginRow(dep, $t('pluginsTab.dependencyOf', { parent: p.filename }))}
								{/each}
							</div>
						{/if}
					</div>
				{/each}
				{#if orphanDependencies.length > 0}
					<div>
						<span class="text-muted-foreground mb-1.5 block text-xs"
							>{$t('pluginsTab.otherDependenciesLabel')}</span
						>
						<div class="space-y-1.5">
							{#each orphanDependencies as dep (dep.id)}
								{@render pluginRow(dep)}
							{/each}
						</div>
					</div>
				{/if}
			</div>
		{/if}
	</div>
</div>

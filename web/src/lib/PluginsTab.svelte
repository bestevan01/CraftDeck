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
	// 종속성은 그 아래 들여쓰기로 묶어서 보여준다. installed_as_dependency
	// 기준으로 최상단/종속성을 나누고(parent_plugin_id 기준이 아님) --
	// parent_plugin_id가 이 마이그레이션 이전에 설치된 레거시 종속성에는
	// 없기 때문에, 그것까지 기타 종속성으로 잡아내려면 이 기준이어야 한다.
	// 부모가 나중에 삭제된 경우(0012_plugin_parent.sql, ON DELETE SET
	// NULL)도 마찬가지로 parent_plugin_id만 비고 종속성 자체는 남는다.
	let topLevelPlugins = $derived(plugins.filter((p) => !p.installed_as_dependency));
	let orphanDependencies = $derived(
		plugins.filter(
			(p) =>
				p.installed_as_dependency &&
				(!p.parent_plugin_id || !plugins.some((x) => x.id === p.parent_plugin_id))
		)
	);
	function childrenOf(parentId: string) {
		return plugins.filter((p) => p.parent_plugin_id === parentId);
	}
</script>

{#snippet pluginRow(p: Plugin, dependencyOfLabel?: string)}
	<div class="border-border flex items-center justify-between rounded-md border px-2 py-1.5 text-xs">
		<span title={p.filename}>
			{p.title || p.filename}
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
	<h2 class="font-medium">{pluginTabLabel(inst.loader)}</h2>

	<div class="mt-3 flex flex-col gap-6 lg:flex-row">
		<div class="lg:w-1/2">
			<p class="text-muted-foreground text-xs">{$t('pluginsTab.restartNotice')}</p>

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

			{#if searchCapableLoaders.includes(inst.loader)}
				<button
					class="border-border mt-4 rounded-md border px-3 py-1.5 text-xs"
					onclick={onOpenPluginSearchModal}>{$t('pluginsTab.searchOnModrinth')}</button
				>
			{/if}

			{#if pluginsError}
				<p class="text-destructive mt-2 text-xs">{pluginsError}</p>
			{/if}
		</div>

		<div class="border-border lg:w-1/2 lg:border-l lg:pl-6">
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
										{@render pluginRow(dep, $t('pluginsTab.dependencyOf', { parent: p.title || p.filename }))}
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
</div>

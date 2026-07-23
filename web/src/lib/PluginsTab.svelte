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

	// 검색으로 직접 설치한(또는 업로드한) 모드만 상단에 평평하게 나열하고,
	// 자동으로 딸려온 종속성은 "공유 종속성" 섹션에 한 번씩만 모아서
	// 보여준다. 예전처럼 특정 부모 밑에 중첩시키지 않는 이유: Fabric API
	// 같은 종속성은 설치된 모드 여러 개가 동시에 필요로 하는 경우가
	// 흔한데, parent_plugin_id는 그중 딱 하나(제일 먼저 설치를 유발한
	// 모드)만 기록할 수 있어서 나머지 모드들은 이 종속성이 자기 때문에
	// 깔려 있다는 사실 자체를 알 방법이 없었다 (확인된 버그). 이제는
	// plugin_dependencies 조인 테이블에서 나온 dependent_of(모든 부모
	// id)를 우선 쓰고, 이 마이그레이션 이전에 설치되어 dependent_of가
	// 비어 있는 레거시 종속성만 parent_plugin_id로 대체한다.
	let topLevelPlugins = $derived(plugins.filter((p) => !p.installed_as_dependency));
	let dependencyPlugins = $derived(plugins.filter((p) => p.installed_as_dependency));

	function parentNames(p: Plugin): string[] {
		const parentIDs =
			p.dependent_of && p.dependent_of.length > 0
				? p.dependent_of
				: p.parent_plugin_id
					? [p.parent_plugin_id]
					: [];
		return parentIDs
			.map((parentID) => plugins.find((x) => x.id === parentID))
			.filter((x): x is Plugin => !!x)
			.map((x) => x.title || x.filename);
	}
</script>

{#snippet pluginRow(p: Plugin)}
	<div class="border-border flex items-center justify-between rounded-md border px-2 py-1.5 text-xs">
		<span title={p.filename}>
			{p.title || p.filename}
			{#if !p.enabled}<span class="text-muted-foreground">{$t('pluginsTab.disabledTag')}</span>{/if}
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

<div class="flex flex-col gap-4 lg:h-full lg:flex-row lg:items-start">
	<div class="border-border bg-card max-h-[26rem] overflow-y-auto rounded-lg border p-4 lg:w-1/2">
		<h2 class="font-medium">{pluginTabLabel(inst.loader)}</h2>
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

	<div
		class="border-border bg-card flex min-h-0 flex-col overflow-hidden rounded-lg border p-4 lg:h-full lg:w-1/2 lg:self-stretch"
	>
		<span class="text-muted-foreground mb-1 block shrink-0 text-xs"
			>{$t('pluginsTab.installedLabel', { label: pluginTabLabel(inst.loader) })}</span
		>
		{#if plugins.length === 0}
			<p class="text-muted-foreground text-xs">
				{$t('pluginsTab.installedEmpty', { label: pluginTabLabel(inst.loader) })}
			</p>
		{:else}
			<div class="min-h-0 flex-1 space-y-2.5 overflow-y-auto">
				{#each topLevelPlugins as p (p.id)}
					{@render pluginRow(p)}
				{/each}
				{#if dependencyPlugins.length > 0}
					<div class="border-border mt-1 border-t pt-2.5">
						<span class="text-muted-foreground mb-1.5 block text-xs"
							>{$t('pluginsTab.sharedDependenciesLabel')}</span
						>
						<div class="space-y-1.5">
							{#each dependencyPlugins as dep (dep.id)}
								<div>
									{@render pluginRow(dep)}
									{#if parentNames(dep).length > 0}
										<div class="mt-1 flex flex-wrap gap-1">
											{#each parentNames(dep) as name (name)}
												<span class="bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-[10px]"
													>{name}</span
												>
											{/each}
										</div>
									{/if}
								</div>
							{/each}
						</div>
					</div>
				{/if}
			</div>
		{/if}
	</div>
</div>

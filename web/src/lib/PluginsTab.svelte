<script lang="ts">
	import type { Instance, Plugin } from '$lib/api';

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
</script>

<div class="border-border bg-card rounded-lg border p-4">
	<div class="flex items-center justify-between">
		<h2 class="font-medium">{pluginTabLabel(inst.loader)}</h2>
		{#if searchCapableLoaders.includes(inst.loader)}
			<button
				class="border-border rounded-md border px-3 py-1.5 text-xs"
				onclick={onOpenPluginSearchModal}>Modrinth에서 검색</button
			>
		{/if}
	</div>
	<p class="text-muted-foreground mt-1 text-xs">설치/삭제/활성화 변경 후에는 서버를 재시작해야 반영됩니다.</p>

	<div class="mt-4">
		<span class="text-muted-foreground mb-1 block text-xs">직접 업로드 (.jar)</span>
		<input
			type="file"
			accept=".jar"
			disabled={uploadingPlugin}
			onchange={onPluginFileChange}
			class="text-muted-foreground file:border-border file:bg-background file:text-foreground file:mr-2 file:rounded-md file:border file:px-3 file:py-1.5 file:text-xs file:font-medium file:cursor-pointer text-xs"
		/>
		{#if uploadingPlugin}
			<span class="text-muted-foreground ml-2 text-xs">업로드 중...</span>
		{/if}
	</div>

	{#if pluginsError}
		<p class="text-destructive mt-2 text-xs">{pluginsError}</p>
	{/if}
	<div class="mt-3">
		<span class="text-muted-foreground mb-1 block text-xs">설치된 {pluginTabLabel(inst.loader)}</span>
		{#if plugins.length === 0}
			<p class="text-muted-foreground text-xs">설치된 {pluginTabLabel(inst.loader)} 목록이 비어 있습니다.</p>
		{:else}
			<div class="space-y-1.5">
				{#each plugins as p (p.id)}
					<div class="border-border flex items-center justify-between rounded-md border px-2 py-1.5 text-xs">
						<span>
							{p.filename}
							{#if !p.enabled}<span class="text-muted-foreground">(비활성화됨)</span>{/if}
							{#if p.installed_as_dependency}<span class="text-muted-foreground"
									>(의존성으로 자동 설치됨)</span
								>{/if}
						</span>
						<div class="flex shrink-0 gap-1.5">
							<button
								class="border-border rounded-md border px-2 py-1 text-xs"
								disabled={busyPluginId === p.id}
								onclick={() => onTogglePlugin(p)}
							>
								{p.enabled ? '비활성화' : '활성화'}
							</button>
							<button
								class="border-border text-destructive rounded-md border px-2 py-1 text-xs"
								disabled={busyPluginId === p.id}
								onclick={() => onDeletePlugin(p)}>삭제</button
							>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>

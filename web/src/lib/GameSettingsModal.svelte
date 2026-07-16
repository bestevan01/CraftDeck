<script lang="ts">
	import type { ServerSetting } from '$lib/api';

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

	const enumOptionLabels: Record<string, Record<string, string>> = {
		difficulty: { peaceful: '평화로움', easy: '쉬움', normal: '보통', hard: '어려움' },
		gamemode: { survival: '서바이벌', creative: '크리에이티브', adventure: '어드벤처', spectator: '관전자' }
	};
	function enumOptionLabel(settingKey: string, value: string) {
		return enumOptionLabels[settingKey]?.[value] ?? value;
	}
</script>

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-8"
		onclick={onClose}
		onkeydown={(e) => {
			if (e.key === 'Escape') onClose();
		}}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="bg-card border-border flex max-h-[80vh] w-full max-w-2xl flex-col rounded-lg border p-4 shadow-lg"
			onclick={(e) => e.stopPropagation()}
		>
			<div class="mb-1 flex shrink-0 items-center justify-between">
				<h2 class="font-medium">게임플레이 설정</h2>
				<button type="button" class="text-muted-foreground text-sm" onclick={onClose}>&times;</button>
			</div>
			<p class="text-muted-foreground mb-3 shrink-0 text-xs">
				변경 사항은 서버를 재시작해야 적용됩니다. 여기 없는 세부 설정은 파일 탭에서
				<code>server.properties</code>를 직접 편집하세요.
			</p>
			{#if loading}
				<p class="text-muted-foreground text-xs">불러오는 중...</p>
			{:else if error && settings.length === 0}
				<p class="text-destructive text-xs">설정을 불러오지 못했습니다: {error}</p>
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
											<option value="true">켜짐</option>
											<option value="false">꺼짐</option>
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
					<p class="mt-2 shrink-0 text-xs text-green-500">저장됐습니다. 다시 시작하면 적용됩니다.</p>
				{/if}
				<button
					class="border-border mt-3 shrink-0 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
					disabled={saving}
					onclick={onSave}
				>
					{saving ? '저장 중...' : '저장'}
				</button>
			{/if}
		</div>
	</div>
{/if}

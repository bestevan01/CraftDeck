<script lang="ts">
	import MemorySlider from '$lib/MemorySlider.svelte';
	import { t } from '$lib/i18n';

	type MemoryConflictItem = {
		id: string;
		name: string;
		memoryGB: number;
		isTarget: boolean;
		isRunning: boolean;
	};

	let {
		open = $bindable(false),
		items = $bindable([]),
		maxGB,
		totalGB,
		overBudget,
		ramBoundaryGB,
		error,
		applying,
		onApply
	}: {
		open: boolean;
		items: MemoryConflictItem[];
		maxGB: number;
		totalGB: number;
		overBudget: boolean;
		ramBoundaryGB: number;
		error: string;
		applying: boolean;
		onApply: () => void;
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
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="bg-card border-border w-full max-w-md rounded-lg border p-4 shadow-lg">
			<h2 class="font-medium">{$t('memoryConflictModal.title')}</h2>
			<p class="text-muted-foreground mt-1 text-xs">
				{$t('memoryConflictModal.description', {
					boundary:
						ramBoundaryGB < maxGB
							? $t('memoryConflictModal.descriptionSwap')
							: $t('memoryConflictModal.descriptionFull')
				})}
			</p>

			<div class="mt-3 space-y-3">
				{#each items as item (item.id)}
					<div>
						<label class="mb-1 flex items-center justify-between text-xs" for="conflict-{item.id}">
							<span>
								{item.name}
								{#if item.isTarget}<span class="text-muted-foreground"
										>{$t('memoryConflictModal.targetTag')}</span
									>
								{:else if item.isRunning}<span class="text-muted-foreground"
										>{$t('memoryConflictModal.runningTag')}</span
									>{/if}
							</span>
							<span>{item.memoryGB}GB</span>
						</label>
						<MemorySlider id="conflict-{item.id}" bind:value={item.memoryGB} {maxGB} {ramBoundaryGB} />
					</div>
				{/each}
			</div>

			<p class="mt-3 text-sm font-medium {overBudget ? 'text-destructive' : ''}">
				{$t('memoryConflictModal.total', { totalGB, maxGB })}
			</p>
			{#if error}
				<p class="text-destructive mt-2 text-xs">{error}</p>
			{/if}

			<div class="mt-3 flex gap-2">
				<button
					class="bg-primary text-primary-foreground rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50"
					disabled={overBudget || applying}
					onclick={onApply}
				>
					{applying ? $t('memoryConflictModal.applying') : $t('memoryConflictModal.applyAndStart')}
				</button>
				<button class="border-border rounded-md border px-3 py-1.5 text-sm" onclick={() => (open = false)}
					>{$t('memoryConflictModal.cancel')}</button
				>
			</div>
		</div>
	</div>
{/if}

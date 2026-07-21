<script lang="ts">
	// A plain <input type="range"> can't grow a marker inside its own track,
	// so this overlays a thin line at the physical-RAM boundary when the
	// slider's max extends past it (CraftDeck's own swap file, FR-46, lets
	// memory_max_mb go above physical RAM once swap is turned on -- see
	// availableMemoryMB in both +page.svelte and instances/[id]/+page.svelte).
	// Without this marker, dragging past physical RAM into swap looked
	// identical to staying within it, even though swap is much slower.
	import { t } from '$lib/i18n';

	let {
		id,
		value = $bindable(),
		maxGB,
		ramBoundaryGB,
		disabled = false
	}: {
		id: string;
		value: number;
		maxGB: number;
		ramBoundaryGB: number;
		disabled?: boolean;
	} = $props();

	let showBoundary = $derived(ramBoundaryGB > 0 && ramBoundaryGB < maxGB);
	let boundaryPercent = $derived(maxGB > 0 ? Math.min(100, (ramBoundaryGB / maxGB) * 100) : 100);
</script>

<div class="relative">
	<input {id} type="range" min="1" max={maxGB} step="1" bind:value {disabled} class="w-full" />
	{#if showBoundary}
		<div
			class="pointer-events-none absolute top-1/2 h-3 w-0.5 -translate-y-1/2 bg-yellow-500"
			style="left: {boundaryPercent}%"
			title={$t('memorySlider.boundaryTitle')}
		></div>
	{/if}
</div>

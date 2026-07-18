<script lang="ts">
	import MemorySlider from '$lib/MemorySlider.svelte';

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
			<h2 class="font-medium">메모리 할당 조정 필요</h2>
			<p class="text-muted-foreground mt-1 text-xs">
				실행하려는 서버들의 메모리 할당 합이 {ramBoundaryGB < maxGB
					? '물리 RAM + 스왑 여유분'
					: '라즈베리파이의 전체 메모리'}을(를) 초과합니다. 아래에서 조정한 뒤 시작할 수 있습니다.
			</p>

			<div class="mt-3 space-y-3">
				{#each items as item (item.id)}
					<div>
						<label class="mb-1 flex items-center justify-between text-xs" for="conflict-{item.id}">
							<span>
								{item.name}
								{#if item.isTarget}<span class="text-muted-foreground">(시작 예정)</span>
								{:else if item.isRunning}<span class="text-muted-foreground"
										>(실행 중, 변경 시 자동으로 재시작됩니다)</span
									>{/if}
							</span>
							<span>{item.memoryGB}GB</span>
						</label>
						<MemorySlider id="conflict-{item.id}" bind:value={item.memoryGB} {maxGB} {ramBoundaryGB} />
					</div>
				{/each}
			</div>

			<p class="mt-3 text-sm font-medium {overBudget ? 'text-destructive' : ''}">
				합계 {totalGB}GB / 전체 {maxGB}GB
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
					{applying ? '적용 중...' : '적용 후 시작'}
				</button>
				<button class="border-border rounded-md border px-3 py-1.5 text-sm" onclick={() => (open = false)}
					>취소</button
				>
			</div>
		</div>
	</div>
{/if}

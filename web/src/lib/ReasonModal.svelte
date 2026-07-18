<script lang="ts">
	// 강제 퇴장/밴 사유 선택 모달 -- kick과 ban 둘 다 같은 사유 프리셋을
	// 공유해서 하나의 모달로 처리한다 (reasonModalKind가 어느 쪽인지 결정).
	let {
		reasonModalKind,
		playerName,
		customReason = $bindable(''),
		onApply,
		onClose
	}: {
		reasonModalKind: 'kick' | 'ban' | null;
		playerName: string;
		customReason: string;
		onApply: (reason: string) => void;
		onClose: () => void;
	} = $props();

	const reasonPresets = ['비매너/욕설', '핵/치트 사용', '광고/스팸 행위', '규칙 위반', '사유 없음'];

	let pressedBackdrop = false;
</script>

{#if reasonModalKind}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
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
		<div class="bg-card border-border w-full max-w-sm rounded-lg border p-4 shadow-lg">
			<h2 class="mb-3 text-sm font-semibold">
				{reasonModalKind === 'kick' ? '강제 퇴장' : '밴'} 사유 선택 ({playerName})
			</h2>
			<div class="mb-3 flex flex-col gap-1.5">
				{#each reasonPresets as preset}
					<button
						type="button"
						class="border-border rounded-md border px-2 py-1.5 text-left text-xs"
						onclick={() => onApply(preset === '사유 없음' ? '' : preset)}
					>
						{preset}
					</button>
				{/each}
			</div>
			<div class="flex gap-2">
				<input
					bind:value={customReason}
					placeholder="직접 입력"
					class="border-input bg-background w-full min-w-0 flex-1 rounded-md border px-2 py-1.5 text-sm"
					onkeydown={(e) => {
						if (e.key === 'Enter') onApply(customReason);
					}}
				/>
				<button
					type="button"
					class="bg-primary text-primary-foreground shrink-0 rounded-md px-3 py-1.5 text-sm"
					onclick={() => onApply(customReason)}>적용</button
				>
			</div>
			<button
				type="button"
				class="text-muted-foreground mt-3 w-full text-center text-xs underline"
				onclick={onClose}>취소</button
			>
		</div>
	</div>
{/if}

<script lang="ts">
	// FR-34: 외부 접속을 켜기 전 경고 + 강력한 비밀번호 유도. 여기서 취소하면
	// 체크박스는 호출부에서 networkSettings.wan_enabled(아직 false)를 그대로
	// 반영하므로 자동으로 꺼진 상태로 되돌아간다.
	let {
		open = $bindable(false),
		onGoToAccountModal,
		onConfirm
	}: {
		open: boolean;
		onGoToAccountModal: () => void;
		onConfirm: () => void;
	} = $props();
</script>

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
		onclick={() => (open = false)}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="border-border bg-card w-full max-w-sm rounded-lg border p-4 shadow-lg"
			onclick={(e) => e.stopPropagation()}
		>
			<h2 class="font-medium text-destructive">⚠ 외부 접속을 켜려고 합니다</h2>
			<p class="text-muted-foreground mt-2 text-sm">
				관리 웹 UI와 게임 포트가 인터넷 전체에 노출됩니다. 누구나 이 주소로 로그인을 시도할 수
				있으니, 계정 비밀번호가 충분히 강력한지(다른 곳에서 재사용하지 않는 긴 무작위 비밀번호) 먼저
				확인하세요.
			</p>
			<button
				type="button"
				class="border-border mt-3 w-full rounded-md border px-4 py-2 text-sm font-medium"
				onclick={() => {
					open = false;
					onGoToAccountModal();
				}}
			>
				비밀번호 변경하러 가기
			</button>
			<div class="mt-3 flex gap-2">
				<button
					type="button"
					class="border-border flex-1 rounded-md border px-4 py-2 text-sm font-medium"
					onclick={() => (open = false)}
				>
					취소
				</button>
				<button
					type="button"
					class="bg-primary text-primary-foreground flex-1 rounded-md px-4 py-2 text-sm font-medium"
					onclick={onConfirm}
				>
					이해했습니다, 계속
				</button>
			</div>
		</div>
	</div>
{/if}

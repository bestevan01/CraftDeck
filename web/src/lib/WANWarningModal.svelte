<script lang="ts">
	// FR-34: 외부 접속을 켜기 전 경고 + 강력한 비밀번호 유도. 여기서 취소하면
	// 체크박스는 호출부에서 networkSettings.wan_enabled(아직 false)를 그대로
	// 반영하므로 자동으로 꺼진 상태로 되돌아간다.
	import { t } from '$lib/i18n';

	let {
		open = $bindable(false),
		onGoToAccountModal,
		onConfirm
	}: {
		open: boolean;
		onGoToAccountModal: () => void;
		onConfirm: () => void;
	} = $props();

	let pressedBackdrop = false;
</script>

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
		onmousedown={(e) => (pressedBackdrop = e.target === e.currentTarget)}
		onclick={(e) => {
			if (pressedBackdrop && e.target === e.currentTarget) open = false;
		}}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="border-border bg-card w-full max-w-sm rounded-lg border p-4 shadow-lg">
			<h2 class="font-medium text-destructive">{$t('wanWarningModal.title')}</h2>
			<p class="text-muted-foreground mt-2 text-sm">
				{$t('wanWarningModal.body')}
			</p>
			<button
				type="button"
				class="border-border mt-3 w-full rounded-md border px-4 py-2 text-sm font-medium"
				onclick={() => {
					open = false;
					onGoToAccountModal();
				}}
			>
				{$t('wanWarningModal.goToAccountModal')}
			</button>
			<div class="mt-3 flex gap-2">
				<button
					type="button"
					class="border-border flex-1 rounded-md border px-4 py-2 text-sm font-medium"
					onclick={() => (open = false)}
				>
					{$t('wanWarningModal.cancel')}
				</button>
				<button
					type="button"
					class="bg-primary text-primary-foreground flex-1 rounded-md px-4 py-2 text-sm font-medium"
					onclick={onConfirm}
				>
					{$t('wanWarningModal.confirm')}
				</button>
			</div>
		</div>
	</div>
{/if}

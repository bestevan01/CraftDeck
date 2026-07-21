<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/i18n';

	// craftdeckd 자체의 새 버전 안내 + 실제 업데이트 실행. 이 프로세스가
	// 업데이트 도중 재시작되는 걸 감안해서(postinst의 restart-on-upgrade
	// 참고), 시작 요청만 보내고 이후엔 /api/system/version을 다시 응답할
	// 때까지 폴링하다가 자동으로 새로고침한다.
	let {
		open = $bindable(false),
		currentVersion,
		latestVersion
	}: {
		open: boolean;
		currentVersion: string;
		latestVersion: string;
	} = $props();

	let pressedBackdrop = false;
	let updating = $state(false);
	let updateError = $state('');

	async function startUpdate() {
		updateError = '';
		updating = true;
		try {
			await api.updateCraftdeck(latestVersion);
		} catch (err) {
			updateError = err instanceof Error ? err.message : String(err);
			updating = false;
			return;
		}
		pollUntilUpdated();
	}

	// 업데이트가 서비스 자체를 재시작시키므로, 그 사이 몇 번의 연결 실패는
	// 당연히 예상된 것 -- 조용히 무시하고 계속 재시도한다. current_version이
	// 실제로 바뀐 응답을 받으면(그냥 "다시 응답함"이 아니라) 새로고침한다.
	// 60초가 지나도 안 되면 폴링을 멈추고 수동 확인을 안내한다.
	function pollUntilUpdated() {
		const deadline = Date.now() + 60_000;
		const interval = setInterval(async () => {
			if (Date.now() > deadline) {
				clearInterval(interval);
				updating = false;
				updateError = $t('updateAvailableModal.timeoutError');
				return;
			}
			try {
				const v = await api.systemVersion();
				if (v.current_version !== currentVersion) {
					clearInterval(interval);
					window.location.reload();
				}
			} catch {
				// still restarting -- keep polling
			}
		}, 3000);
	}
</script>

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-8"
		onmousedown={(e) => (pressedBackdrop = e.target === e.currentTarget)}
		onclick={(e) => {
			if (!updating && pressedBackdrop && e.target === e.currentTarget) open = false;
		}}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="border-border bg-card w-full max-w-sm rounded-lg border p-4 shadow-lg">
			<h2 class="font-medium">{$t('updateAvailableModal.title')}</h2>
			<p class="text-muted-foreground mt-2 text-sm">
				{$t('updateAvailableModal.versionInfo', { current: currentVersion, latest: latestVersion })}
			</p>
			{#if updating}
				<p class="text-muted-foreground mt-2 text-sm">
					{$t('updateAvailableModal.updatingDescription')}
				</p>
				<!-- 실제 설치 진행률(%)은 apt가 제공하지 않아 알 수 없다 -- 그냥
					멈춘 게 아니라 계속 진행 중이라는 걸 보여주는 막연한 로딩
					스피너. -->
				<div class="mt-3 flex items-center gap-2">
					<div
						class="border-muted-foreground/30 border-t-primary h-4 w-4 shrink-0 animate-spin rounded-full border-2"
					></div>
					<span class="text-muted-foreground text-xs">{$t('updateAvailableModal.installing')}</span>
				</div>
			{:else}
				<p class="text-muted-foreground mt-2 text-sm">
					{$t('updateAvailableModal.description')}
				</p>
			{/if}
			{#if updateError}
				<p class="text-destructive mt-2 text-sm">{updateError}</p>
			{/if}
			<div class="mt-3 flex gap-2">
				{#if !updating}
					<button
						type="button"
						class="border-border flex-1 rounded-md border px-4 py-2 text-sm font-medium"
						onclick={() => (open = false)}
					>
						{$t('updateAvailableModal.later')}
					</button>
				{/if}
				<button
					type="button"
					class="bg-primary text-primary-foreground flex-1 rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
					disabled={updating}
					onclick={startUpdate}
				>
					{updating ? $t('updateAvailableModal.updatingButton') : $t('updateAvailableModal.updateNow')}
				</button>
			</div>
		</div>
	</div>
{/if}

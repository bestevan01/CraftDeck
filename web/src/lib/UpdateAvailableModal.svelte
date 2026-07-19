<script lang="ts">
	import { api } from '$lib/api';

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
				updateError = '60초가 지나도 응답이 없습니다. 잠시 후 페이지를 직접 새로고침해보세요.';
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
			<h2 class="font-medium">CraftDeck 새 버전이 있습니다</h2>
			<p class="text-muted-foreground mt-2 text-sm">
				현재 버전 {currentVersion} → 최신 버전 {latestVersion}
			</p>
			{#if updating}
				<p class="text-muted-foreground mt-2 text-sm">
					업데이트 중입니다. 잠깐 재시작되지만, 이미 실행 중인 마인크래프트 서버들은 영향받지
					않습니다. 완료되면 자동으로 새로고침됩니다.
				</p>
			{:else}
				<p class="text-muted-foreground mt-2 text-sm">
					업데이트 중 잠깐 재시작되지만, 이미 실행 중인 마인크래프트 서버들은 영향받지 않습니다.
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
						나중에
					</button>
				{/if}
				<button
					type="button"
					class="bg-primary text-primary-foreground flex-1 rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
					disabled={updating}
					onclick={startUpdate}
				>
					{updating ? '업데이트 중...' : '지금 업데이트'}
				</button>
			</div>
		</div>
	</div>
{/if}

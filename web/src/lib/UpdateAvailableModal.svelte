<script lang="ts">
	// craftdeckd 자체의 새 버전 안내 -- Velocity 프록시의 "업데이트 가능"
	// 배너와 달리 이건 앱 안에서 직접 업데이트를 실행할 방법이 없으므로
	// (apt 트랜잭션이라 서비스 자신을 재시작하게 됨), sudo apt upgrade를
	// 안내만 하고 닫으면 그만인 모달로 둔다.
	let {
		open = $bindable(false),
		currentVersion,
		latestVersion
	}: {
		open: boolean;
		currentVersion: string;
		latestVersion: string;
	} = $props();
</script>

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-8"
		onclick={() => (open = false)}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="border-border bg-card w-full max-w-sm rounded-lg border p-4 shadow-lg"
			onclick={(e) => e.stopPropagation()}
		>
			<h2 class="font-medium">CraftDeck 새 버전이 있습니다</h2>
			<p class="text-muted-foreground mt-2 text-sm">
				현재 버전 {currentVersion} → 최신 버전 {latestVersion}
			</p>
			<p class="text-muted-foreground mt-2 text-sm">
				아래 명령으로 업데이트하세요. 업데이트 중 잠깐 재시작되지만, 이미 실행 중인 마인크래프트
				서버들은 영향받지 않습니다.
			</p>
			<code class="border-border bg-background mt-2 block rounded-md border p-2 text-xs">
				sudo apt update &amp;&amp; sudo apt upgrade craftdeck
			</code>
			<button
				type="button"
				class="border-border mt-3 w-full rounded-md border px-4 py-2 text-sm font-medium"
				onclick={() => (open = false)}
			>
				닫기
			</button>
		</div>
	</div>
{/if}

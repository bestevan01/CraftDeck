<script lang="ts">
	// Cloudflare는 실제 화면을 iframe으로 CraftDeck 안에 띄울 수 없어서(대부분의
	// 로그인 화면이 X-Frame-Options로 막혀 있음), 아래 네 단계는 실제 Cloudflare
	// 대시보드를 축약해 재현한 정적 목업이다 -- 스크린샷이 아니라 재현인 이유는
	// Cloudflare가 UI를 바꿀 때마다 스크린샷이 낡아 오히려 헷갈리게 만들기
	// 때문이다. 값 입력(도메인/토큰)은 기존 DomainConnectionCard와 동일한
	// domainForm/onSave를 그대로 공유해서, 이 모달은 같은 설정을 다른 방식으로
	// 채우는 보조 경로일 뿐 별도 저장 로직을 갖지 않는다.
	let {
		open = $bindable(false),
		domainForm = $bindable(),
		domainSaving,
		domainError,
		onSave
	}: {
		open: boolean;
		domainForm: {
			kind: 'main_domain' | 'free_subdomain';
			provider: string;
			hostname: string;
			token: string;
		};
		domainSaving: boolean;
		domainError: string;
		onSave: () => void;
	} = $props();

	let pressedBackdrop = false;

	$effect(() => {
		if (open) {
			domainForm.kind = 'main_domain';
			domainForm.provider = 'cloudflare';
		}
	});

	async function confirm() {
		await onSave();
		if (!domainError) open = false;
	}
</script>

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
		onmousedown={(e) => (pressedBackdrop = e.target === e.currentTarget)}
		onclick={(e) => {
			if (pressedBackdrop && e.target === e.currentTarget) open = false;
		}}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="border-border bg-card flex max-h-[85vh] w-full max-w-xl flex-col rounded-lg border shadow-lg"
		>
			<div class="border-border shrink-0 border-b p-4 pb-3">
				<div class="flex items-center justify-between">
					<h2 class="font-medium">Cloudflare 연동</h2>
					<button
						type="button"
						aria-label="닫기"
						class="text-muted-foreground text-sm"
						onclick={() => (open = false)}
					>
						✕
					</button>
				</div>
				<p class="text-muted-foreground mt-1 text-xs">
					아래 화면을 그대로 따라 하면서 도메인 하나로 범위를 제한한 API 토큰을 발급받으세요.
				</p>
			</div>

			<div class="flex flex-col gap-5 overflow-y-auto p-4">
				<p class="text-muted-foreground border-border rounded-md border border-dashed p-3 text-xs leading-relaxed">
					소유한 도메인이 없다면 먼저 구매하시고, 아직 Cloudflare에 등록하지 않았다면
					<a
						href="https://developers.cloudflare.com/fundamentals/manage-domains/add-site/"
						target="_blank"
						rel="noreferrer"
						class="text-foreground underline">Cloudflare에 사이트 추가</a
					>로 도메인을 먼저 연결하세요 (네임서버를 Cloudflare로 변경). 아래 단계는 그다음부터입니다.
				</p>
				<div class="flex items-start gap-3">
					<div class="w-56 shrink-0 overflow-hidden rounded-md border border-neutral-300 bg-white">
						<div
							class="flex items-center gap-1.5 border-b border-neutral-200 bg-neutral-100 px-2 py-1.5"
						>
							<span class="h-1.5 w-1.5 rounded-full bg-neutral-300"></span>
							<span class="h-1.5 w-1.5 rounded-full bg-neutral-300"></span>
							<span class="flex-1 rounded bg-white px-2 py-0.5 text-[8px] text-neutral-500"
								>dash.cloudflare.com</span
							>
						</div>
						<div class="relative p-2">
							<div class="mb-1.5 flex items-center justify-end gap-1.5">
								<span class="text-[8px] text-neutral-500">지원</span>
								<div class="h-4 w-4 rounded-full bg-neutral-300"></div>
							</div>
							<div
								class="absolute top-7 right-2 w-24 rounded-md border border-neutral-200 bg-white shadow-sm"
							>
								<div
									class="border-l-2 border-blue-500 bg-blue-50 px-2 py-1 text-[8px] font-medium text-neutral-900"
								>
									프로필
								</div>
								<div class="px-2 py-1 text-[8px] text-neutral-600">청구</div>
								<div class="px-2 py-1 text-[8px] text-neutral-600">모양새</div>
								<div class="px-2 py-1 text-[8px] text-neutral-600">언어</div>
								<div class="px-2 py-1 text-[8px] text-neutral-600">표준 시간대</div>
								<div class="border-t border-neutral-100 px-2 py-1 text-[8px] text-orange-600">
									로그아웃
								</div>
							</div>
							<div class="h-24"></div>
						</div>
					</div>
					<div class="pt-0.5">
						<div class="text-sm font-medium">1. 오른쪽 위 프로필 아이콘 클릭</div>
						<div class="text-muted-foreground mt-1 text-xs leading-relaxed">
							Cloudflare 대시보드에 로그인한 뒤, 오른쪽 위 원형 아이콘을 눌러 뜨는 메뉴에서
							<span class="text-foreground font-medium">프로필</span>을 선택하세요.
						</div>
					</div>
				</div>

				<div class="flex items-start gap-3">
					<div class="w-56 shrink-0 overflow-hidden rounded-md border border-neutral-300 bg-white">
						<div
							class="flex items-center gap-1.5 border-b border-neutral-200 bg-neutral-100 px-2 py-1.5"
						>
							<span class="h-1.5 w-1.5 rounded-full bg-neutral-300"></span>
							<span class="h-1.5 w-1.5 rounded-full bg-neutral-300"></span>
							<span class="flex-1 rounded bg-white px-2 py-0.5 text-[8px] text-neutral-500"
								>dash.cloudflare.com/profile</span
							>
						</div>
						<div class="flex">
							<div class="w-16 border-r border-neutral-100 bg-neutral-50 py-2">
								<div class="px-2 py-1 text-[8px] text-neutral-500">설정</div>
								<div class="px-2 py-1 text-[8px] text-neutral-500">액세스 관리</div>
								<div
									class="border-l-2 border-blue-500 bg-blue-50 px-2 py-1 text-[8px] font-medium text-neutral-900"
								>
									API 토큰
								</div>
							</div>
							<div class="flex-1 p-2">
								<div class="mb-1.5 text-[9px] font-medium text-neutral-900">사용자 API 토큰</div>
								<div
									class="inline-block rounded bg-blue-600 px-2 py-1 text-[8px] font-medium text-white"
								>
									+ 토큰 생성
								</div>
							</div>
						</div>
					</div>
					<div class="pt-0.5">
						<div class="text-sm font-medium">2. API 토큰 탭에서 토큰 생성</div>
						<div class="text-muted-foreground mt-1 text-xs leading-relaxed">
							왼쪽 메뉴의 <span class="text-foreground font-medium">API 토큰</span> 탭으로 이동한 뒤,
							오른쪽 위 파란색 <span class="text-foreground font-medium">+ 토큰 생성</span> 버튼을
							누르세요.
						</div>
					</div>
				</div>

				<div class="flex items-start gap-3">
					<div class="w-56 shrink-0 overflow-hidden rounded-md border border-neutral-300 bg-white">
						<div
							class="flex items-center gap-1.5 border-b border-neutral-200 bg-neutral-100 px-2 py-1.5"
						>
							<span class="h-1.5 w-1.5 rounded-full bg-neutral-300"></span>
							<span class="h-1.5 w-1.5 rounded-full bg-neutral-300"></span>
							<span class="flex-1 rounded bg-white px-2 py-0.5 text-[8px] text-neutral-500"
								>.../api-tokens/create</span
							>
						</div>
						<div class="p-2">
							<div class="mb-1.5 text-[9px] font-medium text-neutral-900">API 토큰 템플릿</div>
							<div
								class="mb-1 flex items-center justify-between rounded border-2 border-blue-500 bg-blue-50 p-1.5"
							>
								<div class="text-[8.5px] font-medium text-neutral-900">영역 DNS 편집</div>
								<div class="rounded bg-blue-600 px-1.5 py-0.5 text-[7px] text-white">템플릿 사용</div>
							</div>
							<div
								class="flex items-center justify-between rounded border border-neutral-200 p-1.5 text-[8px] text-neutral-600"
							>
								<span>청구 정보 읽기</span>
								<span class="rounded bg-neutral-200 px-1.5 py-0.5 text-[7px] text-neutral-600"
									>템플릿 사용</span
								>
							</div>
						</div>
					</div>
					<div class="pt-0.5">
						<div class="text-sm font-medium">3. 영역 DNS 편집 템플릿 선택</div>
						<div class="text-muted-foreground mt-1 text-xs leading-relaxed">
							템플릿 목록에서 <span class="text-foreground font-medium">영역 DNS 편집</span>을 찾아
							오른쪽의 <span class="text-foreground font-medium">템플릿 사용</span>을 누르세요.
							CraftDeck이 DNS 레코드만 관리할 수 있도록 딱 필요한 권한만 담긴 템플릿입니다.
						</div>
					</div>
				</div>

				<div class="flex items-start gap-3">
					<div class="w-56 shrink-0 overflow-hidden rounded-md border border-neutral-300 bg-white">
						<div
							class="flex items-center gap-1.5 border-b border-neutral-200 bg-neutral-100 px-2 py-1.5"
						>
							<span class="h-1.5 w-1.5 rounded-full bg-neutral-300"></span>
							<span class="h-1.5 w-1.5 rounded-full bg-neutral-300"></span>
							<span class="flex-1 rounded bg-white px-2 py-0.5 text-[8px] text-neutral-500"
								>.../api-tokens/create</span
							>
						</div>
						<div class="p-2">
							<div class="mb-1.5 text-[9px] font-medium text-neutral-900">영역 리소스</div>
							<div class="mb-1 flex gap-1">
								<div class="flex-1 rounded border border-neutral-200 p-1 text-[7.5px] text-neutral-900">
									포함
								</div>
								<div class="flex-1 rounded border border-neutral-200 p-1 text-[7.5px] text-neutral-900">
									특정 영역
								</div>
							</div>
							<div
								class="relative rounded border-2 border-blue-500 bg-blue-50 p-1 text-[7.5px] text-neutral-900"
							>
								{domainForm.hostname || 'apple-farm.online'}
							</div>
							<div
								class="mt-1.5 inline-block rounded bg-blue-600 px-2 py-1 text-[8px] font-medium text-white"
							>
								토큰 생성
							</div>
						</div>
					</div>
					<div class="flex-1 pt-0.5">
						<div class="text-sm font-medium">4. 영역 리소스를 도메인 하나로 제한 후 발급</div>
						<div class="text-muted-foreground mt-1 mb-2 text-xs leading-relaxed">
							<span class="text-foreground font-medium">포함 / 특정 영역</span>을 선택하고 드롭다운에서
							아래 입력한 도메인을 고른 뒤 <span class="text-foreground font-medium">토큰 생성</span>을
							누르면 토큰 값이 화면에 딱 한 번 표시됩니다. 그 값을 복사해서 아래에 붙여넣으세요.
						</div>
						<a
							href="https://dash.cloudflare.com/profile/api-tokens"
							target="_blank"
							rel="noreferrer"
							class="border-border mb-3 inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs"
						>
							Cloudflare에서 토큰 발급하기 ↗
						</a>
						<label class="text-muted-foreground mb-1 block text-xs" for="cf-tutorial-hostname"
							>도메인</label
						>
						<input
							id="cf-tutorial-hostname"
							type="text"
							placeholder="예: apple-farm.online"
							bind:value={domainForm.hostname}
							class="border-input bg-background mb-2 w-full rounded-md border px-3 py-1.5 text-sm"
						/>
						<label class="text-muted-foreground mb-1 block text-xs" for="cf-tutorial-token"
							>API 토큰</label
						>
						<input
							id="cf-tutorial-token"
							type="password"
							placeholder="발급받은 토큰을 붙여넣으세요"
							bind:value={domainForm.token}
							class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-sm"
						/>
					</div>
				</div>
			</div>

			{#if domainError}
				<p class="text-destructive shrink-0 px-4 text-xs">{domainError}</p>
			{/if}

			<div class="border-border flex shrink-0 justify-end gap-2 border-t p-3">
				<button
					type="button"
					class="border-border rounded-md border px-4 py-2 text-sm font-medium"
					onclick={() => (open = false)}
				>
					나중에
				</button>
				<button
					type="button"
					class="bg-primary text-primary-foreground rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
					disabled={domainSaving || !domainForm.hostname.trim() || !domainForm.token.trim()}
					onclick={confirm}
				>
					{domainSaving ? '연결 중...' : '확인하고 연결'}
				</button>
			</div>
		</div>
	</div>
{/if}

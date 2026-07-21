<script lang="ts">
	import { t } from '$lib/i18n';

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
					<h2 class="font-medium">{$t('cloudflareTutorialModal.header.title')}</h2>
					<button
						type="button"
						aria-label={$t('cloudflareTutorialModal.header.close')}
						class="text-muted-foreground text-sm"
						onclick={() => (open = false)}
					>
						✕
					</button>
				</div>
				<p class="text-muted-foreground mt-1 text-xs">
					{$t('cloudflareTutorialModal.header.description')}
				</p>
			</div>

			<div class="flex flex-col gap-5 overflow-y-auto p-4">
				<p class="text-muted-foreground border-border rounded-md border border-dashed p-3 text-xs leading-relaxed">
					{$t('cloudflareTutorialModal.intro.before')}
					<a
						href="https://developers.cloudflare.com/fundamentals/manage-domains/add-site/"
						target="_blank"
						rel="noreferrer"
						class="text-foreground underline">{$t('cloudflareTutorialModal.intro.linkText')}</a
					>{$t('cloudflareTutorialModal.intro.after')}
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
								<span class="text-[8px] text-neutral-500"
									>{$t('cloudflareTutorialModal.mockup.support')}</span
								>
								<div class="h-4 w-4 rounded-full bg-neutral-300"></div>
							</div>
							<div
								class="absolute top-7 right-2 w-24 rounded-md border border-neutral-200 bg-white shadow-sm"
							>
								<div
									class="border-l-2 border-blue-500 bg-blue-50 px-2 py-1 text-[8px] font-medium text-neutral-900"
								>
									{$t('cloudflareTutorialModal.mockup.profile')}
								</div>
								<div class="px-2 py-1 text-[8px] text-neutral-600">
									{$t('cloudflareTutorialModal.mockup.billing')}
								</div>
								<div class="px-2 py-1 text-[8px] text-neutral-600">
									{$t('cloudflareTutorialModal.mockup.appearance')}
								</div>
								<div class="px-2 py-1 text-[8px] text-neutral-600">
									{$t('cloudflareTutorialModal.mockup.language')}
								</div>
								<div class="px-2 py-1 text-[8px] text-neutral-600">
									{$t('cloudflareTutorialModal.mockup.timezone')}
								</div>
								<div class="border-t border-neutral-100 px-2 py-1 text-[8px] text-orange-600">
									{$t('cloudflareTutorialModal.mockup.logout')}
								</div>
							</div>
							<div class="h-24"></div>
						</div>
					</div>
					<div class="pt-0.5">
						<div class="text-sm font-medium">{$t('cloudflareTutorialModal.step1.title')}</div>
						<div class="text-muted-foreground mt-1 text-xs leading-relaxed">
							{$t('cloudflareTutorialModal.step1.descBefore')}
							<span class="text-foreground font-medium"
								>{$t('cloudflareTutorialModal.step1.descHighlight')}</span
							>{$t('cloudflareTutorialModal.step1.descAfter')}
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
								<div class="px-2 py-1 text-[8px] text-neutral-500">
									{$t('cloudflareTutorialModal.mockup.settings')}
								</div>
								<div class="px-2 py-1 text-[8px] text-neutral-500">
									{$t('cloudflareTutorialModal.mockup.accessManagement')}
								</div>
								<div
									class="border-l-2 border-blue-500 bg-blue-50 px-2 py-1 text-[8px] font-medium text-neutral-900"
								>
									{$t('cloudflareTutorialModal.mockup.apiTokens')}
								</div>
							</div>
							<div class="flex-1 p-2">
								<div class="mb-1.5 text-[9px] font-medium text-neutral-900">
									{$t('cloudflareTutorialModal.mockup.userApiTokens')}
								</div>
								<div
									class="inline-block rounded bg-blue-600 px-2 py-1 text-[8px] font-medium text-white"
								>
									{$t('cloudflareTutorialModal.mockup.createTokenButton')}
								</div>
							</div>
						</div>
					</div>
					<div class="pt-0.5">
						<div class="text-sm font-medium">{$t('cloudflareTutorialModal.step2.title')}</div>
						<div class="text-muted-foreground mt-1 text-xs leading-relaxed">
							{$t('cloudflareTutorialModal.step2.descBefore')}
							<span class="text-foreground font-medium"
								>{$t('cloudflareTutorialModal.step2.descHighlight1')}</span
							>
							{$t('cloudflareTutorialModal.step2.descMid')}
							<span class="text-foreground font-medium"
								>{$t('cloudflareTutorialModal.step2.descHighlight2')}</span
							>
							{$t('cloudflareTutorialModal.step2.descAfter')}
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
							<div class="mb-1.5 text-[9px] font-medium text-neutral-900">
								{$t('cloudflareTutorialModal.mockup.apiTokenTemplate')}
							</div>
							<div
								class="mb-1 flex items-center justify-between rounded border-2 border-blue-500 bg-blue-50 p-1.5"
							>
								<div class="text-[8.5px] font-medium text-neutral-900">
									{$t('cloudflareTutorialModal.mockup.editZoneDns')}
								</div>
								<div class="rounded bg-blue-600 px-1.5 py-0.5 text-[7px] text-white">
									{$t('cloudflareTutorialModal.mockup.useTemplate')}
								</div>
							</div>
							<div
								class="flex items-center justify-between rounded border border-neutral-200 p-1.5 text-[8px] text-neutral-600"
							>
								<span>{$t('cloudflareTutorialModal.mockup.readBilling')}</span>
								<span class="rounded bg-neutral-200 px-1.5 py-0.5 text-[7px] text-neutral-600"
									>{$t('cloudflareTutorialModal.mockup.useTemplate')}</span
								>
							</div>
						</div>
					</div>
					<div class="pt-0.5">
						<div class="text-sm font-medium">{$t('cloudflareTutorialModal.step3.title')}</div>
						<div class="text-muted-foreground mt-1 text-xs leading-relaxed">
							{$t('cloudflareTutorialModal.step3.descBefore')}
							<span class="text-foreground font-medium"
								>{$t('cloudflareTutorialModal.step3.descHighlight1')}</span
							>{$t('cloudflareTutorialModal.step3.descMid')}
							<span class="text-foreground font-medium"
								>{$t('cloudflareTutorialModal.step3.descHighlight2')}</span
							>{$t('cloudflareTutorialModal.step3.descAfter')}
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
							<div class="mb-1.5 text-[9px] font-medium text-neutral-900">
								{$t('cloudflareTutorialModal.mockup.zoneResources')}
							</div>
							<div class="mb-1 flex gap-1">
								<div class="flex-1 rounded border border-neutral-200 p-1 text-[7.5px] text-neutral-900">
									{$t('cloudflareTutorialModal.mockup.included')}
								</div>
								<div class="flex-1 rounded border border-neutral-200 p-1 text-[7.5px] text-neutral-900">
									{$t('cloudflareTutorialModal.mockup.specificZone')}
								</div>
							</div>
							<div
								class="relative rounded border-2 border-blue-500 bg-blue-50 p-1 text-[7.5px] text-neutral-900"
							>
								{domainForm.hostname || 'craftdeck.cc'}
							</div>
							<div
								class="mt-1.5 inline-block rounded bg-blue-600 px-2 py-1 text-[8px] font-medium text-white"
							>
								{$t('cloudflareTutorialModal.mockup.generateToken')}
							</div>
						</div>
					</div>
					<div class="flex-1 pt-0.5">
						<div class="text-sm font-medium">{$t('cloudflareTutorialModal.step4.title')}</div>
						<div class="text-muted-foreground mt-1 mb-2 text-xs leading-relaxed">
							<span class="text-foreground font-medium"
								>{$t('cloudflareTutorialModal.step4.descHighlight1')}</span
							>{$t('cloudflareTutorialModal.step4.descMid')}
							<span class="text-foreground font-medium"
								>{$t('cloudflareTutorialModal.step4.descHighlight2')}</span
							>{$t('cloudflareTutorialModal.step4.descAfter')}
						</div>
						<a
							href="https://dash.cloudflare.com/profile/api-tokens"
							target="_blank"
							rel="noreferrer"
							class="border-border mb-3 inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs"
						>
							{$t('cloudflareTutorialModal.step4.linkText')}
						</a>
						<label class="text-muted-foreground mb-1 block text-xs" for="cf-tutorial-hostname"
							>{$t('cloudflareTutorialModal.step4.hostnameLabel')}</label
						>
						<input
							id="cf-tutorial-hostname"
							type="text"
							placeholder={$t('cloudflareTutorialModal.step4.hostnamePlaceholder')}
							bind:value={domainForm.hostname}
							class="border-input bg-background mb-2 w-full rounded-md border px-3 py-1.5 text-sm"
						/>
						<label class="text-muted-foreground mb-1 block text-xs" for="cf-tutorial-token"
							>{$t('cloudflareTutorialModal.step4.tokenLabel')}</label
						>
						<input
							id="cf-tutorial-token"
							type="password"
							placeholder={$t('cloudflareTutorialModal.step4.tokenPlaceholder')}
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
					{$t('cloudflareTutorialModal.footer.later')}
				</button>
				<button
					type="button"
					class="bg-primary text-primary-foreground rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
					disabled={domainSaving || !domainForm.hostname.trim() || !domainForm.token.trim()}
					onclick={confirm}
				>
					{domainSaving
						? $t('cloudflareTutorialModal.footer.connecting')
						: $t('cloudflareTutorialModal.footer.confirm')}
				</button>
			</div>
		</div>
	</div>
{/if}

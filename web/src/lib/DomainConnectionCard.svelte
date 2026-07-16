<script lang="ts">
	import type { DomainConfig } from '$lib/api';

	// FR-26 minimal skeleton + FR-1f: 도메인 연결 여부가 Velocity 기본 프록시
	// 사용 여부를 결정합니다.
	let {
		domainConfig,
		domainForm = $bindable(),
		domainSaving,
		domainError,
		domainTokenRequired,
		onKindChange,
		onSave,
		onUnregister
	}: {
		domainConfig: DomainConfig | null;
		domainForm: {
			kind: 'main_domain' | 'free_subdomain';
			provider: string;
			hostname: string;
			token: string;
		};
		domainSaving: boolean;
		domainError: string;
		domainTokenRequired: boolean;
		onKindChange: () => void;
		onSave: () => void;
		onUnregister: () => void;
	} = $props();

	// FR-26a: DuckDNS (active renewal) and ipTime (감시 전용, FR-26b/e) are
	// the only free-subdomain providers implemented so far.
	const freeProviders = [
		{ value: 'duckdns', label: 'DuckDNS' },
		{ value: 'iptime', label: 'ipTime (자동 갱신 불가, 감시 전용)' }
	];
</script>

<!-- FR-26 minimal skeleton + FR-1f: 도메인 연결 여부가 Velocity 기본
	프록시 사용 여부를 결정합니다. -->
<div class="border-border bg-card rounded-lg border p-4">
	<h2 class="font-medium">도메인 연결</h2>
	<p class="text-muted-foreground mt-1 text-xs">
		소유한 메인 도메인을 연결하면 Velocity 프록시가 자동으로 켜져서 여러 서버를 서브도메인으로 묶어
		접속할 수 있게 됩니다. 도메인이 없거나 무료 DDNS 서브도메인만 쓰는 경우 서브도메인 라우팅 자체가
		실제로 닿지 않으므로, Velocity는 꺼지고 각 서버가 포트로 직접 노출됩니다.
	</p>
	{#if domainConfig}
		<p class="mt-2 text-xs">
			<strong>{domainConfig.kind === 'main_domain' ? '메인 도메인' : '무료 DDNS'}</strong>
			연결됨 -- {domainConfig.hostname} ({domainConfig.provider})
		</p>
		{#if domainConfig.kind === 'free_subdomain'}
			{#if domainConfig.mode === 'monitor'}
				<p class="text-muted-foreground mt-1 text-xs">
					이 제공자는 자동 갱신을 지원하지 않으며 공유기 자체 DDNS 기능에 의존합니다. CraftDeck은
					주기적으로 이 호스트명이 실제 공인 IP를 가리키는지만 확인합니다.
				</p>
				{#if domainConfig.mismatch_detected}
					<p class="text-destructive mt-1 text-xs">
						⚠ 이 호스트명이 현재 공인 IP와 일치하지 않습니다 -- 공유기의 ipTime DDNS 기능이 꺼졌거나
						실패했을 수 있습니다.
					</p>
				{/if}
			{:else}
				<p class="text-muted-foreground mt-1 text-xs">
					CraftDeck이 20분마다 자동으로 공인 IP를 갱신합니다.
				</p>
			{/if}
			{#if domainConfig.last_checked_at}
				<p class="text-muted-foreground mt-1 text-xs">
					마지막 확인: {new Date(domainConfig.last_checked_at).toLocaleString('ko-KR')}
					{domainConfig.last_known_ip ? `(${domainConfig.last_known_ip})` : ''}
				</p>
			{/if}
		{:else if domainConfig.kind === 'main_domain' && domainConfig.cert_renewal_error}
			<!-- FR-33a: Let's Encrypt 발급/갱신 실패를 만료 전에 미리 안내 -->
			<p class="text-destructive mt-1 text-xs">
				⚠ HTTPS 인증서 발급/갱신에 실패했습니다 ({new Date(
					domainConfig.cert_renewal_error_at ?? ''
				).toLocaleString('ko-KR')}): {domainConfig.cert_renewal_error}
			</p>
			<p class="text-muted-foreground mt-1 text-xs">
				다음 접속 시도에서 자동으로 재시도하며, 그때까지는 자체 서명 인증서로 대체됩니다. Cloudflare
				토큰이 만료/취소되지 않았는지 확인하세요.
			</p>
		{/if}
		<button
			class="border-border text-destructive mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
			disabled={domainSaving}
			onclick={onUnregister}
		>
			{domainSaving ? '해제 중...' : '연결 해제'}
		</button>
	{:else}
		<div class="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-2">
			<div>
				<label class="text-muted-foreground mb-1 block text-xs" for="domain-kind">연결 방식</label>
				<div class="relative">
					<select
						id="domain-kind"
						bind:value={domainForm.kind}
						onchange={onKindChange}
						class="border-input bg-background w-full appearance-none rounded-md border py-1.5 pl-3 pr-8 text-sm"
					>
						<option value="main_domain">소유한 메인 도메인</option>
						<option value="free_subdomain">무료 DDNS 서브도메인</option>
					</select>
					<svg
						class="text-muted-foreground pointer-events-none absolute top-1/2 right-3 h-4 w-4 -translate-y-1/2"
						viewBox="0 0 20 20"
						fill="none"
						stroke="currentColor"
						stroke-width="1.5"
						><path d="M5 7l5 5 5-5" stroke-linecap="round" stroke-linejoin="round" /></svg
					>
				</div>
			</div>
			{#if domainForm.kind === 'free_subdomain'}
				<div>
					<label class="text-muted-foreground mb-1 block text-xs" for="domain-provider">제공자</label>
					<div class="relative">
						<select
							id="domain-provider"
							bind:value={domainForm.provider}
							class="border-input bg-background w-full appearance-none rounded-md border py-1.5 pl-3 pr-8 text-sm"
						>
							{#each freeProviders as p (p.value)}
								<option value={p.value}>{p.label}</option>
							{/each}
						</select>
						<svg
							class="text-muted-foreground pointer-events-none absolute top-1/2 right-3 h-4 w-4 -translate-y-1/2"
							viewBox="0 0 20 20"
							fill="none"
							stroke="currentColor"
							stroke-width="1.5"
							><path d="M5 7l5 5 5-5" stroke-linecap="round" stroke-linejoin="round" /></svg
						>
					</div>
				</div>
				<div class="sm:col-span-2">
					<label class="text-muted-foreground mb-1 block text-xs" for="domain-hostname">호스트명</label>
					<input
						id="domain-hostname"
						type="text"
						placeholder={domainForm.provider === 'iptime'
							? '예: myrouter.iptime.org'
							: '예: myserver.duckdns.org'}
						bind:value={domainForm.hostname}
						class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-sm"
					/>
				</div>
				{#if domainForm.provider === 'duckdns'}
					<div class="sm:col-span-2">
						<label class="text-muted-foreground mb-1 block text-xs" for="domain-token"
							>DuckDNS 토큰</label
						>
						<input
							id="domain-token"
							type="password"
							bind:value={domainForm.token}
							class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-sm"
						/>
					</div>
				{:else if domainForm.provider === 'iptime'}
					<p class="text-muted-foreground text-xs sm:col-span-2">
						공유기 관리 페이지에서 이미 설정해둔 ipTime DDNS 호스트명을 그대로 입력하세요. 이
						제공자는 CraftDeck이 직접 갱신할 수 없어 공유기 자체 기능에 의존하며, CraftDeck은
						감시만 합니다.
					</p>
				{/if}
			{:else}
				<div>
					<label class="text-muted-foreground mb-1 block text-xs" for="domain-provider">제공자</label>
					<input
						id="domain-provider"
						type="text"
						value="Cloudflare"
						disabled
						class="border-input bg-background text-muted-foreground w-full rounded-md border px-3 py-1.5 text-sm"
					/>
				</div>
				<div class="sm:col-span-2">
					<label class="text-muted-foreground mb-1 block text-xs" for="domain-hostname">도메인</label>
					<input
						id="domain-hostname"
						type="text"
						placeholder="예: apple-farm.online"
						bind:value={domainForm.hostname}
						class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-sm"
					/>
				</div>
				<div class="sm:col-span-2">
					<label class="text-muted-foreground mb-1 block text-xs" for="domain-cf-token"
						>Cloudflare API 토큰</label
					>
					<input
						id="domain-cf-token"
						type="password"
						bind:value={domainForm.token}
						placeholder="Edit zone DNS 권한, 이 도메인 존으로 범위 제한 권장"
						class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-sm"
					/>
					<p class="text-muted-foreground mt-1 text-xs">
						Cloudflare 대시보드 &gt; My Profile &gt; API Tokens에서 "Edit zone DNS" 템플릿으로 이
						도메인 존 하나만 범위를 제한해 발급하세요. 이 토큰으로 해당 존에 실제 접근 가능한지
						확인해 도메인 소유권 검증을 대신합니다.
					</p>
				</div>
			{/if}
		</div>
		<button
			class="bg-primary text-primary-foreground mt-3 rounded-md px-3 py-1.5 text-xs font-medium disabled:opacity-50"
			disabled={domainSaving ||
				!domainForm.provider.trim() ||
				!domainForm.hostname.trim() ||
				(domainTokenRequired && !domainForm.token.trim())}
			onclick={onSave}
		>
			{domainSaving ? '등록 중...' : '등록'}
		</button>
	{/if}
	{#if domainError}
		<p class="text-destructive mt-2 text-xs">{domainError}</p>
	{/if}
</div>

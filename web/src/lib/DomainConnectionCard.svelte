<script lang="ts">
	import type { DomainConfig } from '$lib/api';
	import { t } from '$lib/i18n';

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
		onUnregister,
		onOpenCloudflareGuide
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
		onOpenCloudflareGuide: () => void;
	} = $props();

	// FR-26a: DuckDNS (active renewal) and ipTime (감시 전용, FR-26b/e) are
	// the only free-subdomain providers implemented so far.
	const freeProviders = $derived([
		{ value: 'duckdns', label: 'DuckDNS' },
		{ value: 'iptime', label: $t('domainConnectionCard.iptimeProviderLabel') }
	]);
</script>

<!-- FR-26 minimal skeleton + FR-1f: 도메인 연결 여부가 Velocity 기본
	프록시 사용 여부를 결정합니다. -->
<div id="tour-domain-card" class="border-border bg-card rounded-lg border p-4">
	<h2 class="font-medium">{$t('domainConnectionCard.title')}</h2>
	<p class="text-muted-foreground mt-1 text-xs">
		{$t('domainConnectionCard.description')}
	</p>
	{#if !domainConfig}
		<button
			id="tour-cloudflare-guide"
			type="button"
			class="border-border mt-2 rounded-md border px-3 py-1.5 text-xs"
			onclick={onOpenCloudflareGuide}
		>
			{$t('domainConnectionCard.cloudflareGuideButton')}
		</button>
	{/if}
	{#if domainConfig}
		<p class="mt-2 text-xs">
			<strong
				>{domainConfig.kind === 'main_domain'
					? $t('domainConnectionCard.kindMainDomain')
					: $t('domainConnectionCard.kindFreeSubdomain')}</strong
			>
			{$t('domainConnectionCard.connectedInfo', {
				hostname: domainConfig.hostname,
				provider: domainConfig.provider
			})}
		</p>
		{#if domainConfig.kind === 'free_subdomain'}
			{#if domainConfig.mode === 'monitor'}
				<p class="text-muted-foreground mt-1 text-xs">
					{$t('domainConnectionCard.monitorModeNotice')}
				</p>
				{#if domainConfig.mismatch_detected}
					<p class="text-destructive mt-1 text-xs">
						{$t('domainConnectionCard.mismatchWarning')}
					</p>
				{/if}
			{:else}
				<p class="text-muted-foreground mt-1 text-xs">
					{$t('domainConnectionCard.activeRenewalNotice')}
				</p>
			{/if}
			{#if domainConfig.last_checked_at}
				<p class="text-muted-foreground mt-1 text-xs">
					{$t('domainConnectionCard.lastChecked', {
						time: new Date(domainConfig.last_checked_at).toLocaleString('ko-KR'),
						ip: domainConfig.last_known_ip ? `(${domainConfig.last_known_ip})` : ''
					})}
				</p>
			{/if}
		{:else if domainConfig.kind === 'main_domain' && domainConfig.cert_renewal_error}
			<!-- FR-33a: Let's Encrypt 발급/갱신 실패를 만료 전에 미리 안내 -->
			<p class="text-destructive mt-1 text-xs">
				{$t('domainConnectionCard.certRenewalError', {
					time: new Date(domainConfig.cert_renewal_error_at ?? '').toLocaleString('ko-KR'),
					error: domainConfig.cert_renewal_error
				})}
			</p>
			<p class="text-muted-foreground mt-1 text-xs">
				{$t('domainConnectionCard.certRenewalNotice')}
			</p>
		{/if}
		<button
			class="border-border text-destructive mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
			disabled={domainSaving}
			onclick={onUnregister}
		>
			{domainSaving ? $t('domainConnectionCard.unregistering') : $t('domainConnectionCard.unregister')}
		</button>
	{:else}
		<div class="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-2">
			<div>
				<label class="text-muted-foreground mb-1 block text-xs" for="domain-kind"
					>{$t('domainConnectionCard.kindLabel')}</label
				>
				<div class="relative">
					<select
						id="domain-kind"
						bind:value={domainForm.kind}
						onchange={onKindChange}
						class="border-input bg-background w-full appearance-none rounded-md border py-1.5 pl-3 pr-8 text-sm"
					>
						<option value="main_domain">{$t('domainConnectionCard.kindOptionMainDomain')}</option>
						<option value="free_subdomain"
							>{$t('domainConnectionCard.kindOptionFreeSubdomain')}</option
						>
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
					<label class="text-muted-foreground mb-1 block text-xs" for="domain-provider"
						>{$t('domainConnectionCard.providerLabel')}</label
					>
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
					<label class="text-muted-foreground mb-1 block text-xs" for="domain-hostname"
						>{$t('domainConnectionCard.hostnameLabel')}</label
					>
					<input
						id="domain-hostname"
						type="text"
						placeholder={domainForm.provider === 'iptime'
							? $t('domainConnectionCard.hostnamePlaceholderIptime')
							: $t('domainConnectionCard.hostnamePlaceholderDuckdns')}
						bind:value={domainForm.hostname}
						class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-sm"
					/>
				</div>
				{#if domainForm.provider === 'duckdns'}
					<div class="sm:col-span-2">
						<label class="text-muted-foreground mb-1 block text-xs" for="domain-token"
							>{$t('domainConnectionCard.duckdnsTokenLabel')}</label
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
						{$t('domainConnectionCard.iptimeNotice')}
					</p>
				{/if}
			{:else}
				<div>
					<label class="text-muted-foreground mb-1 block text-xs" for="domain-provider"
						>{$t('domainConnectionCard.providerLabel')}</label
					>
					<input
						id="domain-provider"
						type="text"
						value="Cloudflare"
						disabled
						class="border-input bg-background text-muted-foreground w-full rounded-md border px-3 py-1.5 text-sm"
					/>
				</div>
				<div class="sm:col-span-2">
					<label class="text-muted-foreground mb-1 block text-xs" for="domain-hostname"
						>{$t('domainConnectionCard.domainLabel')}</label
					>
					<input
						id="domain-hostname"
						type="text"
						placeholder={$t('domainConnectionCard.domainPlaceholder')}
						bind:value={domainForm.hostname}
						class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-sm"
					/>
				</div>
				<div class="sm:col-span-2">
					<label class="text-muted-foreground mb-1 block text-xs" for="domain-cf-token"
						>{$t('domainConnectionCard.cfTokenLabel')}</label
					>
					<input
						id="domain-cf-token"
						type="password"
						bind:value={domainForm.token}
						placeholder={$t('domainConnectionCard.cfTokenPlaceholder')}
						class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-sm"
					/>
					<p class="text-muted-foreground mt-1 text-xs">
						{$t('domainConnectionCard.cfTokenNotice')}
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
			{domainSaving ? $t('domainConnectionCard.registering') : $t('domainConnectionCard.register')}
		</button>
	{/if}
	{#if domainError}
		<p class="text-destructive mt-2 text-xs">{domainError}</p>
	{/if}
</div>

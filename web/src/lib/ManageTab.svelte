<script lang="ts">
	import type { Backup, BuildInfo, DomainConfig, Instance, NetworkAddresses, WorldInfo } from '$lib/api';
	import CopyButton from '$lib/CopyButton.svelte';
	import { t } from '$lib/i18n';

	let {
		inst,
		loaderLabel,
		knownLoaders,
		proxyCapableLoaders,
		// 서버 설정 (편집 폼 자체는 ServerSettingsModal -- +page.svelte가 직접 연다)
		pendingRestart,
		restarting,
		onOpenSettingsEdit,
		onRestartForSettings,
		// 접속 주소
		directlyReachable,
		networkAddresses,
		connectPort,
		formatAddress,
		domainConfig,
		subdomain,
		domainAddress,
		domainAddressLabel,
		// 게임플레이 설정
		onOpenGameSettingsModal,
		// 구동기 재설치
		buildOptions,
		buildsError,
		selectedBuildVersion = $bindable(''),
		reinstalling,
		reinstallError,
		reinstallSuccess,
		onReinstallLoader,
		// 프록시 등록
		subdomainError,
		domainSuffix,
		subdomainInput = $bindable(''),
		savingSubdomain,
		registeringProxy,
		unregisteringProxy,
		proxyRegError,
		registeredSecret,
		onRegisterBehindProxy,
		onSaveSubdomain,
		onUnregisterFromProxy,
		// 백업
		backups,
		backupsError,
		creatingBackup,
		busyBackupId,
		onCreateBackup,
		onRestoreBackup,
		onDeleteBackup,
		// 월드 데이터
		worldInfo,
		worldInfoError,
		importFile,
		importing,
		importSuccess,
		importError,
		importForceConfirm,
		onImportFileChange,
		onExportWorld,
		onImportWorld
	}: {
		inst: Instance;
		loaderLabel: (loader: string) => string;
		knownLoaders: string[];
		proxyCapableLoaders: string[];
		pendingRestart: boolean;
		restarting: boolean;
		onOpenSettingsEdit: () => void;
		onRestartForSettings: () => void;
		directlyReachable: boolean;
		networkAddresses: NetworkAddresses | null;
		connectPort: number;
		formatAddress: (host: string, port: number) => string;
		domainConfig: DomainConfig | null;
		subdomain: { registered: boolean; forced_host: string; proxy_port?: number } | null;
		domainAddress: string;
		domainAddressLabel: string;
		onOpenGameSettingsModal: () => void;
		buildOptions: BuildInfo[];
		buildsError: string;
		selectedBuildVersion: string;
		reinstalling: boolean;
		reinstallError: string;
		reinstallSuccess: boolean;
		onReinstallLoader: () => void;
		subdomainError: string;
		domainSuffix: string;
		subdomainInput: string;
		savingSubdomain: boolean;
		registeringProxy: boolean;
		unregisteringProxy: boolean;
		proxyRegError: string;
		registeredSecret: string;
		onRegisterBehindProxy: () => void;
		onSaveSubdomain: () => void;
		onUnregisterFromProxy: () => void;
		backups: Backup[];
		backupsError: string;
		creatingBackup: boolean;
		busyBackupId: string | null;
		onCreateBackup: () => void;
		onRestoreBackup: (backupId: string) => void;
		onDeleteBackup: (backupId: string) => void;
		worldInfo: WorldInfo | null;
		worldInfoError: string;
		importFile: File | null;
		importing: boolean;
		importSuccess: string;
		importError: string;
		importForceConfirm: boolean;
		onImportFileChange: (e: Event) => void;
		onExportWorld: () => void;
		onImportWorld: (force: boolean) => void;
	} = $props();

	let canBackup = $derived(inst.status === 'stopped' || inst.status === 'crashed');

	function formatBytes(bytes: number) {
		return `${(bytes / 1024 / 1024).toFixed(1)}MB`;
	}

	function labelFromForcedHost(forcedHost: string) {
		if (domainSuffix && forcedHost.endsWith(domainSuffix)) {
			return forcedHost.slice(0, -domainSuffix.length);
		}
		return forcedHost;
	}

	// 저장 버튼과 같은 조건(변경 없음/저장 중)일 때는 Enter도 그냥 무시한다
	// -- 버튼이 disabled인 상황에서 엔터로는 저장되는 불일치를 피하기 위해.
	function onSubdomainKeydown(e: KeyboardEvent) {
		if (e.key !== 'Enter') return;
		if (savingSubdomain || subdomainInput.trim() === labelFromForcedHost(subdomain?.forced_host ?? '')) return;
		e.preventDefault();
		onSaveSubdomain();
	}
</script>

<!-- 서버 설정과 게임플레이 설정은 둘 다 짧게 끝나는 카드라 한 줄에 나란히
	둔다 -- proxy 인스턴스는 게임플레이 설정 자체가 없으니(server 전용)
	그 경우 서버 설정이 혼자 전체 폭을 쓴다. -->
<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
	<div class="border-border bg-card rounded-lg border p-4 {inst.kind !== 'server' ? 'md:col-span-2' : ''}">
		<div class="flex items-center justify-between">
			<h2 class="font-medium">{$t('manageTab.serverSettings.title')}</h2>
			<button class="border-border rounded-md border px-3 py-1.5 text-xs" onclick={onOpenSettingsEdit}
				>{$t('manageTab.serverSettings.openButton')}</button
			>
		</div>

		{#if pendingRestart}
			<div
				class="border-border bg-background mt-3 flex items-center justify-between rounded-md border px-3 py-2"
			>
				<p class="text-xs">{$t('manageTab.serverSettings.restartNotice')}</p>
				<button
					class="bg-primary text-primary-foreground shrink-0 rounded-md px-3 py-1.5 text-xs font-medium"
					disabled={restarting}
					onclick={onRestartForSettings}
					>{restarting ? $t('manageTab.serverSettings.restarting') : $t('manageTab.serverSettings.restartButton')}</button
				>
			</div>
		{/if}

		<p class="text-muted-foreground mt-2 text-xs">
			{$t('manageTab.serverSettings.memoryAllocLabel')} {inst.memory_max_mb > 0
				? `${(inst.memory_max_mb / 1024).toFixed(1)}GB`
				: $t('manageTab.common.unlimited')} · {$t('manageTab.serverSettings.cpuAllocLabel')} {inst.cpu_quota_percent >
			0
				? `${inst.cpu_quota_percent}%`
				: $t('manageTab.common.unlimited')}
			{#if inst.kind === 'server'}
				· {$t('manageTab.serverSettings.gamePortLabel')} {inst.game_port}
			{/if}
		</p>
	</div>

	<!-- server.properties GUI form (FR-12) -- a curated, labeled subset; anything
		not listed here is still reachable via the general file manager's raw
		editing (FR-12a), which is aimed at advanced/custom-loader use rather
		than everyday tuning. Opens in a modal (GameSettingsModal) since the
		full form is long -- keeping it inline here would dominate the page. -->
	{#if inst.kind === 'server'}
		<div class="border-border bg-card flex items-center justify-between rounded-lg border p-4">
			<div>
				<h2 class="font-medium">{$t('manageTab.gameplaySettings.title')}</h2>
				<p class="text-muted-foreground mt-1 text-xs">
					{$t('manageTab.gameplaySettings.descPrefix')} <code>server.properties</code>
					{$t('manageTab.gameplaySettings.descSuffix')}
				</p>
			</div>
			<button
				class="border-border shrink-0 rounded-md border px-3 py-1.5 text-xs"
				onclick={onOpenGameSettingsModal}>{$t('manageTab.gameplaySettings.openButton')}</button
			>
		</div>
	{/if}

</div>

<!-- Loader reinstall (FR-4, scoped to same loader + same mc_version -- see
	handleReinstallLoader's doc comment for why nothing broader is offered
	here). 서버 설정/게임플레이 설정과 달리 내용이 길어질 수 있어(빌드
	목록, 안내문 여러 줄) 혼자 전체 폭을 쓴다. -->
<div class="border-border bg-card mt-4 rounded-lg border p-4">
	<h2 class="font-medium">{$t('manageTab.loader.title')}</h2>
	<p class="text-muted-foreground mt-1 text-xs">{loaderLabel(inst.loader)} · {inst.mc_version}</p>
	{#if knownLoaders.includes(inst.loader)}
		<p class="text-muted-foreground mt-1 text-xs">
			{$t('manageTab.loader.restrictionNote')}
		</p>
		{#if buildOptions.length > 0}
			<div class="mt-2">
				<label class="mb-1 block text-xs font-medium" for="reinstall-build">{$t('manageTab.loader.buildLabel')}</label>
				<select
					id="reinstall-build"
					bind:value={selectedBuildVersion}
					class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-xs"
				>
					<option value="">{$t('manageTab.loader.latestOption')}</option>
					{#each buildOptions as build (build.id)}
						<option value={build.id}>{build.id}{build.channel ? ` (${build.channel})` : ''}</option>
					{/each}
				</select>
			</div>
		{:else if buildsError}
			<p class="text-muted-foreground mt-1 text-xs">{$t('manageTab.loader.buildsErrorText', { error: buildsError })}</p>
		{/if}
		<button
			class="border-border mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
			disabled={reinstalling || !canBackup}
			title={canBackup ? '' : $t('manageTab.common.stopServerFirst')}
			onclick={onReinstallLoader}
		>
			{reinstalling
				? $t('manageTab.loader.reinstalling')
				: selectedBuildVersion
					? $t('manageTab.loader.reinstallWithBuild', { build: selectedBuildVersion })
					: $t('manageTab.loader.reinstallLatest')}
		</button>
	{:else}
		<p class="text-muted-foreground mt-1 text-xs">
			{$t('manageTab.loader.customLoaderPrefix')} <code>server.jar</code>{$t('manageTab.loader.customLoaderSuffix')}
		</p>
	{/if}
	{#if reinstallError}
		<p class="text-destructive mt-2 text-xs">{reinstallError}</p>
	{/if}
	{#if reinstallSuccess}
		<p class="mt-2 text-xs text-green-500">{$t('manageTab.loader.reinstallSuccess')}</p>
	{/if}
</div>

<!-- 접속 주소와 프록시는 둘 다 "이 서버에 어떻게 닿는지"에 관한 설정이라
	한 줄에 나란히 둔다. 접속 주소가 조건부로만 보이므로(directlyReachable),
	그게 없을 때는 프록시 카드가 혼자 전체 폭을 쓴다. -->
<div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
	<!-- 접속 주소 복사 버튼 -- 프록시 인스턴스, 독립 노출된 서버, 그리고 이제
		프록시에 등록된 서버(=이 서버 자신의 포트가 아니라 프록시의 포트로
		접속해야 함, connectPort 참고)에도 표시. 공인 IP는 외부 접속이 켜져
		있을 때만 백엔드가 값을 채워 보낸다. -->
	{#if directlyReachable && networkAddresses}
		{@const port = connectPort}
		{@const localAddress = formatAddress(networkAddresses.local_ip, port)}
		{@const publicAddress =
			networkAddresses.public_ip && !domainConfig ? formatAddress(networkAddresses.public_ip, port) : ''}
		<div class="border-border bg-card rounded-lg border p-4">
			<h2 class="font-medium">{$t('manageTab.connectAddress.title')}</h2>
			{#if inst.kind === 'server' && subdomain?.registered}
				<p class="text-muted-foreground mt-1 text-xs">
					{$t('manageTab.connectAddress.behindProxyNote')}
				</p>
			{/if}
			<div class="mt-2 space-y-2">
				{#if domainAddress}
					<div class="flex items-center justify-between gap-2">
						<div class="min-w-0">
							<p class="text-muted-foreground text-xs">{domainAddressLabel}</p>
							<code class="text-sm">{domainAddress}</code>
						</div>
						<CopyButton text={domainAddress} />
					</div>
				{/if}
				<div class="flex items-center justify-between gap-2">
					<div class="min-w-0">
						<p class="text-muted-foreground text-xs">{$t('manageTab.connectAddress.privateIpLabel')}</p>
						<code class="text-sm">{localAddress}</code>
					</div>
					<CopyButton text={localAddress} />
				</div>
				{#if publicAddress}
					<div class="flex items-center justify-between gap-2">
						<div class="min-w-0">
							<p class="text-muted-foreground text-xs">{$t('manageTab.connectAddress.publicIpLabel')}</p>
							<code class="text-sm">{publicAddress}</code>
						</div>
						<CopyButton text={publicAddress} />
					</div>
				{:else if domainConfig}
					<p class="text-muted-foreground text-xs">{$t('manageTab.connectAddress.domainConnectedNote')}</p>
				{:else}
					<p class="text-muted-foreground text-xs">{$t('manageTab.connectAddress.publicHiddenNote')}</p>
				{/if}
			</div>
		</div>
	{/if}

	<!-- Proxy registration -- the operator's one actual proxy-related setting,
		now that the always-on Velocity proxy itself has no UI of its own (see
		ensureProxyInstance/proxyMemoryMaxMB). Shown for every server, not just
		the loaders CraftDeck auto-registers -- a custom loader (FR-3) can still
		be added manually below.

		Per FR-1f, Velocity only exists at all when an owned main domain is
		registered -- with only a free-subdomain DDNS (or nothing) registered,
		there's no proxy to register into, so this card is replaced with a
		one-line explanation instead of showing controls that would just error
		out. -->
	{#if domainConfig?.kind === 'main_domain'}
		<div
			class="border-border bg-card rounded-lg border p-4 {!(directlyReachable && networkAddresses)
				? 'md:col-span-2'
				: ''}"
		>
			<h2 class="font-medium">{$t('manageTab.proxy.title')}</h2>
		{#if inst.loader === 'fabric' || inst.loader === 'neoforge'}
			<p class="mt-1 text-xs text-yellow-500">
				{$t('manageTab.proxy.modIncompatWarning')}
			</p>
		{/if}
		{#if subdomainError}
			<p class="text-destructive mt-2 text-xs">{subdomainError}</p>
		{:else if subdomain && !subdomain.registered}
			<p class="text-muted-foreground mt-2 text-xs">
				{$t('manageTab.proxy.notRegisteredNote')}
				{#if !proxyCapableLoaders.includes(inst.loader)}
					{$t('manageTab.proxy.notRecognizedNote')}
				{/if}
			</p>
			<p class="text-muted-foreground mt-1 text-xs">
				{$t('manageTab.proxy.manualSetupPrefix')}
				<strong>{$t('manageTab.proxy.manualSetupBold')}</strong>{$t('manageTab.proxy.manualSetupSuffix')}
			</p>
			<button
				class="border-border mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
				disabled={registeringProxy || !canBackup}
				title={canBackup ? '' : $t('manageTab.common.stopServerFirst')}
				onclick={onRegisterBehindProxy}
			>
				{registeringProxy ? $t('manageTab.proxy.registering') : $t('manageTab.proxy.registerManualButton')}
			</button>
			{#if proxyRegError}
				<p class="text-destructive mt-2 text-xs">{proxyRegError}</p>
			{/if}
			{#if registeredSecret}
				<div class="border-border bg-background mt-2 rounded-md border p-2">
					<p class="text-muted-foreground text-xs">
						{$t('manageTab.proxy.registeredPrefix')} <code>server-ip</code>/<code>online-mode</code
						>{$t('manageTab.proxy.registeredSuffix')}
					</p>
					<code class="mt-1 block break-all text-xs">{registeredSecret}</code>
				</div>
			{/if}
		{:else if subdomain}
			<p class="text-muted-foreground mt-1 text-xs">
				{$t('manageTab.proxy.subdomainExplainNote')}
			</p>
			<p class="text-muted-foreground mt-1 text-xs">
				{$t('manageTab.proxy.failoverNote')}
			</p>
			<div class="mt-2 flex gap-2">
				{#if domainSuffix}
					<div class="border-input bg-background flex min-w-0 flex-1 items-center rounded-md border px-2 py-1.5">
						<input
							type="text"
							bind:value={subdomainInput}
							onkeydown={onSubdomainKeydown}
							placeholder="survival"
							class="min-w-0 flex-1 bg-transparent text-sm outline-none"
						/>
						<span class="text-muted-foreground shrink-0 text-sm">{domainSuffix}</span>
					</div>
				{:else}
					<input
						type="text"
						bind:value={subdomainInput}
						onkeydown={onSubdomainKeydown}
						placeholder={$t('manageTab.proxy.subdomainPlaceholderExample')}
						class="border-input bg-background min-w-0 flex-1 rounded-md border px-2 py-1.5 text-sm"
					/>
				{/if}
				<button
					class="bg-primary text-primary-foreground shrink-0 rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50"
					disabled={savingSubdomain || subdomainInput.trim() === labelFromForcedHost(subdomain.forced_host)}
					onclick={onSaveSubdomain}
				>
					{savingSubdomain ? $t('manageTab.proxy.savingSubdomain') : $t('manageTab.proxy.saveButton')}
				</button>
			</div>
			<button
				class="border-border text-destructive mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
				disabled={unregisteringProxy || !canBackup}
				title={canBackup ? '' : $t('manageTab.common.stopServerFirst')}
				onclick={onUnregisterFromProxy}
			>
				{unregisteringProxy ? $t('manageTab.proxy.unregistering') : $t('manageTab.proxy.unregisterButton')}
			</button>
			{#if proxyRegError}
				<p class="text-destructive mt-2 text-xs">{proxyRegError}</p>
			{/if}
		{/if}
		</div>
	{:else}
		<div
			class="border-border bg-card rounded-lg border p-4 {!(directlyReachable && networkAddresses)
				? 'md:col-span-2'
				: ''}"
		>
			<h2 class="font-medium">{$t('manageTab.proxy.title')}</h2>
			<p class="text-muted-foreground mt-1 text-xs">
				{$t('manageTab.proxy.noMainDomainNote')}
			</p>
		</div>
	{/if}
</div>

<!-- Backups (FR-13) and world data export/import share one row. Not
	applicable to a Velocity proxy: it has no world of its own. -->
{#if inst.kind === 'server'}
	<div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
		<div class="border-border bg-card rounded-lg border p-4">
			<div class="flex items-center justify-between">
				<h2 class="font-medium">{$t('manageTab.backup.title')}</h2>
				<button
					class="border-border rounded-md border px-3 py-1.5 text-xs"
					disabled={!canBackup || creatingBackup}
					title={canBackup ? '' : $t('manageTab.backup.createButtonTitle')}
					onclick={onCreateBackup}
				>
					{creatingBackup ? $t('manageTab.backup.creating') : $t('manageTab.backup.createButton')}
				</button>
			</div>
			{#if !canBackup}
				<p class="text-muted-foreground mt-1 text-xs">{$t('manageTab.backup.stoppedOnlyNote')}</p>
			{/if}
			{#if backupsError}
				<p class="text-destructive mt-2 text-xs">{backupsError}</p>
			{/if}
			{#if backups.length === 0}
				<p class="text-muted-foreground mt-2 text-xs">{$t('manageTab.backup.noBackupsNote')}</p>
			{:else}
				<div class="mt-2 space-y-1.5">
					{#each backups as b (b.id)}
						<div class="border-border flex items-center justify-between rounded-md border px-2 py-1.5 text-xs">
							<span>{b.filename} · {formatBytes(b.size_bytes)}</span>
							<div class="flex gap-1.5">
								<button
									class="border-border rounded-md border px-2 py-1 text-xs"
									disabled={!canBackup || busyBackupId === b.id}
									onclick={() => onRestoreBackup(b.id)}>{$t('manageTab.backup.restoreButton')}</button
								>
								<button
									class="border-border text-destructive rounded-md border px-2 py-1 text-xs"
									disabled={busyBackupId === b.id}
									onclick={() => onDeleteBackup(b.id)}>{$t('manageTab.backup.deleteButton')}</button
								>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>

		<!-- World data export/import -->
		<div class="border-border bg-card rounded-lg border p-4">
			<h2 class="font-medium">{$t('manageTab.worldData.title')}</h2>
			{#if worldInfoError}
				<p class="text-destructive mt-2 text-xs">{worldInfoError}</p>
			{:else if worldInfo}
				<p class="text-muted-foreground mt-2 text-xs">
					{$t('manageTab.worldData.infoLine', {
						level: worldInfo.level_name,
						version: worldInfo.instance_version,
						detected:
							worldInfo.detected_version ||
							$t('manageTab.worldData.unknownVersion', { error: worldInfo.detect_error })
					})}
				</p>
			{/if}

			<div class="border-border mt-3 grid grid-cols-1 divide-y sm:grid-cols-2 sm:divide-x sm:divide-y-0">
				<div class="pb-3 sm:pr-4 sm:pb-0">
					<span class="text-muted-foreground mb-1 block text-xs">{$t('manageTab.worldData.exportLabel')}</span>
					<button
						class="border-border rounded-md border px-3 py-1.5 text-xs"
						disabled={!canBackup}
						title={canBackup ? '' : $t('manageTab.worldData.exportButtonTitle')}
						onclick={onExportWorld}>{$t('manageTab.worldData.exportButton')}</button
					>
				</div>
				<div class="pt-3 sm:pt-0 sm:pl-4">
					<span class="text-muted-foreground mb-1 block text-xs">{$t('manageTab.worldData.importLabel')}</span>
					<div class="flex items-center justify-between gap-2">
						<input
							type="file"
							accept=".gz,.tar.gz"
							onchange={onImportFileChange}
							class="text-muted-foreground file:border-border file:bg-background file:text-foreground file:mr-2 file:rounded-md file:border file:px-3 file:py-1.5 file:text-xs file:font-medium file:cursor-pointer min-w-0 text-xs"
						/>
						<button
							class="border-border shrink-0 rounded-md border px-3 py-1.5 text-xs"
							disabled={!canBackup || !importFile || importing}
							title={canBackup ? '' : $t('manageTab.worldData.importButtonTitle')}
							onclick={() => onImportWorld(false)}
						>
							{importing ? $t('manageTab.worldData.importing') : $t('manageTab.worldData.importButton')}
						</button>
					</div>
				</div>
			</div>
			{#if !canBackup}
				<p class="text-muted-foreground mt-1 text-xs">{$t('manageTab.worldData.stoppedOnlyNote')}</p>
			{/if}
			{#if importSuccess}
				<p class="mt-2 text-xs text-green-500">{importSuccess}</p>
			{/if}
			{#if importError}
				<p class="text-destructive mt-2 text-xs">{importError}</p>
				{#if importForceConfirm}
					<button
						class="bg-destructive text-destructive-foreground mt-2 rounded-md px-3 py-1.5 text-xs font-medium"
						disabled={importing}
						onclick={() => onImportWorld(true)}>{$t('manageTab.worldData.forceApplyButton')}</button
					>
				{/if}
			{/if}
		</div>
	</div>
{/if}

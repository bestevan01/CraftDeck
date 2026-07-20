<script lang="ts">
	import type { Backup, BuildInfo, DomainConfig, Instance, NetworkAddresses, WorldInfo } from '$lib/api';
	import MemorySlider from '$lib/MemorySlider.svelte';
	import CopyButton from '$lib/CopyButton.svelte';

	let {
		inst,
		loaderLabel,
		knownLoaders,
		proxyCapableLoaders,
		// 서버 설정
		editingSettings,
		pendingRestart,
		restarting,
		settingsMemoryGB = $bindable(1),
		settingsCpu = $bindable(0),
		maxMemoryGB,
		ramBoundaryGB,
		settingsError,
		settingsSaving,
		onOpenSettingsEdit,
		onRestartForSettings,
		onSaveSettings,
		onCancelSettingsEdit,
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
		editingSettings: boolean;
		pendingRestart: boolean;
		restarting: boolean;
		settingsMemoryGB: number;
		settingsCpu: number;
		maxMemoryGB: number;
		ramBoundaryGB: number;
		settingsError: string;
		settingsSaving: boolean;
		onOpenSettingsEdit: () => void;
		onRestartForSettings: () => void;
		onSaveSettings: () => void;
		onCancelSettingsEdit: () => void;
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

</script>

<div class="border-border bg-card rounded-lg border p-4">
	<div class="flex items-center justify-between">
		<h2 class="font-medium">서버 설정</h2>
		{#if !editingSettings}
			<button class="border-border rounded-md border px-3 py-1.5 text-xs" onclick={onOpenSettingsEdit}
				>설정 변경</button
			>
		{/if}
	</div>

	{#if pendingRestart}
		<div
			class="border-border bg-background mt-3 flex items-center justify-between rounded-md border px-3 py-2"
		>
			<p class="text-xs">변경된 설정은 재시작해야 적용됩니다.</p>
			<button
				class="bg-primary text-primary-foreground shrink-0 rounded-md px-3 py-1.5 text-xs font-medium"
				disabled={restarting}
				onclick={onRestartForSettings}>{restarting ? '재시작 중...' : '재시작'}</button
			>
		</div>
	{/if}

	{#if editingSettings}
		<div class="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
			{#if inst.kind === 'proxy'}
				<div>
					<span class="text-muted-foreground mb-1 block text-xs">메모리 할당</span>
					<p class="mt-1.5 text-sm">1GB (고정)</p>
				</div>
			{:else}
				<div>
					<label class="text-muted-foreground mb-1 block text-xs" for="settings-memory">
						메모리 할당 ({settingsMemoryGB}GB / 최대 {maxMemoryGB}GB{#if ramBoundaryGB < maxMemoryGB}<span
								class="text-yellow-500"> · 스왑 {maxMemoryGB - ramBoundaryGB}GB 포함</span
							>{/if})
					</label>
					<MemorySlider id="settings-memory" bind:value={settingsMemoryGB} maxGB={maxMemoryGB} {ramBoundaryGB} />
				</div>
			{/if}
			<div>
				<label class="text-muted-foreground mb-1 block text-xs" for="settings-cpu">
					CPU 할당량 ({settingsCpu > 0 ? `${settingsCpu}%` : '무제한'})
				</label>
				<input
					id="settings-cpu"
					type="range"
					min="0"
					max="100"
					step="5"
					bind:value={settingsCpu}
					class="w-full"
				/>
			</div>
		</div>
		{#if settingsError}
			<p class="text-destructive mt-2 text-xs">{settingsError}</p>
		{/if}
		<div class="mt-3 flex gap-2">
			<button
				class="bg-primary text-primary-foreground rounded-md px-3 py-1.5 text-xs font-medium"
				disabled={settingsSaving}
				onclick={onSaveSettings}>저장</button
			>
			<button class="border-border rounded-md border px-3 py-1.5 text-xs" onclick={onCancelSettingsEdit}
				>취소</button
			>
		</div>
	{:else}
		<p class="text-muted-foreground mt-2 text-xs">
			메모리 할당 {inst.memory_max_mb > 0
				? `${(inst.memory_max_mb / 1024).toFixed(1)}GB`
				: '무제한'} · CPU 할당 {inst.cpu_quota_percent > 0 ? `${inst.cpu_quota_percent}%` : '무제한'}
		</p>
	{/if}
</div>

<!-- server.properties GUI form (FR-12)과 구동기 재설치는 둘 다 "이 서버를
	무엇으로 구동할지"에 관한 설정이라 한 줄에 나란히 둔다 -- 게임플레이
	설정은 모달을 여는 버튼 하나뿐이라 가로로 길 필요가 없다. 구동기는
	proxy 인스턴스에도 적용되므로(게임플레이 설정은 server 전용) 그 경우
	혼자 전체 폭을 쓴다. -->
<div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
	<!-- server.properties GUI form (FR-12) -- a curated, labeled subset; anything
		not listed here is still reachable via the general file manager's raw
		editing (FR-12a), which is aimed at advanced/custom-loader use rather
		than everyday tuning. Opens in a modal (GameSettingsModal) since the
		full form is long -- keeping it inline here would dominate the page. -->
	{#if inst.kind === 'server'}
		<div class="border-border bg-card flex items-center justify-between rounded-lg border p-4">
			<div>
				<h2 class="font-medium">게임플레이 설정</h2>
				<p class="text-muted-foreground mt-1 text-xs">
					난이도, 게임 모드, 최대 인원 등 자주 쓰는 <code>server.properties</code> 옵션
				</p>
			</div>
			<button
				class="border-border shrink-0 rounded-md border px-3 py-1.5 text-xs"
				onclick={onOpenGameSettingsModal}>열기</button
			>
		</div>
	{/if}

	<!-- Loader reinstall (FR-4, scoped to same loader + same mc_version -- see
		handleReinstallLoader's doc comment for why nothing broader is offered
		here). -->
	<div
		class="border-border bg-card rounded-lg border p-4 {inst.kind !== 'server' ? 'md:col-span-2' : ''}"
	>
		<h2 class="font-medium">구동기</h2>
		<p class="text-muted-foreground mt-1 text-xs">{loaderLabel(inst.loader)} · {inst.mc_version}</p>
		{#if knownLoaders.includes(inst.loader)}
			<p class="text-muted-foreground mt-1 text-xs">
				같은 구동기·같은 마인크래프트 버전 안에서만 빌드를 다시 받습니다. 다른 구동기나 버전으로
				바꾸는 기능은 월드/플러그인 호환성이 깨질 수 있어 제공하지 않습니다.
			</p>
			{#if buildOptions.length > 0}
				<div class="mt-2">
					<label class="mb-1 block text-xs font-medium" for="reinstall-build">빌드</label>
					<select
						id="reinstall-build"
						bind:value={selectedBuildVersion}
						class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-xs"
					>
						<option value="">최신</option>
						{#each buildOptions as build (build.id)}
							<option value={build.id}>{build.id}{build.channel ? ` (${build.channel})` : ''}</option>
						{/each}
					</select>
				</div>
			{:else if buildsError}
				<p class="text-muted-foreground mt-1 text-xs">빌드 목록을 불러오지 못했습니다: {buildsError}</p>
			{/if}
			<button
				class="border-border mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
				disabled={reinstalling || !canBackup}
				title={canBackup ? '' : '먼저 서버를 종료하세요'}
				onclick={onReinstallLoader}
			>
				{reinstalling ? '재설치 중...' : selectedBuildVersion ? `빌드 ${selectedBuildVersion}로 재설치` : '최신 빌드로 재설치'}
			</button>
		{:else}
			<p class="text-muted-foreground mt-1 text-xs">
				커스텀 구동기입니다. 새 jar로 교체하려면 파일 탭에서 <code>server.jar</code>를 직접 업로드하세요.
			</p>
		{/if}
		{#if reinstallError}
			<p class="text-destructive mt-2 text-xs">{reinstallError}</p>
		{/if}
		{#if reinstallSuccess}
			<p class="mt-2 text-xs text-green-500">재설치됐습니다. 다시 시작하면 적용됩니다.</p>
		{/if}
	</div>
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
			<h2 class="font-medium">접속 주소</h2>
			{#if inst.kind === 'server' && subdomain?.registered}
				<p class="text-muted-foreground mt-1 text-xs">
					이 서버는 프록시 뒤에 있어 프록시의 포트로 접속합니다. 서브도메인이 지정되어 있으면 그
					주소로, 아니면 프록시의 우선순위에 따라 다른 서버로 연결될 수 있습니다.
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
						<p class="text-muted-foreground text-xs">사설 IP (같은 네트워크에서)</p>
						<code class="text-sm">{localAddress}</code>
					</div>
					<CopyButton text={localAddress} />
				</div>
				{#if publicAddress}
					<div class="flex items-center justify-between gap-2">
						<div class="min-w-0">
							<p class="text-muted-foreground text-xs">공인 IP (외부에서)</p>
							<code class="text-sm">{publicAddress}</code>
						</div>
						<CopyButton text={publicAddress} />
					</div>
				{:else if domainConfig}
					<p class="text-muted-foreground text-xs">도메인이 연결되어 있어 공인 IP 대신 위 주소를 사용하세요.</p>
				{:else}
					<p class="text-muted-foreground text-xs">외부 접속이 꺼져 있어 공인 IP 주소는 표시하지 않습니다.</p>
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
			<h2 class="font-medium">프록시</h2>
		{#if inst.loader === 'fabric' || inst.loader === 'neoforge'}
			<p class="mt-1 text-xs text-yellow-500">
				⚠ 일부 모드(엔티티·블록 상태 등 바닐라 패킷 구조 자체를 변형하는 모드, 예: Create)는
				Velocity와 호환되지 않아 접속 중 "A packet did not decode successfully" 오류로 끊길 수
				있습니다. 이런 모드를 쓴다면 프록시 등록 대신 독립 노출을 사용하세요.
			</p>
		{/if}
		{#if subdomainError}
			<p class="text-destructive mt-2 text-xs">{subdomainError}</p>
		{:else if subdomain && !subdomain.registered}
			<p class="text-muted-foreground mt-2 text-xs">
				이 서버는 프록시에 등록되어 있지 않습니다 (독립적으로 노출된 서버).
				{#if !proxyCapableLoaders.includes(inst.loader)}
					CraftDeck이 이 구동기를 자동으로 인식하지 못해 등록되지 않았습니다.
				{/if}
			</p>
			<p class="text-muted-foreground mt-1 text-xs">
				수동으로 등록하려면, 이 서버가 실제로 Velocity의 모던 포워딩(공유 시크릿)을 신뢰하도록
				<strong>직접 설정되어 있어야 합니다</strong>. CraftDeck은 임의의 구동기 jar가 이걸
				지원하는지 확인할 방법이 없습니다. 잘못 설정된 채로 등록하면 접속이 실패합니다.
			</p>
			<button
				class="border-border mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
				disabled={registeringProxy || !canBackup}
				title={canBackup ? '' : '먼저 서버를 종료하세요'}
				onclick={onRegisterBehindProxy}
			>
				{registeringProxy ? '등록 중...' : '프록시에 수동으로 등록'}
			</button>
			{#if proxyRegError}
				<p class="text-destructive mt-2 text-xs">{proxyRegError}</p>
			{/if}
			{#if registeredSecret}
				<div class="border-border bg-background mt-2 rounded-md border p-2">
					<p class="text-muted-foreground text-xs">
						등록됐습니다. 아래 시크릿을 이 서버의 forwarding 설정(로더에 따라 다름, 파일 탭에서
						직접 편집)에 붙여넣고, <code>server-ip</code>/<code>online-mode</code>는 이미 자동으로
						반영했습니다. 재시작 후 적용됩니다.
					</p>
					<code class="mt-1 block break-all text-xs">{registeredSecret}</code>
				</div>
			{/if}
		{:else if subdomain}
			<p class="text-muted-foreground mt-1 text-xs">
				이 서브도메인으로 접속하면 프록시가 이 서버로 연결합니다. 변경 후 프록시가 자동으로
				재시작되어 반영됩니다.
			</p>
			<p class="text-muted-foreground mt-1 text-xs">
				다른 서버에도 같은 서브도메인을 지정하면, 먼저 만든 서버가 우선순위 1순위가 되어 평소엔
				그쪽으로 연결되고, 그 서버가 다운되면 다음 순위 서버로 자동 장애조치됩니다 (복구되면 새
				접속부터 다시 1순위로 자동 복귀).
			</p>
			<div class="mt-2 flex gap-2">
				{#if domainSuffix}
					<div class="border-input bg-background flex min-w-0 flex-1 items-center rounded-md border px-2 py-1.5">
						<input
							type="text"
							bind:value={subdomainInput}
							placeholder="survival"
							class="min-w-0 flex-1 bg-transparent text-sm outline-none"
						/>
						<span class="text-muted-foreground shrink-0 text-sm">{domainSuffix}</span>
					</div>
				{:else}
					<input
						type="text"
						bind:value={subdomainInput}
						placeholder="예: survival.example.com"
						class="border-input bg-background min-w-0 flex-1 rounded-md border px-2 py-1.5 text-sm"
					/>
				{/if}
				<button
					class="bg-primary text-primary-foreground shrink-0 rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50"
					disabled={savingSubdomain || subdomainInput.trim() === labelFromForcedHost(subdomain.forced_host)}
					onclick={onSaveSubdomain}
				>
					{savingSubdomain ? '저장 중...' : '저장'}
				</button>
			</div>
			<button
				class="border-border text-destructive mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
				disabled={unregisteringProxy || !canBackup}
				title={canBackup ? '' : '먼저 서버를 종료하세요'}
				onclick={onUnregisterFromProxy}
			>
				{unregisteringProxy ? '전환 중...' : '독립 노출로 전환'}
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
			<h2 class="font-medium">프록시</h2>
			<p class="text-muted-foreground mt-1 text-xs">
				소유한 메인 도메인이 연결되어 있어야 프록시(Velocity)가 동작합니다. 무료 DDNS 서브도메인만
				등록했거나 도메인이 없는 경우, 이 서버는 독립 노출(자신의 게임 포트로 직접 접속)로만
				운영됩니다.
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
				<h2 class="font-medium">백업</h2>
				<button
					class="border-border rounded-md border px-3 py-1.5 text-xs"
					disabled={!canBackup || creatingBackup}
					title={canBackup ? '' : '백업을 만들려면 먼저 서버를 종료하세요'}
					onclick={onCreateBackup}
				>
					{creatingBackup ? '생성 중...' : '백업 생성'}
				</button>
			</div>
			{#if !canBackup}
				<p class="text-muted-foreground mt-1 text-xs">백업 생성/복원은 서버가 정지된 상태에서만 가능합니다.</p>
			{/if}
			{#if backupsError}
				<p class="text-destructive mt-2 text-xs">{backupsError}</p>
			{/if}
			{#if backups.length === 0}
				<p class="text-muted-foreground mt-2 text-xs">백업이 아직 없습니다.</p>
			{:else}
				<div class="mt-2 space-y-1.5">
					{#each backups as b (b.id)}
						<div class="border-border flex items-center justify-between rounded-md border px-2 py-1.5 text-xs">
							<span>{b.filename} · {formatBytes(b.size_bytes)}</span>
							<div class="flex gap-1.5">
								<button
									class="border-border rounded-md border px-2 py-1 text-xs"
									disabled={!canBackup || busyBackupId === b.id}
									onclick={() => onRestoreBackup(b.id)}>복원</button
								>
								<button
									class="border-border text-destructive rounded-md border px-2 py-1 text-xs"
									disabled={busyBackupId === b.id}
									onclick={() => onDeleteBackup(b.id)}>삭제</button
								>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>

		<!-- World data export/import -->
		<div class="border-border bg-card rounded-lg border p-4">
			<h2 class="font-medium">월드 데이터</h2>
			{#if worldInfoError}
				<p class="text-destructive mt-2 text-xs">{worldInfoError}</p>
			{:else if worldInfo}
				<p class="text-muted-foreground mt-2 text-xs">
					폴더명 {worldInfo.level_name} · 인스턴스 버전 {worldInfo.instance_version} · 감지된 월드
					버전 {worldInfo.detected_version || `알 수 없음 (${worldInfo.detect_error})`}
				</p>
			{/if}

			<div class="border-border mt-3 grid grid-cols-1 divide-y sm:grid-cols-2 sm:divide-x sm:divide-y-0">
				<div class="pb-3 sm:pr-4 sm:pb-0">
					<span class="text-muted-foreground mb-1 block text-xs">내보내기</span>
					<button
						class="border-border rounded-md border px-3 py-1.5 text-xs"
						disabled={!canBackup}
						title={canBackup ? '' : '내보내려면 먼저 서버를 종료하세요'}
						onclick={onExportWorld}>월드 데이터 다운로드</button
					>
				</div>
				<div class="pt-3 sm:pt-0 sm:pl-4">
					<span class="text-muted-foreground mb-1 block text-xs">가져오기 (tar.gz 업로드)</span>
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
							title={canBackup ? '' : '가져오려면 먼저 서버를 종료하세요'}
							onclick={() => onImportWorld(false)}
						>
							{importing ? '가져오는 중...' : '가져오기'}
						</button>
					</div>
				</div>
			</div>
			{#if !canBackup}
				<p class="text-muted-foreground mt-1 text-xs">내보내기/가져오기는 서버가 정지된 상태에서만 가능합니다.</p>
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
						onclick={() => onImportWorld(true)}>그래도 강제 적용</button
					>
				{/if}
			{/if}
		</div>
	</div>
{/if}

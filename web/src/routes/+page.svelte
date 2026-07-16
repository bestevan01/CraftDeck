<script lang="ts">
	import {
		api,
		PROXY_RESERVED_MEMORY_MB,
		type Instance,
		type SystemResources,
		type ProxyStatus,
		type BuildInfo,
		type NetworkSettings,
		type PortMapping,
		type DomainConfig,
		type SwapInfo
	} from '$lib/api';
	import MemorySlider from '$lib/MemorySlider.svelte';
	import ConfirmDialog from '$lib/ConfirmDialog.svelte';
	import { onDestroy, onMount } from 'svelte';

	// Shared by every destructive action on this page (see ConfirmDialog.svelte
	// for why this replaced the browser's native confirm()).
	let confirmOpen = $state(false);
	let confirmMessage = $state('');
	let confirmAction = $state<() => void>(() => {});
	function askConfirm(message: string, action: () => void) {
		confirmMessage = message;
		confirmAction = action;
		confirmOpen = true;
	}

	async function logout() {
		await api.logout();
		window.location.href = '/login';
	}

	// 내부 네트워크(LAN)에서는 로그인 절차 자체를 건너뛰므로(백엔드
	// requireAuth의 lan_bypass 참고), 그 상태에서 "로그아웃"이나
	// "비밀번호 변경" 버튼을 보여주는 건 의미가 없어서 실제로 로그인된
	// 세션이 있을 때만 두 버튼을 노출한다.
	let isLoggedIn = $state(false);

	// 비밀번호 변경 + 2단계 인증은 둘 다 "계정" 설정이라 별개 버튼/모달로
	// 나눠두는 게 오히려 헷갈려서(둘 다 로그인/보안에 관한 것), 하나의
	// "계정 설정" 모달 안에 두 섹션으로 묶었다.
	let username = $state('');
	let showAccountModal = $state(false);
	let currentPassword = $state('');
	let newPassword = $state('');
	let newPasswordConfirm = $state('');
	let passwordError = $state('');
	let passwordChanged = $state(false);
	let changingPassword = $state(false);

	// FR-38/39: 2FA setup. totpEnabled gates the "외부 접속" toggle
	// backend-side (FR-38) -- this is just the UI to actually get there.
	let totpEnabled = $state(false);
	let totpQrCode = $state('');
	let totpSecret = $state('');
	let totpVerifyCode = $state('');
	let totpError = $state('');
	let startingTOTPSetup = $state(false);
	let verifyingTOTP = $state(false);
	let totpBackupCodes = $state<string[] | null>(null);

	async function openAccountModal() {
		currentPassword = '';
		newPassword = '';
		newPasswordConfirm = '';
		passwordError = '';
		passwordChanged = false;
		totpError = '';
		totpVerifyCode = '';
		totpBackupCodes = null;
		showAccountModal = true;
		if (totpEnabled) return; // already set up -- see the modal's own "already enabled" branch
		startingTOTPSetup = true;
		try {
			const setup = await api.setupTOTP();
			totpQrCode = setup.qr_code_png;
			totpSecret = setup.secret;
		} catch (err) {
			totpError = err instanceof Error ? err.message : String(err);
		} finally {
			startingTOTPSetup = false;
		}
	}

	async function changePassword(e: SubmitEvent) {
		e.preventDefault();
		if (newPassword !== newPasswordConfirm) {
			passwordError = '새 비밀번호가 서로 일치하지 않습니다.';
			return;
		}
		passwordError = '';
		passwordChanged = false;
		changingPassword = true;
		try {
			await api.changePassword(username, currentPassword, newPassword);
			currentPassword = '';
			newPassword = '';
			newPasswordConfirm = '';
			passwordChanged = true;
		} catch (err) {
			passwordError = err instanceof Error ? err.message : String(err);
		} finally {
			changingPassword = false;
		}
	}

	async function submitTOTPVerify(e: SubmitEvent) {
		e.preventDefault();
		totpError = '';
		verifyingTOTP = true;
		try {
			const result = await api.verifyTOTP(totpVerifyCode);
			totpBackupCodes = result.backup_codes;
			totpEnabled = true;
		} catch (err) {
			totpError = err instanceof Error ? err.message : String(err);
		} finally {
			verifyingTOTP = false;
		}
	}

	let instances = $state<Instance[]>([]);
	// 프록시 인스턴스는 목록에 표시하지 않는다 (항상 켜져 있는 내부
	// 구성요소일 뿐이라 운영자가 볼 필요가 없다) -- 메모리 예산 계산 등
	// 나머지 로직에서는 여전히 instances 원본을 그대로 쓴다.
	let visibleInstances = $derived(instances.filter((i) => i.kind !== 'proxy'));
	let loadError = $state('');
	let showCreateForm = $state(false);
	let creating = $state(false);
	let createError = $state('');
	let busyId = $state<string | null>(null);

	// Optional world-data injection at creation time (requirements.md FR-13
	// extension): if set, uploaded right after the instance itself is
	// created, before its first start.
	let worldFile = $state<File | null>(null);
	let worldFileForce = $state(false);

	function onWorldFileChange(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		worldFile = input.files?.[0] ?? null;
	}

	let resources = $state<SystemResources | null>(null);
	let resourceError = $state('');

	async function refreshResources() {
		try {
			resources = await api.systemResources();
			resourceError = '';
		} catch (err) {
			resourceError = err instanceof Error ? err.message : String(err);
		}
	}

	function usagePercent(used: number, total: number) {
		if (total <= 0) return 0;
		return Math.min(100, (used / total) * 100);
	}

	function barClass(percent: number) {
		if (percent >= 90) return 'bg-destructive';
		if (percent >= 75) return 'bg-yellow-500';
		return 'bg-primary';
	}

	// Raspberry Pi SoCs start throttling clock speed around 80-85degC, so
	// that's the destructive threshold here -- 70 is just an early warning.
	function tempTextClass(tempC: number) {
		if (tempC >= 80) return 'text-destructive';
		if (tempC >= 70) return 'text-yellow-500';
		return '';
	}

	// The always-on Velocity proxy's version is picked once, at creation,
	// and never re-checked afterward (see ensureProxyInstance) -- so it can
	// silently fall behind newer Velocity releases, including ones that add
	// support for a new Minecraft protocol version entirely (as happened
	// when Minecraft's 26.x "year.release" scheme shipped well after this
	// proxy had already been created and pinned to an older build). This
	// panel surfaces that gap and lets the operator apply the update.
	let proxyStatus = $state<ProxyStatus | null>(null);
	let proxyUpgrading = $state(false);
	let proxyUpgradeError = $state('');

	async function refreshProxyStatus() {
		try {
			proxyStatus = await api.getProxyStatus();
			proxyUpgradeError = '';
		} catch {
			// non-critical panel -- leave last known status as-is
		}
	}

	async function upgradeProxy() {
		proxyUpgrading = true;
		proxyUpgradeError = '';
		try {
			await api.upgradeProxy();
			await refreshProxyStatus();
		} catch (err) {
			proxyUpgradeError = err instanceof Error ? err.message : String(err);
		} finally {
			proxyUpgrading = false;
		}
	}

	// FR-21/22/23/25: "외부 접속 허용" (web UI port only so far -- the game
	// port half of FR-25 isn't wired up yet). Toggling attempts UPnP then
	// NAT-PMP automatically; manual_info is shown when both fail (FR-23).
	let networkSettings = $state<NetworkSettings | null>(null);
	let networkToggling = $state(false);
	let networkError = $state('');
	let portMappings = $state<PortMapping[]>([]);
	let deletingMappingId = $state('');
	// FR-34: turning WAN exposure on needs an explicit warning + a nudge
	// toward a strong password before it actually takes effect -- turning
	// it off needs no such confirmation.
	let showWANWarningModal = $state(false);

	async function refreshNetworkSettings() {
		try {
			networkSettings = await api.getNetworkSettings();
			networkError = '';
		} catch {
			// non-critical panel -- leave last known status as-is
		}
	}

	async function refreshPortMappings() {
		try {
			portMappings = await api.listPortMappings();
		} catch {
			// non-critical panel -- leave last known list as-is
		}
	}

	// FR-34: enabling the toggle only opens the warning modal -- the actual
	// API call happens in confirmWANEnable once the operator acknowledges
	// it. Disabling needs no confirmation, so it goes straight through.
	function onWANToggleChange(enabled: boolean) {
		if (enabled) {
			showWANWarningModal = true;
			return;
		}
		toggleWANEnabled(false);
	}

	function confirmWANEnable() {
		showWANWarningModal = false;
		toggleWANEnabled(true);
	}

	async function toggleWANEnabled(enabled: boolean) {
		networkToggling = true;
		networkError = '';
		try {
			networkSettings = await api.setWANEnabled(enabled);
			await refreshPortMappings();
		} catch (err) {
			networkError = err instanceof Error ? err.message : String(err);
		} finally {
			networkToggling = false;
		}
	}

	async function deletePortMapping(id: string) {
		deletingMappingId = id;
		try {
			await api.deletePortMapping(id);
			await Promise.all([refreshNetworkSettings(), refreshPortMappings()]);
		} catch (err) {
			networkError = err instanceof Error ? err.message : String(err);
		} finally {
			deletingMappingId = '';
		}
	}

	function mappingMethodLabel(method: PortMapping['method']) {
		return { upnp: 'UPnP', natpmp: 'NAT-PMP', manual: '수동' }[method] ?? method;
	}

	// 가상 메모리(디스크 스왑파일) -- 라즈베리파이 OS의 zram(RAM 압축 스왑)과는
	// 별개로 동작하는, CraftDeck 전용 디스크 기반 스왑. RAM+zram으로도 부족할
	// 때를 대비한 추가 여유분 성격이라 커널이 항상 실제 RAM/zram을 먼저 쓰고
	// 남을 때만 사용한다.
	let swapInfo = $state<SwapInfo | null>(null);
	// GB in the UI, converted to/from the API's MB at the boundary --
	// backend (internal/swap) still stores/reports everything in MB.
	let swapSizeInput = $state('');
	let swapSaving = $state(false);
	let swapError = $state('');

	async function refreshSwap() {
		try {
			swapInfo = await api.getSwap();
			swapFetchError = '';
			if (!swapSizeInput && swapInfo.size_mb > 0) {
				swapSizeInput = String(swapInfo.size_mb / 1024);
			}
		} catch (err) {
			swapFetchError = err instanceof Error ? err.message : String(err);
		}
	}

	async function saveSwap() {
		const sizeGB = Number(swapSizeInput);
		if (!Number.isFinite(sizeGB) || sizeGB <= 0) {
			swapError = '0보다 큰 크기를 GB 단위로 입력하세요.';
			return;
		}
		const sizeMB = Math.round(sizeGB * 1024);
		swapSaving = true;
		swapError = '';
		try {
			swapInfo = await api.setSwap(sizeMB);
		} catch (err) {
			swapError = err instanceof Error ? err.message : String(err);
		} finally {
			swapSaving = false;
		}
	}

	function disableSwap() {
		askConfirm('스왑파일을 완전히 끄고 삭제할까요?', doDisableSwap);
	}

	async function doDisableSwap() {
		swapSaving = true;
		swapError = '';
		try {
			await api.deleteSwap();
			swapInfo = await api.getSwap();
			swapSizeInput = '';
		} catch (err) {
			swapError = err instanceof Error ? err.message : String(err);
		} finally {
			swapSaving = false;
		}
	}

	function mappingOwnerLabel(mapping: PortMapping) {
		if (!mapping.instance_id) return '웹 UI';
		return instances.find((i) => i.id === mapping.instance_id)?.name ?? mapping.instance_id;
	}

	// FR-26 minimal skeleton + FR-1f: whether an owned domain is registered
	// decides whether Velocity runs at all (see ReconcileProxyMode) -- a
	// free-subdomain DDNS provider can only ever point at one server (FR-27)
	// so it doesn't make a multi-server proxy worthwhile either.
	let domainConfig = $state<DomainConfig | null>(null);
	let domainError = $state('');
	let domainSaving = $state(false);
	let domainForm = $state({
		kind: 'main_domain' as 'main_domain' | 'free_subdomain',
		provider: 'cloudflare',
		hostname: '',
		token: ''
	});

	// FR-26a: DuckDNS (active renewal) and ipTime (감시 전용, FR-26b/e) are
	// the only free-subdomain providers implemented so far.
	const freeProviders = [
		{ value: 'duckdns', label: 'DuckDNS' },
		{ value: 'iptime', label: 'ipTime (자동 갱신 불가, 감시 전용)' }
	];

	// A token is required for main_domain (Cloudflare, FR-31's zone-access
	// check) and for the active-renewal free-subdomain provider (DuckDNS,
	// FR-26c) -- not for the monitor-only one (ipTime, FR-26e).
	let domainTokenRequired = $derived(
		domainForm.kind === 'main_domain' ||
			(domainForm.kind === 'free_subdomain' && domainForm.provider === 'duckdns')
	);

	function onDomainKindChange() {
		if (domainForm.kind === 'free_subdomain' && !domainForm.provider) {
			domainForm.provider = 'duckdns';
		} else if (domainForm.kind === 'main_domain') {
			// FR-28~31's automation only talks to Cloudflare's API right now
			// (internal/dns.VerifyZoneAccess) -- the backend rejects anything
			// else, so there's no reason to make the operator type it.
			domainForm.provider = 'cloudflare';
		}
	}

	async function refreshDomainSettings() {
		try {
			const res = await api.getDomainSettings();
			domainConfig = 'id' in res ? res : null;
		} catch {
			// non-critical panel -- leave last known status as-is
		}
	}

	async function saveDomainSettings() {
		domainSaving = true;
		domainError = '';
		try {
			domainConfig = await api.setDomainSettings(
				domainForm.kind,
				domainForm.provider,
				domainForm.hostname,
				domainForm.token || undefined
			);
			domainForm.token = ''; // never linger in memory longer than needed
		} catch (err) {
			domainError = err instanceof Error ? err.message : String(err);
		} finally {
			domainSaving = false;
		}
	}

	async function unregisterDomain() {
		domainSaving = true;
		domainError = '';
		try {
			await api.deleteDomainSettings();
			domainConfig = null;
			domainForm = { kind: 'main_domain', provider: '', hostname: '', token: '' };
		} catch (err) {
			domainError = err instanceof Error ? err.message : String(err);
		} finally {
			domainSaving = false;
		}
	}

	// Loaders that can sit behind CraftDeck's Velocity proxy -- Purpur/Folia/
	// Pufferfish/Leaf are Paper forks that carry proxies.velocity forward
	// unchanged, and Fabric gets there via an auto-installed FabricProxy-Lite
	// mod (see installFabricProxyMods). Mirrors supportsVelocityForwarding in
	// internal/api/handlers_proxy.go.
	const proxyCapableLoaders = ['paper', 'purpur', 'folia', 'pufferfish', 'leaf', 'fabric', 'neoforge'];

	let form = $state({
		name: '',
		loader: 'vanilla' as
			| 'vanilla'
			| 'paper'
			| 'purpur'
			| 'folia'
			| 'pufferfish'
			| 'leaf'
			| 'fabric'
			| 'neoforge'
			| 'custom',
		mc_version: '',
		// Empty means "always latest" -- see buildListerLoaders/loadBuilds below.
		loader_version: '',
		memory_gb: 2,
		cpu_quota_percent: 0, // 0 = unlimited
		accept_eula: false,
		// Paper-family servers sit behind CraftDeck's always-on Velocity
		// proxy by default (game_port stays internal-only) -- see
		// handleCreateInstance. Vanilla can't do modern forwarding at all,
		// so it's always independently exposed regardless of this flag.
		expose_independently: false
	});

	// FR-3: a custom, unlisted loader has no adapter to fetch a version list
	// or auto-download a jar from, so it gets its own free-text name/version
	// fields and a required jar upload instead of the normal dropdown +
	// live-fetched version select.
	let customLoaderName = $state('');
	let customJarFile = $state<File | null>(null);
	function onCustomJarFileChange(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		customJarFile = input.files?.[0] ?? null;
	}

	function openCreateForm() {
		showCreateForm = true;
	}

	// Caps the create-form memory slider at the Pi's actual RAM, plus
	// CraftDeck's own swap file's size if the operator has turned it on
	// (instances only actually get to use that swap when it's on -- see
	// AllowSwap in startInstanceCore), minus the always-on Velocity proxy's
	// fixed 1GB reservation -- but only while the proxy actually exists AND
	// is running; if it's torn down (FR-1f, no main domain registered) or
	// just not running right now, that 1GB isn't reserved by anything.
	let availableMemoryMB = $derived.by(() => {
		if (!resources) return 1024;
		let total = resources.total_memory_mb;
		if (swapInfo?.enabled) total += swapInfo.size_mb;
		if (proxyStatus?.exists && proxyStatus.running) total -= PROXY_RESERVED_MEMORY_MB;
		return total;
	});
	let maxMemoryGB = $derived(Math.max(1, Math.floor(availableMemoryMB / 1024)));
	// Where the slider's marker sits -- physical RAM alone, minus the same
	// proxy reservation availableMemoryMB subtracts, so the marker lines up
	// with "this much is real RAM" rather than counting the reserved 1GB
	// as swappable headroom.
	let ramBoundaryGB = $derived.by(() => {
		if (!resources) return maxMemoryGB;
		let ram = resources.total_memory_mb;
		if (proxyStatus?.exists && proxyStatus.running) ram -= PROXY_RESERVED_MEMORY_MB;
		return Math.max(1, Math.floor(ram / 1024));
	});

	// Version lists for the create-instance dropdown, fetched live from each
	// loader's own distribution API (the same ones internal/loader/*.go use
	// to actually download the server jar) so the list an operator picks
	// from always matches what's downloadable. Vanilla's manifest includes
	// snapshots, so it's filtered to release-only; the rest only ever list
	// versions they have real builds for.
	let vanillaVersionIds = $state<string[]>([]);
	let paperVersionIds = $state<string[]>([]);
	let purpurVersionIds = $state<string[]>([]);
	let foliaVersionIds = $state<string[]>([]);
	let pufferfishVersionIds = $state<string[]>([]);
	let leafVersionIds = $state<string[]>([]);
	let fabricVersionIds = $state<string[]>([]);
	let neoforgeVersionIds = $state<string[]>([]);
	let mcVersionsError = $state('');

	let availableVersionIds = $derived(
		({
			vanilla: vanillaVersionIds,
			paper: paperVersionIds,
			purpur: purpurVersionIds,
			folia: foliaVersionIds,
			pufferfish: pufferfishVersionIds,
			leaf: leafVersionIds,
			fabric: fabricVersionIds,
			neoforge: neoforgeVersionIds
		}[form.loader as string] ?? []) // 'custom' has no fetched version list -- see the free-text mc_version input instead
	);

	async function loadMcVersions() {
		try {
			const [vanilla, paper, purpur, folia, pufferfish, leaf, fabric, neoforge] = await Promise.all([
				api.listVanillaVersions(),
				api.listPaperVersions(),
				api.listPurpurVersions(),
				api.listFoliaVersions(),
				api.listPufferfishVersions(),
				api.listLeafVersions(),
				api.listFabricVersions(),
				api.listNeoForgeVersions()
			]);
			vanillaVersionIds = vanilla.filter((v) => v.type === 'release').map((v) => v.id);
			// These APIs already list newest-first, same as vanilla's manifest.
			paperVersionIds = paper;
			purpurVersionIds = purpur;
			foliaVersionIds = folia;
			pufferfishVersionIds = pufferfish;
			leafVersionIds = leaf;
			fabricVersionIds = fabric;
			neoforgeVersionIds = neoforge;
			if (!form.mc_version && availableVersionIds.length > 0) {
				form.mc_version = availableVersionIds[0];
			}
			mcVersionsError = '';
		} catch (err) {
			mcVersionsError = err instanceof Error ? err.message : String(err);
		}
	}

	function onLoaderChange() {
		form.mc_version = availableVersionIds[0] ?? '';
	}

	// Loaders whose adapter implements BuildLister (see internal/loader) --
	// the only ones where picking a specific build (rather than always
	// getting whatever's newest) means anything.
	const buildListerLoaders = ['paper', 'purpur', 'folia', 'leaf', 'neoforge'];
	let buildOptions = $state<BuildInfo[]>([]);
	let buildsError = $state('');
	let loadingBuilds = $state(false);

	async function loadBuilds() {
		form.loader_version = '';
		if (!buildListerLoaders.includes(form.loader) || !form.mc_version) {
			buildOptions = [];
			buildsError = '';
			return;
		}
		loadingBuilds = true;
		try {
			buildOptions = await api.listLoaderBuilds(form.loader, form.mc_version);
			buildsError = '';
		} catch (err) {
			buildOptions = [];
			buildsError = err instanceof Error ? err.message : String(err);
		} finally {
			loadingBuilds = false;
		}
	}

	$effect(() => {
		// Re-fetch whenever the loader or mc_version selection changes.
		form.loader;
		form.mc_version;
		loadBuilds();
	});

	async function refresh() {
		try {
			instances = await api.listInstances();
			loadError = '';
		} catch (err) {
			loadError = err instanceof Error ? err.message : String(err);
		}
		// Port-forwarding state (ReconcileGamePorts) changes automatically as
		// instances start/stop, so it needs to poll on the same cadence as
		// the instance list itself -- otherwise "외부 접속" 카드 shows stale
		// mappings until a manual page reload.
		await refreshPortMappings();
		await refreshNetworkSettings();
		// Same reasoning for the Velocity 프록시 card: ReconcileProxyMode can
		// create or tear down the proxy as a side effect of registering/
		// unregistering a domain (FR-1f), so proxyStatus.exists needs to be
		// re-checked on the same cadence too -- confirmed on real hardware
		// that without this, the card lingered after unregistering the main
		// domain until a manual page reload.
		await refreshProxyStatus();
	}

	let pollHandle: ReturnType<typeof setInterval>;
	let resourcePollHandle: ReturnType<typeof setInterval>;
	onMount(() => {
		refresh();
		refreshResources();
		refreshProxyStatus();
		refreshDomainSettings();
		refreshSwap();
		loadMcVersions();
		api.authStatus().then((s) => {
			username = s.username;
			isLoggedIn = s.authenticated;
			totpEnabled = s.totp_enabled;
		});
		pollHandle = setInterval(refresh, 3000);
		resourcePollHandle = setInterval(refreshResources, 5000);
	});
	onDestroy(() => {
		clearInterval(pollHandle);
		clearInterval(resourcePollHandle);
	});

	async function createInstance() {
		createError = '';
		if (form.loader === 'custom') {
			if (!customLoaderName.trim()) {
				createError = '구동기 이름을 입력해주세요.';
				return;
			}
			if (!customJarFile) {
				createError = '구동기 jar 파일을 선택해주세요.';
				return;
			}
		}
		creating = true;
		try {
			const created = await api.createInstance({
				name: form.name,
				kind: 'server',
				loader: form.loader === 'custom' ? customLoaderName.trim() : form.loader,
				loader_version: form.loader === 'custom' ? undefined : form.loader_version || undefined,
				mc_version: form.mc_version,
				memory_max_mb: form.memory_gb * 1024,
				cpu_quota_percent: form.cpu_quota_percent,
				accept_eula: form.accept_eula,
				expose_independently: form.expose_independently
			});

			if (form.loader === 'custom' && customJarFile) {
				try {
					await api.uploadServerJar(created.id, customJarFile);
				} catch (err) {
					// Same reasoning as the world-import failure below: the
					// instance itself exists, only the jar upload failed, so
					// surface it directly rather than via createError (the
					// modal is closing regardless).
					alert(
						'서버는 생성됐지만 구동기 jar 업로드에 실패했습니다: ' +
							(err instanceof Error ? err.message : String(err)) +
							'\n인스턴스 상세 페이지의 파일 탭에서 server.jar를 직접 업로드할 수 있습니다.'
					);
				}
			}

			if (worldFile) {
				try {
					await api.importWorld(created.id, worldFile, worldFileForce);
				} catch (err) {
					// The instance itself was created fine -- only the world
					// injection failed (e.g. version mismatch without force
					// checked). The create modal is about to close either way,
					// so alert() here since createError would never be seen.
					alert(
						'서버는 생성됐지만 월드 데이터 적용에 실패했습니다: ' +
							(err instanceof Error ? err.message : String(err)) +
							'\n인스턴스 상세 페이지에서 다시 시도할 수 있습니다.'
					);
				}
			}

			showCreateForm = false;
			form = {
				name: '',
				loader: 'vanilla',
				mc_version: vanillaVersionIds[0] ?? '',
				loader_version: '',
				memory_gb: 2,
				cpu_quota_percent: 0,
				accept_eula: false,
				expose_independently: false
			};
			worldFile = null;
			worldFileForce = false;
			customLoaderName = '';
			customJarFile = null;
			await refresh();
		} catch (err) {
			createError = err instanceof Error ? err.message : String(err);
		} finally {
			creating = false;
		}
	}

	// If starting this instance would push the combined memory allocation of
	// every running/starting instance past what the Pi actually has, offer a
	// modal to redistribute allocations right away instead of just letting
	// the JVMs fight over real RAM (or OOM-kill each other) after the fact.
	type MemoryConflictItem = {
		id: string;
		name: string;
		memoryGB: number;
		isTarget: boolean;
		isRunning: boolean;
	};
	let showMemoryConflictModal = $state(false);
	let conflictTargetId = $state('');
	let conflictItems = $state<MemoryConflictItem[]>([]);
	let conflictError = $state('');
	let applyingConflict = $state(false);

	// The proxy's 1GB isn't resizable here (see openMemoryConflictModal), so
	// the budget being negotiated among the listed servers excludes it too
	// (when it's actually reserved at all -- see availableMemoryMB).
	let conflictMaxGB = $derived(Math.max(1, Math.floor(availableMemoryMB / 1024)));
	let conflictTotalGB = $derived(conflictItems.reduce((sum, i) => sum + i.memoryGB, 0));
	let conflictOverBudget = $derived(conflictTotalGB > conflictMaxGB);

	function openMemoryConflictModal(target: Instance, runningOthers: Instance[]) {
		conflictTargetId = target.id;
		conflictError = '';
		// The proxy's memory is fixed (see PROXY_RESERVED_MEMORY_MB) -- it's
		// still counted against the total in start()'s projectedMB check, but
		// it isn't offered here as something the operator can shrink.
		const adjustableOthers = runningOthers.filter((i) => i.kind !== 'proxy');
		conflictItems = [target, ...adjustableOthers].map((i) => ({
			id: i.id,
			name: i.name,
			memoryGB: Math.max(1, Math.round(i.memory_max_mb / 1024) || 1),
			isTarget: i.id === target.id,
			isRunning: i.status === 'running' || i.status === 'starting'
		}));
		showMemoryConflictModal = true;
	}

	async function applyConflictAndStart() {
		applyingConflict = true;
		conflictError = '';
		try {
			for (const item of conflictItems) {
				const original = instances.find((i) => i.id === item.id);
				if (!original) continue;
				const currentGB = Math.round(original.memory_max_mb / 1024);
				if (currentGB === item.memoryGB) continue;
				await api.updateInstance(item.id, {
					cpu_quota_percent: original.cpu_quota_percent,
					memory_max_mb: item.memoryGB * 1024
				});
				// Already-running instances (not the target, which is about to
				// be started fresh right below and picks up its new allocation
				// automatically) need an explicit restart to actually free up
				// the memory this negotiation assumed they would -- otherwise
				// the old JVM keeps running at its old size until the operator
				// happens to restart it some other time, and "적용 후 시작"
				// only *looked* like it resolved the conflict (confirmed: this
				// was exactly the bug -- already-running instances never
				// actually got restarted).
				if (item.isRunning && !item.isTarget) {
					await api.restartInstance(item.id);
				}
			}
			showMemoryConflictModal = false;
			await start(conflictTargetId, true);
		} catch (err) {
			conflictError = err instanceof Error ? err.message : String(err);
		} finally {
			applyingConflict = false;
		}
	}

	async function start(id: string, skipMemoryCheck = false) {
		if (!skipMemoryCheck) {
			const target = instances.find((i) => i.id === id);
			const runningOthers = instances.filter(
				(i) => i.id !== id && (i.status === 'running' || i.status === 'starting')
			);
			const projectedMB =
				runningOthers.reduce((sum, i) => sum + i.memory_max_mb, 0) + (target?.memory_max_mb ?? 0);
			if (target && resources && projectedMB > availableMemoryMB) {
				openMemoryConflictModal(target, runningOthers);
				return;
			}
		}
		busyId = id;
		try {
			await api.startInstance(id);
			await refresh();
		} finally {
			busyId = null;
		}
	}

	async function stop(id: string) {
		busyId = id;
		try {
			await api.stopInstance(id);
			await refresh();
		} finally {
			busyId = null;
		}
	}

	function remove(id: string) {
		askConfirm('이 인스턴스를 삭제할까요? 월드 데이터도 함께 지워집니다.', () => doRemove(id));
	}

	async function doRemove(id: string) {
		busyId = id;
		try {
			await api.deleteInstance(id);
			await refresh();
		} finally {
			busyId = null;
		}
	}

	// The backend stores loader names lowercase (matches internal/loader's
	// registry keys), but their proper names are capitalized.
	const loaderLabels: Record<string, string> = {
		vanilla: 'Vanilla',
		paper: 'Paper',
		purpur: 'Purpur',
		folia: 'Folia',
		pufferfish: 'Pufferfish',
		leaf: 'Leaf',
		fabric: 'Fabric',
		neoforge: 'NeoForge',
		velocity: 'Velocity'
	};
	function loaderLabel(loader: string) {
		return loaderLabels[loader] ?? loader;
	}

	function statusLabel(status: Instance['status']) {
		return (
			{
				stopped: '중지됨',
				starting: '시작 중',
				running: '실행 중',
				stopping: '종료 중',
				crashed: '비정상 종료'
			}[status] ?? status
		);
	}

	function statusDotClass(status: Instance['status']) {
		if (status === 'running') return 'bg-green-500';
		if (status === 'starting' || status === 'stopping') return 'bg-yellow-500';
		if (status === 'crashed') return 'bg-destructive';
		return 'bg-muted-foreground';
	}

	// Splits the instance list from the host-wide settings cards (Velocity
	// 프록시/외부 접속/가상 메모리/도메인 연결), same tab pattern as the
	// instance detail page -- "라즈베리파이 리소스" stays in its own sticky
	// sidebar regardless of which tab is active, since it's a live status
	// readout an operator would want visible no matter what they're doing.
	let activeTab = $state<'instances' | 'settings'>('instances');
</script>

<main class="bg-background text-foreground flex flex-col p-8 lg:h-screen lg:overflow-hidden">
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-semibold">CraftDeck</h1>
		<div class="flex gap-2">
			<button
				class="bg-primary text-primary-foreground rounded-md px-4 py-2 text-sm font-medium"
				onclick={openCreateForm}
			>
				+ 서버 만들기
			</button>
			{#if isLoggedIn}
				<button
					class="border-border rounded-md border px-4 py-2 text-sm font-medium"
					onclick={openAccountModal}
				>
					계정 설정
				</button>
				<button
					class="border-border rounded-md border px-4 py-2 text-sm font-medium"
					onclick={logout}
				>
					로그아웃
				</button>
			{/if}
		</div>
	</div>

	{#if loadError}
		<p class="text-destructive mt-4 text-sm">서버 목록을 불러오지 못했습니다: {loadError}</p>
	{/if}

	<div class="border-border mt-6 flex gap-1 border-b">
		<button
			class="border-b-2 px-3 py-2 text-sm {activeTab === 'instances'
				? 'border-primary font-medium'
				: 'text-muted-foreground border-transparent'}"
			onclick={() => (activeTab = 'instances')}>인스턴스</button
		>
		<button
			class="border-b-2 px-3 py-2 text-sm {activeTab === 'settings'
				? 'border-primary font-medium'
				: 'text-muted-foreground border-transparent'}"
			onclick={() => (activeTab = 'settings')}>전역 설정</button
		>
	</div>

	<div class="mt-6 grid grid-cols-1 gap-6 lg:min-h-0 lg:flex-1 lg:grid-cols-3">
		<div class="lg:col-span-2 lg:flex lg:min-h-0 lg:flex-col">
		{#if activeTab === 'instances'}
		<div class="space-y-3 lg:min-h-0 lg:flex-1 lg:overflow-y-auto lg:pr-3">
			<!-- Velocity 프록시는 항상 켜져 있는 내부 구성요소일 뿐, 운영자가
				직접 만들거나 관리할 대상이 아니라서(ensureProxyInstance 참고)
				목록에 아예 보이지 않는다. -->
			{#if visibleInstances.length === 0 && !loadError}
				<p class="text-muted-foreground text-sm">서버 인스턴스가 아직 없습니다.</p>
			{/if}
			{#each visibleInstances as inst (inst.id)}
				<div class="border-border bg-card flex items-center justify-between rounded-lg border p-4">
					<div>
						<div class="flex items-center gap-2">
							<span class="h-2 w-2 rounded-full {statusDotClass(inst.status)}"></span>
							<a href="/instances/{inst.id}" class="font-medium hover:underline">{inst.name}</a>
							<span class="text-muted-foreground text-xs">{statusLabel(inst.status)}</span>
						</div>
						<p class="text-muted-foreground mt-1 text-xs">
							{loaderLabel(inst.loader)} · {inst.mc_version} · Java {inst.java_major}
						</p>
					</div>
					<div class="flex gap-2">
						{#if inst.status === 'running' || inst.status === 'starting'}
							<button
								disabled={busyId === inst.id}
								onclick={() => stop(inst.id)}
								class="border-border rounded-md border px-3 py-1.5 text-sm disabled:opacity-50"
							>
								종료
							</button>
						{:else}
							<button
								disabled={busyId === inst.id}
								onclick={() => start(inst.id)}
								class="border-border rounded-md border px-3 py-1.5 text-sm disabled:opacity-50"
							>
								시작
							</button>
						{/if}
						<a
							href="/instances/{inst.id}"
							class="border-border rounded-md border px-3 py-1.5 text-sm">콘솔</a
						>
						<button
							disabled={busyId === inst.id}
							onclick={() => remove(inst.id)}
							class="border-border text-destructive rounded-md border px-3 py-1.5 text-sm disabled:opacity-50"
						>
							삭제
						</button>
					</div>
				</div>
			{/each}
			</div>
			{:else}
			<div class="space-y-4 lg:min-h-0 lg:flex-1 lg:overflow-y-auto lg:pr-3">
			{#if proxyStatus?.exists}
				<div class="border-border bg-card rounded-lg border p-4">
					<h2 class="font-medium">Velocity 프록시</h2>
					<p class="text-muted-foreground mt-2 text-xs">
						현재 버전: {proxyStatus.current_version}
					</p>
					{#if proxyStatus.update_available}
						<p class="mt-1 text-xs text-yellow-500">
							최신 버전 {proxyStatus.latest_version} 사용 가능 (새 마인크래프트 프로토콜 지원이
							추가됐을 수 있습니다)
						</p>
						<button
							disabled={proxyUpgrading}
							onclick={upgradeProxy}
							class="border-border mt-2 rounded-md border px-3 py-1.5 text-sm disabled:opacity-50"
						>
							{proxyUpgrading ? '업데이트 중... (프록시가 잠시 재시작됩니다)' : '프록시 업데이트'}
						</button>
					{:else}
						<p class="text-muted-foreground mt-1 text-xs">최신 버전입니다.</p>
					{/if}
					{#if proxyUpgradeError}
						<p class="text-destructive mt-2 text-xs">{proxyUpgradeError}</p>
					{/if}
				</div>
			{/if}

			<!-- FR-21/22/23/25: 외부 접속 허용 (웹 UI 포트 + 켜진 인스턴스의 게임 포트) -->
			<div class="border-border bg-card rounded-lg border p-4">
				<div class="flex items-center justify-between">
					<h2 class="font-medium">외부 접속</h2>
					<label class="inline-flex cursor-pointer items-center gap-2 text-sm">
						<input
							type="checkbox"
							checked={networkSettings?.wan_enabled ?? false}
							disabled={networkToggling || !networkSettings}
							onchange={(e) => onWANToggleChange((e.target as HTMLInputElement).checked)}
						/>
						{networkToggling ? '적용 중...' : networkSettings?.wan_enabled ? '켜짐' : '꺼짐'}
					</label>
				</div>
				<p class="text-muted-foreground mt-1 text-xs">
					켜면 관리 웹 UI 포트와, 실행 중인 인스턴스 중 실제로 접속 가능한 것(Velocity 프록시 또는
					독립 노출된 서버)의 게임 포트를 UPnP(IGD)나 NAT-PMP로 공유기에 자동 등록합니다. 인스턴스를
					시작/종료하면 그 인스턴스의 포트도 자동으로 열리고 닫힙니다. 둘 다 지원하지 않거나
					실패하면 직접 설정할 정보를 안내합니다. 켜져 있는 동안은 같은 네트워크(LAN) 안에서도
					로그인이 필요합니다.
				</p>
				{#if networkError}
					<p class="text-destructive mt-2 text-xs">{networkError}</p>
				{/if}
				{#if networkSettings?.wan_enabled && networkSettings.web_mapping}
					<p class="mt-2 text-xs text-green-500">
						웹 UI: {mappingMethodLabel(networkSettings.web_mapping.method)} 자동 등록됨 (외부 포트 {networkSettings
							.web_mapping.external_port})
					</p>
				{:else if networkSettings?.wan_enabled && networkSettings.manual_info}
					<div class="border-border bg-background mt-2 rounded-md border p-3 text-xs">
						<p class="mb-1 font-medium">자동 등록에 실패했습니다 -- 공유기에서 직접 설정하세요:</p>
						<p>내부 IP: {networkSettings.manual_info.local_ip}</p>
						<p>포트: {networkSettings.manual_info.internal_port}</p>
						<p>프로토콜: {networkSettings.manual_info.protocol.toUpperCase()}</p>
					</div>
				{/if}

				{#if portMappings.length > 0}
					<div class="mt-3">
						<p class="text-muted-foreground mb-1 text-xs font-medium">등록된 포트포워딩 규칙</p>
						<div class="space-y-1.5">
							{#each portMappings as mapping (mapping.id)}
								<div
									class="border-border flex items-center justify-between rounded-md border px-2 py-1.5 text-xs"
								>
									<span>
										{mappingOwnerLabel(mapping)} · {mapping.external_port} → {mapping.internal_port}/{mapping.protocol.toUpperCase()}
										· {mappingMethodLabel(mapping.method)}
									</span>
									<button
										class="border-border rounded-md border px-2 py-1 text-xs disabled:opacity-50"
										disabled={deletingMappingId === mapping.id}
										onclick={() => deletePortMapping(mapping.id)}
									>
										{deletingMappingId === mapping.id ? '삭제 중...' : '삭제'}
									</button>
								</div>
							{/each}
						</div>
					</div>
				{/if}
			</div>

			<!-- 가상 메모리(디스크 스왑파일) -- 라즈베리파이 OS의 zram(RAM 압축
				스왑)과 별개로 동작하는 CraftDeck 전용 디스크 기반 스왑. SD카드/eMMC
				부팅 환경(swapInfo.supported === false)에서는 카드 자체를 아예
				숨긴다 -- 랜덤 쓰기 성능/수명이 나빠서 켜라고 권할 이유가 없음. 다만
				이건 "확인해보니 지원 안 함"으로 확정된 경우만이고, 조회 자체가
				실패한 경우(swapFetchError)는 구분해서 에러로 보여준다 -- 안 그러면
				일시적 네트워크 오류와 "이 하드웨어는 지원 안 함"이 똑같이 카드가
				사라지는 걸로 보여서 구분이 안 됐다. -->
			{#if swapInfo === null || swapInfo.supported}
				<div class="border-border bg-card rounded-lg border p-4">
					<h2 class="font-medium">가상 메모리 (스왑)</h2>
					<p class="text-muted-foreground mt-1 text-xs">
						라즈베리파이 OS의 zram(RAM 내 압축 스왑)과는 별개로, 실제 디스크 공간을 추가
						여유분으로 씁니다. 커널은 항상 실제 RAM과 zram을 먼저 쓰고, 그걸로도 부족할 때만
						이 스왑파일을 사용합니다.
					</p>
					{#if swapFetchError}
						<p class="text-destructive mt-2 text-xs">
							상태를 불러오지 못했습니다: {swapFetchError}
						</p>
					{:else if swapInfo}
						<p class="mt-2 text-xs">
							{#if swapInfo.enabled}
								<span class="text-green-500">켜짐</span> -- {(swapInfo.size_mb / 1024).toFixed(
									1
								)}GB 중 {(swapInfo.used_mb / 1024).toFixed(1)}GB 사용 중
							{:else if swapInfo.size_mb > 0}
								<span class="text-muted-foreground">꺼짐</span> (파일은 {(
									swapInfo.size_mb / 1024
								).toFixed(1)}GB로 남아있음)
							{:else}
								<span class="text-muted-foreground">설정 안 됨</span>
							{/if}
						</p>
						<p class="text-muted-foreground mt-1 text-xs">
							여유 공간: {(swapInfo.free_disk_mb / 1024).toFixed(1)}GB (스왑파일 자체 크기 포함)
						</p>
						<div class="mt-2 flex gap-2">
							<div
								class="border-input bg-background flex min-w-0 flex-1 items-center rounded-md border px-2 py-1.5"
							>
								<input
									type="number"
									min="0.1"
									step="0.1"
									bind:value={swapSizeInput}
									placeholder="예: 4"
									class="min-w-0 flex-1 bg-transparent text-sm outline-none"
								/>
								<span class="text-muted-foreground shrink-0 text-sm">GB</span>
							</div>
							<button
								class="bg-primary text-primary-foreground shrink-0 rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50"
								disabled={swapSaving}
								onclick={saveSwap}
							>
								{swapSaving ? '적용 중...' : '적용'}
							</button>
						</div>
						{#if swapInfo.enabled}
							<button
								class="border-border text-destructive mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
								disabled={swapSaving}
								onclick={disableSwap}
							>
								끄고 삭제
							</button>
						{/if}
						{#if swapError}
							<p class="text-destructive mt-2 text-xs">{swapError}</p>
						{/if}
					{:else}
						<p class="text-muted-foreground mt-2 text-xs">불러오는 중...</p>
					{/if}
				</div>
			{/if}

			<!-- FR-26 minimal skeleton + FR-1f: 도메인 연결 여부가 Velocity
				기본 프록시 사용 여부를 결정합니다. -->
			<div class="border-border bg-card rounded-lg border p-4">
				<h2 class="font-medium">도메인 연결</h2>
				<p class="text-muted-foreground mt-1 text-xs">
					소유한 메인 도메인을 연결하면 Velocity 프록시가 자동으로 켜져서 여러 서버를 서브도메인으로
					묶어 접속할 수 있게 됩니다. 도메인이 없거나 무료 DDNS 서브도메인만 쓰는 경우 서브도메인
					라우팅 자체가 실제로 닿지 않으므로, Velocity는 꺼지고 각 서버가 포트로 직접 노출됩니다.
				</p>
				{#if domainConfig}
					<p class="mt-2 text-xs">
						<strong>{domainConfig.kind === 'main_domain' ? '메인 도메인' : '무료 DDNS'}</strong>
						연결됨 -- {domainConfig.hostname} ({domainConfig.provider})
					</p>
					{#if domainConfig.kind === 'free_subdomain'}
						{#if domainConfig.mode === 'monitor'}
							<p class="text-muted-foreground mt-1 text-xs">
								이 제공자는 자동 갱신을 지원하지 않으며 공유기 자체 DDNS 기능에 의존합니다.
								CraftDeck은 주기적으로 이 호스트명이 실제 공인 IP를 가리키는지만 확인합니다.
							</p>
							{#if domainConfig.mismatch_detected}
								<p class="text-destructive mt-1 text-xs">
									⚠ 이 호스트명이 현재 공인 IP와 일치하지 않습니다 -- 공유기의 ipTime DDNS 기능이
									꺼졌거나 실패했을 수 있습니다.
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
							다음 접속 시도에서 자동으로 재시도하며, 그때까지는 자체 서명 인증서로 대체됩니다.
							Cloudflare 토큰이 만료/취소되지 않았는지 확인하세요.
						</p>
					{/if}
					<button
						class="border-border text-destructive mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
						disabled={domainSaving}
						onclick={unregisterDomain}
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
									onchange={onDomainKindChange}
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
								<label class="text-muted-foreground mb-1 block text-xs" for="domain-provider"
									>제공자</label
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
									>호스트명</label
								>
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
									공유기 관리 페이지에서 이미 설정해둔 ipTime DDNS 호스트명을 그대로 입력하세요.
									이 제공자는 CraftDeck이 직접 갱신할 수 없어 공유기 자체 기능에 의존하며,
									CraftDeck은 감시만 합니다.
								</p>
							{/if}
						{:else}
							<div>
								<label class="text-muted-foreground mb-1 block text-xs" for="domain-provider"
									>제공자</label
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
									>도메인</label
								>
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
									Cloudflare 대시보드 &gt; My Profile &gt; API Tokens에서 "Edit zone DNS" 템플릿으로
									이 도메인 존 하나만 범위를 제한해 발급하세요. 이 토큰으로 해당 존에 실제 접근
									가능한지 확인해 도메인 소유권 검증을 대신합니다.
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
						onclick={saveDomainSettings}
					>
						{domainSaving ? '등록 중...' : '등록'}
					</button>
				{/if}
				{#if domainError}
					<p class="text-destructive mt-2 text-xs">{domainError}</p>
				{/if}
			</div>
			</div>
			{/if}
		</div>

		<!-- 라즈베리파이 리소스는 실행 중인 인스턴스/전역 설정 중 무엇을 보고
			있든 운영자가 항상 확인하고 싶어할 라이브 상태 값이라, 탭 전환과
			무관하게 별도 사이드바에 고정해서 보여준다. -->
		<div class="lg:col-span-1 lg:min-h-0 lg:overflow-y-auto lg:pr-3">
			<div class="border-border bg-card rounded-lg border p-4">
				<h2 class="font-medium">라즈베리파이 리소스</h2>
				{#if resources}
					{@const swapTotalMB = swapInfo?.enabled ? swapInfo.size_mb : 0}
					{@const swapUsedMB = swapInfo?.enabled ? swapInfo.used_mb : 0}
					{@const memCombinedTotalMB = resources.total_memory_mb + swapTotalMB}
					{@const memRAMPercentOfBar = usagePercent(resources.used_memory_mb, memCombinedTotalMB)}
					{@const memSwapPercentOfBar = usagePercent(swapUsedMB, memCombinedTotalMB)}
					{@const memRAMOwnPercent = usagePercent(resources.used_memory_mb, resources.total_memory_mb)}
					{@const diskPercent = usagePercent(resources.used_disk_mb, resources.total_disk_mb)}
					<div class="mt-3 space-y-4">
						<div>
							<div class="mb-1 flex justify-between text-xs">
								<span class="text-muted-foreground">CPU 사용률</span>
								<span
									>{resources.cpu_percent.toFixed(0)}% ({resources.cpu_count}코어){#if resources.cpu_temp_c !== undefined}<span
											class={tempTextClass(resources.cpu_temp_c)}
											>
											· {resources.cpu_temp_c.toFixed(1)}°C</span
										>{/if}</span
								>
							</div>
							<div class="bg-background h-2 overflow-hidden rounded-full">
								<div
									class="h-full {barClass(resources.cpu_percent)}"
									style="width: {Math.min(100, resources.cpu_percent)}%"
								></div>
							</div>
						</div>
						<div>
							<div class="mb-1 flex justify-between text-xs">
								<span class="text-muted-foreground">
									메모리{#if swapTotalMB > 0}<span class="text-yellow-500"> (+스왑)</span>{/if}
								</span>
								<span
									>{(resources.used_memory_mb / 1024).toFixed(1)}{#if swapUsedMB > 0}<span
											class="text-yellow-500">+{(swapUsedMB / 1024).toFixed(1)}</span
										>{/if}GB / {(memCombinedTotalMB / 1024).toFixed(1)}GB</span
								>
							</div>
							<!-- 라즈베리파이 OS의 zram(RAM 압축 스왑)과 별개인 CraftDeck 자체
								디스크 스왑(FR-46)이 켜져 있으면, 막대 전체 길이를 물리 RAM+스왑
								합산 용량 기준으로 놓고 두 구간으로 나눠 표시한다: 물리 RAM 사용량은
								기존과 같은 임계값 색(barClass), 스왑 사용량 구간은 항상 노란색으로
								구분해서 "지금 스왑까지 파고들었다"는 걸 한눈에 보이게 한다. -->
							<div class="bg-background flex h-2 overflow-hidden rounded-full">
								<div
									class="h-full {barClass(memRAMOwnPercent)}"
									style="width: {memRAMPercentOfBar}%"
								></div>
								{#if memSwapPercentOfBar > 0}
									<div class="h-full bg-yellow-500" style="width: {memSwapPercentOfBar}%"></div>
								{/if}
							</div>
						</div>
						<div>
							<div class="mb-1 flex justify-between text-xs">
								<span class="text-muted-foreground">디스크</span>
								<span
									>{(resources.used_disk_mb / 1024).toFixed(1)}GB / {(
										resources.total_disk_mb / 1024
									).toFixed(1)}GB</span
								>
							</div>
							<div class="bg-background h-2 overflow-hidden rounded-full">
								<div
									class="h-full {barClass(diskPercent)}"
									style="width: {diskPercent}%"
								></div>
							</div>
						</div>
					</div>
				{:else if resourceError}
					<p class="text-destructive mt-3 text-xs">{resourceError}</p>
				{:else}
					<p class="text-muted-foreground mt-3 text-xs">불러오는 중...</p>
				{/if}
			</div>
		</div>
	</div>
</main>

{#if showCreateForm}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
		onclick={() => (showCreateForm = false)}
		onkeydown={(e) => {
			if (e.key === 'Escape') showCreateForm = false;
		}}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="border-border bg-card max-h-[90vh] w-full max-w-md overflow-y-auto rounded-lg border p-4 shadow-lg"
			onclick={(e) => e.stopPropagation()}
		>
			<div class="mb-3 flex items-center justify-between">
				<h2 class="font-medium">서버 만들기</h2>
				<button
					type="button"
					class="text-muted-foreground text-sm"
					onclick={() => (showCreateForm = false)}>&times;</button
				>
			</div>
			<form
				class="space-y-4"
				onsubmit={(e) => {
					e.preventDefault();
					createInstance();
				}}
			>
				<div>
					<label class="mb-1 block text-sm font-medium" for="name">이름</label>
					<input
						id="name"
						required
						bind:value={form.name}
						class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
						placeholder="survival"
					/>
				</div>
				<div>
					<label class="mb-1 block text-sm font-medium" for="loader">구동기</label>
					<div class="relative">
						<select
							id="loader"
							bind:value={form.loader}
							onchange={onLoaderChange}
							class="border-input bg-background w-full appearance-none rounded-md border py-2 pl-3 pr-8 text-sm"
						>
							<option value="vanilla">Vanilla</option>
							<option value="paper">Paper</option>
							<option value="purpur">Purpur</option>
							<option value="folia">Folia</option>
							<option value="pufferfish">Pufferfish</option>
							<option value="leaf">Leaf</option>
							<option value="fabric">Fabric</option>
							<option value="neoforge">NeoForge</option>
							<option value="custom">커스텀 (직접 업로드)</option>
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
				{#if form.loader === 'custom'}
					<div>
						<label class="mb-1 block text-sm font-medium" for="custom-loader-name">구동기 이름</label>
						<input
							id="custom-loader-name"
							type="text"
							required
							bind:value={customLoaderName}
							placeholder="예: MyModpackServer"
							class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
						/>
						<p class="text-muted-foreground mt-1 text-xs">
							목록에 없는 구동기의 jar 파일을 직접 올려 서버를 만듭니다. 자동 다운로드/버전 목록
							조회/플러그인·모드 검색은 지원되지 않고, 파일 탭에서 직접 관리해야 합니다.
						</p>
					</div>
				{/if}
				{#if proxyCapableLoaders.includes(form.loader)}
					<label class="flex items-start gap-2 text-sm">
						<input type="checkbox" bind:checked={form.expose_independently} class="mt-1" />
						<span>
							독립적으로 외부에 노출 (기본은 항상 켜져 있는 Velocity 프록시 뒤에 자동 등록되며,
							게임 포트는 내부용으로만 쓰입니다)
						</span>
					</label>
					{#if form.loader === 'fabric' || form.loader === 'neoforge'}
						<p class="text-muted-foreground -mt-2 text-xs">
							⚠ 엔티티·블록 상태 등 바닐라 패킷 구조 자체를 변형하는 모드(예: Create)는
							Velocity와 호환되지 않아 접속이 끊길 수 있습니다. 이런 모드를 쓸 계획이면 독립
							노출을 체크하세요.
						</p>
					{/if}
				{:else}
					<p class="text-muted-foreground text-xs">
						이 구동기는 프록시의 모던 포워딩을 지원하지 않아 항상 독립적으로 노출됩니다.
					</p>
				{/if}
				<div>
					<label class="mb-1 block text-sm font-medium" for="mc_version">마인크래프트 버전</label>
					{#if form.loader === 'custom'}
						<input
							id="mc_version"
							type="text"
							required
							bind:value={form.mc_version}
							placeholder="예: 1.20.1 (Java 버전 자동 선택에 쓰입니다)"
							class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
						/>
					{:else if mcVersionsError}
						<p class="text-destructive text-xs">
							버전 목록을 불러오지 못했습니다: {mcVersionsError}
						</p>
					{:else if availableVersionIds.length === 0}
						<p class="text-muted-foreground text-xs">버전 목록 불러오는 중...</p>
					{:else}
						<div class="relative">
							<select
								id="mc_version"
								required
								bind:value={form.mc_version}
								class="border-input bg-background w-full appearance-none rounded-md border py-2 pl-3 pr-8 text-sm"
							>
								{#each availableVersionIds as id}
									<option value={id}>{id}</option>
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
					{/if}
				</div>
				{#if buildListerLoaders.includes(form.loader) && buildOptions.length > 0}
					<div>
						<label class="mb-1 block text-sm font-medium" for="loader_version">빌드 (선택사항)</label>
						<div class="relative">
							<select
								id="loader_version"
								bind:value={form.loader_version}
								class="border-input bg-background w-full appearance-none rounded-md border py-2 pl-3 pr-8 text-sm"
							>
								<option value="">최신</option>
								{#each buildOptions as build (build.id)}
									<option value={build.id}>
										{build.id}{build.channel ? ` (${build.channel})` : ''}
									</option>
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
				{:else if buildListerLoaders.includes(form.loader) && buildsError}
					<p class="text-muted-foreground text-xs">빌드 목록을 불러오지 못했습니다: {buildsError}</p>
				{/if}
				{#if form.loader === 'custom'}
					<div>
						<label class="mb-1 block text-sm font-medium" for="custom-jar">구동기 jar 파일</label>
						<input
							id="custom-jar"
							type="file"
							required
							accept=".jar"
							onchange={onCustomJarFileChange}
							class="text-muted-foreground file:border-border file:bg-background file:text-foreground file:mr-2 file:rounded-md file:border file:px-3 file:py-1.5 file:text-xs file:font-medium file:cursor-pointer w-full text-xs"
						/>
					</div>
				{/if}
				<div>
					<label class="mb-1 block text-sm font-medium" for="create-memory">
						최대 메모리 ({form.memory_gb}GB / 최대 {maxMemoryGB}GB{#if ramBoundaryGB < maxMemoryGB}<span
								class="text-yellow-500"> · 스왑 {maxMemoryGB - ramBoundaryGB}GB 포함</span
							>{/if})
					</label>
					<MemorySlider
						id="create-memory"
						bind:value={form.memory_gb}
						maxGB={maxMemoryGB}
						{ramBoundaryGB}
					/>
				</div>
				<div>
					<label class="mb-1 block text-sm font-medium" for="create-cpu">
						CPU 할당량 ({form.cpu_quota_percent > 0 ? `${form.cpu_quota_percent}%` : '무제한'})
					</label>
					<input
						id="create-cpu"
						type="range"
						min="0"
						max="100"
						step="5"
						bind:value={form.cpu_quota_percent}
						class="w-full"
					/>
				</div>
				<div>
					<label class="mb-1 block text-sm font-medium" for="world-file"
						>월드 데이터 가져오기 (선택, tar.gz)</label
					>
					<input
						id="world-file"
						type="file"
						accept=".gz,.tar.gz"
						onchange={onWorldFileChange}
						class="text-muted-foreground file:border-border file:bg-background file:text-foreground file:mr-2 file:rounded-md file:border file:px-3 file:py-1.5 file:text-xs file:font-medium file:cursor-pointer w-full text-sm"
					/>
					{#if worldFile}
						<label class="mt-1 flex items-center gap-2 text-xs">
							<input type="checkbox" bind:checked={worldFileForce} />
							<span>업로드한 월드가 이 인스턴스보다 최신 버전이어도 강제로 적용</span>
						</label>
					{/if}
				</div>
				<label class="flex items-start gap-2 text-sm">
					<input type="checkbox" required bind:checked={form.accept_eula} class="mt-1" />
					<span>
						마인크래프트 <a
							class="underline"
							href="https://www.minecraft.net/eula"
							target="_blank"
							rel="noreferrer">EULA</a
						>에 동의합니다.
					</span>
				</label>
				{#if createError}
					<p class="text-destructive text-sm">{createError}</p>
				{/if}
				<button
					type="submit"
					disabled={creating}
					class="bg-primary text-primary-foreground w-full rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
				>
					{creating ? '생성 중... (jar 다운로드 포함)' : '생성'}
				</button>
			</form>
		</div>
	</div>
{/if}

{#if showAccountModal}
	<!-- 비밀번호 변경과 2단계 인증은 둘 다 계정 보안에 관한 설정이라 하나의
		모달 안에 두 섹션으로 묶었다 (예전엔 헤더에 버튼이 따로 있었음). -->
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-8"
		onclick={() => (showAccountModal = false)}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="border-border bg-card max-h-[85vh] w-full max-w-sm overflow-y-auto rounded-lg border p-4 shadow-lg"
			onclick={(e) => e.stopPropagation()}
		>
			<div class="mb-3 flex items-center justify-between">
				<h2 class="font-medium">계정 설정</h2>
				<button
					type="button"
					class="text-muted-foreground text-sm"
					onclick={() => (showAccountModal = false)}>&times;</button
				>
			</div>

			<h3 class="text-sm font-medium">비밀번호 변경</h3>
			<form class="mt-3 space-y-4" onsubmit={changePassword}>
				<div>
					<label class="mb-1 block text-sm font-medium" for="pw-username">아이디</label>
					<input
						id="pw-username"
						required
						autocomplete="username"
						bind:value={username}
						class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
					/>
				</div>
				<div>
					<label class="mb-1 block text-sm font-medium" for="pw-current">현재 비밀번호</label>
					<input
						id="pw-current"
						type="password"
						required
						autocomplete="current-password"
						bind:value={currentPassword}
						class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
					/>
				</div>
				<div>
					<label class="mb-1 block text-sm font-medium" for="pw-new">새 비밀번호</label>
					<input
						id="pw-new"
						type="password"
						required
						minlength="8"
						autocomplete="new-password"
						bind:value={newPassword}
						class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
					/>
					<p class="text-muted-foreground mt-1 text-xs">8자 이상</p>
				</div>
				<div>
					<label class="mb-1 block text-sm font-medium" for="pw-new-confirm"
						>새 비밀번호 확인</label
					>
					<input
						id="pw-new-confirm"
						type="password"
						required
						autocomplete="new-password"
						bind:value={newPasswordConfirm}
						class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
					/>
				</div>
				{#if passwordError}
					<p class="text-destructive text-sm">{passwordError}</p>
				{/if}
				{#if passwordChanged}
					<p class="text-sm text-green-500">비밀번호가 변경되었습니다.</p>
				{/if}
				<button
					type="submit"
					disabled={changingPassword}
					class="bg-primary text-primary-foreground w-full rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
				>
					{changingPassword ? '변경 중...' : '변경'}
				</button>
			</form>

			<hr class="border-border my-4" />

			<!-- FR-38/39: 2단계 인증 등록 -- QR 스캔 후 코드 한 번 검증해야 실제로
				켜진다(handleTOTPVerify). 이미 켜져 있으면 재설정 대신 안내만 표시
				(handleTOTPSetup이 409를 반환하므로 백엔드와 일관됨). -->
			<h3 class="text-sm font-medium">2단계 인증</h3>
			<div class="mt-3">
				{#if totpEnabled && !totpBackupCodes}
					<p class="text-muted-foreground text-sm">
						이미 설정되어 있습니다. 인증 앱을 분실했다면 로그인 시 백업 코드를 대신 사용하세요.
					</p>
				{:else if totpBackupCodes}
					<p class="text-sm">
						설정 완료됐습니다. 아래 백업 코드를 안전한 곳에 저장하세요 -- 다시 볼 수 없습니다.
					</p>
					<div class="border-border bg-background mt-2 grid grid-cols-2 gap-1 rounded-md border p-3">
						{#each totpBackupCodes as code (code)}
							<code class="text-xs">{code}</code>
						{/each}
					</div>
				{:else if startingTOTPSetup}
					<p class="text-muted-foreground text-sm">준비 중...</p>
				{:else}
					<p class="text-muted-foreground text-sm">
						인증 앱(Google Authenticator, Authy 등)으로 아래 QR 코드를 스캔하세요.
					</p>
					{#if totpQrCode}
						<img src={totpQrCode} alt="2FA QR 코드" class="mx-auto mt-3 h-48 w-48" />
					{/if}
					{#if totpSecret}
						<p class="text-muted-foreground mt-2 text-center text-xs">
							QR을 스캔할 수 없다면 직접 입력: <code class="break-all">{totpSecret}</code>
						</p>
					{/if}
					<form class="mt-4 space-y-4" onsubmit={submitTOTPVerify}>
						<div>
							<label class="mb-1 block text-sm font-medium" for="totp-verify-code"
								>인증 앱의 6자리 코드</label
							>
							<input
								id="totp-verify-code"
								type="text"
								inputmode="numeric"
								required
								bind:value={totpVerifyCode}
								class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
							/>
						</div>
						{#if totpError}
							<p class="text-destructive text-sm">{totpError}</p>
						{/if}
						<button
							type="submit"
							disabled={verifyingTOTP}
							class="bg-primary text-primary-foreground w-full rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
						>
							{verifyingTOTP ? '확인 중...' : '확인 후 활성화'}
						</button>
					</form>
				{/if}
			</div>
		</div>
	</div>
{/if}

{#if showWANWarningModal}
	<!-- FR-34: 외부 접속을 켜기 전 경고 + 강력한 비밀번호 유도. 여기서 취소하면
		checked 값이 여전히 networkSettings.wan_enabled(아직 false)를 그대로
		반영하므로 체크박스는 자동으로 꺼진 상태로 되돌아간다. -->
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
		onclick={() => (showWANWarningModal = false)}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="border-border bg-card w-full max-w-sm rounded-lg border p-4 shadow-lg"
			onclick={(e) => e.stopPropagation()}
		>
			<h2 class="font-medium text-destructive">⚠ 외부 접속을 켜려고 합니다</h2>
			<p class="text-muted-foreground mt-2 text-sm">
				관리 웹 UI와 게임 포트가 인터넷 전체에 노출됩니다. 누구나 이 주소로 로그인을 시도할 수
				있으니, 계정 비밀번호가 충분히 강력한지(다른 곳에서 재사용하지 않는 긴 무작위 비밀번호)
				먼저 확인하세요.
			</p>
			<button
				type="button"
				class="border-border mt-3 w-full rounded-md border px-4 py-2 text-sm font-medium"
				onclick={() => {
					showWANWarningModal = false;
					openAccountModal();
				}}
			>
				비밀번호 변경하러 가기
			</button>
			<div class="mt-3 flex gap-2">
				<button
					type="button"
					class="border-border flex-1 rounded-md border px-4 py-2 text-sm font-medium"
					onclick={() => (showWANWarningModal = false)}
				>
					취소
				</button>
				<button
					type="button"
					class="bg-primary text-primary-foreground flex-1 rounded-md px-4 py-2 text-sm font-medium"
					onclick={confirmWANEnable}
				>
					이해했습니다, 계속
				</button>
			</div>
		</div>
	</div>
{/if}

{#if showMemoryConflictModal}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-8"
		onclick={() => (showMemoryConflictModal = false)}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="bg-card border-border w-full max-w-md rounded-lg border p-4 shadow-lg"
			onclick={(e) => e.stopPropagation()}
		>
			<h2 class="font-medium">메모리 할당 조정 필요</h2>
			<p class="text-muted-foreground mt-1 text-xs">
				실행하려는 서버들의 메모리 할당 합이 {ramBoundaryGB < conflictMaxGB
					? '물리 RAM + 스왑 여유분'
					: '라즈베리파이의 전체 메모리'}을(를) 초과합니다. 아래에서 조정한 뒤 시작할 수 있습니다.
			</p>

			<div class="mt-3 space-y-3">
				{#each conflictItems as item (item.id)}
					<div>
						<label class="mb-1 flex items-center justify-between text-xs" for="conflict-{item.id}">
							<span>
								{item.name}
								{#if item.isTarget}<span class="text-muted-foreground">(시작 예정)</span>
								{:else if item.isRunning}<span class="text-muted-foreground"
										>(실행 중 -- 변경 시 자동으로 재시작됩니다)</span
									>{/if}
							</span>
							<span>{item.memoryGB}GB</span>
						</label>
						<MemorySlider
							id="conflict-{item.id}"
							bind:value={item.memoryGB}
							maxGB={conflictMaxGB}
							{ramBoundaryGB}
						/>
					</div>
				{/each}
			</div>

			<p class="mt-3 text-sm font-medium {conflictOverBudget ? 'text-destructive' : ''}">
				합계 {conflictTotalGB}GB / 전체 {conflictMaxGB}GB
			</p>
			{#if conflictError}
				<p class="text-destructive mt-2 text-xs">{conflictError}</p>
			{/if}

			<div class="mt-3 flex gap-2">
				<button
					class="bg-primary text-primary-foreground rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50"
					disabled={conflictOverBudget || applyingConflict}
					onclick={applyConflictAndStart}
				>
					{applyingConflict ? '적용 중...' : '적용 후 시작'}
				</button>
				<button
					class="border-border rounded-md border px-3 py-1.5 text-sm"
					onclick={() => (showMemoryConflictModal = false)}>취소</button
				>
			</div>
		</div>
	</div>
{/if}

<ConfirmDialog bind:open={confirmOpen} message={confirmMessage} onconfirm={confirmAction} />

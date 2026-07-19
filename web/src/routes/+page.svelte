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
	import ConfirmDialog from '$lib/ConfirmDialog.svelte';
	import AccountModal from '$lib/AccountModal.svelte';
	import VelocityProxyCard from '$lib/VelocityProxyCard.svelte';
	import ExternalAccessCard from '$lib/ExternalAccessCard.svelte';
	import SwapCard from '$lib/SwapCard.svelte';
	import DomainConnectionCard from '$lib/DomainConnectionCard.svelte';
	import ResourcePanel from '$lib/ResourcePanel.svelte';
	import WANWarningModal from '$lib/WANWarningModal.svelte';
	import MemoryConflictModal from '$lib/MemoryConflictModal.svelte';
	import CreateInstanceModal from '$lib/CreateInstanceModal.svelte';
	import UpdateAvailableModal from '$lib/UpdateAvailableModal.svelte';
	import CloudflareTutorialModal from '$lib/CloudflareTutorialModal.svelte';
	import TourOverlay, { type TourStep } from '$lib/TourOverlay.svelte';
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

	// 비밀번호 변경 + 2단계 인증 모달 (AccountModal.svelte) -- 이 페이지는
	// 언제 열지(showAccountModal)와 두 컴포넌트 헤더가 공유하는 신원 정보
	// (username, totpEnabled)만 들고, 나머지 폼 상태는 컴포넌트 내부 소관.
	let username = $state('');
	let showAccountModal = $state(false);
	let totpEnabled = $state(false);
	function openAccountModal() {
		showAccountModal = true;
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
	//
	// FR-38 also requires 2FA before the backend will actually turn WAN on
	// (see handleSetNetworkSettings) -- checking that here too, instead of
	// just letting the toggle fail with a 412 the operator has to notice
	// buried under the checkbox, sends them straight to where they can fix
	// it (confirmed as a real point of confusion: an operator without 2FA
	// set up couldn't tell why the toggle wasn't sticking after a refresh).
	function onWANToggleChange(enabled: boolean) {
		if (enabled) {
			if (!totpEnabled) {
				networkError = '외부 접속을 켜려면 먼저 2단계 인증을 설정해야 합니다.';
				openAccountModal();
				return;
			}
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

	// 가상 메모리(디스크 스왑파일) -- 라즈베리파이 OS의 zram(RAM 압축 스왑)과는
	// 별개로 동작하는, CraftDeck 전용 디스크 기반 스왑. RAM+zram으로도 부족할
	// 때를 대비한 추가 여유분 성격이라 커널이 항상 실제 RAM/zram을 먼저 쓰고
	// 남을 때만 사용한다.
	let swapInfo = $state<SwapInfo | null>(null);
	// Set only when the GET itself fails (network/auth hiccup) -- kept
	// distinct from swapInfo.supported === false (genuinely SD-card/eMMC
	// storage) so the card can tell the two apart instead of just
	// disappearing either way, which made a transient fetch failure look
	// identical to "this feature doesn't exist on this hardware."
	let swapFetchError = $state('');
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
		form = {
			name: '',
			loader: 'vanilla',
			mc_version: '',
			loader_version: '',
			memory_gb: 2,
			cpu_quota_percent: 0,
			accept_eula: false,
			expose_independently: false
		};
		customLoaderName = '';
		worldFile = null;
		worldFileForce = false;
		createError = '';
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

	// craftdeckd 자신의 새 버전 안내 -- 세션당(페이지 로드당) 한 번만 확인
	// 하면 충분해서, 인스턴스 목록/리소스처럼 자주 폴링하진 않는다.
	let showUpdateModal = $state(false);
	let craftdeckCurrentVersion = $state('');
	let craftdeckLatestVersion = $state('');
	async function checkCraftdeckVersion() {
		try {
			const v = await api.systemVersion();
			craftdeckCurrentVersion = v.current_version;
			craftdeckLatestVersion = v.latest_version ?? '';
			if (v.update_available) showUpdateModal = true;
		} catch {
			// non-critical -- just skip the notice this time
		}
	}

	// 처음 접속한 사용자에게 한 번만 자동으로 보여주는 스포트라이트 투어 --
	// "다시 보기"는 언제든 헤더의 튜토리얼 버튼으로 가능하니, 서버에 상태를
	// 두지 않고 이 브라우저에서 이미 봤는지만 localStorage로 기억한다.
	const TOUR_SEEN_KEY = 'craftdeck-tour-seen';
	let showTour = $state(false);
	let showCloudflareGuide = $state(false);

	const tourSteps: TourStep[] = [
		{
			selector: '#tour-create-server',
			title: '서버 만들기',
			body: '여기서 새 마인크래프트 서버를 몇 번의 클릭으로 만들 수 있어요.',
			beforeShow: () => (activeTab = 'instances')
		},
		{
			selector: 'a[href^="/instances/"]',
			title: '실시간 콘솔',
			body: '서버 로그를 실시간으로 보고 명령어를 바로 입력할 수 있어요. 서버를 하나 만들면 이 버튼으로 들어갈 수 있어요.',
			beforeShow: () => (activeTab = 'instances')
		},
		{
			selector: '#tour-settings-tab',
			title: '전역 설정',
			body: '외부 접속, 도메인 연결, 스왑처럼 서버 하나에 속하지 않는 설정은 여기 모여 있어요.',
			beforeShow: () => (activeTab = 'instances')
		},
		{
			selector: '#tour-external-access',
			title: '외부 접속',
			body: '친구를 초대해서 같이 플레이하려면 여기서 외부 접속을 켜세요.',
			beforeShow: () => (activeTab = 'settings')
		},
		{
			selector: '#tour-domain-card',
			title: '도메인 연결',
			body: '소유한 도메인이 있다면 연결해서 서브도메인으로 여러 서버를 묶을 수 있어요. Cloudflare를 쓴다면 가이드 버튼으로 바로 따라 할 수 있어요.',
			beforeShow: () => (activeTab = 'settings')
		},
		{
			selector: '#tour-account-button',
			title: '계정 설정',
			body: '2단계 인증이나 비밀번호는 여기서 관리해요. 이 투어는 언제든 "튜토리얼" 버튼으로 다시 볼 수 있어요.',
			placement: 'left'
		}
	];

	function startTour() {
		showTour = true;
	}
	function markTourSeen() {
		localStorage.setItem(TOUR_SEEN_KEY, '1');
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
		checkCraftdeckVersion();
		api.authStatus().then((s) => {
			username = s.username;
			isLoggedIn = s.authenticated;
			totpEnabled = s.totp_enabled;
			if (s.authenticated && !localStorage.getItem(TOUR_SEEN_KEY)) {
				// 레이아웃이 자리 잡을 시간을 준 다음 시작 -- 너무 빨리 켜면
				// 카드 위치가 아직 안 잡혀서 스포트라이트가 엉뚱한 곳을 짚는다.
				setTimeout(startTour, 600);
			}
		});
		pollHandle = setInterval(refresh, 2000);
		resourcePollHandle = setInterval(refreshResources, 2000);
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
				id="tour-create-server"
				class="bg-primary text-primary-foreground rounded-md px-4 py-2 text-sm font-medium"
				onclick={openCreateForm}
			>
				+ 서버 만들기
			</button>
			{#if isLoggedIn}
				<button
					class="border-border rounded-md border px-4 py-2 text-sm font-medium"
					onclick={startTour}
				>
					튜토리얼
				</button>
				<button
					id="tour-account-button"
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
			id="tour-settings-tab"
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
				<VelocityProxyCard
					{proxyStatus}
					upgrading={proxyUpgrading}
					upgradeError={proxyUpgradeError}
					onUpgrade={upgradeProxy}
				/>
			{/if}

			<ExternalAccessCard
				{networkSettings}
				{networkToggling}
				{networkError}
				onToggle={onWANToggleChange}
				{portMappings}
				{deletingMappingId}
				onDeleteMapping={deletePortMapping}
				{instances}
			/>

			{#if swapInfo === null || swapInfo.supported}
				<SwapCard
					{swapInfo}
					{swapFetchError}
					bind:swapSizeInput
					{swapSaving}
					{swapError}
					onSave={saveSwap}
					onDisable={disableSwap}
				/>
			{/if}

			<DomainConnectionCard
				{domainConfig}
				bind:domainForm
				{domainSaving}
				{domainError}
				{domainTokenRequired}
				onKindChange={onDomainKindChange}
				onSave={saveDomainSettings}
				onUnregister={unregisterDomain}
				onOpenCloudflareGuide={() => (showCloudflareGuide = true)}
			/>
			</div>
			{/if}
		</div>

		<!-- 라즈베리파이 리소스는 실행 중인 인스턴스/전역 설정 중 무엇을 보고
			있든 운영자가 항상 확인하고 싶어할 라이브 상태 값이라, 탭 전환과
			무관하게 별도 사이드바에 고정해서 보여준다. -->
		<div class="lg:col-span-1 lg:min-h-0 lg:overflow-y-auto lg:pr-3">
			<ResourcePanel {resources} {resourceError} {swapInfo} />
		</div>
	</div>
</main>

<CreateInstanceModal
	bind:open={showCreateForm}
	bind:form
	bind:customLoaderName
	bind:worldFile
	bind:worldFileForce
	{proxyCapableLoaders}
	{buildListerLoaders}
	{availableVersionIds}
	{mcVersionsError}
	{buildOptions}
	{buildsError}
	{maxMemoryGB}
	{ramBoundaryGB}
	{createError}
	{creating}
	{onLoaderChange}
	{onCustomJarFileChange}
	{onWorldFileChange}
	onSubmit={createInstance}
/>

<AccountModal bind:open={showAccountModal} bind:username bind:totpEnabled />

<CloudflareTutorialModal
	bind:open={showCloudflareGuide}
	bind:domainForm
	{domainSaving}
	{domainError}
	onSave={saveDomainSettings}
/>

<TourOverlay steps={tourSteps} bind:open={showTour} onFinish={markTourSeen} />

<WANWarningModal
	bind:open={showWANWarningModal}
	onGoToAccountModal={openAccountModal}
	onConfirm={confirmWANEnable}
/>

<MemoryConflictModal
	bind:open={showMemoryConflictModal}
	bind:items={conflictItems}
	maxGB={conflictMaxGB}
	totalGB={conflictTotalGB}
	overBudget={conflictOverBudget}
	{ramBoundaryGB}
	error={conflictError}
	applying={applyingConflict}
	onApply={applyConflictAndStart}
/>

<ConfirmDialog bind:open={confirmOpen} message={confirmMessage} onconfirm={confirmAction} />

<UpdateAvailableModal
	bind:open={showUpdateModal}
	currentVersion={craftdeckCurrentVersion}
	latestVersion={craftdeckLatestVersion}
/>

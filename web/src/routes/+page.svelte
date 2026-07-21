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
		type SwapInfo,
		type HardwareInfo,
		type BenchmarkStatus,
		type UpdateSettings,
		OVERCLOCK_PRESETS
	} from '$lib/api';
	import ConfirmDialog from '$lib/ConfirmDialog.svelte';
	import AccountSettings from '$lib/AccountSettings.svelte';
	import MiscSettings from '$lib/MiscSettings.svelte';
	import VelocityProxyCard from '$lib/VelocityProxyCard.svelte';
	import ExternalAccessCard from '$lib/ExternalAccessCard.svelte';
	import SwapCard from '$lib/SwapCard.svelte';
	import OverclockCard from '$lib/OverclockCard.svelte';
	import UpdateSettingsCard from '$lib/UpdateSettingsCard.svelte';
	import DomainConnectionCard from '$lib/DomainConnectionCard.svelte';
	import ResourcePanel from '$lib/ResourcePanel.svelte';
	import WANWarningModal from '$lib/WANWarningModal.svelte';
	import MemoryConflictModal from '$lib/MemoryConflictModal.svelte';
	import CreateInstanceModal from '$lib/CreateInstanceModal.svelte';
	import UpdateAvailableModal from '$lib/UpdateAvailableModal.svelte';
	import CloudflareTutorialModal from '$lib/CloudflareTutorialModal.svelte';
	import TourOverlay, { type TourStep } from '$lib/TourOverlay.svelte';
	import { onDestroy, onMount } from 'svelte';
	import { page } from '$app/stores';
	import { replaceState } from '$app/navigation';
	import { t } from '$lib/i18n';

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

	// 비밀번호 변경 + 2단계 인증 (AccountSettings.svelte, 설정 탭의 "계정"
	// 서브탭에 인라인으로 렌더링됨) -- 이 페이지는 두 컴포넌트가 공유하는
	// 신원 정보(username, totpEnabled)만 들고, 나머지 폼 상태는 컴포넌트
	// 내부 소관.
	let username = $state('');
	let totpEnabled = $state(false);
	// 2FA 미설정 등으로 계정 설정으로 안내해야 할 때 쓰는 공통 진입점 --
	// 세션 유무와 무관하게 항상 접근 가능해야 해서(예전엔 헤더의 "계정
	// 설정" 버튼이 로그인 세션이 있을 때만 보여서 LAN 접속 중엔 아예 막혀
	// 있었다) 설정 탭 자체로 이동시킨다.
	function openAccountSettings() {
		setActiveTab('settings');
		setSettingsSubTab('account');
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
				networkError = $t('mainPage.network.wanRequiresTotp');
				openAccountSettings();
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
			swapError = $t('mainPage.swap.invalidSize');
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
		askConfirm($t('mainPage.swap.confirmDisable'), doDisableSwap);
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

	// Active Cooler 감지 + 오버클럭 (internal/hardware) -- cooler_detected가
	// false면 카드 안의 조작부는 숨기고 "다시 감지"만 노출한다.
	let hardwareInfo = $state<HardwareInfo | null>(null);
	let hardwareFetchError = $state('');
	let redetectingCooler = $state(false);
	let overclockForm = $state({ preset: '__none__', armFreq: '2600', overVoltageDeltaUV: '30000' });
	let overclockSaving = $state(false);
	let overclockError = $state('');
	let overclockRebooting = $state(false);
	let benchmarkStatus = $state<BenchmarkStatus | null>(null);
	let benchmarkStarting = $state(false);
	let benchmarkPollHandle: ReturnType<typeof setInterval> | undefined;
	let overclockRebootPollHandle: ReturnType<typeof setInterval> | undefined;

	async function refreshHardware() {
		try {
			hardwareInfo = await api.getHardware();
			hardwareFetchError = '';
			if (hardwareInfo.overclock_enabled && hardwareInfo.overclock_preset) {
				overclockForm.preset = hardwareInfo.overclock_preset;
			}
			if (hardwareInfo.overclock_arm_freq) overclockForm.armFreq = String(hardwareInfo.overclock_arm_freq);
			if (hardwareInfo.overclock_over_voltage_delta !== undefined)
				overclockForm.overVoltageDeltaUV = String(hardwareInfo.overclock_over_voltage_delta);
		} catch (err) {
			hardwareFetchError = err instanceof Error ? err.message : String(err);
		}
	}

	async function redetectCooler() {
		redetectingCooler = true;
		try {
			hardwareInfo = await api.redetectCooler();
		} catch (err) {
			hardwareFetchError = err instanceof Error ? err.message : String(err);
		} finally {
			redetectingCooler = false;
		}
	}

	async function applyOverclock() {
		overclockSaving = true;
		overclockError = '';
		try {
			const enabled = overclockForm.preset !== '__none__';
			let armFreq = Number(overclockForm.armFreq);
			let overVoltageDeltaUV = Number(overclockForm.overVoltageDeltaUV);
			const preset = OVERCLOCK_PRESETS.find((p) => p.name === overclockForm.preset);
			if (preset) {
				armFreq = preset.arm_freq_mhz;
				overVoltageDeltaUV = preset.over_voltage_delta_uv;
			}
			const presetName = overclockForm.preset === 'custom' || overclockForm.preset === '__none__' ? '' : overclockForm.preset;
			hardwareInfo = await api.setOverclock(enabled, presetName, armFreq, overVoltageDeltaUV);
		} catch (err) {
			overclockError = err instanceof Error ? err.message : String(err);
		} finally {
			overclockSaving = false;
		}
	}

	// 서버가 실행 중인 상태로 재부팅하면 정상 종료 절차 없이 강제로
	// 죽는 셈이라, 재부팅이 필요한 모든 경로(적용/되돌리기)에서 공통으로
	// 거치는 안전장치: 실행 중인 인스턴스가 있으면 경고 후 동의를 받고,
	// 각각에 종료 명령을 보내 실제로 멈출 때까지 기다린 다음에야 진행한다.
	async function waitForInstancesStopped(ids: string[], timeoutMs = 90_000) {
		const deadline = Date.now() + timeoutMs;
		while (Date.now() < deadline) {
			const current = await api.listInstances();
			const stillRunning = current.filter((i) => ids.includes(i.id) && i.status !== 'stopped');
			if (stillRunning.length === 0) return;
			await new Promise((resolve) => setTimeout(resolve, 2000));
		}
		throw new Error($t('mainPage.overclock.waitTimeout'));
	}

	// Velocity 프록시(kind: 'proxy')는 사용자가 직접 올린 마인크래프트
	// 서버가 아니라 craftdeckd가 부가적으로 관리하는 내부 라우팅 프로세스라,
	// 경고 모달의 "실행 중인 서버" 목록에는 넣지 않고 재부팅 직전에
	// 조용히 내려버린다 -- 사용자 눈엔 서버만 종료 대상으로 보이면 된다.
	async function stopRunningProxiesSilently() {
		const runningProxies = instances.filter((i) => i.status === 'running' && i.kind === 'proxy');
		if (runningProxies.length === 0) return;
		await Promise.all(runningProxies.map((i) => api.stopInstance(i.id)));
		await waitForInstancesStopped(runningProxies.map((i) => i.id));
	}

	async function applyAndRebootNow() {
		try {
			await stopRunningProxiesSilently();
		} catch (err) {
			overclockError = err instanceof Error ? err.message : String(err);
			overclockSaving = false;
			return;
		}
		await applyOverclock();
		if (overclockError) {
			overclockSaving = false;
			return;
		}
		await rebootForOverclock();
	}

	// "적용"과 "재부팅해서 적용"을 하나로 합친 진입점 -- 실행 중인 서버
	// 인스턴스가 있으면 그대로 재부팅하지 않고 먼저 경고 모달로 동의를
	// 구한다(프록시는 별도로, 조용히 처리되므로 여기 목록/판단에서 제외).
	function requestOverclockReboot() {
		const running = instances.filter((i) => i.status === 'running' && i.kind !== 'proxy');
		if (running.length === 0) {
			applyAndRebootNow();
			return;
		}
		const names = running.map((i) => i.name).join(', ');
		askConfirm(
			$t('mainPage.overclock.confirmRebootStop', { names }),
			async () => {
				overclockSaving = true;
				overclockError = '';
				try {
					await Promise.all(running.map((i) => api.stopInstance(i.id)));
					await waitForInstancesStopped(running.map((i) => i.id));
				} catch (err) {
					overclockError = err instanceof Error ? err.message : String(err);
					overclockSaving = false;
					return;
				}
				await applyAndRebootNow();
			}
		);
	}

	// 벤치마크가 FAIL을 감지한 시점엔 이미 그 불안정한 오버클럭이 부팅 때
	// 적용돼서 지금 이 순간 실행 중인 상태라, config.txt만 안전값으로
	// 되돌려서는 부족하다 -- 재부팅까지 같이 해야 실제로 안전해진다.
	function revertOverclock() {
		overclockForm.preset = '__none__';
		requestOverclockReboot();
	}

	// 서비스 재시작(자기 프로세스만 죽었다 다시 뜸)과 달리 재부팅은 기기
	// 전체가 몇십 초간 완전히 내려갔다 올라오므로, 자가 업데이트 폴링과
	// 같은 모양이되 훨씬 긴 데드라인을 쓴다. 트리거 직후의 요청은 아직
	// 재부팅이 실제로 시작되기 전이라 우연히 성공할 수 있어, 한 번이라도
	// 실패(= 실제로 내려감)를 관찰한 뒤에 온 성공만 "복귀"로 인정한다.
	async function rebootForOverclock() {
		overclockRebooting = true;
		try {
			await api.rebootForOverclock();
		} catch (err) {
			overclockError = err instanceof Error ? err.message : String(err);
			overclockRebooting = false;
			overclockSaving = false;
			return;
		}
		overclockSaving = false;
		pollUntilRebooted();
	}

	function pollUntilRebooted() {
		clearInterval(overclockRebootPollHandle);
		const deadline = Date.now() + 240_000;
		let sawDown = false;
		overclockRebootPollHandle = setInterval(async () => {
			if (Date.now() > deadline) {
				clearInterval(overclockRebootPollHandle);
				overclockRebooting = false;
				overclockError = $t('mainPage.overclock.rebootNoResponse');
				return;
			}
			try {
				await api.systemVersion();
				if (sawDown) {
					clearInterval(overclockRebootPollHandle);
					window.location.reload();
				}
			} catch {
				sawDown = true;
			}
		}, 3000);
	}

	// 안정성 테스트는 전체 코어에 90초 넘게 최대 부하를 걸기 때문에, 그동안
	// 실행 중인 서버가 있으면 플레이어 경험이 나빠지고 측정 자체도 왜곡될
	// 수 있다. 재부팅 흐름(requestOverclockReboot)과 같은 패턴으로 실행
	// 중인 서버가 있으면 먼저 동의를 구해 종료한 뒤 진행하고, 이때 종료한
	// 인스턴스만 기억해뒀다가 테스트가 끝나면 자동으로 다시 시작한다.
	let benchmarkStoppedInstanceIds = $state<string[]>([]);

	function requestStartBenchmark() {
		const running = instances.filter((i) => i.status === 'running' && i.kind !== 'proxy');
		if (running.length === 0) {
			doStartBenchmark([]);
			return;
		}
		const names = running.map((i) => i.name).join(', ');
		askConfirm(
			$t('mainPage.benchmark.confirmStop', { names }),
			async () => {
				benchmarkStarting = true;
				overclockError = '';
				try {
					await Promise.all(running.map((i) => api.stopInstance(i.id)));
					await waitForInstancesStopped(running.map((i) => i.id));
				} catch (err) {
					overclockError = err instanceof Error ? err.message : String(err);
					benchmarkStarting = false;
					return;
				}
				await doStartBenchmark(running.map((i) => i.id));
			}
		);
	}

	async function doStartBenchmark(stoppedInstanceIds: string[]) {
		benchmarkStarting = true;
		try {
			await api.startBenchmark();
			benchmarkStoppedInstanceIds = stoppedInstanceIds;
			pollBenchmarkStatus();
		} catch (err) {
			overclockError = err instanceof Error ? err.message : String(err);
		} finally {
			benchmarkStarting = false;
		}
	}

	function pollBenchmarkStatus() {
		clearInterval(benchmarkPollHandle);
		benchmarkPollHandle = setInterval(async () => {
			try {
				benchmarkStatus = await api.getBenchmarkStatus();
				if (!benchmarkStatus.running) {
					clearInterval(benchmarkPollHandle);
					restartBenchmarkStoppedInstances();
				}
			} catch {
				clearInterval(benchmarkPollHandle);
			}
		}, 1000);
	}

	async function restartBenchmarkStoppedInstances() {
		const ids = benchmarkStoppedInstanceIds;
		benchmarkStoppedInstanceIds = [];
		await Promise.all(
			ids.map((instId) =>
				api.startInstance(instId).catch((err) => {
					overclockError = err instanceof Error ? err.message : String(err);
				})
			)
		);
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

	// 업데이트 설정 카드의 "지금 확인" 버튼 -- 확인 주기가 매일/매주/매달로
	// 늦춰져 있어도 이 버튼만은 항상 실제로 apt 저장소를 다시 조회한다
	// (?force=1, handleCraftdeckVersion 참고). 업데이트가 있으면 기존
	// UpdateAvailableModal이 뜨고, 없으면 이 자리에서 바로 안내한다.
	let checkingUpdateNow = $state(false);
	let checkNowMessage = $state('');
	async function checkForUpdateNow() {
		checkingUpdateNow = true;
		checkNowMessage = '';
		try {
			const v = await api.systemVersion(true);
			craftdeckCurrentVersion = v.current_version;
			craftdeckLatestVersion = v.latest_version ?? '';
			if (v.update_available) {
				showUpdateModal = true;
			} else {
				checkNowMessage = $t('mainPage.update.upToDate');
			}
		} catch (err) {
			checkNowMessage = err instanceof Error ? err.message : String(err);
		} finally {
			checkingUpdateNow = false;
		}
	}

	// 업데이트 채널(stable/beta/canary) + 확인 주기 설정. 채널을 바꾸면
	// 백엔드가 sources.list를 재작성하고 apt-get update까지 실행하므로,
	// 적용 직후 checkCraftdeckVersion을 다시 불러 최신 버전 표시를 갱신한다.
	let updateSettings = $state<UpdateSettings | null>(null);
	let updateSettingsFetchError = $state('');
	let updateSettingsForm = $state({ channel: 'stable', check_frequency: 'every_visit' });
	let updateSettingsSaving = $state(false);
	let updateSettingsError = $state('');

	async function refreshUpdateSettings() {
		try {
			updateSettings = await api.getUpdateSettings();
			updateSettingsForm = {
				channel: updateSettings.channel,
				check_frequency: updateSettings.check_frequency
			};
			updateSettingsFetchError = '';
		} catch (err) {
			updateSettingsFetchError = err instanceof Error ? err.message : String(err);
		}
	}

	async function saveUpdateSettings() {
		updateSettingsSaving = true;
		updateSettingsError = '';
		try {
			updateSettings = await api.setUpdateSettings({
				channel: updateSettingsForm.channel as UpdateSettings['channel'],
				check_frequency: updateSettingsForm.check_frequency as UpdateSettings['check_frequency']
			});
			await checkCraftdeckVersion();
		} catch (err) {
			updateSettingsError = err instanceof Error ? err.message : String(err);
		} finally {
			updateSettingsSaving = false;
		}
	}

	// 처음 접속한 사용자에게 한 번만 자동으로 보여주는 스포트라이트 투어 --
	// "다시 보기"는 언제든 계정 설정 모달 안의 버튼으로 가능하니, 서버에
	// 상태를 두지 않고 이 브라우저에서 이미 봤는지만 localStorage로 기억한다.
	const TOUR_SEEN_KEY = 'craftdeck-tour-seen';
	let showTour = $state(false);
	let showCloudflareGuide = $state(false);

	// $derived, not a plain const -- a plain const built from $t(...) once at
	// component init would freeze the tour's text in whatever locale was
	// active at page load, never picking up a later language switch (no
	// full reload happens when switching languages) since nothing here
	// would re-run it. $derived recomputes whenever $t itself changes.
	let tourSteps = $derived<TourStep[]>([
		{
			selector: '#tour-create-server',
			title: $t('mainPage.tour.createServer.title'),
			body: $t('mainPage.tour.createServer.body'),
			beforeShow: () => (activeTab = 'instances')
		},
		{
			selector: '#tour-console-sample, a[href^="/instances/"]',
			title: $t('mainPage.tour.console.title'),
			body: $t('mainPage.tour.console.body'),
			beforeShow: () => (activeTab = 'instances')
		},
		{
			selector: '#tour-settings-tab',
			title: $t('mainPage.tour.settings.title'),
			body: $t('mainPage.tour.settings.body'),
			beforeShow: () => (activeTab = 'instances')
		},
		{
			selector: '#tour-external-access',
			title: $t('mainPage.tour.externalAccess.title'),
			body: $t('mainPage.tour.externalAccess.body'),
			beforeShow: () => {
				activeTab = 'settings';
				settingsSubTab = 'network';
			}
		},
		{
			selector: '#tour-domain-card',
			title: $t('mainPage.tour.domain.title'),
			body: $t('mainPage.tour.domain.body'),
			beforeShow: () => {
				activeTab = 'settings';
				settingsSubTab = 'network';
			}
		},
		{
			selector: '#tour-misc-tab',
			title: $t('mainPage.tour.misc.title'),
			body: $t('mainPage.tour.misc.body'),
			beforeShow: () => {
				activeTab = 'settings';
				settingsSubTab = 'misc';
			}
		}
	]);

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
		refreshHardware();
		api.getBenchmarkStatus().then((s) => {
			benchmarkStatus = s;
			if (s.running) pollBenchmarkStatus();
		});
		loadMcVersions();
		checkCraftdeckVersion();
		refreshUpdateSettings();
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
		clearInterval(benchmarkPollHandle);
		clearInterval(overclockRebootPollHandle);
	});

	async function createInstance() {
		createError = '';
		if (form.loader === 'custom') {
			if (!customLoaderName.trim()) {
				createError = $t('mainPage.create.loaderNameRequired');
				return;
			}
			if (!customJarFile) {
				createError = $t('mainPage.create.jarRequired');
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
						$t('mainPage.create.jarUploadFailed', {
							error: err instanceof Error ? err.message : String(err)
						})
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
						$t('mainPage.create.worldImportFailed', {
							error: err instanceof Error ? err.message : String(err)
						})
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
		askConfirm($t('mainPage.instances.confirmDelete'), () => doRemove(id));
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
				stopped: $t('mainPage.status.stopped'),
				starting: $t('mainPage.status.starting'),
				running: $t('mainPage.status.running'),
				stopping: $t('mainPage.status.stopping'),
				crashed: $t('mainPage.status.crashed')
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
	// URL의 ?tab= 쿼리에 현재 탭을 반영해서, 새로고침해도 "인스턴스"로
	// 튕기지 않고 보고 있던 탭 그대로 돌아오게 한다 (투어 중의 일시적인
	// 탭 전환은 URL을 건드리지 않고 activeTab만 직접 바꾼다 -- 매 스텝마다
	// history를 쌓을 필요는 없어서).
	let activeTab = $state<'instances' | 'settings'>(
		$page.url.searchParams.get('tab') === 'settings' ? 'settings' : 'instances'
	);
	function setActiveTab(tab: 'instances' | 'settings') {
		activeTab = tab;
		const url = new URL(window.location.href);
		if (tab === 'instances') {
			url.searchParams.delete('tab');
		} else {
			url.searchParams.set('tab', tab);
		}
		replaceState(url, {});
	}

	// 설정 탭 안의 2차 분류: 외부 접속/Velocity/도메인 연결처럼 "밖에서
	// 어떻게 들어오는지"에 관한 항목은 network로, 스왑/오버클럭처럼 이
	// 라즈베리파이 자체의 자원을 다루는 항목은 hardware로, 비밀번호/2단계
	// 인증처럼 세션과 무관하게 항상 접근 가능해야 하는 계정 고유 항목은
	// account로, 언어/투어 다시 보기처럼 세션과 무관하지만 계정과는
	// 무관한 부가 항목은 misc로 묶는다. 같은 ?subtab= 쿼리 패턴으로
	// 새로고침해도 유지된다.
	function validSettingsSubTab(v: string | null): 'network' | 'hardware' | 'account' | 'misc' {
		return v === 'hardware' || v === 'account' || v === 'misc' ? v : 'network';
	}
	let settingsSubTab = $state<'network' | 'hardware' | 'account' | 'misc'>(
		validSettingsSubTab($page.url.searchParams.get('subtab'))
	);
	function setSettingsSubTab(tab: 'network' | 'hardware' | 'account' | 'misc') {
		settingsSubTab = tab;
		const url = new URL(window.location.href);
		if (tab === 'network') {
			url.searchParams.delete('subtab');
		} else {
			url.searchParams.set('subtab', tab);
		}
		replaceState(url, {});
	}
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
				{$t('mainPage.header.createServer')}
			</button>
			{#if isLoggedIn}
				<button
					class="border-border rounded-md border px-4 py-2 text-sm font-medium"
					onclick={logout}
				>
					{$t('mainPage.header.logout')}
				</button>
			{/if}
		</div>
	</div>

	{#if loadError}
		<p class="text-destructive mt-4 text-sm">{$t('mainPage.loadError', { error: loadError })}</p>
	{/if}

	<div class="border-border mt-6 flex gap-1 border-b">
		<button
			class="border-b-2 px-3 py-2 text-sm {activeTab === 'instances'
				? 'border-primary font-medium'
				: 'text-muted-foreground border-transparent'}"
			onclick={() => setActiveTab('instances')}>{$t('mainPage.tabs.instances')}</button
		>
		<button
			id="tour-settings-tab"
			class="border-b-2 px-3 py-2 text-sm {activeTab === 'settings'
				? 'border-primary font-medium'
				: 'text-muted-foreground border-transparent'}"
			onclick={() => setActiveTab('settings')}>{$t('mainPage.tabs.settings')}</button
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
				{#if showTour}
					<!-- 처음 접속해서 인스턴스가 하나도 없으면 콘솔 투어 스텝이
						가리킬 대상 자체가 없어서 조용히 건너뛰어졌다 -- 실제
						카드와 똑같이 생긴 예시 카드를 대신 보여줘서, 콘솔 버튼이
						실제로 어떻게 생겼는지는 보여주되 클릭해도 존재하지 않는
						인스턴스로 이동하지 않도록 버튼들은 비활성화해둔다. -->
					<div
						class="border-border bg-card flex items-center justify-between rounded-lg border border-dashed p-4"
					>
						<div>
							<div class="flex items-center gap-2">
								<span class="h-2 w-2 rounded-full bg-muted-foreground/40"></span>
								<span class="font-medium">{$t('mainPage.instances.exampleServerName')}</span>
								<span class="text-muted-foreground text-xs">{$t('mainPage.instances.exampleStatus')}</span>
								<span class="border-border text-muted-foreground rounded border px-1.5 py-0.5 text-[10px]"
									>{$t('mainPage.instances.exampleBadge')}</span
								>
							</div>
							<p class="text-muted-foreground mt-1 text-xs">Paper · 1.21.4 · Java 21</p>
						</div>
						<div class="flex gap-2">
							<button disabled class="border-border rounded-md border px-3 py-1.5 text-sm opacity-50">
								{$t('mainPage.instances.stop')}
							</button>
							<span
								id="tour-console-sample"
								class="border-border rounded-md border px-3 py-1.5 text-sm">{$t('mainPage.instances.console')}</span
							>
							<button
								disabled
								class="border-border text-destructive rounded-md border px-3 py-1.5 text-sm opacity-50"
							>
								{$t('mainPage.instances.delete')}
							</button>
						</div>
					</div>
				{:else}
					<p class="text-muted-foreground text-sm">{$t('mainPage.instances.empty')}</p>
				{/if}
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
								{$t('mainPage.instances.stop')}
							</button>
						{:else}
							<button
								disabled={busyId === inst.id}
								onclick={() => start(inst.id)}
								class="border-border rounded-md border px-3 py-1.5 text-sm disabled:opacity-50"
							>
								{$t('mainPage.instances.start')}
							</button>
						{/if}
						<a
							href="/instances/{inst.id}"
							class="border-border rounded-md border px-3 py-1.5 text-sm">{$t('mainPage.instances.console')}</a
						>
						<button
							disabled={busyId === inst.id}
							onclick={() => remove(inst.id)}
							class="border-border text-destructive rounded-md border px-3 py-1.5 text-sm disabled:opacity-50"
						>
							{$t('mainPage.instances.delete')}
						</button>
					</div>
				</div>
			{/each}
			</div>
			{:else}
			<div class="flex gap-6 lg:min-h-0 lg:flex-1 lg:overflow-hidden">
			<div class="flex w-32 shrink-0 flex-col gap-1">
				<button
					class="rounded-md px-3 py-2 text-left text-sm {settingsSubTab === 'network'
						? 'bg-muted font-medium'
						: 'text-muted-foreground'}"
					onclick={() => setSettingsSubTab('network')}>{$t('mainPage.settings.network')}</button
				>
				<button
					class="rounded-md px-3 py-2 text-left text-sm {settingsSubTab === 'hardware'
						? 'bg-muted font-medium'
						: 'text-muted-foreground'}"
					onclick={() => setSettingsSubTab('hardware')}>{$t('mainPage.settings.hardware')}</button
				>
				<button
					class="rounded-md px-3 py-2 text-left text-sm {settingsSubTab === 'account'
						? 'bg-muted font-medium'
						: 'text-muted-foreground'}"
					onclick={() => setSettingsSubTab('account')}>{$t('mainPage.settings.account')}</button
				>
				<button
					id="tour-misc-tab"
					class="rounded-md px-3 py-2 text-left text-sm {settingsSubTab === 'misc'
						? 'bg-muted font-medium'
						: 'text-muted-foreground'}"
					onclick={() => setSettingsSubTab('misc')}>{$t('mainPage.settings.misc')}</button
				>
			</div>

			<div class="border-border flex-1 border-l pl-6 lg:min-h-0 lg:overflow-y-auto lg:pr-3">
			{#if settingsSubTab === 'network'}
			<div class="space-y-4">
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
			{:else if settingsSubTab === 'hardware'}
			<div class="space-y-4">
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

			<OverclockCard
				{hardwareInfo}
				{hardwareFetchError}
				redetecting={redetectingCooler}
				onRedetect={redetectCooler}
				bind:overclockForm
				{overclockSaving}
				{overclockError}
				onApplyOverclock={requestOverclockReboot}
				rebooting={overclockRebooting}
				{benchmarkStatus}
				{benchmarkStarting}
				onStartBenchmark={requestStartBenchmark}
				onRevertOverclock={revertOverclock}
			/>
			</div>
			{:else if settingsSubTab === 'account'}
			<AccountSettings bind:username bind:totpEnabled />
			{:else}
			<MiscSettings onStartTour={startTour} />
			{/if}
			</div>
			</div>
			{/if}
		</div>

		<!-- 라즈베리파이 리소스는 실행 중인 인스턴스/전역 설정 중 무엇을 보고
			있든 운영자가 항상 확인하고 싶어할 라이브 상태 값이라, 탭 전환과
			무관하게 별도 사이드바에 고정해서 보여준다. 업데이트 설정은 같은
			사이드바 컬럼에 두되, 전역 설정 탭일 때만 노출한다. -->
		<div class="lg:col-span-1 lg:min-h-0 lg:overflow-y-auto lg:pr-3">
			<ResourcePanel {resources} {resourceError} {swapInfo} />
			{#if activeTab === 'settings'}
				<UpdateSettingsCard
					settings={updateSettings}
					fetchError={updateSettingsFetchError}
					bind:form={updateSettingsForm}
					saving={updateSettingsSaving}
					error={updateSettingsError}
					onSave={saveUpdateSettings}
					checkingNow={checkingUpdateNow}
					checkNowMessage={checkNowMessage}
					onCheckNow={checkForUpdateNow}
				/>
			{/if}
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
	onGoToAccountModal={openAccountSettings}
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

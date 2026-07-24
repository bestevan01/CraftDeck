<script lang="ts">
	import { page } from '$app/stores';
	import {
		api,
		PROXY_RESERVED_MEMORY_MB,
		type Instance,
		type OpEntry,
		type Backup,
		type WorldInfo,
		type Plugin,
		type PluginSearchHit,
		type FileEntry,
		type BuildInfo,
		type ServerSetting,
		type NetworkAddresses,
		type DomainConfig,
		type ProxyStatus,
		type SwapInfo
	} from '$lib/api';
	import MemorySlider from '$lib/MemorySlider.svelte';
	import ConfirmDialog from '$lib/ConfirmDialog.svelte';
	import ReasonModal from '$lib/ReasonModal.svelte';
	import PluginSearchModal from '$lib/PluginSearchModal.svelte';
	import GameSettingsModal from '$lib/GameSettingsModal.svelte';
	import ServerSettingsModal from '$lib/ServerSettingsModal.svelte';
	import ManageTab from '$lib/ManageTab.svelte';
	import PluginsTab from '$lib/PluginsTab.svelte';
	import FilesTab from '$lib/FilesTab.svelte';
	import ConsoleTab from '$lib/ConsoleTab.svelte';
	import { onDestroy, onMount, tick } from 'svelte';
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

	const id = $page.params.id as string; // always present: this route only matches with an id segment

	// The 설정/백업/월드데이터/플러그인 cards used to all stack above the
	// console, so as backups/plugins piled up the console (the thing checked
	// most often) kept getting pushed further down the page. Splitting them
	// into tabs means the console tab's height/layout is never affected by
	// how much content the other tabs have.
	// URL의 ?tab= 쿼리에 반영해서, 새로고침해도 보고 있던 탭 그대로 돌아오게
	// 한다 (메인 페이지의 인스턴스/설정 탭과 같은 패턴).
	function validTab(v: string | null): 'console' | 'manage' | 'plugins' | 'files' {
		return v === 'manage' || v === 'plugins' || v === 'files' ? v : 'console';
	}
	let activeTab = $state<'console' | 'manage' | 'plugins' | 'files'>(
		validTab($page.url.searchParams.get('tab'))
	);
	function setActiveTab(tab: 'console' | 'manage' | 'plugins' | 'files') {
		activeTab = tab;
		const url = new URL(window.location.href);
		if (tab === 'console') {
			url.searchParams.delete('tab');
		} else {
			url.searchParams.set('tab', tab);
		}
		replaceState(url, {});
	}
	// URL에서 읽은 탭이 이 인스턴스에 없는 탭일 수도 있다 (예: proxy
	// 인스턴스인데 ?tab=files, 업로드 미지원 로더인데 ?tab=plugins) --
	// 인스턴스 정보가 로드된 뒤 한 번 걸러서 콘솔로 되돌린다.
	$effect(() => {
		if (!inst) return;
		if (activeTab === 'files' && inst.kind !== 'server') setActiveTab('console');
		if (activeTab === 'plugins' && !uploadCapableLoader(inst.loader)) setActiveTab('console');
	});

	let inst = $state<Instance | null>(null);
	let loadError = $state('');
	let lines = $state<string[]>([]);
	let commandText = $state('');
	let logEl: HTMLDivElement;
	let ws: WebSocket | null = null;
	let wsStatus = $state<'connecting' | 'open' | 'closed'>('connecting');
	// Set right before the deliberate ws.close() in onDestroy, so onclose
	// below can tell "we're leaving this page" apart from "the connection
	// dropped out from under us" and only auto-reconnect for the latter.
	let consoleUnmounting = false;
	let consoleReconnectTimer: ReturnType<typeof setTimeout> | undefined;
	// Resets to 1s after every successful open -- a real outage backs off
	// up to 30s instead of hammering the server, but a one-off blip
	// reconnects fast.
	let consoleReconnectDelayMs = 1000;

	// FR-17 quick-command panel state
	let playerName = $state('');
	let announceText = $state('');
	let gamemode = $state('survival');
	let difficulty = $state('easy');
	let onlinePlayers = $state<string[]>([]);
	let bannedPlayers = $state<string[]>([]);
	let ops = $state<OpEntry[]>([]);
	let whitelistedPlayers = $state<string[]>([]);
	let whitelistEnabled = $state(false);

	// Kick/ban reason modal state.
	let reasonModalKind = $state<'kick' | 'ban' | null>(null);
	let customReason = $state('');

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

	// Matches the labels used on the instance list page (+page.svelte's
	// statusLabel) -- inst.status is the backend's raw English state and
	// was being shown to the operator verbatim here.
	function statusLabel(status: Instance['status']) {
		return (
			{
				stopped: $t('instanceDetailPage.status.stopped'),
				starting: $t('instanceDetailPage.status.starting'),
				running: $t('instanceDetailPage.status.running'),
				stopping: $t('instanceDetailPage.status.stopping'),
				crashed: $t('instanceDetailPage.status.crashed')
			}[status] ?? status
		);
	}

	// Mirrors supportsVelocityForwarding in internal/api/handlers_proxy.go --
	// Purpur/Folia/Pufferfish/Leaf are Paper forks that carry proxies.velocity
	// forward unchanged, and Fabric gets there via an auto-installed
	// FabricProxy-Lite mod (see installFabricProxyMods), so it can sit behind
	// the proxy too even though it doesn't use Bukkit-style plugins.
	const proxyCapableLoaders = ['paper', 'purpur', 'folia', 'pufferfish', 'leaf', 'fabric', 'neoforge'];

	// Mirrors internal/loader/loader.go's registry -- loaders CraftDeck can
	// re-download a fresh build for (see reinstallLoader). Anything else is
	// a custom/manually-uploaded loader (FR-3), which only has a jar to
	// replace directly via the 파일 tab, not an adapter to redownload from.
	const knownLoaders = ['vanilla', 'paper', 'purpur', 'folia', 'pufferfish', 'leaf', 'fabric', 'neoforge'];

	// Mirrors uploadSupported in internal/api/handlers_plugin.go -- gates
	// whether the plugin/mod tab shows at all. Broader than
	// searchCapableLoaders below: a manually-uploaded custom loader jar
	// (FR-3) has no Modrinth tag to search by, but it still has *some*
	// directory (pluginsDirName) the server scans for extension jars, so it
	// can still receive manual uploads. Only Vanilla (no extension
	// mechanism) and the Velocity proxy (a different, unmanaged plugin
	// ecosystem) are excluded.
	function uploadCapableLoader(loader: string) {
		return loader !== 'vanilla' && loader !== 'velocity';
	}

	// Mirrors searchSupported in internal/api/handlers_plugin.go -- gates
	// the "Modrinth에서 검색" button specifically, since Modrinth search
	// needs a loader tag it actually recognizes. The Paper-family loaders
	// load plugins the Bukkit-API way, Fabric/NeoForge load mods instead (a
	// different Modrinth project_type and on-disk directory, see
	// pluginsDirName/modrinthProjectType).
	const searchCapableLoaders = ['paper', 'purpur', 'folia', 'pufferfish', 'leaf', 'fabric', 'neoforge'];

	// Mirrors modrinthProjectType in internal/api/handlers_plugin.go -- just
	// the label, since the API calls are identical either way.
	function pluginTabLabel(loader: string | undefined) {
		return loader === 'fabric' || loader === 'neoforge'
			? $t('instanceDetailPage.pluginTab.mode')
			: $t('instanceDetailPage.pluginTab.plugin');
	}

	// Sending an RCON command while the server is mid-shutdown (its main
	// thread has already stopped ticking but the process hasn't exited yet)
	// trips Paper/Spigot's AsyncCatcher and spams "Cannot perform command
	// async" errors into its log -- confirmed by real-hardware logs showing
	// this exact error firing from our own background polls during a
	// graceful stop. Only poll over RCON while the instance is actually
	// running.
	function rconReady() {
		return inst?.status === 'running';
	}

	// Refreshes the online-player chips without touching the console log --
	// used both after an explicit refresh click and by the background poll
	// below. This queries Minecraft's own Server List Ping protocol (the
	// same one the game client uses for the multiplayer server list) rather
	// than RCON's "list" command: plugins like EssentialsX (confirmed on
	// real hardware) freely rewrite "list"'s text output, which silently
	// broke any parser built around vanilla's exact wording. Status Ping is
	// a fixed protocol no plugin can reformat, and it works even without a
	// live RCON connection.
	async function refreshPlayerList() {
		try {
			const res = await api.onlinePlayers(id);
			onlinePlayers = res.sample;
		} catch {
			// server likely not running yet; leave the last known list as-is
			// rather than clearing it out from under the user
		}
	}

	async function refreshBans() {
		if (!rconReady()) return;
		try {
			const res = await api.listBans(id);
			bannedPlayers = res.players;
		} catch {
			// leave last known list as-is (server may be off, RCON not ready)
		}
	}

	async function refreshOps() {
		try {
			ops = await api.listOps(id);
		} catch {
			// ops.json may not exist yet, or server dir isn't readable yet
		}
	}

	async function refreshWhitelist() {
		if (!rconReady()) return;
		try {
			const res = await api.listWhitelist(id);
			whitelistEnabled = res.enabled;
			whitelistedPlayers = res.players;
		} catch {
			// leave last known list as-is (server may be off, RCON not ready)
		}
	}

	// Fetched once, the first time we learn this is a Paper server -- avoids
	// refetching on every 5s refreshInstance poll.
	let subdomainLoaded = false;

	async function refreshInstance() {
		try {
			inst = await api.getInstance(id);
			loadError = '';
			// Baseline the "applied" snapshot once, from whatever's already
			// running when the page loads -- see computePendingRestart above.
			if (!appliedInitialized) {
				appliedCpu = inst.cpu_quota_percent;
				appliedMemoryMB = inst.memory_max_mb;
				appliedGamePort = inst.game_port;
				appliedInitialized = true;
			}
			computePendingRestart();
			if (inst.kind === 'server' && !subdomainLoaded) {
				subdomainLoaded = true;
				loadDomainConfig().then(refreshSubdomain);
			} else if (inst.kind === 'proxy' && !subdomainLoaded) {
				// The proxy's own page has no subdomain form, but still needs
				// domainConfig loaded so the 접속 주소 card's "domain
				// registered -> hide public IP" rule applies here too.
				subdomainLoaded = true;
				loadDomainConfig();
			}
			if (!addressesLoaded) {
				addressesLoaded = true;
				loadNetworkAddresses();
			}
			if (inst.kind === 'server' && !buildsLoaded && knownLoaders.includes(inst.loader)) {
				buildsLoaded = true;
				selectedBuildVersion = inst.loader_version ?? '';
				loadBuilds();
			}
		} catch (err) {
			loadError = err instanceof Error ? err.message : String(err);
		}
	}

	function appendLine(line: string) {
		lines = [...lines, line];
		if (lines.length > 500) lines = lines.slice(-500); // cap client-side buffer
		tick().then(() => {
			logEl?.scrollTo({ top: logEl.scrollHeight });
		});
	}

	// Minecraft log lines look like "[09:50:34] [Server thread/INFO]: msg".
	// Split off the timestamp/thread/level bracket so it can be dimmed, and
	// color the message by level (WARN/ERROR/FATAL) so problems stand out
	// against the wall of plain white INFO text.
	const logLineRE = /^(\[\d{2}:\d{2}:\d{2}\]\s*\[[^\]]+\]:)\s*(.*)$/;
	function parseLogLine(line: string): { prefix: string; message: string; messageClass: string } {
		if (!line) {
			return { prefix: '', message: '', messageClass: 'text-foreground/90' };
		}
		if (line.startsWith('> ')) {
			return { prefix: '', message: line, messageClass: 'text-cyan-400 font-semibold' };
		}
		if (line.startsWith($t('instanceDetailPage.console.errorPrefix'))) {
			return { prefix: '', message: line, messageClass: 'text-red-400' };
		}
		const m = line.match(logLineRE);
		if (!m) {
			return { prefix: '', message: line, messageClass: 'text-foreground/90' };
		}
		const [, prefix, message] = m;
		let messageClass = 'text-foreground/90';
		if (/\/WARN\]:$/.test(prefix)) messageClass = 'text-yellow-400';
		else if (/\/(ERROR|FATAL)\]:$/.test(prefix)) messageClass = 'text-red-400';
		return { prefix, message, messageClass };
	}

	function connectConsole() {
		clearTimeout(consoleReconnectTimer);
		ws = new WebSocket(api.consoleURL(id));
		wsStatus = 'connecting';
		ws.onopen = () => {
			wsStatus = 'open';
			consoleReconnectDelayMs = 1000;
		};
		ws.onclose = () => {
			wsStatus = 'closed';
			if (consoleUnmounting) return;
			// The backend now pings every 20s (see handleConsoleWebSocket) so
			// a real idle-timeout drop shouldn't happen anymore, but this
			// covers whatever still can -- the daemon restarting for an
			// update, a network blip, etc. -- without the operator having to
			// notice and refresh the page themselves (confirmed: that used
			// to be the only way back, even though the instance itself and
			// its console were both still perfectly fine).
			consoleReconnectTimer = setTimeout(connectConsole, consoleReconnectDelayMs);
			consoleReconnectDelayMs = Math.min(consoleReconnectDelayMs * 2, 30000);
		};
		ws.onerror = () => ws?.close();
		ws.onmessage = (event) => {
			const frame = JSON.parse(event.data);
			if (frame.type === 'log') {
				appendLine(frame.line);
			} else if (frame.type === 'cmd_result') {
				appendLine(`> ${frame.command}`);
				appendLine(
					frame.ok
						? frame.line
						: `${$t('instanceDetailPage.console.errorPrefix')} ${frame.error}`
				);
			}
		};
	}

	function sendCommand(command: string) {
		if (!command.trim()) return;
		if (ws && wsStatus === 'open') {
			ws.send(JSON.stringify({ type: 'command', text: command }));
		} else {
			// WS not connected (e.g. server just booting) -- fall back to REST.
			api
				.sendCommand(id, command)
				.then((res) => {
					appendLine(`> ${command}`);
					appendLine(res.result);
				})
				.catch((err) =>
					appendLine(
						`${$t('instanceDetailPage.console.errorPrefix')} ${err instanceof Error ? err.message : err}`
					)
				);
		}
	}

	function submitFreeform(e: SubmitEvent) {
		e.preventDefault();
		sendCommand(commandText);
		commandText = '';
	}

	// Ban/op state changes take effect immediately server-side, but our chip
	// lists are separate REST calls -- refresh them shortly after so the UI
	// catches up without waiting for the next background poll.
	// Opens the reason-picker modal instead of sending kick/ban immediately,
	// so the operator can pick a preset reason (or type a custom one) that
	// gets appended to the command.
	function openReasonModal(kind: 'kick' | 'ban') {
		customReason = '';
		reasonModalKind = kind;
	}
	function closeReasonModal() {
		reasonModalKind = null;
	}
	function applyReason(reason: string) {
		if (!reasonModalKind) return;
		const cmd = reasonModalKind;
		const trimmed = reason.trim();
		sendCommand(trimmed ? `${cmd} ${playerName} ${trimmed}` : `${cmd} ${playerName}`);
		if (cmd === 'ban') setTimeout(refreshBans, 500);
		closeReasonModal();
	}
	function pardonPlayer() {
		sendCommand(`pardon ${playerName}`);
		setTimeout(refreshBans, 500);
	}
	function opPlayer() {
		sendCommand(`op ${playerName}`);
		setTimeout(refreshOps, 500);
	}
	function deopPlayer() {
		sendCommand(`deop ${playerName}`);
		setTimeout(refreshOps, 500);
	}
	function whitelistAdd() {
		sendCommand(`whitelist add ${playerName}`);
		setTimeout(refreshWhitelist, 500);
	}
	function whitelistRemove() {
		sendCommand(`whitelist remove ${playerName}`);
		setTimeout(refreshWhitelist, 500);
	}
	function whitelistToggle(on: boolean) {
		sendCommand(`whitelist ${on ? 'on' : 'off'}`);
		setTimeout(refreshWhitelist, 500);
	}

	// CPU/memory settings (FR-12). Editable even while the instance is
	// running -- CPU/memory limits are only ever applied to a fresh process,
	// so a save just writes the new values without touching the running
	// unit. They take effect once the operator explicitly restarts
	// (pendingRestart flags that).
	//
	// game_port used to be flatly non-editable (auto-assigned at creation,
	// never surfaced here). It's now editable, but only for a server that
	// isn't registered behind the proxy -- one that is gets there by
	// subdomain and is bound to 127.0.0.1, so its own game_port is neither
	// reachable nor meaningful to change. Mirrors the same
	// `!subdomain?.registered` check connectPort above already uses.
	let editingSettings = $state(false);
	let settingsCpu = $state(0); // percent, 0 = unlimited
	let settingsMemoryGB = $state(1);
	let settingsGamePort = $state(25566);
	let settingsError = $state('');
	let settingsSaving = $state(false);
	let pendingRestart = $state(false);
	let restarting = $state(false);
	// Snapshot of the CPU/memory/game_port values actually in effect on the
	// currently-running process, so we can tell a real pending change (needs
	// a restart) apart from the operator editing settings and then putting
	// them back to what's already running -- in which case the restart
	// button should disappear again rather than stay stuck on.
	let appliedCpu = 0;
	let appliedMemoryMB = 0;
	let appliedGamePort = 0;
	let appliedInitialized = false;

	function computePendingRestart() {
		if (!inst || (inst.status !== 'running' && inst.status !== 'starting')) {
			pendingRestart = false;
			return;
		}
		pendingRestart =
			inst.cpu_quota_percent !== appliedCpu ||
			inst.memory_max_mb !== appliedMemoryMB ||
			inst.game_port !== appliedGamePort;
	}
	// Raspberry Pi's total RAM in GB, used to cap the memory slider -- filled
	// in from /api/system/resources on mount; 1 is just a safe placeholder
	// until that responds.
	let maxMemoryGB = $state(1);
	// Where the slider's physical-RAM/swap marker sits (see MemorySlider) --
	// physical RAM alone, minus the same proxy reservation maxMemoryGB
	// subtracts, so the marker lines up with "this much is real RAM" rather
	// than counting the reserved 1GB as swappable headroom. Defaults equal
	// to maxMemoryGB (no marker shown) until the real numbers load.
	let ramBoundaryGB = $state(1);

	async function loadSystemResources() {
		try {
			const [res, proxyStatus, swapInfo] = await Promise.all([
				api.systemResources(),
				api.getProxyStatus().catch(() => null as ProxyStatus | null),
				api.getSwap().catch(() => null as SwapInfo | null)
			]);
			// The always-on Velocity proxy has a fixed 1GB allocation (see
			// PROXY_RESERVED_MEMORY_MB) that this server's slider shouldn't be
			// able to eat into -- but only while it actually exists and is
			// running; if it's torn down (FR-1f, no main domain registered) or
			// just not running right now, nothing is reserved. CraftDeck's own
			// swap file (internal/swap), if turned on, adds to the ceiling too --
			// instances only actually get to use it when it's on (see AllowSwap
			// in startInstanceCore).
			let ram = res.total_memory_mb;
			if (proxyStatus?.exists && proxyStatus.running) ram -= PROXY_RESERVED_MEMORY_MB;
			let total = ram;
			if (swapInfo?.enabled) total += swapInfo.size_mb;
			ramBoundaryGB = Math.max(1, Math.floor(ram / 1024));
			maxMemoryGB = Math.max(1, Math.floor(total / 1024));
		} catch {
			// leave the placeholder -- worst case the slider just caps at 1GB
		}
	}

	function openSettingsEdit() {
		if (!inst) return;
		settingsCpu = inst.cpu_quota_percent;
		settingsMemoryGB = Math.min(
			maxMemoryGB,
			Math.max(1, Math.round(inst.memory_max_mb / 1024) || 1)
		);
		settingsGamePort = inst.game_port;
		settingsError = '';
		editingSettings = true;
	}
	function cancelSettingsEdit() {
		editingSettings = false;
	}
	async function saveSettings() {
		settingsError = '';
		settingsSaving = true;
		try {
			inst = await api.updateInstance(id, {
				cpu_quota_percent: settingsCpu,
				memory_max_mb: settingsMemoryGB * 1024,
				...(canEditGamePort ? { game_port: settingsGamePort } : {})
			});
			editingSettings = false;
			computePendingRestart();
		} catch (err) {
			settingsError = err instanceof Error ? err.message : String(err);
		} finally {
			settingsSaving = false;
		}
	}

	async function restartForSettings() {
		restarting = true;
		try {
			await api.restartInstance(id);
			await refreshInstance();
			// Whatever's in inst now is what actually got applied on restart --
			// re-baseline the snapshot instead of waiting for appliedInitialized
			// (which only fires once, on the very first load).
			if (inst) {
				appliedCpu = inst.cpu_quota_percent;
				appliedMemoryMB = inst.memory_max_mb;
				appliedGamePort = inst.game_port;
			}
			computePendingRestart();
		} finally {
			restarting = false;
		}
	}

	// Backups (FR-13). Create/restore are only allowed while stopped -- see
	// the matching guard on the backend (handlers_backup.go).
	let backups = $state<Backup[]>([]);
	let backupsError = $state('');
	let creatingBackup = $state(false);
	let busyBackupId = $state<string | null>(null);

	// File manager (FR-12 and beyond) -- an Explorer/Finder-style browser
	// over the instance's whole work dir: navigate directories, drag-and-
	// drop or pick a file to upload, download, double-click to edit a text
	// file, rename, delete. All path handling is validated server-side
	// (resolveInstancePath in internal/api/handlers_files.go); this is just
	// the UI on top of it.
	let filesPath = $state(''); // relative dir currently shown, '' = work dir root
	let fileEntries = $state<FileEntry[]>([]);
	let filesError = $state('');
	let loadingFiles = $state(false);
	let isDraggingOverFiles = $state(false);
	let uploadingFiles = $state(false);

	let editingFile = $state<string | null>(null);
	let editingContent = $state('');
	let loadingFileContent = $state(false);
	let savingFileContent = $state(false);
	let fileContentError = $state('');
	let fileContentSaved = $state(false);

	// 열기를 시도했는데 텍스트로 못 읽는 경우(바이너리, 415) 등 -- 편집기
	// 모달을 열어버리면 내용이 빈 채로 "저장" 버튼이 활성화돼 있어서,
	// 실수로 누르면 원본 파일이 빈 내용으로 덮어써질 위험이 있었다. 그래서
	// 이 경우엔 편집기를 아예 열지 않고 별도 경고 모달로 안내한다.
	let fileOpenErrorEntry = $state<FileEntry | null>(null);
	let fileOpenError = $state('');

	let renamingFile = $state<string | null>(null);
	let renameInput = $state('');

	function filesBreadcrumb() {
		return filesPath.split('/').filter(Boolean);
	}

	async function refreshFiles() {
		loadingFiles = true;
		try {
			fileEntries = await api.listFiles(id, filesPath);
			filesError = '';
		} catch (err) {
			filesError = err instanceof Error ? err.message : String(err);
		} finally {
			loadingFiles = false;
		}
	}

	function navigateToPath(path: string) {
		filesPath = path;
		refreshFiles();
	}

	function navigateUp() {
		const parts = filesBreadcrumb();
		parts.pop();
		navigateToPath(parts.join('/'));
	}

	function navigateToBreadcrumb(index: number) {
		navigateToPath(filesBreadcrumb().slice(0, index + 1).join('/'));
	}

	async function openEntry(entry: FileEntry) {
		if (entry.is_dir) {
			navigateToPath(entry.path);
			return;
		}
		fileOpenError = '';
		fileOpenErrorEntry = null;
		loadingFileContent = true;
		try {
			const res = await api.getFileContent(id, entry.path);
			editingFile = entry.path;
			editingContent = res.content;
			fileContentError = '';
			fileContentSaved = false;
		} catch (err) {
			// 어떤 이유로든(바이너리, 용량 초과, 기타 오류) 내용을 못 불러왔으면
			// 편집기 자체를 열지 않는다 -- 빈 textarea에 저장 버튼을 활성화해두면
			// 실수로 원본을 지울 위험이 있다.
			fileOpenError = err instanceof Error ? err.message : String(err);
			fileOpenErrorEntry = entry;
		} finally {
			loadingFileContent = false;
		}
	}

	function closeFileOpenError() {
		fileOpenError = '';
		fileOpenErrorEntry = null;
	}

	function closeFileEditor() {
		editingFile = null;
	}

	async function saveFileContent() {
		if (!editingFile) return;
		savingFileContent = true;
		fileContentError = '';
		fileContentSaved = false;
		try {
			await api.setFileContent(id, editingFile, editingContent);
			fileContentSaved = true;
		} catch (err) {
			fileContentError = err instanceof Error ? err.message : String(err);
		} finally {
			savingFileContent = false;
		}
	}

	function downloadEntry(entry: FileEntry) {
		window.open(api.downloadFileURL(id, entry.path), '_blank');
	}

	async function uploadFiles(fileList: FileList | File[]) {
		uploadingFiles = true;
		filesError = '';
		try {
			for (const file of Array.from(fileList)) {
				await api.uploadFile(id, filesPath, file);
			}
			await refreshFiles();
		} catch (err) {
			filesError = err instanceof Error ? err.message : String(err);
		} finally {
			uploadingFiles = false;
		}
	}

	function onFilePickerChange(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		if (input.files && input.files.length > 0) uploadFiles(input.files);
		input.value = '';
	}

	function onFilesDragOver(e: DragEvent) {
		e.preventDefault();
		isDraggingOverFiles = true;
	}
	function onFilesDragLeave() {
		isDraggingOverFiles = false;
	}
	function onFilesDrop(e: DragEvent) {
		e.preventDefault();
		isDraggingOverFiles = false;
		if (e.dataTransfer?.files && e.dataTransfer.files.length > 0) {
			uploadFiles(e.dataTransfer.files);
		}
	}

	function startRename(entry: FileEntry) {
		renamingFile = entry.path;
		renameInput = entry.name;
	}

	function cancelRename() {
		renamingFile = null;
	}

	async function confirmRename() {
		if (!renamingFile) return;
		const dir = renamingFile.split('/').slice(0, -1).join('/');
		const newPath = dir ? `${dir}/${renameInput}` : renameInput;
		try {
			await api.renameFile(id, renamingFile, newPath);
			renamingFile = null;
			await refreshFiles();
		} catch (err) {
			filesError = err instanceof Error ? err.message : String(err);
		}
	}

	function deleteEntry(entry: FileEntry) {
		const label = entry.is_dir
			? $t('instanceDetailPage.files.deleteEntryLabel.folder')
			: $t('instanceDetailPage.files.deleteEntryLabel.file');
		askConfirm(
			$t('instanceDetailPage.files.deleteConfirm', { label, path: entry.path }),
			() => doDeleteEntry(entry)
		);
	}

	async function doDeleteEntry(entry: FileEntry) {
		try {
			await api.deleteFile(id, entry.path);
			await refreshFiles();
		} catch (err) {
			filesError = err instanceof Error ? err.message : String(err);
		}
	}

	async function refreshBackups() {
		try {
			backups = await api.listBackups(id);
			backupsError = '';
		} catch (err) {
			backupsError = err instanceof Error ? err.message : String(err);
		}
	}

	async function createBackup() {
		creatingBackup = true;
		try {
			await api.createBackup(id);
			await refreshBackups();
		} catch (err) {
			backupsError = err instanceof Error ? err.message : String(err);
		} finally {
			creatingBackup = false;
		}
	}

	function restoreBackup(backupId: string) {
		askConfirm($t('instanceDetailPage.backups.restoreConfirm'), () => doRestoreBackup(backupId));
	}

	async function doRestoreBackup(backupId: string) {
		busyBackupId = backupId;
		try {
			await api.restoreBackup(id, backupId);
		} catch (err) {
			backupsError = err instanceof Error ? err.message : String(err);
		} finally {
			busyBackupId = null;
		}
	}

	function deleteBackup(backupId: string) {
		askConfirm($t('instanceDetailPage.backups.deleteConfirm'), () => doDeleteBackup(backupId));
	}

	async function doDeleteBackup(backupId: string) {
		busyBackupId = backupId;
		try {
			await api.deleteBackup(id, backupId);
			await refreshBackups();
		} catch (err) {
			backupsError = err instanceof Error ? err.message : String(err);
		} finally {
			busyBackupId = null;
		}
	}

	// Config files are typically a few hundred bytes to a few KB -- always
	// dividing by 1024*1024 like formatBytes does (fine for MB/GB-scale
	// backups) would round every one of them down to "0.0MB".
	function formatFileSize(bytes: number) {
		if (bytes < 1024) return `${bytes}B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`;
		return `${(bytes / 1024 / 1024).toFixed(1)}MB`;
	}

	// World data export/import: lets an operator download just the world
	// folders, or replace them with an uploaded archive (e.g. from another
	// server). Both require the instance stopped -- same reasoning as
	// backups.
	let worldInfo = $state<WorldInfo | null>(null);
	let worldInfoError = $state('');
	let importFile = $state<File | null>(null);
	let importing = $state(false);
	let importError = $state('');
	let importForceConfirm = $state(false);
	let importSuccess = $state('');

	async function refreshWorldInfo() {
		try {
			worldInfo = await api.worldInfo(id);
			worldInfoError = '';
		} catch (err) {
			worldInfoError = err instanceof Error ? err.message : String(err);
		}
	}

	function exportWorld() {
		window.open(api.exportWorldURL(id), '_blank');
	}

	function onImportFileChange(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		importFile = input.files?.[0] ?? null;
		importError = '';
		importForceConfirm = false;
		importSuccess = '';
	}

	async function importWorld(force = false) {
		if (!importFile) return;
		importing = true;
		importError = '';
		importSuccess = '';
		try {
			const result = await api.importWorld(id, importFile, force);
			importFile = null;
			importForceConfirm = false;
			importSuccess = $t('instanceDetailPage.world.importSuccess', {
				version: result.detected_version || $t('instanceDetailPage.world.unknownVersion')
			});
			await refreshWorldInfo();
		} catch (err) {
			const message = err instanceof Error ? err.message : String(err);
			// The backend returns 409 with a human-readable downgrade warning
			// and expects a retry with force=true to actually proceed.
			if (message.includes('강제 적용')) {
				importError = message;
				importForceConfirm = true;
			} else {
				importError = message;
			}
		} finally {
			importing = false;
		}
	}

	// Plugins (Modrinth search/install + direct upload). Only meaningful for
	// Paper instances right now -- see internal/api/handlers_plugin.go's
	// pluginsSupported.
	let plugins = $state<Plugin[]>([]);
	let pluginsError = $state('');
	let pluginQuery = $state('');
	let pluginSearchResults = $state<PluginSearchHit[]>([]);
	let pluginSearchError = $state('');
	let searchingPlugins = $state(false);
	let installingProjectId = $state<string | null>(null);
	let uploadingPlugin = $state(false);
	let busyPluginId = $state<string | null>(null);
	let showPluginSearchModal = $state(false);

	function openPluginSearchModal() {
		pluginQuery = '';
		pluginSearchResults = [];
		pluginSearchError = '';
		showPluginSearchModal = true;
	}

	async function refreshPlugins() {
		try {
			plugins = await api.listPlugins(id);
			pluginsError = '';
		} catch (err) {
			pluginsError = err instanceof Error ? err.message : String(err);
		}
	}

	async function searchPlugins(e: SubmitEvent) {
		e.preventDefault();
		searchingPlugins = true;
		pluginSearchError = '';
		try {
			pluginSearchResults = await api.searchPlugins(id, pluginQuery);
		} catch (err) {
			pluginSearchError = err instanceof Error ? err.message : String(err);
		} finally {
			searchingPlugins = false;
		}
	}

	async function installPlugin(projectId: string) {
		installingProjectId = projectId;
		pluginsError = '';
		try {
			await api.installPlugin(id, projectId);
			await refreshPlugins();
		} catch (err) {
			pluginsError = err instanceof Error ? err.message : String(err);
		} finally {
			installingProjectId = null;
		}
	}

	function onPluginFileChange(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		input.value = '';
		if (!file) return;
		uploadingPlugin = true;
		pluginsError = '';
		api
			.uploadPlugin(id, file)
			.then(refreshPlugins)
			.catch((err) => {
				pluginsError = err instanceof Error ? err.message : String(err);
			})
			.finally(() => {
				uploadingPlugin = false;
			});
	}

	async function togglePlugin(p: Plugin) {
		busyPluginId = p.id;
		try {
			await api.setPluginEnabled(id, p.id, !p.enabled);
			await refreshPlugins();
		} catch (err) {
			pluginsError = err instanceof Error ? err.message : String(err);
		} finally {
			busyPluginId = null;
		}
	}

	function deletePlugin(p: Plugin) {
		askConfirm($t('instanceDetailPage.plugins.deleteConfirm', { filename: p.title || p.filename }), () =>
			doDeletePlugin(p)
		);
	}

	async function doDeletePlugin(p: Plugin) {
		busyPluginId = p.id;
		try {
			await api.deletePlugin(id, p.id);
			await refreshPlugins();
		} catch (err) {
			pluginsError = err instanceof Error ? err.message : String(err);
		} finally {
			busyPluginId = null;
		}
	}

	// Subdomain registration (only meaningful for Paper servers -- Vanilla
	// can't trust the proxy's modern forwarding at all, see
	// resolveProxyBackendEntries, so it's never registered as a backend).
	// The always-on Velocity proxy itself has no operator-facing UI at all
	// (see ensureProxyInstance/proxyMemoryMaxMB) -- this is the one setting
	// an operator actually needs day-to-day, so it lives here on the
	// server's own console instead of a separate proxy instance page.
	let subdomain = $state<{ registered: boolean; forced_host: string; proxy_port?: number } | null>(
		null
	);
	let subdomainInput = $state('');
	let subdomainError = $state('');
	let savingSubdomain = $state(false);

	// FR-1f guarantees the proxy only exists at all when an owned main
	// domain is registered, so any server that's registered behind it can
	// assume one is too -- fetched once so the subdomain form can take just
	// the label (e.g. "survival") and append ".<도메인>" automatically
	// instead of making the operator retype the whole domain every time.
	let domainConfig = $state<DomainConfig | null>(null);
	let domainConfigLoaded = false;

	async function loadDomainConfig() {
		try {
			const res = await api.getDomainSettings();
			domainConfig = 'id' in res ? res : null;
		} catch {
			// non-critical -- falls back to a plain full-hostname input
		}
	}

	// ".craftdeck.cc" (leading dot) when a main domain is registered,
	// "" otherwise (independent/free-subdomain cases -- see loadDomainConfig).
	let domainSuffix = $derived(domainConfig?.kind === 'main_domain' ? `.${domainConfig.hostname}` : '');

	function labelFromForcedHost(forcedHost: string) {
		if (domainSuffix && forcedHost.endsWith(domainSuffix)) {
			return forcedHost.slice(0, -domainSuffix.length);
		}
		return forcedHost;
	}

	// 25565 is Minecraft's own standard port -- the client already tries it
	// automatically when no port is typed, so appending ":25565" is just
	// noise (and now that the proxy defaults to 25565, this is the common
	// case for anything reachable through it).
	const MINECRAFT_DEFAULT_PORT = 25565;
	function formatAddress(host: string, port: number) {
		return port === MINECRAFT_DEFAULT_PORT ? host : `${host}:${port}`;
	}

	// 접속 주소 복사 버튼: 사설 IP는 항상, 공인 IP는 외부 접속이 켜져 있을
	// 때만(백엔드가 그때만 값을 채워 보냄) 표시한다. 프록시 인스턴스이거나,
	// 서버 인스턴스(독립 노출이든 프록시 등록이든 -- 프록시에 등록된 경우는
	// 프록시 자신의 포트로 접속해야 하므로 subdomain.proxy_port를 쓴다)일 때
	// 표시한다.
	let networkAddresses = $state<NetworkAddresses | null>(null);
	let addressesLoaded = false;

	let directlyReachable = $derived(inst?.kind === 'proxy' || (inst?.kind === 'server' && subdomain !== null));

	// The port a player actually connects to: the proxy's own port when
	// this server is registered behind it (it's bound to 127.0.0.1 only),
	// otherwise this instance's own game_port (independent exposure, or
	// the proxy instance page itself).
	let connectPort = $derived(
		inst?.kind === 'server' && subdomain?.registered && subdomain.proxy_port
			? subdomain.proxy_port
			: (inst?.game_port ?? 0)
	);

	// Mirrors the same "registered behind the proxy?" check connectPort just
	// used above -- a registered server is reached by subdomain and bound to
	// 127.0.0.1, so its own game_port isn't reachable or meaningful to
	// change (see canEditGamePort's backend-side twin in handleUpdateInstance).
	let canEditGamePort = $derived(inst?.kind === 'server' && !subdomain?.registered);

	// The domain-based address to show, if any -- three cases:
	//   - proxy 인스턴스 자신: 등록된 서브도메인이 없는 접속(기본/우선순위
	//     라우팅)이 도달하는 곳이므로 도메인 자체를 그대로 보여준다.
	//   - 프록시에 등록된 서버: 서브도메인이 지정됐으면 그 서브도메인이 정확히
	//     이 서버로 라우팅되므로 그걸 보여준다. 아직 지정하지 않았어도 "루트
	//     도메인"으로 접속하면 프록시의 우선순위 목록(FR-1d)에 따라 이 서버로
	//     올 수 있으므로(특히 등록된 서버가 이거 하나뿐이거나 1순위인 경우 항상
	//     이 서버로 온다) 루트 도메인 주소를 그대로 보여준다 -- 다른 서버가
	//     1순위일 수 있다는 점은 바로 위 안내 문구(subdomain?.registered
	//     분기)로 설명하지, 주소 자체를 숨기지는 않는다.
	//   - 독립 노출된 서버: 이 서버 자신의 포트가 공유기에 직접 포워딩되어
	//     있으므로, 도메인 + 그 포트로 곧장 도달한다(프록시를 거치지 않음).
	//   - 무료 서브도메인(DuckDNS/ipTime) 등록됨: FR-1f에 따라 이 경우 프록시
	//     자체가 존재하지 않으므로(모든 서버가 독립 노출) 공인 IP와 똑같은
	//     방식으로, 이 서버 자신의 포트와 함께 그대로 보여준다. FR-26h 골격은
	//     아직 서브도메인을 특정 인스턴스에 귀속시키지 않으므로(FR-27 미구현)
	//     서버 종류와 무관하게 항상 표시한다.
	let domainAddress = $derived.by(() => {
		if (!inst || !domainConfig) return '';
		if (domainConfig.kind === 'free_subdomain') {
			if (inst.kind !== 'server') return '';
			return formatAddress(domainConfig.hostname, connectPort);
		}
		if (domainConfig.kind !== 'main_domain') return '';
		if (inst.kind === 'proxy') return formatAddress(domainConfig.hostname, connectPort);
		if (!subdomain) return '';
		if (subdomain.registered && subdomain.forced_host) {
			return formatAddress(subdomain.forced_host, connectPort);
		}
		return formatAddress(domainConfig.hostname, connectPort);
	});

	let domainAddressLabel = $derived(
		domainConfig?.kind === 'free_subdomain'
			? $t('instanceDetailPage.network.domainLabelFree')
			: $t('instanceDetailPage.network.domainLabelDefault')
	);

	async function loadNetworkAddresses() {
		try {
			networkAddresses = await api.getNetworkAddresses();
		} catch {
			// non-critical -- section just won't show an address
		}
	}

	async function refreshSubdomain() {
		try {
			subdomain = await api.getServerSubdomain(id);
			subdomainInput = labelFromForcedHost(subdomain.forced_host);
			subdomainError = '';
		} catch (err) {
			subdomainError = err instanceof Error ? err.message : String(err);
		}
	}

	async function saveSubdomain() {
		savingSubdomain = true;
		subdomainError = '';
		try {
			const fullHost = domainSuffix
				? `${subdomainInput.trim()}${domainSuffix}`
				: subdomainInput.trim();
			subdomain = await api.setServerSubdomain(id, fullHost);
			subdomainInput = labelFromForcedHost(subdomain.forced_host);
		} catch (err) {
			subdomainError = err instanceof Error ? err.message : String(err);
		} finally {
			savingSubdomain = false;
		}
	}

	// Manual proxy registration -- the escape hatch for a custom/manually-
	// uploaded loader (FR-3) that CraftDeck doesn't recognize as Velocity-
	// forwarding-capable, so it never got added to the proxy automatically
	// at creation (see supportsVelocityForwarding in
	// internal/api/handlers_proxy.go). CraftDeck can't verify an arbitrary
	// jar actually trusts the forwarding secret, so this is an explicit,
	// operator-responsible action -- the returned secret has to be pasted
	// into whatever config that loader needs, e.g. via the 파일 tab.
	let registeringProxy = $state(false);
	let unregisteringProxy = $state(false);
	let proxyRegError = $state('');
	let registeredSecret = $state('');

	async function registerBehindProxy() {
		registeringProxy = true;
		proxyRegError = '';
		registeredSecret = '';
		try {
			const res = await api.registerBehindProxy(id);
			registeredSecret = res.forwarding_secret;
			await refreshSubdomain();
		} catch (err) {
			proxyRegError = err instanceof Error ? err.message : String(err);
		} finally {
			registeringProxy = false;
		}
	}

	function unregisterFromProxy() {
		askConfirm($t('instanceDetailPage.proxy.unregisterConfirm'), doUnregisterFromProxy);
	}

	async function doUnregisterFromProxy() {
		unregisteringProxy = true;
		proxyRegError = '';
		try {
			await api.unregisterFromProxy(id);
			registeredSecret = '';
			await refreshSubdomain();
		} catch (err) {
			proxyRegError = err instanceof Error ? err.message : String(err);
		} finally {
			unregisteringProxy = false;
		}
	}

	// FR-12: curated GUI form over server.properties, shown in a modal (see
	// showGameSettingsModal) since the full form is too long to keep inline
	// on the page -- fetched fresh each time the modal opens, edited
	// locally, and saved as a single batch (see handleSetServerSettings).
	// Anything not on this curated list (custom loader-specific config,
	// rare keys) stays editable only via the general file manager (FR-12a).
	let showGameSettingsModal = $state(false);
	let gameSettings = $state<ServerSetting[]>([]);
	let gameSettingsLoading = $state(false);
	let gameSettingsError = $state('');
	let gameSettingsSaving = $state(false);
	let gameSettingsSaved = $state(false);
	let gameSettingsEdits = $state<Record<string, string>>({});

	function openGameSettingsModal() {
		showGameSettingsModal = true;
		gameSettingsSaved = false;
		loadGameSettings();
	}

	function closeGameSettingsModal() {
		showGameSettingsModal = false;
	}

	async function loadGameSettings() {
		gameSettingsLoading = true;
		gameSettingsError = '';
		try {
			gameSettings = await api.getServerSettings(id);
			gameSettingsEdits = Object.fromEntries(gameSettings.map((s) => [s.key, s.value]));
		} catch (err) {
			gameSettingsError = err instanceof Error ? err.message : String(err);
		} finally {
			gameSettingsLoading = false;
		}
	}

	async function saveGameSettings() {
		gameSettingsSaving = true;
		gameSettingsError = '';
		gameSettingsSaved = false;
		try {
			await api.setServerSettings(id, gameSettingsEdits);
			gameSettingsSaved = true;
			await loadGameSettings();
		} catch (err) {
			gameSettingsError = err instanceof Error ? err.message : String(err);
		} finally {
			gameSettingsSaving = false;
		}
	}

	// FR-4, scoped down to the only safe-to-automate case: re-download the
	// current build of the same loader for the same mc_version (see
	// handleReinstallLoader). There's no way to change the loader or
	// Minecraft version itself -- that risks breaking world/plugin
	// compatibility in ways CraftDeck can't safely automate, so it isn't
	// offered at all, by design.
	let reinstalling = $state(false);
	let reinstallError = $state('');
	let reinstallSuccess = $state(false);

	// Build picker (FR-4's build-selection extension) -- only meaningful for
	// loaders whose adapter implements BuildLister; see loadBuilds.
	let buildsLoaded = false;
	let buildOptions = $state<BuildInfo[]>([]);
	let buildsError = $state('');
	let selectedBuildVersion = $state('');

	async function loadBuilds() {
		if (!inst) return;
		try {
			buildOptions = await api.listLoaderBuilds(inst.loader, inst.mc_version);
			buildsError = '';
		} catch (err) {
			buildOptions = [];
			buildsError = err instanceof Error ? err.message : String(err);
		}
	}

	async function reinstallLoader() {
		reinstalling = true;
		reinstallError = '';
		reinstallSuccess = false;
		try {
			await api.reinstallLoader(id, selectedBuildVersion);
			reinstallSuccess = true;
		} catch (err) {
			reinstallError = err instanceof Error ? err.message : String(err);
		} finally {
			reinstalling = false;
		}
	}

	onMount(() => {
		refreshInstance();
		connectConsole();
		refreshPlayerList();
		refreshBans();
		refreshOps();
		refreshWhitelist();
		refreshBackups();
		refreshWorldInfo();
		refreshPlugins();
		refreshFiles();
		loadSystemResources();
		const poll = setInterval(refreshInstance, 5000);
		const playerPoll = setInterval(refreshPlayerList, 10000);
		const banOpPoll = setInterval(() => {
			refreshBans();
			refreshOps();
			refreshWhitelist();
		}, 15000);
		return () => {
			clearInterval(poll);
			clearInterval(playerPoll);
			clearInterval(banOpPoll);
		};
	});
	onDestroy(() => {
		consoleUnmounting = true;
		clearTimeout(consoleReconnectTimer);
		ws?.close();
	});

	async function start() {
		await api.startInstance(id);
		await refreshInstance();
	}
	async function stop() {
		await api.stopInstance(id);
		await refreshInstance();
	}
</script>

<main class="bg-background text-foreground flex flex-col p-8 lg:h-screen lg:overflow-hidden">
	<div class="flex items-center justify-between">
		<div>
			<a href="/" class="text-muted-foreground text-sm hover:underline"
				>{$t('instanceDetailPage.header.backToList')}</a
			>
			<h1 class="mt-1 text-2xl font-semibold">{inst?.name ?? id}</h1>
			{#if inst}
				<p class="text-muted-foreground text-xs">
					{$t('instanceDetailPage.header.statusLine', {
						loader: loaderLabel(inst.loader),
						version: inst.mc_version,
						status: statusLabel(inst.status)
					})}
					{#if inst.kind === 'proxy'}
						{$t('instanceDetailPage.header.connectPort', { port: inst.game_port })}
					{:else if subdomain && !subdomain.registered}
						{$t('instanceDetailPage.header.connectPort', { port: inst.game_port })}
					{/if}
				</p>
			{/if}
		</div>
		<div class="flex gap-2">
			<button class="border-border rounded-md border px-3 py-1.5 text-sm" onclick={start}
				>{$t('instanceDetailPage.buttons.start')}</button
			>
			<button
				class="border-border rounded-md border px-3 py-1.5 text-sm"
				disabled={restarting}
				onclick={restartForSettings}
			>
				{restarting
					? $t('instanceDetailPage.buttons.restarting')
					: $t('instanceDetailPage.buttons.restart')}
			</button>
			<button class="border-border rounded-md border px-3 py-1.5 text-sm" onclick={stop}
				>{$t('instanceDetailPage.buttons.stop')}</button
			>
		</div>
	</div>

	{#if loadError}
		<p class="text-destructive mt-4 text-sm">{loadError}</p>
	{/if}

	<div class="border-border mt-4 flex gap-1 border-b">
		<button
			class="border-b-2 px-3 py-2 text-sm {activeTab === 'console'
				? 'border-primary font-medium'
				: 'text-muted-foreground border-transparent'}"
			onclick={() => setActiveTab('console')}>{$t('instanceDetailPage.tabs.console')}</button
		>
		<button
			class="border-b-2 px-3 py-2 text-sm {activeTab === 'manage'
				? 'border-primary font-medium'
				: 'text-muted-foreground border-transparent'}"
			onclick={() => setActiveTab('manage')}>{$t('instanceDetailPage.tabs.manage')}</button
		>
		{#if inst && uploadCapableLoader(inst.loader)}
			<button
				class="border-b-2 px-3 py-2 text-sm {activeTab === 'plugins'
					? 'border-primary font-medium'
					: 'text-muted-foreground border-transparent'}"
				onclick={() => setActiveTab('plugins')}>{pluginTabLabel(inst?.loader)}</button
			>
		{/if}
		{#if inst && inst.kind === 'server'}
			<button
				class="border-b-2 px-3 py-2 text-sm {activeTab === 'files'
					? 'border-primary font-medium'
					: 'text-muted-foreground border-transparent'}"
				onclick={() => {
					setActiveTab('files');
					refreshFiles();
				}}>{$t('instanceDetailPage.tabs.files')}</button
			>
		{/if}
	</div>

	{#if activeTab === 'manage' && inst}
	<div class="mt-4 lg:min-h-0 lg:flex-1 lg:overflow-y-auto">
		<ManageTab
			{inst}
			{loaderLabel}
			{knownLoaders}
			{proxyCapableLoaders}
			{pendingRestart}
			{restarting}
			onOpenSettingsEdit={openSettingsEdit}
			onRestartForSettings={restartForSettings}
			{directlyReachable}
			{networkAddresses}
			{connectPort}
			{formatAddress}
			{domainConfig}
			{subdomain}
			{domainAddress}
			{domainAddressLabel}
			onOpenGameSettingsModal={openGameSettingsModal}
			{buildOptions}
			{buildsError}
			bind:selectedBuildVersion
			{reinstalling}
			{reinstallError}
			{reinstallSuccess}
			onReinstallLoader={reinstallLoader}
			{subdomainError}
			{domainSuffix}
			bind:subdomainInput
			{savingSubdomain}
			{registeringProxy}
			{unregisteringProxy}
			{proxyRegError}
			{registeredSecret}
			onRegisterBehindProxy={registerBehindProxy}
			onSaveSubdomain={saveSubdomain}
			onUnregisterFromProxy={unregisterFromProxy}
			{backups}
			{backupsError}
			{creatingBackup}
			{busyBackupId}
			onCreateBackup={createBackup}
			onRestoreBackup={restoreBackup}
			onDeleteBackup={deleteBackup}
			{worldInfo}
			{worldInfoError}
			{importFile}
			{importing}
			{importSuccess}
			{importError}
			{importForceConfirm}
			{onImportFileChange}
			onExportWorld={exportWorld}
			onImportWorld={importWorld}
		/>
	</div>
	{:else if activeTab === 'plugins' && inst && uploadCapableLoader(inst.loader)}
		<div class="mt-4 lg:min-h-0 lg:flex-1 lg:overflow-y-auto">
			<PluginsTab
				{inst}
				{pluginTabLabel}
				{searchCapableLoaders}
				{uploadingPlugin}
				{onPluginFileChange}
				{pluginsError}
				{plugins}
				{busyPluginId}
				onOpenPluginSearchModal={openPluginSearchModal}
				onTogglePlugin={togglePlugin}
				onDeletePlugin={deletePlugin}
			/>
		</div>
	{:else if activeTab === 'files' && inst}
		<div class="mt-4 lg:min-h-0 lg:flex-1 lg:overflow-y-auto">
			<FilesTab
				{uploadingFiles}
				{onFilePickerChange}
				{filesBreadcrumb}
				{navigateToPath}
				{navigateToBreadcrumb}
				{filesError}
				{onFilesDragOver}
				{onFilesDragLeave}
				{onFilesDrop}
				{isDraggingOverFiles}
				{loadingFiles}
				{fileEntries}
				{filesPath}
				{navigateUp}
				{renamingFile}
				bind:renameInput
				onConfirmRename={confirmRename}
				onCancelRename={cancelRename}
				onOpenEntry={openEntry}
				{formatFileSize}
				onDownloadEntry={downloadEntry}
				onStartRename={startRename}
				onDeleteEntry={deleteEntry}
				{editingFile}
				onCloseFileEditor={closeFileEditor}
				{loadingFileContent}
				bind:editingContent
				bind:fileContentSaved
				{fileContentError}
				{savingFileContent}
				onSaveFileContent={saveFileContent}
				{fileOpenError}
				fileOpenErrorName={fileOpenErrorEntry?.name ?? ''}
				onCloseFileOpenError={closeFileOpenError}
				onDownloadFileOpenError={() => fileOpenErrorEntry && downloadEntry(fileOpenErrorEntry)}
			/>
		</div>
	{:else if activeTab === 'console'}
	<div class="mt-6 grid grid-cols-1 gap-6 lg:min-h-0 lg:flex-1 lg:grid-cols-3">
		<ConsoleTab
			bind:logEl
			{inst}
			{wsStatus}
			{lines}
			{parseLogLine}
			bind:commandText
			onSubmitFreeform={submitFreeform}
			{onlinePlayers}
			onRefreshPlayerList={refreshPlayerList}
			bind:playerName
			onOpenReasonModal={openReasonModal}
			onPardonPlayer={pardonPlayer}
			onWhitelistAdd={whitelistAdd}
			onWhitelistRemove={whitelistRemove}
			onOpPlayer={opPlayer}
			onDeopPlayer={deopPlayer}
			{bannedPlayers}
			onRefreshBans={refreshBans}
			{ops}
			onRefreshOps={refreshOps}
			{whitelistedPlayers}
			{whitelistEnabled}
			onRefreshWhitelist={refreshWhitelist}
			onWhitelistToggle={whitelistToggle}
			bind:announceText
			onSendCommand={sendCommand}
			bind:gamemode
			bind:difficulty
		/>
	</div>
	{/if}
</main>

<ReasonModal
	{reasonModalKind}
	{playerName}
	bind:customReason
	onApply={applyReason}
	onClose={closeReasonModal}
/>

<PluginSearchModal
	bind:open={showPluginSearchModal}
	loaderLabel={pluginTabLabel(inst?.loader)}
	bind:query={pluginQuery}
	results={pluginSearchResults}
	error={pluginSearchError}
	searching={searchingPlugins}
	{installingProjectId}
	onSearch={searchPlugins}
	onInstall={installPlugin}
/>

<GameSettingsModal
	open={showGameSettingsModal}
	settings={gameSettings}
	bind:edits={gameSettingsEdits}
	loading={gameSettingsLoading}
	error={gameSettingsError}
	saving={gameSettingsSaving}
	saved={gameSettingsSaved}
	onSave={saveGameSettings}
	onClose={closeGameSettingsModal}
/>

{#if inst}
	<ServerSettingsModal
		open={editingSettings}
		{inst}
		bind:settingsMemoryGB
		bind:settingsCpu
		bind:settingsGamePort
		{canEditGamePort}
		{maxMemoryGB}
		{ramBoundaryGB}
		{settingsError}
		{settingsSaving}
		onSave={saveSettings}
		onClose={cancelSettingsEdit}
	/>
{/if}

<ConfirmDialog bind:open={confirmOpen} message={confirmMessage} onconfirm={confirmAction} />

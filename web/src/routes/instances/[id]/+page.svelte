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
		type DomainConfig
	} from '$lib/api';
	import { onDestroy, onMount, tick } from 'svelte';

	const id = $page.params.id as string; // always present: this route only matches with an id segment

	// The 설정/백업/월드데이터/플러그인 cards used to all stack above the
	// console, so as backups/plugins piled up the console (the thing checked
	// most often) kept getting pushed further down the page. Splitting them
	// into tabs means the console tab's height/layout is never affected by
	// how much content the other tabs have.
	let activeTab = $state<'console' | 'manage' | 'plugins' | 'files'>('console');

	let inst = $state<Instance | null>(null);
	let loadError = $state('');
	let lines = $state<string[]>([]);
	let commandText = $state('');
	let logEl: HTMLDivElement;
	let ws: WebSocket | null = null;
	let wsStatus = $state<'connecting' | 'open' | 'closed'>('connecting');

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
	const reasonPresets = ['비매너/욕설', '핵/치트 사용', '광고/스팸 행위', '규칙 위반', '사유 없음'];
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

	// Display-only Korean labels for the GUI settings form's enum options
	// (FR-12) -- server.properties itself always stores the raw English
	// value (e.g. "easy", "survival"), matching Minecraft's own file format
	// and what handleSetServerSettings validates against, so only the
	// rendered <option> text changes here.
	const enumOptionLabels: Record<string, Record<string, string>> = {
		difficulty: { peaceful: '평화로움', easy: '쉬움', normal: '보통', hard: '어려움' },
		gamemode: { survival: '서바이벌', creative: '크리에이티브', adventure: '어드벤처', spectator: '관전자' }
	};
	function enumOptionLabel(settingKey: string, value: string) {
		return enumOptionLabels[settingKey]?.[value] ?? value;
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
		return loader === 'fabric' || loader === 'neoforge' ? '모드' : '플러그인';
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
		if (line.startsWith('[오류]')) {
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
		ws = new WebSocket(api.consoleURL(id));
		wsStatus = 'connecting';
		ws.onopen = () => (wsStatus = 'open');
		ws.onclose = () => (wsStatus = 'closed');
		ws.onerror = () => (wsStatus = 'closed');
		ws.onmessage = (event) => {
			const frame = JSON.parse(event.data);
			if (frame.type === 'log') {
				appendLine(frame.line);
			} else if (frame.type === 'cmd_result') {
				appendLine(`> ${frame.command}`);
				appendLine(frame.ok ? frame.line : `[오류] ${frame.error}`);
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
				.catch((err) => appendLine(`[오류] ${err instanceof Error ? err.message : err}`));
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

	// CPU/memory settings (FR-12). game_port is auto-assigned once at
	// creation and never surfaced here -- see nextFreeGamePort on the
	// backend. Editable even while the instance is running -- CPU/memory
	// limits are only ever applied to a fresh process, so a save just writes
	// the new values without touching the running unit. They take effect
	// once the operator explicitly restarts (pendingRestart flags that).
	let editingSettings = $state(false);
	let settingsCpu = $state(0); // percent, 0 = unlimited
	let settingsMemoryGB = $state(1);
	let settingsError = $state('');
	let settingsSaving = $state(false);
	let pendingRestart = $state(false);
	let restarting = $state(false);
	// Snapshot of the CPU/memory values actually in effect on the
	// currently-running process, so we can tell a real pending change (needs
	// a restart) apart from the operator editing settings and then putting
	// them back to what's already running -- in which case the restart
	// button should disappear again rather than stay stuck on.
	let appliedCpu = 0;
	let appliedMemoryMB = 0;
	let appliedInitialized = false;

	function computePendingRestart() {
		if (!inst || (inst.status !== 'running' && inst.status !== 'starting')) {
			pendingRestart = false;
			return;
		}
		pendingRestart =
			inst.cpu_quota_percent !== appliedCpu || inst.memory_max_mb !== appliedMemoryMB;
	}
	// Raspberry Pi's total RAM in GB, used to cap the memory slider -- filled
	// in from /api/system/resources on mount; 1 is just a safe placeholder
	// until that responds.
	let maxMemoryGB = $state(1);

	async function loadSystemResources() {
		try {
			const res = await api.systemResources();
			// The always-on Velocity proxy has a fixed 1GB allocation (see
			// PROXY_RESERVED_MEMORY_MB) that this server's slider shouldn't be
			// able to eat into.
			maxMemoryGB = Math.max(1, Math.floor((res.total_memory_mb - PROXY_RESERVED_MEMORY_MB) / 1024));
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
				memory_max_mb: settingsMemoryGB * 1024
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
		editingFile = entry.path;
		editingContent = '';
		fileContentError = '';
		fileContentSaved = false;
		loadingFileContent = true;
		try {
			const res = await api.getFileContent(id, entry.path);
			editingContent = res.content;
		} catch (err) {
			fileContentError = err instanceof Error ? err.message : String(err);
		} finally {
			loadingFileContent = false;
		}
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

	async function deleteEntry(entry: FileEntry) {
		const label = entry.is_dir ? '폴더(안의 모든 내용 포함)' : '파일';
		if (!confirm(`이 ${label}을(를) 삭제할까요? 되돌릴 수 없습니다.\n\n${entry.path}`)) return;
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

	async function restoreBackup(backupId: string) {
		if (
			!confirm('이 백업으로 복원하면 현재 월드/설정이 백업 시점 상태로 전부 대체됩니다. 계속할까요?')
		) {
			return;
		}
		busyBackupId = backupId;
		try {
			await api.restoreBackup(id, backupId);
		} catch (err) {
			backupsError = err instanceof Error ? err.message : String(err);
		} finally {
			busyBackupId = null;
		}
	}

	async function deleteBackup(backupId: string) {
		if (!confirm('이 백업을 삭제할까요?')) return;
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

	function formatBytes(bytes: number) {
		return `${(bytes / 1024 / 1024).toFixed(1)}MB`;
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
			importSuccess = `가져오기 완료 (감지된 버전: ${result.detected_version || '알 수 없음'}). 서버를 시작하면 반영됩니다.`;
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

	async function deletePlugin(p: Plugin) {
		if (!confirm(`${p.filename}을(를) 삭제할까요?`)) return;
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

	// ".apple-farm.online" (leading dot) when a main domain is registered,
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
	let copiedAddress = $state('');

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
		domainConfig?.kind === 'free_subdomain' ? 'DuckDNS/ipTime 주소' : '도메인'
	);

	async function loadNetworkAddresses() {
		try {
			networkAddresses = await api.getNetworkAddresses();
		} catch {
			// non-critical -- section just won't show an address
		}
	}

	function copyAddress(address: string) {
		navigator.clipboard.writeText(address).then(() => {
			copiedAddress = address;
			setTimeout(() => {
				if (copiedAddress === address) copiedAddress = '';
			}, 1500);
		});
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

	async function unregisterFromProxy() {
		if (
			!confirm(
				'이 서버를 프록시에서 빼고 독립 노출로 전환할까요? 서버를 재시작해야 실제로 적용됩니다.'
			)
		) {
			return;
		}
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
	onDestroy(() => ws?.close());

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
			<a href="/" class="text-muted-foreground text-sm hover:underline">&larr; 목록으로</a>
			<h1 class="mt-1 text-2xl font-semibold">{inst?.name ?? id}</h1>
			{#if inst}
				<p class="text-muted-foreground text-xs">
					{loaderLabel(inst.loader)} · {inst.mc_version} · 상태 {inst.status}
					{#if inst.kind === 'proxy'}
						· 접속 포트 {inst.game_port}
					{:else if subdomain && !subdomain.registered}
						· 접속 포트 {inst.game_port}
					{/if}
				</p>
			{/if}
		</div>
		<div class="flex gap-2">
			<button class="border-border rounded-md border px-3 py-1.5 text-sm" onclick={start}
				>시작</button
			>
			<button
				class="border-border rounded-md border px-3 py-1.5 text-sm"
				disabled={restarting}
				onclick={restartForSettings}
			>
				{restarting ? '재시작 중...' : '재시작'}
			</button>
			<button class="border-border rounded-md border px-3 py-1.5 text-sm" onclick={stop}
				>종료</button
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
			onclick={() => (activeTab = 'console')}>콘솔</button
		>
		<button
			class="border-b-2 px-3 py-2 text-sm {activeTab === 'manage'
				? 'border-primary font-medium'
				: 'text-muted-foreground border-transparent'}"
			onclick={() => (activeTab = 'manage')}>서버 관리</button
		>
		{#if inst && uploadCapableLoader(inst.loader)}
			<button
				class="border-b-2 px-3 py-2 text-sm {activeTab === 'plugins'
					? 'border-primary font-medium'
					: 'text-muted-foreground border-transparent'}"
				onclick={() => (activeTab = 'plugins')}>{pluginTabLabel(inst?.loader)}</button
			>
		{/if}
		{#if inst && inst.kind === 'server'}
			<button
				class="border-b-2 px-3 py-2 text-sm {activeTab === 'files'
					? 'border-primary font-medium'
					: 'text-muted-foreground border-transparent'}"
				onclick={() => {
					activeTab = 'files';
					refreshFiles();
				}}>파일</button
			>
		{/if}
	</div>

	{#if activeTab === 'manage' && inst}
	{@const canBackup = inst.status === 'stopped' || inst.status === 'crashed'}
	<div class="mt-4 lg:min-h-0 lg:flex-1 lg:overflow-y-auto">
		<div class="border-border bg-card rounded-lg border p-4">
			<div class="flex items-center justify-between">
				<h2 class="font-medium">서버 설정</h2>
				{#if !editingSettings}
					<button
						class="border-border rounded-md border px-3 py-1.5 text-xs"
						onclick={openSettingsEdit}>설정 변경</button
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
						onclick={restartForSettings}>{restarting ? '재시작 중...' : '재시작'}</button
					>
				</div>
			{/if}

			{#if editingSettings}
				<div class="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
					{#if inst?.kind === 'proxy'}
						<div>
							<span class="text-muted-foreground mb-1 block text-xs">메모리 할당</span>
							<p class="mt-1.5 text-sm">1GB (고정)</p>
						</div>
					{:else}
						<div>
							<label class="text-muted-foreground mb-1 block text-xs" for="settings-memory">
								메모리 할당 ({settingsMemoryGB}GB / 최대 {maxMemoryGB}GB)
							</label>
							<input
								id="settings-memory"
								type="range"
								min="1"
								max={maxMemoryGB}
								step="1"
								bind:value={settingsMemoryGB}
								class="w-full"
							/>
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
						onclick={saveSettings}>저장</button
					>
					<button
						class="border-border rounded-md border px-3 py-1.5 text-xs"
						onclick={cancelSettingsEdit}>취소</button
					>
				</div>
			{:else}
				<p class="text-muted-foreground mt-2 text-xs">
					메모리 할당 {inst.memory_max_mb > 0
						? `${(inst.memory_max_mb / 1024).toFixed(1)}GB`
						: '무제한'} · CPU 할당 {inst.cpu_quota_percent > 0
						? `${inst.cpu_quota_percent}%`
						: '무제한'}
				</p>
			{/if}
		</div>

		<!-- 접속 주소 복사 버튼 -- 프록시 인스턴스, 독립 노출된 서버, 그리고
			이제 프록시에 등록된 서버(=이 서버 자신의 포트가 아니라 프록시의
			포트로 접속해야 함, connectPort 참고)에도 표시. 공인 IP는 외부
			접속이 켜져 있을 때만 백엔드가 값을 채워 보낸다. -->
		{#if inst && directlyReachable && networkAddresses}
			{@const port = connectPort}
			{@const localAddress = formatAddress(networkAddresses.local_ip, port)}
			{@const publicAddress =
				networkAddresses.public_ip && !domainConfig
					? formatAddress(networkAddresses.public_ip, port)
					: ''}
			<div class="border-border bg-card mt-4 rounded-lg border p-4">
				<h2 class="font-medium">접속 주소</h2>
				{#if inst.kind === 'server' && subdomain?.registered}
					<p class="text-muted-foreground mt-1 text-xs">
						이 서버는 프록시 뒤에 있어 프록시의 포트로 접속합니다. 서브도메인이 지정되어
						있으면 그 주소로, 아니면 프록시의 우선순위에 따라 다른 서버로 연결될 수 있습니다.
					</p>
				{/if}
				<div class="mt-2 space-y-2">
					{#if domainAddress}
						<div class="flex items-center justify-between gap-2">
							<div class="min-w-0">
								<p class="text-muted-foreground text-xs">{domainAddressLabel}</p>
								<code class="text-sm">{domainAddress}</code>
							</div>
							<button
								class="border-border shrink-0 rounded-md border px-2 py-1 text-xs"
								onclick={() => copyAddress(domainAddress)}
							>
								{copiedAddress === domainAddress ? '복사됨' : '복사'}
							</button>
						</div>
					{/if}
					<div class="flex items-center justify-between gap-2">
						<div class="min-w-0">
							<p class="text-muted-foreground text-xs">사설 IP (같은 네트워크에서)</p>
							<code class="text-sm">{localAddress}</code>
						</div>
						<button
							class="border-border shrink-0 rounded-md border px-2 py-1 text-xs"
							onclick={() => copyAddress(localAddress)}
						>
							{copiedAddress === localAddress ? '복사됨' : '복사'}
						</button>
					</div>
					{#if publicAddress}
						<div class="flex items-center justify-between gap-2">
							<div class="min-w-0">
								<p class="text-muted-foreground text-xs">공인 IP (외부에서)</p>
								<code class="text-sm">{publicAddress}</code>
							</div>
							<button
								class="border-border shrink-0 rounded-md border px-2 py-1 text-xs"
								onclick={() => copyAddress(publicAddress)}
							>
								{copiedAddress === publicAddress ? '복사됨' : '복사'}
							</button>
						</div>
					{:else if domainConfig}
						<p class="text-muted-foreground text-xs">
							도메인이 연결되어 있어 공인 IP 대신 위 주소를 사용하세요.
						</p>
					{:else}
						<p class="text-muted-foreground text-xs">
							외부 접속이 꺼져 있어 공인 IP 주소는 표시하지 않습니다.
						</p>
					{/if}
				</div>
			</div>
		{/if}

		<!-- server.properties GUI form (FR-12) -- a curated, labeled subset;
			anything not listed here is still reachable via the general file
			manager's raw editing (FR-12a), which is aimed at advanced/custom-
			loader use rather than everyday tuning. Opens in a modal (see
			showGameSettingsModal below) since the full form is long -- keeping
			it inline here would dominate the page. -->
		{#if inst.kind === 'server'}
			<div class="border-border bg-card mt-4 flex items-center justify-between rounded-lg border p-4">
				<div>
					<h2 class="font-medium">게임플레이 설정</h2>
					<p class="text-muted-foreground mt-1 text-xs">
						난이도, 게임 모드, 최대 인원 등 자주 쓰는 <code>server.properties</code> 옵션
					</p>
				</div>
				<button
					class="border-border shrink-0 rounded-md border px-3 py-1.5 text-xs"
					onclick={openGameSettingsModal}>열기</button
				>
			</div>
		{/if}

		<!-- Loader reinstall (FR-4, scoped to same loader + same mc_version --
			see handleReinstallLoader's doc comment for why nothing broader is
			offered here). -->
		<div class="border-border bg-card mt-4 rounded-lg border p-4">
			<h2 class="font-medium">구동기</h2>
			<p class="text-muted-foreground mt-1 text-xs">
				{loaderLabel(inst.loader)} · {inst.mc_version}
			</p>
			{#if knownLoaders.includes(inst.loader)}
				<p class="text-muted-foreground mt-1 text-xs">
					같은 구동기·같은 마인크래프트 버전 안에서만 빌드를 다시 받습니다. 다른 구동기나
					버전으로 바꾸는 기능은 월드/플러그인 호환성이 깨질 수 있어 제공하지 않습니다.
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
								<option value={build.id}>
									{build.id}{build.channel ? ` (${build.channel})` : ''}
								</option>
							{/each}
						</select>
					</div>
				{:else if buildsError}
					<p class="text-muted-foreground mt-1 text-xs">
						빌드 목록을 불러오지 못했습니다: {buildsError}
					</p>
				{/if}
				<button
					class="border-border mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
					disabled={reinstalling || !canBackup}
					title={canBackup ? '' : '먼저 서버를 종료하세요'}
					onclick={reinstallLoader}
				>
					{reinstalling
						? '재설치 중...'
						: selectedBuildVersion
							? `빌드 ${selectedBuildVersion}로 재설치`
							: '최신 빌드로 재설치'}
				</button>
			{:else}
				<p class="text-muted-foreground mt-1 text-xs">
					커스텀 구동기입니다. 새 jar로 교체하려면 파일 탭에서 <code>server.jar</code>를 직접
					업로드하세요.
				</p>
			{/if}
			{#if reinstallError}
				<p class="text-destructive mt-2 text-xs">{reinstallError}</p>
			{/if}
			{#if reinstallSuccess}
				<p class="mt-2 text-xs text-green-500">재설치됐습니다. 다시 시작하면 적용됩니다.</p>
			{/if}
		</div>

		<!-- Proxy registration -- the operator's one actual proxy-related
			setting, now that the always-on Velocity proxy itself has no UI of
			its own (see ensureProxyInstance/proxyMemoryMaxMB). Shown for every
			server, not just the loaders CraftDeck auto-registers -- a custom
			loader (FR-3) can still be added manually below.

			Per FR-1f, Velocity only exists at all when an owned main domain is
			registered -- with only a free-subdomain DDNS (or nothing) registered,
			there's no proxy to register into, so this card is replaced with a
			one-line explanation instead of showing controls that would just
			error out. -->
		{#if domainConfig?.kind === 'main_domain'}
		<div class="border-border bg-card mt-4 rounded-lg border p-4">
			<h2 class="font-medium">프록시</h2>
			{#if inst.loader === 'fabric' || inst.loader === 'neoforge'}
				<p class="mt-1 text-xs text-yellow-500">
					⚠ 일부 모드(엔티티·블록 상태 등 바닐라 패킷 구조 자체를 변형하는 모드 -- 예:
					Create)는 Velocity와 호환되지 않아 접속 중 "A packet did not decode
					successfully" 오류로 끊길 수 있습니다. 이런 모드를 쓴다면 프록시 등록 대신
					독립 노출을 사용하세요.
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
					<strong>직접 설정되어 있어야 합니다</strong> -- CraftDeck은 임의의 구동기 jar가 이걸
					지원하는지 확인할 방법이 없습니다. 잘못 설정된 채로 등록하면 접속이 실패합니다.
				</p>
				<button
					class="border-border mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
					disabled={registeringProxy || !canBackup}
					title={canBackup ? '' : '먼저 서버를 종료하세요'}
					onclick={registerBehindProxy}
				>
					{registeringProxy ? '등록 중...' : '프록시에 수동으로 등록'}
				</button>
				{#if proxyRegError}
					<p class="text-destructive mt-2 text-xs">{proxyRegError}</p>
				{/if}
				{#if registeredSecret}
					<div class="border-border bg-background mt-2 rounded-md border p-2">
						<p class="text-muted-foreground text-xs">
							등록됐습니다. 아래 시크릿을 이 서버의 forwarding 설정(로더에 따라 다름 -- 파일
							탭에서 직접 편집)에 붙여넣고, <code>server-ip</code>/<code>online-mode</code>는
							이미 자동으로 반영했습니다. 재시작 후 적용됩니다.
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
					다른 서버에도 같은 서브도메인을 지정하면, 먼저 만든 서버가 우선순위 1순위가 되어
					평소엔 그쪽으로 연결되고, 그 서버가 다운되면 다음 순위 서버로 자동 장애조치됩니다
					(복구되면 새 접속부터 다시 1순위로 자동 복귀).
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
						disabled={savingSubdomain ||
							subdomainInput.trim() === labelFromForcedHost(subdomain.forced_host)}
						onclick={saveSubdomain}
					>
						{savingSubdomain ? '저장 중...' : '저장'}
					</button>
				</div>
				<button
					class="border-border text-destructive mt-2 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
					disabled={unregisteringProxy || !canBackup}
					title={canBackup ? '' : '먼저 서버를 종료하세요'}
					onclick={unregisterFromProxy}
				>
					{unregisteringProxy ? '전환 중...' : '독립 노출로 전환'}
				</button>
				{#if proxyRegError}
					<p class="text-destructive mt-2 text-xs">{proxyRegError}</p>
				{/if}
			{/if}
		</div>
		{:else}
		<div class="border-border bg-card mt-4 rounded-lg border p-4">
			<h2 class="font-medium">프록시</h2>
			<p class="text-muted-foreground mt-1 text-xs">
				소유한 메인 도메인이 연결되어 있어야 프록시(Velocity)가 동작합니다. 무료 DDNS
				서브도메인만 등록했거나 도메인이 없는 경우, 이 서버는 독립 노출(자신의 게임 포트로 직접
				접속)로만 운영됩니다.
			</p>
		</div>
		{/if}

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
					onclick={createBackup}
				>
					{creatingBackup ? '생성 중...' : '백업 생성'}
				</button>
			</div>
			{#if !canBackup}
				<p class="text-muted-foreground mt-1 text-xs">
					백업 생성/복원은 서버가 정지된 상태에서만 가능합니다.
				</p>
			{/if}
			{#if backupsError}
				<p class="text-destructive mt-2 text-xs">{backupsError}</p>
			{/if}
			{#if backups.length === 0}
				<p class="text-muted-foreground mt-2 text-xs">백업이 아직 없습니다.</p>
			{:else}
				<div class="mt-2 space-y-1.5">
					{#each backups as b (b.id)}
						<div
							class="border-border flex items-center justify-between rounded-md border px-2 py-1.5 text-xs"
						>
							<span>{b.filename} · {formatBytes(b.size_bytes)}</span>
							<div class="flex gap-1.5">
								<button
									class="border-border rounded-md border px-2 py-1 text-xs"
									disabled={!canBackup || busyBackupId === b.id}
									onclick={() => restoreBackup(b.id)}>복원</button
								>
								<button
									class="border-border text-destructive rounded-md border px-2 py-1 text-xs"
									disabled={busyBackupId === b.id}
									onclick={() => deleteBackup(b.id)}>삭제</button
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
						onclick={exportWorld}>월드 데이터 다운로드</button
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
							onclick={() => importWorld(false)}
						>
							{importing ? '가져오는 중...' : '가져오기'}
						</button>
					</div>
				</div>
			</div>
			{#if !canBackup}
				<p class="text-muted-foreground mt-1 text-xs">
					내보내기/가져오기는 서버가 정지된 상태에서만 가능합니다.
				</p>
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
						onclick={() => importWorld(true)}>그래도 강제 적용</button
					>
				{/if}
			{/if}
		</div>
		</div>

		{/if}
	</div>
	{:else if activeTab === 'plugins' && inst && uploadCapableLoader(inst.loader)}
		<div class="mt-4 lg:min-h-0 lg:flex-1 lg:overflow-y-auto">
			<div class="border-border bg-card rounded-lg border p-4">
				<div class="flex items-center justify-between">
					<h2 class="font-medium">{pluginTabLabel(inst.loader)}</h2>
					{#if searchCapableLoaders.includes(inst.loader)}
						<button
							class="border-border rounded-md border px-3 py-1.5 text-xs"
							onclick={openPluginSearchModal}>Modrinth에서 검색</button
						>
					{/if}
				</div>
				<p class="text-muted-foreground mt-1 text-xs">
					설치/삭제/활성화 변경 후에는 서버를 재시작해야 반영됩니다.
				</p>

				<div class="mt-4">
					<span class="text-muted-foreground mb-1 block text-xs">직접 업로드 (.jar)</span>
					<input
						type="file"
						accept=".jar"
						disabled={uploadingPlugin}
						onchange={onPluginFileChange}
						class="text-muted-foreground file:border-border file:bg-background file:text-foreground file:mr-2 file:rounded-md file:border file:px-3 file:py-1.5 file:text-xs file:font-medium file:cursor-pointer text-xs"
					/>
					{#if uploadingPlugin}
						<span class="text-muted-foreground ml-2 text-xs">업로드 중...</span>
					{/if}
				</div>

				{#if pluginsError}
					<p class="text-destructive mt-2 text-xs">{pluginsError}</p>
				{/if}
				<div class="mt-3">
					<span class="text-muted-foreground mb-1 block text-xs"
						>설치된 {pluginTabLabel(inst.loader)}</span
					>
					{#if plugins.length === 0}
						<p class="text-muted-foreground text-xs">
							설치된 {pluginTabLabel(inst.loader)} 목록이 비어 있습니다.
						</p>
					{:else}
						<div class="space-y-1.5">
							{#each plugins as p (p.id)}
								<div
									class="border-border flex items-center justify-between rounded-md border px-2 py-1.5 text-xs"
								>
									<span>
										{p.filename}
										{#if !p.enabled}<span class="text-muted-foreground">(비활성화됨)</span
											>{/if}
										{#if p.installed_as_dependency}<span class="text-muted-foreground"
												>(의존성으로 자동 설치됨)</span
											>{/if}
									</span>
									<div class="flex shrink-0 gap-1.5">
										<button
											class="border-border rounded-md border px-2 py-1 text-xs"
											disabled={busyPluginId === p.id}
											onclick={() => togglePlugin(p)}
										>
											{p.enabled ? '비활성화' : '활성화'}
										</button>
										<button
											class="border-border text-destructive rounded-md border px-2 py-1 text-xs"
											disabled={busyPluginId === p.id}
											onclick={() => deletePlugin(p)}>삭제</button
										>
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</div>
			</div>
		</div>
	{:else if activeTab === 'files' && inst}
		<div class="mt-4 lg:min-h-0 lg:flex-1 lg:overflow-y-auto">
			<div class="border-border bg-card rounded-lg border p-4">
				<div class="flex items-center justify-between">
					<h2 class="font-medium">파일</h2>
					<label
						class="border-border cursor-pointer rounded-md border px-3 py-1.5 text-xs {uploadingFiles
							? 'opacity-50'
							: ''}"
					>
						{uploadingFiles ? '업로드 중...' : '업로드'}
						<input type="file" multiple class="hidden" disabled={uploadingFiles} onchange={onFilePickerChange} />
					</label>
				</div>

				<!-- Breadcrumb -->
				<div class="text-muted-foreground mt-2 flex flex-wrap items-center gap-1 text-xs">
					<button type="button" class="underline" onclick={() => navigateToPath('')}>루트</button>
					{#each filesBreadcrumb() as segment, i}
						<span>/</span>
						<button type="button" class="underline" onclick={() => navigateToBreadcrumb(i)}
							>{segment}</button
						>
					{/each}
				</div>

				{#if filesError}
					<p class="text-destructive mt-2 text-xs">{filesError}</p>
				{/if}

				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div
					ondragover={onFilesDragOver}
					ondragleave={onFilesDragLeave}
					ondrop={onFilesDrop}
					class="mt-2 rounded-md border {isDraggingOverFiles
						? 'border-primary bg-primary/5'
						: 'border-border'}"
				>
					{#if loadingFiles}
						<p class="text-muted-foreground p-3 text-xs">불러오는 중...</p>
					{:else if fileEntries.length === 0}
						<p class="text-muted-foreground p-3 text-xs">
							빈 폴더입니다. 파일을 여기로 드래그해서 업로드할 수 있습니다.
						</p>
					{:else}
						<div class="divide-border divide-y">
							{#if filesPath}
								<!-- svelte-ignore a11y_click_events_have_key_events -->
								<!-- svelte-ignore a11y_no_static_element_interactions -->
								<div
									class="hover:bg-background/50 flex cursor-pointer items-center gap-2 px-3 py-2 text-sm"
									ondblclick={navigateUp}
									onclick={navigateUp}
								>
									<span>📁</span>
									<span class="text-muted-foreground">..</span>
								</div>
							{/if}
							{#each fileEntries as entry (entry.path)}
								{#if renamingFile === entry.path}
									<div class="flex items-center gap-2 px-3 py-2 text-sm">
										<span>{entry.is_dir ? '📁' : '📄'}</span>
										<input
											type="text"
											bind:value={renameInput}
											class="border-input bg-background min-w-0 flex-1 rounded-md border px-2 py-1 text-sm"
										/>
										<button
											class="bg-primary text-primary-foreground shrink-0 rounded-md px-2 py-1 text-xs"
											onclick={confirmRename}>저장</button
										>
										<button
											class="border-border shrink-0 rounded-md border px-2 py-1 text-xs"
											onclick={cancelRename}>취소</button
										>
									</div>
								{:else}
									<!-- svelte-ignore a11y_click_events_have_key_events -->
									<!-- svelte-ignore a11y_no_static_element_interactions -->
									<div
										class="hover:bg-background/50 flex items-center gap-2 px-3 py-2 text-sm"
										ondblclick={() => openEntry(entry)}
									>
										<span class="cursor-pointer" onclick={() => openEntry(entry)}
											>{entry.is_dir ? '📁' : '📄'}</span
										>
										<span
											class="min-w-0 flex-1 cursor-pointer truncate"
											onclick={() => openEntry(entry)}>{entry.name}</span
										>
										{#if !entry.is_dir}
											<span class="text-muted-foreground shrink-0 text-xs"
												>{formatFileSize(entry.size)}</span
											>
										{/if}
										<div class="flex shrink-0 gap-1">
											<button
												class="border-border rounded-md border px-2 py-1 text-xs"
												onclick={() => downloadEntry(entry)}
												>{entry.is_dir ? '다운로드 (zip)' : '다운로드'}</button
											>
											<button
												class="border-border rounded-md border px-2 py-1 text-xs"
												onclick={() => startRename(entry)}>이름변경</button
											>
											<button
												class="border-border text-destructive rounded-md border px-2 py-1 text-xs"
												onclick={() => deleteEntry(entry)}>삭제</button
											>
										</div>
									</div>
								{/if}
							{/each}
						</div>
					{/if}
				</div>
			</div>
		</div>

		{#if editingFile}
			<!-- svelte-ignore a11y_click_events_have_key_events -->
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<div
				class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-8"
				onclick={closeFileEditor}
			>
				<!-- svelte-ignore a11y_click_events_have_key_events -->
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div
					class="bg-card border-border flex max-h-[80vh] w-full max-w-2xl flex-col rounded-lg border p-4 shadow-lg"
					onclick={(e) => e.stopPropagation()}
				>
					<div class="mb-2 flex items-center justify-between">
						<h2 class="truncate font-medium">{editingFile}</h2>
						<button type="button" class="text-muted-foreground text-sm" onclick={closeFileEditor}
							>&times;</button
						>
					</div>
					{#if loadingFileContent}
						<p class="text-muted-foreground text-xs">불러오는 중...</p>
					{:else}
						<textarea
							bind:value={editingContent}
							oninput={() => (fileContentSaved = false)}
							rows="20"
							spellcheck="false"
							class="border-input bg-background w-full flex-1 rounded-md border p-2 font-mono text-xs"
						></textarea>
						{#if fileContentError}
							<p class="text-destructive mt-2 text-xs">{fileContentError}</p>
						{/if}
						<div class="mt-2 flex items-center gap-2">
							<button
								class="bg-primary text-primary-foreground rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50"
								disabled={savingFileContent}
								onclick={saveFileContent}
							>
								{savingFileContent ? '저장 중...' : '저장'}
							</button>
							{#if fileContentSaved}
								<span class="text-muted-foreground text-xs">저장됨 · 재시작해야 반영됩니다</span>
							{/if}
						</div>
					{/if}
				</div>
			</div>
		{/if}
	{:else if activeTab === 'console'}
	<div class="mt-6 grid grid-cols-1 gap-6 lg:min-h-0 lg:flex-1 lg:grid-cols-3">
		<!-- Live console: FR-14, FR-15, FR-20 -->
		<div
			class="border-border bg-card rounded-lg border p-4 lg:flex lg:min-h-0 lg:flex-col {inst?.kind ===
			'proxy'
				? 'lg:col-span-3'
				: 'lg:col-span-2'}"
		>
			<div class="mb-2 flex items-center justify-between">
				<h2 class="font-medium">실시간 콘솔</h2>
				<span class="text-muted-foreground text-xs">
					{wsStatus === 'open' ? '연결됨' : wsStatus === 'connecting' ? '연결 중...' : '연결 끊김'}
				</span>
			</div>
			<div
				bind:this={logEl}
				class="bg-background h-96 overflow-y-auto rounded-md p-3 font-mono text-xs lg:h-auto lg:min-h-0 lg:flex-1"
			>
				{#each lines as line}
					{@const parsed = parseLogLine(line)}
					<div class="whitespace-pre-wrap">
						{#if parsed.prefix}<span class="text-muted-foreground">{parsed.prefix}</span>
						{/if}<span class={parsed.messageClass}>{parsed.message}</span>
					</div>
				{/each}
			</div>
			<form class="mt-3 flex gap-2" onsubmit={submitFreeform}>
				<input
					bind:value={commandText}
					placeholder="명령어 직접 입력 (예: say hello)"
					class="border-input bg-background flex-1 rounded-md border px-3 py-2 font-mono text-sm"
				/>
				<button
					type="submit"
					class="bg-primary text-primary-foreground rounded-md px-4 py-2 text-sm font-medium"
					>전송</button
				>
			</form>
		</div>

		<!-- GUI command buttons: FR-17, FR-18, FR-19, FR-20. Velocity has no
			RCON in this MVP, so none of these apply to a proxy instance. -->
		{#if inst?.kind === 'server'}
		<div
			class="border-border bg-card space-y-4 rounded-lg border p-4 lg:min-h-0 lg:overflow-y-auto"
		>
			<h2 class="font-medium">자주 쓰는 명령</h2>

			<div class="flex gap-2">
				<button
					class="border-border flex-1 rounded-md border px-3 py-1.5 text-sm"
					onclick={() => sendCommand('save-all')}>월드 저장</button
				>
			</div>

			<div>
				<div class="mb-1 flex items-center justify-between">
					<label class="text-muted-foreground block text-xs" for="player">플레이어</label>
					<button
						type="button"
						class="text-muted-foreground text-xs underline"
						onclick={refreshPlayerList}>새로고침</button
					>
				</div>
				{#if onlinePlayers.length > 0}
					<div class="mb-2 flex flex-wrap gap-1.5">
						{#each onlinePlayers as p}
							<button
								type="button"
								class="border-border rounded-full border px-2 py-0.5 text-xs {playerName === p
									? 'bg-primary text-primary-foreground'
									: ''}"
								onclick={() => (playerName = p)}
							>
								{p}
							</button>
						{/each}
					</div>
				{:else}
					<p class="text-muted-foreground mb-2 text-xs">현재 접속 중인 플레이어가 없습니다.</p>
				{/if}
				<div class="flex gap-2">
					<input
						id="player"
						bind:value={playerName}
						placeholder="닉네임 (위에서 선택하거나 직접 입력)"
						class="border-input bg-background w-full min-w-0 flex-1 rounded-md border px-2 py-1.5 text-sm"
					/>
				</div>
				<div class="mt-2 grid grid-cols-2 gap-2">
					<button
						class="border-border col-span-2 rounded-md border px-2 py-1.5 text-xs"
						disabled={!playerName}
						onclick={() => openReasonModal('kick')}>강제 퇴장</button
					>
					<button
						class="border-border rounded-md border px-2 py-1.5 text-xs"
						disabled={!playerName}
						onclick={() => openReasonModal('ban')}>밴</button
					>
					<button
						class="border-border rounded-md border px-2 py-1.5 text-xs"
						onclick={pardonPlayer}>밴 해제</button
					>
					<button
						class="border-border rounded-md border px-2 py-1.5 text-xs"
						onclick={whitelistAdd}>화이트리스트 추가</button
					>
					<button
						class="border-border rounded-md border px-2 py-1.5 text-xs"
						onclick={whitelistRemove}>화이트리스트 삭제</button
					>
					<button
						class="border-border rounded-md border px-2 py-1.5 text-xs"
						onclick={opPlayer}>운영자 부여</button
					>
					<button
						class="border-border rounded-md border px-2 py-1.5 text-xs"
						onclick={deopPlayer}>운영자 해제</button
					>
				</div>
			</div>

			<!-- Ban list -->
			<div>
				<div class="mb-1 flex items-center justify-between">
					<span class="text-muted-foreground text-xs">밴 목록</span>
					<button
						type="button"
						class="text-muted-foreground text-xs underline"
						onclick={refreshBans}>새로고침</button
					>
				</div>
				{#if bannedPlayers.length > 0}
					<div class="flex flex-wrap gap-1.5">
						{#each bannedPlayers as p}
							<button
								type="button"
								class="border-border rounded-full border px-2 py-0.5 text-xs {playerName === p
									? 'bg-primary text-primary-foreground'
									: ''}"
								onclick={() => (playerName = p)}
							>
								{p}
							</button>
						{/each}
					</div>
				{:else}
					<p class="text-muted-foreground text-xs">밴 처리된 플레이어가 없습니다.</p>
				{/if}
			</div>

			<!-- Op list -->
			<div>
				<div class="mb-1 flex items-center justify-between">
					<span class="text-muted-foreground text-xs">운영자 목록</span>
					<button type="button" class="text-muted-foreground text-xs underline" onclick={refreshOps}
						>새로고침</button
					>
				</div>
				{#if ops.length > 0}
					<div class="flex flex-wrap gap-1.5">
						{#each ops as opEntry}
							<button
								type="button"
								class="border-border rounded-full border px-2 py-0.5 text-xs {playerName ===
								opEntry.name
									? 'bg-primary text-primary-foreground'
									: ''}"
								onclick={() => (playerName = opEntry.name)}
								title="권한 레벨 {opEntry.level}"
							>
								{opEntry.name}
							</button>
						{/each}
					</div>
				{:else}
					<p class="text-muted-foreground text-xs">운영자가 없습니다.</p>
				{/if}
			</div>

			<!-- Whitelist -->
			<div>
				<div class="mb-1 flex items-center justify-between">
					<span class="text-muted-foreground text-xs">화이트리스트</span>
					<button
						type="button"
						class="text-muted-foreground text-xs underline"
						onclick={refreshWhitelist}>새로고침</button
					>
				</div>
				{#if !whitelistEnabled}
					<p class="text-muted-foreground text-xs">화이트리스트가 꺼져 있습니다.</p>
				{:else if whitelistedPlayers.length > 0}
					<div class="flex flex-wrap gap-1.5">
						{#each whitelistedPlayers as p}
							<button
								type="button"
								class="border-border rounded-full border px-2 py-0.5 text-xs {playerName === p
									? 'bg-primary text-primary-foreground'
									: ''}"
								onclick={() => (playerName = p)}
							>
								{p}
							</button>
						{/each}
					</div>
				{:else}
					<p class="text-muted-foreground text-xs">화이트리스트에 등록된 플레이어가 없습니다.</p>
				{/if}
			</div>

			<div class="flex gap-2">
				<button
					class="border-border rounded-md border px-2 py-1.5 text-xs"
					onclick={() => whitelistToggle(true)}>화이트리스트 켜기</button
				>
				<button
					class="border-border rounded-md border px-2 py-1.5 text-xs"
					onclick={() => whitelistToggle(false)}>화이트리스트 끄기</button
				>
			</div>

			<div>
				<label class="text-muted-foreground mb-1 block text-xs" for="announce">전체 공지</label>
				<div class="flex gap-2">
					<input
						id="announce"
						bind:value={announceText}
						placeholder="메시지"
						class="border-input bg-background w-full min-w-0 flex-1 rounded-md border px-2 py-1.5 text-sm"
						onkeydown={(e) => {
							if (e.key === 'Enter') sendCommand(`say ${announceText}`);
						}}
					/>
					<button
						class="border-border shrink-0 rounded-md border px-3 py-1.5 text-sm"
						onclick={() => sendCommand(`say ${announceText}`)}>방송</button
					>
				</div>
			</div>

			<div class="grid grid-cols-2 gap-2">
				<div>
					<label class="text-muted-foreground mb-1 block truncate text-xs" for="gamemode" title="대상: {playerName || '미지정'}">
						게임모드 (대상: {playerName || '미지정'})
					</label>
					<div class="flex gap-2">
						<select
							id="gamemode"
							bind:value={gamemode}
							class="border-input bg-background w-full rounded-md border px-2 py-1.5 text-sm"
						>
							<option value="survival">서바이벌</option>
							<option value="creative">크리에이티브</option>
							<option value="adventure">어드벤처</option>
							<option value="spectator">관전자</option>
						</select>
					</div>
					<button
						class="border-border mt-2 w-full rounded-md border px-2 py-1.5 text-xs"
						disabled={!playerName}
						onclick={() => sendCommand(`gamemode ${gamemode} ${playerName}`)}>적용</button
					>
				</div>
				<div>
					<label class="text-muted-foreground mb-1 block text-xs" for="difficulty">난이도</label>
					<select
						id="difficulty"
						bind:value={difficulty}
						class="border-input bg-background w-full rounded-md border px-2 py-1.5 text-sm"
					>
						<option value="peaceful">평화로움</option>
						<option value="easy">쉬움</option>
						<option value="normal">보통</option>
						<option value="hard">어려움</option>
					</select>
					<button
						class="border-border mt-2 w-full rounded-md border px-2 py-1.5 text-xs"
						onclick={() => sendCommand(`difficulty ${difficulty}`)}>적용</button
					>
				</div>
			</div>

			<div>
				<span class="text-muted-foreground mb-1 block text-xs">시간</span>
				<div class="flex gap-2">
					<button
						class="border-border flex-1 rounded-md border px-2 py-1.5 text-xs"
						onclick={() => sendCommand('time set day')}>낮</button
					>
					<button
						class="border-border flex-1 rounded-md border px-2 py-1.5 text-xs"
						onclick={() => sendCommand('time set night')}>밤</button
					>
				</div>
			</div>

			<div>
				<span class="text-muted-foreground mb-1 block text-xs">날씨</span>
				<div class="flex gap-2">
					<button
						class="border-border flex-1 rounded-md border px-2 py-1.5 text-xs"
						onclick={() => sendCommand('weather clear')}>맑음</button
					>
					<button
						class="border-border flex-1 rounded-md border px-2 py-1.5 text-xs"
						onclick={() => sendCommand('weather rain')}>비</button
					>
					<button
						class="border-border flex-1 rounded-md border px-2 py-1.5 text-xs"
						onclick={() => sendCommand('weather thunder')}>뇌우</button
					>
				</div>
			</div>
		</div>
		{/if}
	</div>
	{/if}
</main>

{#if reasonModalKind}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
		onclick={closeReasonModal}
		onkeydown={(e) => {
			if (e.key === 'Escape') closeReasonModal();
		}}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="bg-card border-border w-full max-w-sm rounded-lg border p-4 shadow-lg"
			onclick={(e) => e.stopPropagation()}
		>
			<h2 class="mb-3 text-sm font-semibold">
				{reasonModalKind === 'kick' ? '강제 퇴장' : '밴'} 사유 선택 -- {playerName}
			</h2>
			<div class="mb-3 flex flex-col gap-1.5">
				{#each reasonPresets as preset}
					<button
						type="button"
						class="border-border rounded-md border px-2 py-1.5 text-left text-xs"
						onclick={() => applyReason(preset === '사유 없음' ? '' : preset)}
					>
						{preset}
					</button>
				{/each}
			</div>
			<div class="flex gap-2">
				<input
					bind:value={customReason}
					placeholder="직접 입력"
					class="border-input bg-background w-full min-w-0 flex-1 rounded-md border px-2 py-1.5 text-sm"
					onkeydown={(e) => {
						if (e.key === 'Enter') applyReason(customReason);
					}}
				/>
				<button
					type="button"
					class="bg-primary text-primary-foreground shrink-0 rounded-md px-3 py-1.5 text-sm"
					onclick={() => applyReason(customReason)}>적용</button
				>
			</div>
			<button
				type="button"
				class="text-muted-foreground mt-3 w-full text-center text-xs underline"
				onclick={closeReasonModal}>취소</button
			>
		</div>
	</div>
{/if}

{#if showPluginSearchModal}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-8"
		onclick={() => (showPluginSearchModal = false)}
		onkeydown={(e) => {
			if (e.key === 'Escape') showPluginSearchModal = false;
		}}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="bg-card border-border flex max-h-[80vh] w-full max-w-lg flex-col rounded-lg border p-4 shadow-lg"
			onclick={(e) => e.stopPropagation()}
		>
			<div class="mb-3 flex items-center justify-between">
				<h2 class="font-medium">Modrinth에서 {pluginTabLabel(inst?.loader)} 검색</h2>
				<button
					type="button"
					class="text-muted-foreground text-sm"
					onclick={() => (showPluginSearchModal = false)}>&times;</button
				>
			</div>
			<form class="flex gap-2" onsubmit={searchPlugins}>
				<input
					bind:value={pluginQuery}
					placeholder="{pluginTabLabel(inst?.loader)} 이름"
					class="border-input bg-background w-full min-w-0 flex-1 rounded-md border px-3 py-2 text-sm"
				/>
				<button
					type="submit"
					disabled={searchingPlugins}
					class="border-border shrink-0 rounded-md border px-3 py-1.5 text-sm"
				>
					{searchingPlugins ? '검색 중...' : '검색'}
				</button>
			</form>
			{#if pluginSearchError}
				<p class="text-destructive mt-2 text-xs">{pluginSearchError}</p>
			{/if}
			<div class="mt-2 flex-1 space-y-1.5 overflow-y-auto">
				{#each pluginSearchResults as hit (hit.project_id)}
					<div
						class="border-border flex items-center justify-between rounded-md border px-2 py-1.5 text-xs"
					>
						<div class="min-w-0">
							<span class="font-medium">{hit.title}</span>
							<span class="text-muted-foreground ml-2">
								다운로드 {hit.downloads.toLocaleString()}
							</span>
							<p class="text-muted-foreground truncate">{hit.description}</p>
						</div>
						<button
							class="border-border ml-2 shrink-0 rounded-md border px-2 py-1 text-xs"
							disabled={installingProjectId === hit.project_id}
							onclick={() => installPlugin(hit.project_id)}
						>
							{installingProjectId === hit.project_id ? '설치 중...' : '설치'}
						</button>
					</div>
				{/each}
			</div>
		</div>
	</div>
{/if}

{#if showGameSettingsModal}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-8"
		onclick={closeGameSettingsModal}
		onkeydown={(e) => {
			if (e.key === 'Escape') closeGameSettingsModal();
		}}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="bg-card border-border flex max-h-[80vh] w-full max-w-2xl flex-col rounded-lg border p-4 shadow-lg"
			onclick={(e) => e.stopPropagation()}
		>
			<div class="mb-1 flex shrink-0 items-center justify-between">
				<h2 class="font-medium">게임플레이 설정</h2>
				<button type="button" class="text-muted-foreground text-sm" onclick={closeGameSettingsModal}
					>&times;</button
				>
			</div>
			<p class="text-muted-foreground mb-3 shrink-0 text-xs">
				변경 사항은 서버를 재시작해야 적용됩니다. 여기 없는 세부 설정은 파일 탭에서
				<code>server.properties</code>를 직접 편집하세요.
			</p>
			{#if gameSettingsLoading}
				<p class="text-muted-foreground text-xs">불러오는 중...</p>
			{:else if gameSettingsError && gameSettings.length === 0}
				<p class="text-destructive text-xs">설정을 불러오지 못했습니다: {gameSettingsError}</p>
			{:else}
				<div class="min-h-0 flex-1 overflow-y-auto">
					<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
					{#each gameSettings as setting (setting.key)}
						<div>
							<label class="text-muted-foreground mb-1 flex items-center gap-1 text-xs" for="gs-{setting.key}">
								<span>{setting.label}</span>
								{#if setting.description}
									<span class="group relative inline-flex">
										<span
											class="border-muted-foreground text-muted-foreground inline-flex h-3.5 w-3.5 cursor-help items-center justify-center rounded-full border text-[9px] leading-none"
											>?</span
										>
										<span
											class="bg-popover text-popover-foreground border-border pointer-events-none absolute bottom-full left-1/2 z-10 mb-1.5 w-56 -translate-x-1/2 rounded-md border p-2 text-xs opacity-0 shadow-lg transition-opacity group-hover:opacity-100"
											>{setting.description}</span
										>
									</span>
								{/if}
							</label>
							{#if setting.type === 'bool'}
								<div class="relative">
									<select
										id="gs-{setting.key}"
										bind:value={gameSettingsEdits[setting.key]}
										class="border-input bg-background w-full appearance-none rounded-md border py-1.5 pl-3 pr-8 text-sm"
									>
										<option value="true">켜짐</option>
										<option value="false">꺼짐</option>
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
							{:else if setting.type === 'enum'}
								<div class="relative">
									<select
										id="gs-{setting.key}"
										bind:value={gameSettingsEdits[setting.key]}
										class="border-input bg-background w-full appearance-none rounded-md border py-1.5 pl-3 pr-8 text-sm"
									>
										{#each setting.options ?? [] as opt (opt)}
											<option value={opt}>{enumOptionLabel(setting.key, opt)}</option>
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
							{:else if setting.type === 'int'}
								<input
									id="gs-{setting.key}"
									type="number"
									bind:value={gameSettingsEdits[setting.key]}
									class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-sm"
								/>
							{:else}
								<input
									id="gs-{setting.key}"
									type="text"
									bind:value={gameSettingsEdits[setting.key]}
									class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-sm"
								/>
							{/if}
						</div>
					{/each}
					</div>
				</div>
				{#if gameSettingsError}
					<p class="text-destructive mt-2 shrink-0 text-xs">{gameSettingsError}</p>
				{/if}
				{#if gameSettingsSaved}
					<p class="mt-2 shrink-0 text-xs text-green-500">저장됐습니다. 다시 시작하면 적용됩니다.</p>
				{/if}
				<button
					class="border-border mt-3 shrink-0 rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
					disabled={gameSettingsSaving}
					onclick={saveGameSettings}
				>
					{gameSettingsSaving ? '저장 중...' : '저장'}
				</button>
			{/if}
		</div>
	</div>
{/if}

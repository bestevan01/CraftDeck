// Mirrors proxyMemoryMaxMB in internal/api/handlers_proxy.go -- the always-on
// Velocity proxy has a fixed 1GB allocation, so per-server memory sliders
// reserve it off the top of total system memory rather than letting the
// operator carve it out of an already-fixed number.
export const PROXY_RESERVED_MEMORY_MB = 1024;

export type InstanceStatus = 'stopped' | 'starting' | 'running' | 'stopping' | 'crashed';

export type Instance = {
	id: string;
	name: string;
	kind: 'server' | 'proxy';
	loader: string;
	loader_version: string;
	mc_version: string;
	java_major: number;
	game_port: number;
	rcon_port: number;
	cpu_quota_percent: number;
	memory_max_mb: number;
	work_dir: string;
	status: InstanceStatus;
	created_at: string;
	proxy_opt_out: boolean;
};

export type SystemResources = {
	cpu_percent: number;
	cpu_count: number;
	cpu_temp_c?: number;
	total_memory_mb: number;
	used_memory_mb: number;
	total_disk_mb: number;
	used_disk_mb: number;
};

export type VanillaVersion = {
	id: string;
	type: 'release' | 'snapshot' | 'old_beta' | 'old_alpha';
};

// Mirrors internal/loader.BuildInfo -- one selectable build for a loader
// that has a genuine per-mc_version build concept (see BuildLister).
export type BuildInfo = {
	id: string;
	channel?: string;
	time?: string;
};

export type Backup = {
	id: string;
	instance_id: string;
	filename: string;
	size_bytes: number;
	created_at: string;
};

// Mirrors internal/api's serverSettingValue -- one curated
// server.properties key the GUI settings form (FR-12) can show/edit.
export type ServerSetting = {
	key: string;
	label: string;
	description?: string;
	type: 'bool' | 'int' | 'string' | 'enum';
	options?: string[];
	value: string;
};

export type OpEntry = {
	uuid: string;
	name: string;
	level: number;
	bypassesPlayerLimit: boolean;
};

export type CreateInstanceRequest = {
	name: string;
	kind: 'server';
	loader: string;
	loader_version?: string;
	mc_version: string;
	cpu_quota_percent?: number;
	memory_max_mb?: number;
	accept_eula: boolean;
	// Only meaningful for loader: 'paper' | 'purpur' | 'folia' -- see
	// internal/api/handlers_instance.go's handleCreateInstance. Vanilla
	// can't sit behind the proxy at all, so it's always independently
	// exposed regardless of this flag.
	expose_independently?: boolean;
};

export type ServerSubdomain = {
	registered: boolean;
	forced_host: string;
	// The singleton proxy's own game_port, present when registered is true
	// -- a proxied server is bound to 127.0.0.1 only, so this (not the
	// server's own game_port) is what a player actually connects to.
	proxy_port?: number;
};

export type FileEntry = {
	name: string;
	path: string;
	is_dir: boolean;
	size: number;
	mod_time: string;
};

export type ProxyStatus = {
	exists: boolean;
	// Lets the memory-slider ceiling free up the 1GB reserved for the
	// proxy whenever it isn't actually running, not just whenever it
	// doesn't exist at all.
	running: boolean;
	current_version?: string;
	latest_version?: string;
	update_available: boolean;
};

export type ProxyBackend = {
	proxy_id: string;
	backend_instance_id: string;
	priority: number;
	forced_host?: string;
};

export type CraftdeckVersion = {
	current_version: string;
	latest_version?: string;
	update_available: boolean;
};

// Mirrors internal/update.Settings -- the apt channel craftdeckd's own
// package tracks, and how often /api/system/version is allowed to actually
// hit the apt repo rather than reply from its cached last-checked result.
export type UpdateSettings = {
	channel: 'stable' | 'beta' | 'canary';
	check_frequency: 'every_visit' | 'daily' | 'weekly' | 'monthly';
};

// Mirrors internal/network.PortMapping (FR-22~24).
export type PortMapping = {
	id: string;
	instance_id?: string;
	external_port: number;
	internal_port: number;
	protocol: 'tcp' | 'udp';
	method: 'upnp' | 'natpmp' | 'manual';
	created_at: string;
};

// Mirrors internal/network.ManualInfo -- shown when neither UPnP nor
// NAT-PMP could set up the mapping automatically (FR-23).
export type ManualPortInfo = {
	local_ip: string;
	internal_port: number;
	external_port: number;
	protocol: string;
};

// Mirrors internal/api's networkSettingsResponse (FR-21/22/23/25) -- one
// toggle covers both the web UI port and every directly-reachable
// Minecraft game port (proxy or independently-exposed server).
export type NetworkSettings = {
	wan_enabled: boolean;
	web_mapping?: PortMapping;
	manual_info?: ManualPortInfo;
};

// Mirrors internal/api's networkAddressesResponse -- public_ip is only
// present while FR-21's WAN toggle is on.
export type NetworkAddresses = {
	local_ip: string;
	public_ip?: string;
};

// Mirrors internal/swap.Info -- CraftDeck's own disk-backed swap file,
// independent of any RAM-based swap (e.g. zram) the OS already runs.
export type SwapInfo = {
	// False on storage a disk-backed swap file would actively hurt (an SD
	// card/eMMC) rather than just not need -- the frontend hides the
	// feature entirely in that case instead of just disabling controls.
	supported: boolean;
	enabled: boolean;
	size_mb: number;
	used_mb: number;
	free_disk_mb: number;
};

// Mirrors internal/hardware.Config -- Active Cooler detection result plus
// the current overclock config and last benchmark outcome, all one
// singleton row. cooler_detected gates whether the overclock card renders
// at all (see +page.svelte); the backend enforces the same gate on
// PUT /api/system/overclock independently of what the UI shows.
export type HardwareInfo = {
	cooler_detected: boolean;
	cooler_checked_at?: string;
	overclock_enabled: boolean;
	overclock_preset: string;
	overclock_arm_freq?: number;
	overclock_over_voltage_delta?: number;
	overclock_applied_at?: string;
	last_benchmark_result: '' | 'pass' | 'fail';
	last_benchmark_at?: string;
};

// Mirrors internal/hardware.BenchmarkStatus -- polled while the stability
// self-test (internal/hardware.RunBenchmark) is running.
export type BenchmarkStatus = {
	running: boolean;
	elapsed_sec: number;
	total_sec: number;
	current_temp_c: number;
	min_temp_c: number;
	max_temp_c: number;
	avg_temp_c: number;
	result: '' | 'pass' | 'fail';
	under_voltage_detected: boolean;
	throttled_detected: boolean;
};

// Mirrors hardware.Presets -- kept in sync by hand since it's a short,
// rarely-changed list; not worth a round trip just to render four radio
// options. over_voltage_delta_uv is in microvolts (Pi 5 firmware's actual
// config.txt key, over_voltage_delta) -- "high" matches values already
// confirmed stable on real hardware, not a guess.
export const OVERCLOCK_PRESETS = [
	{ name: 'default', label: '기본값', arm_freq_mhz: 2400, over_voltage_delta_uv: 0 },
	{ name: 'safe', label: '안전', arm_freq_mhz: 2600, over_voltage_delta_uv: 30000 },
	{ name: 'medium', label: '보통', arm_freq_mhz: 2800, over_voltage_delta_uv: 50000 },
	{ name: 'high', label: '높음', arm_freq_mhz: 3000, over_voltage_delta_uv: 80000 }
] as const;

// Mirrors internal/ddns.Config -- whether an owned domain or only a free
// DDNS subdomain is registered decides whether Velocity runs at all
// (FR-1f). mode/last_known_ip/last_checked_at/mismatch_detected only carry
// real data for kind=free_subdomain (FR-26/30). cert_renewal_error(_at) only
// carries real data for kind=main_domain (FR-33a) -- set whenever
// internal/tlscert.Manager's certmagic-managed Let's Encrypt certificate
// fails to obtain/renew, cleared the next time one succeeds.
export type DomainConfig = {
	id: string;
	kind: 'main_domain' | 'free_subdomain';
	provider: string;
	hostname: string;
	mode: 'active' | 'monitor';
	last_known_ip?: string;
	last_checked_at?: string;
	mismatch_detected: boolean;
	cert_renewal_error?: string;
	cert_renewal_error_at?: string;
	created_at: string;
};

export type UpgradeProxyResult = {
	upgraded: boolean;
	version: string;
};

async function req<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(path, {
		headers: { 'Content-Type': 'application/json' },
		...init
	});
	if (!res.ok) {
		const text = await res.text();
		// A session that expires mid-use (or a login/setup attempt with
		// wrong credentials) both come back as 401. Only force-redirect for
		// the former -- login/setup requests to /api/auth/* should just
		// surface their error inline instead of bouncing the user right back
		// to the page they're already on.
		if (res.status === 401 && !path.startsWith('/api/auth/')) {
			window.location.href = '/login';
		}
		// craftdeckd's own handlers always write a plain-text error body.
		// An HTML body means this response never reached craftdeckd at all
		// -- e.g. a reverse proxy's own 502 page while the service is
		// mid-restart (self-update, apt upgrade) -- so dumping it verbatim
		// would show the user a full raw HTML document instead of a
		// message. Confirmed on real hardware during a self-update.
		const contentType = res.headers.get('content-type') ?? '';
		const message = contentType.includes('html')
			? `서버에 일시적으로 연결할 수 없습니다 (${res.status})`
			: text || `${res.status} ${res.statusText}`;
		throw new Error(message);
	}
	if (res.status === 204) return undefined as T;
	const contentType = res.headers.get('content-type') ?? '';
	return contentType.includes('application/json') ? ((await res.json()) as T) : (undefined as T);
}

export type WorldInfo = {
	level_name: string;
	instance_version: string;
	detected_version: string;
	detect_error: string;
};

export type WorldImportResult = {
	detected_version: string;
	detect_error?: string;
};

export type PluginSearchHit = {
	project_id: string;
	slug: string;
	title: string;
	description: string;
	downloads: number;
	icon_url: string;
};

export type Plugin = {
	id: string;
	instance_id: string;
	source: 'modrinth' | 'upload';
	modrinth_project_id?: string;
	modrinth_version_id?: string;
	filename: string;
	title?: string;
	sha512?: string;
	enabled: boolean;
	installed_as_dependency: boolean;
	parent_plugin_id?: string;
	// Every plugin ID that requires this one -- not just the first (see
	// Plugin.DependentOf on the Go side).
	dependent_of?: string[];
	created_at: string;
};

export type AuthStatus = {
	setup_required: boolean;
	authenticated: boolean;
	lan_bypass: boolean;
	username: string;
	totp_enabled: boolean;
};

export type TOTPSetup = {
	secret: string;
	otpauth_url: string;
	qr_code_png: string;
};

export type TOTPVerifyResult = {
	enabled: boolean;
	backup_codes: string[];
};

export const api = {
	authStatus: () => req<AuthStatus>('/api/auth/status'),
	setup: (username: string, password: string) =>
		req<void>('/api/auth/setup', { method: 'POST', body: JSON.stringify({ username, password }) }),
	// login resolves (rather than throwing) with { totp_required: true } when
	// FR-37 requires a code this account hasn't supplied yet -- the frontend
	// can't know that in advance, so a first attempt omits totpCode and a
	// second one (once the operator sees totp_required) resends the same
	// username/password plus the code.
	login: async (
		username: string,
		password: string,
		totpCode?: string
	): Promise<{ totp_required: boolean }> => {
		const res = await fetch('/api/auth/login', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ username, password, totp_code: totpCode || undefined })
		});
		if (res.status === 401) {
			const text = await res.text();
			try {
				const parsed = JSON.parse(text);
				if (parsed && parsed.totp_required) return { totp_required: true };
			} catch {
				// not JSON -- an ordinary wrong-credentials/wrong-code message,
				// fall through to the generic error below.
			}
			throw new Error(text || `${res.status} ${res.statusText}`);
		}
		if (!res.ok) {
			const text = await res.text();
			throw new Error(text || `${res.status} ${res.statusText}`);
		}
		return { totp_required: false };
	},
	logout: () => req<void>('/api/auth/logout', { method: 'POST' }),
	// FR-39: setup returns a fresh secret/QR every call (overwriting any
	// unconfirmed prior attempt) until verify actually turns 2FA on.
	setupTOTP: () => req<TOTPSetup>('/api/auth/2fa/setup', { method: 'POST' }),
	verifyTOTP: (code: string) =>
		req<TOTPVerifyResult>('/api/auth/2fa/verify', { method: 'POST', body: JSON.stringify({ code }) }),
	disableTOTP: (password: string) =>
		req<void>('/api/auth/2fa/disable', { method: 'POST', body: JSON.stringify({ password }) }),
	regenerateBackupCodes: () =>
		req<{ backup_codes: string[] }>('/api/auth/2fa/backup-codes/regenerate', { method: 'POST' }),
	changePassword: (username: string, currentPassword: string, newPassword: string) =>
		req<void>('/api/auth/password', {
			method: 'POST',
			body: JSON.stringify({
				username,
				current_password: currentPassword,
				new_password: newPassword
			})
		}),
	listInstances: () => req<Instance[]>('/api/instances'),
	getInstance: (id: string) => req<Instance>(`/api/instances/${id}`),
	createInstance: (body: CreateInstanceRequest) =>
		req<Instance>('/api/instances', { method: 'POST', body: JSON.stringify(body) }),
	deleteInstance: (id: string) => req<void>(`/api/instances/${id}`, { method: 'DELETE' }),
	// FR-3: a custom/unlisted loader has no adapter to auto-download a jar,
	// so this is the only way it gets one -- called right after
	// createInstance for a loader the create form doesn't otherwise
	// recognize. Also works on any other instance to manually swap its jar.
	uploadServerJar: async (id: string, file: File) => {
		const form = new FormData();
		form.append('jar', file);
		const res = await fetch(`/api/instances/${id}/jar`, { method: 'POST', body: form });
		if (!res.ok) {
			const text = await res.text();
			if (res.status === 401) window.location.href = '/login';
			throw new Error(text || `${res.status} ${res.statusText}`);
		}
	},
	updateInstance: (
		id: string,
		body: { cpu_quota_percent: number; memory_max_mb: number; game_port?: number }
	) => req<Instance>(`/api/instances/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
	// FR-4, scoped to "redownload the same loader for the same mc_version" --
	// see handleReinstallLoader. Omit loaderVersion (or pass '') for "always
	// latest"; pass a loader.BuildInfo.ID from listLoaderBuilds to pin one
	// specific build instead.
	reinstallLoader: (id: string, loaderVersion?: string) =>
		req<{ ok: boolean }>(`/api/instances/${id}/reinstall`, {
			method: 'POST',
			body: JSON.stringify({ loader_version: loaderVersion ?? '' })
		}),
	startInstance: (id: string) => req<void>(`/api/instances/${id}/start`, { method: 'POST' }),
	stopInstance: (id: string) => req<void>(`/api/instances/${id}/stop`, { method: 'POST' }),
	restartInstance: (id: string) => req<void>(`/api/instances/${id}/restart`, { method: 'POST' }),
	sendCommand: (id: string, command: string) =>
		req<{ result: string }>(`/api/instances/${id}/command`, {
			method: 'POST',
			body: JSON.stringify({ command })
		}),
	onlinePlayers: (id: string) =>
		req<{ online: number; max: number; sample: string[] }>(`/api/instances/${id}/players`),
	listBans: (id: string) => req<{ players: string[] }>(`/api/instances/${id}/bans`),
	listOps: (id: string) => req<OpEntry[]>(`/api/instances/${id}/ops`),
	listWhitelist: (id: string) =>
		req<{ enabled: boolean; players: string[] }>(`/api/instances/${id}/whitelist`),
	// FR-12: curated GUI form over server.properties -- see the general file
	// manager (handlers_files.go) for raw/advanced editing instead.
	getServerSettings: (id: string) => req<ServerSetting[]>(`/api/instances/${id}/settings`),
	setServerSettings: (id: string, updates: Record<string, string>) =>
		req<{ ok: boolean }>(`/api/instances/${id}/settings`, {
			method: 'PUT',
			body: JSON.stringify(updates)
		}),
	systemResources: () => req<SystemResources>('/api/system/resources'),
	systemVersion: (force = false) =>
		req<CraftdeckVersion>(`/api/system/version${force ? '?force=1' : ''}`),
	updateCraftdeck: (targetVersion: string) =>
		req<void>('/api/system/update', {
			method: 'POST',
			body: JSON.stringify({ target_version: targetVersion })
		}),
	getUpdateSettings: () => req<UpdateSettings>('/api/system/update-settings'),
	setUpdateSettings: (settings: UpdateSettings) =>
		req<UpdateSettings>('/api/system/update-settings', {
			method: 'PUT',
			body: JSON.stringify(settings)
		}),
	listVanillaVersions: () => req<VanillaVersion[]>('/api/loaders/vanilla/versions'),
	listPaperVersions: () => req<string[]>('/api/loaders/paper/versions'),
	listPurpurVersions: () => req<string[]>('/api/loaders/purpur/versions'),
	listFoliaVersions: () => req<string[]>('/api/loaders/folia/versions'),
	listPufferfishVersions: () => req<string[]>('/api/loaders/pufferfish/versions'),
	listLeafVersions: () => req<string[]>('/api/loaders/leaf/versions'),
	listFabricVersions: () => req<string[]>('/api/loaders/fabric/versions'),
	listNeoForgeVersions: () => req<string[]>('/api/loaders/neoforge/versions'),
	// Empty array (not an error) for a loader whose adapter doesn't support
	// build-pinning (e.g. vanilla, pufferfish, fabric) -- see
	// handleListLoaderBuilds.
	listLoaderBuilds: (loaderName: string, mcVersion: string) =>
		req<BuildInfo[]>(
			`/api/loaders/${loaderName}/builds?mc_version=${encodeURIComponent(mcVersion)}`
		),
	// Subdomain (forced-host) is keyed by the server's own instance ID, not
	// the proxy's -- the proxy is hidden from the UI entirely, so this is
	// how each server's own console page manages its own subdomain.
	getServerSubdomain: (id: string) => req<ServerSubdomain>(`/api/instances/${id}/subdomain`),
	setServerSubdomain: (id: string, forcedHost: string) =>
		req<ServerSubdomain>(`/api/instances/${id}/subdomain`, {
			method: 'PUT',
			body: JSON.stringify({ forced_host: forcedHost })
		}),
	// Manual escape hatch for a custom/unlisted loader (FR-3) that
	// supportsVelocityForwarding doesn't recognize, so it was never added to
	// the proxy automatically at creation -- see handlers_proxy.go.
	registerBehindProxy: (id: string) =>
		req<{ forwarding_secret: string }>(`/api/instances/${id}/proxy/register`, { method: 'POST' }),
	unregisterFromProxy: (id: string) =>
		req<{ ok: boolean }>(`/api/instances/${id}/proxy/unregister`, { method: 'POST' }),
	getProxyStatus: () => req<ProxyStatus>('/api/proxy/status'),
	getProxyBackends: (proxyId: string) =>
		req<ProxyBackend[]>(`/api/instances/${proxyId}/proxy/backends`),
	// Replaces the whole backend list -- pass every backend back, not just
	// the ones that moved (see handleSetProxyBackends).
	setProxyBackends: (proxyId: string, backends: ProxyBackend[]) =>
		req<ProxyBackend[]>(`/api/instances/${proxyId}/proxy/backends`, {
			method: 'PUT',
			body: JSON.stringify({
				backends: backends.map((b) => ({
					backend_instance_id: b.backend_instance_id,
					priority: b.priority,
					forced_host: b.forced_host ?? ''
				}))
			})
		}),
	// File manager (FR-12 and beyond): Explorer/Finder-style browsing of an
	// instance's whole work dir -- list/read/write/download/upload/rename/
	// delete, all path-traversal-checked server-side (see
	// resolveInstancePath in internal/api/handlers_files.go).
	listFiles: (id: string, path: string) =>
		req<FileEntry[]>(`/api/instances/${id}/files?path=${encodeURIComponent(path)}`),
	getFileContent: (id: string, path: string) =>
		req<{ content: string }>(`/api/instances/${id}/files/content?path=${encodeURIComponent(path)}`),
	setFileContent: (id: string, path: string, content: string) =>
		req<{ ok: boolean }>(`/api/instances/${id}/files/content?path=${encodeURIComponent(path)}`, {
			method: 'PUT',
			body: JSON.stringify({ content })
		}),
	downloadFileURL: (id: string, path: string) =>
		`/api/instances/${id}/files/download?path=${encodeURIComponent(path)}`,
	uploadFile: async (id: string, dirPath: string, file: File) => {
		const form = new FormData();
		form.append('file', file);
		// Not routed through req(): a multipart body needs the browser to set
		// its own Content-Type (with boundary), which req() would override by
		// always forcing application/json.
		const res = await fetch(`/api/instances/${id}/files/upload?path=${encodeURIComponent(dirPath)}`, {
			method: 'POST',
			body: form
		});
		if (!res.ok) {
			const text = await res.text();
			if (res.status === 401) window.location.href = '/login';
			throw new Error(text || `${res.status} ${res.statusText}`);
		}
		return (await res.json()) as FileEntry;
	},
	renameFile: (id: string, from: string, to: string) =>
		req<{ ok: boolean }>(`/api/instances/${id}/files/rename`, {
			method: 'PUT',
			body: JSON.stringify({ from, to })
		}),
	deleteFile: (id: string, path: string) =>
		req<void>(`/api/instances/${id}/files?path=${encodeURIComponent(path)}`, { method: 'DELETE' }),
	upgradeProxy: () => req<UpgradeProxyResult>('/api/proxy/upgrade', { method: 'POST' }),
	// CraftDeck's own disk-backed swap file -- independent of any RAM-based
	// swap (e.g. Raspberry Pi OS's zram) the base OS may already run.
	getSwap: () => req<SwapInfo>('/api/system/swap'),
	setSwap: (sizeMB: number) =>
		req<SwapInfo>('/api/system/swap', { method: 'PUT', body: JSON.stringify({ size_mb: sizeMB }) }),
	deleteSwap: () => req<{ ok: boolean }>('/api/system/swap', { method: 'DELETE' }),
	// Active Cooler 감지 + 오버클럭 (internal/hardware) -- 감지 결과가 없으면
	// setOverclock 자체가 서버에서 403으로 거부된다.
	getHardware: () => req<HardwareInfo>('/api/system/hardware'),
	redetectCooler: () => req<HardwareInfo>('/api/system/hardware/redetect', { method: 'POST' }),
	setOverclock: (enabled: boolean, preset: string, armFreqMHz: number, overVoltageDeltaUV: number) =>
		req<HardwareInfo>('/api/system/overclock', {
			method: 'PUT',
			body: JSON.stringify({
				enabled,
				preset,
				arm_freq_mhz: armFreqMHz,
				over_voltage_delta_uv: overVoltageDeltaUV
			})
		}),
	rebootForOverclock: () => req<void>('/api/system/overclock/reboot', { method: 'POST' }),
	startBenchmark: () => req<void>('/api/system/overclock/benchmark', { method: 'POST' }),
	getBenchmarkStatus: () => req<BenchmarkStatus>('/api/system/overclock/benchmark/status'),
	// FR-21/22/23/25: "외부 접속 허용" toggle (web UI + every reachable game
	// port) + UPnP/NAT-PMP automation.
	getNetworkSettings: () => req<NetworkSettings>('/api/network/settings'),
	setWANEnabled: (enabled: boolean) =>
		req<NetworkSettings>('/api/network/settings', {
			method: 'PUT',
			body: JSON.stringify({ wan_enabled: enabled })
		}),
	// FR-24: review/revoke individual port-forwarding rules CraftDeck registered.
	listPortMappings: () => req<PortMapping[]>('/api/network/port-mappings'),
	deletePortMapping: (id: string) =>
		req<{ ok: boolean }>(`/api/network/port-mappings/${id}`, { method: 'DELETE' }),
	// FR-26 minimal skeleton + FR-1f: registering/clearing this decides
	// whether Velocity runs at all (see ReconcileProxyMode).
	// 접속 주소 복사 버튼(사설/공인 IP) -- public_ip는 외부 접속이 켜져 있을 때만 채워짐.
	getNetworkAddresses: () => req<NetworkAddresses>('/api/network/addresses'),
	getDomainSettings: () => req<DomainConfig | { registered: false }>('/api/domain/settings'),
	// token is the provider API credential (FR-26c, e.g. DuckDNS's token) --
	// required for an active-renewal provider, omit/ignore for a
	// monitor-only one (ipTime, FR-26e) or main_domain.
	setDomainSettings: (
		kind: 'main_domain' | 'free_subdomain',
		provider: string,
		hostname: string,
		token?: string
	) =>
		req<DomainConfig>('/api/domain/settings', {
			method: 'PUT',
			body: JSON.stringify({ kind, provider, hostname, token })
		}),
	deleteDomainSettings: () => req<{ ok: boolean }>('/api/domain/settings', { method: 'DELETE' }),
	listBackups: (id: string) => req<Backup[]>(`/api/instances/${id}/backups`),
	createBackup: (id: string) => req<Backup>(`/api/instances/${id}/backups`, { method: 'POST' }),
	restoreBackup: (id: string, backupId: string) =>
		req<void>(`/api/instances/${id}/backups/${backupId}/restore`, { method: 'POST' }),
	deleteBackup: (id: string, backupId: string) =>
		req<void>(`/api/instances/${id}/backups/${backupId}`, { method: 'DELETE' }),
	searchPlugins: (id: string, query: string) =>
		req<PluginSearchHit[]>(`/api/instances/${id}/plugins/search?query=${encodeURIComponent(query)}`),
	listPlugins: (id: string) => req<Plugin[]>(`/api/instances/${id}/plugins`),
	installPlugin: (id: string, projectId: string) =>
		req<Plugin>(`/api/instances/${id}/plugins`, {
			method: 'POST',
			body: JSON.stringify({ project_id: projectId })
		}),
	uploadPlugin: async (id: string, file: File) => {
		const form = new FormData();
		form.append('plugin', file);
		const res = await fetch(`/api/instances/${id}/plugins/upload`, { method: 'POST', body: form });
		if (!res.ok) {
			const text = await res.text();
			if (res.status === 401) window.location.href = '/login';
			throw new Error(text || `${res.status} ${res.statusText}`);
		}
		return (await res.json()) as Plugin;
	},
	setPluginEnabled: (id: string, pluginId: string, enabled: boolean) =>
		req<Plugin>(`/api/instances/${id}/plugins/${pluginId}`, {
			method: 'PATCH',
			body: JSON.stringify({ enabled })
		}),
	deletePlugin: (id: string, pluginId: string) =>
		req<void>(`/api/instances/${id}/plugins/${pluginId}`, { method: 'DELETE' }),
	worldInfo: (id: string) => req<WorldInfo>(`/api/instances/${id}/world/info`),
	exportWorldURL: (id: string) => `/api/instances/${id}/world/export`,
	importWorld: async (id: string, file: File, force: boolean) => {
		const form = new FormData();
		form.append('world', file);
		if (force) form.append('force', 'true');
		// Not routed through req(): a multipart body needs the browser to set
		// its own Content-Type (with boundary), which req() would override by
		// always forcing application/json.
		const res = await fetch(`/api/instances/${id}/world/import`, { method: 'POST', body: form });
		if (!res.ok) {
			const text = await res.text();
			if (res.status === 401) window.location.href = '/login';
			throw new Error(text || `${res.status} ${res.statusText}`);
		}
		return (await res.json()) as WorldImportResult;
	},
	consoleURL: (id: string) => {
		const proto = location.protocol === 'https:' ? 'wss' : 'ws';
		return `${proto}://${location.host}/api/instances/${id}/console`;
	}
};

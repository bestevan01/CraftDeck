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
};

export type SystemResources = {
	cpu_percent: number;
	cpu_count: number;
	total_memory_mb: number;
	used_memory_mb: number;
	total_disk_mb: number;
	used_disk_mb: number;
};

export type VanillaVersion = {
	id: string;
	type: 'release' | 'snapshot' | 'old_beta' | 'old_alpha';
};

export type Backup = {
	id: string;
	instance_id: string;
	filename: string;
	size_bytes: number;
	created_at: string;
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
	// Only meaningful for loader: 'paper' -- see internal/api/handlers_instance.go's
	// handleCreateInstance. Vanilla can't sit behind the proxy at all, so it's
	// always independently exposed regardless of this flag.
	expose_independently?: boolean;
};

export type ServerSubdomain = {
	registered: boolean;
	forced_host: string;
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
		throw new Error(text || `${res.status} ${res.statusText}`);
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
	sha512?: string;
	enabled: boolean;
	installed_as_dependency: boolean;
	created_at: string;
};

export type AuthStatus = {
	setup_required: boolean;
	authenticated: boolean;
	lan_bypass: boolean;
	username: string;
};

export const api = {
	authStatus: () => req<AuthStatus>('/api/auth/status'),
	setup: (username: string, password: string) =>
		req<void>('/api/auth/setup', { method: 'POST', body: JSON.stringify({ username, password }) }),
	login: (username: string, password: string) =>
		req<void>('/api/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) }),
	logout: () => req<void>('/api/auth/logout', { method: 'POST' }),
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
	updateInstance: (id: string, body: { cpu_quota_percent: number; memory_max_mb: number }) =>
		req<Instance>(`/api/instances/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
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
	systemResources: () => req<SystemResources>('/api/system/resources'),
	listVanillaVersions: () => req<VanillaVersion[]>('/api/loaders/vanilla/versions'),
	listPaperVersions: () => req<string[]>('/api/loaders/paper/versions'),
	// Subdomain (forced-host) is keyed by the server's own instance ID, not
	// the proxy's -- the proxy is hidden from the UI entirely, so this is
	// how each server's own console page manages its own subdomain.
	getServerSubdomain: (id: string) => req<ServerSubdomain>(`/api/instances/${id}/subdomain`),
	setServerSubdomain: (id: string, forcedHost: string) =>
		req<ServerSubdomain>(`/api/instances/${id}/subdomain`, {
			method: 'PUT',
			body: JSON.stringify({ forced_host: forcedHost })
		}),
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

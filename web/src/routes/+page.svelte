<script lang="ts">
	import { api, PROXY_RESERVED_MEMORY_MB, type Instance, type SystemResources } from '$lib/api';
	import { onDestroy, onMount } from 'svelte';

	async function logout() {
		await api.logout();
		window.location.href = '/login';
	}

	// 내부 네트워크(LAN)에서는 로그인 절차 자체를 건너뛰므로(백엔드
	// requireAuth의 lan_bypass 참고), 그 상태에서 "로그아웃"이나
	// "비밀번호 변경" 버튼을 보여주는 건 의미가 없어서 실제로 로그인된
	// 세션이 있을 때만 두 버튼을 노출한다.
	let isLoggedIn = $state(false);

	// Change-password modal state.
	let username = $state('');
	let showPasswordModal = $state(false);
	let currentPassword = $state('');
	let newPassword = $state('');
	let newPasswordConfirm = $state('');
	let passwordError = $state('');
	let changingPassword = $state(false);

	function openPasswordModal() {
		currentPassword = '';
		newPassword = '';
		newPasswordConfirm = '';
		passwordError = '';
		showPasswordModal = true;
	}

	async function changePassword(e: SubmitEvent) {
		e.preventDefault();
		if (newPassword !== newPasswordConfirm) {
			passwordError = '새 비밀번호가 서로 일치하지 않습니다.';
			return;
		}
		passwordError = '';
		changingPassword = true;
		try {
			await api.changePassword(username, currentPassword, newPassword);
			showPasswordModal = false;
		} catch (err) {
			passwordError = err instanceof Error ? err.message : String(err);
		} finally {
			changingPassword = false;
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

	let form = $state({
		name: '',
		loader: 'vanilla' as 'vanilla' | 'paper',
		mc_version: '',
		memory_gb: 2,
		cpu_quota_percent: 0, // 0 = unlimited
		accept_eula: false,
		// Paper servers sit behind CraftDeck's always-on Velocity proxy by
		// default (game_port stays internal-only) -- see handleCreateInstance.
		// Vanilla can't do modern forwarding at all, so it's always
		// independently exposed regardless of this flag.
		expose_independently: false
	});

	function openCreateForm() {
		showCreateForm = true;
	}

	// Caps the create-form memory slider at the Pi's actual RAM minus the
	// always-on Velocity proxy's fixed 1GB, same as the instance-settings
	// slider on the instance detail page -- both read from the same
	// /api/system/resources this page already polls for the resource-monitor
	// panel, so no extra request is needed here.
	let maxMemoryGB = $derived(
		resources
			? Math.max(1, Math.floor((resources.total_memory_mb - PROXY_RESERVED_MEMORY_MB) / 1024))
			: 1
	);

	// Version lists for the create-instance dropdown, fetched live from each
	// loader's own distribution API (the same ones internal/loader/*.go use
	// to actually download the server jar) so the list an operator picks
	// from always matches what's downloadable. Vanilla's manifest includes
	// snapshots, so it's filtered to release-only; Paper's API only ever
	// lists versions it has real builds for.
	let vanillaVersionIds = $state<string[]>([]);
	let paperVersionIds = $state<string[]>([]);
	let mcVersionsError = $state('');

	let availableVersionIds = $derived(form.loader === 'paper' ? paperVersionIds : vanillaVersionIds);

	async function loadMcVersions() {
		try {
			const [vanilla, paper] = await Promise.all([
				api.listVanillaVersions(),
				api.listPaperVersions()
			]);
			vanillaVersionIds = vanilla.filter((v) => v.type === 'release').map((v) => v.id);
			// PaperMC's v3 API already lists newest-first, same as vanilla's manifest.
			paperVersionIds = paper;
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

	async function refresh() {
		try {
			instances = await api.listInstances();
			loadError = '';
		} catch (err) {
			loadError = err instanceof Error ? err.message : String(err);
		}
	}

	let pollHandle: ReturnType<typeof setInterval>;
	let resourcePollHandle: ReturnType<typeof setInterval>;
	onMount(() => {
		refresh();
		refreshResources();
		loadMcVersions();
		api.authStatus().then((s) => {
			username = s.username;
			isLoggedIn = s.authenticated;
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
		creating = true;
		try {
			const created = await api.createInstance({
				name: form.name,
				kind: 'server',
				loader: form.loader,
				mc_version: form.mc_version,
				memory_max_mb: form.memory_gb * 1024,
				cpu_quota_percent: form.cpu_quota_percent,
				accept_eula: form.accept_eula,
				expose_independently: form.expose_independently
			});

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
				memory_gb: 2,
				cpu_quota_percent: 0,
				accept_eula: false,
				expose_independently: false
			};
			worldFile = null;
			worldFileForce = false;
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
	// the budget being negotiated among the listed servers excludes it too.
	let conflictMaxGB = $derived(
		resources
			? Math.max(1, Math.floor((resources.total_memory_mb - PROXY_RESERVED_MEMORY_MB) / 1024))
			: 1
	);
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
			if (target && resources && projectedMB > resources.total_memory_mb) {
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

	async function remove(id: string) {
		if (!confirm('이 인스턴스를 삭제할까요? 월드 데이터도 함께 지워집니다.')) return;
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
</script>

<main class="min-h-screen bg-background text-foreground p-8">
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
					onclick={openPasswordModal}
				>
					비밀번호 변경
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

	<div class="mt-6 grid grid-cols-1 gap-6 lg:grid-cols-3">
		<div class="space-y-3 lg:col-span-2">
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

		<div class="lg:col-span-1">
			<div class="border-border bg-card rounded-lg border p-4 lg:sticky lg:top-8">
				<h2 class="font-medium">라즈베리파이 리소스</h2>
				{#if resources}
					{@const memPercent = usagePercent(resources.used_memory_mb, resources.total_memory_mb)}
					{@const diskPercent = usagePercent(resources.used_disk_mb, resources.total_disk_mb)}
					<div class="mt-3 space-y-4">
						<div>
							<div class="mb-1 flex justify-between text-xs">
								<span class="text-muted-foreground">CPU 사용률</span>
								<span>{resources.cpu_percent.toFixed(0)}% ({resources.cpu_count}코어)</span>
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
								<span class="text-muted-foreground">메모리</span>
								<span
									>{(resources.used_memory_mb / 1024).toFixed(1)}GB / {(
										resources.total_memory_mb / 1024
									).toFixed(1)}GB</span
								>
							</div>
							<div class="bg-background h-2 overflow-hidden rounded-full">
								<div
									class="h-full {barClass(memPercent)}"
									style="width: {memPercent}%"
								></div>
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
					<select
						id="loader"
						bind:value={form.loader}
						onchange={onLoaderChange}
						class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
					>
						<option value="vanilla">Vanilla</option>
						<option value="paper">Paper</option>
					</select>
				</div>
				{#if form.loader === 'paper'}
					<label class="flex items-start gap-2 text-sm">
						<input type="checkbox" bind:checked={form.expose_independently} class="mt-1" />
						<span>
							독립적으로 외부에 노출 (기본은 항상 켜져 있는 Velocity 프록시 뒤에 자동 등록되며,
							게임 포트는 내부용으로만 쓰입니다)
						</span>
					</label>
				{:else}
					<p class="text-muted-foreground text-xs">
						Vanilla는 프록시의 모던 포워딩을 지원하지 않아 항상 독립적으로 노출됩니다.
					</p>
				{/if}
				<div>
					<label class="mb-1 block text-sm font-medium" for="mc_version">마인크래프트 버전</label>
					{#if mcVersionsError}
						<p class="text-destructive text-xs">
							버전 목록을 불러오지 못했습니다: {mcVersionsError}
						</p>
					{:else if availableVersionIds.length === 0}
						<p class="text-muted-foreground text-xs">버전 목록 불러오는 중...</p>
					{:else}
						<select
							id="mc_version"
							required
							bind:value={form.mc_version}
							class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
						>
							{#each availableVersionIds as id}
								<option value={id}>{id}</option>
							{/each}
						</select>
					{/if}
				</div>
				<div>
					<label class="mb-1 block text-sm font-medium" for="create-memory">
						최대 메모리 ({form.memory_gb}GB / 최대 {maxMemoryGB}GB)
					</label>
					<input
						id="create-memory"
						type="range"
						min="1"
						max={maxMemoryGB}
						step="1"
						bind:value={form.memory_gb}
						class="w-full"
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

{#if showPasswordModal}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
		onclick={() => (showPasswordModal = false)}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="border-border bg-card w-full max-w-sm rounded-lg border p-4 shadow-lg"
			onclick={(e) => e.stopPropagation()}
		>
			<div class="mb-3 flex items-center justify-between">
				<h2 class="font-medium">비밀번호 변경</h2>
				<button
					type="button"
					class="text-muted-foreground text-sm"
					onclick={() => (showPasswordModal = false)}>&times;</button
				>
			</div>
			<form class="space-y-4" onsubmit={changePassword}>
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
				<button
					type="submit"
					disabled={changingPassword}
					class="bg-primary text-primary-foreground w-full rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
				>
					{changingPassword ? '변경 중...' : '변경'}
				</button>
			</form>
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
				실행하려는 서버들의 메모리 할당 합이 라즈베리파이의 전체 메모리를 초과합니다. 아래에서
				조정한 뒤 시작할 수 있습니다.
			</p>

			<div class="mt-3 space-y-3">
				{#each conflictItems as item (item.id)}
					<div>
						<label class="mb-1 flex items-center justify-between text-xs" for="conflict-{item.id}">
							<span>
								{item.name}
								{#if item.isTarget}<span class="text-muted-foreground">(시작 예정)</span>
								{:else if item.isRunning}<span class="text-muted-foreground"
										>(실행 중 -- 재시작해야 반영됨)</span
									>{/if}
							</span>
							<span>{item.memoryGB}GB</span>
						</label>
						<input
							id="conflict-{item.id}"
							type="range"
							min="1"
							max={conflictMaxGB}
							step="1"
							bind:value={item.memoryGB}
							class="w-full"
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

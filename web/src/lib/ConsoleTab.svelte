<script lang="ts">
	import type { Instance, OpEntry } from '$lib/api';

	let {
		logEl = $bindable(),
		inst,
		wsStatus,
		lines,
		parseLogLine,
		commandText = $bindable(''),
		onSubmitFreeform,
		onlinePlayers,
		onRefreshPlayerList,
		playerName = $bindable(''),
		onOpenReasonModal,
		onPardonPlayer,
		onWhitelistAdd,
		onWhitelistRemove,
		onOpPlayer,
		onDeopPlayer,
		bannedPlayers,
		onRefreshBans,
		ops,
		onRefreshOps,
		whitelistedPlayers,
		whitelistEnabled,
		onRefreshWhitelist,
		onWhitelistToggle,
		announceText = $bindable(''),
		onSendCommand,
		gamemode = $bindable('survival'),
		difficulty = $bindable('easy')
	}: {
		logEl?: HTMLDivElement;
		inst: Instance | null;
		wsStatus: 'connecting' | 'open' | 'closed';
		lines: string[];
		parseLogLine: (line: string) => { prefix: string; message: string; messageClass: string };
		commandText: string;
		onSubmitFreeform: (e: SubmitEvent) => void;
		onlinePlayers: string[];
		onRefreshPlayerList: () => void;
		playerName: string;
		onOpenReasonModal: (kind: 'kick' | 'ban') => void;
		onPardonPlayer: () => void;
		onWhitelistAdd: () => void;
		onWhitelistRemove: () => void;
		onOpPlayer: () => void;
		onDeopPlayer: () => void;
		bannedPlayers: string[];
		onRefreshBans: () => void;
		ops: OpEntry[];
		onRefreshOps: () => void;
		whitelistedPlayers: string[];
		whitelistEnabled: boolean;
		onRefreshWhitelist: () => void;
		onWhitelistToggle: (on: boolean) => void;
		announceText: string;
		onSendCommand: (command: string) => void;
		gamemode: string;
		difficulty: string;
	} = $props();
</script>

<!-- Live console: FR-14, FR-15, FR-20 -->
<div
	class="border-border bg-card rounded-lg border p-4 lg:flex lg:min-h-0 lg:flex-col {inst?.kind === 'proxy'
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
	<form class="mt-3 flex gap-2" onsubmit={onSubmitFreeform}>
		<input
			bind:value={commandText}
			placeholder="명령어 직접 입력 (예: say hello)"
			class="border-input bg-background flex-1 rounded-md border px-3 py-2 font-mono text-sm"
		/>
		<button type="submit" class="bg-primary text-primary-foreground rounded-md px-4 py-2 text-sm font-medium"
			>전송</button
		>
	</form>
</div>

<!-- GUI command buttons: FR-17, FR-18, FR-19, FR-20. Velocity has no RCON in
	this MVP, so none of these apply to a proxy instance. -->
{#if inst?.kind === 'server'}
	<div class="border-border bg-card space-y-4 rounded-lg border p-4 lg:min-h-0 lg:overflow-y-auto">
		<h2 class="font-medium">자주 쓰는 명령</h2>

		<div class="flex gap-2">
			<button
				class="border-border flex-1 rounded-md border px-3 py-1.5 text-sm"
				onclick={() => onSendCommand('save-all')}>월드 저장</button
			>
		</div>

		<div>
			<div class="mb-1 flex items-center justify-between">
				<label class="text-muted-foreground block text-xs" for="player">플레이어</label>
				<button
					type="button"
					class="text-muted-foreground text-xs underline"
					onclick={onRefreshPlayerList}>새로고침</button
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
					onclick={() => onOpenReasonModal('kick')}>강제 퇴장</button
				>
				<button
					class="border-border rounded-md border px-2 py-1.5 text-xs"
					disabled={!playerName}
					onclick={() => onOpenReasonModal('ban')}>밴</button
				>
				<button class="border-border rounded-md border px-2 py-1.5 text-xs" onclick={onPardonPlayer}
					>밴 해제</button
				>
				<button class="border-border rounded-md border px-2 py-1.5 text-xs" onclick={onWhitelistAdd}
					>화이트리스트 추가</button
				>
				<button class="border-border rounded-md border px-2 py-1.5 text-xs" onclick={onWhitelistRemove}
					>화이트리스트 삭제</button
				>
				<button class="border-border rounded-md border px-2 py-1.5 text-xs" onclick={onOpPlayer}
					>운영자 부여</button
				>
				<button class="border-border rounded-md border px-2 py-1.5 text-xs" onclick={onDeopPlayer}
					>운영자 해제</button
				>
			</div>
		</div>

		<!-- Ban list -->
		<div>
			<div class="mb-1 flex items-center justify-between">
				<span class="text-muted-foreground text-xs">밴 목록</span>
				<button type="button" class="text-muted-foreground text-xs underline" onclick={onRefreshBans}
					>새로고침</button
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
				<button type="button" class="text-muted-foreground text-xs underline" onclick={onRefreshOps}
					>새로고침</button
				>
			</div>
			{#if ops.length > 0}
				<div class="flex flex-wrap gap-1.5">
					{#each ops as opEntry}
						<button
							type="button"
							class="border-border rounded-full border px-2 py-0.5 text-xs {playerName === opEntry.name
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
					onclick={onRefreshWhitelist}>새로고침</button
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
				onclick={() => onWhitelistToggle(true)}>화이트리스트 켜기</button
			>
			<button
				class="border-border rounded-md border px-2 py-1.5 text-xs"
				onclick={() => onWhitelistToggle(false)}>화이트리스트 끄기</button
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
						if (e.key === 'Enter') onSendCommand(`say ${announceText}`);
					}}
				/>
				<button
					class="border-border shrink-0 rounded-md border px-3 py-1.5 text-sm"
					onclick={() => onSendCommand(`say ${announceText}`)}>방송</button
				>
			</div>
		</div>

		<div class="grid grid-cols-2 gap-2">
			<div>
				<label
					class="text-muted-foreground mb-1 block truncate text-xs"
					for="gamemode"
					title="대상: {playerName || '미지정'}"
				>
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
					onclick={() => onSendCommand(`gamemode ${gamemode} ${playerName}`)}>적용</button
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
					onclick={() => onSendCommand(`difficulty ${difficulty}`)}>적용</button
				>
			</div>
		</div>

		<div>
			<span class="text-muted-foreground mb-1 block text-xs">시간</span>
			<div class="flex gap-2">
				<button
					class="border-border flex-1 rounded-md border px-2 py-1.5 text-xs"
					onclick={() => onSendCommand('time set day')}>낮</button
				>
				<button
					class="border-border flex-1 rounded-md border px-2 py-1.5 text-xs"
					onclick={() => onSendCommand('time set night')}>밤</button
				>
			</div>
		</div>

		<div>
			<span class="text-muted-foreground mb-1 block text-xs">날씨</span>
			<div class="flex gap-2">
				<button
					class="border-border flex-1 rounded-md border px-2 py-1.5 text-xs"
					onclick={() => onSendCommand('weather clear')}>맑음</button
				>
				<button
					class="border-border flex-1 rounded-md border px-2 py-1.5 text-xs"
					onclick={() => onSendCommand('weather rain')}>비</button
				>
				<button
					class="border-border flex-1 rounded-md border px-2 py-1.5 text-xs"
					onclick={() => onSendCommand('weather thunder')}>뇌우</button
				>
			</div>
		</div>
	</div>
{/if}

<script lang="ts">
	import type { Instance, OpEntry } from '$lib/api';
	import { t } from '$lib/i18n';

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
		<h2 class="font-medium">{$t('consoleTab.console.title')}</h2>
		<span class="text-muted-foreground text-xs">
			{wsStatus === 'open'
				? $t('consoleTab.console.statusOpen')
				: wsStatus === 'connecting'
					? $t('consoleTab.console.statusConnecting')
					: $t('consoleTab.console.statusClosed')}
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
			placeholder={$t('consoleTab.console.commandPlaceholder')}
			class="border-input bg-background flex-1 rounded-md border px-3 py-2 font-mono text-sm"
		/>
		<button type="submit" class="bg-primary text-primary-foreground rounded-md px-4 py-2 text-sm font-medium"
			>{$t('consoleTab.console.send')}</button
		>
	</form>
</div>

<!-- GUI command buttons: FR-17, FR-18, FR-19, FR-20. Velocity has no RCON in
	this MVP, so none of these apply to a proxy instance. -->
{#if inst?.kind === 'server'}
	<div class="border-border bg-card space-y-4 rounded-lg border p-4 lg:min-h-0 lg:overflow-y-auto">
		<h2 class="font-medium">{$t('consoleTab.commands.title')}</h2>

		<div class="flex gap-2">
			<button
				class="border-border flex-1 rounded-md border px-3 py-1.5 text-sm"
				onclick={() => onSendCommand('save-all')}>{$t('consoleTab.commands.saveAll')}</button
			>
		</div>

		<div>
			<div class="mb-1 flex items-center justify-between">
				<label class="text-muted-foreground block text-xs" for="player">{$t('consoleTab.player.label')}</label>
				<button
					type="button"
					class="text-muted-foreground text-xs underline"
					onclick={onRefreshPlayerList}>{$t('consoleTab.player.refresh')}</button
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
				<p class="text-muted-foreground mb-2 text-xs">{$t('consoleTab.player.none')}</p>
			{/if}
			<div class="flex gap-2">
				<input
					id="player"
					bind:value={playerName}
					placeholder={$t('consoleTab.player.placeholder')}
					class="border-input bg-background w-full min-w-0 flex-1 rounded-md border px-2 py-1.5 text-sm"
				/>
			</div>
			<div class="mt-2 grid grid-cols-2 gap-2">
				<button
					class="border-border col-span-2 rounded-md border px-2 py-1.5 text-xs"
					disabled={!playerName}
					onclick={() => onOpenReasonModal('kick')}>{$t('consoleTab.player.kick')}</button
				>
				<button
					class="border-border rounded-md border px-2 py-1.5 text-xs"
					disabled={!playerName}
					onclick={() => onOpenReasonModal('ban')}>{$t('consoleTab.player.ban')}</button
				>
				<button class="border-border rounded-md border px-2 py-1.5 text-xs" onclick={onPardonPlayer}
					>{$t('consoleTab.player.pardon')}</button
				>
				<button class="border-border rounded-md border px-2 py-1.5 text-xs" onclick={onWhitelistAdd}
					>{$t('consoleTab.player.whitelistAdd')}</button
				>
				<button class="border-border rounded-md border px-2 py-1.5 text-xs" onclick={onWhitelistRemove}
					>{$t('consoleTab.player.whitelistRemove')}</button
				>
				<button class="border-border rounded-md border px-2 py-1.5 text-xs" onclick={onOpPlayer}
					>{$t('consoleTab.player.op')}</button
				>
				<button class="border-border rounded-md border px-2 py-1.5 text-xs" onclick={onDeopPlayer}
					>{$t('consoleTab.player.deop')}</button
				>
			</div>
		</div>

		<!-- Ban list -->
		<div>
			<div class="mb-1 flex items-center justify-between">
				<span class="text-muted-foreground text-xs">{$t('consoleTab.bans.title')}</span>
				<button type="button" class="text-muted-foreground text-xs underline" onclick={onRefreshBans}
					>{$t('consoleTab.player.refresh')}</button
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
				<p class="text-muted-foreground text-xs">{$t('consoleTab.bans.none')}</p>
			{/if}
		</div>

		<!-- Op list -->
		<div>
			<div class="mb-1 flex items-center justify-between">
				<span class="text-muted-foreground text-xs">{$t('consoleTab.ops.title')}</span>
				<button type="button" class="text-muted-foreground text-xs underline" onclick={onRefreshOps}
					>{$t('consoleTab.player.refresh')}</button
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
							title={$t('consoleTab.ops.levelTitle', { level: opEntry.level })}
						>
							{opEntry.name}
						</button>
					{/each}
				</div>
			{:else}
				<p class="text-muted-foreground text-xs">{$t('consoleTab.ops.none')}</p>
			{/if}
		</div>

		<!-- Whitelist -->
		<div>
			<div class="mb-1 flex items-center justify-between">
				<span class="text-muted-foreground text-xs">{$t('consoleTab.whitelist.title')}</span>
				<button
					type="button"
					class="text-muted-foreground text-xs underline"
					onclick={onRefreshWhitelist}>{$t('consoleTab.player.refresh')}</button
				>
			</div>
			{#if !whitelistEnabled}
				<p class="text-muted-foreground text-xs">{$t('consoleTab.whitelist.disabled')}</p>
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
				<p class="text-muted-foreground text-xs">{$t('consoleTab.whitelist.none')}</p>
			{/if}
		</div>

		<div class="flex gap-2">
			<button
				class="border-border rounded-md border px-2 py-1.5 text-xs"
				onclick={() => onWhitelistToggle(true)}>{$t('consoleTab.whitelist.on')}</button
			>
			<button
				class="border-border rounded-md border px-2 py-1.5 text-xs"
				onclick={() => onWhitelistToggle(false)}>{$t('consoleTab.whitelist.off')}</button
			>
		</div>

		<div>
			<label class="text-muted-foreground mb-1 block text-xs" for="announce"
				>{$t('consoleTab.announce.label')}</label
			>
			<div class="flex gap-2">
				<input
					id="announce"
					bind:value={announceText}
					placeholder={$t('consoleTab.announce.placeholder')}
					class="border-input bg-background w-full min-w-0 flex-1 rounded-md border px-2 py-1.5 text-sm"
					onkeydown={(e) => {
						if (e.key === 'Enter') onSendCommand(`say ${announceText}`);
					}}
				/>
				<button
					class="border-border shrink-0 rounded-md border px-3 py-1.5 text-sm"
					onclick={() => onSendCommand(`say ${announceText}`)}>{$t('consoleTab.announce.send')}</button
				>
			</div>
		</div>

		<div class="grid grid-cols-2 gap-2">
			<div>
				<label
					class="text-muted-foreground mb-1 block truncate text-xs"
					for="gamemode"
					title={$t('consoleTab.gamemode.label', {
						player: playerName || $t('consoleTab.gamemode.unspecified')
					})}
				>
					{$t('consoleTab.gamemode.label', {
						player: playerName || $t('consoleTab.gamemode.unspecified')
					})}
				</label>
				<div class="flex gap-2">
					<select
						id="gamemode"
						bind:value={gamemode}
						class="border-input bg-background w-full rounded-md border px-2 py-1.5 text-sm"
					>
						<option value="survival">{$t('consoleTab.gamemode.survival')}</option>
						<option value="creative">{$t('consoleTab.gamemode.creative')}</option>
						<option value="adventure">{$t('consoleTab.gamemode.adventure')}</option>
						<option value="spectator">{$t('consoleTab.gamemode.spectator')}</option>
					</select>
				</div>
				<button
					class="border-border mt-2 w-full rounded-md border px-2 py-1.5 text-xs"
					disabled={!playerName}
					onclick={() => onSendCommand(`gamemode ${gamemode} ${playerName}`)}
					>{$t('consoleTab.gamemode.apply')}</button
				>
			</div>
			<div>
				<label class="text-muted-foreground mb-1 block text-xs" for="difficulty"
					>{$t('consoleTab.difficulty.label')}</label
				>
				<select
					id="difficulty"
					bind:value={difficulty}
					class="border-input bg-background w-full rounded-md border px-2 py-1.5 text-sm"
				>
					<option value="peaceful">{$t('consoleTab.difficulty.peaceful')}</option>
					<option value="easy">{$t('consoleTab.difficulty.easy')}</option>
					<option value="normal">{$t('consoleTab.difficulty.normal')}</option>
					<option value="hard">{$t('consoleTab.difficulty.hard')}</option>
				</select>
				<button
					class="border-border mt-2 w-full rounded-md border px-2 py-1.5 text-xs"
					onclick={() => onSendCommand(`difficulty ${difficulty}`)}
					>{$t('consoleTab.difficulty.apply')}</button
				>
			</div>
		</div>

		<div>
			<span class="text-muted-foreground mb-1 block text-xs">{$t('consoleTab.time.label')}</span>
			<div class="flex gap-2">
				<button
					class="border-border flex-1 rounded-md border px-2 py-1.5 text-xs"
					onclick={() => onSendCommand('time set day')}>{$t('consoleTab.time.day')}</button
				>
				<button
					class="border-border flex-1 rounded-md border px-2 py-1.5 text-xs"
					onclick={() => onSendCommand('time set night')}>{$t('consoleTab.time.night')}</button
				>
			</div>
		</div>

		<div>
			<span class="text-muted-foreground mb-1 block text-xs">{$t('consoleTab.weather.label')}</span>
			<div class="flex gap-2">
				<button
					class="border-border flex-1 rounded-md border px-2 py-1.5 text-xs"
					onclick={() => onSendCommand('weather clear')}>{$t('consoleTab.weather.clear')}</button
				>
				<button
					class="border-border flex-1 rounded-md border px-2 py-1.5 text-xs"
					onclick={() => onSendCommand('weather rain')}>{$t('consoleTab.weather.rain')}</button
				>
				<button
					class="border-border flex-1 rounded-md border px-2 py-1.5 text-xs"
					onclick={() => onSendCommand('weather thunder')}>{$t('consoleTab.weather.thunder')}</button
				>
			</div>
		</div>
	</div>
{/if}

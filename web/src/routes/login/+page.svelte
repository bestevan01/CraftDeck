<script lang="ts">
	import { api } from '$lib/api';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';

	let mode = $state<'loading' | 'setup' | 'login'>('loading');
	let username = $state('');
	let password = $state('');
	let error = $state('');
	let submitting = $state(false);
	// FR-37: set once handleLogin reports totp_required for this
	// username/password -- shows the code field and resubmits the same
	// credentials with it filled in.
	let needsTotp = $state(false);
	let totpCode = $state('');

	onMount(async () => {
		try {
			const status = await api.authStatus();
			if (status.setup_required) {
				mode = 'setup';
				return;
			}
			// lan_bypass: the backend doesn't actually require a session for
			// this client, so there's nothing to log into -- just go back.
			if (status.authenticated || status.lan_bypass) {
				await goto('/');
				return;
			}
			mode = 'login';
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		}
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		submitting = true;
		try {
			if (mode === 'setup') {
				await api.setup(username, password);
			} else {
				const result = await api.login(username, password, needsTotp ? totpCode : undefined);
				if (result.totp_required) {
					// Not an error -- the password was correct, this account
					// just also has 2FA enabled. A separate screen (rather
					// than a field quietly appearing below the password box)
					// makes it unambiguous that this is a distinct step, not
					// a glitch.
					needsTotp = true;
					submitting = false;
					return;
				}
			}
			await goto('/');
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		} finally {
			submitting = false;
		}
	}

	// Drops back to the username/password screen -- clears the password
	// (and any leftover code) rather than leaving a verified-but-unused
	// credential sitting in memory, same reasoning as clearing other
	// one-time-use secrets elsewhere in this app (e.g. domain tokens).
	function backToCredentials() {
		needsTotp = false;
		totpCode = '';
		password = '';
		error = '';
	}
</script>

<main class="bg-background text-foreground flex min-h-screen items-center justify-center p-8">
	<div class="border-border bg-card w-full max-w-sm rounded-lg border p-6">
		<h1 class="text-xl font-semibold">CraftDeck</h1>

		{#if mode === 'loading'}
			<p class="text-muted-foreground mt-4 text-sm">불러오는 중...</p>
		{:else if needsTotp}
			<!-- FR-37: a separate screen once the password's already been
				verified -- makes it unambiguous that this is a distinct step
				(not the same form growing a field, which could look like a
				glitch). -->
			<p class="text-muted-foreground mt-1 text-sm">
				비밀번호가 확인됐습니다. 이 계정은 2단계 인증이 켜져 있어, 인증 앱의 코드를 마저
				입력해야 합니다.
			</p>
			<form class="mt-4 space-y-4" onsubmit={submit}>
				<div>
					<label class="mb-1 block text-sm font-medium" for="totp-code">2단계 인증 코드</label>
					<!-- svelte-ignore a11y_autofocus -->
					<input
						id="totp-code"
						type="text"
						inputmode="numeric"
						autocomplete="one-time-code"
						autofocus
						required
						placeholder="인증 앱의 6자리 코드 (또는 백업 코드)"
						bind:value={totpCode}
						class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
					/>
				</div>
				{#if error}
					<p class="text-destructive text-sm">{error}</p>
				{/if}
				<button
					type="submit"
					disabled={submitting}
					class="bg-primary text-primary-foreground w-full rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
				>
					{submitting ? '확인 중...' : '확인'}
				</button>
				<button
					type="button"
					class="text-muted-foreground w-full text-center text-xs hover:underline"
					onclick={backToCredentials}
				>
					다른 계정으로 로그인
				</button>
			</form>
		{:else}
			{#if mode === 'setup'}
				<p class="text-muted-foreground mt-1 text-sm">
					처음 실행이네요. 관리자 계정을 만들어주세요.
				</p>
			{:else}
				<p class="text-muted-foreground mt-1 text-sm">로그인해주세요.</p>
			{/if}

			<form class="mt-4 space-y-4" method="post" onsubmit={submit}>
				<div>
					<label class="mb-1 block text-sm font-medium" for="username">아이디</label>
					<input
						id="username"
						name="username"
						required
						autocomplete="username"
						bind:value={username}
						class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
					/>
				</div>
				<div>
					<label class="mb-1 block text-sm font-medium" for="password">비밀번호</label>
					<input
						id="password"
						name="password"
						type="password"
						required
						minlength={mode === 'setup' ? 8 : undefined}
						autocomplete={mode === 'setup' ? 'new-password' : 'current-password'}
						bind:value={password}
						class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
					/>
					{#if mode === 'setup'}
						<p class="text-muted-foreground mt-1 text-xs">8자 이상</p>
					{/if}
				</div>
				{#if error}
					<p class="text-destructive text-sm">{error}</p>
				{/if}
				<button
					type="submit"
					disabled={submitting}
					class="bg-primary text-primary-foreground w-full rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
				>
					{#if submitting}
						처리 중...
					{:else if mode === 'setup'}
						계정 만들기
					{:else}
						로그인
					{/if}
				</button>
			</form>
		{/if}
	</div>
</main>

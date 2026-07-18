<script lang="ts">
	import { api } from '$lib/api';
	import TwoFactorModal from '$lib/TwoFactorModal.svelte';

	// 비밀번호 변경은 여기서 바로 처리하고, 2단계 인증은 QR 스캔 등 별도
	// 흐름이라 이 모달의 버튼에서 여는 별도 모달(TwoFactorModal)로 뺐다.
	let {
		open = $bindable(false),
		username = $bindable(''),
		totpEnabled = $bindable(false)
	}: {
		open: boolean;
		username: string;
		totpEnabled: boolean;
	} = $props();

	let currentPassword = $state('');
	let newPassword = $state('');
	let newPasswordConfirm = $state('');
	let passwordError = $state('');
	let passwordChanged = $state(false);
	let changingPassword = $state(false);
	let showTwoFactorModal = $state(false);

	// Resets the password form's transient state each time the modal opens.
	$effect(() => {
		if (!open) return;
		currentPassword = '';
		newPassword = '';
		newPasswordConfirm = '';
		passwordError = '';
		passwordChanged = false;
	});

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
			currentPassword = '';
			newPassword = '';
			newPasswordConfirm = '';
			passwordChanged = true;
		} catch (err) {
			passwordError = err instanceof Error ? err.message : String(err);
		} finally {
			changingPassword = false;
		}
	}
</script>

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-8"
		onclick={() => (open = false)}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="border-border bg-card max-h-[85vh] w-full max-w-sm overflow-y-auto rounded-lg border p-4 shadow-lg"
			onclick={(e) => e.stopPropagation()}
		>
			<div class="mb-3 flex items-center justify-between">
				<h2 class="font-medium">계정 설정</h2>
				<button type="button" class="text-muted-foreground text-sm" onclick={() => (open = false)}
					>&times;</button
				>
			</div>

			<h3 class="text-sm font-medium">비밀번호 변경</h3>
			<form class="mt-3 space-y-4" onsubmit={changePassword}>
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
					<label class="mb-1 block text-sm font-medium" for="pw-new-confirm">새 비밀번호 확인</label>
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
				{#if passwordChanged}
					<p class="text-sm text-green-500">비밀번호가 변경되었습니다.</p>
				{/if}
				<button
					type="submit"
					disabled={changingPassword}
					class="bg-primary text-primary-foreground w-full rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
				>
					{changingPassword ? '변경 중...' : '변경'}
				</button>
			</form>

			<hr class="border-border my-4" />

			<div class="flex items-center justify-between">
				<div>
					<h3 class="text-sm font-medium">2단계 인증</h3>
					<p class="text-muted-foreground mt-1 text-xs">
						{totpEnabled ? '설정되어 있습니다.' : '아직 설정하지 않았습니다.'}
					</p>
				</div>
				<button
					type="button"
					class="border-border shrink-0 rounded-md border px-3 py-1.5 text-xs"
					onclick={() => (showTwoFactorModal = true)}
				>
					{totpEnabled ? '관리' : '설정'}
				</button>
			</div>
		</div>
	</div>
{/if}

<TwoFactorModal bind:open={showTwoFactorModal} bind:totpEnabled />

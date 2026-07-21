<script lang="ts">
	import { api } from '$lib/api';
	import TwoFactorModal from '$lib/TwoFactorModal.svelte';
	import { locale, setLocale, t, type Locale } from '$lib/i18n';

	// 예전엔 헤더의 "계정 설정" 버튼이 여는 모달이었지만, LAN 접속(lan_bypass)
	// 상태에서는 실제 로그인 세션이 없어 그 버튼 자체가 안 보였다 --
	// 비밀번호/2단계 인증/언어처럼 세션 유무와 무관하게 접근 가능해야 하는
	// 설정들이 막혀 있었던 셈이라, 전역 설정 탭 안의 "계정" 서브탭으로
	// 옮겨서 언제나 접근 가능하게 한다. 비밀번호 변경은 여기서 바로
	// 처리하고, 2단계 인증은 QR 스캔 등 별도 흐름이라 별도 모달
	// (TwoFactorModal)로 뺐다.
	let {
		username = $bindable(''),
		totpEnabled = $bindable(false),
		onStartTour
	}: {
		username: string;
		totpEnabled: boolean;
		onStartTour: () => void;
	} = $props();

	let currentPassword = $state('');
	let newPassword = $state('');
	let newPasswordConfirm = $state('');
	let passwordError = $state('');
	let passwordChanged = $state(false);
	let changingPassword = $state(false);
	let showTwoFactorModal = $state(false);

	async function changePassword(e: SubmitEvent) {
		e.preventDefault();
		if (newPassword !== newPasswordConfirm) {
			passwordError = $t('accountModal.password.mismatchError');
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

<div class="space-y-4">
	<div class="border-border bg-card rounded-lg border p-4">
		<h2 class="font-medium">{$t('accountModal.password.title')}</h2>
		<form class="mt-3 space-y-4" onsubmit={changePassword}>
			<div>
				<label class="mb-1 block text-sm font-medium" for="pw-username"
					>{$t('accountModal.password.usernameLabel')}</label
				>
				<input
					id="pw-username"
					required
					autocomplete="username"
					bind:value={username}
					class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
				/>
			</div>
			<div>
				<label class="mb-1 block text-sm font-medium" for="pw-current"
					>{$t('accountModal.password.currentLabel')}</label
				>
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
				<label class="mb-1 block text-sm font-medium" for="pw-new"
					>{$t('accountModal.password.newLabel')}</label
				>
				<input
					id="pw-new"
					type="password"
					required
					minlength="8"
					autocomplete="new-password"
					bind:value={newPassword}
					class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
				/>
				<p class="text-muted-foreground mt-1 text-xs">{$t('accountModal.password.newHint')}</p>
			</div>
			<div>
				<label class="mb-1 block text-sm font-medium" for="pw-new-confirm"
					>{$t('accountModal.password.confirmLabel')}</label
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
			{#if passwordChanged}
				<p class="text-sm text-green-500">{$t('accountModal.password.changed')}</p>
			{/if}
			<button
				type="submit"
				disabled={changingPassword}
				class="bg-primary text-primary-foreground w-full rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
			>
				{changingPassword ? $t('accountModal.password.changing') : $t('accountModal.password.changeButton')}
			</button>
		</form>
	</div>

	<div class="border-border bg-card flex items-center justify-between rounded-lg border p-4">
		<div>
			<h2 class="font-medium">{$t('accountModal.twoFactor.title')}</h2>
			<p class="text-muted-foreground mt-1 text-xs">
				{totpEnabled ? $t('accountModal.twoFactor.enabled') : $t('accountModal.twoFactor.disabled')}
			</p>
		</div>
		<button
			type="button"
			class="border-border shrink-0 rounded-md border px-3 py-1.5 text-xs"
			onclick={() => (showTwoFactorModal = true)}
		>
			{totpEnabled ? $t('accountModal.twoFactor.manageButton') : $t('accountModal.twoFactor.setupButton')}
		</button>
	</div>

	<div class="border-border bg-card flex items-center justify-between rounded-lg border p-4">
		<h2 class="font-medium">{$t('common.language.label')}</h2>
		<div class="relative w-40">
			<select
				value={$locale}
				onchange={(e) => setLocale(e.currentTarget.value as Locale)}
				class="border-input bg-background w-full appearance-none rounded-md border py-1.5 pr-8 pl-3 text-sm"
			>
				<option value="ko">{$t('common.language.ko')}</option>
				<option value="en">{$t('common.language.en')}</option>
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
	</div>

	<div class="border-border bg-card flex items-center justify-between rounded-lg border p-4">
		<div>
			<h2 class="font-medium">{$t('accountModal.tour.title')}</h2>
			<p class="text-muted-foreground mt-1 text-xs">{$t('accountModal.tour.description')}</p>
		</div>
		<button
			type="button"
			class="border-border shrink-0 rounded-md border px-3 py-1.5 text-xs"
			onclick={onStartTour}
		>
			{$t('accountModal.tour.replayButton')}
		</button>
	</div>
</div>

<TwoFactorModal bind:open={showTwoFactorModal} bind:totpEnabled />

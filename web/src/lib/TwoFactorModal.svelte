<script lang="ts">
	import { api } from '$lib/api';
	import { untrack } from 'svelte';
	import CopyButton from '$lib/CopyButton.svelte';
	import { t } from '$lib/i18n';

	// FR-38/39: 2단계 인증 등록/현황 -- 계정 설정 모달과 분리된 별도 모달로,
	// 그 모달의 버튼에서 열린다.
	let {
		open = $bindable(false),
		totpEnabled = $bindable(false)
	}: {
		open: boolean;
		totpEnabled: boolean;
	} = $props();

	let totpQrCode = $state('');
	let totpSecret = $state('');
	let totpVerifyCode = $state('');
	let totpError = $state('');
	let startingTOTPSetup = $state(false);
	let verifyingTOTP = $state(false);
	let totpBackupCodes = $state<string[] | null>(null);
	let regenerating = $state(false);
	let regenerateError = $state('');
	let showDisableForm = $state(false);
	let disablePassword = $state('');
	let disabling = $state(false);
	let disableError = $state('');
	let justDisabled = $state(false);

	// Resets transient state and (if not already enabled) kicks off a fresh
	// TOTP setup each time the modal opens. Only tracks `open` as a
	// dependency (via untrack on totpEnabled) -- otherwise, since
	// totpEnabled flips true right after a successful verify (see
	// submitTOTPVerify below) while the modal is still open, this effect
	// would re-run from that write alone and immediately null out the
	// backup codes it just received before the operator ever saw them
	// (confirmed: that's exactly what was happening).
	$effect(() => {
		if (!open) return;
		totpError = '';
		totpVerifyCode = '';
		totpBackupCodes = null;
		regenerateError = '';
		showDisableForm = false;
		disablePassword = '';
		disableError = '';
		justDisabled = false;
		if (untrack(() => totpEnabled)) return; // already set up -- see the "already enabled" branch below
		startingTOTPSetup = true;
		api
			.setupTOTP()
			.then((setup) => {
				totpQrCode = setup.qr_code_png;
				totpSecret = setup.secret;
			})
			.catch((err) => {
				totpError = err instanceof Error ? err.message : String(err);
			})
			.finally(() => {
				startingTOTPSetup = false;
			});
	});

	async function submitTOTPVerify(e: SubmitEvent) {
		e.preventDefault();
		totpError = '';
		verifyingTOTP = true;
		try {
			const result = await api.verifyTOTP(totpVerifyCode);
			totpBackupCodes = result.backup_codes;
			totpEnabled = true;
		} catch (err) {
			totpError = err instanceof Error ? err.message : String(err);
		} finally {
			verifyingTOTP = false;
		}
	}

	// Replaces the account's backup codes wholesale (old ones stop working)
	// -- for an operator who's used most of theirs up, without having to
	// turn 2FA off and re-enroll from scratch just to get a fresh set.
	async function regenerateBackupCodes() {
		regenerateError = '';
		regenerating = true;
		try {
			const result = await api.regenerateBackupCodes();
			totpBackupCodes = result.backup_codes;
		} catch (err) {
			regenerateError = err instanceof Error ? err.message : String(err);
		} finally {
			regenerating = false;
		}
	}

	async function submitDisable(e: SubmitEvent) {
		e.preventDefault();
		disableError = '';
		disabling = true;
		try {
			await api.disableTOTP(disablePassword);
			totpEnabled = false;
			disablePassword = '';
			showDisableForm = false;
			justDisabled = true;
		} catch (err) {
			disableError = err instanceof Error ? err.message : String(err);
		} finally {
			disabling = false;
		}
	}

	let pressedBackdrop = false;
</script>

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-8"
		onmousedown={(e) => (pressedBackdrop = e.target === e.currentTarget)}
		onclick={(e) => {
			if (pressedBackdrop && e.target === e.currentTarget) open = false;
		}}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="border-border bg-card max-h-[85vh] w-full max-w-sm overflow-y-auto rounded-lg border p-4 shadow-lg"
		>
			<div class="mb-3 flex items-center justify-between">
				<h2 class="font-medium">{$t('twoFactorModal.title')}</h2>
				<button type="button" class="text-muted-foreground text-sm" onclick={() => (open = false)}
					>&times;</button
				>
			</div>

			<!-- QR 스캔 후 코드 한 번 검증해야 실제로 켜진다(submitTOTPVerify).
				이미 켜져 있으면 재설정 대신 안내와 백업 코드 재발급/끄기를
				보여준다 (setupTOTP이 409를 반환하므로 백엔드와 일관됨). -->
			{#if justDisabled}
				<p class="text-sm">{$t('twoFactorModal.disabledNotice')}</p>
			{:else if totpEnabled && !totpBackupCodes}
				<p class="text-muted-foreground text-sm">
					{$t('twoFactorModal.alreadySetupNotice')}
				</p>
				<button
					type="button"
					class="border-border mt-3 w-full rounded-md border px-3 py-1.5 text-xs disabled:opacity-50"
					disabled={regenerating}
					onclick={regenerateBackupCodes}
				>
					{regenerating ? $t('twoFactorModal.regenerating') : $t('twoFactorModal.regenerateBackupCodes')}
				</button>
				{#if regenerateError}
					<p class="text-destructive mt-2 text-xs">{regenerateError}</p>
				{/if}

				{#if !showDisableForm}
					<button
						type="button"
						class="border-border text-destructive mt-2 w-full rounded-md border px-3 py-1.5 text-xs"
						onclick={() => (showDisableForm = true)}
					>
						{$t('twoFactorModal.disable')}
					</button>
				{:else}
					<form class="mt-2 space-y-2" onsubmit={submitDisable}>
						<label class="text-muted-foreground block text-xs" for="totp-disable-password"
							>{$t('twoFactorModal.passwordConfirmLabel')}</label
						>
						<input
							id="totp-disable-password"
							type="password"
							required
							autocomplete="current-password"
							bind:value={disablePassword}
							class="border-input bg-background w-full rounded-md border px-3 py-1.5 text-sm"
						/>
						{#if disableError}
							<p class="text-destructive text-xs">{disableError}</p>
						{/if}
						<div class="flex gap-2">
							<button
								type="button"
								class="border-border flex-1 rounded-md border px-3 py-1.5 text-xs"
								onclick={() => (showDisableForm = false)}
							>
								{$t('twoFactorModal.cancel')}
							</button>
							<button
								type="submit"
								disabled={disabling}
								class="bg-destructive text-destructive-foreground flex-1 rounded-md px-3 py-1.5 text-xs font-medium disabled:opacity-50"
							>
								{disabling ? $t('twoFactorModal.disabling') : $t('twoFactorModal.disableConfirm')}
							</button>
						</div>
					</form>
				{/if}
			{:else if totpBackupCodes}
				<p class="text-sm">
					{$t('twoFactorModal.setupCompleteNotice')}
				</p>
				<div class="border-border bg-background mt-2 grid grid-cols-2 gap-1 rounded-md border p-3">
					{#each totpBackupCodes as code (code)}
						<code class="text-xs">{code}</code>
					{/each}
				</div>
				<div class="mt-2 flex justify-end">
					<CopyButton text={totpBackupCodes.join('\n')} label={$t('twoFactorModal.copyAll')} />
				</div>
			{:else if startingTOTPSetup}
				<p class="text-muted-foreground text-sm">{$t('twoFactorModal.preparing')}</p>
			{:else}
				<p class="text-muted-foreground text-sm">
					{$t('twoFactorModal.scanQrNotice')}
				</p>
				{#if totpQrCode}
					<img src={totpQrCode} alt={$t('twoFactorModal.qrAlt')} class="mx-auto mt-3 h-48 w-48" />
				{/if}
				{#if totpSecret}
					<p class="text-muted-foreground mt-2 text-center text-xs">
						{$t('twoFactorModal.manualEntryNotice')}
						<code class="break-all">{totpSecret}</code>
					</p>
				{/if}
				<form class="mt-4 space-y-4" onsubmit={submitTOTPVerify}>
					<div>
						<label class="mb-1 block text-sm font-medium" for="totp-verify-code"
							>{$t('twoFactorModal.verifyCodeLabel')}</label
						>
						<input
							id="totp-verify-code"
							type="text"
							inputmode="numeric"
							required
							bind:value={totpVerifyCode}
							class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
						/>
					</div>
					{#if totpError}
						<p class="text-destructive text-sm">{totpError}</p>
					{/if}
					<button
						type="submit"
						disabled={verifyingTOTP}
						class="bg-primary text-primary-foreground w-full rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
					>
						{verifyingTOTP ? $t('twoFactorModal.verifying') : $t('twoFactorModal.verifyAndActivate')}
					</button>
				</form>
			{/if}
		</div>
	</div>
{/if}

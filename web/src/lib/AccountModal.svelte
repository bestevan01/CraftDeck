<script lang="ts">
	import { api } from '$lib/api';

	// 비밀번호 변경 + 2단계 인증은 둘 다 "계정" 설정이라 하나의 모달 안에
	// 두 섹션으로 묶었다 (헤더에 버튼이 따로 있으면 오히려 헷갈림).
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

	// FR-38/39: 2FA setup. totpEnabled gates the "외부 접속" toggle
	// backend-side -- this is just the UI to actually get there.
	let totpQrCode = $state('');
	let totpSecret = $state('');
	let totpVerifyCode = $state('');
	let totpError = $state('');
	let startingTOTPSetup = $state(false);
	let verifyingTOTP = $state(false);
	let totpBackupCodes = $state<string[] | null>(null);

	// Resets both sections' transient state and (if not already enabled)
	// kicks off a fresh TOTP setup, each time the modal opens.
	$effect(() => {
		if (!open) return;
		currentPassword = '';
		newPassword = '';
		newPasswordConfirm = '';
		passwordError = '';
		passwordChanged = false;
		totpError = '';
		totpVerifyCode = '';
		totpBackupCodes = null;
		if (totpEnabled) return; // already set up -- see the "already enabled" branch below
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

	async function changePassword(e: SubmitEvent) {
		e.preventDefault();
		if (newPassword !== newPasswordConfirm) {
			passwordError = '새 비밀번호가 서로 일치하지 않습니다.';
			return;
		}
		passwordError = '';
		passwordChanged = false;
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

			<!-- FR-38/39: 2단계 인증 등록 -- QR 스캔 후 코드 한 번 검증해야 실제로
				켜진다(submitTOTPVerify). 이미 켜져 있으면 재설정 대신 안내만 표시
				(setupTOTP이 409를 반환하므로 백엔드와 일관됨). -->
			<h3 class="text-sm font-medium">2단계 인증</h3>
			<div class="mt-3">
				{#if totpEnabled && !totpBackupCodes}
					<p class="text-muted-foreground text-sm">
						이미 설정되어 있습니다. 인증 앱을 분실했다면 로그인 시 백업 코드를 대신 사용하세요.
					</p>
				{:else if totpBackupCodes}
					<p class="text-sm">
						설정 완료됐습니다. 아래 백업 코드를 안전한 곳에 저장하세요 -- 다시 볼 수 없습니다.
					</p>
					<div class="border-border bg-background mt-2 grid grid-cols-2 gap-1 rounded-md border p-3">
						{#each totpBackupCodes as code (code)}
							<code class="text-xs">{code}</code>
						{/each}
					</div>
				{:else if startingTOTPSetup}
					<p class="text-muted-foreground text-sm">준비 중...</p>
				{:else}
					<p class="text-muted-foreground text-sm">
						인증 앱(Google Authenticator, Authy 등)으로 아래 QR 코드를 스캔하세요.
					</p>
					{#if totpQrCode}
						<img src={totpQrCode} alt="2FA QR 코드" class="mx-auto mt-3 h-48 w-48" />
					{/if}
					{#if totpSecret}
						<p class="text-muted-foreground mt-2 text-center text-xs">
							QR을 스캔할 수 없다면 직접 입력: <code class="break-all">{totpSecret}</code>
						</p>
					{/if}
					<form class="mt-4 space-y-4" onsubmit={submitTOTPVerify}>
						<div>
							<label class="mb-1 block text-sm font-medium" for="totp-verify-code"
								>인증 앱의 6자리 코드</label
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
							{verifyingTOTP ? '확인 중...' : '확인 후 활성화'}
						</button>
					</form>
				{/if}
			</div>
		</div>
	</div>
{/if}

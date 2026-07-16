<script lang="ts">
	// A shared confirmation modal so every destructive action (delete
	// instance/backup/file/plugin, disable swap, unregister from proxy...)
	// looks and behaves the same way, instead of some going through the
	// browser's native confirm() (unstyled, blocks the whole tab, can't
	// show a multi-line explanation nicely) and others through one-off
	// custom modals -- confirmed as an actual inconsistency worth fixing,
	// not just a style nit, since operators had to learn two different
	// "are you sure" patterns in the same app.
	let {
		open = $bindable(false),
		title = '확인',
		message,
		confirmLabel = '확인',
		cancelLabel = '취소',
		danger = true,
		onconfirm
	}: {
		open: boolean;
		title?: string;
		message: string;
		confirmLabel?: string;
		cancelLabel?: string;
		danger?: boolean;
		onconfirm: () => void;
	} = $props();

	function confirmed() {
		open = false;
		onconfirm();
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
			class="border-border bg-card w-full max-w-sm rounded-lg border p-4 shadow-lg"
			onclick={(e) => e.stopPropagation()}
		>
			<h2 class="font-medium {danger ? 'text-destructive' : ''}">{title}</h2>
			<p class="text-muted-foreground mt-2 text-sm whitespace-pre-line">{message}</p>
			<div class="mt-3 flex gap-2">
				<button
					type="button"
					class="border-border flex-1 rounded-md border px-4 py-2 text-sm font-medium"
					onclick={() => (open = false)}
				>
					{cancelLabel}
				</button>
				<button
					type="button"
					class="flex-1 rounded-md px-4 py-2 text-sm font-medium {danger
						? 'bg-destructive text-destructive-foreground'
						: 'bg-primary text-primary-foreground'}"
					onclick={confirmed}
				>
					{confirmLabel}
				</button>
			</div>
		</div>
	</div>
{/if}

<script lang="ts">
	import { fade } from 'svelte/transition';

	// 클립보드 복사 버튼 + 피드백 -- 버튼 안에 "복사됨" 텍스트를 넣는 대신
	// 버튼 위로 살짝 뜨는 짙은 배경의 말풍선으로 보여준다 (0.15s로 나타나고
	// 사라지고, 2.5초간 유지).
	let {
		text,
		label = '복사',
		class: className = 'border-border shrink-0 rounded-md border px-2 py-1 text-xs'
	}: {
		text: string;
		label?: string;
		class?: string;
	} = $props();

	let showCopied = $state(false);
	let hideTimeout: ReturnType<typeof setTimeout>;

	function flashCopied() {
		showCopied = true;
		clearTimeout(hideTimeout);
		hideTimeout = setTimeout(() => (showCopied = false), 2500);
	}

	// navigator.clipboard only exists in a secure context (HTTPS or
	// localhost) -- CraftDeck serves plain HTTP over LAN whenever WAN
	// exposure is off (FR-33), so on a typical LAN-only setup that API is
	// simply undefined and silently threw before this fallback existed
	// (confirmed: that's exactly why backup-code copying looked like it
	// did nothing). The old execCommand path still works over plain HTTP.
	function legacyCopy(value: string) {
		const textarea = document.createElement('textarea');
		textarea.value = value;
		textarea.style.position = 'fixed';
		textarea.style.opacity = '0';
		document.body.appendChild(textarea);
		textarea.focus();
		textarea.select();
		try {
			document.execCommand('copy');
		} finally {
			document.body.removeChild(textarea);
		}
	}

	function copy() {
		if (navigator.clipboard && window.isSecureContext) {
			navigator.clipboard.writeText(text).then(flashCopied, () => {
				legacyCopy(text);
				flashCopied();
			});
		} else {
			legacyCopy(text);
			flashCopied();
		}
	}
</script>

<span class="relative inline-block">
	{#if showCopied}
		<span
			transition:fade={{ duration: 150 }}
			class="bg-foreground text-background pointer-events-none absolute bottom-full left-1/2 mb-1.5 -translate-x-1/2 rounded-full px-3 py-1 text-xs whitespace-nowrap shadow-lg"
		>
			복사됨!
		</span>
	{/if}
	<button type="button" class={className} onclick={copy}>
		{label}
	</button>
</span>

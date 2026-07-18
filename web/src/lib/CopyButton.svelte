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

	function copy() {
		navigator.clipboard.writeText(text).then(() => {
			showCopied = true;
			clearTimeout(hideTimeout);
			hideTimeout = setTimeout(() => (showCopied = false), 2500);
		});
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

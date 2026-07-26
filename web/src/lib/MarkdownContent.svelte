<script lang="ts">
	// marked/dompurify are dynamically imported (not top-level) so their
	// ~20-30KB gzipped stays out of the app's initial bundle and is only
	// ever fetched the first time something here actually renders Markdown.
	// The text itself is untrusted external content (a Modrinth project's
	// description or a version's changelog) -- sanitize the rendered HTML
	// before it ever reaches {@html}, since an author could otherwise embed
	// a <script> or an event-handler attribute.
	let { markdown, class: className = '' }: { markdown: string; class?: string } = $props();

	let renderedHtml = $state('');

	$effect(() => {
		const text = markdown;
		if (!text) {
			renderedHtml = '';
			return;
		}
		let cancelled = false;
		(async () => {
			const [{ marked }, { default: DOMPurify }] = await Promise.all([
				import('marked'),
				import('dompurify')
			]);
			const html = await marked.parse(text);
			if (!cancelled) renderedHtml = DOMPurify.sanitize(html);
		})();
		return () => {
			cancelled = true;
		};
	});
</script>

{#if renderedHtml}
	<div class="markdown-body {className}">
		{@html renderedHtml}
	</div>
{/if}

<style>
	/* Tailwind's preflight strips all default element styling, so a raw
	   {@html} dump of marked's output would otherwise render as unstyled
	   run-on text -- just enough rules to make headings/lists/links/code
	   readable at this app's usual font sizes. */
	.markdown-body :global(h1),
	.markdown-body :global(h2),
	.markdown-body :global(h3),
	.markdown-body :global(h4) {
		font-weight: 500;
		margin-top: 0.75em;
		margin-bottom: 0.25em;
	}
	.markdown-body :global(p) {
		margin-bottom: 0.5em;
	}
	.markdown-body :global(ul),
	.markdown-body :global(ol) {
		margin: 0.25em 0 0.5em 1.25em;
		list-style: revert;
	}
	.markdown-body :global(a) {
		color: var(--color-primary);
		text-decoration: underline;
	}
	.markdown-body :global(code) {
		background: var(--color-muted);
		border-radius: 3px;
		padding: 0.1em 0.3em;
		font-size: 0.9em;
	}
	.markdown-body :global(img) {
		max-width: 100%;
	}
</style>

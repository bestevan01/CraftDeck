<script module lang="ts">
	export type TourStep = {
		selector: string;
		title: string;
		body: string;
		placement?: 'below' | 'above' | 'left' | 'right';
		beforeShow?: () => void;
	};
</script>

<script lang="ts">
	// Deliberately hand-rolled instead of a tour library (driver.js etc.) --
	// this only needs to run once on first login and occasionally on manual
	// replay, so the maintenance cost of a few dozen lines here is lower
	// than a new dependency (matches the project's minimal-dependency
	// philosophy, NFR-9). The "spotlight" is a separate `position: fixed`
	// div sized/positioned to match the target's getBoundingClientRect(),
	// not a box-shadow applied directly to the target -- an earlier version
	// did that, but the 전역 설정 tab content sits inside a
	// `lg:overflow-y-auto` scroll container, and setting only `overflow-y`
	// makes the CSS overflow spec treat `overflow-x` as `auto` too (an
	// element can't scroll on one axis while overflow paints freely past
	// its bounds on the other) -- so the ring got clipped at the
	// container's right/bottom edges instead of wrapping the whole card
	// (confirmed on real hardware). A fixed-position sibling outside that
	// container has no such ancestor clipping it.
	import { t } from '$lib/i18n';

	let {
		steps,
		open = $bindable(false),
		onFinish
	}: {
		steps: TourStep[];
		open: boolean;
		onFinish?: () => void;
	} = $props();

	let index = $state(0);
	let targetEl: HTMLElement | null = null;
	let spotlight = $state<{ top: number; left: number; width: number; height: number; radius: string } | null>(
		null
	);
	let tooltipTop = $state(0);
	let tooltipLeft = $state(0);
	let ready = $state(false);
	// Bumped on every place() call so a stale requestAnimationFrame callback
	// from an earlier, already-superseded step can tell it's obsolete and
	// bail out. Without this, when a step's target is missing (e.g. no
	// instance yet) and advance() chains straight through 2-3 steps in one
	// synchronous burst, each step's rAF callback still fires on its own
	// later frame -- an older callback can land its highlight *after* a
	// newer step has already highlighted its own target, leaving two
	// elements stuck lit at once (confirmed on real hardware).
	let generation = 0;

	function clearHighlight() {
		targetEl = null;
		spotlight = null;
	}

	// Recomputes both the spotlight box and the tooltip position from the
	// target's *current* rect -- shared by the initial placement and by the
	// scroll/resize listeners below, since the target can move after either
	// (e.g. scrolling the settings tab's own scroll container, which
	// doesn't fire a plain `window` scroll event without `capture: true`).
	function reposition(placement: TourStep['placement']) {
		if (!targetEl) return;
		const r = targetEl.getBoundingClientRect();
		const cs = getComputedStyle(targetEl);
		spotlight = { top: r.top, left: r.left, width: r.width, height: r.height, radius: cs.borderRadius };

		const maxLeft = window.innerWidth - 300;
		const maxTop = window.innerHeight - 160;
		if (placement === 'above') {
			tooltipTop = Math.max(8, Math.min(r.top - 140, maxTop));
			tooltipLeft = Math.max(8, Math.min(r.left, maxLeft));
		} else if (placement === 'left') {
			tooltipTop = Math.max(8, Math.min(r.top, maxTop));
			tooltipLeft = Math.max(8, r.left - 292);
		} else if (placement === 'right') {
			tooltipTop = Math.max(8, Math.min(r.top, maxTop));
			tooltipLeft = Math.min(maxLeft, r.right + 12);
		} else {
			tooltipTop = Math.min(r.bottom + 12, maxTop);
			tooltipLeft = Math.max(8, Math.min(r.left, maxLeft));
		}
	}

	function place() {
		clearHighlight();
		ready = false;
		const step = steps[index];
		if (!step) return;
		const myGeneration = ++generation;
		step.beforeShow?.();
		// beforeShow may switch tabs etc. -- give Svelte a tick to re-render
		// before querying the DOM for the new target.
		requestAnimationFrame(() => {
			if (myGeneration !== generation) return; // superseded by a later place()
			const el = document.querySelector<HTMLElement>(step.selector);
			if (!el) {
				// Target isn't on screen right now (e.g. no server instances
				// yet, so there's no console link to point at) -- don't
				// dead-end the tour, just skip past this step.
				advance();
				return;
			}
			targetEl = el;
			// 'smooth' scrolling is asynchronous -- reading getBoundingClientRect()
			// right after calling it would still see the *pre-scroll* position.
			// 'auto' jumps immediately so the rect read right after is accurate.
			el.scrollIntoView({ block: 'center', behavior: 'auto' });
			reposition(step.placement ?? 'below');
			ready = true;
		});
	}

	function advance() {
		if (index >= steps.length - 1) {
			finish();
			return;
		}
		index += 1;
		place();
	}

	function finish() {
		generation++; // invalidate any rAF callback still in flight
		clearHighlight();
		ready = false;
		index = 0;
		open = false;
		onFinish?.();
	}

	function handleReflow() {
		if (ready) reposition(steps[index]?.placement ?? 'below');
	}

	$effect(() => {
		if (open) place();
		else clearHighlight();
	});

	// `<svelte:window onscroll>` only attaches a bubble-phase listener, but
	// 'scroll' events never bubble -- so it silently misses scrolling inside
	// the 전역 설정 tab's own `overflow-y-auto` container (a real bug,
	// confirmed on real hardware: after that inner container scrolled, the
	// spotlight/tooltip stayed put at their old viewport coordinates while
	// the actual card moved, so the stale ring ended up sitting over
	// whatever card happened to scroll into that same screen position
	// instead). Listening on `window` with `capture: true` catches scroll
	// events from any descendant scrollable element too, since capture
	// phase traverses window -> ... -> the actual scrolled element.
	$effect(() => {
		if (!open) return;
		window.addEventListener('scroll', handleReflow, true);
		window.addEventListener('resize', handleReflow);
		return () => {
			window.removeEventListener('scroll', handleReflow, true);
			window.removeEventListener('resize', handleReflow);
		};
	});
</script>

{#if open && ready && spotlight}
	<div
		class="pointer-events-none fixed z-[60]"
		style="top:{spotlight.top}px; left:{spotlight.left}px; width:{spotlight.width}px; height:{spotlight.height}px; border-radius:{spotlight.radius}; box-shadow: 0 0 0 2px color-mix(in oklch, var(--primary) 55%, transparent), 0 0 0 9999px rgba(0,0,0,0.65);"
	></div>
{/if}

{#if open && ready && steps[index]}
	<div
		class="border-border bg-card fixed z-[70] w-72 rounded-lg border p-3 shadow-lg"
		style="top:{tooltipTop}px; left:{tooltipLeft}px;"
	>
		<div class="text-sm font-medium">{steps[index].title}</div>
		<div class="text-muted-foreground mt-1 text-xs leading-relaxed">{steps[index].body}</div>
		<div class="mt-3 flex items-center justify-between">
			<div class="flex gap-1">
				{#each steps as _, i (i)}
					<span
						class="h-1.5 w-1.5 rounded-full {i === index ? 'bg-primary' : 'bg-muted-foreground/30'}"
					></span>
				{/each}
			</div>
			<div class="flex gap-2">
				<button type="button" class="text-muted-foreground text-xs" onclick={finish}
					>{$t('tourOverlay.skip')}</button
				>
				<button
					type="button"
					class="bg-primary text-primary-foreground rounded-md px-2.5 py-1 text-xs font-medium"
					onclick={advance}
				>
					{index === steps.length - 1 ? $t('tourOverlay.finish') : $t('tourOverlay.next')}
				</button>
			</div>
		</div>
	</div>
{/if}

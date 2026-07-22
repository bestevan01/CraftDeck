<script lang="ts">
	import { locale, setLocale, t, type Locale } from '$lib/i18n';

	let { onStartTour, currentVersion }: { onStartTour: () => void; currentVersion: string } =
		$props();
</script>

<div class="space-y-4">
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
			<h2 class="font-medium">{$t('miscSettings.tour.title')}</h2>
			<p class="text-muted-foreground mt-1 text-xs">{$t('miscSettings.tour.description')}</p>
		</div>
		<button
			type="button"
			class="border-border shrink-0 rounded-md border px-3 py-1.5 text-xs"
			onclick={onStartTour}
		>
			{$t('miscSettings.tour.replayButton')}
		</button>
	</div>

	<div class="border-border bg-card rounded-lg border p-4">
		<h2 class="font-medium">{$t('miscSettings.about.title')}</h2>
		<dl class="mt-3 space-y-1.5 text-sm">
			<div class="flex justify-between">
				<dt class="text-muted-foreground">{$t('miscSettings.about.programName')}</dt>
				<dd>CraftDeck</dd>
			</div>
			<div class="flex justify-between">
				<dt class="text-muted-foreground">{$t('miscSettings.about.version')}</dt>
				<dd class="font-mono">{currentVersion || '-'}</dd>
			</div>
			<div class="flex justify-between">
				<dt class="text-muted-foreground">{$t('miscSettings.about.license')}</dt>
				<dd>
					<a
						href="https://github.com/bestevan01/CraftDeck/blob/main/LICENSE"
						target="_blank"
						rel="noreferrer"
						class="underline underline-offset-2"
					>
						MIT
					</a>
				</dd>
			</div>
		</dl>
	</div>
</div>

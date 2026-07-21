<script lang="ts">
	import type { BuildInfo } from '$lib/api';
	import MemorySlider from '$lib/MemorySlider.svelte';
	import { t } from '$lib/i18n';

	type Loader =
		| 'vanilla'
		| 'paper'
		| 'purpur'
		| 'folia'
		| 'pufferfish'
		| 'leaf'
		| 'fabric'
		| 'neoforge'
		| 'custom';

	let {
		open = $bindable(false),
		form = $bindable(),
		customLoaderName = $bindable(''),
		worldFile = $bindable(null),
		worldFileForce = $bindable(false),
		proxyCapableLoaders,
		buildListerLoaders,
		availableVersionIds,
		mcVersionsError,
		buildOptions,
		buildsError,
		maxMemoryGB,
		ramBoundaryGB,
		createError,
		creating,
		onLoaderChange,
		onCustomJarFileChange,
		onWorldFileChange,
		onSubmit
	}: {
		open: boolean;
		form: {
			name: string;
			loader: Loader;
			mc_version: string;
			loader_version: string;
			memory_gb: number;
			cpu_quota_percent: number;
			accept_eula: boolean;
			expose_independently: boolean;
		};
		customLoaderName: string;
		worldFile: File | null;
		worldFileForce: boolean;
		proxyCapableLoaders: string[];
		buildListerLoaders: string[];
		availableVersionIds: string[];
		mcVersionsError: string;
		buildOptions: BuildInfo[];
		buildsError: string;
		maxMemoryGB: number;
		ramBoundaryGB: number;
		createError: string;
		creating: boolean;
		onLoaderChange: () => void;
		onCustomJarFileChange: (e: Event) => void;
		onWorldFileChange: (e: Event) => void;
		onSubmit: () => void;
	} = $props();

	let pressedBackdrop = false;
</script>

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
		onmousedown={(e) => {
			// Only close when the *press* also started on the backdrop
			// itself -- otherwise selecting text (or dragging a slider) that
			// starts inside the dialog and happens to release outside it
			// closes the whole thing, which isn't what "click outside"
			// should mean (confirmed: that's exactly what was happening).
			pressedBackdrop = e.target === e.currentTarget;
		}}
		onclick={(e) => {
			if (pressedBackdrop && e.target === e.currentTarget) open = false;
		}}
		onkeydown={(e) => {
			if (e.key === 'Escape') open = false;
		}}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="border-border bg-card max-h-[90vh] w-full max-w-md overflow-y-auto rounded-lg border p-4 shadow-lg"
		>
			<div class="mb-3 flex items-center justify-between">
				<h2 class="font-medium">{$t('createInstanceModal.title')}</h2>
				<button type="button" class="text-muted-foreground text-sm" onclick={() => (open = false)}
					>&times;</button
				>
			</div>
			<form
				class="space-y-4"
				onsubmit={(e) => {
					e.preventDefault();
					onSubmit();
				}}
			>
				<div>
					<label class="mb-1 block text-sm font-medium" for="name">{$t('createInstanceModal.nameLabel')}</label>
					<input
						id="name"
						required
						bind:value={form.name}
						class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
						placeholder="survival"
					/>
				</div>
				<div>
					<label class="mb-1 block text-sm font-medium" for="loader">{$t('createInstanceModal.loaderLabel')}</label>
					<div class="relative">
						<select
							id="loader"
							bind:value={form.loader}
							onchange={onLoaderChange}
							class="border-input bg-background w-full appearance-none rounded-md border py-2 pl-3 pr-8 text-sm"
						>
							<option value="vanilla">Vanilla</option>
							<option value="paper">Paper</option>
							<option value="purpur">Purpur</option>
							<option value="folia">Folia</option>
							<option value="pufferfish">Pufferfish</option>
							<option value="leaf">Leaf</option>
							<option value="fabric">Fabric</option>
							<option value="neoforge">NeoForge</option>
							<option value="custom">{$t('createInstanceModal.loaderCustomOption')}</option>
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
				{#if form.loader === 'custom'}
					<div>
						<label class="mb-1 block text-sm font-medium" for="custom-loader-name">{$t('createInstanceModal.customLoaderNameLabel')}</label>
						<input
							id="custom-loader-name"
							type="text"
							required
							bind:value={customLoaderName}
							placeholder={$t('createInstanceModal.customLoaderNamePlaceholder')}
							class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
						/>
						<p class="text-muted-foreground mt-1 text-xs">
							{$t('createInstanceModal.customLoaderDescription')}
						</p>
					</div>
				{/if}
				{#if proxyCapableLoaders.includes(form.loader)}
					<label class="flex items-start gap-2 text-sm">
						<input type="checkbox" bind:checked={form.expose_independently} class="mt-1" />
						<span>
							{$t('createInstanceModal.exposeIndependently')}
						</span>
					</label>
					{#if form.loader === 'fabric' || form.loader === 'neoforge'}
						<p class="text-muted-foreground -mt-2 text-xs">
							{$t('createInstanceModal.modIncompatibilityWarning')}
						</p>
					{/if}
				{:else}
					<p class="text-muted-foreground text-xs">
						{$t('createInstanceModal.noProxyForwarding')}
					</p>
				{/if}
				<div>
					<label class="mb-1 block text-sm font-medium" for="mc_version">{$t('createInstanceModal.mcVersionLabel')}</label>
					{#if form.loader === 'custom'}
						<input
							id="mc_version"
							type="text"
							required
							bind:value={form.mc_version}
							placeholder={$t('createInstanceModal.mcVersionCustomPlaceholder')}
							class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
						/>
					{:else if mcVersionsError}
						<p class="text-destructive text-xs">
							{$t('createInstanceModal.mcVersionsFetchError', { error: mcVersionsError })}
						</p>
					{:else if availableVersionIds.length === 0}
						<p class="text-muted-foreground text-xs">{$t('createInstanceModal.mcVersionsLoading')}</p>
					{:else}
						<div class="relative">
							<select
								id="mc_version"
								required
								bind:value={form.mc_version}
								class="border-input bg-background w-full appearance-none rounded-md border py-2 pl-3 pr-8 text-sm"
							>
								{#each availableVersionIds as id}
									<option value={id}>{id}</option>
								{/each}
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
					{/if}
				</div>
				{#if buildListerLoaders.includes(form.loader) && buildOptions.length > 0}
					<div>
						<label class="mb-1 block text-sm font-medium" for="loader_version">{$t('createInstanceModal.buildLabel')}</label>
						<div class="relative">
							<select
								id="loader_version"
								bind:value={form.loader_version}
								class="border-input bg-background w-full appearance-none rounded-md border py-2 pl-3 pr-8 text-sm"
							>
								<option value="">{$t('createInstanceModal.buildLatest')}</option>
								{#each buildOptions as build (build.id)}
									<option value={build.id}>
										{build.id}{build.channel ? ` (${build.channel})` : ''}
									</option>
								{/each}
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
				{:else if buildListerLoaders.includes(form.loader) && buildsError}
					<p class="text-muted-foreground text-xs">{$t('createInstanceModal.buildsFetchError', { error: buildsError })}</p>
				{/if}
				{#if form.loader === 'custom'}
					<div>
						<label class="mb-1 block text-sm font-medium" for="custom-jar">{$t('createInstanceModal.customJarLabel')}</label>
						<input
							id="custom-jar"
							type="file"
							required
							accept=".jar"
							onchange={onCustomJarFileChange}
							class="text-muted-foreground file:border-border file:bg-background file:text-foreground file:mr-2 file:rounded-md file:border file:px-3 file:py-1.5 file:text-xs file:font-medium file:cursor-pointer w-full text-xs"
						/>
					</div>
				{/if}
				<div>
					<label class="mb-1 block text-sm font-medium" for="create-memory">
						{$t('createInstanceModal.memoryLabel', {
							memory: form.memory_gb,
							maxMemory: maxMemoryGB
						})}{#if ramBoundaryGB < maxMemoryGB}<span class="text-yellow-500"
								>{$t('createInstanceModal.swapIncluded', { swap: maxMemoryGB - ramBoundaryGB })}</span
							>{/if})
					</label>
					<MemorySlider id="create-memory" bind:value={form.memory_gb} maxGB={maxMemoryGB} {ramBoundaryGB} />
				</div>
				<div>
					<label class="mb-1 block text-sm font-medium" for="create-cpu">
						{$t('createInstanceModal.cpuLabel', {
							cpu:
								form.cpu_quota_percent > 0
									? `${form.cpu_quota_percent}%`
									: $t('createInstanceModal.cpuUnlimited')
						})}
					</label>
					<input
						id="create-cpu"
						type="range"
						min="0"
						max="100"
						step="5"
						bind:value={form.cpu_quota_percent}
						class="w-full"
					/>
				</div>
				<div>
					<label class="mb-1 block text-sm font-medium" for="world-file"
						>{$t('createInstanceModal.worldFileLabel')}</label
					>
					<input
						id="world-file"
						type="file"
						accept=".gz,.tar.gz"
						onchange={onWorldFileChange}
						class="text-muted-foreground file:border-border file:bg-background file:text-foreground file:mr-2 file:rounded-md file:border file:px-3 file:py-1.5 file:text-xs file:font-medium file:cursor-pointer w-full text-sm"
					/>
					{#if worldFile}
						<label class="mt-1 flex items-center gap-2 text-xs">
							<input type="checkbox" bind:checked={worldFileForce} />
							<span>{$t('createInstanceModal.worldFileForce')}</span>
						</label>
					{/if}
				</div>
				<label class="flex items-start gap-2 text-sm">
					<input type="checkbox" required bind:checked={form.accept_eula} class="mt-1" />
					<span>
						{$t('createInstanceModal.eulaAgree')} <a
							class="underline"
							href="https://www.minecraft.net/eula"
							target="_blank"
							rel="noreferrer">EULA</a
						>{$t('createInstanceModal.eulaAgreeSuffix')}
					</span>
				</label>
				{#if createError}
					<p class="text-destructive text-sm">{createError}</p>
				{/if}
				<button
					type="submit"
					disabled={creating}
					class="bg-primary text-primary-foreground w-full rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
				>
					{creating ? $t('createInstanceModal.creating') : $t('createInstanceModal.create')}
				</button>
			</form>
		</div>
	</div>
{/if}

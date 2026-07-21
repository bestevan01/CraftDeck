<script lang="ts">
	import type { Instance, NetworkSettings, PortMapping } from '$lib/api';
	import { t } from '$lib/i18n';

	// FR-21/22/23/25: 외부 접속 허용 (웹 UI 포트 + 켜진 인스턴스의 게임 포트)
	let {
		networkSettings,
		networkToggling,
		networkError,
		onToggle,
		portMappings,
		deletingMappingId,
		onDeleteMapping,
		instances
	}: {
		networkSettings: NetworkSettings | null;
		networkToggling: boolean;
		networkError: string;
		onToggle: (enabled: boolean) => void;
		portMappings: PortMapping[];
		deletingMappingId: string;
		onDeleteMapping: (id: string) => void;
		instances: Instance[];
	} = $props();

	function mappingMethodLabel(method: PortMapping['method']) {
		return { upnp: 'UPnP', natpmp: 'NAT-PMP', manual: $t('externalAccessCard.methodManual') }[method] ?? method;
	}

	function mappingOwnerLabel(mapping: PortMapping) {
		if (!mapping.instance_id) return $t('externalAccessCard.webUiOwner');
		return instances.find((i) => i.id === mapping.instance_id)?.name ?? mapping.instance_id;
	}
</script>

<!-- FR-21/22/23/25: 외부 접속 허용 (웹 UI 포트 + 켜진 인스턴스의 게임 포트) -->
<div id="tour-external-access" class="border-border bg-card rounded-lg border p-4">
	<div class="flex items-center justify-between">
		<h2 class="font-medium">{$t('externalAccessCard.title')}</h2>
		<label class="inline-flex cursor-pointer items-center gap-2 text-sm">
			<input
				type="checkbox"
				checked={networkSettings?.wan_enabled ?? false}
				disabled={networkToggling || !networkSettings}
				onchange={(e) => onToggle((e.target as HTMLInputElement).checked)}
			/>
			{networkToggling
				? $t('externalAccessCard.applying')
				: networkSettings?.wan_enabled
					? $t('externalAccessCard.on')
					: $t('externalAccessCard.off')}
		</label>
	</div>
	<p class="text-muted-foreground mt-1 text-xs">
		{$t('externalAccessCard.description')}
	</p>
	{#if networkError}
		<p class="text-destructive mt-2 text-xs">{networkError}</p>
	{/if}
	{#if networkSettings?.wan_enabled && networkSettings.web_mapping}
		<p class="mt-2 text-xs text-green-500">
			{$t('externalAccessCard.webUiRegistered', {
				method: mappingMethodLabel(networkSettings.web_mapping.method),
				port: networkSettings.web_mapping.external_port
			})}
		</p>
	{:else if networkSettings?.wan_enabled && networkSettings.manual_info}
		<div class="border-border bg-background mt-2 rounded-md border p-3 text-xs">
			<p class="mb-1 font-medium">{$t('externalAccessCard.manualSetupTitle')}</p>
			<p>{$t('externalAccessCard.localIp', { ip: networkSettings.manual_info.local_ip })}</p>
			<p>{$t('externalAccessCard.port', { port: networkSettings.manual_info.internal_port })}</p>
			<p>
				{$t('externalAccessCard.protocol', {
					protocol: networkSettings.manual_info.protocol.toUpperCase()
				})}
			</p>
		</div>
	{/if}

	{#if portMappings.length > 0}
		<div class="mt-3">
			<p class="text-muted-foreground mb-1 text-xs font-medium">
				{$t('externalAccessCard.registeredRulesTitle')}
			</p>
			<div class="space-y-1.5">
				{#each portMappings as mapping (mapping.id)}
					<div
						class="border-border flex items-center justify-between rounded-md border px-2 py-1.5 text-xs"
					>
						<span>
							{mappingOwnerLabel(mapping)} · {mapping.external_port} → {mapping.internal_port}/{mapping.protocol.toUpperCase()}
							· {mappingMethodLabel(mapping.method)}
						</span>
						<button
							class="border-border rounded-md border px-2 py-1 text-xs disabled:opacity-50"
							disabled={deletingMappingId === mapping.id}
							onclick={() => onDeleteMapping(mapping.id)}
						>
							{deletingMappingId === mapping.id
								? $t('externalAccessCard.deleting')
								: $t('externalAccessCard.delete')}
						</button>
					</div>
				{/each}
			</div>
		</div>
	{/if}
</div>

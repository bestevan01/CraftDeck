<script lang="ts">
	import type { Instance, NetworkSettings, PortMapping } from '$lib/api';

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
		return { upnp: 'UPnP', natpmp: 'NAT-PMP', manual: '수동' }[method] ?? method;
	}

	function mappingOwnerLabel(mapping: PortMapping) {
		if (!mapping.instance_id) return '웹 UI';
		return instances.find((i) => i.id === mapping.instance_id)?.name ?? mapping.instance_id;
	}
</script>

<!-- FR-21/22/23/25: 외부 접속 허용 (웹 UI 포트 + 켜진 인스턴스의 게임 포트) -->
<div class="border-border bg-card rounded-lg border p-4">
	<div class="flex items-center justify-between">
		<h2 class="font-medium">외부 접속</h2>
		<label class="inline-flex cursor-pointer items-center gap-2 text-sm">
			<input
				type="checkbox"
				checked={networkSettings?.wan_enabled ?? false}
				disabled={networkToggling || !networkSettings}
				onchange={(e) => onToggle((e.target as HTMLInputElement).checked)}
			/>
			{networkToggling ? '적용 중...' : networkSettings?.wan_enabled ? '켜짐' : '꺼짐'}
		</label>
	</div>
	<p class="text-muted-foreground mt-1 text-xs">
		켜면 관리 웹 UI 포트와, 실행 중인 인스턴스 중 실제로 접속 가능한 것(Velocity 프록시 또는 독립
		노출된 서버)의 게임 포트를 UPnP(IGD)나 NAT-PMP로 공유기에 자동 등록합니다. 인스턴스를 시작/종료하면
		그 인스턴스의 포트도 자동으로 열리고 닫힙니다. 둘 다 지원하지 않거나 실패하면 직접 설정할 정보를
		안내합니다. 켜져 있는 동안은 같은 네트워크(LAN) 안에서도 로그인이 필요합니다.
	</p>
	{#if networkError}
		<p class="text-destructive mt-2 text-xs">{networkError}</p>
	{/if}
	{#if networkSettings?.wan_enabled && networkSettings.web_mapping}
		<p class="mt-2 text-xs text-green-500">
			웹 UI: {mappingMethodLabel(networkSettings.web_mapping.method)} 자동 등록됨 (외부 포트 {networkSettings
				.web_mapping.external_port})
		</p>
	{:else if networkSettings?.wan_enabled && networkSettings.manual_info}
		<div class="border-border bg-background mt-2 rounded-md border p-3 text-xs">
			<p class="mb-1 font-medium">자동 등록에 실패했습니다 -- 공유기에서 직접 설정하세요:</p>
			<p>내부 IP: {networkSettings.manual_info.local_ip}</p>
			<p>포트: {networkSettings.manual_info.internal_port}</p>
			<p>프로토콜: {networkSettings.manual_info.protocol.toUpperCase()}</p>
		</div>
	{/if}

	{#if portMappings.length > 0}
		<div class="mt-3">
			<p class="text-muted-foreground mb-1 text-xs font-medium">등록된 포트포워딩 규칙</p>
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
							{deletingMappingId === mapping.id ? '삭제 중...' : '삭제'}
						</button>
					</div>
				{/each}
			</div>
		</div>
	{/if}
</div>

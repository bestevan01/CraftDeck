export const messages = {
	ko: {
		'domainConnectionCard.title': '도메인 연결',
		'domainConnectionCard.description':
			'소유한 메인 도메인을 연결하면 Velocity 프록시가 자동으로 켜져서 여러 서버를 서브도메인으로 묶어 접속할 수 있게 됩니다. 도메인이 없거나 무료 DDNS 서브도메인만 쓰는 경우 서브도메인 라우팅 자체가 실제로 닿지 않으므로, Velocity는 꺼지고 각 서버가 포트로 직접 노출됩니다.',
		'domainConnectionCard.cloudflareGuideButton': 'Cloudflare 연동 가이드로 설정하기',
		'domainConnectionCard.kindMainDomain': '메인 도메인',
		'domainConnectionCard.kindFreeSubdomain': '무료 DDNS',
		'domainConnectionCard.connectedInfo': '연결됨: {hostname} ({provider})',
		'domainConnectionCard.monitorModeNotice':
			'이 제공자는 자동 갱신을 지원하지 않으며 공유기 자체 DDNS 기능에 의존합니다. CraftDeck은 주기적으로 이 호스트명이 실제 공인 IP를 가리키는지만 확인합니다.',
		'domainConnectionCard.mismatchWarning':
			'⚠ 이 호스트명이 현재 공인 IP와 일치하지 않습니다. 공유기의 ipTime DDNS 기능이 꺼졌거나 실패했을 수 있습니다.',
		'domainConnectionCard.activeRenewalNotice': 'CraftDeck이 20분마다 자동으로 공인 IP를 갱신합니다.',
		'domainConnectionCard.lastChecked': '마지막 확인: {time}{ip}',
		'domainConnectionCard.certRenewalError':
			'⚠ HTTPS 인증서 발급/갱신에 실패했습니다 ({time}): {error}',
		'domainConnectionCard.certRenewalNotice':
			'다음 접속 시도에서 자동으로 재시도하며, 그때까지는 자체 서명 인증서로 대체됩니다. Cloudflare 토큰이 만료/취소되지 않았는지 확인하세요.',
		'domainConnectionCard.unregistering': '해제 중...',
		'domainConnectionCard.unregister': '연결 해제',
		'domainConnectionCard.kindLabel': '연결 방식',
		'domainConnectionCard.kindOptionMainDomain': '소유한 메인 도메인',
		'domainConnectionCard.kindOptionFreeSubdomain': '무료 DDNS 서브도메인',
		'domainConnectionCard.providerLabel': '제공자',
		'domainConnectionCard.hostnameLabel': '호스트명',
		'domainConnectionCard.hostnamePlaceholderIptime': '예: myrouter.iptime.org',
		'domainConnectionCard.hostnamePlaceholderDuckdns': '예: myserver.duckdns.org',
		'domainConnectionCard.duckdnsTokenLabel': 'DuckDNS 토큰',
		'domainConnectionCard.iptimeNotice':
			'공유기 관리 페이지에서 이미 설정해둔 ipTime DDNS 호스트명을 그대로 입력하세요. 이 제공자는 CraftDeck이 직접 갱신할 수 없어 공유기 자체 기능에 의존하며, CraftDeck은 감시만 합니다.',
		'domainConnectionCard.domainLabel': '도메인',
		'domainConnectionCard.domainPlaceholder': '예: craftdeck.cc',
		'domainConnectionCard.cfTokenLabel': 'Cloudflare API 토큰',
		'domainConnectionCard.cfTokenPlaceholder': 'Edit zone DNS 권한, 이 도메인 존으로 범위 제한 권장',
		'domainConnectionCard.cfTokenNotice':
			'Cloudflare 대시보드 > My Profile > API Tokens에서 "Edit zone DNS" 템플릿으로 이 도메인 존 하나만 범위를 제한해 발급하세요. 이 토큰으로 해당 존에 실제 접근 가능한지 확인해 도메인 소유권 검증을 대신합니다.',
		'domainConnectionCard.registering': '등록 중...',
		'domainConnectionCard.register': '등록',
		'domainConnectionCard.iptimeProviderLabel': 'ipTime (자동 갱신 불가, 감시 전용)'
	},
	en: {
		'domainConnectionCard.title': 'Domain connection',
		'domainConnectionCard.description':
			'Connecting a domain you own automatically turns on the Velocity proxy, letting multiple servers be reached through subdomains. If you have no domain, or only use a free DDNS subdomain, subdomain routing can\'t actually reach it, so Velocity is turned off and each server is exposed directly by port.',
		'domainConnectionCard.cloudflareGuideButton': 'Set up with the Cloudflare guide',
		'domainConnectionCard.kindMainDomain': 'Main domain',
		'domainConnectionCard.kindFreeSubdomain': 'Free DDNS',
		'domainConnectionCard.connectedInfo': 'Connected: {hostname} ({provider})',
		'domainConnectionCard.monitorModeNotice':
			"This provider doesn't support automatic renewal and relies on your router's own DDNS feature. CraftDeck only periodically checks whether this hostname points to the actual public IP.",
		'domainConnectionCard.mismatchWarning':
			"⚠ This hostname doesn't match the current public IP. Your router's ipTime DDNS feature may be off or failing.",
		'domainConnectionCard.activeRenewalNotice': 'CraftDeck automatically renews the public IP every 20 minutes.',
		'domainConnectionCard.lastChecked': 'Last checked: {time}{ip}',
		'domainConnectionCard.certRenewalError': '⚠ Failed to issue/renew the HTTPS certificate ({time}): {error}',
		'domainConnectionCard.certRenewalNotice':
			"It will automatically retry on the next connection attempt, and a self-signed certificate will be used until then. Make sure your Cloudflare token hasn't expired or been revoked.",
		'domainConnectionCard.unregistering': 'Disconnecting...',
		'domainConnectionCard.unregister': 'Disconnect',
		'domainConnectionCard.kindLabel': 'Connection type',
		'domainConnectionCard.kindOptionMainDomain': 'Domain I own',
		'domainConnectionCard.kindOptionFreeSubdomain': 'Free DDNS subdomain',
		'domainConnectionCard.providerLabel': 'Provider',
		'domainConnectionCard.hostnameLabel': 'Hostname',
		'domainConnectionCard.hostnamePlaceholderIptime': 'e.g. myrouter.iptime.org',
		'domainConnectionCard.hostnamePlaceholderDuckdns': 'e.g. myserver.duckdns.org',
		'domainConnectionCard.duckdnsTokenLabel': 'DuckDNS token',
		'domainConnectionCard.iptimeNotice':
			"Enter the ipTime DDNS hostname you've already configured on your router's admin page. CraftDeck can't renew this provider directly, so it relies on the router's own feature and only monitors it.",
		'domainConnectionCard.domainLabel': 'Domain',
		'domainConnectionCard.domainPlaceholder': 'e.g. craftdeck.cc',
		'domainConnectionCard.cfTokenLabel': 'Cloudflare API token',
		'domainConnectionCard.cfTokenPlaceholder': 'Edit zone DNS permission, recommended to scope to this domain zone',
		'domainConnectionCard.cfTokenNotice':
			'In the Cloudflare dashboard, go to My Profile > API Tokens and issue one using the "Edit zone DNS" template, scoped to just this domain zone. This token is used to verify actual access to that zone, standing in for domain ownership verification.',
		'domainConnectionCard.registering': 'Registering...',
		'domainConnectionCard.register': 'Register',
		'domainConnectionCard.iptimeProviderLabel': 'ipTime (no auto-renewal, monitor only)'
	}
};

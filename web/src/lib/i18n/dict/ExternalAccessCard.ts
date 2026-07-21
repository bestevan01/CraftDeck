export const messages = {
	ko: {
		'externalAccessCard.methodManual': '수동',
		'externalAccessCard.webUiOwner': '웹 UI',
		'externalAccessCard.title': '외부 접속',
		'externalAccessCard.applying': '적용 중...',
		'externalAccessCard.on': '켜짐',
		'externalAccessCard.off': '꺼짐',
		'externalAccessCard.description':
			'켜면 관리 웹 UI 포트와, 실행 중인 인스턴스 중 실제로 접속 가능한 것(Velocity 프록시 또는 독립 노출된 서버)의 게임 포트를 UPnP(IGD)나 NAT-PMP로 공유기에 자동 등록합니다. 인스턴스를 시작/종료하면 그 인스턴스의 포트도 자동으로 열리고 닫힙니다. 둘 다 지원하지 않거나 실패하면 직접 설정할 정보를 안내합니다. 켜져 있는 동안은 같은 네트워크(LAN) 안에서도 로그인이 필요합니다.',
		'externalAccessCard.webUiRegistered': '웹 UI: {method} 자동 등록됨 (외부 포트 {port})',
		'externalAccessCard.manualSetupTitle': '자동 등록에 실패했습니다. 공유기에서 직접 설정하세요:',
		'externalAccessCard.localIp': '내부 IP: {ip}',
		'externalAccessCard.port': '포트: {port}',
		'externalAccessCard.protocol': '프로토콜: {protocol}',
		'externalAccessCard.registeredRulesTitle': '등록된 포트포워딩 규칙',
		'externalAccessCard.deleting': '삭제 중...',
		'externalAccessCard.delete': '삭제'
	},
	en: {
		'externalAccessCard.methodManual': 'Manual',
		'externalAccessCard.webUiOwner': 'Web UI',
		'externalAccessCard.title': 'External Access',
		'externalAccessCard.applying': 'Applying...',
		'externalAccessCard.on': 'On',
		'externalAccessCard.off': 'Off',
		'externalAccessCard.description':
			'When enabled, the management web UI port and the game ports of any running instance that is actually reachable (a Velocity proxy or a standalone exposed server) are automatically registered on your router via UPnP (IGD) or NAT-PMP. Starting or stopping an instance automatically opens or closes its port as well. If neither method is supported or registration fails, you will be shown the details needed to set it up manually. While this is on, logging in is required even from the same local network (LAN).',
		'externalAccessCard.webUiRegistered': 'Web UI: {method} auto-registered (external port {port})',
		'externalAccessCard.manualSetupTitle': 'Automatic registration failed. Set it up manually on your router:',
		'externalAccessCard.localIp': 'Local IP: {ip}',
		'externalAccessCard.port': 'Port: {port}',
		'externalAccessCard.protocol': 'Protocol: {protocol}',
		'externalAccessCard.registeredRulesTitle': 'Registered port forwarding rules',
		'externalAccessCard.deleting': 'Deleting...',
		'externalAccessCard.delete': 'Delete'
	}
};

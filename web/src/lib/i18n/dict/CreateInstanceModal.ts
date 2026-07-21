export const messages = {
	ko: {
		'createInstanceModal.title': '서버 만들기',
		'createInstanceModal.nameLabel': '이름',
		'createInstanceModal.loaderLabel': '구동기',
		'createInstanceModal.loaderCustomOption': '커스텀 (직접 업로드)',
		'createInstanceModal.customLoaderNameLabel': '구동기 이름',
		'createInstanceModal.customLoaderNamePlaceholder': '예: MyModpackServer',
		'createInstanceModal.customLoaderDescription':
			'목록에 없는 구동기의 jar 파일을 직접 올려 서버를 만듭니다. 자동 다운로드/버전 목록 조회/플러그인·모드 검색은 지원되지 않고, 파일 탭에서 직접 관리해야 합니다.',
		'createInstanceModal.exposeIndependently':
			'독립적으로 외부에 노출 (기본은 항상 켜져 있는 Velocity 프록시 뒤에 자동 등록되며, 게임 포트는 내부용으로만 쓰입니다)',
		'createInstanceModal.modIncompatibilityWarning':
			'⚠ 엔티티·블록 상태 등 바닐라 패킷 구조 자체를 변형하는 모드(예: Create)는 Velocity와 호환되지 않아 접속이 끊길 수 있습니다. 이런 모드를 쓸 계획이면 독립 노출을 체크하세요.',
		'createInstanceModal.noProxyForwarding': '이 구동기는 프록시의 모던 포워딩을 지원하지 않아 항상 독립적으로 노출됩니다.',
		'createInstanceModal.mcVersionLabel': '마인크래프트 버전',
		'createInstanceModal.mcVersionCustomPlaceholder': '예: 1.20.1 (Java 버전 자동 선택에 쓰입니다)',
		'createInstanceModal.mcVersionsFetchError': '버전 목록을 불러오지 못했습니다: {error}',
		'createInstanceModal.mcVersionsLoading': '버전 목록 불러오는 중...',
		'createInstanceModal.buildLabel': '빌드 (선택사항)',
		'createInstanceModal.buildLatest': '최신',
		'createInstanceModal.buildsFetchError': '빌드 목록을 불러오지 못했습니다: {error}',
		'createInstanceModal.customJarLabel': '구동기 jar 파일',
		'createInstanceModal.memoryLabel': '최대 메모리 ({memory}GB / 최대 {maxMemory}GB',
		'createInstanceModal.swapIncluded': ' · 스왑 {swap}GB 포함',
		'createInstanceModal.cpuLabel': 'CPU 할당량 ({cpu})',
		'createInstanceModal.cpuUnlimited': '무제한',
		'createInstanceModal.worldFileLabel': '월드 데이터 가져오기 (선택, tar.gz)',
		'createInstanceModal.worldFileForce': '업로드한 월드가 이 인스턴스보다 최신 버전이어도 강제로 적용',
		'createInstanceModal.eulaAgree': '마인크래프트',
		'createInstanceModal.eulaAgreeSuffix': '에 동의합니다.',
		'createInstanceModal.creating': '생성 중... (jar 다운로드 포함)',
		'createInstanceModal.create': '생성'
	},
	en: {
		'createInstanceModal.title': 'Create Server',
		'createInstanceModal.nameLabel': 'Name',
		'createInstanceModal.loaderLabel': 'Loader',
		'createInstanceModal.loaderCustomOption': 'Custom (upload manually)',
		'createInstanceModal.customLoaderNameLabel': 'Loader name',
		'createInstanceModal.customLoaderNamePlaceholder': 'e.g. MyModpackServer',
		'createInstanceModal.customLoaderDescription':
			'Upload a jar file for a loader not in the list to create a server. Automatic downloads, version listing, and plugin/mod search are not supported — manage it manually from the Files tab.',
		'createInstanceModal.exposeIndependently':
			'Expose independently (by default, servers are automatically registered behind the always-on Velocity proxy, and the game port is for internal use only)',
		'createInstanceModal.modIncompatibilityWarning':
			'⚠ Mods that alter vanilla packet structures such as entity or block state (e.g. Create) are incompatible with Velocity and may cause disconnects. If you plan to use such mods, enable independent exposure.',
		'createInstanceModal.noProxyForwarding': 'This loader does not support the proxy\'s modern forwarding, so it is always exposed independently.',
		'createInstanceModal.mcVersionLabel': 'Minecraft version',
		'createInstanceModal.mcVersionCustomPlaceholder': 'e.g. 1.20.1 (used for automatic Java version selection)',
		'createInstanceModal.mcVersionsFetchError': 'Failed to load version list: {error}',
		'createInstanceModal.mcVersionsLoading': 'Loading version list...',
		'createInstanceModal.buildLabel': 'Build (optional)',
		'createInstanceModal.buildLatest': 'Latest',
		'createInstanceModal.buildsFetchError': 'Failed to load build list: {error}',
		'createInstanceModal.customJarLabel': 'Loader jar file',
		'createInstanceModal.memoryLabel': 'Max memory ({memory}GB / max {maxMemory}GB',
		'createInstanceModal.swapIncluded': ' · includes {swap}GB swap',
		'createInstanceModal.cpuLabel': 'CPU allocation ({cpu})',
		'createInstanceModal.cpuUnlimited': 'Unlimited',
		'createInstanceModal.worldFileLabel': 'Import world data (optional, tar.gz)',
		'createInstanceModal.worldFileForce': 'Force apply even if the uploaded world is newer than this instance',
		'createInstanceModal.eulaAgree': 'I agree to the Minecraft',
		'createInstanceModal.eulaAgreeSuffix': '.',
		'createInstanceModal.creating': 'Creating... (includes jar download)',
		'createInstanceModal.create': 'Create'
	}
};

export const messages = {
	ko: {
		'mainPage.header.createServer': '+ 서버 만들기',
		'mainPage.header.accountSettings': '계정 설정',
		'mainPage.header.logout': '로그아웃',
		'mainPage.loadError': '서버 목록을 불러오지 못했습니다: {error}',
		'mainPage.tabs.instances': '인스턴스',
		'mainPage.tabs.settings': '설정',
		'mainPage.settings.network': '네트워크',
		'mainPage.settings.hardware': '하드웨어',
		'mainPage.settings.account': '계정',

		'mainPage.instances.exampleServerName': '예시 서버',
		'mainPage.instances.exampleStatus': '실행 중',
		'mainPage.instances.exampleBadge': '예시',
		'mainPage.instances.empty': '서버 인스턴스가 아직 없습니다.',
		'mainPage.instances.stop': '종료',
		'mainPage.instances.start': '시작',
		'mainPage.instances.console': '콘솔',
		'mainPage.instances.delete': '삭제',
		'mainPage.instances.confirmDelete': '이 인스턴스를 삭제할까요? 월드 데이터도 함께 지워집니다.',

		'mainPage.status.stopped': '중지됨',
		'mainPage.status.starting': '시작 중',
		'mainPage.status.running': '실행 중',
		'mainPage.status.stopping': '종료 중',
		'mainPage.status.crashed': '비정상 종료',

		'mainPage.network.wanRequiresTotp': '외부 접속을 켜려면 먼저 2단계 인증을 설정해야 합니다.',

		'mainPage.swap.invalidSize': '0보다 큰 크기를 GB 단위로 입력하세요.',
		'mainPage.swap.confirmDisable': '스왑파일을 완전히 끄고 삭제할까요?',

		'mainPage.overclock.waitTimeout': '서버 종료 대기 시간이 초과됐습니다. 인스턴스 상태를 확인한 뒤 다시 시도해주세요.',
		'mainPage.overclock.confirmRebootStop':
			'다음 서버가 실행 중입니다: {names}\n재부팅 전에 먼저 각 서버를 안전하게 종료합니다. 계속할까요?',
		'mainPage.overclock.rebootNoResponse': '재부팅 후 응답이 없습니다. 잠시 후 페이지를 직접 새로고침해보세요.',

		'mainPage.benchmark.confirmStop':
			'다음 서버가 실행 중입니다: {names}\n안정성 테스트 동안 먼저 종료하고, 끝나면 자동으로 다시 시작합니다. 계속할까요?',

		'mainPage.update.upToDate': '최신 버전입니다.',

		'mainPage.tour.createServer.title': '서버 만들기',
		'mainPage.tour.createServer.body': '여기서 새 마인크래프트 서버를 몇 번의 클릭으로 만들 수 있어요.',
		'mainPage.tour.console.title': '실시간 콘솔',
		'mainPage.tour.console.body': '서버 로그를 실시간으로 보고 명령어를 바로 입력할 수 있어요.',
		'mainPage.tour.settings.title': '설정',
		'mainPage.tour.settings.body':
			'외부 접속, 도메인 연결, 스왑처럼 서버 하나에 속하지 않는 설정은 여기 모여 있어요.',
		'mainPage.tour.externalAccess.title': '외부 접속',
		'mainPage.tour.externalAccess.body': '친구를 초대해서 같이 플레이하려면 여기서 외부 접속을 켜세요.',
		'mainPage.tour.domain.title': '도메인 연결',
		'mainPage.tour.domain.body':
			'소유한 도메인이 있다면 연결해서 서브도메인으로 여러 서버를 묶을 수 있어요. Cloudflare를 쓴다면 가이드 버튼으로 바로 따라 할 수 있어요.',
		'mainPage.tour.account.title': '계정 설정',
		'mainPage.tour.account.body':
			'2단계 인증이나 비밀번호는 여기서 관리해요. 이 투어는 여기 안의 "다시 보기" 버튼으로 언제든 다시 볼 수 있어요.',

		'mainPage.create.loaderNameRequired': '구동기 이름을 입력해주세요.',
		'mainPage.create.jarRequired': '구동기 jar 파일을 선택해주세요.',
		'mainPage.create.jarUploadFailed':
			'서버는 생성됐지만 구동기 jar 업로드에 실패했습니다: {error}\n인스턴스 상세 페이지의 파일 탭에서 server.jar를 직접 업로드할 수 있습니다.',
		'mainPage.create.worldImportFailed':
			'서버는 생성됐지만 월드 데이터 적용에 실패했습니다: {error}\n인스턴스 상세 페이지에서 다시 시도할 수 있습니다.'
	},
	en: {
		'mainPage.header.createServer': '+ New Server',
		'mainPage.header.accountSettings': 'Account Settings',
		'mainPage.header.logout': 'Log out',
		'mainPage.loadError': 'Failed to load server list: {error}',
		'mainPage.tabs.instances': 'Instances',
		'mainPage.tabs.settings': 'Settings',
		'mainPage.settings.network': 'Network',
		'mainPage.settings.hardware': 'Hardware',
		'mainPage.settings.account': 'Account',

		'mainPage.instances.exampleServerName': 'Example Server',
		'mainPage.instances.exampleStatus': 'Running',
		'mainPage.instances.exampleBadge': 'Example',
		'mainPage.instances.empty': 'No server instances yet.',
		'mainPage.instances.stop': 'Stop',
		'mainPage.instances.start': 'Start',
		'mainPage.instances.console': 'Console',
		'mainPage.instances.delete': 'Delete',
		'mainPage.instances.confirmDelete': 'Delete this instance? Its world data will also be deleted.',

		'mainPage.status.stopped': 'Stopped',
		'mainPage.status.starting': 'Starting',
		'mainPage.status.running': 'Running',
		'mainPage.status.stopping': 'Stopping',
		'mainPage.status.crashed': 'Crashed',

		'mainPage.network.wanRequiresTotp': 'You must set up two-factor authentication before enabling external access.',

		'mainPage.swap.invalidSize': 'Enter a size greater than 0, in GB.',
		'mainPage.swap.confirmDisable': 'Turn off and delete the swap file completely?',

		'mainPage.overclock.waitTimeout':
			'Timed out waiting for servers to stop. Check the instance status and try again.',
		'mainPage.overclock.confirmRebootStop':
			'The following servers are running: {names}\nEach will be safely stopped before rebooting. Continue?',
		'mainPage.overclock.rebootNoResponse': 'No response after reboot. Try refreshing the page in a moment.',

		'mainPage.benchmark.confirmStop':
			'The following servers are running: {names}\nThey will be stopped for the stability test and restarted automatically when it finishes. Continue?',

		'mainPage.update.upToDate': 'You are on the latest version.',

		'mainPage.tour.createServer.title': 'Create a Server',
		'mainPage.tour.createServer.body': 'Create a new Minecraft server here in just a few clicks.',
		'mainPage.tour.console.title': 'Live Console',
		'mainPage.tour.console.body': 'View server logs in real time and type commands directly.',
		'mainPage.tour.settings.title': 'Settings',
		'mainPage.tour.settings.body':
			"Settings that don't belong to any single server, like external access, domain connection, and swap, live here.",
		'mainPage.tour.externalAccess.title': 'External Access',
		'mainPage.tour.externalAccess.body': 'Turn on external access here to invite friends to play with you.',
		'mainPage.tour.domain.title': 'Domain Connection',
		'mainPage.tour.domain.body':
			'If you own a domain, connect it to group multiple servers under subdomains. If you use Cloudflare, the guide button walks you through it.',
		'mainPage.tour.account.title': 'Account Settings',
		'mainPage.tour.account.body':
			'Manage two-factor authentication and your password here. You can replay this tour anytime with the "Replay" button in here.',

		'mainPage.create.loaderNameRequired': 'Enter a loader name.',
		'mainPage.create.jarRequired': 'Select a loader jar file.',
		'mainPage.create.jarUploadFailed':
			'The server was created, but uploading the loader jar failed: {error}\nYou can upload server.jar directly from the Files tab on the instance detail page.',
		'mainPage.create.worldImportFailed':
			'The server was created, but applying the world data failed: {error}\nYou can try again from the instance detail page.'
	}
};

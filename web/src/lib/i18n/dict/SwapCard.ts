export const messages = {
	ko: {
		'swapCard.title': '가상 메모리 (스왑)',
		'swapCard.description':
			'라즈베리파이 OS의 zram(RAM 내 압축 스왑)과는 별개로, 실제 디스크 공간을 추가 여유분으로 씁니다. 커널은 항상 실제 RAM과 zram을 먼저 쓰고, 그걸로도 부족할 때만 이 스왑파일을 사용합니다.',
		'swapCard.fetchError': '상태를 불러오지 못했습니다: {error}',
		'swapCard.statusOn': '켜짐',
		'swapCard.statusOnUsage': ': {sizeGb}GB 중 {usedGb}GB 사용 중',
		'swapCard.statusOff': '꺼짐',
		'swapCard.statusOffRemaining': ' (파일은 {sizeGb}GB로 남아있음)',
		'swapCard.statusNotSet': '설정 안 됨',
		'swapCard.freeDisk': '여유 공간: {freeGb}GB (스왑파일 자체 크기 포함)',
		'swapCard.sizePlaceholder': '예: 4',
		'swapCard.sizeUnit': 'GB',
		'swapCard.applying': '적용 중...',
		'swapCard.apply': '적용',
		'swapCard.disable': '끄고 삭제',
		'swapCard.loading': '불러오는 중...'
	},
	en: {
		'swapCard.title': 'Virtual memory (swap)',
		'swapCard.description':
			"Separate from Raspberry Pi OS's zram (compressed swap in RAM), this uses actual disk space as extra headroom. The kernel always uses real RAM and zram first, and only falls back to this swap file when those aren't enough.",
		'swapCard.fetchError': 'Failed to load status: {error}',
		'swapCard.statusOn': 'On',
		'swapCard.statusOnUsage': ': {usedGb}GB used out of {sizeGb}GB',
		'swapCard.statusOff': 'Off',
		'swapCard.statusOffRemaining': ' (file remains at {sizeGb}GB)',
		'swapCard.statusNotSet': 'Not set up',
		'swapCard.freeDisk': 'Free space: {freeGb}GB (including the swap file itself)',
		'swapCard.sizePlaceholder': 'e.g. 4',
		'swapCard.sizeUnit': 'GB',
		'swapCard.applying': 'Applying...',
		'swapCard.apply': 'Apply',
		'swapCard.disable': 'Turn off and delete',
		'swapCard.loading': 'Loading...'
	}
};

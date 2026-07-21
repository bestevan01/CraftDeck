export const messages = {
	ko: {
		'memoryConflictModal.title': '메모리 할당 조정 필요',
		'memoryConflictModal.descriptionSwap': '물리 RAM + 스왑 여유분',
		'memoryConflictModal.descriptionFull': '라즈베리파이의 전체 메모리',
		'memoryConflictModal.description': '실행하려는 서버들의 메모리 할당 합이 {boundary}을(를) 초과합니다. 아래에서 조정한 뒤 시작할 수 있습니다.',
		'memoryConflictModal.targetTag': '(시작 예정)',
		'memoryConflictModal.runningTag': '(실행 중, 변경 시 자동으로 재시작됩니다)',
		'memoryConflictModal.total': '합계 {totalGB}GB / 전체 {maxGB}GB',
		'memoryConflictModal.applying': '적용 중...',
		'memoryConflictModal.applyAndStart': '적용 후 시작',
		'memoryConflictModal.cancel': '취소'
	},
	en: {
		'memoryConflictModal.title': 'Memory allocation needs adjusting',
		'memoryConflictModal.descriptionSwap': 'physical RAM + available swap',
		'memoryConflictModal.descriptionFull': "the Raspberry Pi's total memory",
		'memoryConflictModal.description':
			'The combined memory allocation of the servers you want to start exceeds {boundary}. Adjust below, then you can start them.',
		'memoryConflictModal.targetTag': '(about to start)',
		'memoryConflictModal.runningTag': '(running, will restart automatically on change)',
		'memoryConflictModal.total': 'Total {totalGB}GB / {maxGB}GB',
		'memoryConflictModal.applying': 'Applying...',
		'memoryConflictModal.applyAndStart': 'Apply and start',
		'memoryConflictModal.cancel': 'Cancel'
	}
};

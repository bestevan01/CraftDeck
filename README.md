# CraftDeck

라즈베리파이에서 마인크래프트 서버를 웹 UI만으로 만들고, 켜고, 관리할 수 있게 해주는 셀프 호스팅 패널입니다.

Go 단일 바이너리 + SvelteKit 정적 프론트엔드로 구성되어 있으며, `apt` 패키지로 배포되어 `sudo apt install craftdeck` 한 번으로 systemd 서비스까지 자동으로 등록/구동됩니다.

## 주요 기능

### 서버 관리
- **다양한 구동기 지원**: Vanilla, Paper, Purpur, Folia, Pufferfish, Leaf, Fabric, NeoForge, 그리고 Velocity 프록시
- 커스텀 `.jar` 업로드 및 특정 빌드 고정 재설치
- 한 대의 라즈베리파이에서 여러 인스턴스를 동시에 운영, 인스턴스별 CPU/메모리 상한(cgroup)
- 인스턴스별 전용 시스템 계정으로 프로세스 격리 (한 서버의 플러그인/모드가 사고를 쳐도 다른 인스턴스에는 영향 없음)

### 여러 서버를 하나의 주소로
- Velocity 프록시를 자동으로 구성해 서브도메인별로 서로 다른 서버로 라우팅 (`survival.mydomain.com`, `creative.mydomain.com` 등)
- 예기치 못한 서버 다운 시 자동 페일오버

### 플러그인 / 모드
- Modrinth API 기반 검색 및 설치, 구동기별 호환 필터링
- 의존 플러그인/모드 자동 설치, SHA-512 무결성 검증
- 활성/비활성 전환 및 삭제

### 설정 / 파일 관리
- `server.properties`를 위한 GUI 폼과, 그 외 파일을 직접 편집할 수 있는 파일 관리자
- 월드 백업 및 복원

### 실시간 콘솔 및 명령
- WebSocket 기반 실시간 콘솔 로그 스트리밍
- 자주 쓰는 명령(저장, 종료, 강퇴, 밴, 화이트리스트, 운영자 권한, 공지, 게임모드, 시간/날씨, 난이도 등)을 버튼 클릭으로 실행
- 원문 명령어 직접 입력도 항상 가능

### 네트워크 / 외부 접속
- UPnP/NAT-PMP를 통한 포트 포워딩 자동 설정 (미지원 공유기는 수동 설정 안내)
- 무료 DDNS(DuckDNS 등) 또는 직접 소유한 도메인(Cloudflare 연동) 등록 지원
- 소유 도메인 사용 시 서브도메인별 A/AAAA/SRV 레코드 자동 생성 및 공인 IP 변경 시 자동 갱신

### 보안
- 외부 접속(WAN) 노출 시 자동으로 HTTPS 적용 (도메인이 있으면 Let's Encrypt, 없으면 자체 서명 인증서)
- 외부 접속 시 2단계 인증(TOTP) 강제, QR 코드 등록과 백업 코드 발급
- 로그인 실패 횟수 기반 계정 잠금, 외부 노출 시 더 엄격한 기본값 적용

### 유지보수
- Java 8/17/21/25 런타임을 패키지 설치 시 자동으로 함께 설치, 마인크래프트 버전에 맞는 런타임을 자동 선택
- `apt upgrade`로 다른 시스템 패키지(Java, systemd 등)가 업데이트될 때도 실행 중인 서버를 안전하게 보호(그레이스풀 셧다운 후 재개)
- 웹 UI에서 새 버전 알림을 받고 버튼 클릭 한 번으로 CraftDeck 자체를 업데이트

## 설치

라즈베리파이 OS(Debian 12/13 계열, arm64) 기준입니다.

```bash
curl -fsSL https://apt.craftdeck.cc/install.sh | sudo bash
```

이 스크립트는 CraftDeck과 Java 런타임(Eclipse Adoptium) apt 저장소를 등록하고, `craftdeck` 패키지를 설치한 뒤 systemd 서비스를 자동으로 시작합니다.

설치가 끝나면 터미널에 표시되는 주소로 접속하세요.

```
http://<라즈베리파이의-IP>:8080
```

### 수동 설치

저장소를 직접 등록하고 싶다면:

```bash
curl -fsSL https://apt.craftdeck.cc/craftdeck-archive-keyring.gpg | sudo tee /usr/share/keyrings/craftdeck-archive-keyring.gpg > /dev/null
echo "deb [arch=arm64 signed-by=/usr/share/keyrings/craftdeck-archive-keyring.gpg] https://apt.craftdeck.cc trixie main" | sudo tee /etc/apt/sources.list.d/craftdeck.list
sudo apt update
sudo apt install craftdeck
```

Java 런타임(Adoptium Temurin) 저장소는 `craftdeck` 패키지 설치 과정에서 필요 시 자동으로 함께 등록됩니다.

### 업데이트 / 삭제

```bash
sudo apt update && sudo apt upgrade craftdeck   # 업데이트 (또는 웹 UI의 업데이트 버튼 사용)
sudo apt remove craftdeck                        # 서비스 제거, 설정/데이터는 보존
sudo apt purge craftdeck                         # 설정까지 제거 (월드/백업 데이터는 삭제되지 않음)
```

## 기술 스택

| 영역 | 구성 |
|---|---|
| 백엔드 | Go (단일 정적 바이너리) |
| 프론트엔드 | SvelteKit (정적 빌드, 바이너리에 내장) |
| 저장소 | SQLite |
| 실시간 통신 | WebSocket |
| 게임 서버 제어 | RCON (자체 구현) |
| 프로세스 격리 | systemd-run / cgroup |
| 패키징 | apt (.deb, nfpm + reprepro) |

더 자세한 요구사항과 아키텍처 문서는 [requirements.md](requirements.md)를 참고하세요.

## 라이센스

[MIT](LICENSE)

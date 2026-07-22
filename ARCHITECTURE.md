# CraftDeck 아키텍처 설계 문서

이 문서는 [requirements.md](requirements.md)의 FR/NFR을 구현 가능한 구조로 옮긴 아키텍처 설계이다. 각 절에는 관련 FR/NFR 번호를 함께 표기하여 요구사항과의 추적성을 유지한다.

## 1. 설계 원칙

- **단일 바이너리, 단일 프로세스 데몬**: Go 정적 바이너리 하나가 웹 서버 + REST API + WebSocket 허브 + 모든 매니저(구동기/플러그인/네트워크/DNS/2FA)를 포함한다. 외부 런타임 의존성은 Java(Adoptium)뿐이다. (NFR-1, NFR-2)
- **컨테이너 없이 커널 네이티브 격리**: Docker 대신 `systemd-run` transient unit + cgroup v2로 서버별 자원 상한을 건다. (FR-11)
- **권한 분리**: 패널 데몬=root(승격 권한), 생성되는 마인크래프트/프록시 프로세스=강등된 별도 계정. (FR-43a)
- **어댑터 패턴으로 확장점 격리**: DDNS 제공자, 구동기 설치 파이프라인은 공통 인터페이스 뒤에 벤더별 구현체를 꽂는 구조로, 한 어댑터의 장애가 다른 기능에 전파되지 않는다. (FR-26c, FR-26d)
- **원격 API 장애에 대한 로컬 우선(Local-first) 원칙**: Modrinth API, DDNS API, Adoptium 저장소 등 외부 의존이 끊겨도 이미 설치된 서버/플러그인의 조회·실행·삭제는 항상 동작해야 한다. (FR-6b, NFR-7)

## 2. 컴포넌트 개요

박스 내부 라벨은 영문 컴포넌트 이름으로 통일했다(한글은 monospace 폰트에서 가로폭이 2칸으로 렌더링되어 박스 정렬이 깨지기 쉬움). 각 컴포넌트의 세부 동작은 5절 딥다이브에서 한글로 설명한다.

```
┌──────────────────────────┐
│ Web Browser (LAN or WAN) │
└──────────────────────────┘
               │  HTTPS (FR-33) / WebSocket
               ▼
┌──────────────────────────────────────────────────────────────────────┐
│ craftdeckd (Go binary, systemd service, runs as root)                │
│                                                                      │
│ ┌────────────────┐  ┌───────────────────┐  ┌───────────────┐         │
│ │ HTTP Router    │  │ Auth / Session    │  │ 2FA / TOTP    │         │
│ │ (embedded SPA) │  │ (bcrypt + cookie) │  │ (pquerna/otp) │         │
│ └────────────────┘  └───────────────────┘  └───────────────┘         │
│                                                                      │
│ ┌────────────────┐    ┌───────────────┐                              │
│ │ Server Manager │    │ Proxy Manager │                              │
│ └────────────────┘    └───────────────┘                              │
│                                                                      │
│ ┌────────────────┐    ┌────────────────────┐                         │
│ │ Loader Manager │    │ Plugin/Mod Manager │                         │
│ └────────────────┘    └────────────────────┘                         │
│                                                                      │
│ ┌────────────────────┐    ┌──────────┐                               │
│ │ Process Supervisor │    │ RCON Hub │                               │
│ └────────────────────┘    └──────────┘                               │
│                         ┌───────────────┐                            │
│                         │ WebSocket Hub │                            │
│                         └───────────────┘                            │
│                                                                      │
│ ┌─────────────────┐    ┌─────────────┐                               │
│ │ Network Manager │    │ DNS Manager │                               │
│ └─────────────────┘    └─────────────┘                               │
│                                                                      │
│ ┌──────────────────────────────────────────────────────────────────┐ │
│ │ SQLite (single file, WAL mode)                                   │ │
│ └──────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────┘
               │  systemd-run (fixed per-instance user, cgroup isolation)
               ▼
     ┌────────────────────┐  ┌────────────────────┐  ┌────────────────┐
     │ MC Server #1 (JVM) │  │ MC Server #2 (JVM) │  │ Velocity Proxy │
     │ Temurin 17         │  │ Temurin 21         │  │ (own sys user) │
     │ (own sys user)     │  │ (own sys user)     │  │                │
     └────────────────────┘  └────────────────────┘  └────────────────┘
```

## 3. 데이터 모델 (SQLite)

단일 SQLite 파일(WAL 모드로 동시 읽기 성능 확보). 아래는 핵심 테이블만 발췌.

```sql
-- 계정 및 2FA (FR-32, FR-39)
CREATE TABLE users (
  id INTEGER PRIMARY KEY,
  username TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,        -- bcrypt
  totp_secret TEXT,                   -- NULL이면 2FA 미등록
  totp_enabled INTEGER NOT NULL DEFAULT 0,
  backup_codes_json TEXT,             -- 해시된 복구 코드 배열
  created_at TEXT NOT NULL
);

CREATE TABLE sessions (
  id TEXT PRIMARY KEY,                -- 랜덤 세션 토큰(해시 저장)
  user_id INTEGER NOT NULL REFERENCES users(id),
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

-- 서버/프록시 인스턴스 (FR-1, FR-1a, FR-10)
CREATE TABLE instances (
  id TEXT PRIMARY KEY,                -- uuid
  name TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('server','proxy')),
  loader TEXT NOT NULL,               -- vanilla|paper|purpur|forge|fabric|velocity|bungeecord
  loader_version TEXT,
  mc_version TEXT,
  java_major INTEGER,                 -- 8|17|21 (kind=server일 때만)
  game_port INTEGER,
  rcon_port INTEGER,
  rcon_password TEXT NOT NULL,        -- 인스턴스별 랜덤 생성, 암호화 저장
  cpu_quota_percent INTEGER,          -- 예: 200 = 코어 2개
  memory_max_mb INTEGER,
  work_dir TEXT NOT NULL,
  status TEXT NOT NULL,               -- stopped|starting|running|stopping|crashed
  created_at TEXT NOT NULL
);

-- 프록시 백엔드 우선순위 목록 (FR-1b, FR-1c, FR-1d)
CREATE TABLE proxy_backends (
  proxy_id TEXT NOT NULL REFERENCES instances(id),
  backend_instance_id TEXT NOT NULL REFERENCES instances(id),
  priority INTEGER NOT NULL,          -- 1이 기본 접속 서버
  forced_host TEXT,                   -- 예: survival.mydomain.com (FR-1c)
  PRIMARY KEY (proxy_id, backend_instance_id)
);

-- 플러그인/모드 (FR-5~9, FR-6c, FR-6d)
CREATE TABLE plugins (
  id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL REFERENCES instances(id),
  source TEXT NOT NULL CHECK (source IN ('modrinth','upload')),
  modrinth_project_id TEXT,
  modrinth_version_id TEXT,
  filename TEXT NOT NULL,
  sha512 TEXT,                        -- 다운로드분만 검증 대상
  enabled INTEGER NOT NULL DEFAULT 1,
  installed_as_dependency INTEGER NOT NULL DEFAULT 0,  -- FR-6c 자동 설치 여부 추적
  created_at TEXT NOT NULL
);

-- 백업 (FR-13)
CREATE TABLE backups (
  id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL REFERENCES instances(id),
  filename TEXT NOT NULL,
  size_bytes INTEGER,
  created_at TEXT NOT NULL
);

-- 포트 포워딩 규칙 (FR-21~25)
CREATE TABLE port_mappings (
  id TEXT PRIMARY KEY,
  instance_id TEXT REFERENCES instances(id),  -- NULL이면 관리 웹 UI 포트
  external_port INTEGER NOT NULL,
  internal_port INTEGER NOT NULL,
  protocol TEXT NOT NULL CHECK (protocol IN ('tcp','udp')),
  method TEXT NOT NULL CHECK (method IN ('upnp','natpmp','manual')),
  created_at TEXT NOT NULL
);

-- DDNS 설정 (FR-26 ~ FR-31)
CREATE TABLE ddns_configs (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('free_subdomain','main_domain')),
  provider TEXT NOT NULL,             -- duckdns|noip|iptime|freedns|dynu|cloudflare
  hostname TEXT NOT NULL,
  mode TEXT NOT NULL CHECK (mode IN ('active','monitor')),  -- iptime=monitor
  credentials_encrypted TEXT,         -- API 토큰 등, active 모드에서만 사용
  last_known_ip TEXT,
  last_checked_at TEXT,
  created_at TEXT NOT NULL
);

-- 메인 도메인 서브도메인 할당 (FR-28, FR-29)
CREATE TABLE domain_assignments (
  ddns_config_id TEXT NOT NULL REFERENCES ddns_configs(id),
  instance_id TEXT NOT NULL REFERENCES instances(id),
  subdomain TEXT NOT NULL,
  srv_port INTEGER NOT NULL,
  PRIMARY KEY (ddns_config_id, subdomain)
);
```

**설계 노트**
- `rcon_password`, `credentials_encrypted`는 애플리케이션 레벨에서 AES-GCM으로 암호화 후 저장한다(키는 `/etc/craftdeck/master.key`, 파일 권한 `0600`, root 소유).
- `instances.status`는 실제 systemd 유닛 상태를 주기적으로 폴링해 동기화하는 캐시 컬럼이다. 진실의 원천(source of truth)은 systemd 자체이고, DB는 조회 성능을 위한 미러다.
- 콘솔 로그는 DB에 넣지 않고 파일시스템에 인스턴스별 로그 파일로 로테이션 저장한다(FR-16). WebSocket 재연결 시 최근 N줄만 파일에서 읽어 리플레이한다.

## 4. API 설계

### 4.1 REST API

```
Auth
  POST   /api/auth/login                 { username, password, totp? } → 세션 쿠키
  POST   /api/auth/logout
  POST   /api/auth/2fa/setup             → { qr_code_svg, secret, backup_codes }
  POST   /api/auth/2fa/verify            { code } → 2FA 활성화 확정

Instances (서버 + 프록시 공통)
  GET    /api/instances
  POST   /api/instances                  { name, kind, loader, mc_version|..., resources }
  GET    /api/instances/:id
  PATCH  /api/instances/:id              (설정 변경: server.properties, 자원 상한 등)
  DELETE /api/instances/:id
  POST   /api/instances/:id/start
  POST   /api/instances/:id/stop
  POST   /api/instances/:id/restart
  POST   /api/instances/:id/loader       { loader, version }   # FR-4 구동기 교체

Proxy 전용 (kind=proxy)
  GET    /api/instances/:id/backends
  PUT    /api/instances/:id/backends     [{ backend_instance_id, priority, forced_host }]
  GET    /api/instances/:id/failover-state   # 현재 실제 라우팅 중인 백엔드

Plugins/Mods
  GET    /api/instances/:id/plugins
  GET    /api/instances/:id/plugins/search?q=&loader=&mc_version=   # Modrinth 프록시
  POST   /api/instances/:id/plugins/install    { modrinth_version_id }  # FR-6c 의존성 자동 포함
  POST   /api/instances/:id/plugins/upload     (multipart, FR-3/FR-8)
  PATCH  /api/instances/:id/plugins/:pluginId  { enabled }
  DELETE /api/instances/:id/plugins/:pluginId

Backups
  GET    /api/instances/:id/backups
  POST   /api/instances/:id/backups
  POST   /api/instances/:id/backups/:backupId/restore

Console (GUI 버튼도 이 경로로 귀결, FR-18)
  POST   /api/instances/:id/command      { command }   # 텍스트/버튼 공통 진입점

Network
  GET    /api/network/port-mappings
  DELETE /api/network/port-mappings/:id
  GET    /api/network/gateway-status     # UPnP/NAT-PMP 탐색 결과

DDNS
  GET    /api/ddns
  POST   /api/ddns                        { kind, provider, hostname, credentials? }
  POST   /api/ddns/:id/verify-ownership   # 메인 도메인 TXT 레코드 검증 (FR-31)
  PUT    /api/ddns/:id/assignments        [{ instance_id, subdomain }]  # 메인 도메인만
  DELETE /api/ddns/:id

System
  GET    /api/system/java-runtimes        # 설치된 8/17/21 확인
  GET    /api/system/health
```

### 4.2 WebSocket 프로토콜

`GET /api/instances/:id/console` (업그레이드 후 JSON 프레임 교환)

```jsonc
// 서버 → 클라이언트
{ "type": "log",   "seq": 1234, "line": "[12:00:01] [Server thread/INFO]: Done!" }
{ "type": "state", "status": "running" }
{ "type": "cmd_result", "command": "whitelist add Steve", "ok": true }

// 클라이언트 → 서버
{ "type": "command", "text": "say hello" }        // 텍스트 콘솔 입력
```

GUI 버튼(FR-17)은 REST `POST /api/instances/:id/command`로도, 이 WebSocket 채널로도 보낼 수 있게 하되 **내부적으로는 항상 같은 RCON 실행 함수를 호출**한다 (FR-18). 프론트엔드는 REST를 기본으로 쓰고, 결과 반영은 WebSocket 스트림으로 확인한다(요청/응답 분리 → 응답 지연이 콘솔 스트림을 막지 않음).

## 5. 컴포넌트 딥다이브

### 5.1 Process Supervisor — systemd-run + 권한 분리 (FR-11, FR-43a~c)

인스턴스 시작 시 패널(root)이 실행하는 명령의 형태:

```
systemd-run \
  --unit=craftdeck-instance-<id> \
  --property=User=mc-<id 앞 12자리> \
  --property=Group=mc-<id 앞 12자리> \
  --property=MemoryMax=<memory_max_mb>M \
  --property=MemorySwapMax=0 \
  --property=CPUQuota=<cpu_quota_percent>% \
  --property=WorkingDirectory=<work_dir> \
  --property=Restart=no \
  -- /usr/lib/jvm/temurin-<java_major>-jre-<arch>/bin/java -jar server.jar nogui
```

- **계정 모델(확정, 실기 검증 후 수정): 인스턴스별 고정 시스템 계정**. 처음에는 systemd의 `DynamicUser=yes` + `StateDirectory=`로 설계했으나, 실제 라즈베리파이(Debian 13/trixie, arm64)에서 검증한 결과 이 조합이 우리 흐름(인스턴스 생성 시점에 구동기 jar를 미리 받아 작업 디렉터리에 심어두고, 이후 별도 시점에 시작)과 맞지 않았다. systemd는 "pre-existing public StateDirectory 디렉터리를 발견해 마이그레이션했다"는 로그를 남기고도, 실제로는 프로세스가 작업 디렉터리로 진입(`CHDIR`)하는 단계에서 `Permission denied`로 계속 실패했다. 그래서 대신 **패널이 인스턴스 생성 시 `useradd --system --no-create-home --shell /usr/sbin/nologin mc-<id>`로 고정 시스템 계정을 직접 만들고, 파일을 다 내려받은 뒤 그 디렉터리를 `chown -R`로 넘겨주는 방식**으로 확정했다(`internal/process/instanceuser.go`). `systemd-run`은 이 고정 계정을 `--property=User=`/`--property=Group=`으로 지정하기만 하면 된다.
- **경로 순회 권한(실기에서 발견)**: 최종 작업 디렉터리만 인스턴스 계정 소유로 바꾸는 것으로는 부족하다. 유닉스에서 하위 디렉터리에 CHDIR하려면 경로상의 **모든 상위 디렉터리**에 최소 실행(순회) 권한이 있어야 하므로, `<dataDir>`과 `<dataDir>/instances` 두 단계 모두 `0711`(다른 계정도 통과는 가능하지만 목록은 못 봄)로 만들어야 한다. 또한 데이터 루트를 사용자 홈 디렉터리처럼 기본이 `0700`인 경로 아래 두면, 하위 디렉터리 권한을 아무리 손봐도 그 상위에서 막히므로 반드시 `/var/lib/craftdeck`처럼 처음부터 순회 가능한 전용 경로를 써야 한다.
- **Java 실행 파일 경로(실기에서 발견)**: Adoptium 패키지가 실제로 설치하는 경로는 `/usr/lib/jvm/temurin-<major>-jre/bin/java`가 아니라 아키텍처 접미사가 붙은 `/usr/lib/jvm/temurin-<major>-jre-<arch>/bin/java`(예: `temurin-21-jre-arm64`)였다.
- **트레이드오프**: 한 인스턴스의 플러그인/모드가 RCE로 이어지더라도 다른 인스턴스의 월드 데이터·설정 파일은 서로 다른 계정으로 격리되어 접근 불가능하다 — 공유 계정 방식보다 침해 확산 범위가 크게 줄어든다. 대신 인스턴스 삭제 시 계정도 함께 정리(`userdel`)해야 시스템에 유령 계정이 쌓이지 않고, `/etc/passwd`에 인스턴스 수만큼 계정이 늘어난다는 점은 감안해야 한다.
- 패널은 systemd 유닛 상태(`systemctl is-active craftdeck-instance-<id>`)를 폴링하여 `instances.status`를 갱신하고, 유닛 종료 이벤트(OOM Kill 포함)를 감지해 크래시 여부를 판별한다.

### 5.2 Loader Manager (FR-1~4)

공통 인터페이스:

```go
type LoaderAdapter interface {
    ListVersions(ctx context.Context) ([]LoaderVersion, error)
    Download(ctx context.Context, version string, destDir string) error
}
```

구현체: `MojangVanillaAdapter`(version_manifest.json), `PaperAdapter`/`PurpurAdapter`(PaperMC API v2 스펙 공유), `FabricAdapter`(Fabric Meta API), `ForgeAdapter`(공식 installer jar 실행 후 산출물 수집 — 설치 스크립트 특성상 다른 어댑터보다 실패 케이스가 많아 별도 타임아웃/재시도 로직 필요), `VelocityAdapter`/`BungeeCordAdapter`.

구동기 교체(FR-4)는 `work_dir` 내 월드 데이터(`world/`, `server.properties` 등)를 보존한 채 실행 jar만 교체하는 방식으로 구현한다.

### 5.3 Plugin/Mod Manager (FR-5~9, FR-6a~d)

설치 파이프라인:

1. `GET /plugins/search`는 Modrinth API를 그대로 프록시하되 `loader`, `game_version` 파라미터로 서버 스펙에 안 맞는 항목을 미리 필터링한다(FR-6a).
2. 설치 요청 시 Modrinth 버전 메타데이터의 `dependencies` 필드를 재귀 탐색하여 미설치 필수 의존성을 큐에 추가한다(FR-6c). 순환 의존성 방지를 위해 방문한 project_id 집합을 유지한다.
3. 큐에 쌓인 파일들을 병렬 다운로드하고, 각 파일의 `sha512`를 Modrinth 응답값과 대조한다(FR-6d). 불일치 시 해당 파일과 그 파일에 의존하던 항목 전체를 롤백(임시 디렉터리 삭제)한다.
4. 검증 통과분만 `plugins/`(또는 `mods/`) 디렉터리로 이동, DB에 기록한다.
5. 비활성화는 파일을 `plugins_disabled/`로 이동하는 방식으로 구현하여 재시작 없이도 상태를 추적할 수 있게 한다(FR-7).

Modrinth API 장애 시(FR-6b) 위 파이프라인 전체가 실패하더라도, 목록 조회/삭제/토글 API는 DB와 로컬 파일시스템만 사용하므로 영향받지 않는다.

### 5.4 RCON Hub & WebSocket Hub (FR-14~20) — 실기 검증 완료

- 인스턴스 생성 시 `server.properties`에 `enable-rcon=true`, `rcon.port`(`game_port + 10000`), 무작위 `rcon.password`를 주입한다(`internal/api/handlers_instance.go`의 `provisionServerFiles`).
- **RCON 프로토콜**: `internal/rcon/rcon.go`가 Source RCON 와이어 프로토콜(인증/명령 실행 패킷)을 직접 구현한다.
- **상시 연결 매니저**: `internal/rcon/manager.go`의 `Manager`가 인스턴스당 하나의 지속 연결을 관리한다. `StartMaintaining`은 서버 시작 직후 호출되어, RCON 리스너가 뜰 때까지(부팅 중) 지수 백오프로 자체 재시도하는 백그라운드 goroutine을 띄운다. 연결이 끊기면(`Execute` 실패 감지) 자동으로 재다이얼을 시도한다. `StopMaintaining`은 인스턴스 중지/삭제 시 호출되어 연결을 정리한다.
- REST `POST /instances/{id}/command`와 WebSocket 콘솔의 `{"type":"command"}` 프레임 모두 동일한 `Manager.Execute(instanceID, cmd)`를 호출한다(FR-18). GUI 버튼은 프론트엔드에서 커맨드 문자열로 조립되어 같은 REST 엔드포인트로 전송되므로, 백엔드에는 "버튼 전용 코드 경로"가 존재하지 않는다.
- **콘솔 로그 스트리밍**: 별도의 파일 로테이션 파이프라인 대신, `journalctl -u craftdeck-instance-<id> -f -n 50 -o cat`을 서브프로세스로 띄워 그 표준출력을 그대로 WebSocket에 중계한다(`internal/api/handlers_console.go`). systemd가 이미 유닛별 로그를 관리해주므로 이 경로가 가장 단순했다.
- **그레이스풀 스톱**: `POST /instances/{id}/stop`은 먼저 매니저를 통해 RCON `stop` 명령을 보내고, 유닛이 최대 20초 안에 스스로 종료하는지 폴링한다. 그 안에 안 끝나면 그제서야 `supervisor.Stop`(강제 종료)으로 폴백한다.
- **실기 검증**: 라즈베리파이에서 실제로 (1) 부팅 중 매니저가 자동 재시도하다 RCON이 뜨자마자 연결 1회 수립, (2) 명령을 연달아 여러 번 보내도 연결 로그가 늘지 않고 재사용됨, (3) RCON `stop` → "월드 저장 완료" 로그 → 11초 내 정상 종료(강제 종료 없음), (4) WebSocket으로 실시간 로그 스트리밍 + 명령 전송 → `cmd_result` 응답 왕복까지 전부 확인했다.

### 5.5 Proxy Manager (FR-1a~1d)

- Velocity: `proxy_backends` 테이블 내용을 `velocity.toml`의 `[servers]`, `[forced-hosts]` 섹션으로 렌더링 후 `/reload` RCON 명령(또는 API 플러그인)으로 반영.
- 헬스체크는 각 백엔드 인스턴스의 game_port에 대해 주기적 TCP 핑(Server List Ping 패킷)을 수행. 1순위 실패 감지 시 `try` 순서를 2순위로 스왑하고 즉시 반영, 1순위 회복 감지 시 기본값으로 자동 원복(FR-1d).

### 5.6 Network Manager (FR-21~25)

- 시작 시 SSDP 브로드캐스트로 IGD 탐색 → 성공 시 `AddPortMapping` 호출, 실패 시 NAT-PMP 폴백, 그마저 실패하면 FR-23의 수동 안내 화면으로 전환.
- 인스턴스별 게임 포트와 관리 웹 UI 포트는 `port_mappings.instance_id`가 NULL인지 여부로 구분해 독립적으로 관리(FR-25).
- 인스턴스 삭제/정지, 서비스 종료 시 대응하는 매핑을 `DeletePortMapping`으로 정리(테어다운).

### 5.7 DNS Manager (FR-26~31)

공통 인터페이스는 능동 갱신과 감시를 분리한다:

```go
type ActiveDDNSAdapter interface {  // DuckDNS, No-IP 등
    Update(ctx context.Context, hostname string, ip net.IP, creds Credentials) error
}

type MonitorOnlyDDNSAdapter interface {  // ipTime
    Resolve(ctx context.Context, hostname string) (net.IP, error)  // 그냥 공개 DNS 조회
}
```

- 능동 어댑터는 주기적 타이머(예: 5분)로 현재 WAN IP를 조회 후 갱신 요청.
- 감시 전용(ipTime)은 같은 주기로 `Resolve`만 호출해 현재 WAN IP와 비교, 불일치 시 알림만 발생시킨다(FR-26f).
- 메인 도메인(Cloudflare)은 A/AAAA + `domain_assignments`에 있는 서브도메인마다 `_minecraft._tcp.<subdomain>` SRV 레코드까지 갱신한다(FR-29).
- 어댑터 하나가 패닉/에러를 던져도 `recover()`로 격리하여 다른 어댑터 및 앱 전체에 전파되지 않도록 한다(FR-26d).

### 5.8 Java Runtime & 패키징 (FR-41~45)

- `postinst`: (1) 패널용 전용 시스템 계정 `craftdeck` 생성 (인스턴스 프로세스용 `mc-<id>` 계정은 5.1절대로 각 인스턴스 생성 시점에 패널이 직접 `useradd`로 만들므로 여기서 사전 생성할 필요 없음) → (2) 패키지에 내장된 Adoptium GPG 키로 APT 저장소 등록 → (3) `temurin-8-jre`, `temurin-17-jre`, `temurin-21-jre` 설치(실패 시 설치 중단, FR-42d) → (4) systemd 서비스 enable + start.
- Java 버전 선택기는 인스턴스 생성 시 `mc_version`을 파싱해 필요 메이저 버전을 결정하고 `instances.java_major`에 고정 저장 — 이후 마인크래프트 버전이 패치되어도 이미 만든 인스턴스의 Java 버전은 사용자가 명시적으로 바꾸기 전까지 유지된다.
- `apt upgrade` 중에는 실행 중인 유닛에 `systemctl stop` 대신 RCON `save-all` + graceful `stop` 명령을 우선 시도한 뒤 패키지 파일 교체, 이후 서비스 재기동(FR-45).

## 6. 신뢰성 및 실패 모드

| 실패 시나리오 | 영향 범위 | 대응 |
|---|---|---|
| 특정 인스턴스 JVM OOM Kill | 해당 인스턴스만 (cgroup 격리) | Process Supervisor가 크래시 감지 → 상태를 `crashed`로 갱신, 프록시가 백엔드였다면 FR-1d 페일오버 |
| Modrinth API 다운 | 신규 설치/검색만 불가 | 기존 플러그인 조회/삭제/토글은 로컬 DB 기반이라 정상 동작 (FR-6b) |
| 특정 DDNS 제공자 API 오류 | 해당 어댑터만 | 다른 DDNS 설정 및 앱 전체 기능 정상 (FR-26d) |
| Adoptium 저장소 접속 불가(설치 시) | 설치 자체 중단 | 명확한 에러로 설치 실패 처리, 반쪽 설치 방지 (FR-42d) |
| 공유기 UPnP/NAT-PMP 미지원 | 자동 포트포워딩만 불가 | 수동 설정 안내 화면 (FR-23) |
| 패널 프로세스 자체 크래시 | 전체 서비스 중단 | systemd `Restart=on-failure`로 자동 재기동. 재기동 후 각 인스턴스 상태를 systemd에서 재동기화 |

## 7. 트레이드오프 요약

- **백엔드 언어 Go (vs. Node.js/Python/JVM 기반, vs. Rust)**: GC가 있어 Rust만큼 런타임 오버헤드를 줄이지는 못하지만, Node/Python/JVM 런타임보다는 훨씬 가벼워 상시 구동 시 유휴 메모리 사용량이 낮다(NFR-1). 단일 정적 바이너리로 arm64/armhf 크로스컴파일이 간단해 NFR-2를 쉽게 만족하고, WebSocket/RCON/UPnP/TOTP 등 이미 검증된 라이브러리 생태계 덕분에 Rust보다 개발 속도에서 유리하다 — 이 프로젝트 규모엔 합리적인 중간 지점(상세 비교는 requirements.md 6.1절).
- **프론트엔드 SvelteKit (vs. React 19)**: React는 가상 DOM diffing 런타임을 브라우저로 함께 보내야 해서 번들이 더 크지만, Svelte는 컴파일 타임에 반응형 갱신 코드를 생성해 런타임이 거의 없다 — Go 바이너리에 `embed.FS`로 통째로 내장하는 이 프로젝트엔 가벼운 정적 빌드 산출물이 유리하다. 다만 이는 "단일 관리 대시보드 + 정적 빌드 임베드" 규모에서 성립하는 판단이고, React 19의 생태계 규모나 팀 숙련도가 더 중요해지는 상황이라면 뒤집힐 수 있다(상세 비교는 requirements.md 6.1절).
- **컨테이너 미사용**: 오버헤드는 최소화되고, 인스턴스별 고정 시스템 계정으로 파일시스템 격리 수준도 확보했다(5.1절). 다만 네트워크 네임스페이스 격리 등 Docker가 기본 제공하는 일부 격리축은 없다 — 라즈베리파이 자원 제약을 감안한 의도적 트레이드오프.
- **단일 SQLite 파일 (vs. Postgres/MySQL)**: 운영 단순성은 높지만 다중 라이터 동시성은 Postgres류보다 낮음 — 이 프로젝트 규모(개인/소규모 홈랩)에서는 충분하다고 판단. 별도 DB 서버 프로세스가 없어 NFR-1(유휴 메모리 최소화)과 apt 배포 단순성(NFR-4)에도 유리하며, "단일 노드, 낮은 동시 쓰기 부하" 전제는 NFR-3의 확장 방향(단일 Pi 내 스케일업)과도 어긋나지 않는다(상세 비교는 requirements.md 6.1절).
- **패널=root 권한**: systemd-run/cgroup 제어를 위한 실용적 선택이지만, 웹 인터페이스 자체의 보안이 곧 시스템 전체의 보안 경계가 된다(리스크 문서화 완료, requirements.md 8절).
- **인스턴스별 고정 시스템 계정 (당초 DynamicUser에서 실기 검증 후 변경)**: 침해 확산 범위를 인스턴스 단위로 좁히는 대가로, 인스턴스 생성/삭제 시마다 시스템 계정을 만들고 지워야 하는 관리 부담이 늘어난다(5.1절). 처음에는 systemd `DynamicUser=yes`로 이 관리 부담 자체를 없애려 했으나, 실제 라즈베리파이에서 검증한 결과 파일을 미리 심어두는 우리 흐름과 맞지 않아 CHDIR 권한 오류가 계속 발생해 고정 계정 방식으로 전환했다.

## 8. 성장 시 재검토할 지점

- 인스턴스 수가 많아지면(예: 여러 라즈베리파이로 분산) 현재의 "패널=root, 단일 SQLite" 구조는 재검토 필요 — 지금은 요구사항(NFR-3의 단일 Pi 스케일업, 멀티 Pi 클러스터링은 명시적으로 1차 범위 제외)과 일치하므로 현 설계 유지.
- CurseForge 지원, NeoForge/Quilt/Spigot 구동기 확장은 requirements.md에 이미 백로그로 명시된 대로 Loader/Plugin 어댑터 인터페이스에 구현체만 추가하면 되는 구조로 미리 열어둠.
- 인스턴스별 고정 시스템 계정 방식은 인스턴스를 생성/삭제할 때마다 `/etc/passwd`/`/etc/group`에 계정을 추가/제거한다 — 인스턴스 수가 매우 많아지는 시나리오(현재 목표 규모를 벗어남)에서는 이 방식의 관리 오버헤드를 재검토할 필요가 있음.

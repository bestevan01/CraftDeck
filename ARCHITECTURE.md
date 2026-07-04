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
               │  systemd-run (DynamicUser per instance, cgroup isolation)
               ▼
     ┌────────────────────┐  ┌────────────────────┐  ┌────────────────┐
     │ MC Server #1 (JVM) │  │ MC Server #2 (JVM) │  │ Velocity Proxy │
     │ Temurin 17         │  │ Temurin 21         │  │ (dynamic user) │
     │ (dynamic user)     │  │ (dynamic user)     │  │                │
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

### 5.1 Process Supervisor — systemd-run + 권한 분리 (FR-11, FR-43a)

인스턴스 시작 시 패널(root)이 실행하는 명령의 형태:

```
systemd-run \
  --unit=craftdeck-instance-<id> \
  --property=DynamicUser=yes \
  --property=StateDirectory=craftdeck/instances/<id> \
  --property=MemoryMax=<memory_max_mb>M \
  --property=MemorySwapMax=0 \
  --property=CPUQuota=<cpu_quota_percent>% \
  --property=WorkingDirectory=<work_dir> \
  --property=Restart=no \
  -- /usr/lib/jvm/temurin-<java_major>-jre/bin/java -jar server.jar nogui
```

- **계정 모델(확정): 인스턴스별 `DynamicUser`**. 각 서버/프록시 인스턴스는 `systemd-run --property=DynamicUser=yes`로 그때그때 할당되는 전용 임시 UID/GID 아래에서 실행된다. `StateDirectory=`에 인스턴스 ID를 포함시켜 systemd가 `/var/lib/craftdeck/instances/<id>`를 해당 동적 UID 소유로 자동 생성·유지하게 하고, 패널은 인스턴스의 `work_dir`을 항상 이 경로로 고정한다.
- 인스턴스 재시작 시에도 동일한 유닛 이름(`craftdeck-instance-<id>`)에 대해 systemd가 이전에 할당한 UID를 재사용하므로(같은 유닛명 기준으로 `DynamicUser`가 매핑을 캐시), 재시작 사이 파일 소유권 문제가 생기지 않는다.
- **트레이드오프**: 한 인스턴스의 플러그인/모드가 RCE로 이어지더라도 다른 인스턴스의 월드 데이터·설정 파일은 서로 다른 UID로 격리되어 접근 불가능하다 — 공유 계정 방식보다 침해 확산 범위가 크게 줄어든다. 대신 (a) 각 인스턴스의 `StateDirectory` 경로/권한을 패널이 정확히 추적·관리해야 하고, (b) 외부 도구(SFTP, 수동 `ls` 디버깅 등)로 특정 인스턴스 파일에 접근하려면 매번 동적 UID를 조회해야 하는 등 운영 편의성 측면의 복잡도가 공유 계정 대비 늘어난다.
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

### 5.4 RCON Hub & WebSocket Hub (FR-14~20)

- 인스턴스 시작 시 `server.properties`에 `enable-rcon=true`, 무작위 RCON 비밀번호/포트를 주입.
- 패널은 인스턴스당 하나의 상시 RCON 소켓 연결을 유지(재연결 백오프 포함).
- REST `POST /command`와 WebSocket `{"type":"command"}` 모두 동일한 `RCONClient.Execute(instanceID, cmd)` 함수를 호출 → 실행 결과는 해당 인스턴스를 구독 중인 모든 WebSocket 클라이언트에 브로드캐스트.
- GUI 버튼(FR-17 목록)은 프론트엔드에서 커맨드 문자열로 조립되어 동일 API로 전송되므로, 백엔드에는 "버튼 전용 코드 경로"가 존재하지 않는다.

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

- `postinst`: (1) 패널용 전용 시스템 계정 `craftdeck` 생성 (인스턴스 프로세스는 5.1절대로 `DynamicUser`로 그때그때 할당되므로 사전 생성 불필요) → (2) 패키지에 내장된 Adoptium GPG 키로 APT 저장소 등록 → (3) `temurin-8-jre`, `temurin-17-jre`, `temurin-21-jre` 설치(실패 시 설치 중단, FR-42d) → (4) systemd 서비스 enable + start.
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

- **컨테이너 미사용**: 오버헤드는 최소화되고, `DynamicUser` 기반 인스턴스별 UID 분리로 파일시스템 격리 수준도 확보했다(5.1절). 다만 네트워크 네임스페이스 격리 등 Docker가 기본 제공하는 일부 격리축은 없다 — 라즈베리파이 자원 제약을 감안한 의도적 트레이드오프.
- **단일 SQLite 파일**: 운영 단순성은 높지만 다중 라이터 동시성은 Postgres류보다 낮음 — 이 프로젝트 규모(개인/소규모 홈랩)에서는 충분하다고 판단.
- **패널=root 권한**: systemd-run/cgroup 제어를 위한 실용적 선택이지만, 웹 인터페이스 자체의 보안이 곧 시스템 전체의 보안 경계가 된다(리스크 문서화 완료, requirements.md 8절).
- **인스턴스별 DynamicUser**: 침해 확산 범위를 인스턴스 단위로 좁히는 대가로, `StateDirectory` 경로/동적 UID 추적 등 패널 쪽 구현 복잡도가 공유 계정 방식보다 늘어난다(5.1절). 격리 강화를 우선한 선택.

## 8. 성장 시 재검토할 지점

- 인스턴스 수가 많아지면(예: 여러 라즈베리파이로 분산) 현재의 "패널=root, 단일 SQLite" 구조는 재검토 필요 — 지금은 요구사항(NFR-3의 단일 Pi 스케일업, 멀티 Pi 클러스터링은 명시적으로 1차 범위 제외)과 일치하므로 현 설계 유지.
- CurseForge 지원, NeoForge/Quilt/Spigot 구동기 확장은 requirements.md에 이미 백로그로 명시된 대로 Loader/Plugin 어댑터 인터페이스에 구현체만 추가하면 되는 구조로 미리 열어둠.

# PerfectPixel 스킬 (외부 배포용)

설치형 PerfectPixel 데스크톱 앱과 **동일한** 스프라이트 생성 파이프라인을 Claude Code
스킬로 패키징한 것이다. 텍스트 설명 한 줄로 캐릭터 + 동작 애니메이션 + 8방향 세트를
만들고 게임 엔진용 번들로 내보낸다.

내부적으로 헤드리스 CLI `ppgen`(`cmd/ppgen`)을 구동하며, 이 CLI는 앱의
`GenerateState` + `ExportProject` 로직을 GUI 대화상자 없이 재현한다.

## 구성 (플러그인 레이아웃)

```
.claude-plugin/marketplace.json            저장소 = 마켓플레이스 매니페스트 (repo 루트)
skill/perfectpixel/                         플러그인 루트
  .claude-plugin/plugin.json                플러그인 매니페스트
  README.md
  skills/perfectpixel/                       실제 스킬
    SKILL.md                                 스킬 정의 + Claude 실행 지침 (user-invocable)
    scripts/install.sh                       ppgen 설치 (프리빌트 다운로드 → 소스 빌드 폴백)
    reference/
      presets.md                             100여 종 동작 프리셋 카탈로그
      providers.md                           프로바이더 · 모델 · 환경변수 · 스타일
    bin/ppgen                                설치 산출물 (install.sh 생성, git 미추적)
    .src/                                     소스 클론 캐시 (git 미추적)
```

## 설치 (외부 사용자)

### A. 마켓플레이스 플러그인 (권장)

Claude Code 세션에서:

```
/plugin marketplace add gykim80/perfectpixel-studio
/plugin install perfectpixel@perfectpixel-studio
```

설치 후 `/perfectpixel` 또는 자연어 요청("기사 캐릭터 걷기/공격 스프라이트 만들어줘")으로
호출한다. 첫 호출 시 `install.sh`가 OS/arch에 맞는 프리빌트 `ppgen`을 GitHub Releases에서
내려받는다(Go 불필요). 릴리스가 없거나 네트워크가 막히면 Go 소스 빌드로 폴백한다.

### B. 개인 스킬로 직접 복사

```bash
# 스킬 폴더만 복사 (플러그인 매니페스트 없이도 동작)
cp -R skill/perfectpixel/skills/perfectpixel ~/.claude/skills/perfectpixel
bash ~/.claude/skills/perfectpixel/scripts/install.sh   # ppgen 설치

# API 키 설정 (택1)
export GEMINI_API_KEY=...                 # 환경변수, 또는
#   ~/.config/perfectpixel/config.json    # 설치형 앱 설정 공유, 또는
#   작업 폴더의 .env                        # GEMINI_API_KEY=... 한 줄
```

### C. 직접 CLI로 사용

```bash
skills/perfectpixel/bin/ppgen \
  -desc "a small knight with silver armor" \
  -style pixel \
  -states "idle,walk,run,attack" \
  -dirset walk \
  -out ./knight-sprites \
  -json
```

## ppgen 플래그 요약

| 플래그 | 설명 |
|---|---|
| `-desc` | 캐릭터 설명 (영어 권장) |
| `-style` | `pixel`(기본) / `chibi` / `cartoon` / `retro16` |
| `-states` | 쉼표 구분 상태 키 (`reference/presets.md` 참고) |
| `-percat N` | 카테고리당 N개 자동 선택 |
| `-all` | 전체 프리셋 (100여 개, 비용 큼) |
| `-dirset KEY` | 한 상태의 8방향 세트 추가 생성 |
| `-base FILE` | 기존 `base.png`를 재사용해 같은 캐릭터 정체성으로 상태/방향 재생성 |
| `-out DIR` | 출력 디렉토리 (기본 `./perfectpixel-out`) |
| `-provider` / `-key` / `-model` | 프로바이더/키/모델 강제 지정 |
| `-seed N` | 지원 프로바이더에서 결정적 샘플링 요청 (`fal`, `gemini`, `openrouter`) |
| `-attempts N` | 상태별 품질 보정 재시도 (기본 3) |
| `-timeout` | 전체 타임아웃 (기본 30m) |
| `-json` | 결과 요약 JSON만 stdout 출력 (스크립트/스킬용) |
| `-dump` | 프리셋·방향·스타일·프로바이더 카탈로그 출력 후 종료 |

캐릭터 정체성을 런 간 고정하려면 먼저 `-baseonly -out ./char-base`로 `base.png`를 확정한 뒤,
이후 생성은 `-base ./char-base/base.png`를 함께 넘긴다. `-seed`는 같은 요청의 재현성을 돕지만,
복장·팔레트 고정의 1차 수단은 실제 이미지 참조인 `-base`다.

## 산출물

`-out` 디렉토리에 `base.png`, `sprite-sheet.png`, `manifest.json`(PerfectPixel schema v2),
`sprite-sheet.json`(Aseprite 호환), `frames/<state>/frame-NN.png`, `gif/<state>.gif`,
`apng/<state>.png` 가 생성된다. 대부분의 게임 엔진은 `sprite-sheet.png` + `sprite-sheet.json`
한 쌍으로 임포트한다 (Phaser / Unity / Godot 의 Aseprite 임포터).

## 배포(메인테이너용)

프리빌트 바이너리는 GitHub Actions(`.github/workflows/release-ppgen.yml`)가 만든다.
버전 태그를 push 하면 darwin/linux(amd64·arm64) + windows(amd64)용 `ppgen-<os>-<arch>`를
크로스컴파일해 릴리스에 첨부한다.

```bash
git tag v1.0.0
git push origin v1.0.0   # → release-ppgen 워크플로 실행, 자산 첨부
```

이후 사용자의 `install.sh`는 `releases/latest/download/ppgen-<os>-<arch>`를 받아 Go 없이
설치한다. `PP_VERSION=v1.0.0`으로 특정 버전 고정, `PP_BUILD=1`로 다운로드를 건너뛰고
항상 소스 빌드도 가능하다.

마켓플레이스 갱신: 스킬/플러그인 내용이 바뀌면 `skill/perfectpixel/.claude-plugin/plugin.json`의
`version`을 올리고 푸시하면 된다. 사용자는 `/plugin marketplace update perfectpixel-studio`로 갱신한다.

## 설치형 앱과의 관계

스킬과 앱은 같은 Go 코드(`internal/sprite`, `internal/gen`, `internal/config`)를 공유한다.
알고리즘이 개선되면 앱과 스킬 모두 동일하게 반영되고, 새 릴리스 태그를 통해 프리빌트
`ppgen`이 갱신된다(또는 `install.sh`가 최신 소스로 재빌드).

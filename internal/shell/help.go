package shell

import "fmt"

// PrintHelp prints help. If topic is empty, prints the full help.
func PrintHelp(topic string) {
	if topic == "" {
		printHelpAll()
		return
	}
	switch topic {
	case "shell", "repl":
		fmt.Println(`
[help:shell]
  - 인터랙티브 모드: kiki-ai-shell
  - 프롬프트:
      ?질문    -> LLM에게 질문
      ??질문   -> 안전한 명령 제안(실행하지 않음)
  - 일반 명령 입력은 /bin/bash -lc 로 실행됩니다.
`)
	case "llm":
		fmt.Println(`
[help:llm]
  - OpenAI 호환 /v1/chat/completions 엔드포인트를 호출합니다.
  - 환경변수:
      LLM_BASE_URL (예: http://10.0.2.253:8080)

  - 실행 중 엔드포인트 변경(권장):
      :llm show
      :llm set http://10.0.2.253:8080
      :llm clear

      LLM_MODEL (default llama)
      LLM_TEMP
      LLM_MAX_TOKENS
      LLM_TIMEOUT
      LLM_PROFILE (none|fast|deep)
      LLM_STREAM (0|1)
      LLM_SYSTEM_PROMPT

      LLM_CTX_TARGET (표시/가이드용)
      LLM_CTX_OBSERVED (관측값 강제)
`)
	case "file":
		fmt.Println(`
[help:file]
  - 원샷 첨부:
      ./kiki-ai-shell -f /var/log/messages ask "이 로그 분석"
  - 쉘 첨부:
      :file add /var/log/messages
      :file list
      :file rm 1
      :file clear

  - 제한:
      LLM_FILE_MAX_BYTES (default 256KB)
      LLM_FILE_MAX_CHARS (default 20000)
`)
	case "ctx":
		fmt.Println(`
[help:ctx]
  - 컨텍스트는 시스템 프롬프트 뒤에 [Context] 섹션으로 붙습니다.
      :ctx set cluster=prod
      :ctx set ns=kube-system
      :ctx show
      :ctx clear
`)
	case "ctx-size":
		fmt.Println(`
[help:ctx-size]
  - ctx-size는 LLM의 컨텍스트 윈도우(토큰 수)입니다.
  - kiki-ai-shell은 "목표값"을 저장/표시/가이드할 수 있지만,
    실제 적용은 llama.cpp 서버를 --ctx-size 로 재시작해야 합니다.

  명령:
      :ctx-size 15000

  환경변수:
      LLM_CTX_TARGET=15000
      LLM_CTX_OBSERVED=8192   (선택: 관측값 강제)
`)
	case "ui":
		fmt.Println(`
[help:ui]
  - 헤더 고정(권장):
      KIKI_UI_FIXED=1
      KIKI_UI_HEADERLINES=5
  - 토글:
      :ui header on|off
      :ui clear on|off   (레거시: 전체 clear)
		:nofence on|off     LLM 출력에서 마크다운 코드펜스(three backticks) 제거
`)
	case "history":
		fmt.Println(`
[help:history]
  - 저장된 사용 이력(명령/질문)을 조회/요약합니다.
  - 명령:
      :history tail [N]
      :history search "keyword"
      :history summarize [days]
`)
	case "pcp":
		fmt.Println(`
[help:pcp]
  - PCP(Performance Co-Pilot)로 시스템 지표를 조회합니다.
  - 로컬은 pcp 패키지(특히 pmrep)가 설치되어 있어야 합니다.
  - 원격은 대상 서버에 pmcd가 실행 중이어야 하며 방화벽에서 44321/tcp 접근이 가능해야 합니다.

  명령:
    :pcp show
    :pcp host local|<hostname|ip>
    :pcp cpu
    :pcp mem
    :pcp load
    :pcp raw <metric...>

  예:
    :pcp host 192.168.10.20
    :pcp cpu
    :pcp raw kernel.all.load mem.util.used
`)
	default:
		printHelpAll()
		fmt.Println("topics: shell | llm | file | ctx | ctx-size | ui | history | pcp")
	}
}

func printHelpAll() {
	fmt.Println(`
kiki-ai-shell

=== 실행 방식 ===
  kiki-ai-shell                  인터랙티브 쉘
  kiki-ai-shell ask "질문"       단일 질문(원샷)
  kiki-ai-shell "질문"           ask 단축형
  kiki-ai-shell --help           도움말(전체)

=== LLM 질문(대화) ===
  ?질문                           LLM 질의 (현재 profile/stream/files 반영)
  ??질문                          안전한 "명령 제안" 모드(직접 실행하지 않음)

=== 내부 명령(:로 시작) ===
  :help [topic]                   도움말 (topic: shell|llm|file|ctx|ctx-size|ui|history)
  :profile fast|deep|none         프로파일 변경
  :stream on|off                  스트리밍 출력 on/off
  :nofence on|off                 LLM 출력에서 코드펜스(three backticks, yaml fence 포함) 제거
  :ui header on|off               상단 헤더 표시 on/off
  :ui clear on|off                화면 전체 clear(레거시) on/off

  :llm show                       현재 LLM_BASE_URL 표시
  :llm set <base_url>             실행 중 LLM_BASE_URL 변경
  :llm clear                      LLM_BASE_URL 초기화(Host:Port로 fallback)

  :ctx set key=value              컨텍스트 설정 (예: cluster, ns)
  :ctx show                       컨텍스트 표시
  :ctx clear                      컨텍스트 초기화

  :ctx-size N                     목표 ctx-size 설정(서버 재시작 필요)

  :gen <path> <prompt...>         코드만 생성 후 파일로 저장(저장 전 확인)

  :file add /path                 파일 첨부
  :file list                      첨부 목록
  :file rm N                      N번째 제거
  :file clear                     전체 제거

  gen <path> <prompt...>          (REPL) 코드만 생성 후 파일로 저장

  :history ...                    사용 이력 조회/요약
  :pcp ...                        PCP 기반 시스템 지표 조회(:help pcp)
  :bash                           PTY 기반 bash 진입 (exit로 복귀)
  :exit | :quit                   종료

TIP:
  - 헤더 고정: KIKI_UI_FIXED=1 (default)
  - LLM 서버:  LLM_BASE_URL (또는 쉘에서 :llm set ...)
`)
}

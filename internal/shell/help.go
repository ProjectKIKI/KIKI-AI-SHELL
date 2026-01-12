package shell

import "fmt"

func PrintHelp() {
	fmt.Print(`
KIKI AI SHELL (Step 3)

=== 실행 ===
  kiki-ai-shell                 인터랙티브 쉘
  kiki-ai-shell ask "질문"      단일 질문(원샷)
  kiki-ai-shell -f /path "질문" 파일 첨부 + 질문

=== 질문 ===
  ?질문                         LLM 질의 (profile/stream/files/rag 반영)

=== 내부 명령(:) ===
  :help                         도움말
  :profile fast|deep|none       프로파일 변경
  :stream on|off                스트리밍 출력 on/off
  :timeout N                    LLM HTTP timeout(sec) 변경
  :ui header on|off             상단 헤더 표시
  :ctx set key=value            컨텍스트 설정 (cluster/ns 등)
  :ctx show                     컨텍스트 표시
  :ctx clear                    컨텍스트 초기화
  :ctx-size N                   목표 ctx-size 설정(서버 재시작 필요)
  :rag on|off                   RAG 사용 on/off
  :rag add /path                파일을 RAG 인덱스에 추가 (file->rag)
  :rag stats                    RAG 상태/문서 수
  :history show [N]             히스토리 최근 N개 요약
  :history search <regex> [N]   히스토리 검색(정규식)
  :history path                 히스토리 파일 경로 출력
  :rag clear                    RAG 문서 초기화
  :file add /path               파일 첨부
  :file list                    첨부 목록
  :file rm N                    N번째 제거
  :file clear                   전체 제거
  :bash                         bash 진입 (exit로 복귀)
  :exit | :quit                 종료
`)
}

# kiki-ai-shell

KIKI AI Shell은 **OpenAI 호환 `/v1/chat/completions` 엔드포인트**(예: llama.cpp server)를 대상으로, 터미널에서 **LLM 질의 + 파일 첨부 + 컨텍스트(ctx) 관리 + bash 실행**을 하나로 묶은 **인터랙티브 쉘**입니다.

- `?질문`으로 LLM에게 바로 질문
- `:ctx`로 K8S cluster/namespace 같은 컨텍스트를 주입
- `:ctx-size`로 목표 context window를 관리하고(서버 재시작 가이드)
- 상단 고정 헤더에서 현재 상태(LLM/profile/stream/ctx/K8S/files)를 계속 보여줍니다.
- `:file`로 특정파일을 등록 후 분석이 가능 합니다.

---

## 1) 요구사항

- Go 1.21+ 권장
- OpenAI 호환 LLM 런타임(현재, llama.cpp 사용)
  - 기본값: `http://10.0.2.253:8080/v1/chat/completions`
  - 환경변수 `LLM_HOST/LLM_PORT` 또는 `LLM_BASE_URL`로 변경

---

## 2) 빌드/실행

### 빌드
```bash
go build -o kiki kiki-shell.go
cp -m 644 kiki-shell /usr/local/bin
```

### 인터렉티브 쉘
```bash
usermod -s /usr/local/bin/kiki-shell <USERNAME>
usermod -s /usr/local/bin/kiki-shell xinick
```

## 파일 첨부 및 분석

```bash
kiki -f /var/log/message ask "이 로그 분석해줘"
```
혹은 아래와 같이 사용이 가능하다. 등록된 파일 제거시 rm혹은 clear명령어를 통해서 초기화가 가능하다.
```bash
:file add /var/log/messages
:file add /var/log/secure
?시간 순서로 사건 흐름 정리해줘
?보안 이벤트 의심되는 부분만 뽑아줘
:file rm 2
?messages만 기준으로 장애 원인 추정해줘
:file list|ls
:file rm 1
:file clear
```
## 파일 첨부 제한(중요)

너무 큰 로그 혹은 텍스트 파일은 CPU혹은 GPU기반에서 토큰에 전부 전달이 불가능할 수도 있습니다. 이러한 이유로 파일을 잘라서 토큰으로 전달 합니다.

- 바이트 제한: LLM_FILE_MAX_BYTES(기본 256KB)
- 문자 제한: LLM_FILE_MAX_CHARS(기본 20000)

```bash
export LLM_FILE_MAX_BYTES=$((1024*1024))
export LLM_FILE_MAX_CHARS=80000
kiki -f /var/log/message ask "전체적인 내용 분석해줘"
```
파일 제한을 키우면 ctx-size초과가 발생할 확율이 높아집니다. 이런 경우, llama.cpp에서 `--ctx-size`크기를 조정 후 재시작 하세요.
또한, 같은 첨부파일이 있는지 확인해야 합니다. 그렇지 않는 경우 느린 결과 및 많은 메모리를 소모 합니다. 
전체적으로 **토큰 초과** 가 되는 경우 다음처럼 진행 합니다.

1. 첨부 파일 수/크기를 줄이거나
2. `LLM_FILE_MAX_BYTES / LLM_FILE_MAX_CHARS`를 낮추거나
3. 서버 ctx-size를 늘려야 합니다.

### RAG 토글

작은 기능의 사용자 RAG 기능을 제공 합니다. 아래와 같이 기능을 **켜기/끄기** 가 가능 합니다.
```text
:rag on
:rag off
```

### 파일을 RAG 인덱스에 적재

RAG에 반영할 파일이 있는 경우, 아래와 같이 파일을 명시 합니다.
```text
:rag add /var/log/messages
:rag stats
```

또는 `:file add` / `-f` 를 사용할 때 **RAG가 on이면 자동 인덱싱**됩니다.

### RAG 검색 테스트

특정 문자열 검색을 위해서 아래와 같이 수행이 가능 합니다.
```text
:rag query "certificate verify failed"
```

## 환경변수

### LLM

- `LLM_HOST` (default: `10.0.2.253`)
- `LLM_PORT` (default: `8080`)
- `LLM_BASE_URL` (예: `http://10.0.2.253:8080`)
- `LLM_MODEL` (default: `llama`)
- `LLM_TEMP` (default: `0.2`)
- `LLM_MAX_TOKENS` (default: `512`)
- `LLM_TIMEOUT` (default: `60`)
- `LLM_PROFILE` (`none|fast|deep`)
- `LLM_STREAM` (`0|1`)
- `LLM_SYSTEM_PROMPT`
- `LLM_CTX_TARGET` (목표 ctx-size)
- `LLM_CTX_OBSERVED` (관측 ctx-size 강제 지정)

### RAG

- `LLM_RAG` (`1|0`, default: `1`)
- `LLM_RAG_PATH` (default: `~/.kiki/rag.json`)
- `LLM_RAG_TOPK` (default: `3`)
- `LLM_RAG_CHUNK_CHARS` (default: `1200`)
- `LLM_RAG_OVERLAP` (default: `200`)
- `LLM_RAG_MAXCHARS` (default: `4000`)

### UI

- `KIKI_UI_HEADER` (`1|0`)
- `KIKI_UI_FIXED` (`1|0`)
- `KIKI_UI_HEADERLINES` (default: `5`)
- `KIKI_UI_CLEAR` (`1|0`)
- `KIKI_UI_MAXFILES` (default: `120`)

### History

- `LLM_HISTORY` (`1|0`, default: `1`)
- `LLM_HISTORY_PATH` (default: `~/.kiki/history.jsonl`)
- `LLM_HISTORY_PREVIEW` (default: `800`)
- `LLM_CAPTURE_FULL` (`1|0`)
- `LLM_CAPTURE_MAX` (default: `2000000`)
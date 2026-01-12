# kiki-ai-shell (Step 4)

4단계: **히스토리 조회/검색 기능**을 추가했습니다. (Step3의 file -> RAG 포함)

## Build
```bash
go build -o kiki-ai-shell ./cmd/kiki-ai-shell
```

## Run
```bash
./kiki-ai-shell
```

## file -> RAG 사용
```text
:rag on
:rag add /var/log/messages
?이 로그에서 가장 의심되는 에러가 뭐야?
```

또는 RAG가 켜져있으면 `:file add`가 자동으로 RAG 인덱싱도 합니다.
```text
:rag on
:file add /var/log/messages
?요약해줘
```

## 히스토리 사용
히스토리는 기본으로 `~/.kiki/history.jsonl`에 쌓입니다.

- 최근 N개 보기
```text
:history show 20
```

- 정규식 검색
```text
:history search timeout 50
:history search "HTTP 5.." 30
```

- 히스토리 파일 경로
```text
:history path
```

## timeout 조절
`context deadline exceeded`가 뜨면 타임아웃을 늘려주세요.
```text
:timeout 180
```

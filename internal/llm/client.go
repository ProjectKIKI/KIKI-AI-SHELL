package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func makeHTTPClient(timeoutSec int) *http.Client {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if timeoutSec <= 0 {
		return &http.Client{Transport: tr}
	}
	return &http.Client{Timeout: time.Duration(timeoutSec) * time.Second, Transport: tr}
}

func DoNonStream(ctx context.Context, endpoint string, timeoutSec int, reqPayload ChatRequest) (string, error) {
	client := makeHTTPClient(timeoutSec)
	body, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 400 {
		var ew APIErrorWrapper
		if json.Unmarshal(raw, &ew) == nil && ew.Error.Message != "" {
			return "", fmt.Errorf("API Error: %s", ew.Error.Message)
		}
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var cr ChatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", fmt.Errorf("응답 파싱 실패: %w", err)
	}
	if len(cr.Choices) == 0 {
		return "", errors.New("choices가 비어있음")
	}
	content := strings.TrimSpace(cr.Choices[0].Message.Content)
	if content == "" {
		content = strings.TrimSpace(cr.Choices[0].Text)
	}
	return content, nil
}

func DoStream(ctx context.Context, endpoint string, reqPayload ChatRequest, capLimit int, onText func(string)) (string, error) {
	client := makeHTTPClient(0)
	body, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		var ew APIErrorWrapper
		if json.Unmarshal(raw, &ew) == nil && ew.Error.Message != "" {
			return "", fmt.Errorf("API Error: %s", ew.Error.Message)
		}
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}

	reader := bufio.NewReader(resp.Body)
	var captured strings.Builder

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return captured.String(), nil
			}
			return captured.String(), err
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "event:") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			onText("\n")
			return captured.String(), nil
		}

		var chunk StreamChunk
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		text := chunk.Choices[0].Delta.Content
		if text == "" {
			text = chunk.Choices[0].Message.Content
		}
		if text == "" {
			continue
		}

		onText(text)
		if capLimit > 0 && captured.Len() < capLimit {
			remain := capLimit - captured.Len()
			if len(text) > remain {
				captured.WriteString(text[:remain])
			} else {
				captured.WriteString(text)
			}
		}
	}
}

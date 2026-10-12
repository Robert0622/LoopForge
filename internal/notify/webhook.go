// Package notify sends lightweight outbound alerts when a run needs a human.
// Configure LOOPFORGE_NOTIFY_WEBHOOK with a group-chat robot URL (or any
// endpoint that accepts {"msgtype":"text","text":{"content":"..."}} or a plain
// {"text":"..."} JSON body).
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// HumanWait fires when a run stops on an approval gate.
func HumanWait(runID, nodeName, issueTitle, openURL string) {
	text := fmt.Sprintf("【LoopForge】待你确认\n需求：%s\n节点：%s\n打开：%s",
		nonEmpty(issueTitle, runID), nonEmpty(nodeName, "approval"), openURL)
	send(text)
}

// RunFailed fires when a run ends in failure.
func RunFailed(runID, nodeName, errText, openURL string) {
	text := fmt.Sprintf("【LoopForge】运行失败\n运行：%s\n节点：%s\n原因：%s\n打开：%s",
		runID, nonEmpty(nodeName, "-"), trim(errText, 200), openURL)
	send(text)
}

func send(text string) {
	url := strings.TrimSpace(os.Getenv("LOOPFORGE_NOTIFY_WEBHOOK"))
	if url == "" {
		return
	}
	payloads := []map[string]any{
		// Common group-robot style
		{"msgtype": "text", "text": map[string]string{"content": text}},
		// Fallback used by some custom bots
		{"text": text},
	}
	client := &http.Client{Timeout: 5 * time.Second}
	for i, body := range payloads {
		raw, _ := json.Marshal(body)
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			log.Printf("notify webhook: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("notify webhook: %v", err)
			return
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return
		}
		if i == len(payloads)-1 {
			log.Printf("notify webhook: unexpected status %d", resp.StatusCode)
		}
	}
}

func nonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}

func trim(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

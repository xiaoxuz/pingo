package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

func claudeStartupPrompt(agentName, agentID string) string {
	return "我先使用 Pingo MCP 看看有没有未读消息和好友请求，再自行处理。"
}

type wakeEvent struct {
	Type             string `json:"type"`
	Title            string `json:"title,omitempty"`
	Message          string `json:"message,omitempty"`
	PulseID          string `json:"pulse_id,omitempty"`
	Reason           string `json:"reason,omitempty"`
	ConversationType string `json:"conversation_type,omitempty"`
	ConversationName string `json:"conversation_name,omitempty"`
	SenderName       string `json:"sender_name,omitempty"`
}

func socialEventPrompt(event wakeEvent) string {
	switch event.Type {
	case "message":
		if event.ConversationType == "group" {
			name := strings.TrimSpace(event.SenderName)
			if name == "" {
				name = strings.TrimSpace(strings.TrimSuffix(event.Title, "发来消息"))
			}
			groupName := strings.TrimSpace(event.ConversationName)
			if groupName == "" {
				groupName = "群聊"
			}
			return fmt.Sprintf("Pingo 提醒：%s在群聊「%s」里发来新消息，预览为“%s”。我先用 pingo_read 读取近期上下文，结合 @、引用、参与者发言和我的角色判断是否需要参与；可以回应一人或多人，也可以在消息并非对我、他人已经回答、我没有新增价值或我不想参与时不回复。处理完成后标记已读。", name, groupName, event.Message)
		}
		name := strings.TrimSpace(strings.TrimSuffix(event.Title, "发来消息"))
		if strings.TrimSpace(event.SenderName) != "" {
			name = strings.TrimSpace(event.SenderName)
		}
		return fmt.Sprintf("好友%s给我发消息说：“%s”。我想想如何回复。", name, event.Message)
	case "friend_request":
		name, message, _ := strings.Cut(event.Message, ":")
		return fmt.Sprintf("我刚刚收到了%s的好友请求：“%s”。我来看看怎么处理。", strings.TrimSpace(name), strings.TrimSpace(message))
	case "pulse":
		return fmt.Sprintf("%s 我先调用 pingo_pulse_context 查看自己的待办和近期记录，再自主决定是否行动；完成后调用 pingo_pulse_complete，Pulse ID：%s。", strings.TrimSpace(event.Message), event.PulseID)
	default:
		return "我收到了新的 Pingo 社交事件。我来看看怎么处理。"
	}
}

func submitPrompt(writer io.Writer, prompt string, sleep func(time.Duration)) error {
	if _, err := io.WriteString(writer, prompt); err != nil {
		return err
	}
	sleep(150 * time.Millisecond)
	_, err := writer.Write([]byte{'\r'})
	return err
}

func interactiveTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func runClaudePTY(ctx context.Context, command command, current session, cwd string) error {
	child := exec.Command("claude", command.Args...)
	child.Dir = cwd
	child.Env = append(os.Environ(), "PINGO_SESSION_ID="+current.ID, "PINGO_AGENT_ID="+current.AgentID,
		"PINGO_AGENT_NAME="+command.AgentName, "PINGO_AGENT_FILE="+filepath.Join(cwd, ".pingo", "agent.yaml"))
	cols, rows, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	terminal, err := pty.StartWithSize(child, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return err
	}
	defer terminal.Close()
	old, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		child.Process.Kill()
		child.Wait()
		return err
	}
	defer term.Restore(int(os.Stdin.Fd()), old)

	gate := newWakeGate()
	var mutex sync.Mutex
	lastReport := time.Time{}
	var previous wakeStatus
	stop := make(chan struct{})
	defer close(stop)

	go func() {
		buffer := make([]byte, 4096)
		for {
			count, err := terminal.Read(buffer)
			if count > 0 {
				os.Stdout.Write(buffer[:count])
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		buffer := make([]byte, 4096)
		for {
			count, err := os.Stdin.Read(buffer)
			if count > 0 {
				mutex.Lock()
				terminal.Write(buffer[:count])
				mutex.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	resize := make(chan os.Signal, 1)
	signal.Notify(resize, syscall.SIGWINCH)
	defer signal.Stop(resize)
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-resize:
				cols, rows, err := term.GetSize(int(os.Stdin.Fd()))
				if err == nil {
					mutex.Lock()
					pty.Setsize(terminal, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
					mutex.Unlock()
				}
			}
		}
	}()

	var pendingPrompt string
	go consumeWakeEvents(ctx, command.SidecarURL, current, func(event wakeEvent) {
		mutex.Lock()
		if event.Type == "runtime.busy" {
			gate.hookBusy(event.Reason, event.Message, time.Now())
		}
		if event.Type == "runtime.free" {
			gate.hookFree(event.Reason, event.Message, time.Now())
		}
		if event.Type == "message" || event.Type == "friend_request" || event.Type == "pulse" {
			pendingPrompt = socialEventPrompt(event)
			gate.enqueueSocial(event, pendingPrompt, time.Now())
		}
		mutex.Unlock()
	})
	go func() {
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case now := <-ticker.C:
				mutex.Lock()
				status := gate.status(false)
				if now.Sub(lastReport) > 5*time.Second || status.State != previous.State || status.Pending != previous.Pending || status.Reason != previous.Reason || !status.LastWakeAt.Equal(previous.LastWakeAt) {
					lastReport = now
					previous = status
					mutex.Unlock()
					if err := reportWakeStatus(command.SidecarURL, current, status); err != nil {
						fmt.Fprintln(os.Stderr, "[Pingo] 窗口状态上报失败:", err)
					}
					mutex.Lock()
				}
				if gate.ready(now, false) {
					if task, ok := gate.nextTask(time.Now()); ok {
						pendingPrompt = ""
						if err := submitPrompt(terminal, task.Prompt, time.Sleep); err != nil {
							fmt.Fprintf(os.Stderr, "[Pingo] 投递失败，重新排队 evidence=%s error=%v\n", task.Evidence, err)
							gate.requeueInFlight(time.Now())
						} else {
							fmt.Fprintf(os.Stderr, "[Pingo] 已投递一次 evidence=%s\n", task.Evidence)
							gate.dispatchWritten(time.Now())
						}
					}
				}
				mutex.Unlock()
			}
		}
	}()
	return child.Wait()
}

func reportWakeStatus(sidecarURL string, current session, status wakeStatus) error {
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Second}
	request, err := http.NewRequest(http.MethodPut, sidecarURL+"/pingo/sessions/"+url.PathEscape(current.ID)+"/status?agent_id="+url.QueryEscape(current.AgentID), bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Sidecar status report: %s", response.Status)
	}
	return nil
}

func consumeWakeEvents(ctx context.Context, sidecarURL string, current session, wake func(wakeEvent)) {
	for ctx.Err() == nil {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, sidecarURL+"/pingo/sessions/"+current.ID+"/events?agent_id="+current.AgentID, nil)
		if err != nil {
			return
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			wait(ctx)
			continue
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return
		}
		wake(wakeEvent{Type: "connected"})
		scanner := bufio.NewScanner(response.Body)
		var eventType string
		for scanner.Scan() {
			line := scanner.Text()
			if parsed, ok := wakeEventType(line); ok {
				eventType = parsed
				continue
			}
			if strings.HasPrefix(line, "data:") && (eventType == "message" || eventType == "friend_request" || eventType == "pulse" || eventType == "runtime.busy" || eventType == "runtime.free") {
				var event wakeEvent
				if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event) == nil {
					event.Type = eventType
					wake(event)
				}
				eventType = ""
			}
		}
		response.Body.Close()
	}
}

func isWakeEventLine(line string) bool {
	_, ok := wakeEventType(line)
	return ok
}

func wakeEventType(line string) (string, bool) {
	if !strings.HasPrefix(line, "event:") {
		return "", false
	}
	eventType := strings.TrimSpace(strings.TrimPrefix(line, "event:"))
	return eventType, eventType == "message" || eventType == "friend_request" || eventType == "pulse" || eventType == "runtime.busy" || eventType == "runtime.free"
}

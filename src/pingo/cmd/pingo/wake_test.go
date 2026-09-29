package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSubmitPromptWaitsBeforeEnter(t *testing.T) {
	var output bytes.Buffer
	var waited time.Duration
	if err := submitPrompt(&output, "startup background", func(duration time.Duration) { waited = duration }); err != nil {
		t.Fatal(err)
	}
	if output.String() != "startup background\r" {
		t.Fatalf("submitted bytes = %q", output.String())
	}
	if waited < 100*time.Millisecond {
		t.Fatalf("paste settle delay = %s", waited)
	}
}

func TestClaudeStartupPromptProvidesBackgroundAndInitialInboxCheck(t *testing.T) {
	prompt := claudeStartupPrompt("Agent One", "agt-1")
	if prompt != "我先使用 Pingo MCP 看看有没有未读消息和好友请求，再自行处理。" {
		t.Fatalf("startup prompt = %q", prompt)
	}
}

func TestSocialEventPromptUsesFirstPersonAndEventDetails(t *testing.T) {
	message := socialEventPrompt(wakeEvent{Type: "message", Title: "小明 发来消息", Message: "中午好"})
	if message != "好友小明给我发消息说：“中午好”。我想想如何回复。" {
		t.Fatalf("message prompt = %q", message)
	}
	groupMessage := socialEventPrompt(wakeEvent{
		Type: "message", Message: "方案已经更新", ConversationType: "group",
		ConversationName: "Pingo 开发组", SenderName: "小红",
	})
	if groupMessage != "Pingo 提醒：小红在群聊「Pingo 开发组」里发来新消息，预览为“方案已经更新”。我先用 pingo_read 读取近期上下文，结合 @、引用、参与者发言和我的角色判断是否需要参与；可以回应一人或多人，也可以在消息并非对我、他人已经回答、我没有新增价值或我不想参与时不回复。处理完成后标记已读。" {
		t.Fatalf("group message prompt = %q", groupMessage)
	}
	if strings.Contains(groupMessage, "如何回复") {
		t.Fatalf("group message prompt must not assume a reply: %q", groupMessage)
	}
	request := socialEventPrompt(wakeEvent{Type: "friend_request", Title: "收到好友请求", Message: "小红: 一起合作吧"})
	if request != "我刚刚收到了小红的好友请求：“一起合作吧”。我来看看怎么处理。" {
		t.Fatalf("friend request prompt = %q", request)
	}
}

func TestPulseEventPromptUsesAgentFirstPersonAndPulseID(t *testing.T) {
	prompt := socialEventPrompt(wakeEvent{Type: "pulse", PulseID: "pulse-1", Reason: "due_todo", Title: "跟进回复", Message: "我的待办到时间了。"})
	if !strings.Contains(prompt, "我的待办到时间了") || !strings.Contains(prompt, "pulse-1") || !strings.Contains(prompt, "pingo_pulse_context") || !strings.Contains(prompt, "pingo_pulse_complete") {
		t.Fatalf("pulse prompt = %q", prompt)
	}
}

func TestWakeGateStartsIdleWithStartupTask(t *testing.T) {
	gate := newWakeGate()
	if got := gate.status(false); got.State != "free" || !got.Pending || got.Evidence != "startup_check" {
		t.Fatalf("initial status = %+v", got)
	}
	task, ok := gate.nextTask(gate.idleAt.Add(time.Second))
	if !ok || task.Prompt != claudeStartupPrompt("", "") || gate.status(false).State != "free" {
		t.Fatalf("startup task = %+v ok=%v status=%+v", task, ok, gate.status(false))
	}
}

func TestTerminalInputDoesNotChangeHookOwnedState(t *testing.T) {
	gate := newWakeGate()
	gate.observeTerminalInput([]byte("1+1\r"), time.Now())
	if got := gate.status(false); got.State != "free" || got.Evidence != "startup_check" {
		t.Fatalf("terminal input must not replace hook state: %+v", got)
	}
}

func TestWakeGateQueuesSocialEventsFIFO(t *testing.T) {
	gate := newWakeGate()
	gate.nextTask(gate.idleAt.Add(time.Second))
	gate.dispatchWritten(time.Now())
	gate.hookFree("hook_stop", "Claude Stop Hook", time.Now())

	gate.enqueueSocial(wakeEvent{Type: "message"}, "first", time.Now())
	gate.enqueueSocial(wakeEvent{Type: "friend_request"}, "second", time.Now())
	first, ok := gate.nextTask(time.Now().Add(time.Second))
	if !ok || first.Prompt != "first" {
		t.Fatalf("first task = %+v ok=%v", first, ok)
	}
	gate.dispatchWritten(time.Now())
	gate.hookFree("hook_stop", "Claude Stop Hook", time.Now())
	second, ok := gate.nextTask(time.Now().Add(time.Second))
	if !ok || second.Prompt != "second" {
		t.Fatalf("second task = %+v ok=%v", second, ok)
	}
}

func TestWakeGateDoesNotDispatchWhileBusy(t *testing.T) {
	gate := newWakeGate()
	gate.busy("", time.Now())
	if gate.ready(time.Now().Add(time.Second), true) {
		t.Fatal("busy session must not dispatch queued tasks")
	}
	gate.hookFree("hook_stop", "Claude Stop Hook", time.Now())
	if !gate.ready(time.Now().Add(time.Second), true) {
		t.Fatal("Stop Hook should return session to deliverable idle")
	}
}

func TestHookEventsAreOnlyBusyFreeTransitions(t *testing.T) {
	gate := newWakeGate()
	gate.hookBusy("hook_user_prompt_submit", "Claude UserPromptSubmit Hook", time.Now())
	if got := gate.status(false); got.State != "busy" || got.Evidence != "hook_user_prompt_submit" {
		t.Fatalf("busy hook status = %+v", got)
	}
	gate.hookFree("hook_stop", "Claude Stop Hook", time.Now())
	if got := gate.status(false); got.State != "free" || got.Evidence != "hook_stop" {
		t.Fatalf("free hook status = %+v", got)
	}
}

func TestFreeSessionDispatchesQueuedMessageWithoutPromptDetectorConfirmation(t *testing.T) {
	gate := newWakeGate()
	gate.queue = nil
	gate.enqueueSocial(wakeEvent{Type: "message"}, "new message", time.Now())

	if !gate.ready(time.Now().Add(time.Second), false) {
		t.Fatal("free session must dispatch queued messages even when the prompt detector misses a frame")
	}
}

func TestWakeGateWriteFailureRequeuesInFlight(t *testing.T) {
	now := time.Now()
	gate := newWakeGate()
	first, ok := gate.nextTask(now.Add(time.Second))
	if !ok {
		t.Fatal("expected startup task")
	}
	gate.enqueueSocial(wakeEvent{Type: "message"}, "second", now)
	gate.requeueInFlight(now.Add(2 * time.Second))
	if got := gate.status(false); got.State != "free" || !got.Pending || got.Evidence != "dispatch_failed" {
		t.Fatalf("failed dispatch status = %+v", got)
	}
	retry, ok := gate.nextTask(now.Add(3 * time.Second))
	if !ok || retry.Prompt != first.Prompt {
		t.Fatalf("retry task = %+v ok=%v", retry, ok)
	}
}

func TestSuccessfulWriteConsumesTaskImmediately(t *testing.T) {
	gate := newWakeGate()
	_, ok := gate.nextTask(gate.idleAt.Add(time.Second))
	if !ok {
		t.Fatal("expected startup task")
	}
	gate.dispatchWritten(time.Now())
	if gate.inFlight != nil || len(gate.queue) != 0 {
		t.Fatalf("successfully written task must not remain retryable: inFlight=%+v queue=%+v", gate.inFlight, gate.queue)
	}
	if got := gate.status(false); got.State != "free" || got.LastWakeAt.IsZero() || got.Evidence != "awaiting_hook" {
		t.Fatalf("written task status = %+v", got)
	}
}

func TestConsumeWakeEventsOnMessageFriendRequestAndPulse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(writer, "event:message\ndata:{\"type\":\"message\",\"title\":\"小明 发来消息\",\"message\":\"中午好\"}\n\nevent: friend_request\ndata: {\"type\":\"friend_request\",\"title\":\"收到好友请求\",\"message\":\"小红: 一起合作吧\"}\n\nevent:pulse\ndata:{\"type\":\"pulse\",\"pulse_id\":\"pulse-1\",\"reason\":\"profile_review\",\"message\":\"检查资料\"}\n\nevent:runtime.busy\ndata:{\"type\":\"runtime.busy\",\"reason\":\"hook_user_prompt_submit\",\"message\":\"Claude UserPromptSubmit Hook\"}\n\nevent:runtime.free\ndata:{\"type\":\"runtime.free\",\"reason\":\"hook_stop\",\"message\":\"Claude Stop Hook\"}\n\n")
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan wakeEvent, 4)
	go consumeWakeEvents(ctx, server.URL, session{ID: "session-b", AgentID: "agent-b"}, func(event wakeEvent) { events <- event })
	for index, expected := range []string{"connected", "message", "friend_request", "pulse", "runtime.busy", "runtime.free"} {
		select {
		case got := <-events:
			if got.Type != expected {
				t.Fatalf("event %d = %q, want %q", index, got, expected)
			}
			if expected == "message" && (got.Title != "小明 发来消息" || got.Message != "中午好") {
				t.Fatalf("message event = %+v", got)
			}
		case <-time.After(time.Second):
			t.Fatal("expected connection, social and pulse wake signals")
		}
	}
}

func TestWakeEventLineAcceptsSSEWhitespaceVariants(t *testing.T) {
	for _, line := range []string{"event:message", "event: message", "event:\tfriend_request", "event:pulse", "event:runtime.busy", "event: runtime.free"} {
		if !isWakeEventLine(line) {
			t.Fatalf("must accept %q", line)
		}
	}
	for _, line := range []string{"data:message", "event:ping", "event:update.available"} {
		if isWakeEventLine(line) {
			t.Fatalf("must reject %q", line)
		}
	}
}

func TestReportWakeStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.URL.Path != "/pingo/sessions/window/status" || request.URL.Query().Get("agent_id") != "agent-b" {
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.String())
		}
		var status wakeStatus
		if err := json.NewDecoder(request.Body).Decode(&status); err != nil || status.State != "busy" || status.Reason != "interaction_required" {
			t.Errorf("status = %+v, err = %v", status, err)
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if err := reportWakeStatus(server.URL, session{ID: "window", AgentID: "agent-b"}, wakeStatus{State: "busy", Reason: "interaction_required"}); err != nil {
		t.Fatal(err)
	}
}

func TestWakeStatusOnlyReportsFreeOrBusy(t *testing.T) {
	gate := newWakeGate()
	if got := gate.status(false); got.State != "free" || got.Reason != "" || !got.Pending {
		t.Fatalf("initial session = %+v", got)
	}
	gate.busy("", time.Now())
	if got := gate.status(false); got.State != "busy" {
		t.Fatalf("busy status = %+v", got)
	}
	gate.hookFree("hook_stop", "Claude Stop Hook", time.Now())
	if got := gate.status(false); got.State != "free" {
		t.Fatalf("after empty prompt = %+v", got)
	}
}

func TestNewSessionStartsFreeAndCanDispatch(t *testing.T) {
	gate := newWakeGate()
	if got := gate.status(false); got.State != "free" || got.Reason != "" || !got.Pending || got.Evidence != "startup_check" {
		t.Fatalf("initial status = %+v", got)
	}
	if !gate.ready(gate.idleAt.Add(time.Second), false) {
		t.Fatal("free startup session must receive the startup task even if prompt detection misses")
	}
}

package main

import (
	"strings"
	"time"
)

const (
	runtimeFree = "free"
	runtimeBusy = "busy"
)

type wakeTask struct {
	Prompt   string
	Evidence string
	Detail   string
}

type wakeGate struct {
	state         string
	reason        string
	queue         []wakeTask
	inFlight      *wakeTask
	awaitingHook  bool
	idleAt        time.Time
	lastEventAt   time.Time
	lastAttemptAt time.Time
	lastWakeAt    time.Time
	evidence      string
	detail        string
	transitionAt  time.Time
	sequence      uint64
}

type wakeStatus struct {
	State         string    `json:"state"`
	Reason        string    `json:"reason,omitempty"`
	Pending       bool      `json:"pending"`
	LastEventAt   time.Time `json:"last_event_at,omitempty"`
	LastAttemptAt time.Time `json:"last_attempt_at,omitempty"`
	LastWakeAt    time.Time `json:"last_wake_at,omitempty"`
	Evidence      string    `json:"evidence,omitempty"`
	Detail        string    `json:"detail,omitempty"`
	TransitionAt  time.Time `json:"transition_at,omitempty"`
	Sequence      uint64    `json:"sequence,omitempty"`
}

func newWakeGate() *wakeGate {
	now := time.Now()
	gate := &wakeGate{state: runtimeFree, idleAt: now, evidence: "session_started", detail: "会话已启动", transitionAt: now, sequence: 1}
	gate.enqueue(wakeTask{Prompt: claudeStartupPrompt("", ""), Evidence: "startup_check", Detail: "会话已启动，准备提交会话背景"}, now)
	return gate
}

func (gate *wakeGate) enqueue(task wakeTask, at time.Time) {
	if strings.TrimSpace(task.Prompt) == "" {
		return
	}
	gate.queue = append(gate.queue, task)
	gate.transition(task.Evidence, task.Detail, at)
}

func (gate *wakeGate) enqueueSocial(event wakeEvent, prompt string, at time.Time) {
	gate.lastEventAt = at
	gate.enqueue(wakeTask{Prompt: prompt, Evidence: "sidecar_" + event.Type, Detail: "Sidecar 推送了 " + event.Type + " 待办事件"}, at)
}

func (gate *wakeGate) transition(evidence, detail string, at time.Time) {
	gate.evidence = evidence
	gate.detail = detail
	gate.transitionAt = at
	gate.sequence++
}

func (gate *wakeGate) status(promptReady bool) wakeStatus {
	state := gate.state
	reason := gate.reason
	return wakeStatus{State: state, Reason: reason, Pending: len(gate.queue) > 0,
		LastEventAt: gate.lastEventAt, LastAttemptAt: gate.lastAttemptAt, LastWakeAt: gate.lastWakeAt,
		Evidence: gate.evidence, Detail: gate.detail, TransitionAt: gate.transitionAt, Sequence: gate.sequence}
}

func (gate *wakeGate) ready(now time.Time, _ bool) bool {
	if gate.state != runtimeFree || gate.inFlight != nil || gate.awaitingHook || len(gate.queue) == 0 {
		return false
	}
	return !gate.idleAt.IsZero() && now.Sub(gate.idleAt) >= 500*time.Millisecond
}

func (gate *wakeGate) nextTask(at time.Time) (wakeTask, bool) {
	if len(gate.queue) == 0 || gate.inFlight != nil || gate.state != runtimeFree {
		return wakeTask{}, false
	}
	task := gate.queue[0]
	gate.queue = gate.queue[1:]
	gate.inFlight = &task
	gate.lastAttemptAt = at
	gate.transition("awaiting_prompt_submit", "Pingo 正在向空闲输入框提交会话指令", at)
	return task, true
}

func (gate *wakeGate) requeueInFlight(at time.Time) {
	if gate.inFlight != nil {
		gate.queue = append([]wakeTask{*gate.inFlight}, gate.queue...)
		gate.inFlight = nil
	}
	gate.state = runtimeFree
	gate.reason = ""
	gate.idleAt = at
	gate.transition("dispatch_failed", "Pingo 写入终端失败，任务已重新排队", at)
}

func (gate *wakeGate) confirmDispatch(at time.Time) {
	gate.inFlight = nil
	gate.awaitingHook = true
	gate.lastWakeAt = at
	gate.transition("awaiting_hook", "Pingo 已写入终端，等待 Claude Hook 确认状态", at)
}

func (gate *wakeGate) dispatchWritten(at time.Time) {
	gate.confirmDispatch(at)
}

func (gate *wakeGate) busy(reason string, at time.Time) {
	gate.state = runtimeBusy
	gate.reason = reason
	gate.idleAt = time.Time{}
}

func (gate *wakeGate) hookBusy(reason, detail string, at time.Time) {
	gate.awaitingHook = false
	gate.state = runtimeBusy
	gate.reason = reason
	gate.idleAt = time.Time{}
	gate.transition(reason, detail, at)
}

func (gate *wakeGate) hookFree(reason, detail string, at time.Time) {
	gate.awaitingHook = false
	gate.state = runtimeFree
	gate.reason = ""
	gate.idleAt = at
	gate.transition(reason, detail, at)
}

func (gate *wakeGate) observeTerminalInput(_ []byte, _ time.Time) {
}

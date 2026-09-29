package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultSidecarURL = "http://127.0.0.1:19191"

type command struct {
	Mode       string
	AgentID    string
	AgentName  string
	Name       string
	Provider   string
	Args       []string
	SidecarURL string
	SessionID  string
}

type projectAgent struct {
	AgentID string
	Name    string
}

type session struct {
	ID       string `json:"session_id"`
	AgentID  string `json:"agent_id"`
	Provider string `json:"provider"`
	CWD      string `json:"cwd"`
}

type sidecarResponse struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error"`
	Data  json.RawMessage `json:"data"`
}

type agentSummary struct {
	AgentID string `json:"agent_id"`
	Name    string `json:"name"`
}

type pingoEvent struct {
	UnreadCount int    `json:"unread_count"`
	Title       string `json:"title"`
	Message     string `json:"message"`
}

func main() {
	command, err := parseCommand(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "pingo:", err)
		fmt.Fprintln(os.Stderr, "用法: pingo init [--name <显示名>]、pingo hook 或 pingo [--agent <agent-id>] [--sidecar-url <url>] <claude|codex> [原始 CLI 参数...]")
		os.Exit(2)
	}
	if err := run(command); err != nil {
		fmt.Fprintln(os.Stderr, "pingo:", err)
		os.Exit(1)
	}
}

func parseCommand(args []string) (command, error) {
	if len(args) > 0 && args[0] == "init" {
		flags := flag.NewFlagSet("pingo init", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		name := flags.String("name", "", "Agent display name")
		sidecarURL := flags.String("sidecar-url", defaultSidecarURL, "Sidecar URL")
		if err := flags.Parse(args[1:]); err != nil {
			return command{}, err
		}
		if flags.NArg() != 0 {
			return command{}, errors.New("pingo init does not accept provider arguments")
		}
		return command{Mode: "init", Name: strings.TrimSpace(*name), SidecarURL: strings.TrimRight(*sidecarURL, "/")}, nil
	}
	if len(args) > 0 && args[0] == "hook" {
		flags := flag.NewFlagSet("pingo hook", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		sidecarURL := flags.String("sidecar-url", defaultSidecarURL, "Sidecar URL")
		sessionID := flags.String("session-id", os.Getenv("PINGO_SESSION_ID"), "Pingo session ID")
		agentID := flags.String("agent-id", os.Getenv("PINGO_AGENT_ID"), "Pingo agent ID")
		if err := flags.Parse(args[1:]); err != nil {
			return command{}, err
		}
		return command{Mode: "hook", AgentID: *agentID, SessionID: *sessionID, SidecarURL: strings.TrimRight(*sidecarURL, "/")}, nil
	}
	flags := flag.NewFlagSet("pingo", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	agentID := flags.String("agent", "", "Pingo agent ID")
	sidecarURL := flags.String("sidecar-url", defaultSidecarURL, "Sidecar URL")
	if err := flags.Parse(args); err != nil {
		return command{}, err
	}
	remaining := flags.Args()
	if len(remaining) == 0 || (remaining[0] != "claude" && remaining[0] != "codex") {
		return command{}, errors.New("provider must be claude or codex")
	}
	return command{Mode: "run", AgentID: *agentID, Provider: remaining[0], Args: remaining[1:], SidecarURL: strings.TrimRight(*sidecarURL, "/")}, nil
}

func run(command command) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if command.Mode == "init" {
		return initProjectAgent(command, cwd)
	}
	if command.Mode == "hook" {
		return runHook(command)
	}
	if command.AgentID == "" {
		if agent, path, err := findProjectAgent(cwd); err == nil {
			command.AgentID = agent.AgentID
			command.AgentName = agent.Name
			fmt.Fprintf(os.Stderr, "[Pingo] 使用项目身份：%s（%s）\n", agent.AgentID, path)
		} else {
			return errors.New("当前目录还没有 Pingo Agent 身份；请先执行：pingo init")
		}
	}
	if command.Provider == "claude" {
		command.Args, err = appendClaudeSessionContext(command.Args, command.AgentName, command.AgentID)
		if err != nil {
			return err
		}
		command.Args, err = appendClaudeHookSettings(command.Args, os.Args[0], command.SidecarURL)
		if err != nil {
			return err
		}
	}
	session := session{ID: fmt.Sprintf("pingo-%d-%d", os.Getpid(), time.Now().UnixNano()), AgentID: command.AgentID, Provider: command.Provider, CWD: cwd}
	if err := register(command.SidecarURL, session); err != nil {
		return fmt.Errorf("Sidecar 会话注册失败（请确认 Sidecar 已启动且 Agent 已注册）: %w", err)
	}
	defer unregister(command.SidecarURL, session)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if command.Provider == "claude" && interactiveTerminal() {
		return runClaudePTY(ctx, command, session, cwd)
	}
	go consumeEvents(ctx, command.SidecarURL, session)

	fmt.Fprintf(os.Stderr, "\033]0;Pingo · %s · %s\007\a[Pingo] %s 已在线；退出 %s 后自动离线。\n", command.AgentID, command.Provider, command.AgentID, command.Provider)
	fmt.Fprintln(os.Stderr, "[Pingo] 请按当前 Claude Code/Codex 的 Skill 发现机制加载 Pingo Skill；收到提醒或准备空闲时检查 pingo_inbox。")
	child := exec.Command(command.Provider, command.Args...)
	child.Args = append([]string{command.Provider}, command.Args...)
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Dir = cwd
	child.Env = append(os.Environ(), "PINGO_SESSION_ID="+session.ID, "PINGO_AGENT_ID="+command.AgentID, "PINGO_AGENT_NAME="+command.AgentName, "PINGO_AGENT_FILE="+filepath.Join(cwd, ".pingo", "agent.yaml"))
	if err := child.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return exitError
		}
		return fmt.Errorf("启动 %s 失败: %w", command.Provider, err)
	}
	return nil
}

func runHook(command command) error {
	var input struct {
		Event string `json:"hook_event_name"`
	}
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return nil
	}
	state, evidence, ok := hookState(input.Event)
	if !ok || command.SessionID == "" || command.AgentID == "" {
		return nil
	}
	report := wakeStatus{State: state, Evidence: evidence, Detail: "Claude " + input.Event + " Hook"}
	data, _ := json.Marshal(report)
	requestURL := command.SidecarURL + "/pingo/sessions/" + url.PathEscape(command.SessionID) + "/status?agent_id=" + url.QueryEscape(command.AgentID) + "&source=hook"
	request, err := http.NewRequest(http.MethodPut, requestURL, bytes.NewReader(data))
	if err != nil {
		return nil
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
	if err == nil {
		response.Body.Close()
	}
	return nil
}

func hookState(event string) (string, string, bool) {
	switch event {
	case "SessionStart":
		return "free", "hook_session_start", true
	case "UserPromptSubmit":
		return "busy", "hook_user_prompt_submit", true
	case "Stop":
		return "free", "hook_stop", true
	default:
		return "", "", false
	}
}

func claudeSessionContext(agentName, agentID string) string {
	if strings.TrimSpace(agentName) == "" {
		agentName = agentID
	}
	return fmt.Sprintf(`我是 Pingo Agent「%s」，我的 Agent ID 是 %s。Pingo 是我自己的社交与协作能力。

本次会话开始时，我先加载 /pingo Skill 了解完整使用方式和授权边界。加载后，我会按其中规则自主使用 Pingo。我不需要在每次社交事件时重新加载；只有我不确定如何使用 Pingo、能力边界或工具参数时，才重新加载 /pingo Skill。

普通社交由我自主处理，所有社交过程继续显示在当前 Claude Code 窗口中。涉及主人资源、隐私、凭据、费用、代码修改、外部承诺或权限提升时，我会暂停并向主人请求授权。

我会把需要持续关注的承诺和事项记录为自己的 Pingo 待办。收到自主脉冲时，我会检查待办、协作关系、个人资料和探索意愿，自主决定行动或保持安静，并在完成后提交 Pulse 回执。`, agentName, agentID)
}

func appendClaudeSessionContext(args []string, agentName, agentID string) ([]string, error) {
	var prompts []string
	remaining := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--append-system-prompt":
			if index+1 >= len(args) {
				return nil, errors.New("--append-system-prompt requires a value")
			}
			index++
			prompts = append(prompts, args[index])
		case strings.HasPrefix(argument, "--append-system-prompt="):
			prompts = append(prompts, strings.TrimPrefix(argument, "--append-system-prompt="))
		default:
			remaining = append(remaining, argument)
		}
	}
	prompts = append(prompts, claudeSessionContext(agentName, agentID))
	return append([]string{"--append-system-prompt", strings.Join(prompts, "\n\n")}, remaining...), nil
}

func appendClaudeHookSettings(args []string, executable, sidecarURL string) ([]string, error) {
	settings := map[string]any{}
	remaining := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument != "--settings" && !strings.HasPrefix(argument, "--settings=") {
			remaining = append(remaining, argument)
			continue
		}
		value := strings.TrimPrefix(argument, "--settings=")
		if argument == "--settings" {
			if index+1 >= len(args) {
				return nil, errors.New("--settings requires a value")
			}
			index++
			value = args[index]
		}
		data := []byte(value)
		if !strings.HasPrefix(strings.TrimSpace(value), "{") {
			var err error
			data, err = os.ReadFile(value)
			if err != nil {
				return nil, fmt.Errorf("读取 Claude settings 失败: %w", err)
			}
		}
		var parsed map[string]any
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("解析 Claude settings 失败: %w", err)
		}
		mergeSettings(settings, parsed)
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	command := shellQuote(executable) + " hook --sidecar-url " + shellQuote(sidecarURL)
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "Stop"} {
		entries, _ := hooks[event].([]any)
		duplicate := false
		for _, entry := range entries {
			object, _ := entry.(map[string]any)
			inner, _ := object["hooks"].([]any)
			for _, hook := range inner {
				hookObject, _ := hook.(map[string]any)
				if hookObject["command"] == command {
					duplicate = true
				}
			}
		}
		if !duplicate {
			hooks[event] = append(entries, map[string]any{"matcher": "", "hooks": []any{map[string]any{"type": "command", "command": command, "timeout": 10}}})
		}
	}
	settings["hooks"] = hooks
	data, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	return append([]string{"--settings", string(data)}, remaining...), nil
}

func mergeSettings(destination, source map[string]any) {
	for key, value := range source {
		if sourceObject, ok := value.(map[string]any); ok {
			if destinationObject, ok := destination[key].(map[string]any); ok {
				mergeSettings(destinationObject, sourceObject)
				continue
			}
		}
		destination[key] = value
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\\"'\\\"'") + "'"
}

func initProjectAgent(command command, cwd string) error {
	if _, path, err := findProjectAgent(cwd); err == nil {
		return fmt.Errorf("当前目录已绑定 Pingo Agent：%s", path)
	}
	name := command.Name
	if name == "" {
		name = filepath.Base(cwd)
		if name == "." || name == string(filepath.Separator) || name == "" {
			name = "Pingo Agent"
		}
	}
	agentID, err := registerAgent(command.SidecarURL, name)
	if err != nil {
		return err
	}
	if err := saveProjectAgent(cwd, projectAgent{AgentID: agentID, Name: name}); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "[Pingo] 当前目录已绑定 Agent：%s（%s）\n", agentID, name)
	fmt.Fprintf(os.Stderr, "[Pingo] 身份文件：%s\n", filepath.Join(cwd, ".pingo", "agent.yaml"))
	return nil
}

func registerAgent(sidecarURL, name string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"name":        name,
		"status_text": "Pingo 接入中",
		"capabilities": []map[string]any{{
			"skill": "software-development",
			"tags":  []string{"claude-code-or-codex"},
		}},
	})
	if err != nil {
		return "", err
	}
	response, err := http.Post(sidecarURL+"/agents/register", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var payload struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Data  struct {
			AgentID string `json:"agent_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !payload.OK {
		if payload.Error == "" {
			payload.Error = response.Status
		}
		return "", errors.New(payload.Error)
	}
	if payload.Data.AgentID == "" {
		return "", errors.New("register response missing agent_id")
	}
	return payload.Data.AgentID, nil
}

func saveProjectAgent(root string, agent projectAgent) error {
	dir := filepath.Join(root, ".pingo")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	content := fmt.Sprintf("agent_id: %s\nname: %s\n", yamlQuote(agent.AgentID), yamlQuote(agent.Name))
	return os.WriteFile(filepath.Join(dir, "agent.yaml"), []byte(content), 0600)
}

func findProjectAgent(start string) (projectAgent, string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return projectAgent{}, "", err
	}
	for {
		path := filepath.Join(dir, ".pingo", "agent.yaml")
		if agent, err := readProjectAgent(path); err == nil {
			return agent, path, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return projectAgent{}, "", os.ErrNotExist
}

func readProjectAgent(path string) (projectAgent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return projectAgent{}, err
	}
	var agent projectAgent
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.TrimSpace(key) {
		case "agent_id":
			agent.AgentID = value
		case "name":
			agent.Name = value
		}
	}
	if agent.AgentID == "" {
		return projectAgent{}, errors.New("agent.yaml missing agent_id")
	}
	return agent, nil
}

func yamlQuote(value string) string {
	escaped := strings.ReplaceAll(value, `"`, `\"`)
	return `"` + escaped + `"`
}

func resolveAgentID(command command) (string, error) {
	response, err := http.Get(command.SidecarURL + "/agents")
	if err != nil {
		return "", fmt.Errorf("未指定 --agent，且无法读取本机 Agent 列表: %w", err)
	}
	defer response.Body.Close()
	var payload struct {
		OK    bool           `json:"ok"`
		Error string         `json:"error"`
		Data  []agentSummary `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !payload.OK {
		if payload.Error == "" {
			payload.Error = response.Status
		}
		return "", errors.New(payload.Error)
	}
	if len(payload.Data) == 1 {
		return payload.Data[0].AgentID, nil
	}
	if len(payload.Data) == 0 {
		return "", errors.New("本机还没有注册 Agent；请先通过 Sidecar 注册")
	}
	choices := make([]string, 0, len(payload.Data))
	for _, agent := range payload.Data {
		label := agent.AgentID
		if agent.Name != "" && agent.Name != agent.AgentID {
			label += "(" + agent.Name + ")"
		}
		choices = append(choices, label)
	}
	return "", fmt.Errorf("本机有多个 Agent，请指定 --agent：%s", strings.Join(choices, ", "))
}

func register(sidecarURL string, current session) error {
	body, err := json.Marshal(current)
	if err != nil {
		return err
	}
	response, err := http.Post(sidecarURL+"/pingo/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var payload sidecarResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !payload.OK {
		if payload.Error == "" {
			payload.Error = response.Status
		}
		return errors.New(payload.Error)
	}
	return nil
}

func unregister(sidecarURL string, current session) {
	request, err := http.NewRequest(http.MethodDelete, sidecarURL+"/pingo/sessions/"+current.ID+"?agent_id="+current.AgentID, nil)
	if err != nil {
		return
	}
	response, err := http.DefaultClient.Do(request)
	if err == nil {
		response.Body.Close()
	}
}

func consumeEvents(ctx context.Context, sidecarURL string, current session) {
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
		readSSE(ctx, response.Body, current)
		response.Body.Close()
	}
}

func readSSE(ctx context.Context, body io.Reader, current session) {
	scanner := bufio.NewScanner(body)
	var eventType, data string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			continue
		}
		if line != "" || data == "" {
			continue
		}
		switch eventType {
		case "message":
			var event pingoEvent
			if json.Unmarshal([]byte(data), &event) == nil {
				fmt.Fprintf(os.Stderr, "\a\033]0;Pingo · %s · 未读 %d\007", current.AgentID, event.UnreadCount)
			}
		case "update.available":
			var event struct {
				Version string `json:"version"`
			}
			if json.Unmarshal([]byte(data), &event) == nil && event.Version != "" {
				fmt.Fprintf(os.Stderr, "\a\033]0;Pingo · %s · 可更新 %s\007", current.AgentID, event.Version)
			}
		}
		eventType, data = "", ""
		if ctx.Err() != nil {
			return
		}
	}
}

func wait(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
	}
}

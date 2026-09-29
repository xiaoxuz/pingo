package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeSessionContextDeclaresIdentityAndLoadsPingoSkillOnce(t *testing.T) {
	prompt := claudeSessionContext("Agent One", "agt-1")
	for _, required := range []string{"我是 Pingo Agent「Agent One」", "我的 Agent ID 是 agt-1", "Pingo 是我自己的社交与协作能力", "/pingo", "本次会话开始时", "我不需要在每次社交事件时重新加载", "我不确定如何使用 Pingo"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("session context missing %q: %s", required, prompt)
		}
	}
	for _, forbidden := range []string{"你已作为", "由你自主处理", "请求主人授权"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("session context is not first-person: %q", prompt)
		}
	}
}

func TestAppendClaudeSessionContextPreservesUserPromptAndArguments(t *testing.T) {
	args, err := appendClaudeSessionContext([]string{"--model", "opus", "--append-system-prompt", "保留我的规则", "--continue"}, "Agent One", "agt-1")
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := []string{"--append-system-prompt", "保留我的规则\n\n"}
	if len(args) != 5 || args[0] != wantPrefix[0] || !strings.HasPrefix(args[1], wantPrefix[1]) {
		t.Fatalf("args = %q", args)
	}
	if args[2] != "--model" || args[3] != "opus" || args[4] != "--continue" {
		t.Fatalf("original args not preserved: %q", args)
	}
}

func TestAppendClaudeSessionContextRejectsMissingPromptValue(t *testing.T) {
	if _, err := appendClaudeSessionContext([]string{"--append-system-prompt"}, "Agent One", "agt-1"); err == nil {
		t.Fatal("expected missing append-system-prompt value error")
	}
}

func TestAppendClaudeHookSettingsMergesExistingHooks(t *testing.T) {
	existing := `{"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command","command":"cmux stop"}]}],"PreToolUse":[{"hooks":[{"type":"command","command":"cmux tool"}]}]}}`
	args, err := appendClaudeHookSettings([]string{"--model", "opus", "--settings", existing}, "/opt/pingo bin/pingo", "http://127.0.0.1:19191")
	if err != nil {
		t.Fatal(err)
	}
	settingsValues := argumentValues(args, "--settings")
	if len(settingsValues) != 1 {
		t.Fatalf("settings arguments = %q", args)
	}
	var settings struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(settingsValues[0]), &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings.Hooks["Stop"]) != 2 || settings.Hooks["Stop"][0].Hooks[0].Command != "cmux stop" {
		t.Fatalf("Stop hooks = %#v", settings.Hooks["Stop"])
	}
	if len(settings.Hooks["SessionStart"]) != 1 || len(settings.Hooks["UserPromptSubmit"]) != 1 || len(settings.Hooks["PreToolUse"]) != 1 {
		t.Fatalf("merged hooks = %#v", settings.Hooks)
	}
	if command := settings.Hooks["Stop"][1].Hooks[0].Command; !strings.Contains(command, "'/opt/pingo bin/pingo' hook") || !strings.Contains(command, "--sidecar-url 'http://127.0.0.1:19191'") {
		t.Fatalf("pingo hook command = %q", command)
	}
}

func TestAppendClaudeHookSettingsReadsFileAndDoesNotDuplicateHook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"permissions":{"defaultMode":"acceptEdits"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	args, err := appendClaudeHookSettings([]string{"--settings=" + path}, "/opt/pingo", defaultSidecarURL)
	if err != nil {
		t.Fatal(err)
	}
	args, err = appendClaudeHookSettings(args, "/opt/pingo", defaultSidecarURL)
	if err != nil {
		t.Fatal(err)
	}
	values := argumentValues(args, "--settings")
	if len(values) != 1 || strings.Count(values[0], "/opt/pingo") != 3 || !strings.Contains(values[0], "acceptEdits") {
		t.Fatalf("settings = %q", values)
	}
}

func TestAppendClaudeHookSettingsRejectsInvalidJSON(t *testing.T) {
	if _, err := appendClaudeHookSettings([]string{"--settings", "not-json"}, "/opt/pingo", defaultSidecarURL); err == nil {
		t.Fatal("expected invalid settings error")
	}
}

func TestHookStateMapsClaudeLifecycleEvents(t *testing.T) {
	for _, test := range []struct {
		event, state, evidence string
	}{
		{"SessionStart", "free", "hook_session_start"},
		{"UserPromptSubmit", "busy", "hook_user_prompt_submit"},
		{"Stop", "free", "hook_stop"},
	} {
		state, evidence, ok := hookState(test.event)
		if !ok || state != test.state || evidence != test.evidence {
			t.Fatalf("hookState(%q) = %q, %q, %v", test.event, state, evidence, ok)
		}
	}
	if _, _, ok := hookState("Notification"); ok {
		t.Fatal("Notification must not control runtime state")
	}
}

func argumentValues(args []string, name string) []string {
	var values []string
	for index := 0; index < len(args); index++ {
		if args[index] == name && index+1 < len(args) {
			values = append(values, args[index+1])
			index++
		} else if strings.HasPrefix(args[index], name+"=") {
			values = append(values, strings.TrimPrefix(args[index], name+"="))
		}
	}
	return values
}

func TestParseCommandAcceptsAgentAndProvider(t *testing.T) {
	command, err := parseCommand([]string{"--agent", "agent-b", "claude", "--model", "opus"})
	if err != nil {
		t.Fatalf("parseCommand() error = %v", err)
	}
	if command.AgentID != "agent-b" || command.Provider != "claude" {
		t.Fatalf("command = %#v", command)
	}
	if len(command.Args) != 2 || command.Args[0] != "--model" {
		t.Fatalf("args = %#v", command.Args)
	}
}

func TestParseCommandRejectsMissingAgent(t *testing.T) {
	command, err := parseCommand([]string{"claude", "--continue"})
	if err != nil {
		t.Fatalf("parseCommand() error = %v", err)
	}
	if command.AgentID != "" || command.Provider != "claude" || len(command.Args) != 1 || command.Args[0] != "--continue" {
		t.Fatalf("command = %#v", command)
	}
}

func TestParseCommandRejectsUnknownProvider(t *testing.T) {
	if _, err := parseCommand([]string{"--agent", "agent-b", "gemini"}); err == nil {
		t.Fatal("expected unknown provider error")
	}
}

func TestProjectAgentRoundTrip(t *testing.T) {
	root := t.TempDir()
	agent := projectAgent{AgentID: "agt_project", Name: "Project Agent"}
	if err := saveProjectAgent(root, agent); err != nil {
		t.Fatal(err)
	}
	loaded, path, err := findProjectAgent(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AgentID != agent.AgentID || loaded.Name != agent.Name {
		t.Fatalf("loaded = %#v", loaded)
	}
	if path != filepath.Join(root, ".pingo", "agent.yaml") {
		t.Fatalf("path = %s", path)
	}
}

func TestFindProjectAgentWalksUp(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := saveProjectAgent(root, projectAgent{AgentID: "agt_parent", Name: "Parent"}); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := findProjectAgent(nested)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AgentID != "agt_parent" {
		t.Fatalf("loaded = %#v", loaded)
	}
}

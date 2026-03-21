package setup

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetSetupSeams(t *testing.T) {
	t.Helper()
	oldRuntimeGOOS := runtimeGOOS
	oldUserHomeDir := userHomeDir
	oldLookPathFn := lookPathFn
	oldRunCommand := runCommand
	oldStatFn := statFn
	oldOpenCodeReadFile := openCodeReadFile
	oldOpenCodeWriteFileFn := openCodeWriteFileFn
	oldReadFileFn := readFileFn
	oldWriteFileFn := writeFileFn
	oldJSONMarshalFn := jsonMarshalFn
	oldJSONMarshalIndentFn := jsonMarshalIndentFn
	oldInjectOpenCodeMCPFn := injectOpenCodeMCPFn
	oldInjectGeminiMCPFn := injectGeminiMCPFn
	oldWriteGeminiSystemPromptFn := writeGeminiSystemPromptFn
	oldWriteCodexMemoryInstructionFilesFn := writeCodexMemoryInstructionFilesFn
	oldInjectCodexMCPFn := injectCodexMCPFn
	oldInjectCodexMemoryConfigFn := injectCodexMemoryConfigFn
	oldAddClaudeCodeAllowlistFn := addClaudeCodeAllowlistFn
	oldOsExecutable := osExecutable
	oldWriteClaudeCodeUserMCPFn := writeClaudeCodeUserMCPFn

	t.Cleanup(func() {
		runtimeGOOS = oldRuntimeGOOS
		userHomeDir = oldUserHomeDir
		lookPathFn = oldLookPathFn
		runCommand = oldRunCommand
		statFn = oldStatFn
		openCodeReadFile = oldOpenCodeReadFile
		openCodeWriteFileFn = oldOpenCodeWriteFileFn
		readFileFn = oldReadFileFn
		writeFileFn = oldWriteFileFn
		jsonMarshalFn = oldJSONMarshalFn
		jsonMarshalIndentFn = oldJSONMarshalIndentFn
		injectOpenCodeMCPFn = oldInjectOpenCodeMCPFn
		injectGeminiMCPFn = oldInjectGeminiMCPFn
		writeGeminiSystemPromptFn = oldWriteGeminiSystemPromptFn
		writeCodexMemoryInstructionFilesFn = oldWriteCodexMemoryInstructionFilesFn
		injectCodexMCPFn = oldInjectCodexMCPFn
		injectCodexMemoryConfigFn = oldInjectCodexMemoryConfigFn
		addClaudeCodeAllowlistFn = oldAddClaudeCodeAllowlistFn
		osExecutable = oldOsExecutable
		writeClaudeCodeUserMCPFn = oldWriteClaudeCodeUserMCPFn
	})
}

func useTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	userHomeDir = func() (string, error) { return home, nil }
	return home
}

func TestSupportedAgentsIncludesGeminiAndCodex(t *testing.T) {
	agents := SupportedAgents()

	var hasGemini bool
	var hasCodex bool
	for _, agent := range agents {
		if agent.Name == "gemini-cli" {
			hasGemini = true
		}
		if agent.Name == "codex" {
			hasCodex = true
		}
	}

	if !hasGemini {
		t.Fatalf("expected gemini-cli in supported agents")
	}
	if !hasCodex {
		t.Fatalf("expected codex in supported agents")
	}
}

func TestInstallGeminiCLIInjectsMCPConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configPath := filepath.Join(home, ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	original := `{"theme":"dark","mcpServers":{"other":{"command":"foo","args":["bar"]}}}`
	if err := os.WriteFile(configPath, []byte(original), 0644); err != nil {
		t.Fatalf("write initial settings: %v", err)
	}

	result, err := Install("gemini-cli")
	if err != nil {
		t.Fatalf("install gemini-cli: %v", err)
	}

	if result.Agent != "gemini-cli" {
		t.Fatalf("unexpected agent in result: %q", result.Agent)
	}

	if result.Files != 2 {
		t.Fatalf("expected 2 files written, got %d", result.Files)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse settings: %v", err)
	}

	mcpServers, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcpServers object")
	}

	engram, ok := mcpServers["engram"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcpServers.engram object")
	}

	if got := engram["command"]; got != "engram" {
		t.Fatalf("expected command engram, got %#v", got)
	}

	args, ok := engram["args"].([]any)
	if !ok || len(args) != 2 || args[0] != "mcp" || args[1] != "--tools=agent" {
		t.Fatalf("expected args [mcp --tools=agent], got %#v", engram["args"])
	}

	if _, ok := mcpServers["other"]; !ok {
		t.Fatalf("expected existing mcp server to be preserved")
	}

	systemPath := filepath.Join(home, ".gemini", "system.md")
	systemRaw, err := os.ReadFile(systemPath)
	if err != nil {
		t.Fatalf("read system prompt: %v", err)
	}
	systemText := string(systemRaw)
	if !strings.Contains(systemText, "### AFTER COMPACTION") {
		t.Fatalf("expected AFTER COMPACTION section in system prompt")
	}
	if !strings.Contains(systemText, "FIRST ACTION REQUIRED") {
		t.Fatalf("expected FIRST ACTION REQUIRED guidance in system prompt")
	}

	// GEMINI_SYSTEM_MD should NOT be set (it breaks Gemini outside $HOME)
	envPath := filepath.Join(home, ".gemini", ".env")
	if _, err := os.Stat(envPath); err == nil {
		envRaw, _ := os.ReadFile(envPath)
		if strings.Contains(string(envRaw), "GEMINI_SYSTEM_MD") {
			t.Fatalf("GEMINI_SYSTEM_MD should not be present in .env, got:\n%s", string(envRaw))
		}
	}

	if _, err := Install("gemini-cli"); err != nil {
		t.Fatalf("second install should be idempotent: %v", err)
	}
}

func TestInstallCodexInjectsTOMLAndIsIdempotent(t *testing.T) {
	resetSetupSeams(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	configPath := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	original := strings.Join([]string{
		"[profile]",
		"name = \"dev\"",
		"",
		"[mcp_servers.existing]",
		"command = \"existing\"",
		"args = [\"x\"]",
		"",
		"[mcp_servers.engram]",
		"command = \"wrong\"",
		"args = [\"wrong\"]",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(original), 0644); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	result, err := Install("codex")
	if err != nil {
		t.Fatalf("install codex: %v", err)
	}

	if result.Agent != "codex" {
		t.Fatalf("unexpected agent in result: %q", result.Agent)
	}

	if result.Files != 3 {
		t.Fatalf("expected 3 files written, got %d", result.Files)
	}

	readAndAssert := func() string {
		t.Helper()
		raw, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read codex config: %v", err)
		}
		text := string(raw)

		if !strings.Contains(text, "[profile]") {
			t.Fatalf("expected existing profile section to be preserved")
		}
		if !strings.Contains(text, "[mcp_servers.existing]") {
			t.Fatalf("expected existing mcp server section to be preserved")
		}
		if strings.Count(text, "[mcp_servers.engram]") != 1 {
			t.Fatalf("expected exactly one engram section, got:\n%s", text)
		}
		if !strings.Contains(text, "command = \"engram\"") {
			t.Fatalf("expected engram command in config, got:\n%s", text)
		}
		if !strings.Contains(text, `args = ["mcp", "--tools=agent"]`) {
			t.Fatalf("expected engram args in config, got:\n%s", text)
		}
		instructionsPath := filepath.Join(home, ".codex", "engram-instructions.md")
		if !strings.Contains(text, "model_instructions_file = \""+instructionsPath+"\"") {
			t.Fatalf("expected model_instructions_file in config, got:\n%s", text)
		}
		compactPromptPath := filepath.Join(home, ".codex", "engram-compact-prompt.md")
		if !strings.Contains(text, "experimental_compact_prompt_file = \""+compactPromptPath+"\"") {
			t.Fatalf("expected compact prompt file key in config, got:\n%s", text)
		}
		firstSection := strings.Index(text, "[profile]")
		if firstSection == -1 {
			t.Fatalf("expected [profile] section in config")
		}
		if idx := strings.Index(text, "model_instructions_file"); idx == -1 || idx > firstSection {
			t.Fatalf("expected model_instructions_file to be top-level before sections, got:\n%s", text)
		}
		if idx := strings.Index(text, "experimental_compact_prompt_file"); idx == -1 || idx > firstSection {
			t.Fatalf("expected compact prompt key to be top-level before sections, got:\n%s", text)
		}
		return text
	}

	first := readAndAssert()

	if _, err := Install("codex"); err != nil {
		t.Fatalf("second install should be idempotent: %v", err)
	}

	second := readAndAssert()
	if first != second {
		t.Fatalf("expected no changes on second install")
	}

	instructionsRaw, err := os.ReadFile(filepath.Join(home, ".codex", "engram-instructions.md"))
	if err != nil {
		t.Fatalf("read codex instructions: %v", err)
	}
	if !strings.Contains(string(instructionsRaw), "### AFTER COMPACTION") {
		t.Fatalf("expected AFTER COMPACTION section in codex instructions")
	}

	compactRaw, err := os.ReadFile(filepath.Join(home, ".codex", "engram-compact-prompt.md"))
	if err != nil {
		t.Fatalf("read codex compact prompt: %v", err)
	}
	if !strings.Contains(string(compactRaw), "FIRST ACTION REQUIRED") {
		t.Fatalf("expected FIRST ACTION REQUIRED text in compact prompt")
	}
}

func TestInstallUnknownAgent(t *testing.T) {
	resetSetupSeams(t)
	_, err := Install("unknown")
	if err == nil || !strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("expected unknown agent error, got %v", err)
	}
}

func TestInstallOpenCodeSuccessAndMCPRegistered(t *testing.T) {
	resetSetupSeams(t)
	home := useTestHome(t)
	runtimeGOOS = "linux"
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)

	result, err := installOpenCode()
	if err != nil {
		t.Fatalf("installOpenCode failed: %v", err)
	}
	if result.Files != 2 {
		t.Fatalf("expected 2 files after MCP registration, got %d", result.Files)
	}

	pluginPath := filepath.Join(xdg, "opencode", "plugins", "engram.ts")
	if _, err := os.Stat(pluginPath); err != nil {
		t.Fatalf("expected plugin file to exist: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(xdg, "opencode", "opencode.json"))
	if err != nil {
		t.Fatalf("read opencode config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse opencode config: %v", err)
	}
	mcp, ok := cfg["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcp object in opencode.json")
	}
	if _, ok := mcp["engram"]; !ok {
		t.Fatalf("expected mcp.engram registration")
	}
}

func TestInstallOpenCodeReadEmbeddedError(t *testing.T) {
	resetSetupSeams(t)
	home := useTestHome(t)
	runtimeGOOS = "linux"
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	openCodeReadFile = func(string) ([]byte, error) {
		return nil, errors.New("boom")
	}

	_, err := installOpenCode()
	if err == nil || !strings.Contains(err.Error(), "read embedded engram.ts") {
		t.Fatalf("expected read embedded error, got %v", err)
	}
}

func TestInstallOpenCodeWriteError(t *testing.T) {
	resetSetupSeams(t)
	home := useTestHome(t)
	runtimeGOOS = "linux"
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	openCodeWriteFileFn = func(string, []byte, os.FileMode) error {
		return errors.New("write boom")
	}

	_, err := installOpenCode()
	if err == nil || !strings.Contains(err.Error(), "write ") {
		t.Fatalf("expected write error, got %v", err)
	}
}

func TestInstallOpenCodeMCPInjectionFailureIsNonFatal(t *testing.T) {
	resetSetupSeams(t)
	home := useTestHome(t)
	runtimeGOOS = "linux"
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	injectOpenCodeMCPFn = func() error {
		return errors.New("cannot write config")
	}

	result, err := installOpenCode()
	if err != nil {
		t.Fatalf("expected non-fatal MCP injection failure, got %v", err)
	}
	if result.Files != 1 {
		t.Fatalf("expected only plugin file counted when MCP injection fails, got %d", result.Files)
	}
}

func TestInjectOpenCodeMCPPreservesExistingAndIsIdempotent(t *testing.T) {
	resetSetupSeams(t)
	home := useTestHome(t)
	runtimeGOOS = "linux"
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)

	configPath := filepath.Join(xdg, "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	initial := `{"theme":"kanagawa","mcp":{"other":{"type":"local","command":["foo"]}}}`
	if err := os.WriteFile(configPath, []byte(initial), 0644); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	if err := injectOpenCodeMCP(); err != nil {
		t.Fatalf("injectOpenCodeMCP failed: %v", err)
	}
	if err := injectOpenCodeMCP(); err != nil {
		t.Fatalf("injectOpenCodeMCP should be idempotent: %v", err)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse updated config: %v", err)
	}
	mcp, ok := cfg["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcp object")
	}
	if _, ok := mcp["other"]; !ok {
		t.Fatalf("expected existing mcp entry to be preserved")
	}
	engram, ok := mcp["engram"].(map[string]any)
	if !ok {
		t.Fatalf("expected engram object")
	}
	if engram["enabled"] != true {
		t.Fatalf("expected engram.enabled=true")
	}
}

func TestInjectOpenCodeMCPConfigErrors(t *testing.T) {
	t.Run("invalid root json", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "linux"
		xdg := filepath.Join(home, "xdg")
		t.Setenv("XDG_CONFIG_HOME", xdg)

		configPath := filepath.Join(xdg, "opencode", "opencode.json")
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			t.Fatalf("mkdir config dir: %v", err)
		}
		if err := os.WriteFile(configPath, []byte("{"), 0644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		err := injectOpenCodeMCP()
		if err == nil || !strings.Contains(err.Error(), "parse config") {
			t.Fatalf("expected parse config error, got %v", err)
		}
	})

	t.Run("invalid mcp block", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "linux"
		xdg := filepath.Join(home, "xdg")
		t.Setenv("XDG_CONFIG_HOME", xdg)

		configPath := filepath.Join(xdg, "opencode", "opencode.json")
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			t.Fatalf("mkdir config dir: %v", err)
		}
		if err := os.WriteFile(configPath, []byte(`{"mcp":"nope"}`), 0644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		err := injectOpenCodeMCP()
		if err == nil || !strings.Contains(err.Error(), "parse mcp block") {
			t.Fatalf("expected parse mcp block error, got %v", err)
		}
	})

	t.Run("read error", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "linux"
		xdg := filepath.Join(home, "xdg")
		t.Setenv("XDG_CONFIG_HOME", xdg)

		configPath := filepath.Join(xdg, "opencode", "opencode.json")
		if err := os.MkdirAll(configPath, 0755); err != nil {
			t.Fatalf("create directory at config path: %v", err)
		}

		err := injectOpenCodeMCP()
		if err == nil || !strings.Contains(err.Error(), "read config") {
			t.Fatalf("expected read config error, got %v", err)
		}
	})

	t.Run("marshal engram entry error", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "linux"
		xdg := filepath.Join(home, "xdg")
		t.Setenv("XDG_CONFIG_HOME", xdg)

		jsonMarshalFn = func(any) ([]byte, error) {
			return nil, errors.New("marshal entry boom")
		}

		err := injectOpenCodeMCP()
		if err == nil || !strings.Contains(err.Error(), "marshal engram entry") {
			t.Fatalf("expected marshal engram entry error, got %v", err)
		}
	})

	t.Run("marshal mcp block error", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "linux"
		xdg := filepath.Join(home, "xdg")
		t.Setenv("XDG_CONFIG_HOME", xdg)

		calls := 0
		jsonMarshalFn = func(v any) ([]byte, error) {
			calls++
			if calls == 2 {
				return nil, errors.New("marshal mcp boom")
			}
			return json.Marshal(v)
		}

		err := injectOpenCodeMCP()
		if err == nil || !strings.Contains(err.Error(), "marshal mcp block") {
			t.Fatalf("expected marshal mcp block error, got %v", err)
		}
	})

	t.Run("marshal config error", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "linux"
		xdg := filepath.Join(home, "xdg")
		t.Setenv("XDG_CONFIG_HOME", xdg)

		jsonMarshalIndentFn = func(any, string, string) ([]byte, error) {
			return nil, errors.New("marshal config boom")
		}

		err := injectOpenCodeMCP()
		if err == nil || !strings.Contains(err.Error(), "marshal config") {
			t.Fatalf("expected marshal config error, got %v", err)
		}
	})
}

func TestDefaultRunCommandExecutes(t *testing.T) {
	resetSetupSeams(t)
	out, err := runCommand("sh", "-c", "printf ok")
	if err != nil {
		t.Fatalf("expected default runCommand to execute, got %v", err)
	}
	if string(out) != "ok" {
		t.Fatalf("unexpected output: %q", string(out))
	}
}

func TestInstallClaudeCodeBranches(t *testing.T) {
	t.Run("cli missing", func(t *testing.T) {
		resetSetupSeams(t)
		lookPathFn = func(string) (string, error) {
			return "", errors.New("not found")
		}

		_, err := installClaudeCode()
		if err == nil || !strings.Contains(err.Error(), "claude CLI not found") {
			t.Fatalf("expected not found error, got %v", err)
		}
	})

	t.Run("marketplace add hard failure", func(t *testing.T) {
		resetSetupSeams(t)
		lookPathFn = func(string) (string, error) { return "claude", nil }
		runCommand = func(string, ...string) ([]byte, error) {
			return []byte("permission denied"), errors.New("exit 1")
		}

		_, err := installClaudeCode()
		if err == nil || !strings.Contains(err.Error(), "marketplace add failed") {
			t.Fatalf("expected marketplace add failure, got %v", err)
		}
	})

	t.Run("marketplace already then install success", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		lookPathFn = func(string) (string, error) { return "claude", nil }
		writeClaudeCodeUserMCPFn = func() error { return nil }
		calls := 0
		runCommand = func(_ string, args ...string) ([]byte, error) {
			calls++
			if calls == 1 {
				if strings.Join(args, " ") != "plugin marketplace add "+claudeCodeMarketplace {
					t.Fatalf("unexpected first command args: %q", strings.Join(args, " "))
				}
				return []byte("already added"), errors.New("exit 1")
			}
			if strings.Join(args, " ") != "plugin install engram" {
				t.Fatalf("unexpected second command args: %q", strings.Join(args, " "))
			}
			return []byte("installed"), nil
		}

		result, err := installClaudeCode()
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if result.Agent != "claude-code" {
			t.Fatalf("unexpected agent: %q", result.Agent)
		}
		// When writeClaudeCodeUserMCP succeeds, files == 1
		if result.Files != 1 {
			t.Fatalf("expected 1 file when user MCP write succeeds, got %d", result.Files)
		}
		// Destination should point to the .claude/mcp dir, not be empty
		expectedDir := filepath.Join(home, ".claude", "mcp")
		if result.Destination != expectedDir {
			t.Fatalf("expected destination %q, got %q", expectedDir, result.Destination)
		}
	})

	t.Run("install hard failure", func(t *testing.T) {
		resetSetupSeams(t)
		lookPathFn = func(string) (string, error) { return "claude", nil }
		writeClaudeCodeUserMCPFn = func() error { return nil }
		calls := 0
		runCommand = func(string, ...string) ([]byte, error) {
			calls++
			if calls == 1 {
				return []byte("ok"), nil
			}
			return []byte("network failure"), errors.New("exit 1")
		}

		_, err := installClaudeCode()
		if err == nil || !strings.Contains(err.Error(), "plugin install failed") {
			t.Fatalf("expected plugin install failure, got %v", err)
		}
	})

	t.Run("install already is success", func(t *testing.T) {
		resetSetupSeams(t)
		useTestHome(t)
		lookPathFn = func(string) (string, error) { return "claude", nil }
		writeClaudeCodeUserMCPFn = func() error { return nil }
		calls := 0
		runCommand = func(string, ...string) ([]byte, error) {
			calls++
			if calls == 1 {
				return []byte("ok"), nil
			}
			return []byte("already installed"), errors.New("exit 1")
		}

		if _, err := installClaudeCode(); err != nil {
			t.Fatalf("expected already-installed branch to succeed, got %v", err)
		}
	})

	t.Run("user mcp write failure is non-fatal", func(t *testing.T) {
		resetSetupSeams(t)
		useTestHome(t)
		lookPathFn = func(string) (string, error) { return "claude", nil }
		runCommand = func(string, ...string) ([]byte, error) { return []byte("ok"), nil }
		writeClaudeCodeUserMCPFn = func() error { return errors.New("disk full") }

		result, err := installClaudeCode()
		if err != nil {
			t.Fatalf("user MCP write failure should be non-fatal, got %v", err)
		}
		// files == 0 when writeClaudeCodeUserMCP fails
		if result.Files != 0 {
			t.Fatalf("expected 0 files when user MCP write fails, got %d", result.Files)
		}
	})
}

// ─── Issue #100: Windows PATH fix ────────────────────────────────────────────

func TestWriteClaudeCodeUserMCP(t *testing.T) {
	t.Run("writes json with absolute binary path", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		osExecutable = func() (string, error) { return "/usr/local/bin/engram", nil }

		if err := writeClaudeCodeUserMCP(); err != nil {
			t.Fatalf("writeClaudeCodeUserMCP failed: %v", err)
		}

		mcpPath := filepath.Join(home, ".claude", "mcp", "engram.json")
		raw, err := os.ReadFile(mcpPath)
		if err != nil {
			t.Fatalf("read mcp config: %v", err)
		}

		var cfg map[string]any
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("parse mcp config: %v", err)
		}

		if cfg["command"] != "/usr/local/bin/engram" {
			t.Fatalf("expected absolute path command, got %#v", cfg["command"])
		}
		args, ok := cfg["args"].([]any)
		if !ok || len(args) != 2 || args[0] != "mcp" || args[1] != "--tools=agent" {
			t.Fatalf("expected args [mcp --tools=agent], got %#v", cfg["args"])
		}
	})

	t.Run("overwrites existing (idempotent — always refreshes path)", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		osExecutable = func() (string, error) { return "/new/path/engram", nil }

		mcpDir := filepath.Join(home, ".claude", "mcp")
		if err := os.MkdirAll(mcpDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(mcpDir, "engram.json"), []byte(`{"command":"old"}`), 0644); err != nil {
			t.Fatalf("write old config: %v", err)
		}

		if err := writeClaudeCodeUserMCP(); err != nil {
			t.Fatalf("writeClaudeCodeUserMCP failed: %v", err)
		}

		raw, err := os.ReadFile(filepath.Join(mcpDir, "engram.json"))
		if err != nil {
			t.Fatalf("read updated config: %v", err)
		}
		var cfg map[string]any
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("parse config: %v", err)
		}
		if cfg["command"] != "/new/path/engram" {
			t.Fatalf("expected updated command, got %#v", cfg["command"])
		}
	})

	t.Run("os.Executable failure returns error", func(t *testing.T) {
		resetSetupSeams(t)
		useTestHome(t)
		osExecutable = func() (string, error) { return "", errors.New("exec not found") }

		err := writeClaudeCodeUserMCP()
		if err == nil || !strings.Contains(err.Error(), "resolve binary path") {
			t.Fatalf("expected resolve binary path error, got %v", err)
		}
	})

	t.Run("marshal error returns error", func(t *testing.T) {
		resetSetupSeams(t)
		useTestHome(t)
		osExecutable = func() (string, error) { return "/bin/engram", nil }
		jsonMarshalIndentFn = func(any, string, string) ([]byte, error) {
			return nil, errors.New("marshal boom")
		}

		err := writeClaudeCodeUserMCP()
		if err == nil || !strings.Contains(err.Error(), "marshal mcp config") {
			t.Fatalf("expected marshal mcp config error, got %v", err)
		}
	})

	t.Run("write error returns error", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		osExecutable = func() (string, error) { return "/bin/engram", nil }
		// Make ~/.claude/mcp/engram.json a directory so write fails
		mcpDir := filepath.Join(home, ".claude", "mcp")
		if err := os.MkdirAll(mcpDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(mcpDir, "engram.json"), 0755); err != nil {
			t.Fatalf("create dir as file: %v", err)
		}

		err := writeClaudeCodeUserMCP()
		if err == nil || !strings.Contains(err.Error(), "write mcp config") {
			t.Fatalf("expected write mcp config error, got %v", err)
		}
	})

	t.Run("create dir error returns error", func(t *testing.T) {
		resetSetupSeams(t)
		// Block ~/.claude/mcp creation by making .claude a file
		blocked := t.TempDir()
		if err := os.WriteFile(filepath.Join(blocked, ".claude"), []byte("x"), 0644); err != nil {
			t.Fatalf("write blocking file: %v", err)
		}
		userHomeDir = func() (string, error) { return blocked, nil }
		osExecutable = func() (string, error) { return "/bin/engram", nil }

		err := writeClaudeCodeUserMCP()
		if err == nil || !strings.Contains(err.Error(), "create mcp dir") {
			t.Fatalf("expected create mcp dir error, got %v", err)
		}
	})
}

func TestResolveEngramCommand(t *testing.T) {
	t.Run("unix returns bare name", func(t *testing.T) {
		resetSetupSeams(t)
		runtimeGOOS = "linux"
		osExecutable = func() (string, error) { return "/usr/local/bin/engram", nil }

		if got := resolveEngramCommand(); got != "engram" {
			t.Fatalf("expected bare 'engram' on unix, got %q", got)
		}
	})

	t.Run("darwin returns bare name", func(t *testing.T) {
		resetSetupSeams(t)
		runtimeGOOS = "darwin"
		osExecutable = func() (string, error) { return "/opt/homebrew/bin/engram", nil }

		if got := resolveEngramCommand(); got != "engram" {
			t.Fatalf("expected bare 'engram' on darwin, got %q", got)
		}
	})

	t.Run("windows returns absolute path", func(t *testing.T) {
		resetSetupSeams(t)
		runtimeGOOS = "windows"
		osExecutable = func() (string, error) { return `C:\Users\user\bin\engram.exe`, nil }

		got := resolveEngramCommand()
		// EvalSymlinks may change the path on real OS but in tests it should
		// either equal the input or the resolved form — either way not bare "engram"
		if got == "engram" {
			t.Fatalf("expected absolute path on windows, got bare 'engram'")
		}
		if !strings.Contains(got, "engram") {
			t.Fatalf("expected engram in path, got %q", got)
		}
	})

	t.Run("windows executable error falls back to bare name", func(t *testing.T) {
		resetSetupSeams(t)
		runtimeGOOS = "windows"
		osExecutable = func() (string, error) { return "", errors.New("no executable") }

		if got := resolveEngramCommand(); got != "engram" {
			t.Fatalf("expected fallback to bare 'engram', got %q", got)
		}
	})
}

func TestClaudeCodeMCPDirPaths(t *testing.T) {
	resetSetupSeams(t)
	userHomeDir = func() (string, error) { return "/home/tester", nil }

	expectedDir := filepath.Join("/home/tester", ".claude", "mcp")
	if got := claudeCodeMCPDir(); got != expectedDir {
		t.Fatalf("expected %s, got %s", expectedDir, got)
	}

	expectedPath := filepath.Join("/home/tester", ".claude", "mcp", "engram.json")
	if got := claudeCodeUserMCPPath(); got != expectedPath {
		t.Fatalf("expected %s, got %s", expectedPath, got)
	}
}

func TestGeminiInjectUsesAbsolutePathOnWindows(t *testing.T) {
	t.Run("windows uses absolute path", func(t *testing.T) {
		resetSetupSeams(t)
		runtimeGOOS = "windows"
		osExecutable = func() (string, error) { return `C:\Users\user\bin\engram.exe`, nil }

		configPath := filepath.Join(t.TempDir(), "settings.json")
		if err := injectGeminiMCP(configPath); err != nil {
			t.Fatalf("injectGeminiMCP failed: %v", err)
		}

		raw, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		var cfg map[string]any
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("parse config: %v", err)
		}
		mcpServers := cfg["mcpServers"].(map[string]any)
		engram := mcpServers["engram"].(map[string]any)
		cmd := engram["command"].(string)
		if cmd == "engram" {
			t.Fatalf("expected absolute path on windows, got bare 'engram'")
		}
		if !strings.Contains(cmd, "engram") {
			t.Fatalf("expected engram in command path, got %q", cmd)
		}
	})

	t.Run("unix uses bare name", func(t *testing.T) {
		resetSetupSeams(t)
		runtimeGOOS = "linux"

		configPath := filepath.Join(t.TempDir(), "settings.json")
		if err := injectGeminiMCP(configPath); err != nil {
			t.Fatalf("injectGeminiMCP failed: %v", err)
		}

		raw, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		var cfg map[string]any
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("parse config: %v", err)
		}
		mcpServers := cfg["mcpServers"].(map[string]any)
		engram := mcpServers["engram"].(map[string]any)
		if got := engram["command"]; got != "engram" {
			t.Fatalf("expected bare 'engram' on unix, got %#v", got)
		}
	})
}

func TestCodexBlockUsesAbsolutePathOnWindows(t *testing.T) {
	t.Run("windows uses absolute path in codex block", func(t *testing.T) {
		resetSetupSeams(t)
		runtimeGOOS = "windows"
		osExecutable = func() (string, error) { return `C:\Users\user\bin\engram.exe`, nil }

		block := codexEngramBlockStr()
		if strings.Contains(block, `"engram"`) && !strings.Contains(block, `C:\`) {
			// The block should contain an absolute path, not just bare "engram"
			t.Fatalf("expected absolute path in windows codex block, got:\n%s", block)
		}
		if !strings.Contains(block, "[mcp_servers.engram]") {
			t.Fatalf("expected mcp_servers.engram header, got:\n%s", block)
		}
		if !strings.Contains(block, `args = ["mcp", "--tools=agent"]`) {
			t.Fatalf("expected args in codex block, got:\n%s", block)
		}
	})

	t.Run("unix uses bare name in codex block", func(t *testing.T) {
		resetSetupSeams(t)
		runtimeGOOS = "linux"

		block := codexEngramBlockStr()
		if !strings.Contains(block, `command = "engram"`) {
			t.Fatalf("expected bare engram in unix codex block, got:\n%s", block)
		}
	})
}

func TestPathHelpersAcrossOSVariants(t *testing.T) {
	resetSetupSeams(t)
	userHomeDir = func() (string, error) { return "/home/tester", nil }

	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("APPDATA", "")

	runtimeGOOS = "linux"
	if got := openCodeConfigPath(); got != filepath.Join("/home/tester", ".config", "opencode", "opencode.json") {
		t.Fatalf("unexpected linux openCodeConfigPath: %s", got)
	}
	if got := openCodePluginDir(); got != filepath.Join("/home/tester", ".config", "opencode", "plugins") {
		t.Fatalf("unexpected linux openCodePluginDir: %s", got)
	}
	if got := geminiConfigPath(); got != filepath.Join("/home/tester", ".gemini", "settings.json") {
		t.Fatalf("unexpected linux geminiConfigPath: %s", got)
	}
	if got := codexConfigPath(); got != filepath.Join("/home/tester", ".codex", "config.toml") {
		t.Fatalf("unexpected linux codexConfigPath: %s", got)
	}

	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got := openCodeConfigPath(); got != filepath.Join("/xdg", "opencode", "opencode.json") {
		t.Fatalf("unexpected linux xdg openCodeConfigPath: %s", got)
	}
	if got := openCodePluginDir(); got != filepath.Join("/xdg", "opencode", "plugins") {
		t.Fatalf("unexpected linux xdg openCodePluginDir: %s", got)
	}

	runtimeGOOS = "windows"
	t.Setenv("APPDATA", "C:/AppData/Roaming")
	t.Setenv("XDG_CONFIG_HOME", "")
	// OpenCode uses ~/.config/opencode/ on ALL platforms, ignoring %APPDATA%
	if got := openCodeConfigPath(); got != filepath.Join("/home/tester", ".config", "opencode", "opencode.json") {
		t.Fatalf("unexpected windows openCodeConfigPath: %s", got)
	}
	if got := openCodePluginDir(); got != filepath.Join("/home/tester", ".config", "opencode", "plugins") {
		t.Fatalf("unexpected windows openCodePluginDir: %s", got)
	}
	if got := geminiConfigPath(); got != filepath.Join("C:/AppData/Roaming", "gemini", "settings.json") {
		t.Fatalf("unexpected windows geminiConfigPath: %s", got)
	}
	if got := codexConfigPath(); got != filepath.Join("C:/AppData/Roaming", "codex", "config.toml") {
		t.Fatalf("unexpected windows codexConfigPath: %s", got)
	}

	t.Setenv("APPDATA", "")
	// OpenCode still uses ~/.config/opencode/ even without APPDATA
	if got := openCodeConfigPath(); got != filepath.Join("/home/tester", ".config", "opencode", "opencode.json") {
		t.Fatalf("unexpected windows fallback openCodeConfigPath: %s", got)
	}
	if got := openCodePluginDir(); got != filepath.Join("/home/tester", ".config", "opencode", "plugins") {
		t.Fatalf("unexpected windows fallback openCodePluginDir: %s", got)
	}
	if got := geminiConfigPath(); got != filepath.Join("/home/tester", "AppData", "Roaming", "gemini", "settings.json") {
		t.Fatalf("unexpected windows fallback geminiConfigPath: %s", got)
	}
	if got := codexConfigPath(); got != filepath.Join("/home/tester", "AppData", "Roaming", "codex", "config.toml") {
		t.Fatalf("unexpected windows fallback codexConfigPath: %s", got)
	}

	runtimeGOOS = "plan9"
	if got := openCodeConfigPath(); got != filepath.Join("/home/tester", ".config", "opencode", "opencode.json") {
		t.Fatalf("unexpected default openCodeConfigPath: %s", got)
	}
	if got := openCodePluginDir(); got != filepath.Join("/home/tester", ".config", "opencode", "plugins") {
		t.Fatalf("unexpected default openCodePluginDir: %s", got)
	}

	if got := geminiSystemPromptPath(); got != filepath.Join(filepath.Dir(geminiConfigPath()), "system.md") {
		t.Fatalf("unexpected gemini system prompt path: %s", got)
	}
	if got := geminiEnvPath(); got != filepath.Join(filepath.Dir(geminiConfigPath()), ".env") {
		t.Fatalf("unexpected gemini env path: %s", got)
	}
	if got := codexInstructionsPath(); got != filepath.Join(filepath.Dir(codexConfigPath()), "engram-instructions.md") {
		t.Fatalf("unexpected codex instructions path: %s", got)
	}
	if got := codexCompactPromptPath(); got != filepath.Join(filepath.Dir(codexConfigPath()), "engram-compact-prompt.md") {
		t.Fatalf("unexpected codex compact prompt path: %s", got)
	}
}

func TestInstallGeminiCLIErrorPropagation(t *testing.T) {
	t.Run("inject mcp fails", func(t *testing.T) {
		resetSetupSeams(t)
		injectGeminiMCPFn = func(string) error { return errors.New("inject failed") }

		_, err := installGeminiCLI()
		if err == nil || !strings.Contains(err.Error(), "inject failed") {
			t.Fatalf("expected inject failure, got %v", err)
		}
	})

	t.Run("write system prompt fails", func(t *testing.T) {
		resetSetupSeams(t)
		injectGeminiMCPFn = func(string) error { return nil }
		writeGeminiSystemPromptFn = func() error { return errors.New("prompt failed") }

		_, err := installGeminiCLI()
		if err == nil || !strings.Contains(err.Error(), "prompt failed") {
			t.Fatalf("expected system prompt failure, got %v", err)
		}
	})

}

func TestInstallCodexErrorPropagation(t *testing.T) {
	t.Run("write instruction files fails", func(t *testing.T) {
		resetSetupSeams(t)
		writeCodexMemoryInstructionFilesFn = func() (string, error) {
			return "", errors.New("instructions failed")
		}

		_, err := installCodex()
		if err == nil || !strings.Contains(err.Error(), "instructions failed") {
			t.Fatalf("expected instructions failure, got %v", err)
		}
	})

	t.Run("inject mcp fails", func(t *testing.T) {
		resetSetupSeams(t)
		writeCodexMemoryInstructionFilesFn = func() (string, error) { return "/tmp/instructions", nil }
		injectCodexMCPFn = func(string) error { return errors.New("mcp failed") }

		_, err := installCodex()
		if err == nil || !strings.Contains(err.Error(), "mcp failed") {
			t.Fatalf("expected mcp failure, got %v", err)
		}
	})

	t.Run("inject memory config fails", func(t *testing.T) {
		resetSetupSeams(t)
		writeCodexMemoryInstructionFilesFn = func() (string, error) { return "/tmp/instructions", nil }
		injectCodexMCPFn = func(string) error { return nil }
		injectCodexMemoryConfigFn = func(string, string, string) error { return errors.New("memory config failed") }

		_, err := installCodex()
		if err == nil || !strings.Contains(err.Error(), "memory config failed") {
			t.Fatalf("expected memory config failure, got %v", err)
		}
	})
}

func TestGeminiAndCodexHelpersErrorPaths(t *testing.T) {
	t.Run("injectGeminiMCP creates file from missing config", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "settings.json")

		if err := injectGeminiMCP(configPath); err != nil {
			t.Fatalf("injectGeminiMCP failed: %v", err)
		}

		raw, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}

		var cfg map[string]any
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("parse config: %v", err)
		}

		mcpServers, ok := cfg["mcpServers"].(map[string]any)
		if !ok {
			t.Fatalf("expected mcpServers object")
		}
		engram, ok := mcpServers["engram"].(map[string]any)
		if !ok {
			t.Fatalf("expected engram server object")
		}
		if engram["command"] != "engram" {
			t.Fatalf("expected engram command, got %#v", engram["command"])
		}
	})

	t.Run("injectGeminiMCP marshal entry error", func(t *testing.T) {
		resetSetupSeams(t)
		configPath := filepath.Join(t.TempDir(), "settings.json")
		jsonMarshalFn = func(any) ([]byte, error) {
			return nil, errors.New("marshal boom")
		}

		err := injectGeminiMCP(configPath)
		if err == nil || !strings.Contains(err.Error(), "marshal engram entry") {
			t.Fatalf("expected marshal engram entry error, got %v", err)
		}
	})

	t.Run("injectGeminiMCP marshal indent error", func(t *testing.T) {
		resetSetupSeams(t)
		configPath := filepath.Join(t.TempDir(), "settings.json")
		jsonMarshalIndentFn = func(any, string, string) ([]byte, error) {
			return nil, errors.New("indent boom")
		}

		err := injectGeminiMCP(configPath)
		if err == nil || !strings.Contains(err.Error(), "marshal config") {
			t.Fatalf("expected marshal config error, got %v", err)
		}
	})

	t.Run("injectGeminiMCP marshal mcpServers error", func(t *testing.T) {
		resetSetupSeams(t)
		configPath := filepath.Join(t.TempDir(), "settings.json")
		calls := 0
		jsonMarshalFn = func(v any) ([]byte, error) {
			calls++
			if calls == 2 {
				return nil, errors.New("mcp marshal boom")
			}
			return json.Marshal(v)
		}

		err := injectGeminiMCP(configPath)
		if err == nil || !strings.Contains(err.Error(), "marshal mcpServers block") {
			t.Fatalf("expected marshal mcpServers block error, got %v", err)
		}
	})

	t.Run("injectGeminiMCP write error", func(t *testing.T) {
		resetSetupSeams(t)
		configPath := filepath.Join(t.TempDir(), "settings.json")
		writeFileFn = func(string, []byte, os.FileMode) error {
			return errors.New("write boom")
		}

		err := injectGeminiMCP(configPath)
		if err == nil || !strings.Contains(err.Error(), "write config") {
			t.Fatalf("expected write config error, got %v", err)
		}
	})

	t.Run("injectGeminiMCP parse error", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(configPath, []byte("{"), 0644); err != nil {
			t.Fatalf("write invalid json: %v", err)
		}
		err := injectGeminiMCP(configPath)
		if err == nil || !strings.Contains(err.Error(), "parse config") {
			t.Fatalf("expected parse config error, got %v", err)
		}
	})

	t.Run("injectGeminiMCP parse mcpServers error", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(configPath, []byte(`{"mcpServers":"bad"}`), 0644); err != nil {
			t.Fatalf("write invalid mcpServers: %v", err)
		}
		err := injectGeminiMCP(configPath)
		if err == nil || !strings.Contains(err.Error(), "parse mcpServers block") {
			t.Fatalf("expected parse mcpServers error, got %v", err)
		}
	})

	t.Run("injectGeminiMCP create config dir error", func(t *testing.T) {
		base := t.TempDir()
		parent := filepath.Join(base, "blocked")
		if err := os.WriteFile(parent, []byte("x"), 0644); err != nil {
			t.Fatalf("write blocking file: %v", err)
		}
		err := injectGeminiMCP(filepath.Join(parent, "settings.json"))
		if err == nil || !strings.Contains(err.Error(), "create config dir") {
			t.Fatalf("expected create config dir error, got %v", err)
		}
	})

	t.Run("removeGeminiEnvOverride strips GEMINI_SYSTEM_MD line", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "linux"

		envPath := filepath.Join(home, ".gemini", ".env")
		if err := os.MkdirAll(filepath.Dir(envPath), 0755); err != nil {
			t.Fatalf("mkdir env dir: %v", err)
		}
		if err := os.WriteFile(envPath, []byte("OTHER=1\r\nGEMINI_SYSTEM_MD=1\r\n"), 0644); err != nil {
			t.Fatalf("write env file: %v", err)
		}

		removeGeminiEnvOverride()

		raw, err := os.ReadFile(envPath)
		if err != nil {
			t.Fatalf("read env file: %v", err)
		}
		text := string(raw)
		if strings.Contains(text, "GEMINI_SYSTEM_MD") {
			t.Fatalf("expected GEMINI_SYSTEM_MD removed, got:\n%s", text)
		}
		if !strings.Contains(text, "OTHER=1") {
			t.Fatalf("expected OTHER=1 preserved, got:\n%s", text)
		}
	})

	t.Run("removeGeminiEnvOverride deletes empty env file", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "linux"

		envPath := filepath.Join(home, ".gemini", ".env")
		if err := os.MkdirAll(filepath.Dir(envPath), 0755); err != nil {
			t.Fatalf("mkdir env dir: %v", err)
		}
		if err := os.WriteFile(envPath, []byte("GEMINI_SYSTEM_MD=1\n"), 0644); err != nil {
			t.Fatalf("write env file: %v", err)
		}

		removeGeminiEnvOverride()

		if _, err := os.Stat(envPath); !os.IsNotExist(err) {
			t.Fatalf("expected env file deleted when only GEMINI_SYSTEM_MD was present")
		}
	})

	t.Run("removeGeminiEnvOverride no-op when file missing", func(t *testing.T) {
		resetSetupSeams(t)
		_ = useTestHome(t)
		runtimeGOOS = "linux"

		// should not panic or error
		removeGeminiEnvOverride()
	})

	t.Run("writeGeminiSystemPrompt create dir error", func(t *testing.T) {
		resetSetupSeams(t)
		blocked := filepath.Join(t.TempDir(), "home-as-file")
		if err := os.WriteFile(blocked, []byte("x"), 0644); err != nil {
			t.Fatalf("write home file: %v", err)
		}
		userHomeDir = func() (string, error) { return blocked, nil }
		runtimeGOOS = "linux"

		err := writeGeminiSystemPrompt()
		if err == nil || !strings.Contains(err.Error(), "create gemini system prompt dir") {
			t.Fatalf("expected create dir error, got %v", err)
		}
	})

	t.Run("injectCodexMCP read error", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "config.toml")
		if err := os.MkdirAll(configPath, 0755); err != nil {
			t.Fatalf("make config path directory: %v", err)
		}

		err := injectCodexMCP(configPath)
		if err == nil || !strings.Contains(err.Error(), "read config") {
			t.Fatalf("expected read config error, got %v", err)
		}
	})

	t.Run("injectCodexMemoryConfig read error", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "config.toml")
		if err := os.MkdirAll(configPath, 0755); err != nil {
			t.Fatalf("make config path directory: %v", err)
		}

		err := injectCodexMemoryConfig(configPath, "/tmp/instructions.md", "/tmp/compact.md")
		if err == nil || !strings.Contains(err.Error(), "read config") {
			t.Fatalf("expected read config error, got %v", err)
		}
	})

	t.Run("injectCodexMemoryConfig creates missing config", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "config.toml")

		err := injectCodexMemoryConfig(configPath, "/tmp/instructions.md", "/tmp/compact.md")
		if err != nil {
			t.Fatalf("injectCodexMemoryConfig failed: %v", err)
		}

		raw, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		text := string(raw)
		if !strings.Contains(text, "model_instructions_file = \"/tmp/instructions.md\"") {
			t.Fatalf("expected model_instructions_file in config, got:\n%s", text)
		}
		if !strings.Contains(text, "experimental_compact_prompt_file = \"/tmp/compact.md\"") {
			t.Fatalf("expected compact prompt file in config, got:\n%s", text)
		}
	})

	t.Run("injectCodexMemoryConfig write error", func(t *testing.T) {
		resetSetupSeams(t)
		configPath := filepath.Join(t.TempDir(), "config.toml")
		writeFileFn = func(string, []byte, os.FileMode) error {
			return errors.New("write config boom")
		}

		err := injectCodexMemoryConfig(configPath, "/tmp/instructions.md", "/tmp/compact.md")
		if err == nil || !strings.Contains(err.Error(), "write config") {
			t.Fatalf("expected write config error, got %v", err)
		}
	})

	t.Run("upsertCodexEngramBlock replaces section before another section", func(t *testing.T) {
		input := strings.Join([]string{
			"[mcp_servers.engram]",
			"command = \"wrong\"",
			"args = [\"wrong\"]",
			"",
			"[mcp_servers.other]",
			"command = \"other\"",
		}, "\n")

		output := upsertCodexEngramBlock(input)
		if strings.Count(output, "[mcp_servers.engram]") != 1 {
			t.Fatalf("expected one engram block, got:\n%s", output)
		}
		if !strings.Contains(output, "[mcp_servers.other]") {
			t.Fatalf("expected other section preserved, got:\n%s", output)
		}
	})

	t.Run("upsertCodexEngramBlock from empty content", func(t *testing.T) {
		output := upsertCodexEngramBlock("\n\n")
		if output != codexEngramBlock+"\n" {
			t.Fatalf("unexpected output for empty content:\n%s", output)
		}
	})
}

func TestInstallRoutesForOpenCodeAndClaude(t *testing.T) {
	t.Run("opencode route", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "linux"
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

		result, err := Install("opencode")
		if err != nil {
			t.Fatalf("Install(opencode) failed: %v", err)
		}
		if result.Agent != "opencode" {
			t.Fatalf("expected opencode result, got %#v", result)
		}
	})

	t.Run("claude-code route", func(t *testing.T) {
		resetSetupSeams(t)
		useTestHome(t)
		lookPathFn = func(string) (string, error) { return "claude", nil }
		runCommand = func(string, ...string) ([]byte, error) { return []byte("ok"), nil }
		writeClaudeCodeUserMCPFn = func() error { return nil }

		result, err := Install("claude-code")
		if err != nil {
			t.Fatalf("Install(claude-code) failed: %v", err)
		}
		if result.Agent != "claude-code" {
			t.Fatalf("expected claude-code result, got %#v", result)
		}
	})
}

func TestAdditionalHelperBranches(t *testing.T) {
	t.Run("installOpenCode mkdir error", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "linux"

		blocked := filepath.Join(home, "xdg-block")
		if err := os.WriteFile(blocked, []byte("x"), 0644); err != nil {
			t.Fatalf("write blocker file: %v", err)
		}
		t.Setenv("XDG_CONFIG_HOME", blocked)

		_, err := installOpenCode()
		if err == nil || !strings.Contains(err.Error(), "create plugin dir") {
			t.Fatalf("expected create plugin dir error, got %v", err)
		}
	})

	t.Run("injectOpenCodeMCP write error when parent missing", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "linux"
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

		err := injectOpenCodeMCP()
		if err == nil || !strings.Contains(err.Error(), "write config") {
			t.Fatalf("expected write config error, got %v", err)
		}
	})

	t.Run("injectCodexMCP create config dir error", func(t *testing.T) {
		base := t.TempDir()
		blocked := filepath.Join(base, "blocked")
		if err := os.WriteFile(blocked, []byte("x"), 0644); err != nil {
			t.Fatalf("write blocker: %v", err)
		}

		err := injectCodexMCP(filepath.Join(blocked, "config.toml"))
		if err == nil || !strings.Contains(err.Error(), "create config dir") {
			t.Fatalf("expected create config dir error, got %v", err)
		}
	})

	t.Run("injectCodexMCP write error", func(t *testing.T) {
		resetSetupSeams(t)
		configPath := filepath.Join(t.TempDir(), "codex", "config.toml")
		writeFileFn = func(string, []byte, os.FileMode) error {
			return errors.New("write codex boom")
		}

		err := injectCodexMCP(configPath)
		if err == nil || !strings.Contains(err.Error(), "write config") {
			t.Fatalf("expected write config error, got %v", err)
		}
	})

	t.Run("writeCodexMemoryInstructionFiles instructions write error", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "linux"

		instructionsPath := filepath.Join(home, ".codex", "engram-instructions.md")
		if err := os.MkdirAll(instructionsPath, 0755); err != nil {
			t.Fatalf("create instructions path as dir: %v", err)
		}

		_, err := writeCodexMemoryInstructionFiles()
		if err == nil || !strings.Contains(err.Error(), "write codex instructions") {
			t.Fatalf("expected instructions write error, got %v", err)
		}
	})

	t.Run("writeCodexMemoryInstructionFiles compact write error", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "linux"

		compactPath := filepath.Join(home, ".codex", "engram-compact-prompt.md")
		if err := os.MkdirAll(compactPath, 0755); err != nil {
			t.Fatalf("create compact path as dir: %v", err)
		}

		_, err := writeCodexMemoryInstructionFiles()
		if err == nil || !strings.Contains(err.Error(), "write codex compact prompt") {
			t.Fatalf("expected compact prompt write error, got %v", err)
		}
	})

	t.Run("injectGeminiMCP read error", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "settings.json")
		if err := os.MkdirAll(configPath, 0755); err != nil {
			t.Fatalf("create config path as dir: %v", err)
		}

		err := injectGeminiMCP(configPath)
		if err == nil || !strings.Contains(err.Error(), "read config") {
			t.Fatalf("expected read config error, got %v", err)
		}
	})

	t.Run("writeGeminiSystemPrompt write error", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "linux"

		systemPath := filepath.Join(home, ".gemini", "system.md")
		if err := os.MkdirAll(systemPath, 0755); err != nil {
			t.Fatalf("create system path as dir: %v", err)
		}

		err := writeGeminiSystemPrompt()
		if err == nil || !strings.Contains(err.Error(), "write gemini system prompt") {
			t.Fatalf("expected write system prompt error, got %v", err)
		}
	})

	t.Run("writeCodexMemoryInstructionFiles create dir error", func(t *testing.T) {
		resetSetupSeams(t)
		blocked := filepath.Join(t.TempDir(), "home-as-file")
		if err := os.WriteFile(blocked, []byte("x"), 0644); err != nil {
			t.Fatalf("write home file: %v", err)
		}
		userHomeDir = func() (string, error) { return blocked, nil }
		runtimeGOOS = "linux"

		_, err := writeCodexMemoryInstructionFiles()
		if err == nil || !strings.Contains(err.Error(), "create codex instructions dir") {
			t.Fatalf("expected create instructions dir error, got %v", err)
		}
	})
}

func TestAddClaudeCodeAllowlist(t *testing.T) {
	t.Run("creates file from scratch", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)

		if err := AddClaudeCodeAllowlist(); err != nil {
			t.Fatalf("AddClaudeCodeAllowlist() failed: %v", err)
		}

		settingsPath := filepath.Join(home, ".claude", "settings.json")
		raw, err := os.ReadFile(settingsPath)
		if err != nil {
			t.Fatalf("read settings: %v", err)
		}

		var cfg map[string]any
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("parse settings: %v", err)
		}

		perms, ok := cfg["permissions"].(map[string]any)
		if !ok {
			t.Fatalf("expected permissions object")
		}

		allowRaw, ok := perms["allow"].([]any)
		if !ok {
			t.Fatalf("expected allow array")
		}

		if len(allowRaw) != len(claudeCodeMCPTools) {
			t.Fatalf("expected %d tools, got %d", len(claudeCodeMCPTools), len(allowRaw))
		}

		for i, tool := range claudeCodeMCPTools {
			if allowRaw[i] != tool {
				t.Fatalf("expected tool %q at index %d, got %q", tool, i, allowRaw[i])
			}
		}
	})

	t.Run("preserves existing entries", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)

		settingsPath := filepath.Join(home, ".claude", "settings.json")
		if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		existing := `{"attribution":{"commit":""},"permissions":{"allow":["Read","Write","Glob"],"deny":["Read(.env)"]}}`
		if err := os.WriteFile(settingsPath, []byte(existing), 0644); err != nil {
			t.Fatalf("write initial settings: %v", err)
		}

		if err := AddClaudeCodeAllowlist(); err != nil {
			t.Fatalf("AddClaudeCodeAllowlist() failed: %v", err)
		}

		raw, err := os.ReadFile(settingsPath)
		if err != nil {
			t.Fatalf("read settings: %v", err)
		}

		var cfg map[string]any
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("parse settings: %v", err)
		}

		// Check attribution preserved
		if _, ok := cfg["attribution"]; !ok {
			t.Fatalf("expected attribution key to be preserved")
		}

		perms := cfg["permissions"].(map[string]any)

		// Check deny preserved
		deny, ok := perms["deny"].([]any)
		if !ok || len(deny) != 1 || deny[0] != "Read(.env)" {
			t.Fatalf("expected deny list preserved, got %#v", perms["deny"])
		}

		// Check allow has original + new entries
		allow := perms["allow"].([]any)
		expectedLen := 3 + len(claudeCodeMCPTools)
		if len(allow) != expectedLen {
			t.Fatalf("expected %d allow entries, got %d", expectedLen, len(allow))
		}

		// First 3 should be original
		if allow[0] != "Read" || allow[1] != "Write" || allow[2] != "Glob" {
			t.Fatalf("expected original entries preserved at start, got %v %v %v", allow[0], allow[1], allow[2])
		}
	})

	t.Run("idempotent when all tools present", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)

		settingsPath := filepath.Join(home, ".claude", "settings.json")
		if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		// Write settings with all tools already present
		allowJSON, _ := json.Marshal(claudeCodeMCPTools)
		initial := `{"permissions":{"allow":` + string(allowJSON) + `}}`
		if err := os.WriteFile(settingsPath, []byte(initial), 0644); err != nil {
			t.Fatalf("write initial settings: %v", err)
		}

		beforeRaw, _ := os.ReadFile(settingsPath)

		if err := AddClaudeCodeAllowlist(); err != nil {
			t.Fatalf("AddClaudeCodeAllowlist() failed: %v", err)
		}

		afterRaw, _ := os.ReadFile(settingsPath)

		// File should not have been rewritten (early return)
		if string(afterRaw) != string(beforeRaw) {
			t.Fatalf("expected file unchanged when all tools present")
		}
	})

	t.Run("partial existing adds only missing", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)

		settingsPath := filepath.Join(home, ".claude", "settings.json")
		if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		// Include 3 of 11 tools
		partial := []string{
			claudeCodeMCPTools[0],
			claudeCodeMCPTools[3],
			claudeCodeMCPTools[7],
		}
		allowJSON, _ := json.Marshal(partial)
		initial := `{"permissions":{"allow":` + string(allowJSON) + `}}`
		if err := os.WriteFile(settingsPath, []byte(initial), 0644); err != nil {
			t.Fatalf("write initial settings: %v", err)
		}

		if err := AddClaudeCodeAllowlist(); err != nil {
			t.Fatalf("AddClaudeCodeAllowlist() failed: %v", err)
		}

		raw, err := os.ReadFile(settingsPath)
		if err != nil {
			t.Fatalf("read settings: %v", err)
		}

		var cfg map[string]any
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("parse settings: %v", err)
		}

		allow := cfg["permissions"].(map[string]any)["allow"].([]any)
		if len(allow) != len(claudeCodeMCPTools) {
			t.Fatalf("expected %d tools (no duplicates), got %d", len(claudeCodeMCPTools), len(allow))
		}

		// Verify no duplicates
		seen := make(map[string]int)
		for _, entry := range allow {
			seen[entry.(string)]++
		}
		for tool, count := range seen {
			if count > 1 {
				t.Fatalf("duplicate tool entry: %q (count %d)", tool, count)
			}
		}
	})

	t.Run("read error returns error", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)

		settingsPath := filepath.Join(home, ".claude", "settings.json")
		if err := os.MkdirAll(settingsPath, 0755); err != nil {
			t.Fatalf("mkdir as file: %v", err)
		}

		err := AddClaudeCodeAllowlist()
		if err == nil || !strings.Contains(err.Error(), "read settings") {
			t.Fatalf("expected read settings error, got %v", err)
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)

		settingsPath := filepath.Join(home, ".claude", "settings.json")
		if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(settingsPath, []byte("{broken"), 0644); err != nil {
			t.Fatalf("write invalid json: %v", err)
		}

		err := AddClaudeCodeAllowlist()
		if err == nil || !strings.Contains(err.Error(), "parse settings") {
			t.Fatalf("expected parse settings error, got %v", err)
		}
	})

	t.Run("invalid permissions returns error", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)

		settingsPath := filepath.Join(home, ".claude", "settings.json")
		if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(settingsPath, []byte(`{"permissions":"bad"}`), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}

		err := AddClaudeCodeAllowlist()
		if err == nil || !strings.Contains(err.Error(), "parse permissions") {
			t.Fatalf("expected parse permissions error, got %v", err)
		}
	})

	t.Run("invalid allow list returns error", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)

		settingsPath := filepath.Join(home, ".claude", "settings.json")
		if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(settingsPath, []byte(`{"permissions":{"allow":"bad"}}`), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}

		err := AddClaudeCodeAllowlist()
		if err == nil || !strings.Contains(err.Error(), "parse allow list") {
			t.Fatalf("expected parse allow list error, got %v", err)
		}
	})

	t.Run("marshal allow list error", func(t *testing.T) {
		resetSetupSeams(t)
		useTestHome(t)

		jsonMarshalFn = func(any) ([]byte, error) {
			return nil, errors.New("marshal boom")
		}

		err := AddClaudeCodeAllowlist()
		if err == nil || !strings.Contains(err.Error(), "marshal allow list") {
			t.Fatalf("expected marshal allow list error, got %v", err)
		}
	})

	t.Run("marshal permissions error", func(t *testing.T) {
		resetSetupSeams(t)
		useTestHome(t)

		calls := 0
		jsonMarshalFn = func(v any) ([]byte, error) {
			calls++
			if calls == 2 {
				return nil, errors.New("marshal perms boom")
			}
			return json.Marshal(v)
		}

		err := AddClaudeCodeAllowlist()
		if err == nil || !strings.Contains(err.Error(), "marshal permissions") {
			t.Fatalf("expected marshal permissions error, got %v", err)
		}
	})

	t.Run("marshal settings error", func(t *testing.T) {
		resetSetupSeams(t)
		useTestHome(t)

		jsonMarshalIndentFn = func(any, string, string) ([]byte, error) {
			return nil, errors.New("indent boom")
		}

		err := AddClaudeCodeAllowlist()
		if err == nil || !strings.Contains(err.Error(), "marshal settings") {
			t.Fatalf("expected marshal settings error, got %v", err)
		}
	})

	t.Run("write error returns error", func(t *testing.T) {
		resetSetupSeams(t)
		useTestHome(t)

		writeFileFn = func(string, []byte, os.FileMode) error {
			return errors.New("write boom")
		}

		err := AddClaudeCodeAllowlist()
		if err == nil || !strings.Contains(err.Error(), "write settings") {
			t.Fatalf("expected write settings error, got %v", err)
		}
	})

	t.Run("claudeCodeSettingsPath uses home dir", func(t *testing.T) {
		resetSetupSeams(t)
		userHomeDir = func() (string, error) { return "/test/home", nil }

		got := claudeCodeSettingsPath()
		expected := filepath.Join("/test/home", ".claude", "settings.json")
		if got != expected {
			t.Fatalf("expected %q, got %q", expected, got)
		}
	})
}

// ─── Issue #18: opencode.jsonc regression tests ─────────────────────────────

func TestStripJSONC(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no comments", `{"key":"value"}`, `{"key":"value"}`},
		{"single line comment", "{\n// comment\n\"key\":\"value\"}", "{\n\n\"key\":\"value\"}"},
		{"multi line comment", "{/* block */\"key\":\"value\"}", "{\"key\":\"value\"}"},
		{"comment inside string preserved", `{"key":"val // not a comment"}`, `{"key":"val // not a comment"}`},
		{"escaped quote in string", `{"key":"val\"ue"}`, `{"key":"val\"ue"}`},
		{"trailing single-line comment", "{\"key\":\"value\" // inline\n}", "{\"key\":\"value\" \n}"},
		{"empty input", "", ""},
		{"only comments", "// nothing here\n/* also nothing */", "\n"},
		{"comment at EOF without newline", "{\"a\":1}// trailing", "{\"a\":1}"},
		{"unterminated multi-line comment", "{\"a\":1}/* never closed", "{\"a\":1}"},
		{"block comment with stars", "{/* ** fancy ** */\"a\":1}", "{\"a\":1}"},
		{"multi-line block comment preserves newlines", "{\n/* line1\nline2 */\n\"a\":1}", "{\n\n\"a\":1}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(stripJSONC([]byte(tt.input)))
			if got != tt.want {
				t.Fatalf("stripJSONC(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestOpenCodeConfigPathPrefersJSONC(t *testing.T) {
	resetSetupSeams(t)
	home := useTestHome(t)
	runtimeGOOS = "linux"
	t.Setenv("XDG_CONFIG_HOME", "")

	// When .jsonc exists, return .jsonc path
	statFn = func(name string) (os.FileInfo, error) {
		if strings.HasSuffix(name, "opencode.jsonc") {
			return nil, nil // exists
		}
		return nil, os.ErrNotExist
	}

	got := openCodeConfigPath()
	expected := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestOpenCodeConfigPathFallsBackToJSON(t *testing.T) {
	resetSetupSeams(t)
	home := useTestHome(t)
	runtimeGOOS = "linux"
	t.Setenv("XDG_CONFIG_HOME", "")

	// When .jsonc does NOT exist, return .json path
	statFn = func(name string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}

	got := openCodeConfigPath()
	expected := filepath.Join(home, ".config", "opencode", "opencode.json")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestOpenCodeConfigPathXDGWithJSONC(t *testing.T) {
	resetSetupSeams(t)
	_ = useTestHome(t)
	runtimeGOOS = "linux"
	t.Setenv("XDG_CONFIG_HOME", "/custom/xdg")

	statFn = func(name string) (os.FileInfo, error) {
		if strings.HasSuffix(name, "opencode.jsonc") {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}

	got := openCodeConfigPath()
	expected := filepath.Join("/custom/xdg", "opencode", "opencode.jsonc")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestOpenCodeConfigPathWindowsWithJSONC(t *testing.T) {
	resetSetupSeams(t)
	home := useTestHome(t)
	runtimeGOOS = "windows"
	t.Setenv("APPDATA", "C:/Users/test/AppData/Roaming")
	t.Setenv("XDG_CONFIG_HOME", "")

	statFn = func(name string) (os.FileInfo, error) {
		if strings.HasSuffix(name, "opencode.jsonc") {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}

	got := openCodeConfigPath()
	// OpenCode uses ~/.config/opencode/ on all platforms, not %APPDATA%
	expected := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestInjectOpenCodeMCPHandlesJSONC(t *testing.T) {
	resetSetupSeams(t)
	home := useTestHome(t)
	runtimeGOOS = "linux"
	t.Setenv("XDG_CONFIG_HOME", "")

	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create a .jsonc file with comments
	jsoncPath := filepath.Join(configDir, "opencode.jsonc")
	content := `{
  // This is a comment
  "theme": "kanagawa",
  "mcp": {
    /* existing server */
    "other": {"type": "local", "command": ["foo"]}
  }
}`
	if err := os.WriteFile(jsoncPath, []byte(content), 0644); err != nil {
		t.Fatalf("write jsonc: %v", err)
	}

	// statFn should find the .jsonc file
	statFn = os.Stat

	if err := injectOpenCodeMCP(); err != nil {
		t.Fatalf("injectOpenCodeMCP with JSONC failed: %v", err)
	}

	// Verify engram was added to the .jsonc file
	raw, err := os.ReadFile(jsoncPath)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("result should be valid JSON: %v", err)
	}
	mcp, ok := cfg["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcp object in result")
	}
	if _, ok := mcp["engram"]; !ok {
		t.Fatalf("expected engram to be registered")
	}
	if _, ok := mcp["other"]; !ok {
		t.Fatalf("expected existing 'other' entry to be preserved")
	}
}

// ─── Issue #112: OpenCode MCP absolute-path config ───────────────────────────

// TestInjectOpenCodeMCPUsesResolvedCommand verifies that injectOpenCodeMCP()
// writes the correct command based on the OS:
//   - Windows: absolute path from os.Executable() so headless MCP subprocesses
//     don't need PATH.
//   - Unix: bare "engram" (PATH is reliably inherited by child processes).
func TestInjectOpenCodeMCPUsesResolvedCommand(t *testing.T) {
	t.Run("windows writes absolute path in command array", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "windows"
		osExecutable = func() (string, error) { return `C:\Users\user\bin\engram.exe`, nil }
		t.Setenv("XDG_CONFIG_HOME", "")

		configDir := filepath.Join(home, ".config", "opencode")
		if err := os.MkdirAll(configDir, 0755); err != nil {
			t.Fatalf("mkdir config dir: %v", err)
		}

		if err := injectOpenCodeMCP(); err != nil {
			t.Fatalf("injectOpenCodeMCP failed: %v", err)
		}

		raw, err := os.ReadFile(filepath.Join(configDir, "opencode.json"))
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		var cfg map[string]any
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("parse config: %v", err)
		}
		mcp := cfg["mcp"].(map[string]any)
		engram := mcp["engram"].(map[string]any)
		cmd := engram["command"].([]any)
		if len(cmd) == 0 {
			t.Fatalf("expected non-empty command array")
		}
		first := cmd[0].(string)
		if first == "engram" {
			t.Fatalf("expected absolute path on windows, got bare 'engram'")
		}
		if !strings.Contains(first, "engram") {
			t.Fatalf("expected engram in command path, got %q", first)
		}
	})

	t.Run("linux writes bare engram in command array", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "linux"
		t.Setenv("XDG_CONFIG_HOME", "")

		configDir := filepath.Join(home, ".config", "opencode")
		if err := os.MkdirAll(configDir, 0755); err != nil {
			t.Fatalf("mkdir config dir: %v", err)
		}

		if err := injectOpenCodeMCP(); err != nil {
			t.Fatalf("injectOpenCodeMCP failed: %v", err)
		}

		raw, err := os.ReadFile(filepath.Join(configDir, "opencode.json"))
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		var cfg map[string]any
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("parse config: %v", err)
		}
		mcp := cfg["mcp"].(map[string]any)
		engram := mcp["engram"].(map[string]any)
		cmd := engram["command"].([]any)
		if len(cmd) == 0 {
			t.Fatalf("expected non-empty command array")
		}
		if got := cmd[0].(string); got != "engram" {
			t.Fatalf("expected bare 'engram' on linux, got %q", got)
		}
		// Remaining args should be the MCP flags
		if len(cmd) != 3 || cmd[1] != "mcp" || cmd[2] != "--tools=agent" {
			t.Fatalf("expected args [engram mcp --tools=agent], got %v", cmd)
		}
	})

	t.Run("darwin writes bare engram in command array", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "darwin"
		t.Setenv("XDG_CONFIG_HOME", "")

		configDir := filepath.Join(home, ".config", "opencode")
		if err := os.MkdirAll(configDir, 0755); err != nil {
			t.Fatalf("mkdir config dir: %v", err)
		}

		if err := injectOpenCodeMCP(); err != nil {
			t.Fatalf("injectOpenCodeMCP failed: %v", err)
		}

		raw, err := os.ReadFile(filepath.Join(configDir, "opencode.json"))
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		var cfg map[string]any
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("parse config: %v", err)
		}
		mcp := cfg["mcp"].(map[string]any)
		engram := mcp["engram"].(map[string]any)
		cmd := engram["command"].([]any)
		if len(cmd) == 0 {
			t.Fatalf("expected non-empty command array")
		}
		if got := cmd[0].(string); got != "engram" {
			t.Fatalf("expected bare 'engram' on darwin, got %q", got)
		}
	})

	t.Run("windows executable error falls back to bare engram", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "windows"
		osExecutable = func() (string, error) { return "", errors.New("no executable") }
		t.Setenv("XDG_CONFIG_HOME", "")

		configDir := filepath.Join(home, ".config", "opencode")
		if err := os.MkdirAll(configDir, 0755); err != nil {
			t.Fatalf("mkdir config dir: %v", err)
		}

		if err := injectOpenCodeMCP(); err != nil {
			t.Fatalf("injectOpenCodeMCP failed: %v", err)
		}

		raw, err := os.ReadFile(filepath.Join(configDir, "opencode.json"))
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		var cfg map[string]any
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("parse config: %v", err)
		}
		mcp := cfg["mcp"].(map[string]any)
		engram := mcp["engram"].(map[string]any)
		cmd := engram["command"].([]any)
		if len(cmd) == 0 {
			t.Fatalf("expected non-empty command array")
		}
		// Should fall back gracefully to bare "engram"
		if got := cmd[0].(string); got != "engram" {
			t.Fatalf("expected fallback to bare 'engram' when os.Executable fails, got %q", got)
		}
	})
}

// TestInstallOpenCodeWarningUsesResolvedCommand verifies that when MCP injection
// fails, the warning message printed to stderr uses the resolved command rather
// than always printing bare "engram". On Windows this means the user's manual
// config snippet will contain the absolute path they actually need.
func TestInstallOpenCodeWarningUsesResolvedCommand(t *testing.T) {
	t.Run("windows warning contains absolute path", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "windows"
		osExecutable = func() (string, error) { return `C:\bin\engram.exe`, nil }
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

		// Force MCP injection to fail so the warning branch is exercised
		injectOpenCodeMCPFn = func() error {
			return errors.New("cannot write config")
		}

		// Capture stderr
		origStderr := os.Stderr
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("pipe: %v", err)
		}
		os.Stderr = w

		_, installErr := installOpenCode()
		w.Close()
		os.Stderr = origStderr

		if installErr != nil {
			t.Fatalf("installOpenCode should not fail when MCP injection is non-fatal: %v", installErr)
		}

		buf := make([]byte, 4096)
		n, _ := r.Read(buf)
		stderr := string(buf[:n])

		// The path is written via %q so backslashes are escaped in the output.
		if !strings.Contains(stderr, `engram.exe`) {
			t.Fatalf("expected absolute path in warning message, got:\n%s", stderr)
		}
		// Must NOT be the bare "engram" unquoted form
		if strings.Contains(stderr, `["engram",`) {
			t.Fatalf("expected absolute path (not bare engram) in warning message, got:\n%s", stderr)
		}
	})

	t.Run("linux warning contains bare engram", func(t *testing.T) {
		resetSetupSeams(t)
		home := useTestHome(t)
		runtimeGOOS = "linux"
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

		injectOpenCodeMCPFn = func() error {
			return errors.New("cannot write config")
		}

		origStderr := os.Stderr
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("pipe: %v", err)
		}
		os.Stderr = w

		_, installErr := installOpenCode()
		w.Close()
		os.Stderr = origStderr

		if installErr != nil {
			t.Fatalf("installOpenCode should not fail when MCP injection is non-fatal: %v", installErr)
		}

		buf := make([]byte, 4096)
		n, _ := r.Read(buf)
		stderr := string(buf[:n])

		if !strings.Contains(stderr, `"engram"`) {
			t.Fatalf("expected bare 'engram' in warning message on linux, got:\n%s", stderr)
		}
	})
}

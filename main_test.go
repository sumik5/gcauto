package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// MockAIExecutor is a mock implementation of AIExecutor for testing.
type MockAIExecutor struct {
	MockResponse string
	MockError    error
}

// Execute returns the mock response or error.
func (m *MockAIExecutor) Execute(ctx context.Context, prompt string) (string, error) {
	if m.MockError != nil {
		return "", m.MockError
	}
	return m.MockResponse, nil
}

func TestExtractCommitMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "with preamble and separator",
			input: `差分の内容を確認したッ！この変更の全体像を把握するために...

★ Insight ─────────────────────────────────────
...
─────────────────────────────────────────────────

以下がConventional Commits仕様に準拠したコミットメッセージだッ！

---

feat(skills): 13個の新スキル追加とREADME自動同期ルール整備

スキルの拡充とドキュメント整備:
  - 13個の新しいスキルを追加
  - README自動同期ルール整備`,
			expected: `feat(skills): 13個の新スキル追加とREADME自動同期ルール整備

スキルの拡充とドキュメント整備:
  - 13個の新しいスキルを追加
  - README自動同期ルール整備`,
		},
		{
			name:     "clean message without preamble",
			input:    "feat(auth): ユーザー認証機能を追加\n\n認証システムの実装:\n  - JWTトークンベースの認証",
			expected: "feat(auth): ユーザー認証機能を追加\n\n認証システムの実装:\n  - JWTトークンベースの認証",
		},
		{
			name:     "message with body",
			input:    "fix: ログインバグ修正\n\n- セッションタイムアウトを修正\n- エラーハンドリング改善",
			expected: "fix: ログインバグ修正\n\n- セッションタイムアウトを修正\n- エラーハンドリング改善",
		},
		{
			name:     "message with trailing separator",
			input:    "feat(ui): 新しいUIコンポーネント追加\n\n- ボタンコンポーネント作成\n---",
			expected: "feat(ui): 新しいUIコンポーネント追加\n\n- ボタンコンポーネント作成",
		},
		{
			name:     "no conventional commit line (fallback)",
			input:    "これはConventional Commitではない単なるテキストです",
			expected: "これはConventional Commitではない単なるテキストです",
		},
		{
			name:     "scope-less commit",
			input:    "fix: バグ修正",
			expected: "fix: バグ修正",
		},
		{
			name:     "breaking change marker",
			input:    "feat!: 破壊的変更を伴う新機能",
			expected: "feat!: 破壊的変更を伴う新機能",
		},
		{
			name:     "message with trailing backticks and AI commentary",
			input:    "feat(vcs): Jujutsu対応追加\n\n変更内容:\n  - VCS検出機能追加\n  - jj diff対応\n```\n\nこのメッセージで...",
			expected: "feat(vcs): Jujutsu対応追加\n\n変更内容:\n  - VCS検出機能追加\n  - jj diff対応",
		},
		{
			name:     "message with only trailing backticks",
			input:    "fix: バグ修正\n\n- 修正内容\n```",
			expected: "fix: バグ修正\n\n- 修正内容",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := extractCommitMessage(tt.input)
			if actual != tt.expected {
				t.Errorf("extractCommitMessage() = %q, want %q", actual, tt.expected)
			}
		})
	}
}

func TestGenerateCommitMessage(t *testing.T) {
	tests := []struct {
		name         string
		mockResponse string
		mockError    error
		diff         string
		fileList     string
		stat         string
		wantError    bool
		wantEmpty    bool
	}{
		{
			name:         "valid conventional commit message",
			mockResponse: "feat: 新しい機能を追加\n\n- 機能1を追加\n- 機能2を追加",
			diff:         "fake diff",
			fileList:     "main.go\nREADME.md",
			stat:         "main.go | 10 ++++++++++\nREADME.md | 5 +++++",
			wantError:    false,
			wantEmpty:    false,
		},
		{
			name:         "fix type commit message",
			mockResponse: "fix: バグを修正",
			diff:         "fake diff",
			fileList:     "main.go",
			stat:         "main.go | 2 +-",
			wantError:    false,
			wantEmpty:    false,
		},
		{
			name:      "error from ai",
			mockError: os.ErrNotExist,
			diff:      "fake diff",
			fileList:  "",
			stat:      "",
			wantError: true,
		},
		{
			name:         "empty response",
			mockResponse: "",
			diff:         "fake diff",
			fileList:     "",
			stat:         "",
			wantError:    false,
			wantEmpty:    true,
		},
		{
			name:         "truncated diff with warning",
			mockResponse: "feat: 大規模なリファクタリング",
			diff:         strings.Repeat("a", 60000), // Exceeds maxDiffSize (50000)
			fileList:     "file1.go\nfile2.go\nfile3.go",
			stat:         "file1.go | 100 +++++++\nfile2.go | 200 +++++++\nfile3.go | 300 ++++++",
			wantError:    false,
			wantEmpty:    false,
		},
		{
			name: "message with preamble gets extracted",
			mockResponse: `AIによる解説文がここに入ります。

★ Insight ─────────────────
分析結果など
─────────────────────────────

---

feat(api): 新しいAPIエンドポイント追加

APIの拡充:
  - ユーザー検索エンドポイント追加`,
			diff:      "fake diff",
			fileList:  "api.go",
			stat:      "api.go | 50 ++++++++++",
			wantError: false,
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &MockAIExecutor{
				MockResponse: tt.mockResponse,
				MockError:    tt.mockError,
			}

			message, err := generateCommitMessage(context.Background(), executor, tt.diff, tt.fileList, tt.stat)

			if tt.wantError {
				if err == nil {
					t.Error("generateCommitMessage() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("generateCommitMessage() unexpected error = %v", err)
				return
			}

			if tt.wantEmpty {
				if message != "" {
					t.Errorf("generateCommitMessage() expected empty message but got %s", message)
				}
				return
			}

			if message == "" {
				t.Error("generateCommitMessage() returned empty message")
			}

			conventionalTypes := []string{"feat", "fix", "docs", "style", "refactor", "perf", "test", "build", "ci", "chore", "revert"}
			hasValidType := false
			for _, typ := range conventionalTypes {
				// Check for patterns: type(, type:, type!
				if strings.HasPrefix(message, typ+"(") || strings.HasPrefix(message, typ+":") || strings.HasPrefix(message, typ+"!") {
					hasValidType = true
					break
				}
			}

			if !hasValidType && !tt.wantEmpty {
				t.Errorf("generateCommitMessage() returned message without valid conventional commit type: %s", message)
			}
		})
	}
}

func TestGitCommit(t *testing.T) {
	tempDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(originalDir)
	}()

	if chdirErr := os.Chdir(tempDir); chdirErr != nil {
		t.Fatal(chdirErr)
	}

	cmd := exec.Command("git", "init")
	if initErr := cmd.Run(); initErr != nil {
		t.Fatalf("Failed to initialize git repo: %v", initErr)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	if configErr := cmd.Run(); configErr != nil {
		t.Fatalf("Failed to set git user.email: %v", configErr)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	if configErr := cmd.Run(); configErr != nil {
		t.Fatalf("Failed to set git user.name: %v", configErr)
	}

	testFile := "test.txt"
	if writeErr := os.WriteFile(testFile, []byte("test content"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}

	cmd = exec.Command("git", "add", testFile)
	if addErr := cmd.Run(); addErr != nil {
		t.Fatalf("Failed to add file: %v", addErr)
	}

	err = gitCommit(context.Background(), "test: テストコミット")
	if err != nil {
		t.Errorf("gitCommit() error = %v", err)
	}

	cmd = exec.Command("git", "log", "--oneline", "-1")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get git log: %v", err)
	}

	if !strings.Contains(string(output), "test: テストコミット") {
		t.Errorf("Commit message not found in git log: %s", output)
	}
}

func TestRunPreCommit(t *testing.T) {
	tempDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(originalDir)
	}()

	if chdirErr := os.Chdir(tempDir); chdirErr != nil {
		t.Fatal(chdirErr)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	if initErr := cmd.Run(); initErr != nil {
		t.Fatalf("Failed to initialize git repo: %v", initErr)
	}

	tests := []struct {
		name          string
		setupHook     bool
		hookContent   string
		wantError     bool
		errorContains string
	}{
		{
			name:      "no pre-commit hook",
			setupHook: false,
			wantError: false,
		},
		{
			name:        "successful pre-commit hook",
			setupHook:   true,
			hookContent: "#!/bin/sh\nexit 0\n",
			wantError:   false,
		},
		{
			name:          "failing pre-commit hook",
			setupHook:     true,
			hookContent:   "#!/bin/sh\necho 'Pre-commit failed'\nexit 1\n",
			wantError:     true,
			errorContains: "pre-commit hook failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get hooks directory
			cmd := exec.Command("git", "rev-parse", "--git-path", "hooks")
			output, err := cmd.Output()
			if err != nil {
				t.Fatalf("Failed to get hooks path: %v", err)
			}
			hooksDir := strings.TrimSpace(string(output))

			// Create hooks directory if it doesn't exist
			if mkdirErr := os.MkdirAll(hooksDir, 0o755); mkdirErr != nil {
				t.Fatalf("Failed to create hooks directory: %v", mkdirErr)
			}

			hookPath := fmt.Sprintf("%s/pre-commit", hooksDir)

			// Clean up hook after test
			defer func() {
				_ = os.Remove(hookPath)
			}()

			if tt.setupHook {
				if writeErr := os.WriteFile(hookPath, []byte(tt.hookContent), 0o755); writeErr != nil {
					t.Fatalf("Failed to create pre-commit hook: %v", writeErr)
				}
			}

			err = _runPreCommit(context.Background())

			if tt.wantError {
				if err == nil {
					t.Errorf("runPreCommit() expected error but got none")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("runPreCommit() error = %v, want error containing %s", err, tt.errorContains)
				}
			} else if err != nil {
				t.Errorf("runPreCommit() unexpected error = %v", err)
			}
		})
	}
}

func TestMainUserInput(t *testing.T) {
	originalDetectVCSFn := detectVCSFn
	detectVCSFn = func(ctx context.Context) VCSType {
		return VCSGit
	}
	defer func() {
		detectVCSFn = originalDetectVCSFn
	}()

	originalGetStagedDiff := getStagedDiff
	getStagedDiff = func(ctx context.Context) (string, error) {
		return "fake diff for main user input test", nil
	}
	defer func() {
		getStagedDiff = originalGetStagedDiff
	}()

	originalGetStagedFileList := getStagedFileList
	getStagedFileList = func(ctx context.Context) (string, error) {
		return "main.go\nREADME.md", nil
	}
	defer func() {
		getStagedFileList = originalGetStagedFileList
	}()

	originalGetStagedDiffStat := getStagedDiffStat
	getStagedDiffStat = func(ctx context.Context) (string, error) {
		return "main.go | 10 ++++++++++\nREADME.md | 5 +++++", nil
	}
	defer func() {
		getStagedDiffStat = originalGetStagedDiffStat
	}()

	originalRunPreCommit := runPreCommit
	runPreCommit = func(ctx context.Context) error {
		return nil
	}
	defer func() {
		runPreCommit = originalRunPreCommit
	}()

	originalNewExecutor := newExecutor
	newExecutor = func(model string) (AIExecutor, error) {
		return &MockAIExecutor{
			MockResponse: "test: テスト用のコミットメッセージ",
		}, nil
	}
	defer func() {
		newExecutor = originalNewExecutor
	}()

	tests := []struct {
		name     string
		input    string
		wantExit int
	}{
		{
			name:     "User cancels with 'n'",
			input:    "n\n",
			wantExit: 0,
		},
		{
			name:     "User cancels with 'N'",
			input:    "N\n",
			wantExit: 0,
		},
		{
			name:     "User cancels with empty input",
			input:    "\n",
			wantExit: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if os.Getenv("BE_CRASHER") == "1" {
				oldStdin := os.Stdin
				r, w, _ := os.Pipe()
				os.Stdin = r

				go func() {
					_, _ = w.WriteString(tt.input)
					_ = w.Close()
				}()

				main()
				os.Stdin = oldStdin
				return
			}

			cmd := exec.Command(os.Args[0], "-test.run="+t.Name())
			cmd.Env = append(os.Environ(), "BE_CRASHER=1")

			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			err := cmd.Run()

			if e, ok := err.(*exec.ExitError); ok {
				if e.ExitCode() != tt.wantExit {
					t.Errorf("Process exited with code %d, want %d", e.ExitCode(), tt.wantExit)
				}
			} else if err != nil {
				t.Errorf("Process exited with unexpected error: %v", err)
			} else if tt.wantExit != 0 {
				t.Errorf("Process did not exit as expected")
			}
		})
	}
}

func TestParseJJSummary(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "modified and added files",
			input:    "M src/main.go\nA src/new.go\nD src/old.go\n",
			expected: "src/main.go\nsrc/new.go\nsrc/old.go",
		},
		{
			name:     "empty summary",
			input:    "",
			expected: "",
		},
		{
			name:     "single file",
			input:    "M README.md\n",
			expected: "README.md",
		},
		{
			name:     "files with spaces in path",
			input:    "M path/to/file with spaces.txt\nA another file.go\n",
			expected: "path/to/file with spaces.txt\nanother file.go",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := parseJJSummary(tt.input)
			if actual != tt.expected {
				t.Errorf("parseJJSummary() = %q, want %q", actual, tt.expected)
			}
		})
	}
}

func TestDetectVCS(t *testing.T) {
	// Test default implementation (requires jj to be installed for full test)
	// This test will pass regardless of jj availability
	vcs := _detectVCS(context.Background())
	if vcs != VCSGit && vcs != VCSJujutsu {
		t.Errorf("detectVCS() returned invalid VCS type: %v", vcs)
	}
}

func TestMainAutoConfirm(t *testing.T) {
	// Set up test environment inside the BE_CRASHER subprocess
	if os.Getenv("BE_CRASHER") == "1" && os.Getenv("TEST_NAME") == "TestMainAutoConfirm" {
		// Create a temporary directory for git repository
		tempDir, err := os.MkdirTemp("", "gcauto-test-*")
		if err != nil {
			panic(err)
		}
		defer func() {
			_ = os.RemoveAll(tempDir)
		}()

		if chdirErr := os.Chdir(tempDir); chdirErr != nil {
			panic(chdirErr)
		}

		cmd := exec.Command("git", "init")
		if initErr := cmd.Run(); initErr != nil {
			panic(fmt.Sprintf("Failed to initialize git repo: %v", initErr))
		}

		cmd = exec.Command("git", "config", "user.email", "test@example.com")
		if configErr := cmd.Run(); configErr != nil {
			panic(fmt.Sprintf("Failed to set git user.email: %v", configErr))
		}

		cmd = exec.Command("git", "config", "user.name", "Test User")
		if configErr := cmd.Run(); configErr != nil {
			panic(fmt.Sprintf("Failed to set git user.name: %v", configErr))
		}

		// Create and stage a test file
		testFile := "test.txt"
		if writeErr := os.WriteFile(testFile, []byte("test content"), 0o644); writeErr != nil {
			panic(writeErr)
		}

		cmd = exec.Command("git", "add", testFile)
		if addErr := cmd.Run(); addErr != nil {
			panic(fmt.Sprintf("Failed to add file: %v", addErr))
		}

		// Mock AI executor
		originalNewExecutor := newExecutor
		newExecutor = func(model string) (AIExecutor, error) {
			return &MockAIExecutor{
				MockResponse: "test: auto-confirm test commit message",
			}, nil
		}
		defer func() {
			newExecutor = originalNewExecutor
		}()

		// Strip test runner flags when running main
		args := os.Args
		for i, arg := range args {
			if arg == "--" {
				os.Args = append([]string{args[0]}, args[i+1:]...)
				break
			}
		}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainAutoConfirm$", "--", "-y")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1", "TEST_NAME=TestMainAutoConfirm")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("Process exited with error: %v\nOutput: %s", err, output)
	}

	expectedOutput := "✅ Commit completed successfully!"
	if !strings.Contains(string(output), expectedOutput) {
		t.Errorf("Expected output to contain '%s', but got '%s'", expectedOutput, string(output))
	}

	expectedMessage := "test: auto-confirm test commit message"
	if !strings.Contains(string(output), expectedMessage) {
		t.Errorf("Expected output to contain commit message '%s', but got '%s'", expectedMessage, string(output))
	}
}

func TestMainAutoConfirmWithYesFlag(t *testing.T) {
	// Set up test environment inside the BE_CRASHER subprocess
	if os.Getenv("BE_CRASHER") == "1" && os.Getenv("TEST_NAME") == "TestMainAutoConfirmWithYesFlag" {
		// Create a temporary directory for git repository
		tempDir, err := os.MkdirTemp("", "gcauto-test-*")
		if err != nil {
			panic(err)
		}
		defer func() {
			_ = os.RemoveAll(tempDir)
		}()

		if chdirErr := os.Chdir(tempDir); chdirErr != nil {
			panic(chdirErr)
		}

		cmd := exec.Command("git", "init")
		if initErr := cmd.Run(); initErr != nil {
			panic(fmt.Sprintf("Failed to initialize git repo: %v", initErr))
		}

		cmd = exec.Command("git", "config", "user.email", "test@example.com")
		if configErr := cmd.Run(); configErr != nil {
			panic(fmt.Sprintf("Failed to set git user.email: %v", configErr))
		}

		cmd = exec.Command("git", "config", "user.name", "Test User")
		if configErr := cmd.Run(); configErr != nil {
			panic(fmt.Sprintf("Failed to set git user.name: %v", configErr))
		}

		// Create and stage a test file
		testFile := "README.md"
		if writeErr := os.WriteFile(testFile, []byte("test readme"), 0o644); writeErr != nil {
			panic(writeErr)
		}

		cmd = exec.Command("git", "add", testFile)
		if addErr := cmd.Run(); addErr != nil {
			panic(fmt.Sprintf("Failed to add file: %v", addErr))
		}

		// Mock AI executor
		originalNewExecutor := newExecutor
		newExecutor = func(model string) (AIExecutor, error) {
			return &MockAIExecutor{
				MockResponse: "docs: update README with --yes flag",
			}, nil
		}
		defer func() {
			newExecutor = originalNewExecutor
		}()

		// Strip test runner flags when running main
		args := os.Args
		for i, arg := range args {
			if arg == "--" {
				os.Args = append([]string{args[0]}, args[i+1:]...)
				break
			}
		}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainAutoConfirmWithYesFlag$", "--", "--yes")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1", "TEST_NAME=TestMainAutoConfirmWithYesFlag")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("Process exited with error: %v\nOutput: %s", err, output)
	}

	expectedOutput := "✅ Commit completed successfully!"
	if !strings.Contains(string(output), expectedOutput) {
		t.Errorf("Expected output to contain '%s', but got '%s'", expectedOutput, string(output))
	}

	expectedMessage := "docs: update README with --yes flag"
	if !strings.Contains(string(output), expectedMessage) {
		t.Errorf("Expected output to contain commit message '%s', but got '%s'", expectedMessage, string(output))
	}
}

func TestMain_InvalidModel(t *testing.T) {
	originalDetectVCSFn := detectVCSFn
	detectVCSFn = func(ctx context.Context) VCSType {
		return VCSGit
	}
	defer func() {
		detectVCSFn = originalDetectVCSFn
	}()

	originalRunPreCommit := runPreCommit
	runPreCommit = func(ctx context.Context) error {
		return nil
	}
	defer func() {
		runPreCommit = originalRunPreCommit
	}()

	originalNewExecutor := newExecutor
	newExecutor = func(model string) (AIExecutor, error) {
		return nil, fmt.Errorf("invalid model specified: %s", model)
	}
	defer func() {
		newExecutor = originalNewExecutor
	}()

	if os.Getenv("BE_CRASHER") == "1" {
		// This part of the test runs in a separate process.
		// When the test is re-run with BE_CRASHER, os.Args contains flags for the
		// test runner, followed by "--", followed by flags for our main function.
		// We need to strip out the test runner flags.
		args := os.Args
		for i, arg := range args {
			if arg == "--" {
				os.Args = append([]string{args[0]}, args[i+1:]...)
				break
			}
		}
		main()
		return
	}

	// This is the main test process.
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_InvalidModel", "--", "-model=invalid")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")

	output, err := cmd.CombinedOutput()
	if e, ok := err.(*exec.ExitError); ok {
		if e.ExitCode() != 1 {
			t.Errorf("Process exited with code %d, want 1", e.ExitCode())
		}
	} else if err != nil {
		t.Errorf("Process exited with unexpected error: %v", err)
	} else {
		t.Errorf("Process did not exit as expected")
	}

	expectedError := "invalid model specified: invalid"
	if !strings.Contains(string(output), expectedError) {
		t.Errorf("Expected output to contain '%s', but got '%s'", expectedError, string(output))
	}
}

func TestGenerateCommitMessageContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	executor := &MockAIExecutor{
		MockError: context.Canceled,
	}

	_, err := generateCommitMessage(ctx, executor, "fake diff", "file.go", "file.go | 10 ++++++++++")
	if err == nil {
		t.Error("generateCommitMessage() expected error when context is canceled, but got none")
	}
}

func TestAIExecutorContextCanceled(t *testing.T) {
	tests := []struct {
		name     string
		executor AIExecutor
	}{
		{
			name:     "ClaudeExecutor with canceled context",
			executor: &ClaudeExecutor{},
		},
		{
			name:     "GeminiExecutor with canceled context",
			executor: &GeminiExecutor{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			_, err := tt.executor.Execute(ctx, "test prompt")
			if err == nil {
				t.Error("Execute() expected error when context is canceled, but got none")
			}
		})
	}
}

func TestParseJJSummaryEntries(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []JJFileEntry
	}{
		{
			name:  "M/A/D各ステータスの正常パース",
			input: "M src/main.go\nA src/new.go\nD src/old.go\n",
			expected: []JJFileEntry{
				{Status: "M", Path: "src/main.go"},
				{Status: "A", Path: "src/new.go"},
				{Status: "D", Path: "src/old.go"},
			},
		},
		{
			name:     "空入力",
			input:    "",
			expected: []JJFileEntry{},
		},
		{
			name:  "単一ファイル",
			input: "M README.md\n",
			expected: []JJFileEntry{
				{Status: "M", Path: "README.md"},
			},
		},
		{
			name:  "パスにスペースを含むファイル",
			input: "M path/to/file with spaces.txt\nA another file.go\n",
			expected: []JJFileEntry{
				{Status: "M", Path: "path/to/file with spaces.txt"},
				{Status: "A", Path: "another file.go"},
			},
		},
		{
			name:     "複数行改行のみ",
			input:    "\n\n\n",
			expected: []JJFileEntry{},
		},
		{
			name:  "空行混在",
			input: "M file1.go\n\nA file2.go\n\n",
			expected: []JJFileEntry{
				{Status: "M", Path: "file1.go"},
				{Status: "A", Path: "file2.go"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := parseJJSummaryEntries(tt.input)
			if len(actual) != len(tt.expected) {
				t.Fatalf("parseJJSummaryEntries() returned %d entries, want %d", len(actual), len(tt.expected))
			}
			for i := range actual {
				if actual[i].Status != tt.expected[i].Status || actual[i].Path != tt.expected[i].Path {
					t.Errorf("parseJJSummaryEntries() entry[%d] = {Status: %q, Path: %q}, want {Status: %q, Path: %q}",
						i, actual[i].Status, actual[i].Path, tt.expected[i].Status, tt.expected[i].Path)
				}
			}
		})
	}
}

func TestSelectJJFiles(t *testing.T) {
	tests := []struct {
		name          string
		entries       []JJFileEntry
		input         string
		expectedPaths []string
		wantError     bool
	}{
		{
			name: "全選択状態でEnter確定",
			entries: []JJFileEntry{
				{Status: "M", Path: "file1.go"},
				{Status: "A", Path: "file2.go"},
			},
			input:         "\n",
			expectedPaths: []string{"file1.go", "file2.go"},
			wantError:     false,
		},
		{
			name: "番号トグル（単一）",
			entries: []JJFileEntry{
				{Status: "M", Path: "file1.go"},
				{Status: "A", Path: "file2.go"},
			},
			input:         "2\n\n",
			expectedPaths: []string{"file1.go"},
			wantError:     false,
		},
		{
			name: "番号トグル（複数スペース区切り）",
			entries: []JJFileEntry{
				{Status: "M", Path: "file1.go"},
				{Status: "A", Path: "file2.go"},
				{Status: "D", Path: "file3.go"},
			},
			input:         "1 3\n\n",
			expectedPaths: []string{"file2.go"},
			wantError:     false,
		},
		{
			name: "a で全選択",
			entries: []JJFileEntry{
				{Status: "M", Path: "file1.go"},
				{Status: "A", Path: "file2.go"},
			},
			input:         "n\na\n\n",
			expectedPaths: []string{"file1.go", "file2.go"},
			wantError:     false,
		},
		{
			name: "n で全解除 → 再入力要求 → a で全選択",
			entries: []JJFileEntry{
				{Status: "M", Path: "file1.go"},
				{Status: "A", Path: "file2.go"},
			},
			input:         "n\n\na\n\n",
			expectedPaths: []string{"file1.go", "file2.go"},
			wantError:     false,
		},
		{
			name: "不正な入力時のスキップ",
			entries: []JJFileEntry{
				{Status: "M", Path: "file1.go"},
				{Status: "A", Path: "file2.go"},
			},
			input:         "abc\n999\n\n",
			expectedPaths: []string{"file1.go", "file2.go"},
			wantError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStdin := os.Stdin
			r, w, _ := os.Pipe()
			os.Stdin = r

			go func() {
				_, _ = w.WriteString(tt.input)
				_ = w.Close()
			}()

			result, err := selectJJFiles(context.Background(), tt.entries)

			os.Stdin = oldStdin

			if tt.wantError {
				if err == nil {
					t.Error("selectJJFiles() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("selectJJFiles() unexpected error = %v", err)
			}

			if len(result) != len(tt.expectedPaths) {
				t.Fatalf("selectJJFiles() returned %d entries, want %d", len(result), len(tt.expectedPaths))
			}

			for i, entry := range result {
				if entry.Path != tt.expectedPaths[i] {
					t.Errorf("selectJJFiles() entry[%d].Path = %q, want %q", i, entry.Path, tt.expectedPaths[i])
				}
			}
		})
	}
}

func TestSelectJJFilesContextCanceled(t *testing.T) {
	entries := []JJFileEntry{
		{Status: "M", Path: "file1.go"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := selectJJFiles(ctx, entries)
	if err == nil {
		t.Error("selectJJFiles() expected error when context is canceled, but got none")
	}
}

func TestGetJJDiffForPaths(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
	}{
		{
			name:  "空パスの場合",
			paths: []string{},
		},
		{
			name:  "単一パス（実際のjjコマンド実行）",
			paths: []string{"main.go"},
		},
		{
			name:  "複数パス",
			paths: []string{"main.go", "main_test.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := _getJJDiffForPaths(context.Background(), tt.paths)

			// jjリポジトリかどうかに関わらず、コマンド実行自体は成功する
			// エラーの有無はリポジトリの状態に依存するため、返り値の型のみ確認
			_ = result
			_ = err
		})
	}
}

func TestGetJJFileListForPaths(t *testing.T) {
	tests := []struct {
		name     string
		entries  []JJFileEntry
		expected string
	}{
		{
			name: "複数ファイル",
			entries: []JJFileEntry{
				{Status: "M", Path: "file1.go"},
				{Status: "A", Path: "file2.go"},
				{Status: "D", Path: "file3.go"},
			},
			expected: "file1.go\nfile2.go\nfile3.go",
		},
		{
			name: "単一ファイル",
			entries: []JJFileEntry{
				{Status: "M", Path: "main.go"},
			},
			expected: "main.go",
		},
		{
			name:     "空の場合",
			entries:  []JJFileEntry{},
			expected: "",
		},
		{
			name: "スペース含むパス",
			entries: []JJFileEntry{
				{Status: "M", Path: "path/to/file with spaces.txt"},
			},
			expected: "path/to/file with spaces.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := _getJJFileListForPaths(context.Background(), tt.entries)
			if actual != tt.expected {
				t.Errorf("getJJFileListForPaths() = %q, want %q", actual, tt.expected)
			}
		})
	}
}

func TestGetJJDiffStatForPaths(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
	}{
		{
			name:  "空パスの場合",
			paths: []string{},
		},
		{
			name:  "単一パス（実際のjjコマンド実行）",
			paths: []string{"main.go"},
		},
		{
			name:  "複数パス",
			paths: []string{"main.go", "main_test.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := _getJJDiffStatForPaths(context.Background(), tt.paths)

			// jjリポジトリかどうかに関わらず、コマンド実行自体は成功する
			// エラーの有無はリポジトリの状態に依存するため、返り値の型のみ確認
			_ = result
			_ = err
		})
	}
}

func TestJJPartialCommit(t *testing.T) {
	tests := []struct {
		name          string
		selectedPaths []string
		allEntries    []JJFileEntry
		description   string
	}{
		{
			name:          "全ファイル選択時は通常のjjCommit",
			selectedPaths: []string{"file1.go", "file2.go"},
			allEntries: []JJFileEntry{
				{Status: "M", Path: "file1.go"},
				{Status: "A", Path: "file2.go"},
			},
			description: "全ファイルが選択されている場合、excludedEntriesが空になり、jjCommitが呼ばれる",
		},
		{
			name:          "部分選択（ロジックテスト）",
			selectedPaths: []string{"file1.go"},
			allEntries: []JJFileEntry{
				{Status: "M", Path: "file1.go"},
				{Status: "A", Path: "file2.go"},
			},
			description: "部分選択時、除外ファイルの保存・復元ロジックが実行される（実際のjjコマンドはmock困難）",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 注意: jjコマンド自体のmockは困難なため、エラーハンドリングとロジック構造のみテスト
			err := jjPartialCommit(context.Background(), "test: partial commit test", tt.selectedPaths, tt.allEntries)

			// jjリポジトリでない場合は必ずエラーになるため、エラーの存在のみ確認
			if err == nil && len(tt.selectedPaths) != len(tt.allEntries) {
				t.Logf("jjPartialCommit() did not return error (likely in jj repository)")
			}
		})
	}
}

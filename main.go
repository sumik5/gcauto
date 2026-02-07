// Package main provides gcauto, a tool that automatically generates git commit messages using AI.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// AIExecutor defines the interface for executing AI models.
type AIExecutor interface {
	Execute(prompt string) (string, error)
}

// ClaudeExecutor implements AIExecutor for the Claude model.
type ClaudeExecutor struct{}

// Execute runs the claude command with the given prompt.
func (e *ClaudeExecutor) Execute(prompt string) (string, error) {
	cmd := exec.Command("claude", "-p")
	cmd.Stdin = strings.NewReader(prompt)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("claude execution failed: %w: %s", err, string(exitErr.Stderr))
		}
		return "", fmt.Errorf("failed to run claude command: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	var filteredLines []string
	for _, line := range lines {
		if !strings.Contains(line, "🤖 Generated with") &&
			!strings.Contains(line, "Co-Authored-By: Claude") {
			filteredLines = append(filteredLines, line)
		}
	}

	return strings.TrimSpace(strings.Join(filteredLines, "\n")), nil
}

// GeminiExecutor implements AIExecutor for the Gemini model.
type GeminiExecutor struct{}

// Execute runs the gemini command with the given prompt.
func (e *GeminiExecutor) Execute(prompt string) (string, error) {
	// Assuming gemini command has a similar interface to claude.
	cmd := exec.Command("gemini", "-p", prompt)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("gemini execution failed: %w: %s", err, string(exitErr.Stderr))
		}
		return "", fmt.Errorf("failed to run gemini command: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	var filteredLines []string
	for _, line := range lines {
		if !strings.Contains(line, "Loaded cached credentials.") {
			filteredLines = append(filteredLines, line)
		}
	}

	return strings.TrimSpace(strings.Join(filteredLines, "\n")), nil
}

var newExecutor = func(model string) (AIExecutor, error) {
	switch model {
	case "claude":
		return &ClaudeExecutor{}, nil
	case "gemini":
		return &GeminiExecutor{}, nil
	default:
		return nil, fmt.Errorf("invalid model specified: %s", model)
	}
}

var version = "dev" // Can be set during build

func main() {
	model := flag.String("model", "claude", "AI model to use (claude or gemini)")
	modelShort := flag.String("m", "", "AI model to use (claude or gemini) (shorthand for -model)")
	showHelp := flag.Bool("h", false, "Show help message")
	showHelpLong := flag.Bool("help", false, "Show help message (longhand for -h)")
	showVersion := flag.Bool("version", false, "Show version information")

	flag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, "gcauto: AI-powered git commit message generator.\n\n")
		_, _ = fmt.Fprintf(os.Stderr, "Usage of gcauto:\n")
		_, _ = fmt.Fprintf(os.Stderr, "  gcauto [flags]\n\n")
		_, _ = fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *modelShort != "" {
		*model = *modelShort
	}

	if *showHelp || *showHelpLong {
		flag.Usage()
		os.Exit(0)
	}

	if *showVersion {
		fmt.Printf("gcauto version %s\n", version)
		os.Exit(0)
	}

	fmt.Printf("🚀 gcauto: Starting automatic commit process using %s...\n", *model)

	executor, err := newExecutor(*model)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}

	diff, err := getStagedDiff()
	if err != nil {
		fmt.Printf("❌ Error: Failed to get git diff: %v\n", err)
		os.Exit(1)
	}

	if diff == "" {
		fmt.Println("✅ No changes staged for commit. Nothing to do.")
		os.Exit(0)
	}

	// Run pre-commit hooks before generating commit message
	if preCommitErr := runPreCommit(); preCommitErr != nil {
		fmt.Printf("\n❌ Pre-commit hook failed: %v\n", preCommitErr)
		fmt.Println("\nPlease fix the issues and try again.")
		os.Exit(1)
	}

	// Get diff again in case pre-commit hooks modified files
	diff, err = getStagedDiff()
	if err != nil {
		fmt.Printf("❌ Error: Failed to get git diff after pre-commit: %v\n", err)
		os.Exit(1)
	}

	if diff == "" {
		fmt.Println("✅ No changes staged for commit after pre-commit hooks. Nothing to do.")
		os.Exit(0)
	}

	// Get file list and stat (non-fatal if these fail)
	fileList, fileListErr := getStagedFileList()
	if fileListErr != nil {
		fmt.Printf("⚠️ Warning: Failed to get staged file list: %v\n", fileListErr)
		fileList = ""
	}

	stat, statErr := getStagedDiffStat()
	if statErr != nil {
		fmt.Printf("⚠️ Warning: Failed to get diff stat: %v\n", statErr)
		stat = ""
	}

	commitMessage, err := generateCommitMessage(executor, diff, fileList, stat)
	if err != nil {
		fmt.Printf("❌ Error: Failed to generate commit message: %v\n", err)
		os.Exit(1)
	}

	// Check for common error responses from AI
	if commitMessage == "" {
		fmt.Println("❌ Error: Commit message is empty")
		os.Exit(1)
	}

	// Handle error responses from AI
	lowerMsg := strings.ToLower(commitMessage)
	if strings.Contains(lowerMsg, "execution error") ||
		strings.Contains(lowerMsg, "error:") ||
		strings.Contains(lowerMsg, "failed") {
		fmt.Printf("❌ Error: AI returned an error response: %s\n", commitMessage)
		fmt.Println("\nPossible causes:")
		fmt.Println("  - The diff might be too large")
		fmt.Println("  - The claude CLI might not be properly configured")
		fmt.Println("  - Try staging fewer files or use --model gemini")
		os.Exit(1)
	}

	// Loop for confirmation with edit option
	for {
		fmt.Println("\n📝 Generated Commit Message:")
		fmt.Println("===================================")
		fmt.Println(commitMessage)
		fmt.Println("===================================")

		fmt.Print("\nDo you want to commit with this message? [y/N/e]: ")
		fmt.Print("\n  y/yes - Commit with this message")
		fmt.Print("\n  n/no  - Cancel commit")
		fmt.Print("\n  e/edit - Edit message in your editor")
		fmt.Print("\n\nYour choice: ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("❌ Error: Failed to read input: %v\n", err)
			os.Exit(1)
		}

		response = strings.TrimSpace(strings.ToLower(response))

		switch response {
		case "y", "yes":
			if err := gitCommit(commitMessage); err != nil {
				fmt.Printf("\n❌ Commit failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("\n✅ Commit completed successfully!")
			return
		case "e", "edit":
			editedMessage, err := editMessageInEditor(commitMessage)
			if err != nil {
				fmt.Printf("\n❌ Error editing message: %v\n", err)
				fmt.Println("Keeping original message...")
				continue
			}
			if editedMessage == "" {
				fmt.Println("\n⚠️ Empty message, keeping original...")
				continue
			}
			commitMessage = editedMessage
			fmt.Println("\n✏️ Message updated!")
			continue
		case "n", "no", "":
			fmt.Println("\n⏹️ Commit canceled.")
			os.Exit(0)
		default:
			fmt.Println("\n⚠️ Invalid choice. Please enter y, n, or e.")
		}
	}
}

func generateCommitMessage(executor AIExecutor, diff string, fileList string, stat string) (string, error) {
	// Limit diff size to prevent issues with command line argument limits
	maxDiffSize := 50000
	truncatedDiff := diff
	wasTruncated := false
	if len(diff) > maxDiffSize {
		truncatedDiff = diff[:maxDiffSize] + "\n...(diff truncated for size)..."
		wasTruncated = true
	}

	truncationNote := ""
	if wasTruncated {
		truncationNote = "\n注意: 差分が大きいため一部省略されています。ファイル一覧と変更統計を参考に、全体像を把握してください。"
	}

	prompt := fmt.Sprintf(`以下のgitの差分情報に基づいて、Conventional Commits仕様に準拠したコミットメッセージを生成してください。

変更ファイル一覧:
---
%s
---

変更統計:
---
%s
---
%s
差分:
---
%s
---

Conventional Commits仕様 (https://www.conventionalcommits.org/ja/v1.0.0/):
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]

コミットタイプの選択基準：
- feat: 新機能の追加
- fix: バグ修正
- docs: ドキュメントのみの変更
- style: コードの意味に影響しない変更（空白、フォーマット、セミコロンの欠落など）
- refactor: バグ修正でも機能追加でもないコード変更
- perf: パフォーマンス改善のためのコード変更
- test: テストの追加や修正
- build: ビルドシステムや外部依存関係に影響する変更
- ci: CI設定ファイルとスクリプトへの変更
- chore: その他の変更（srcやtestフォルダーの変更を含まない）
- revert: 以前のコミットを取り消す

生成ルール：
1. 変更内容から最も適切なタイプを自動判定
2. scopeは変更された主要なモジュール/コンポーネントがあれば括弧内に含める
3. descriptionは50文字以内で変更内容を簡潔に要約（日本語可）
4. bodyでは箇条書きを使う場合、「  - 」（スペース2つ + ハイフン + スペース）でインデント
5. 破壊的変更がある場合は、フッターに「BREAKING CHANGE:」を記載し、次の行から「  - 」形式で詳細を記載

フォーマット例：
feat(auth): ユーザー認証機能を追加

認証システムの実装:
  - JWTトークンベースの認証
  - リフレッシュトークン機能
  - セッション管理の改善

BREAKING CHANGE:
  - 認証APIのエンドポイントが/api/authから/api/v2/authに変更
  - 旧形式のトークンは無効になります

重要な注意事項：
- 絶対に最初の行（<type>行）より前に説明文を付けない
- コミットメッセージ本文のみを出力（説明や前置きは一切不要）
- バッククォート（三つの連続したバッククォート）やコードブロック記号は使用禁止
- マークダウン記法は使用せず、プレーンテキストとして出力`, fileList, stat, truncationNote, truncatedDiff)

	return executor.Execute(prompt)
}

func editMessageInEditor(originalMessage string) (string, error) {
	// Get the editor from environment variable, default to vi
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// Create a temporary file
	tmpfile, err := os.CreateTemp("", "gcauto-*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpfileName := tmpfile.Name()
	defer func() {
		// nolint:errcheck // Best-effort cleanup in defer
		_ = os.Remove(tmpfileName)
	}()

	// Write the original message to the file
	if _, writeErr := tmpfile.WriteString(originalMessage); writeErr != nil {
		// nolint:errcheck // Already handling write error
		_ = tmpfile.Close()
		return "", fmt.Errorf("failed to write to temporary file: %w", writeErr)
	}
	if closeErr := tmpfile.Close(); closeErr != nil {
		return "", fmt.Errorf("failed to close temporary file: %w", closeErr)
	}

	// Open the editor
	// #nosec G204 - editor is from environment variable, which is expected behavior
	cmd := exec.Command(editor, tmpfileName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if runErr := cmd.Run(); runErr != nil {
		return "", fmt.Errorf("failed to run editor: %w", runErr)
	}

	// Read the edited content
	editedContent, readErr := os.ReadFile(tmpfileName)
	if readErr != nil {
		return "", fmt.Errorf("failed to read edited file: %w", readErr)
	}

	return strings.TrimSpace(string(editedContent)), nil
}

func gitCommit(message string) error {
	// Use --no-verify to skip pre-commit hooks since we already ran them
	cmd := exec.Command("git", "commit", "--no-verify", "-m", message)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func _runPreCommit() error {
	// Get git repository root directory first
	rootCmd := exec.Command("git", "rev-parse", "--show-toplevel")
	rootOutput, err := rootCmd.Output()
	if err != nil {
		// Not in a git repository
		return nil
	}
	rootDir := strings.TrimSpace(string(rootOutput))

	// Check if .pre-commit-config.yaml exists in repository root
	configPath := rootDir + "/.pre-commit-config.yaml"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// No pre-commit configuration file, check for git hook
		cmd := exec.Command("git", "rev-parse", "--git-path", "hooks/pre-commit")
		output, hookErr := cmd.Output()
		if hookErr != nil {
			// No pre-commit available, skip silently
			return nil
		}

		hookPath := strings.TrimSpace(string(output))
		if _, statErr := os.Stat(hookPath); os.IsNotExist(statErr) {
			// No pre-commit hook exists, skip silently
			return nil
		}

		// Git hook exists but no config file, run the hook directly
		fmt.Println("\n🔍 Running pre-commit hook...")
		hookCmd := exec.Command(hookPath)
		hookCmd.Stdout = os.Stdout
		hookCmd.Stderr = os.Stderr
		hookCmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+os.Getenv("GIT_INDEX_FILE"))

		if runErr := hookCmd.Run(); runErr != nil {
			return fmt.Errorf("pre-commit hook failed: %w", runErr)
		}

		fmt.Println("✅ Pre-commit hook passed!")
		return nil
	}

	// .pre-commit-config.yaml exists, check if pre-commit command is available
	if _, err := exec.LookPath("pre-commit"); err != nil {
		// pre-commit command not installed but config exists
		fmt.Println("\n⚠️  .pre-commit-config.yaml found but pre-commit is not installed")
		fmt.Println("   Skipping pre-commit hooks. Install with: pip install pre-commit")
		return nil
	}

	// Run pre-commit on staged files
	fmt.Println("\n🔍 Running pre-commit hooks...")

	// Get list of staged files
	stagedCmd := exec.Command("git", "diff", "--cached", "--name-only", "--diff-filter=ACM")
	stagedCmd.Dir = rootDir
	stagedOutput, err := stagedCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get staged files: %w", err)
	}

	stagedFiles := strings.Split(strings.TrimSpace(string(stagedOutput)), "\n")
	if len(stagedFiles) == 0 || (len(stagedFiles) == 1 && stagedFiles[0] == "") {
		fmt.Println("✅ No staged files to check")
		return nil
	}

	// Run pre-commit with explicit file list
	args := append([]string{"run", "--files"}, stagedFiles...)
	cmd := exec.Command("pre-commit", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = rootDir

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pre-commit hooks failed: %w", err)
	}

	fmt.Println("✅ Pre-commit hooks passed!")
	return nil
}

var runPreCommit = _runPreCommit

func _getStagedDiff() (string, error) {
	cmd := exec.Command("git", "diff", "--staged")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

var getStagedDiff = _getStagedDiff

func _getStagedFileList() (string, error) {
	cmd := exec.Command("git", "diff", "--staged", "--name-only")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

var getStagedFileList = _getStagedFileList

func _getStagedDiffStat() (string, error) {
	cmd := exec.Command("git", "diff", "--staged", "--stat")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

var getStagedDiffStat = _getStagedDiffStat

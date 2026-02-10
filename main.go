// Package main provides gcauto, a tool that automatically generates git commit messages using AI.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

// VCSType represents the version control system type.
type VCSType int

const (
	VCSGit VCSType = iota
	VCSJujutsu
)

// JJFileEntry represents a file entry from jj diff --summary.
type JJFileEntry struct {
	Status string // "M", "A", "D"
	Path   string
}

// AIExecutor defines the interface for executing AI models.
type AIExecutor interface {
	Execute(ctx context.Context, prompt string) (string, error)
}

// ClaudeExecutor implements AIExecutor for the Claude model.
type ClaudeExecutor struct{}

// Execute runs the claude command with the given prompt.
func (e *ClaudeExecutor) Execute(ctx context.Context, prompt string) (string, error) {
	cmd := exec.CommandContext(ctx, "claude", "-p")
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
func (e *GeminiExecutor) Execute(ctx context.Context, prompt string) (string, error) {
	// Assuming gemini command has a similar interface to claude.
	cmd := exec.CommandContext(ctx, "gemini", "-p", prompt)
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

// _detectVCS detects the version control system type.
// It checks if the current directory is a Jujutsu repository by running "jj root".
func _detectVCS(ctx context.Context) VCSType {
	cmd := exec.CommandContext(ctx, "jj", "root")
	if err := cmd.Run(); err != nil {
		return VCSGit
	}
	return VCSJujutsu
}

var detectVCSFn = _detectVCS

var version = "dev" // Can be set during build

func main() {
	model := flag.String("model", "claude", "AI model to use (claude or gemini)")
	modelShort := flag.String("m", "", "AI model to use (claude or gemini) (shorthand for -model)")
	showHelp := flag.Bool("h", false, "Show help message")
	showHelpLong := flag.Bool("help", false, "Show help message (longhand for -h)")
	showVersion := flag.Bool("version", false, "Show version information")
	yesShort := flag.Bool("y", false, "Automatically confirm and commit without prompting")
	yesLong := flag.Bool("yes", false, "Automatically confirm and commit without prompting (longhand for -y)")

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

	autoConfirm := *yesShort || *yesLong

	if *showHelp || *showHelpLong {
		flag.Usage()
		os.Exit(0)
	}

	if *showVersion {
		fmt.Printf("gcauto version %s\n", version)
		os.Exit(0)
	}

	// Signal handling: create context that cancels on SIGINT/SIGTERM
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("🚀 gcauto: Starting automatic commit process using %s...\n", *model)

	executor, err := newExecutor(*model)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		cancel()   // Cleanup before exit
		os.Exit(1) // nolint:gocritic // cancel() is explicitly called before exit
	}

	// VCS detection
	vcs := detectVCSFn(ctx)

	// Select VCS-specific functions
	var getDiff func(context.Context) (string, error)
	var getFileList func(context.Context) (string, error)
	var getDiffStat func(context.Context) (string, error)
	var commitFn func(context.Context, string) error

	switch vcs {
	case VCSJujutsu:
		fmt.Println("📦 Detected Jujutsu repository")

		// For Jujutsu in non-autoConfirm mode, allow file selection
		if !autoConfirm {
			// Get file list first
			summaryCmd := exec.CommandContext(ctx, "jj", "diff", "--summary")
			summaryOutput, summaryErr := summaryCmd.Output()
			if summaryErr != nil {
				fmt.Printf("❌ Error: Failed to get file list: %v\n", summaryErr)
				cancel()
				os.Exit(1)
			}

			allEntries := parseJJSummaryEntries(string(summaryOutput))

			// If there are 2 or more files, allow selection
			if len(allEntries) >= 2 {
				selectedEntries, selectErr := selectJJFiles(ctx, allEntries)
				if selectErr != nil {
					if ctx.Err() != nil {
						fmt.Println("\n⏹️ Interrupted. Cleaning up...")
						cancel()
						os.Exit(1)
					}
					fmt.Printf("❌ Error: Failed to select files: %v\n", selectErr)
					cancel()
					os.Exit(1)
				}

				// Check if all files are selected
				if len(selectedEntries) == len(allEntries) {
					// All files selected - use regular commit flow
					getDiff = getJJDiff
					getFileList = getJJFileList
					getDiffStat = getJJDiffStat
					commitFn = jjCommit
				} else {
					// Partial selection - use filtered functions
					selectedPaths := make([]string, len(selectedEntries))
					for i, entry := range selectedEntries {
						selectedPaths[i] = entry.Path
					}

					getDiff = func(ctx context.Context) (string, error) {
						return getJJDiffForPaths(ctx, selectedPaths)
					}
					getFileList = func(ctx context.Context) (string, error) {
						return getJJFileListForPaths(ctx, selectedEntries), nil
					}
					getDiffStat = func(ctx context.Context) (string, error) {
						return getJJDiffStatForPaths(ctx, selectedPaths)
					}
					commitFn = func(ctx context.Context, message string) error {
						return jjPartialCommit(ctx, message, selectedPaths, allEntries)
					}

					fmt.Printf("\n🤖 Generating commit message for %d selected files...\n", len(selectedEntries))
				}
			} else {
				// 0 or 1 file - use regular commit flow
				getDiff = getJJDiff
				getFileList = getJJFileList
				getDiffStat = getJJDiffStat
				commitFn = jjCommit
			}
		} else {
			// Auto-confirm mode - use regular commit flow
			getDiff = getJJDiff
			getFileList = getJJFileList
			getDiffStat = getJJDiffStat
			commitFn = jjCommit
		}
	case VCSGit:
		fmt.Println("📦 Detected Git repository")
		getDiff = getStagedDiff
		getFileList = getStagedFileList
		getDiffStat = getStagedDiffStat
		commitFn = gitCommit
	}

	diff, err := getDiff(ctx)
	if err != nil {
		if ctx.Err() != nil {
			fmt.Println("\n⏹️ Interrupted. Cleaning up...")
			cancel()
			os.Exit(1)
		}
		fmt.Printf("❌ Error: Failed to get diff: %v\n", err)
		cancel()
		os.Exit(1)
	}

	if diff == "" {
		if vcs == VCSJujutsu {
			fmt.Println("✅ No changes in current working copy. Nothing to do.")
		} else {
			fmt.Println("✅ No changes staged for commit. Nothing to do.")
		}
		cancel()
		os.Exit(0)
	}

	// Run pre-commit hooks before generating commit message (only for Git)
	if vcs == VCSGit {
		if preCommitErr := runPreCommit(ctx); preCommitErr != nil {
			if ctx.Err() != nil {
				fmt.Println("\n⏹️ Interrupted. Cleaning up...")
				cancel()
				os.Exit(1)
			}
			fmt.Printf("\n❌ Pre-commit hook failed: %v\n", preCommitErr)
			fmt.Println("\nPlease fix the issues and try again.")
			cancel()
			os.Exit(1)
		}

		// Get diff again in case pre-commit hooks modified files
		diff, err = getDiff(ctx)
		if err != nil {
			if ctx.Err() != nil {
				fmt.Println("\n⏹️ Interrupted. Cleaning up...")
				cancel()
				os.Exit(1)
			}
			fmt.Printf("❌ Error: Failed to get diff after pre-commit: %v\n", err)
			cancel()
			os.Exit(1)
		}

		if diff == "" {
			fmt.Println("✅ No changes staged for commit after pre-commit hooks. Nothing to do.")
			cancel()
			os.Exit(0)
		}
	}

	// Get file list and stat (non-fatal if these fail)
	fileList, fileListErr := getFileList(ctx)
	if fileListErr != nil {
		if ctx.Err() != nil {
			fmt.Println("\n⏹️ Interrupted. Cleaning up...")
			cancel()
			os.Exit(1)
		}
		fmt.Printf("⚠️ Warning: Failed to get file list: %v\n", fileListErr)
		fileList = ""
	}

	stat, statErr := getDiffStat(ctx)
	if statErr != nil {
		if ctx.Err() != nil {
			fmt.Println("\n⏹️ Interrupted. Cleaning up...")
			cancel()
			os.Exit(1)
		}
		fmt.Printf("⚠️ Warning: Failed to get diff stat: %v\n", statErr)
		stat = ""
	}

	commitMessage, err := generateCommitMessage(ctx, executor, diff, fileList, stat)
	if err != nil {
		if ctx.Err() != nil {
			fmt.Println("\n⏹️ Interrupted. Cleaning up...")
			cancel()
			os.Exit(1)
		}
		fmt.Printf("❌ Error: Failed to generate commit message: %v\n", err)
		cancel()
		os.Exit(1)
	}

	// Check for common error responses from AI
	if commitMessage == "" {
		fmt.Println("❌ Error: Commit message is empty")
		cancel()
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
		cancel()
		os.Exit(1)
	}

	// Auto-confirm mode: commit without prompting
	if autoConfirm {
		fmt.Println("\n📝 Generated Commit Message:")
		fmt.Println("===================================")
		fmt.Println(commitMessage)
		fmt.Println("===================================")
		if err := commitFn(ctx, commitMessage); err != nil {
			if ctx.Err() != nil {
				fmt.Println("\n⏹️ Interrupted. Cleaning up...")
				cancel()
				os.Exit(1)
			}
			fmt.Printf("\n❌ Commit failed: %v\n", err)
			cancel()
			os.Exit(1)
		}
		fmt.Println("\n✅ Commit completed successfully!")
		return
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
			cancel()
			os.Exit(1)
		}

		response = strings.TrimSpace(strings.ToLower(response))

		switch response {
		case "y", "yes":
			if err := commitFn(ctx, commitMessage); err != nil {
				if ctx.Err() != nil {
					fmt.Println("\n⏹️ Interrupted. Cleaning up...")
					cancel()
					os.Exit(1)
				}
				fmt.Printf("\n❌ Commit failed: %v\n", err)
				cancel()
				os.Exit(1)
			}
			fmt.Println("\n✅ Commit completed successfully!")
			return
		case "e", "edit":
			editedMessage, err := editMessageInEditor(ctx, commitMessage)
			if err != nil {
				if ctx.Err() != nil {
					fmt.Println("\n⏹️ Interrupted. Cleaning up...")
					cancel()
					os.Exit(1)
				}
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
			cancel()
			os.Exit(0)
		default:
			fmt.Println("\n⚠️ Invalid choice. Please enter y, n, or e.")
		}
	}
}

func extractCommitMessage(raw string) string {
	lines := strings.Split(raw, "\n")
	conventionalTypes := []string{"feat", "fix", "docs", "style", "refactor", "perf", "test", "build", "ci", "chore", "revert"}

	startIndex := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		for _, typ := range conventionalTypes {
			// Check for patterns: type(scope):, type:, type!
			if strings.HasPrefix(trimmed, typ+"(") || strings.HasPrefix(trimmed, typ+":") || strings.HasPrefix(trimmed, typ+"!") {
				startIndex = i
				break
			}
		}
		if startIndex != -1 {
			break
		}
	}

	// Fallback: return original if no conventional commit line found
	if startIndex == -1 {
		return raw
	}

	// Extract from the conventional commit line onwards, but stop at code block end markers
	var extractedLines []string
	for _, line := range lines[startIndex:] {
		trimmedLine := strings.TrimSpace(line)
		// バッククォート3つのみの行で切断（AIがコードブロック終端として出力したもの）
		if trimmedLine == "```" {
			break
		}
		extractedLines = append(extractedLines, line)
	}
	extracted := strings.Join(extractedLines, "\n")

	// Trim trailing "---" and empty lines
	extracted = strings.TrimSpace(extracted)
	for strings.HasSuffix(extracted, "---") {
		extracted = strings.TrimSuffix(extracted, "---")
		extracted = strings.TrimSpace(extracted)
	}

	return extracted
}

func generateCommitMessage(ctx context.Context, executor AIExecutor, diff, fileList, stat string) (string, error) {
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

	prompt := fmt.Sprintf(`以下の差分情報に基づいて、Conventional Commits仕様に準拠したコミットメッセージを生成してください。

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

	raw, err := executor.Execute(ctx, prompt)
	if err != nil {
		return "", err
	}
	return extractCommitMessage(raw), nil
}

func editMessageInEditor(ctx context.Context, originalMessage string) (string, error) {
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
	cmd := exec.CommandContext(ctx, editor, tmpfileName)
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

func gitCommit(ctx context.Context, message string) error {
	// Use --no-verify to skip pre-commit hooks since we already ran them
	cmd := exec.CommandContext(ctx, "git", "commit", "--no-verify", "-m", message)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func _runPreCommit(ctx context.Context) error {
	// Get git repository root directory first
	rootCmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	rootOutput, err := rootCmd.Output()
	if err != nil {
		// Not in a git repository
		return nil
	}
	rootDir := strings.TrimSpace(string(rootOutput))

	// Check if .pre-commit-config.yaml exists in repository root
	configPath := rootDir + "/.pre-commit-config.yaml"
	if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
		// No pre-commit configuration file, check for git hook
		cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-path", "hooks/pre-commit")
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
		hookCmd := exec.CommandContext(ctx, hookPath)
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
	if _, lookErr := exec.LookPath("pre-commit"); lookErr != nil {
		// pre-commit command not installed but config exists
		fmt.Println("\n⚠️  .pre-commit-config.yaml found but pre-commit is not installed")
		fmt.Println("   Skipping pre-commit hooks. Install with: pip install pre-commit")
		return nil
	}

	// Run pre-commit on staged files
	fmt.Println("\n🔍 Running pre-commit hooks...")

	// Get list of staged files
	stagedCmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--name-only", "--diff-filter=ACM")
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
	cmd := exec.CommandContext(ctx, "pre-commit", args...)
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

func _getStagedDiff(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--staged")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

var getStagedDiff = _getStagedDiff

func _getStagedFileList(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--staged", "--name-only")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

var getStagedFileList = _getStagedFileList

func _getStagedDiffStat(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--staged", "--stat")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

var getStagedDiffStat = _getStagedDiffStat

func _getJJDiff(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "jj", "diff")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

var getJJDiff = _getJJDiff

func _getJJFileList(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "jj", "diff", "--summary")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return parseJJSummary(string(output)), nil
}

var getJJFileList = _getJJFileList

// parseJJSummary parses "jj diff --summary" output to extract file names.
// Each line is like "M path/to/file" or "A path/to/file".
func parseJJSummary(summary string) string {
	lines := strings.Split(strings.TrimSpace(summary), "\n")
	var files []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "M path/to/file" or "A path/to/file" or "D path/to/file"
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			files = append(files, strings.TrimSpace(parts[1]))
		}
	}
	return strings.Join(files, "\n")
}

// parseJJSummaryEntries parses "jj diff --summary" output to extract file entries with status.
func parseJJSummaryEntries(summary string) []JJFileEntry {
	lines := strings.Split(strings.TrimSpace(summary), "\n")
	var entries []JJFileEntry
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "M path/to/file" or "A path/to/file" or "D path/to/file"
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			entries = append(entries, JJFileEntry{
				Status: strings.TrimSpace(parts[0]),
				Path:   strings.TrimSpace(parts[1]),
			})
		}
	}
	return entries
}

// selectJJFiles presents an interactive UI for selecting files to commit.
// Default is all files selected. Returns selected entries or error.
func selectJJFiles(ctx context.Context, entries []JJFileEntry) ([]JJFileEntry, error) {
	// Initialize all files as selected
	selected := make([]bool, len(entries))
	for i := range selected {
		selected[i] = true
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		// Display file list with selection status
		fmt.Println("\n📋 Changed files in working copy (@):")
		for i, entry := range entries {
			checkbox := "✅"
			if !selected[i] {
				checkbox = "❌"
			}
			fmt.Printf("  [%d] %s %s %s\n", i+1, checkbox, entry.Status, entry.Path)
		}

		fmt.Print("\nSelect files to commit (toggle: number, a: all, n: none, Enter: confirm): ")

		// Check for context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		input, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)

		// Handle empty input (Enter) - confirm selection
		if input == "" {
			// Check if at least one file is selected
			hasSelection := false
			for _, sel := range selected {
				if sel {
					hasSelection = true
					break
				}
			}
			if !hasSelection {
				fmt.Println("\n⚠️ No files selected. Please select at least one file.")
				continue
			}

			// Build result
			var result []JJFileEntry
			for i, entry := range entries {
				if selected[i] {
					result = append(result, entry)
				}
			}
			return result, nil
		}

		// Handle 'a' - select all
		if input == "a" {
			for i := range selected {
				selected[i] = true
			}
			continue
		}

		// Handle 'n' - select none
		if input == "n" {
			for i := range selected {
				selected[i] = false
			}
			continue
		}

		// Handle number(s) - toggle selection
		numbers := strings.Fields(input)
		for _, numStr := range numbers {
			var num int
			if _, scanErr := fmt.Sscanf(numStr, "%d", &num); scanErr != nil {
				fmt.Printf("⚠️ Invalid input: %s\n", numStr)
				continue
			}
			if num < 1 || num > len(entries) {
				fmt.Printf("⚠️ Number out of range: %d (valid: 1-%d)\n", num, len(entries))
				continue
			}
			// Toggle selection (1-indexed to 0-indexed)
			selected[num-1] = !selected[num-1]
		}
	}
}

// _getJJDiffForPaths gets diff for specific paths.
func _getJJDiffForPaths(ctx context.Context, paths []string) (string, error) {
	args := append([]string{"diff"}, paths...)
	cmd := exec.CommandContext(ctx, "jj", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

var getJJDiffForPaths = _getJJDiffForPaths

// _getJJFileListForPaths formats file list from entries.
func _getJJFileListForPaths(ctx context.Context, entries []JJFileEntry) string {
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	return strings.Join(paths, "\n")
}

var getJJFileListForPaths = _getJJFileListForPaths

// _getJJDiffStatForPaths gets diff stat for specific paths.
func _getJJDiffStatForPaths(ctx context.Context, paths []string) (string, error) {
	args := append([]string{"diff", "--stat"}, paths...)
	cmd := exec.CommandContext(ctx, "jj", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

var getJJDiffStatForPaths = _getJJDiffStatForPaths

// jjPartialCommit performs a partial commit by:
// 1. Saving excluded files' content
// 2. Restoring excluded files to parent state
// 3. Committing selected files
// 4. Restoring excluded files' changes
func jjPartialCommit(ctx context.Context, message string, selectedPaths []string, allEntries []JJFileEntry) error {
	// Build map of selected paths for quick lookup
	selectedMap := make(map[string]bool)
	for _, path := range selectedPaths {
		selectedMap[path] = true
	}

	// Find excluded files
	var excludedEntries []JJFileEntry
	for _, entry := range allEntries {
		if !selectedMap[entry.Path] {
			excludedEntries = append(excludedEntries, entry)
		}
	}

	// If no files are excluded, use regular commit
	if len(excludedEntries) == 0 {
		return jjCommit(ctx, message)
	}

	// Step 1: Save excluded files' content
	type savedFile struct {
		path    string
		content []byte
		deleted bool
	}
	savedFiles := make([]savedFile, 0, len(excludedEntries))

	for _, entry := range excludedEntries {
		if entry.Status == "D" {
			// Deleted file - just mark as deleted
			savedFiles = append(savedFiles, savedFile{
				path:    entry.Path,
				deleted: true,
			})
		} else {
			// Modified or Added - save content
			content, err := os.ReadFile(entry.Path)
			if err != nil {
				return fmt.Errorf("failed to read excluded file %s: %w", entry.Path, err)
			}
			savedFiles = append(savedFiles, savedFile{
				path:    entry.Path,
				content: content,
				deleted: false,
			})
		}
	}

	// Step 2: Restore excluded files to parent state
	excludedPaths := make([]string, 0, len(excludedEntries))
	for _, entry := range excludedEntries {
		excludedPaths = append(excludedPaths, entry.Path)
	}

	restoreArgs := append([]string{"restore"}, excludedPaths...)
	restoreCmd := exec.CommandContext(ctx, "jj", restoreArgs...)
	restoreCmd.Stdout = os.Stdout
	restoreCmd.Stderr = os.Stderr
	if err := restoreCmd.Run(); err != nil {
		return fmt.Errorf("failed to restore excluded files: %w", err)
	}

	// Step 3: Commit selected files
	if err := jjCommit(ctx, message); err != nil {
		// Try to restore excluded files before returning error
		fmt.Println("\n⚠️ Commit failed. Attempting to restore excluded files...")
		for _, saved := range savedFiles {
			if saved.deleted {
				// Re-delete file
				if removeErr := os.Remove(saved.path); removeErr != nil && !os.IsNotExist(removeErr) {
					fmt.Printf("⚠️ Failed to re-delete %s: %v\n", saved.path, removeErr)
				}
			} else {
				// Restore content
				if writeErr := os.WriteFile(saved.path, saved.content, 0o644); writeErr != nil {
					fmt.Printf("⚠️ Failed to restore %s: %v\n", saved.path, writeErr)
				}
			}
		}
		return fmt.Errorf("commit failed: %w", err)
	}

	// Step 4: Restore excluded files' changes (jj auto-tracks them)
	for _, saved := range savedFiles {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			fmt.Println("\n⚠️ Interrupted during file restoration. Some excluded files may not be restored.")
			fmt.Println("Please manually check and restore if needed:")
			for _, remaining := range savedFiles {
				fmt.Printf("  - %s\n", remaining.path)
			}
			return ctx.Err()
		default:
		}

		if saved.deleted {
			// Re-delete file
			if err := os.Remove(saved.path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to re-delete excluded file %s: %w (manual restoration needed)", saved.path, err)
			}
		} else {
			// Restore content
			if err := os.WriteFile(saved.path, saved.content, 0o644); err != nil {
				return fmt.Errorf("failed to restore excluded file %s: %w (manual restoration needed)", saved.path, err)
			}
		}
	}

	return nil
}

func _getJJDiffStat(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "jj", "diff", "--stat")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

var getJJDiffStat = _getJJDiffStat

func jjCommit(ctx context.Context, message string) error {
	cmd := exec.CommandContext(ctx, "jj", "commit", "-m", message)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

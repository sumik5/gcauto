// Package main provides gcauto, a tool that automatically generates git commit messages using AI.
package main

import (
	"bufio"
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
	cmd := exec.Command("claude", "-p", prompt)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// GeminiExecutor implements AIExecutor for the Gemini model.
type GeminiExecutor struct{}

// Execute runs the gemini command with the given prompt.
func (e *GeminiExecutor) Execute(prompt string) (string, error) {
	// Assuming gemini command has a similar interface to claude.
	cmd := exec.Command("gemini", "-p", prompt)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
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

	commitMessage, err := generateCommitMessage(executor, diff)
	if err != nil {
		fmt.Printf("❌ Error: Failed to generate commit message: %v\n", err)
		os.Exit(1)
	}

	if commitMessage == "" {
		fmt.Println("❌ Error: Commit message is empty")
		os.Exit(1)
	}

	fmt.Println("\n📝 Generated Commit Message:")
	fmt.Println("===================================")
	fmt.Println(commitMessage)
	fmt.Println("===================================")

	fmt.Print("\nDo you want to commit with this message? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("❌ Error: Failed to read input: %v\n", err)
		os.Exit(1)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response == "y" || response == "yes" {
		if err := gitCommit(commitMessage); err != nil {
			fmt.Printf("\n❌ Commit failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("\n✅ Commit completed successfully!")
	} else {
		fmt.Println("\n⏹️ Commit cancelled.")
		os.Exit(0)
	}
}

func generateCommitMessage(executor AIExecutor, diff string) (string, error) {
	prompt := fmt.Sprintf(`以下のgitの差分情報に基づいて、conventional commitsフォーマットで日本語のコミットメッセージを作成してください。

---
%s
---

以下の形式で直接出力してください：
型: 簡潔な変更内容

- 具体的な変更点1
- 具体的な変更点2
- 具体的な変更点3

注意事項：
- 前置きや説明文は一切含めないでください
- コミットメッセージ本文のみを出力してください
- 🤖やCo-Authored-Byなどの情報は含めないでください
- 型は feat/fix/docs/style/refactor/test/chore から適切なものを選択してください`, diff)

	return executor.Execute(prompt)
}

func gitCommit(message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func _getStagedDiff() (string, error) {
	cmd := exec.Command("git", "diff", "--staged")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

var getStagedDiff = _getStagedDiff

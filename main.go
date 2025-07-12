package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	fmt.Println("🚀 gcauto: 自動コミット処理を開始します...")

	commitMessage, err := generateCommitMessage()
	if err != nil {
		fmt.Printf("❌ エラー: コミットメッセージの生成に失敗しました: %v\n", err)
		os.Exit(1)
	}

	if commitMessage == "" {
		fmt.Println("❌ エラー: コミットメッセージが空です")
		os.Exit(1)
	}

	fmt.Println("\n📝 生成されたコミットメッセージ:")
	fmt.Println("================================")
	fmt.Println(commitMessage)
	fmt.Println("================================")

	fmt.Print("\nこのメッセージでコミットしますか？ [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("❌ エラー: 入力の読み取りに失敗しました: %v\n", err)
		os.Exit(1)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response == "y" || response == "yes" {
		if err := gitCommit(commitMessage); err != nil {
			fmt.Printf("\n❌ コミットに失敗しました: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("\n✅ コミットが正常に完了しました!")
	} else {
		fmt.Println("\n⏹️  コミットをキャンセルしました")
		os.Exit(0)
	}
}

func generateCommitMessage() (string, error) {
	prompt := `ステージングされたgitの変更を確認し、conventional commitsフォーマットで日本語のコミットメッセージを作成してください。以下の形式で出力してください：

型: 簡潔な変更内容

- 具体的な変更点1
- 具体的な変更点2
- 具体的な変更点3

注意事項：
- 🤖やCo-Authored-Byなどの情報は含めないでください
- コミットメッセージ本文のみを出力してください
- 型は feat/fix/docs/style/refactor/test/chore から適切なものを選択してください`

	cmd := exec.Command("claude", "-p", prompt)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

func gitCommit(message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

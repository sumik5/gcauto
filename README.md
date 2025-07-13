# gcauto

[![CI](https://github.com/shivase/gcauto/actions/workflows/ci.yml/badge.svg)](https://github.com/shivase/gcauto/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

自動でGitコミットメッセージを生成するCLIツール

## 概要

gcautoは、ステージングされたGitの変更を分析し、Claude AIを使用してConventional Commitsフォーマットの日本語コミットメッセージを自動生成するツールです。

## 機能

- ステージングされた変更を自動で分析
- Conventional Commitsフォーマットに準拠したコミットメッセージを生成
- 日本語でのわかりやすいコミットメッセージ
- コミット前の確認プロンプト

## 必要条件

- Go 1.22以上
- Git
- [Claude CLI](https://docs.anthropic.com/claude/docs/claude-cli)がインストールされ、設定済みであること

## インストール

### リリースバイナリから（推奨）

[Releases](https://github.com/shivase/gcauto/releases)ページから、お使いのOS/アーキテクチャに対応したZIPファイルをダウンロードしてください。

```bash
# macOS (Intel)
curl -L https://github.com/shivase/gcauto/releases/latest/download/gcauto-darwin-amd64.zip -o gcauto.zip

# macOS (Apple Silicon)
curl -L https://github.com/shivase/gcauto/releases/latest/download/gcauto-darwin-arm64.zip -o gcauto.zip

# Linux (x86_64)
curl -L https://github.com/shivase/gcauto/releases/latest/download/gcauto-linux-amd64.zip -o gcauto.zip

# Linux (ARM64)
curl -L https://github.com/shivase/gcauto/releases/latest/download/gcauto-linux-arm64.zip -o gcauto.zip

# ZIPファイルを解凍
unzip gcauto.zip

# 実行権限を付与してインストール
chmod +x gcauto
sudo mv gcauto /usr/local/bin/

# 一時ファイルを削除
rm gcauto.zip
```

### ソースからビルド

```bash
# リポジトリをクローン
git clone https://github.com/shivase/gcauto.git
cd gcauto

# ビルドしてインストール
make install
```

### 手動インストール

```bash
# ビルド
go build -o gcauto

# 実行権限を付与
chmod +x gcauto

# パスの通った場所に移動
sudo mv gcauto /usr/local/bin/
```

## コミットメッセージの形式

gcautoは以下の形式でコミットメッセージを生成します：

```
型: 簡潔な変更内容

- 具体的な変更点1
- 具体的な変更点2
- 具体的な変更点3
```

型は以下から選択されます：
- `feat`: 新機能
- `fix`: バグ修正
- `docs`: ドキュメントのみの変更
- `style`: コードの意味に影響を与えない変更（空白、フォーマット等）
- `refactor`: バグ修正や機能追加を伴わないコード変更
- `test`: テストの追加や修正
- `chore`: ビルドプロセスやツールの変更

## 開発

### ビルド

```bash
# 現在のシステム向けビルド
make build

# すべてのプラットフォーム向けビルド
make build-all

# 特定のプラットフォーム向けビルド
GOOS=linux GOARCH=arm64 make build
```

### テスト

```bash
make test
```

### Lintチェック

```bash
make lint
```

### CI/CD

このプロジェクトはGitHub Actionsを使用して自動テストとリリースを行っています：

- **プッシュ/PR時**: 自動的にLintチェックを実行
- **タグ付け時**: Lintチェック後、各プラットフォーム向けのバイナリをビルドし、GitHubリリースを作成

### リリース方法

```bash
# バージョンタグを作成してプッシュ
git tag v1.0.0
git push origin v1.0.0
```

### その他のコマンド

```bash
# ヘルプを表示
make help

# クリーンアップ
make clean

# アンインストール
make uninstall

# 開発ビルド（race detector付き）
make dev-build
```

## プロジェクト構造

```
gcauto/
├── main.go              # メインプログラム
├── go.mod               # Goモジュール定義
├── Makefile             # ビルド・開発用タスク
├── LICENSE              # MITライセンス
├── README.md            # このドキュメント
├── .github/
│   └── workflows/
│       └── ci.yml       # GitHub Actions CI/CD設定
└── .golangci.yml        # golangci-lint設定
```

## ライセンス

このプロジェクトは[MIT License](LICENSE)の下で公開されています。

## 貢献

バグ報告、機能要望、Pull Requestは歓迎します。貢献する際は以下のガイドラインに従ってください：

1. Issueを作成して議論する
2. フォークしてブランチを作成 (`git checkout -b feature/amazing-feature`)
3. 変更をコミット (`git commit -m 'feat: Add amazing feature'`)
4. ブランチにプッシュ (`git push origin feature/amazing-feature`)
5. Pull Requestを作成

### コントリビューションガイドライン

- コミットメッセージはConventional Commitsフォーマットに従ってください
- `make lint`でLintチェックをパスすることを確認してください
- 適切なテストを追加してください

## 作者

[shivase](https://github.com/shivase)

## サポート

問題が発生した場合は、[Issues](https://github.com/shivase/gcauto/issues)で報告してください。

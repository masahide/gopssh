# gopssh CLIモダナイゼーション要求仕様書

- 文書種別: 要求仕様書
- 対象プロダクト: `gopssh`
- 対象コード: ユーザー提供アーカイブ `gopssh-main(1).zip`
- 対象アーカイブSHA-256: `43e197a4e3865ebf894955cd5235ba142da0b5153445d0a10fbbbc71721675dc`
- 作成日: 2026-07-25
- 最終更新日: 2026-07-25
- 文書版: 1.1
- 文書ステータス: 実装着手用ドラフト
- 主要制約: 既存CLI引数・既存スクリプトとの後方互換性を維持する

## 1. 目的

本仕様は、現在の `gopssh` を、OpenAIの `cli-creator` および `agent-cli-patterns` が示すCLI設計原則に沿った、発見しやすく、機械処理しやすく、安全に自動化できるCLIへ拡張するための要求を定義する。

同時に、既存利用者が使用している次の形式を壊さないことを必須条件とする。

```bash
gopssh -h hosts.txt -u root -p 10 -d uptime
```

新しい正規構文はサブコマンド形式とするが、従来構文は互換モードとして継続提供する。

## 2. 参照資料

- OpenAI CLI Creator
  - https://github.com/openai/skills/blob/main/skills/.curated/cli-creator/SKILL.md
- OpenAI Codex CLI Patterns
  - https://github.com/openai/skills/blob/main/skills/.curated/cli-creator/references/agent-cli-patterns.md
- 現行実装
  - `cmd/gopssh/main.go`
  - `cmd/gopssh/main_test.go`
  - `pkg/pssh/pssh.go`
  - `pkg/pssh/output.go`
  - `README.md`

## 3. 設計原則

### 3.1 後方互換性を最優先する

既存の有効なコマンドライン、フラグ名、既定値、標準入出力、ホストファイル解釈、終了コード、およびリモートコマンド組み立て方式を、互換モードでは変更しない。

### 3.2 新しい正規構文を明確にする

新規利用者、ドキュメント、サンプル、およびCodexなどの自動実行主体には、サブコマンド形式を正規インターフェースとして案内する。

### 3.3 人間向け出力と機械向け出力を分離する

通常は読みやすいテキストを出力し、`--json` 指定時には安定した機械可読形式を出力する。JSONモードでは標準出力にJSON以外を混在させない。

### 3.4 危険な操作を暗黙に実行しない

`doctor`、`hosts`、`config` などの検査系コマンドは、明示的なオプションなしにSSH接続やリモートコマンド実行を行わない。

### 3.5 単一の実行エンジンを使用する

従来構文と新構文は、解析後に同じ内部オプション構造へ正規化し、同じ `pkg/pssh` 実行経路を使用する。実行ロジックを二重実装しない。

### 3.6 大量出力への既存対策を維持する

共有メモリ予算、ディスクスプール、スプール上限、ファイル権限、キャンセル処理、およびセッション停止処理を、新しい出力形式でも維持する。

### 3.7 Helpをエラー回復インターフェースとして扱う

新しい正規コマンド体系では、helpを単なる参照文書ではなく、構文エラーから正しい呼び出しへ復帰するためのインターフェースとして設計する。メインコマンド、コマンドグループ、および末端サブコマンドのいずれで構文エラーが発生した場合も、エラー箇所に対応するhelpと次に実行すべき具体的なコマンドを提示する。

## 4. 現行CLIの互換対象

### 4.1 現行構文

```text
gopssh [legacy-flags] remote-command [remote-arguments...]
```

### 4.2 維持対象フラグ

以下の既存フラグは削除、改名、意味変更を行わない。

| 既存フラグ | 現在の意味 | 互換要件 |
|---|---|---|
| `-a` | SSH Agent同時接続数 | 維持 |
| `-c` | カラー出力の有効・無効 | `-c=false` を含め維持 |
| `-ciphers` | SSH暗号方式 | 維持 |
| `-d` | ホスト名表示 | 維持 |
| `-debug` | デバッグ出力 | 維持 |
| `-h` | ホストファイル | helpへ転用しない |
| `-i` | カンマ区切り秘密鍵ファイル | 維持 |
| `-identities-only` | SSH Agentを使用しない | 維持 |
| `-k` | known_hosts検証を無効化 | 維持 |
| `-kex` | SSH鍵交換方式 | 維持 |
| `-legacy-crypto` | 旧暗号方式と優先順を使用 | 維持 |
| `-macs` | SSH MAC方式 | 維持 |
| `-max-buffer-memory` | 出力共有メモリ上限 | 維持 |
| `-max-spool-size` | 出力スプール総量上限 | 維持 |
| `-p` | SSH同時接続数 | 維持 |
| `-s` | 入力順ソート出力 | `-s=false` を含め維持 |
| `-spool-dir` | スプール親ディレクトリ | 維持 |
| `-timeout` | TCP接続タイムアウト | 維持 |
| `-u` | SSHユーザー名 | 維持 |
| `-version` | バージョン表示 | 出力書式を含め維持 |

Go標準 `flag` が現在受理している一重ハイフン・二重ハイフンの双方について、既存テストで有効と確認された形式を維持する。

### 4.3 維持対象の既存挙動

互換モードでは、少なくとも以下を維持する。

1. `-h` はホストファイル指定であり、短縮helpではない。
2. フラグ解析は最初の非フラグ引数で終了する。
3. 残りの引数を空白で連結してリモートコマンド文字列を作る。
4. 標準入力がTTYでない場合、標準入力全体を読み込み、各ホストへ送信する。
5. 既定の並列数は32。
6. 既定の結果出力順はホストファイルの入力順。
7. 既定でカラー出力を有効にし、実際のANSI出力は出力先ごとのTTY状態に従う。
8. 接続失敗はホスト単位の終了コード255として扱う。
9. 複数ホストの場合、現在の「最初に採用された非ゼロコード」をプロセス終了コードとして返す。
10. パラメータ不足時は終了コード2を返す。
11. `-version` は次の3行形式を維持する。

```text
version: <version>
commit: <commit>
built_at: <date>
```

12. ホストファイルの空白区切り、コメント、IPv4、IPv6、ポート検証の既存仕様を維持する。
13. 旧構文の利用時に、既定では非推奨警告をstderrへ追加しない。

## 5. 新しい正規コマンド体系

### 5.1 トップレベル

```text
gopssh <command> [options]
```

MUSTとして提供するコマンド:

```text
gopssh run
gopssh doctor
gopssh hosts list
gopssh hosts validate
gopssh config show
gopssh version
gopssh help
```

SHOULDとして提供するコマンド:

```text
gopssh completion <bash|zsh|fish|powershell>
```

### 5.2 正規利用例

```bash
gopssh run \
  --hosts-file hosts.txt \
  --user root \
  --parallel 10 \
  --show-host \
  -- uptime
```

```bash
gopssh --json doctor --hosts-file hosts.txt
```

```bash
gopssh --json hosts list --file hosts.txt
```

```bash
gopssh run --dry-run --hosts-file hosts.txt -- 'sudo systemctl restart app'
```

## 6. 互換ディスパッチ

### CLI-COMPAT-001: モード判定

プログラムは、本格的なフラグ解析を開始する前に次の規則でモードを判定する。

1. 第1引数が以下のサブコマンド名に完全一致する場合は、新しいサブコマンドモードとする。
2. 第1引数が新規追加の `--json` で、第2引数が以下のサブコマンド名に完全一致する場合も、新しいサブコマンドモードとする。
3. 第1引数が `--help` の場合は、新しいトップレベルhelpとする。
4. 第1引数がハイフンで始まる場合、または既存のlegacyフラグ解析が必要な場合は従来互換モードとする。
5. 第1引数がハイフンで始まらず、既知のサブコマンドにも一致しない場合は、新しいトップレベルコマンド名の誤りとして扱い、トップレベルの構文エラーとhelpを表示する。これは従来構文で有効だった呼び出しを変更してはならず、少なくとも既存の有効な呼び出しが先頭にlegacyフラグを持つことを互換性テストで確認する。

```text
run
doctor
hosts
config
version
completion
help
```

これにより、OpenAIのCLIパターンで推奨される次の形式と、コマンド後に指定する形式の双方を許可する。

```bash
gopssh --json doctor
gopssh doctor --json
```

既存の `--debug`、`-h`、`-u` などをサブコマンド探索のために飛び越えてはならない。例えば次は従来構文であり、リモートコマンド `doctor` を実行する。

```bash
gopssh --debug -h hosts.txt doctor
```

`-version` および、現行Go `flag` 実装が受理している `--version` は従来互換モードの出力形式を維持する。新しい出力形式は `gopssh version` で提供する。

### CLI-COMPAT-002: 旧構文内のコマンド名衝突

次の形式は第1引数が `-h` であるため、従来構文として扱わなければならない。

```bash
gopssh -h hosts.txt run --something
```

この場合のリモートコマンドは従来どおり `run --something` とする。

### CLI-COMPAT-003: 共通内部モデル

両モードの解析結果は、以下のような共通内部モデルへ変換する。

```go
type RunOptions struct {
    Targets             TargetSource
    User                string
    Parallel            int
    MaxAgentConnections int
    IdentityFiles       []string
    IdentitiesOnly      bool
    HostKeyPolicy       string
    ConnectTimeout      time.Duration
    OutputOrder         string
    Color               string
    Debug               bool
    MaxBufferMemory     int64
    MaxSpoolSize        int64
    SpoolDir            string
    Command             CommandSpec
    Stdin               StdinSpec
    Output              OutputSpec
    ExitPolicy          string
}
```

型名や詳細フィールドは実装上変更可能だが、旧・新パーサーが同じ実行サービスを呼び出す構造は必須とする。

### CLI-COMPAT-004: 互換モードの隔離

従来の `flag.FlagSet` 相当の挙動を互換アダプターとして維持し、新しいサブコマンド用パーサーの既定動作によって旧構文を解釈し直さない。

## 7. `gopssh run` 要求

### 7.1 基本構文

```text
gopssh run [run-options] -- command [arguments...]
```

または、複雑なシェル式を文字列として明示する。

```text
gopssh run [run-options] --command '<shell command>'
```

`--command` と `--` 以降のコマンド指定は排他的とする。

### 7.2 ターゲット指定

MUST:

```text
-H, --hosts-file <path>
--host <host[:port]>    繰り返し指定可能
```

要件:

1. `--hosts-file` は現行ホストファイル形式を使用する。
2. `--host` は現行 `normalizeHost` と同等の正規化・ポート範囲検証を行う。
3. 新構文では、ターゲットが0件の場合を入力エラーとする。
4. `--hosts-file -` をサポートする場合、リモート標準入力転送との同時使用を禁止する。
5. `--hosts-file` と `--host` は同時指定可能とする。
6. 同時指定時は、ホストファイル内の入力順を先に配置し、その後へ `--host` の指定順で追加する。
7. 重複は自動削除せず、入力どおり実行する。`hosts validate` では重複をwarningとして報告する。

### 7.3 正規フラグ

| 正規フラグ | 意味 | 旧フラグとの対応 |
|---|---|---|
| `-H, --hosts-file` | ホストファイル | `-h` |
| `--host` | 単一ホストを追加 | 新規 |
| `-u, --user` | SSHユーザー | `-u` |
| `-p, --parallel` | 同時SSH接続数 | `-p` |
| `--max-agent-connections` | Agent同時接続数 | `-a` |
| `-i, --identity` | 秘密鍵ファイル、繰り返し可 | `-i`のカンマ区切り |
| `--identities-only` | Agentを無効化 | `-identities-only` |
| `--connect-timeout` | TCP接続タイムアウト | `-timeout` |
| `--show-host` | ホスト名と終了コードを表示 | `-d` |
| `--order input` | 入力順に出力 | `-s=true` |
| `--order completion` | 完了順に出力 | `-s=false` |
| `--color auto` | 出力先TTYごとに自動判定 | `-c=true` |
| `--color never` | 色を無効化 | `-c=false` |
| `--color always` | TTY以外でも明示的に色を付与 | 新規 |
| `--insecure-ignore-host-key` | known_hosts検証を無効化 | `-k` |
| `--legacy-crypto` | 旧暗号方式 | 同名 |
| `--kex` | 鍵交換方式 | 同名 |
| `--ciphers` | 暗号方式 | 同名 |
| `--macs` | MAC方式 | 同名 |
| `--max-buffer-memory` | 出力メモリ上限 | 同名 |
| `--max-spool-size` | スプール上限 | 同名 |
| `--spool-dir` | スプール親ディレクトリ | 同名 |
| `--debug` | デバッグ診断 | `-debug` |
| `--dry-run` | 接続・実行せず実行計画を表示 | 新規 |
| `--json` | 機械可読出力 | 新規 |
| `--output-dir` | ホスト別出力ファイル保存先 | 新規 |
| `--exit-policy` | 集約終了コード方針 | 新規 |

### 7.4 コマンド引数の忠実性

新構文の `-- command [arguments...]` は、個々の引数境界を保持する。

実装は次のいずれかを採用し、READMEに明記する。

1. POSIXシェル向けに各引数を安全にクォートして1本のコマンド文字列へ変換する。
2. `--command` の文字列をそのままリモートシェルへ渡す。

新構文で単純な `strings.Join(args, " ")` のみを使用し、空白や引用符を含む引数の意味を失わせてはならない。

互換モードでは既存の `strings.Join` 相当の挙動を維持する。

### 7.5 標準入力

新構文では標準入力転送を明示化する。

MUST:

```text
--stdin
--stdin-file <path>
```

要件:

1. 指定なしの場合、新構文はstdinを消費しない。
2. `--stdin` はプロセスstdinを読み、全対象ホストへ同一内容を送信する。
3. `--stdin-file` は指定ファイルを読み、全対象ホストへ同一内容を送信する。
4. `--stdin` と `--stdin-file` は排他的とする。
5. 互換モードでは、非TTY stdinを自動転送する既存挙動を維持する。
6. 大容量stdinについて、最大サイズまたはストリーミング制約を文書化する。

### 7.6 Dry-run

`--dry-run` は次を行う。

1. ホストファイルを読み込み、構文検証する。
2. ターゲットを正規化する。
3. 有効なユーザー、並列数、認証ソース種別、known_hosts方針、暗号設定、出力設定を表示する。
4. 送信予定のリモートコマンドを表示する。
5. SSH接続、SSH Agent署名、リモートセッション作成、リモートコマンド実行を行わない。
6. 秘密鍵内容、Agent内秘密情報、環境変数の秘密値を表示しない。
7. `--json` と併用可能とする。

## 8. `gopssh doctor` 要求

### 8.1 目的

設定、認証経路、ローカルファイル、スプール領域、バージョン情報を安全に診断する。

### 8.2 既定動作

既定ではネットワーク接続を行わない。

検査項目:

1. バイナリのversion、commit、built_at。
2. OS、アーキテクチャ。
3. 有効なユーザー名の有無。
4. `SSH_AUTH_SOCK` の設定有無、ソケットの存在、接続可能性。
5. 指定されたidentity fileの存在と読取可否。
6. `~/.ssh/known_hosts` の存在と読取可否。
7. `--insecure-ignore-host-key` が有効か。
8. ホストファイルの存在、読取可否、構文、対象数。
9. 並列数、Agent接続数、メモリ上限、スプール上限の妥当性。
10. スプール親ディレクトリの存在、作成可否、書込可否。
11. `NO_COLOR`、`TERM`、stdout/stderrのTTY状態。
12. 認証候補が1つもない場合の不足設定。

### 8.3 オプションのネットワーク診断

SHOULD:

```text
--connect
--limit <n>
```

`--connect` 指定時のみ、最大 `--limit` 件のターゲットに対してTCP接続およびSSHハンドシェイクを確認する。リモートコマンドは実行しない。

### 8.4 doctorの終了コード

| 状態 | 終了コード |
|---|---:|
| 必須検査がすべて正常 | 0 |
| 1件以上の必須検査が異常 | 1 |
| 引数または設定値が不正 | 2 |

認証不足などで異常でも、`--json` の診断結果は可能な限り出力してから終了する。

## 9. `gopssh hosts` 要求

### 9.1 `hosts list`

```text
gopssh hosts list --file hosts.txt
```

出力項目:

- 入力順インデックス
- 元の入力値
- 正規化済みホスト名
- ポート
- IPv4、IPv6、DNS名の種別
- 重複状態
- 元ファイルの行番号

通常出力は人間向け表形式または1行1ホストとし、`--json` では安定した構造を返す。

### 9.2 `hosts validate`

```text
gopssh hosts validate --file hosts.txt
```

検証項目:

1. ファイル存在・読取可否。
2. コメントと空行の解釈。
3. ホスト名形式。
4. IPv4、IPv6形式。
5. ポートが1から65535の範囲内であること。
6. 対象が1件以上存在すること。
7. 重複ターゲット。

重複は既定ではwarningとし、`--strict` 指定時はerrorとする。

### 9.3 DNSと接続

`hosts list` と `hosts validate` は既定ではDNS解決や接続を行わない。将来 `--resolve-dns` または `--connect` を追加する場合は明示オプションとする。

## 10. `gopssh config show` 要求

```text
gopssh config show
```

目的は、実行時に使用される有効設定とその取得元を確認できるようにすることである。

表示対象:

- user
- parallel
- max_agent_connections
- identity filesのパス
- identities_only
- host key policy
- connect timeout
- output order
- color policy
- max buffer memory
- max spool size
- spool directory
- SSH algorithm policy
- SSH Agent利用可否

各項目に可能な範囲で取得元を付与する。

```text
default
environment
flag
legacy-flag
```

秘密鍵内容、Agent鍵素材、秘密情報は表示しない。

初期リリースでは永続的なgopssh専用設定ファイルや `init` コマンドを必須としない。SSH Agent、identity file、環境変数で十分に運用可能なためである。専用設定ファイルを後日追加する場合は、優先順位を次の順にする。

```text
明示CLIフラグ > 環境変数 > ユーザー設定ファイル > 既定値
```

## 11. `gopssh version` 要求

通常出力:

```text
gopssh <version> (<commit>, built <built_at>)
```

JSON出力:

```json
{
  "schema_version": "1",
  "version": "0.6.0",
  "commit": "abcdef0",
  "built_at": "2026-07-25T00:00:00Z",
  "go_version": "go1.xx.x",
  "os": "linux",
  "arch": "amd64"
}
```

既存の `gopssh -version` は従来の3行形式を維持する。

## 12. Help要求

### CLI-HELP-001: トップレベルhelp

`gopssh --help` は、少なくとも以下を1画面内で発見できる構成とする。

- リモートコマンド実行
- ローカル診断
- ホストファイル確認
- 有効設定確認
- バージョン表示
- JSON出力
- shell completion
- 従来構文が引き続き利用可能であること

### CLI-HELP-002: コマンド別help

以下をサポートする。

```text
gopssh help run
gopssh run --help
gopssh hosts --help
gopssh hosts list --help
```

新サブコマンド内では `-h` と `--help` をhelpとして使用できる。トップレベル従来構文の `-h <file>` とは、互換ディスパッチによって競合を回避する。

### CLI-HELP-003: 具体的な説明

各フラグ説明は、抽象語だけでなく、単位、既定値、危険性、出力先を明記する。

例:

```text
--insecure-ignore-host-key
    Skip known_hosts verification. This permits man-in-the-middle attacks.
```

### CLI-HELP-004: 構文エラー時のコンテキストhelp

新しい正規コマンド体系では、引数解析またはコマンド選択の構文エラーが発生した場合、エラーだけで終了してはならない。エラーが発生したコマンド階層に対応するhelpを、エラーと同時に表示する。

対象となるエラーには、少なくとも以下を含める。

1. 未知のトップレベルコマンド。
2. 未知のサブコマンド。
3. サブコマンド必須のコマンドグループで、サブコマンドが省略された場合。
4. 必須引数または必須フラグの不足。
5. 未知のフラグ。
6. フラグ値の形式不正または範囲外。
7. 排他的なオプションの同時指定。
8. 余分な位置引数。
9. `--` または `--command` に関するコマンド指定エラー。

表示するhelpの階層は、最も具体的にエラーを修正できるコマンドとする。

| 入力例 | 表示するhelp |
|---|---|
| `gopssh hosst` | トップレベル `gopssh --help` 相当 |
| `gopssh hosts` | `gopssh hosts --help` 相当 |
| `gopssh hosts lsit` | `gopssh hosts --help` 相当 |
| `gopssh hosts list` で `--file` 不足 | `gopssh hosts list --help` 相当 |
| `gopssh run --parallel abc` | `gopssh run --help` 相当 |
| `gopssh config unknown` | `gopssh config --help` 相当 |

構文エラー時のテキスト出力は、次の順序をMUSTとする。

1. 問題を1行で説明するエラー。
2. 修正候補または不足している要素。
3. 空行。
4. 該当コマンド階層のUsage。
5. 該当階層で利用可能なサブコマンド、必須引数、主要オプション。
6. 1件以上の正しい実行例。
7. 完全なhelpを再表示するための具体的なコマンド。

例:

```text
Error: unknown subcommand "lsit" for "gopssh hosts".
Did you mean "list"?

Usage:
  gopssh hosts <command> [options]

Commands:
  list       List and normalize targets from a hosts file
  validate   Validate a hosts file without connecting

Examples:
  gopssh hosts list --file hosts.txt
  gopssh hosts validate --file hosts.txt

Run 'gopssh hosts --help' for full help.
```

通常テキストモードでは、構文エラーと自動表示helpをstderrへ出力し、stdoutは空にする。終了コードは2とする。

### CLI-HELP-005: 未知コマンド・フラグの候補提示

未知のコマンド、サブコマンド、またはフラグが入力された場合、編集距離、共通接頭辞、および同じコマンド階層の候補だけを使用して、妥当な候補を最大3件提示する。候補の確度が低い場合は、誤った提案を無理に表示しない。

例:

```text
Error: unknown option "--paralel" for "gopssh run".
Did you mean "--parallel"?
```

候補提示によってコマンドを自動実行または自動補正してはならない。

### CLI-HELP-006: 明示helpとエラーhelpの終了状態

以下を区別する。

| 操作 | 終了コード | 出力先 |
|---|---:|---|
| `gopssh --help`、`gopssh run --help` など明示help | 0 | stdout |
| 構文エラーに伴う自動help | 2 | stderr |
| `gopssh help <command-path>` | 0 | stdout |

明示helpに `Error:` を付加してはならない。構文エラー時にhelpを表示したことを理由として終了コード0にしてはならない。

### CLI-HELP-007: JSONモードにおける構文エラーhelp

`--json` 指定時の構文エラーでは、次をMUSTとする。

1. stdoutへJSONエラーオブジェクトを1件だけ出力する。
2. stderrへ該当コマンド階層の人間向けコンテキストhelpを出力する。
3. stdoutのJSONにはANSIエスケープ、Usage本文の装飾、進捗ログを混入させない。
4. 終了コード2を返す。
5. JSONエラーに、機械的な修正判断に必要なコマンド階層、usage、候補、helpコマンドを含める。

例:

```json
{
  "schema_version": "1",
  "ok": false,
  "error": {
    "code": "unknown_subcommand",
    "message": "unknown subcommand \"lsit\" for \"gopssh hosts\"",
    "command_path": ["gopssh", "hosts"],
    "invalid_token": "lsit",
    "suggestions": ["list"],
    "usage": "gopssh hosts <command> [options]",
    "help_command": "gopssh hosts --help"
  }
}
```

JSON利用者がstderrを取得しない場合でも次の操作を判断できるよう、`usage` と `help_command` は必須とする。

### CLI-HELP-008: 一貫したエラーレンダリング

メインコマンドと各サブコマンドが個別に異なる書式でエラーとhelpを組み立ててはならない。解析エラーを共通の構造化エラーへ変換し、単一のエラー・helpレンダラーでテキストおよびJSONを生成する。

概念モデル:

```go
type UsageError struct {
    Code         string
    Message      string
    CommandPath  []string
    InvalidToken string
    Suggestions  []string
    Usage        string
    HelpCommand  string
}
```

型名は変更可能だが、すべての構文エラーが同じ出力規則と終了コードへ収束することを必須とする。パーサーライブラリが自動でエラーやhelpを出力する場合は、その自動出力を抑制し、二重表示を防止する。

### CLI-HELP-009: 互換モードへの影響禁止

CLI-HELP-004からCLI-HELP-008は、新しい正規コマンド体系に適用する。従来互換モードの既存テキスト、既存help、`-h` の意味、および既存終了コードを無条件に変更してはならない。

ただし、既存構文として有効ではない、先頭が未知の非フラグトークンである呼び出しについては、トップレベルの未知コマンドとして新しいエラーhelpを表示できる。

## 13. JSONおよび構造化出力ポリシー

### 13.1 共通規則

1. `--json` 時、stdoutにはJSON以外を出力しない。
2. `--json` はカラー出力を常に無効化する。`--color always` と同時指定された場合もJSONへANSIエスケープを混入させない。
3. 進捗、警告、デバッグ、ローカル診断はstderrへ出力する。
4. 全レコードに `schema_version` を含める。
5. エラーに秘密鍵内容、トークン、Cookie、秘密ヘッダーを含めない。
6. 空の結果は成功として表現し、コマンド要件上0件が不正な場合だけ非ゼロとする。
7. フィールド削除や意味変更はschema major version変更なしに行わない。
8. 新規フィールド追加は後方互換とする。
9. リモート出力が有効なUTF-8でない場合にもデータを欠落させない。
10. 有効なUTF-8は `stdout` / `stderr` と `*_encoding: "utf-8"` で表現する。
11. 無効なUTF-8を含む場合はBase64へ変換し、`stdout_base64` / `stderr_base64` と `*_encoding: "base64"` で表現する。
12. `--output-dir` 使用時は、ファイルへ元のバイト列をそのまま保存する。

### 13.2 `run --json`

大量出力を逐次処理できるよう、`run --json` はNDJSONを正規形式とする。

結果レコード例:

```json
{"schema_version":"1","type":"result","index":0,"target":"host1:22","status":"success","exit_code":0,"stdout":"...","stderr":"","error":null,"duration_ms":1234}
```

最終サマリーレコード例:

```json
{"schema_version":"1","type":"summary","total":2,"succeeded":1,"failed":1,"connection_failed":0,"canceled":0,"aggregate_exit_code":1}
```

要件:

1. ホストごとに `result` レコードを1件出力する。
2. 最後に `summary` レコードを1件出力する。
3. `--order input` では入力順、`--order completion` では完了順に結果レコードを出力する。
4. stdout/stderrの文字列化により、既存の共有メモリ上限を無効化してはならない。
5. スプールファイルからJSONへ書き出す際、全内容を再度メモリへ読み込まない。
6. JSONエスケープをストリーミング可能な方式で実装する。
7. 出力途中でローカル書込エラーが発生した場合は非ゼロ終了とする。

### 13.3 `--output-dir`

```text
--output-dir <directory>
```

指定時はホストごとのstdout/stderrをファイルへ保存できる。

推奨ファイル名:

```text
<index>-<sanitized-host>.stdout
<index>-<sanitized-host>.stderr
```

JSON結果には次を含める。

- stdout_path
- stderr_path
- stdout_bytes
- stderr_bytes
- target
- exit_code

パスは可能な限り絶対パスまたは起点が明確なパスとする。ファイル権限は所有者のみが読み書き可能な設定を既定とする。

### 13.4 JSONエラー形式

コマンド全体が開始前に失敗した場合:

```json
{
  "schema_version": "1",
  "ok": false,
  "error": {
    "code": "invalid_argument",
    "message": "hosts file is required",
    "details": {}
  }
}
```

安定したエラーコード候補:

```text
invalid_argument
unknown_command
unknown_subcommand
unknown_option
missing_argument
conflicting_options
invalid_config
hosts_file_not_found
hosts_file_invalid
auth_unavailable
known_hosts_unavailable
connection_failed
ssh_handshake_failed
remote_start_failed
remote_exit_nonzero
output_spool_limit
output_io_failed
canceled
internal_error
```

## 14. テキスト出力ポリシー

### 14.1 新構文

1. リモートstdoutはstdoutへ出力する。
2. リモートstderr、接続エラー、診断はstderrへ出力する。
3. 通常エラーは簡潔なメッセージとし、既定ではGoのソースファイル名や行番号を表示しない。
4. `--debug` 時のみ詳細な内部情報を追加する。
5. カラーはstdoutとstderrを独立して判定する。
6. リダイレクトされた通常ファイルへANSIエスケープを混入させない。
7. `NO_COLOR` と `TERM=dumb` を尊重する。
8. 構文エラー時は、エラー発生箇所に対応するコンテキストhelpをstderrへ続けて表示する。

### 14.2 互換モード

既存スクリプトへの影響を避けるため、互換モードのテキスト形式は原則維持する。新しい診断文や移行警告を無条件に追加しない。

## 15. 終了コード

### 15.1 互換モード

既存動作を維持する。

| 条件 | 終了コード |
|---|---:|
| 全ホスト成功 | 0 |
| 利用方法・引数不足 | 2 |
| ローカル処理、出力、スプール、キャンセルエラー | 現行どおり1 |
| 接続失敗が採用された最初の失敗 | 255 |
| リモートコマンド非ゼロ | 現行の集約順序に従ったリモート終了コード |

### 15.2 新構文

既定の `--exit-policy first` は、既存利用者が理解しやすいよう、最初に採用された非ゼロ結果を返す。

```text
--exit-policy first
--exit-policy any
--exit-policy always-zero
```

- `first`: 結果出力順で最初の非ゼロコードを返す。
- `any`: 1件でも失敗した場合は1を返す。
- `always-zero`: リモート結果にかかわらず0を返す。ただしCLI解析、ローカルI/O、内部エラーは非ゼロとする。

新構文の共通コード:

| 条件 | 終了コード |
|---|---:|
| 成功 | 0 |
| ローカルまたは集約エラー | 1 |
| 利用方法・設定値エラー。コンテキストhelpを伴う | 2 |
| SIGINT | 130 |
| SIGTERM | 143 |

`first` の場合、リモート終了コードまたは接続失敗255を返せる。JSONのsummaryには必ず `aggregate_exit_code` を含める。

## 16. 認証・設定・セキュリティ

### 16.1 認証ソース

現行のSSH Agentおよびidentity file方式を維持する。

認証方式の試行順序は、互換モードと新構文で原則統一する。

1. `--identities-only` が無効で `SSH_AUTH_SOCK` が利用可能な場合、SSH Agent。
2. `-i` または `--identity` で明示されたidentity file。
3. identity fileが明示されていない場合は、現行の既定identity file群。

`--identities-only` 指定時はSSH Agentを使用しない。認証順序の変更は接続先の `MaxAuthTries` や選択される鍵へ影響するため、将来変更する場合も独立した互換性変更として扱う。

### 16.2 秘密情報保護

1. 秘密鍵ファイル内容を表示しない。
2. SSH Agentの秘密情報を表示しない。
3. doctorでは鍵の存在、読取可否、公開鍵fingerprintなどの非秘密情報だけを扱う。
4. JSONエラーへ環境変数全体を含めない。
5. デバッグ出力でもstdin本文やリモートコマンド内の秘密値を自動展開しない。
6. スプールディレクトリ0700、ファイル0600の既存要件を維持する。
7. 終了、エラー、シグナル、キャンセル時にスプールを削除する。

### 16.3 known_hosts

既定はknown_hosts検証を有効とする。検証無効化フラグは危険性が分かる名称とhelpを使用する。

旧 `-k` は維持するが、新構文では `--insecure-ignore-host-key` を正規名称とする。

## 17. 性能・資源管理

1. 既定の共有出力メモリ上限128MiBを維持する。
2. 既定のスプール上限10GiBを維持する。
3. 新しいJSONエンコード処理によって、ホスト数または総出力量に比例して全データをメモリ保持しない。
4. 入力順出力で後続ホストの結果を待機する場合も、既存のスプール機構を使用する。
5. スプール上限、作成失敗、書込失敗時は対象SSHセッションを停止する。
6. キャンセル後に未開始ホストを起動しない。
7. TCP接続中、SSHハンドシェイク中、リモートセッション実行中のキャンセルを維持する。
8. `doctor` と `hosts` のローカル検査は、ホスト数に対して線形時間・定数個の開放可能なリソースで処理する。

## 18. ドキュメント要求

### 18.1 README

READMEは新しい正規構文を先に掲載する。

推奨構成:

1. 概要
2. 3分で実行できるクイックスタート
3. `run`
4. `doctor`
5. `hosts`
6. JSON/NDJSON仕様
7. stdout/stderrと終了コード
8. 認証とknown_hosts
9. 大量出力とスプール
10. 従来構文との互換性
11. インストール
12. 開発・ビルド

### 18.2 互換性ガイド

旧構文から新構文への対応例を掲載する。

```bash
# 従来構文
gopssh -h hosts.txt -u root -p 10 -d uptime

# 正規構文
gopssh run --hosts-file hosts.txt --user root --parallel 10 --show-host -- uptime
```

```bash
# 従来構文
gopssh -h hosts.txt -c=false -s=false command

# 正規構文
gopssh run --hosts-file hosts.txt --color never --order completion -- command
```

```bash
# 従来構文
gopssh -h hosts.txt -i ~/.ssh/id_a,~/.ssh/id_b command

# 正規構文
gopssh run --hosts-file hosts.txt \
  --identity ~/.ssh/id_a \
  --identity ~/.ssh/id_b \
  -- command
```

### 18.3 JSON仕様

READMEまたは専用文書で次を固定する。

- schema version
- resultレコード
- summaryレコード
- errorレコード
- nullと空文字の扱い
- stdout/stderrのエンコーディング
- 出力順
- 出力ファイルモード
- 終了コードとの関係

### 18.4 Codex向けコンパニオンスキル

新CLI実装後、次のような小さなコンパニオンスキルを追加することをSHOULDとする。

```text
.codex/skills/gopssh/SKILL.md
```

内容:

1. `command -v gopssh` による導入確認。
2. 最初に `gopssh --json doctor` を実行すること。
3. `hosts validate` によるターゲット確認。
4. `run --dry-run` による実行計画確認。
5. 読み取り系コマンドの例。
6. 再起動、削除、更新など破壊的なリモートコマンドは明示依頼なしに実行しないこと。
7. 3件以上のコピー可能な実行例。

## 19. 非対象

初回モダナイゼーションでは以下を必須対象としない。

1. SSHプロトコル実装の置換。
2. ファイル転送、SCP、SFTP。
3. インタラクティブTTYセッション。
4. ポートフォワーディング。
5. SSH configの全機能互換パーサー。
6. Windows向け正式Release追加。
7. Web APIや汎用HTTP `request` コマンド。
8. リモートコマンド内容に基づく自動的な危険判定。
9. 既存フラグの削除予定設定。

`gopssh run` 自体が、任意のリモートコマンドを実行するためのraw escape hatchに相当する。

## 20. 実装上の制約

1. 実装言語は既存コードに合わせてGoを維持する。
2. 新しいCLIパーサーライブラリを採用する場合でも、互換モードは専用の解析経路で保護する。
3. `cmd/gopssh` は引数解析、出力形式選択、シグナル処理に集中させる。
4. 実行、接続、出力保持は `pkg/pssh` の再利用可能なAPIとして保持する。
5. `pkg/pssh` がグローバルな `flag.Args()` や `os.Stdout` に直接依存する箇所は、新構文対応時に段階的に注入可能な形へ整理する。
6. ただし互換モードの結果が変わらないことをテストで保証する。
7. ライブラリAPIへ `io.Reader`、`io.Writer`、command、contextを明示的に渡せる設計を推奨する。

## 21. テスト要求

### 21.1 互換性ゴールデンテスト

以下を含む既存構文を、変更前バイナリと変更後バイナリで比較する。

1. 全既存フラグのhelpと既定値。
2. `-c=false`、`-s=false` などbool値指定。
3. `-h` がホストファイルであること。
4. `-version` の3行出力。
5. stdin自動転送。
6. リモートコマンド引数の既存連結方式。
7. 入力順・完了順出力。
8. stdout/stderr分離。
9. 接続失敗255。
10. リモート終了コード。
11. パラメータ不足2。
12. SIGINT/SIGTERM時の既存終了挙動。
13. スプール上限、I/O失敗、キャンセル。

### 21.2 新CLIテスト

1. トップレベルhelpに全主要コマンドが表示される。
2. 各サブコマンドの `-h` と `--help`。
3. `run` のターゲット不足が終了コード2。
4. `--host` と `--hosts-file` の正規化。
5. `--dry-run` がDialを一度も呼ばない。
6. `--command` と `--` が排他的。
7. 空白・引用符・改行を含む引数の忠実性。
8. `--stdin`、`--stdin-file`、指定なしの挙動。
9. `doctor` が認証不足でもJSONを返す。
10. `hosts validate` のIPv4、IPv6、ポート境界、コメント、空行、重複。
11. `config show` が秘密情報を出さない。
12. JSON stdoutにログ文字列が混入しない。
13. resultとsummaryのschema snapshot。
14. NDJSONを1行ずつデコードできる。
15. 大量JSON出力時も共有メモリ上限を維持する。
16. `--output-dir` の権限、ファイル名、byte count。
17. stdoutとstderrを別々にリダイレクトした場合の色判定。
18. `NO_COLOR` と `TERM=dumb`。
19. exit policyごとの終了コード。
20. SIGINT 130、SIGTERM 143。
21. 未知のトップレベルコマンドで、エラー、候補、トップレベルhelp、終了コード2。
22. コマンドグループのサブコマンド不足で、そのグループのhelpと終了コード2。
23. 未知のサブコマンドで、同一階層の候補とグループhelp。
24. 末端コマンドの必須引数不足、未知フラグ、値不正、排他違反で、末端コマンドhelp。
25. 明示 `--help` はstdoutへ出力し終了コード0、エラーhelpはstderrへ出力し終了コード2。
26. `--json` 構文エラー時、stdoutはJSONエラー1件だけ、stderrはコンテキストhelp。
27. JSONエラーに `command_path`、`usage`、`suggestions`、`help_command` が含まれる。
28. パーサーと共通レンダラーによるエラーまたはhelpの二重表示がない。
29. 候補確度が低い未知トークンに不適切な `Did you mean` を表示しない。

### 21.3 品質ゲート

```bash
go test -race ./...
go vet ./...
golangci-lint run
make build
```

加えて、インストール後にリポジトリ外から次を確認する。

```bash
cd /tmp
command -v gopssh
gopssh --help
gopssh --json doctor
gopssh hosts validate --file /path/to/hosts
gopssh run --dry-run --hosts-file /path/to/hosts -- uptime
```

## 22. 受け入れ基準

### AC-001 後方互換性

既存の有効なコマンドラインを変更せずに実行でき、互換性ゴールデンテストがすべて成功すること。

### AC-002 正規コマンド面

`run`、`doctor`、`hosts list`、`hosts validate`、`config show`、`version` がhelpから発見できること。

### AC-003 機械可読性

`--json` 指定時にstdoutが有効なJSONまたはNDJSONだけで構成され、診断がstderrへ分離されること。

### AC-004 安全な事前確認

`run --dry-run` が接続せずに、実行対象、コマンド、認証種別、重要な設定を確認できること。

### AC-005 大量出力耐性

テキストおよびJSONモードで、出力共有メモリ上限とスプール上限が機能し、データ欠落または無限待機が発生しないこと。

### AC-006 出力先別カラー

stdoutとstderrを独立判定し、非TTYファイルへANSIエスケープを混入させないこと。

### AC-007 終了コード

互換モードの既存終了コードを維持し、新構文の終了コードとexit policyを文書どおりに返すこと。

### AC-008 秘密情報保護

doctor、config、JSONエラー、debug出力に秘密鍵内容または認証秘密情報を含めないこと。

### AC-009 リポジトリ外実行

インストール済み `gopssh` が任意の作業ディレクトリからhelp、doctor、hosts、dry-runを実行できること。

### AC-010 構文エラーからの回復支援

新しい正規コマンド体系のメインコマンド、コマンドグループ、および末端サブコマンドで構文エラーが発生した場合に、エラー内容、妥当な修正候補、該当階層のUsage・主要オプション・実行例・helpコマンドが表示されること。明示helpは終了コード0、構文エラーhelpは終了コード2であり、JSONモードではstdoutの機械可読性が維持されること。

## 23. 推奨実装順序

### フェーズ1: 互換ディスパッチとhelp

- legacy/modernモード判定
- `run` サブコマンド
- `version`
- トップレベルhelp
- コマンド階層別の構文エラーhelp
- 未知コマンド・フラグ候補提示
- 共通UsageErrorレンダラー
- 互換性ゴールデンテスト

### フェーズ2: 共通オプションモデル

- `RunOptions` への正規化
- `pkg/pssh` から `flag.Args()` などのグローバル依存を除去
- commandとstdinの明示注入
- `--dry-run`

### フェーズ3: 検査コマンド

- `doctor`
- `hosts list`
- `hosts validate`
- `config show`

### フェーズ4: 構造化出力

- 共通JSONエラー
- `run` NDJSON
- summary
- `--output-dir`
- JSON schema文書

### フェーズ5: 配布と自動利用

- shell completion
- README更新
- リポジトリ外smoke test
- Codex向けコンパニオンスキル

## 24. 完了定義

本変更は、次のすべてを満たした時点で完了とする。

1. 受け入れ基準AC-001からAC-010を満たす。
2. 既存テストと新規テストがrace付きで成功する。
3. READMEが新構文を正規構文として説明する。
4. 旧構文の互換性と `-h` の特例が明記される。
5. JSON/NDJSON schemaと終了コードが文書化される。
6. リポジトリ外からインストール済みバイナリをsmoke testできる。
7. 既存のメモリ・スプール・キャンセル安全性に回帰がない。

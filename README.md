# gopssh

[![Go Report Card](https://goreportcard.com/badge/github.com/masahide/gopssh)](https://goreportcard.com/report/github.com/masahide/gopssh)
[![Build status](https://github.com/masahide/gopssh/actions/workflows/buildpkg.yml/badge.svg)](https://github.com/masahide/gopssh/actions/workflows/buildpkg.yml)

`gopssh` は、複数のSSHターゲットで同じコマンドを並列実行するCLIです。
正規インターフェースはサブコマンド形式です。従来のフラグ形式も互換性のため
引き続き利用できます。

## クイックスタート

```bash
printf '%s\n' host1 host2 > hosts.txt
gopssh hosts validate --file hosts.txt
gopssh run --dry-run --hosts-file hosts.txt -- uptime
gopssh run --hosts-file hosts.txt -- uptime
```

既定では `~/.ssh/known_hosts` を検証し、利用可能ならSSH Agentを先に、
続いてidentity fileを使用します。

## `run`

```text
gopssh run [options] -- command [arguments...]
gopssh run [options] --command '<shell command>'
```

```bash
gopssh run \
  --hosts-file hosts.txt \
  --user root \
  --parallel 10 \
  --show-host \
  -- uptime

gopssh run \
  --hosts-file hosts.txt \
  --host extra.example.com:2222 \
  --identity ~/.ssh/id_ed25519 \
  --order completion \
  -- printf '%s\n' 'hello world'
```

`--` 以降では、引数境界を保持するため各引数をPOSIXシェル向けに安全に
シングルクォートしてからリモートシェルへ渡します。複雑なシェル式を
そのまま渡す場合は `--command` を使用してください。この2形式は同時に
指定できません。

ターゲットは `-H, --hosts-file` と繰り返し可能な `--host` を併用できます。
ホストファイル内の順序、続いて `--host` の指定順となり、重複は削除しません。

新構文は既定でstdinを読みません。プロセスstdinを転送するには `--stdin`、
ファイルを転送するには `--stdin-file PATH` を明示します。stdinは全ホストへ
同じ内容を送り、上限は64MiBです。

### Dry-run

```bash
gopssh run --dry-run --hosts-file hosts.txt -- 'sudo systemctl restart app'
gopssh run --json --dry-run --host host1 -- uptime
```

`--dry-run` はターゲット、コマンド、認証候補、known_hosts方針、並列数、
出力設定を検証・表示します。TCP接続、SSH Agent署名、SSHセッション作成、
リモート実行は行いません。

## `doctor`

```bash
gopssh doctor --hosts-file hosts.txt
gopssh --json doctor --hosts-file hosts.txt
```

バージョン、OS、ユーザー、SSH Agentソケット、identity file、known_hosts、
ホストファイル、並列数、メモリ・スプール上限、TTY・色環境、認証候補を
ローカルで診断します。既定ではネットワークへ接続しません。
`--connect --limit N` を明示した場合だけ、先頭NターゲットへTCP接続、
SSHハンドシェイク、ホスト鍵検証、認証を試します。リモートセッションの作成や
コマンド実行は行いません。

終了コードは、正常なら0、必須検査の異常があれば1、引数不正なら2です。
SSH Agentとidentity fileは代替の認証経路であり、どちらか1つが利用可能なら
認証検査は成功します。JSONの各checkにある `required` は、そのcheckの失敗が
doctor全体の失敗になるかを示します。

## `hosts`

```bash
gopssh hosts list --file hosts.txt
gopssh --json hosts list --file hosts.txt
gopssh hosts validate --file hosts.txt
gopssh hosts validate --strict --file hosts.txt
```

`list` は入力順、元の値、正規化値、ホスト、ポート、IPv4/IPv6/DNS種別、
重複、行番号を表示します。`validate` はコメント・空行、名前・IP形式、
1〜65535のポート、空ファイル、重複を検査します。重複は通常warningで、
`--strict` ではerrorです。DNS解決やネットワーク接続は行いません。

## `config` と `version`

```bash
gopssh config show
gopssh --json config show
gopssh version
gopssh --json version
```

`config show` は有効値と `default` / `environment` などの取得元を表示します。
秘密鍵の内容やAgentの鍵素材は表示しません。従来の `gopssh -version` は
互換の3行形式を維持し、`gopssh version` は新しい1行形式を使用します。

## JSON / NDJSON

schema major versionは `1` です。`--json` 時のstdoutにはJSON以外を出力せず、
診断とコンテキストhelpはstderrへ分離します。色は常に無効です。

`run --json` は、ホストごとの `result` を1行ずつ出力し、最後に `summary`
を1行出力するNDJSONです。

```json
{"schema_version":"1","type":"result","index":0,"target":"host1:22","status":"success","exit_code":0,"error":null,"duration_ms":1234,"stdout":"ok\n","stdout_encoding":"utf-8","stderr":"","stderr_encoding":"utf-8"}
{"schema_version":"1","type":"summary","total":1,"succeeded":1,"failed":0,"connection_failed":0,"canceled":0,"local_errors":0,"aggregate_exit_code":0}
```

- 有効なUTF-8は `stdout` / `stderr` と `*_encoding: "utf-8"` で表します。
- 無効なUTF-8を含む出力は `stdout_base64` / `stderr_base64` と
  `*_encoding: "base64"` で欠落なく表します。
- 空出力は空文字です。エラーがなければ `error` は `null` です。
- `connection_failed` は実行エンジンが接続段階の失敗と判定した場合だけです。
  リモートコマンド自身の終了255は通常の `failed` として扱います。
- `--order input` は入力順、`--order completion` は完了順です。
- フィールド追加は後方互換です。削除・意味変更時はschema majorを変更します。

`--output-dir DIR` を指定すると、元のstdout/stderrバイト列を
`<index>-<sanitized-target>.stdout` / `.stderr` へ保存します。ディレクトリは
0700、ファイルは0600です。JSON resultには絶対パスとbyte countを含め、
インライン出力は含めません。
ファイル保存はresultをstdoutへ書く前に完了させます。保存に失敗した場合も
完全な `status: "output_failed"` resultを出力し、stderrへ
`output_io_failed` を表示し、summaryの `local_errors` を加算します。

実行開始前のJSONエラーは1個のJSONオブジェクトです。

```json
{"schema_version":"1","ok":false,"error":{"code":"missing_argument","message":"remote command is required","command_path":["gopssh","run"],"suggestions":[],"usage":"gopssh run [options] -- command [arguments...]","help_command":"gopssh run --help"}}
```

## stdout / stderr・色・終了コード

新構文ではリモートstdoutをstdoutへ、リモートstderr、接続エラー、診断を
stderrへ出力します。`--color auto` は出力先ごとにTTYを判定し、
`NO_COLOR` と `TERM=dumb` を尊重します。`never` は無効、`always` は
非TTYにも明示的に色を付けます。

`run` の `--exit-policy`:

- `first`: 結果出力順で最初の非ゼロ終了コード（接続失敗は255）
- `any`: 1件でも失敗すれば1
- `always-zero`: リモート失敗を0にするが、構文・ローカルI/O・内部エラーは非ゼロ

共通コードは成功0、ローカル/集約エラー1、構文・設定エラー2、
SIGINT 130、SIGTERM 143です。NDJSON summaryの `aggregate_exit_code` は
プロセス終了コードと対応します。

## 認証とknown_hosts

認証は原則としてSSH Agent、明示identity file、identity未指定時の既定
identity file群の順です。`--identities-only` はAgentを無効にします。
known_hosts検証は既定で有効です。
`--insecure-ignore-host-key` は中間者攻撃を許す危険なオプションなので、
接続先の鍵を別経路で確認できる場合だけ使用してください。

## 大量出力とスプール

リモート出力はプロセス全体で既定128MiBの共有メモリ予算を使い、超過分を
0700の一時ディレクトリ内の0600ファイルへ退避します。スプール総量の既定
上限は10GiBです。JSONもスプールからストリーミングして全出力を再保持しません。
上限超過、ディスクエラー、キャンセル時はSSHセッションを停止し、一時ファイルを
削除します。

```text
--max-buffer-memory 128MiB
--max-spool-size 10GiB
--spool-dir /path/to/private-parent
```

## 従来構文との互換性

トップレベルがフラグで始まる呼び出しは従来パーサーへ渡します。従来構文では
`-h` はhelpではなくホストファイルです。フラグ解析は最初の非フラグ引数で止まり、
残りを空白で連結します。また、非TTY stdinを自動転送します。

```bash
# 従来構文
gopssh -h hosts.txt -u root -p 10 -d uptime
# 正規構文
gopssh run --hosts-file hosts.txt --user root --parallel 10 --show-host -- uptime

# 従来構文
gopssh -h hosts.txt -c=false -s=false command
# 正規構文
gopssh run --hosts-file hosts.txt --color never --order completion -- command

# 従来構文
gopssh -h hosts.txt -i ~/.ssh/id_a,~/.ssh/id_b command
# 正規構文
gopssh run --hosts-file hosts.txt \
  --identity ~/.ssh/id_a \
  --identity ~/.ssh/id_b \
  -- command
```

従来構文のフラグ、既定値、出力形式、stdin、結果集約、終了コードは維持され、
非推奨警告は追加されません。

## インストール

macOS / Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/gopssh/main/install.sh | sh
```

既定では `~/.local/bin/gopssh` へインストールします。バージョンや宛先を変える場合:

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/gopssh/main/install.sh |
  GOPSSH_VERSION=v0.5.6 GOPSSH_INSTALL_DIR="$HOME/bin" sh
```

アンインストール:

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/gopssh/main/uninstall.sh | sh
```

## 開発・ビルド

```bash
make build
make test
make lint
```

リリースメタデータ付きビルド:

```bash
go build -ldflags \
  "-X main.version=0.6.0 -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date --iso-8601=seconds)" \
  -o .bin/gopssh ./cmd/gopssh
```

RPM/DEB生成時は `.bin/gopssh -version` の互換3行出力からメタデータを取得できます。

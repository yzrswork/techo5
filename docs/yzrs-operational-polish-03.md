# Operational Polish 03 — CODEX TEMP実測

受入基準HEAD: `6bf873868865a5521e4765b9a63f6bce1ad29bb1`。
受入済みrootfs SHA256: `53607a6abc9b72fde92511bbb5958ae65026cf7ba7d17f187cc1d18d99abb097`。
実機配備・boot target変更は別のHuman Gate。Draft PR #1はマージしない。

## 計測と周期

`tools/yzrs/codex-temp.ps1` は環境変数で解決した `%LOCALAPPDATA%\Temp` の直下の
`codex-*` ディレクトリだけを再帰走査する。Windowsのファイル長を64bit整数で合算し、削除・移動・書換えは行わない。
ファイルおよびディレクトリをOPEN_REPARSE_POINTで開き、reparseはたどらずpartialにする。
走査中は祖先ディレクトリの削除共有を許可しないハンドルを保持し、junctionへの差替えを防ぐ。
列挙後の消失・アクセス拒否・その他の読取障害はbytes:nullにする。
15秒、250,000エントリ、10,000ディレクトリ、深さ256、16 TiBが上限。上限到達は数値を無効にする。

`send-host-metrics.ps1` が実測JSONだけを専用 `/dashboard/host` に送信する（HTTP timeout 8秒・redirect禁止）。
credentialは既存と同じCurrent User DPAPIで `host-write-token.dpapi` に保管する。
診断ログは時刻と状態名だけ。個人ディレクトリ名、ファイル名、内容、秘密値は送らない。

`send-operational-metrics.ps1` は既存のhostとAI senderを別プロセスで順に実行する。
host 30秒・AI 65秒の強制上限を設け、一方が失敗・終了・ハングしても他方を実行する。
既存AI sender本体は変更しない。既存5分タスクとhidden VBSの呼出先だけをwrapperに変更する予定。
新規タスク・可視コンソール・Token Monitor設定変更は不要。
未来時刻拒否を緩めず実測時刻を保持するため、POST前に固定2秒の配送待機を置く。
本番Workerと専用credentialの準備完了までローカルの定期送信は切り替えない。

## データとHUD

GET `/dashboard` の任意 `host` はschemaVersion/source/measuredAt/codexTemp(bytes,status)/stale。
native readerは欠損・null・型・範囲・時刻・statusを検証し、不正なhostだけを破棄する。
TODAY/AIとは独立して有効hostのLKGを保持する。欠損・失敗・古い時刻の応答では元の観測時刻を保ち、staleにする。
offlineのLKGもstale表示。15分を超えた値は `CODEX TEMP STALE`、有効値なしは `UNAVAILABLE`。
実測0は `0 MB`、非ゼロで1 MiB未満は `<1 MB`。
MBはbytes/1,048,576、GBはbytes/1,073,741,824（binary conversion）。
MBは整数、GBは小数2桁。ラベルと値の座標・書体・幅176pxはそのまま。

## 絞った検証

- Windows: `test_codex_temp.ps1`（正確な合算・空・対象なし・上限・junction・ACL拒否・時間切れ・消失分類）。
- 周期独立性: `test_operational_sender.ps1`（host失敗/timeoutでもAI実行、AI失敗でもhost値を保持）。
- Worker: `tests/echo-dashboard/*.test.mjs`（認証分離、不正値、時刻、逆順完了、LKG、TODAY/AI互換）。
- Native: `go test ./internal/yzrs` と `go vet ./internal/yzrs`。
  実測/zero/stale/unavailableの書式と幅、全4モードでCODEX TEMP以外の画素が一致することを確認。
- `yzrs-preview -host-cases` は明示的fixture。`-now` で独立hostの検証時計を指定可能。
- Linux ARMv7/CGO無効でdaemonをビルドする。

## 候補作成と保全

`prepare-polish-rootfs.py` の受入hashを今回の承認済みimageに更新した。
候補は受入imageから `usr/local/bin/techo5` と `etc/techo5-release` だけを置換する。
全メンバーの一覧・既存内容・type/mode/uid/gid/linkname/mtimeを比較する。
フォントとライセンスは受入済みimageのものをそのまま保持する。
Slot A/B、slot manager、boot/recovery、watchdog、PTT、天気、navigationを変更する操作は含めない。
候補SHA256・source HEAD・実通信/fixtureの区別は最終成果物のmanifestに記録する。

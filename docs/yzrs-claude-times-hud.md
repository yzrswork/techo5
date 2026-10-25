# YZRS 左HUD Claude / YZRS Times と日本語STT強化

2026-10-25。左HUDのCodex表示（TOKENS TODAY / LIMIT REMAINING / CODEX TEMP / COMMITS / NOTES / HOST時刻 / AI STALE）を撤去し、上段にClaude Codeの利用枠、下段にYZRS Timesのトップ記事を置く。背景・時計・天気・TODAY・AI・VOICE・PTT操作は変更しない。

## 表示

- 上段: `CLAUDE 5H LEFT` の残り%リング、5時間枠のリセット時刻（JST）、`7D LEFT` の残り%
- リセット時刻を過ぎた枠は値が不明なので「—」。古い%を表示し続けない
- 下段: `YZRS TIMES <号数> 朝刊|昼刊`、トップ記事見出し（最大4行、英数字の単語は行をまたがない）、出典
- データがなければ `NO DATA`

## データ経路

既存の完全オンデマンド更新に相乗りする。新しい通信先・定期通信・KVアクセスは作らない。

1. Claude Codeの `statusLine` が `tools/yzrs/claude_statusline.py` を実行する。公式status line入力の `rate_limits.five_hour` / `seven_day`（Pro/Max、セッション初回応答以降）から残り%とリセット時刻だけを `%LOCALAPPDATA%\YZRS-TECHO5\claude-usage.json` へ原子的に保存する。認証情報は読まない
2. 実機の「更新」タップで、Windowsの `voice_bridge.py` が `claude` laneとしてこのファイルを読み、`times` laneとして公開JSON `https://yzrswork.com/data/latest.json` から `top[0]` の記事を取得する（認証なし、転送先がyzrswork.com以外なら破棄、512 KiB上限）
3. 両laneはallowlistで再構成し、既存の `dashboard_result` に追加して送る
4. 実機は検証済みの値だけを受理する。Claudeは観測時刻が戻らない場合、Timesは号数が戻らない場合のみ更新し、失敗時は前回値を残す。Worker GETからは受け取らず、ローカルキャッシュに保存して再起動後も復元する。既存のAI / HOST laneと更新状態の判定は変更しない

旧daemonは未知のlaneを無視するため、Windows側を先に更新しても互換。

## 日本語STT

`pc.json` の任意項目で指定する。未指定なら受入済みの `small`・ヒントなし。

```json
{
  "stt_model": "large-v3-turbo",
  "stt_vocabulary": ["Claude", "Codex", "Obsidian", "Vault", "GitHub", "プルリクエスト", "Echo Show", "TECHO5", "YZRS", "や印工務店"]
}
```

- `stt_model` は `small` / `large-v3-turbo` のみ
- `stt_vocabulary` は最大40語、1語32文字以内。`initial_prompt` として渡し、用語の表記と句読点の出力を促す
- RTX 3060実測（4.8秒の日本語）: small 0.30秒、large-v3-turbo + 語彙 0.39秒。kotoba-whisper v2.0は語彙ヒントで発話が欠落したため不採用

## 実機配備

受入済みsource `2805931` + `on-demand-runtime.patch` + 本変更のruntime差分でARM daemonをビルドし、受入済み `.20261012.1` rootfsのdaemonとreleaseだけを `prepare-polish-rootfs.py --accepted-sha256` で差し替える。latest mainの無関係な更新は実機へ入れない。

- rootfs `v1.1.1-yzrs.20261025.1` SHA256 `63969353f92fbfebf3ea26a12a3554e419b549f42e4b2d4a8db96a172e850731`
- daemon SHA256 `bfb261bf5662cae57c9b196fd2aff094ff909c1663e23cb3a8e072ecdf9eb035`
- ビルド: go1.27.0、`GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 GOFLAGS="-ldflags=-s -w" go build -trimpath`
- 2026-10-25 Human GO後、A切替・B導入・300秒watchdog自動good確定。Slot Aは変更なし

## 更新ログ

- 2026-10-25 左HUDのClaude / YZRS Times化、STTモデル・語彙設定、実機配備を記録。

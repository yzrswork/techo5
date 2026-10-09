# YZRS TECHO5 日本語 Dashboard / Windows PTT

対象は Echo Show 5 第1世代 `checkers`、960×480、TECHO5 v1.1.0。
上流基準は `32b47682bfbf08f6410f547ea76275535f18de2d`。
この変更は実装・PC検証済みの配備候補であり、実機受入完了ではない。

## 2026-10-09 Windows / native UI 継続検証

起動失敗の直接原因は、既存TLS identityだけが生成され、`pc.json` / `yzrs.json` が存在しなかったこと。
`prepare-config.ps1 -ReuseIdentity -PTTOnly` は既存鍵・トークンを再利用し、不足した設定だけを作成する。
既存JSONは上書きしない。ACLは現在ユーザーだけに制限する。PTT-onlyは予約 `.invalid` URLを使用し、
本番Worker表示の受入とは区別する。PowerShell 5.1で実行し、生成JSONを読み戻してからREADYを表示する。

起動診断はCONFIG / TLS / CUDA / DEPENDENCIES / MODEL_CUDA / HOTKEY等の固定カテゴリだけを出力する。
証明書不一致・認証拒否・不正WSS endpointは停止し、ネットワーク断だけを再接続する。
`start-voice.ps1 -CheckStartup` は実GPUモデルとTLS、キーボードhookを検証して終了する。
通常の `start-voice.ps1` は既存singletonを使って非表示起動し、私用ディレクトリに安全な診断を残す。
旧Butler F8クライアントとの共存は実音声操作が競合するため、試験時に旧クライアントを終了して切り替える。

`test_windows_readiness.py <loopback-cert>` は実GPU・hook登録・認証拒否・TLS拒否・本番F8 callback・
日本語認識・paste callback・Esc・singletonを検証する。F8をOS全体に送信しない。
`--reconnect` は別の認証TLS loopback serverで、本番runの切断→再接続→Escを検証する。
既存日本語PCMは私用領域で最後の20ms frameをゼロ埋めし、音声や認識結果は公開しない。

プレビューに `-cases` を追加すると、長い日本語、空TODAY、AI unavailable、OFFLINE/LKG、STALE、
NO DATA、VOICE listeningの明示的なfixtureを生成する。既定のVOICEはdisconnected / Deck unavailable。
AIのLKG表示はOFFLINEを優先し、直近キャッシュをLIVEと誤表示しない。
通常のCLOCK/TODAY/AI/VOICEと同じnative rendererで960×480の画像を生成する。

`start-trial.ps1` は私用 `readiness.json` の検証済みIP、serial、binary、SHA256を使う。
承認スイッチなしではHuman Gateを表示するだけで実機を変更しない。
承認後もupload前にstrict SSH・actual serial・A good/B empty・bind/trial/private stagingなしを再検証する。
ARMソース更新時は再ビルドし、manifestのSHA256を更新してから使う。
物理画面・実音声paste・修正版300秒watchdogの実機受入は承認後に行う。

## 構成

- `echod/internal/yzrs`: ハードウェアから独立したスキーマ検証、データ保存、描画、PTT。
- `echod/internal/feature/yzrs`: 既存 service supervisor、mic、Deck への接続。
- `echod/internal/feature/display/yzrs.go`: 既存画面へ差し込む限定的な描画・タップ経路。
- `tools/yzrs`: Windows PTT、設定準備、承認後の一時配備、PC受入テスト。

日本語は M PLUS 1p Regular（約1.76 MB、OFL）を固定して組み込む。ブラウザーや Android は不要。
通常の日本語グリフをテストし、長い文字列は文字単位で省略する。フォント外の記号は `?` に置換する。
4つの主モードは CLOCK / TODAY / AI / VOICE。NEXT は追加しない。
ネイビー、青・シアン、時計、固定 HUD。既存の設定、PIN、Wi-Fi、通話、アラーム等は優先する。
SETUP を押すと既存の設定経路に入る。日本語対応は YZRS の画面に限定する。

## Dashboard

Worker の既存 `GET /dashboard` を read token だけで取得する。
現在の `schema_version`, `data_date`, `generated_at`, `source.commit_window`, `today`, `activity`,
合成済み `ai.source / updatedAt / codex` を利用する。AI POST・snapshot publish・refresh token は不要。
Worker、KV、Token Monitor、sender、Actions は変更しない。

- 取得間隔: 60秒、1件ずつ、HTTP timeout 8秒。リダイレクトは禁止。
- 最大応答: 128 KiB。未知フィールドを保存せず、検証済みの表示モデルだけを保存。
- LKG: `/data/misc/techo5/yzrs-dashboard.json`、一時ファイル→sync→rename。
- LIVE: 取得成功、生成から2時間以内、JSTで当日。
- STALE: 生成から2時間超、または日付が前日。
- OFFLINE / LKG: 取得・検証・保存失敗、または再起動後で未取得。
- NO DATA: 有効な保存データがない。
- AI: `ok`, `stale=false`, 更新から15分以内で LIVE。失敗時には検証済みAI LKGをSTALEとして残す。
- 古い生成時刻の応答はLKGを上書きしない。

HUD の数値は実応答から表示する。null は `—`。CODEX TEMP は現行 Worker スキーマに存在しないため
`UNAVAILABLE`。独自の推定値、温度 HUD、端末画面は追加しない。

## PTT の通信と保護

Windows → Show の独立した `wss://<Show予約IP>:17327/ptt` を使用する。
Deck の Noise 暗号化制御は変更しない。PTT は TLS、独立32バイト乱数の bearer token、
明示したPCの予約IP、ブラウザーOrigin拒否、同時PC1台で保護する。
自己署名証明書は Show IP を SAN に含め、PC側はその証明書を信頼する。TLS検証は無効にしない。
上流のWi-Fi firewallは新規ポートを閉じているため、実行中だけ専用 `YZRS-PTT` chainを挿入し、
指定PCのIPv4から17327/tcpだけを許可する。終了・試験rollback時に専用chainを除去する。
上流 `TECHO5-IN` の既存ruleはflush・置換しない。PTT設定は予約IPv4・固定17327ポートに限定する。
Home Assistant の暗号鍵をPTTへ流用しない。秘密値はログ・リポジトリへ出さない。
証明書は1年。期限前に所有者管理で更新する。ルーターで Show/PC のIP予約が必要。

F8 DOWN → UUIDセッション START → `mic.Get().Listen("yzrs-ptt")`。
受信する `[]int16` は16 kHz / mono / 320 samples（20 ms）。PCM16LEへ変換して送る。
F8 UP → STOP ACK → Windows側RAMの音声を faster-whisper small / CUDA / float16 へ渡す。
日本語テキストは既存方式の clipboard + Ctrl+V で現在の入力欄へ送る。

- Windowsのキーイベントは非同期キューで保持し、START ACK前の短いKEY_UPもSTOPとして送る。
- autorepeat・二重STOP・同じ直前IDの再STARTは重複処理しない。
- heartbeatは2秒、受信無通信6秒で接続を閉じてmic listenerを解除。
- 録音上限60秒。超過時はセッションを破棄する。PCの上限PCMは1.92 MB。
- socket書込timeout 2秒、制御メッセージ512 bytes、キュー8件。
- TRANSCRIBING状態は最大180秒。完了・失敗・切断でIDLEへ復帰。
- 切断中の認識結果は貼り付けない。再接続で押し続けていたF8から勝手に録音を再開しない。
- GPU処理中も通信・キー監視を継続する。切断後も残るGPU呼出は排他し、同時推論を開始しない。
- 音声PCM/WAVを永続ファイルへ書き出さない。デジタル無音/ミュートはSTTへ送らない。

Windowsクライアントは localhost:17328 の排他的ソケットで重複起動を防ぐ。
このポートはロック用途だけであり、制御APIではない。
他のアプリが同ポートを使用中の場合は起動できない。旧Android PTTとのF8競合を避けるため、
所有者の実機切替時に旧クライアントを終了する。既存スタートアップファイルは自動上書きしない。

## VOICE / Deck

VOICEタップは録音開始ではなく、設定済みDeckの `pc_run` ボタンを実行する。
ページ・ボタンは0始まり。既にpair済みのPC、PC側 `scripts.txt` の許可済み名前だけを利用する。
PC側の script 名は例として `YZRS Voice` とする。`ensure-voice.ps1` を指す行をPCで登録する。

```text
YZRS Voice = powershell.exe -NoProfile -File "<checkout>\tools\yzrs\ensure-voice.ps1" -Python "<venv>\Scripts\pythonw.exe" -Config "<private>\pc.json"
```

Showの既存 SETUP → Screen & Photos → Deck → Computers でpairし、Run scriptを上記の名前へ設定する。
`voice_deck_page` と `voice_deck_button` をその位置へ合わせる。
画面にはPC CLIENT ACTIVE / UNAVAILABLE、IDLE / LISTENING / TRANSCRIBING、Deckの状態を表示する。
スクリプトの実行ACKはクライアントの起動完了を意味しない。ACTIVEはPTT接続から判断する。
Deck agentのpairing keyを公開しない。既存settings lockを維持する。

公式 v1.1.0 Deck agentを使用できる。非表示常駐は `wscript.exe` からVBSを使用する。
Deck本体の標準 `-startup on` がコンソールを表示する場合、所有者管理で非表示起動経路へ切り替える。
PTTの `start-hidden.vbs` は pythonw.exe / voice_bridge.py / 私用設定JSONの3引数を取る。
Startup登録は実機受入後に行う。まず手動試験を通す。

## ビルド・PC検証

Go 1.26.8以上。最初に `echod` へ移動する。

```powershell
go test ./internal/yzrs
```

```powershell
go run ./cmd/yzrs-preview -snapshot internal/yzrs/testdata/dashboard.json -out ../bin/preview
```

ARMビルドは環境変数を1コマンドずつ設定する。

```powershell
$env:GOOS = 'linux'
```

```powershell
$env:GOARCH = 'arm'
```

```powershell
$env:GOARM = '7'
```

```powershell
$env:CGO_ENABLED = '0'
```

```powershell
go build -trimpath -o ../bin/echod-arm ./cmd/echod
```

Linux CI は `go test ./...`、dot/spot回帰、race、ARMビルド、PC描画、TLS P2Aを実行する。
Windowsでは上流Linuxハードウェアパッケージのため全体 `go test ./...` は実行できない。
追加した portable package と Windows client は直接テストする。
音声テストは `tools/yzrs/test_voice_bridge.py`、実通信は `test_p2a.py`。
任意の既存私用PCMによるCUDA検証は `test_gpu_p2a.py`（PCだけ、入力欄への実貼付はしない）。

## 私用設定

私用ファイルはリポジトリ・出力配布ZIPに入れない。`prepare-config.ps1` は新規ディレクトリの
Windows ACLを現在のユーザーだけに制限し、read tokenを非表示入力で受け取る。
`yzrs-identity` は独立TLS identityを生成し、既存ファイルを上書きしない。
Showの設定は `/data/misc/techo5/yzrs.json`、PC設定は任意のACL保護済み場所の `pc.json`。
一時試験用の証明書・鍵は `/tmp/yzrs-private/`。正式常用時は `/data/misc/techo5/yzrs-private/`
へ移し、設定内パスも変更する。Show側秘密ファイルは0600、ディレクトリ0700。

PTT未設定なら空の `ptt_addr` でDashboardのみ使用できる。設定なし/disabledなら上流画面のまま。
設定変更はデーモン再起動で反映する。Wi-Fiアドレス未取得時もDashboardは描画し、PTT bindを2秒ごとに待つ。
Dashboardと音声の復帰にHome Assistantは不要。

## 実機配備のHuman Gate

既存Slot Aのhealthy baselineを維持する。installer、boot/recovery flash、mkstore、wipe、forceは禁止。
初回候補は上流 `docs/building.md` のデーモンbind試験を限定した5分の一時試験。
所有者承認を得るまで `deploy-trial.ps1` を実行しない。

必要事項: SSHを所有者の鍵で有効化、host fingerprint確認、IP予約、private設定、binary SHA256。
`deploy-trial.ps1` は明示 `-HumanApproved` と SHA256 を要求する。
Windows OpenSSHと実機Dropbearの互換性のため、curve25519-sha256とlegacy SCPを使用する。
USB経由で確認した公開ホスト鍵を私用known_hostsへ保存した場合は、`-KnownHosts <path>` を指定できる。
ホスト鍵検証は常に有効。実機shellとwatchdogは管理コマンドのPATHを明示する。
実機側でも active=a / booted=a / a=good / b=empty、既存bind mountなし、SHA一致を確認する。
別状態ならSTOP。状態を合わせるためのslot操作はしない。

実行時は `/tmp/yzrs-<SHA>.elf` を既存 `/usr/local/bin/techo5` へbindし、デーモンを再起動する。
5分後にSSH接続と独立したwatchdogがunmountし、元の設定とデーモンへ戻す。
実行中のbinaryで通常unmountがbusyになった場合はlazy detachし、元imageが見える状態でdaemonを終了する。
元のYZRS設定は `/data/misc/techo5/yzrs-trial-backup/` に保全する。
早期復帰はそこにある `rollback.sh` を実行する。
試験中にrebootすればbind mountは消える。設定復元は保全したrollback.shで行う。
Slot A / B、boot、recovery、storeは変更しない。
デプロイスクリプト自体の実機実行は受入前に未検証であるため、USBコンソールを退路として確保する。

試験後も保全ディレクトリは残し、同じ試験を重ねられないようにする。
再試験は復帰結果を確認してから所有者が保全ディレクトリを別名で保管する。
rootfs常用配備はこの試験PASS後に別のHuman Gateで行う。上流slotctlの空きB trial/fallbackを利用し、
Aを上書きしない。カスタムrootfsの作成・署名・実機rollbackは本候補のPC検証には含まれない。

## 実機受入（未実施項目はPENDING）

| 項目 | 確認 |
|---|---|
| baseline | 既存A good / B emptyを保護し、起動・touch・表示を確認 |
| UI | CLOCK/TODAY/AI/VOICE、日本語、省略、HUD、SETUP、PIN、表示960×480 |
| Worker | 実read tokenでLIVE、AI、sync、Worker断でLKG、復帰、restart後復元 |
| P2B | F8 DOWN/UP、実機内蔵mic、CUDA日本語認識、現在の入力欄へ貼付 |
| PTT連続 | 短押し、autorepeat、10回連続、重複なし、60秒上限、ミュート |
| PTT障害 | Wi-Fi断、PC終了、F8保持中切断、再接続で勝手に録音しない |
| Deck | VOICEでensure-running、2回押してもGPUモデル1個、unpaired/offline表示 |
| 常用 | 非表示Windows起動、daemon restart、Wi-Fi復帰、CPU/RSS実測 |
| 退路 | 5分watchdogまたは手動rollback後、上流daemonとA goodへ復帰 |

PCテストPASSだけで上記をPASSへ変更しない。
上流の初回Hello案内は維持するため、本体画面で閉じてからDashboardを確認する。
checkersの物理ミュートがON（amazon-gating/state=1）ならPCMはゼロになる。
解除は所有者が本体ボタンで行う。ソフトウェアによる解除やバイパスは行わない。

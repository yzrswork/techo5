# YZRS 完全オンデマンド更新

2026-10-12。CLOCKはローカル時刻だけで描画し、起動時も待機中もDashboard GETを行わない。タブ切替はキャッシュ表示のみ。右上の「更新」（118×48px）を押すと、最大65秒の更新を1回開始する。処理中と完了後5秒の連打は無視する。

## データ経路

1. 既存のread tokenでWorker `GET /dashboard`を1回実行。Vault生成時刻はそのまま保持し、GitHub Actionsは起動しない。
2. 既存のPC IP制限・bearer認証・証明書検証付きTLS `/ptt` WebSocketで、固定操作 `dashboard_refresh` とランダム32桁IDを送る。PCが未接続なら直ちに失敗し、接続済みの場合も55秒で打ち切る。
3. WindowsはAIの既存Token Monitor extractorとHOST `-MeasureOnly`を並列実行（20秒/30秒上限）。GPU文字起こしとheartbeatは継続する。新しいcommand/service/public endpointは作らない。
4. 同じWebSocketで `dashboard_result`、ID、native AI/HOST allowlistを返す。ID一致、タップ開始以降の実測時刻、未来不可、schema、status、独立した単調増加を確認し、検証済みローカルキャッシュへ原子的に保存する。
5. 失敗・古い結果・欠落したlaneは前回値をstaleとして残す。値や生成時刻を捏造しない。Worker失敗時も前回TODAYが残る。無限再試行・完了確認のCloudflare pollingはない。

Windowsへ配布するruntimeは `voice_bridge.py` と同じフォルダの `send-ai-usage.ps1`。PowerShell 7の`pwsh`が必要。HOSTの既存installed `send-host-metrics.ps1` / `codex-temp.ps1`、DPAPI資格情報、PTT設定・証明書・鍵は保持する。AIスクリプトの通常POST互換は保全するが、LAN経路ではPOSTしない。VOICE選択に伴う既存Deck起動の意味は変更しない。

## KV予算（推計、Cloudflare実測値ではない）

無料枠はRead 100,000、Write/List/Delete各1,000/日。[公式料金](https://developers.cloudflare.com/kv/platform/pricing/)。KVはaccount共通。UTC 00:00（JST 09:00）リセット。

Dashboard手動GETはRead最大5（base1、AI latest/LKG最大2、HOST valid/failure最大2）、List2、Write0、Delete0。AI/HOSTのLAN更新はKVアクセス0。放置24時間はDashboard操作の4種すべて0。Workerのreverse-time HOST key・valid/failure lane・30日TTLは変更しない。既存keyはTTLに従って自然に失効し、新経路では増えない。

| 更新/日 | Dashboard Read上限 | Write | List | Delete |
|---:|---:|---:|---:|---:|
| 0 | 0 | 0 | 0 | 0 |
| 10 | 50 | 0 | 20 | 0 |
| 30 | 150 | 0 | 60 | 0 |
| 100 | 500 | 0 | 200 | 0 |

移行前の稼働モデルはGET1,440回、Windows HOST/AI288回ずつ：Readは正常経路4,608程度～保守上限7,776、List3,456、Write864、Delete0/日。失敗や手動操作は別枠。

継続する独立サービス：Vault scheduled snapshot5回/日＝Read最大25、Write5、List10、Delete0。Commerce hourly24回/日＝OAuth Read48、snapshot Write24、OAuth cache missが毎回なら追加Write24（通常Write24～48）、List0、正常時Delete0。Commerceエラー時の明示Delete/追加認証は別枠。Vault main push等の追加公開P回＝Read最大5P、WriteP、List2P。Commerce公開GET Q回＝Read Q。ほかのaccount利用Uは未計測。

既知の定期サービスを加えた保守予算：Read ≤ 5N+73+5P+Q+U_read、Write ≤ 53+P+U_write、List = 2N+10+2P+U_list、正常Delete = U_delete。N=100、P=Q=U=0ならRead573、Write53、List210、Delete0。これは残りのaccount利用がこのheadroom内という条件での無料枠維持であり、account全体0アクセスの主張ではない。

## 移行・受入・rollback

実機rootfs/Slot変更は対象artifact/hashを示したHuman Gate後。Slot Aを保護し、稼働中Bを直接上書きしない。B更新後は300秒watchdogの自動good確定を待つ。現在のmainには受入済みimage以外のruntime更新もあるため、実機候補は受入済み`.5` sourceへ本PRのruntime差分だけを適用し、rootfsでdaemon/release以外のmember・metadataが一致することを確認する。

旧WindowsタスクXML・installed launcher・startup shortcutを秘密値を出さず保護済みlocal backupへ保全。新client実測と実機の明示タップでLAN結果・時刻表示・連打抑止が通ってから `YZRS Echo Dashboard AI Usage` を無効化する（削除しない）。XMLだけの変更をinstalled変更とは扱わない。PC不在、Worker不在、4画面、touch、F8実発話を受入し、PRは実機受入後にmergeする。

Workerは変更しないためdeploy/rollback不要。Windows rollbackは旧bridge/shortcutに戻しても5分senderタスクを無効のまま保つ。実機は旧B archiveを保持し、重大障害では `STORE=/store slotctl rollback; sync; reboot` でAへ退避。ただし旧B再導入は60秒GETが復活するため、安全な一時構成（Dashboard機能停止、または当該read経路の遮断）を併用する。Slot Aは退路として保護し、polling再開を無言で許可しない。ローカルvalidated cacheとPTT identityを削除しない。

## 更新ログ

- 2026-10-12 完全オンデマンド更新の通信・予算・受入・rollback条件を記録。

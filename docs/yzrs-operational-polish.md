# YZRS TECHO5 Operational Polish 01

2026-10-10。既存Draft PR #1のブランチで実装。Slot A/Bは読み取り確認のみ。

## 対象変更

- ネイティブ時計を主領域中央へ配置し、日本語日付と天気欄を分離。
- HUDの実測セッション残量を時計回りのリング表示。STALE/OFFLINE/欠損を明記し、消費率へ反転しない。
- CLOCK/TODAY/AI/VOICEを110×86px、間隔14pxのアイコン付きタイルへ変更。描画とタップ判定は共通矩形。
- 数値をカンマ区切り。TODAYは独立した時刻列と最大2行の本文、3件表示＋残件数。元の本文・snapshotは変更しない。
- 既存home.Weatherを描画へ渡す。HAまたは設定済みhome.placeのOpen-Meteoを使用し、既存30分取得を維持。取得時刻を伴うキャッシュは45分を超えると経過分を表示、3時間を超えると既存の非表示判定を維持。
- Token Monitor 0.66.0の標準trayMode/showTrayIconとWindows Startup shortcutを利用。アプリ内startAtLoginが既に有効な場合、追加shortcutを作らない。常駐中の設定ファイル書き換えはhelperが拒否する。
- 受入済みSHA256固定のrootfsから、ARM daemonとrelease識別子だけを差し替えるローカル準備helper。全件の内容・権限・所有者・リンク・mtimeを照合。端末には接続しない。

## 調査結果

Token MonitorのtrayMode=false、startAtLogin=false、ローカル17321 API停止を確認。 senderは既存APIへ依存し、独自収集は行わない。標準トレイモードで起動すると表示ウィンドウなしでAPIが復帰。Window Closeは標準コードでhide、Quitは収集プロセス終了であり別動作。

既存wscript→VBS→pwsh、5分周期、DPAPI、Worker認証は変更していない。07:56:18と08:01:18 JSTのWorker AI updatedAt進行をGETで確認。既存実機のネイティブLKGキャッシュでも07:56:18の更新を確認。LastTaskResult=0だけを合格根拠にしていない。

senderログは手動の非表示送信成功を記録するが、定期実行によるWorker更新はログへ反映されなかった。ログ欠落の原因は未確定。VBS診断とOSフォーカス監視は自動承認レビューがblocked by policyで拒否したため、フォーカス奪取なし・黒窓ちらつきなしの実測は未完了。

既存端末state.jsonのhome.placeは空。場所を推測せず天気の中立fallbackを使用する。場所は既存SETUPで設定可能。新しい天気バックエンドやWorker変更は不要。

TODAYは2026-10-10の通常snapshotを確認。午前0時直後のSTALEは現在再現せず、日付・2時間判定を変更していない。

## 検証と制約

- PASS: Windows portable yzrs tests/vet、Linux yzrs/home/display限定テスト、12ケースのネイティブプレビュー、差分チェック。
- PASS: API復帰、5分周期でのWorker AI更新、既存実機キャッシュへの通常refresh、active/booted B、A/B good、store read-only。
- CONDITIONAL GO: ログイン起動設定の準備。通常再ログインとフォーカス・ちらつきの実測は未完了。
- CONDITIONAL GO: UI候補と天気fallback。新UIの実機描画・指タッチ、設定された地域の実天気は未受入。
- STOP: 実機rootfs更新・boot target変更は今回のHuman Gate未承認。Slot Aへは書き込まない。PRはDraft、マージ禁止。

受入済み星雲/PCB背景の未コミット変更を作業前に保全し、そのまま含めた。既存echod/workは変更・コミットしない。指定の写真と参考図、RTK.mdは提供場所で見つからず、現行描画を基準にした。

## 次回候補（今回未実装）

1. P2 / 小: Worker更新は確認できる一方、定期senderログが増えない。既存ログの保存先・書込失敗だけを限定診断し、機密を含まない結果記録を確実にする。無人運用時の原因追跡が容易になる。
2. P1 / 小: 既存deploy-slot-b.ps1はA起動/B empty専用の初回導入条件で、現在のB good更新には使えない。次の承認済み更新時に、Aを保護した既存slotctl更新手順を現在状態に合わせて限定整備する。受入済み端末への誤操作を防ぐ。

## 最終配備・受入（2026-10-10）

Human Gateの同一対象/候補/Slot A保護範囲に明示承認を受け、前回成功したdevice-update.ps1と端末slotctl実装を照合して再利用。Slot B goodは更新不可を意味しない。初回導入用deploy-slot-b.ps1の制約であり、既存の更新手順で対応できる。

Verified SSH/serial、承認済み候補SHA一致、active/booted B、A/B good、Aのdaemon/release/boot.shと常用設定の指紋を確認。支持済みslotctl switch a→実際のA起動→承認候補の転送・hash照合→非稼働Bへslotctl install→導入daemon hash照合→B起動。追加のrollback/stress試験や手動commitは行わず、既存300秒機構がuptime 317.53秒で自動確定した。

最終active/booted B、A/B good、store read-only。Aの確認済み指紋と常用設定は一致し、boot/recoveryの変更なし。適用rootfs SHA256: b6a24fecfe83521175016c938ae07feb0c652db115a93e58e9b6e32d0f8d45c7、daemon source a01fef4。

所有者が4タイルの指touch/4モード切替・表示の重なり・残量ring・桁区切りを確認。F8は当初使えなかった。Windows native PTT clientが停止し、Startup linkが存在しない$PSHOME/powershell.exeを指していることを確認。PowerShell 7でinstaller実行時に存在しない実行先を組み立てる小さな不具合を修正し、Windows標準PowerShellの存在を検証して登録。既存常用clientを起動してWSS接続/READY/IDLEを確認、所有者の代表的F8操作で実入力が成功した。モデル・PTT protocol・認証・rootfsは再変更していない。

UI/指touch/F8/実データrefresh/安全更新: PASS。Windows通常ログイン時の前面化/黒窓観測のみPENDING、天気地域設定は任意。全体CONDITIONAL GO。PRはDraftで未マージ。framebuffer dumpは起動ロゴの残像だったためUI受入証拠に使用していない。

上記の配備前STOP/Human Gate待ちとP1更新手順提案は、この最終結果で解消。追加提案は既存senderログの欠落診断（P2/小）のみで、今回未実装。

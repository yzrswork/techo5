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

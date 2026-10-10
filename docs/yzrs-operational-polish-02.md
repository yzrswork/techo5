# Operational Polish 02 — 表示調整

基準HEADは `16c63f4f0469e68c40eae23fc8239bd21f35e48b`、受入済みrootfsは
`b6a24fecfe83521175016c938ae07feb0c652db115a93e58e9b6e32d0f8d45c7`。
このタスクはソース・プレビュー・候補ビルドまで。実機配備とboot target変更は未承認。

## 変更

- HUDの文字開始をx=24から36へ移動（+12px）。幅176px、リングも同じ領域の中央へ移動。文字サイズは維持。
- OFFLINE / LKGの長い状態名がリング線に重なることをプレビューで確認したため、状態名をリング直下（baseline y=256）へ移して解消。
- HUD背面だけを紺色で穏やかに減光。外周PCBと主領域の星雲は維持。背景は初期化時に生成し、毎フレームの処理を増やさない。
- 時計112px、ASCIIだけの英数字24/16pxにChakra Petch Regularを追加。78,488 bytes、未改変、SIL OFL 1.1。出典・固定hash・ライセンスはassetsに同梱。
- 日本語・日本語混在行はM PLUS 1p 24/16pxを維持。中央寄せの幅測定にも実際に選択した書体を使い、位置ずれを防ぐ。
- VOICEからDECK状態の描画だけを削除。IDLE/LISTENING/TRANSCRIBING、PC CLIENT表示、既存説明文を維持。Deck実装・設定・VOICE操作・F8経路は変更なし。
- 4タイルの矩形・順序・アイコン・タッチ判定は変更なし。

## 検証

- `go test ./internal/yzrs`、`go vet ./internal/yzrs`。
- ASCII glyph、99,999,999（24px）、UNAVAILABLE（24px）、OFFLINE / LKGとCODEX TEMP（16px）のHUD幅、時計の字形境界、日本語書体維持を確認。
- DECK状態を変えてもVOICE画像が一致すること、PTT phaseとPC接続状態が表示に反映されることを確認。
- native 960×480プレビュー14ケース：4モード、長い日本語、空活動、AI unavailable/offline/stale、NO DATA、VOICE listening/idle/transcribing。
- Windows AMD Ryzen 7 5700Xの100フレーム計測は変更前0.863ms/frame、変更後0.817ms/frame。大きな低下は観測なし。実機性能の測定ではない。

提供された添付には指示テキストだけがあり、参照コンセプト画像と実機写真は取得できなかった。
書面の方向性に沿ったプレビュー確認であり、参照画像への一致や今回UIの実機受入は未確認。
プレビューは固定fixtureの値で、実Workerや現在の天気を表すものではない。

## 配備境界

Draft PR #1は未マージ。次のrootfs候補は受入済みimageを基にdaemonとrelease識別子を差し替え、両書体の著作権・OFLを通常のテキストファイルとして追加する。既存の他メンバーの内容と保護metadataを比較する。
候補の具体的SHA256とsource HEADはローカル成果物のmanifestに記録する。
対象はSlot B、Slot Aを保全。配備には今回の候補に対する明示的Human Gateが必要。
場所設定 `Setagaya City, Tokyo`、Worker/Vault/Token Monitor/Startup/音声/slot/boot/recoveryには手を加えない。

追加提案なし。

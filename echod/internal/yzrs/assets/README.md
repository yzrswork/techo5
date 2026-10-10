# Japanese font provenance

- Font: M PLUS 1p Regular, M+ Fonts Project
- License: SIL Open Font License 1.1, included as `OFL.txt`
- Source: https://github.com/google/fonts/tree/d714b17ce2379f06daf6295617f961df605dccb5/ofl/mplus1p
- File: `MPLUS1p-Regular.ttf`, 1,758,688 bytes
- SHA256: `2f294ad496432b1608f070d310e3aa2adcf1de4af429f4901df97ec4bd361ed1`
- Unmodified, embedded only in the YZRS raster renderer; two fixed Japanese sizes (24px/16px); Japanese coverage is unchanged.

Everyday Japanese glyphs are checked by the renderer test. Width truncation uses Unicode runes and
an ellipsis. Symbols outside the font are rendered as `?` rather than missing-glyph boxes.

# Selected Latin/numeric font (Operational Polish 02)

- Owner selected Candidate A: Inter v4.1, unmodified static TTFs from the official release.
- Source: https://github.com/rsms/inter/releases/tag/v4.1
- License: SIL Open Font License 1.1, included as `Inter-OFL.txt` with copyright.
- Clock: `Inter-SemiBold.ttf` (weight 600), 419,744 bytes, SHA256 `78a843fade9d4612a5567302fb595b56976eb5fcebf4fea5a5912d638bafcde3`.
- ASCII labels/metrics: `Inter-Medium.ttf` (weight 500), 417,300 bytes, SHA256 `97ad806f526e41546d46365bb3a393145f75b7b1568913db74549ad8b8dba872`.
- Fixed sizes: clock 112px, ASCII 24px/16px. Mixed/Japanese strings retain M PLUS 1p.
- Default numerals are proportional. Existing whole-string centering remains in use; no OpenType feature subsystem is introduced.
- Fonts do not come from an installed application. Copyright and OFL must accompany binary redistribution, including future rootfs images.

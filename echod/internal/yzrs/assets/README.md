# Japanese font provenance

- Font: M PLUS 1p Regular, M+ Fonts Project
- License: SIL Open Font License 1.1, included as `OFL.txt`
- Source: https://github.com/google/fonts/tree/d714b17ce2379f06daf6295617f961df605dccb5/ofl/mplus1p
- File: `MPLUS1p-Regular.ttf`, 1,758,688 bytes
- SHA256: `2f294ad496432b1608f070d310e3aa2adcf1de4af429f4901df97ec4bd361ed1`
- Unmodified, embedded only in the YZRS raster renderer; two fixed Japanese sizes (24px/16px); Japanese coverage is unchanged.

Everyday Japanese glyphs are checked by the renderer test. Width truncation uses Unicode runes and
an ellipsis. Symbols outside the font are rendered as `?` rather than missing-glyph boxes.

# Geometric Latin/numeric font (Operational Polish 02)

- Chakra Petch Regular, unmodified, 78,488 bytes.
- License: SIL Open Font License 1.1, bundled as `ChakraPetch-OFL.txt` including copyright.
- Pinned source: https://github.com/google/fonts/tree/bd8f81ddb5c74d5c8897b36ad88b440266245103/ofl/chakrapetch
- SHA256: 98fcd638baa5c81ff0316b7538ce330ee3b23b1302726de3526d5933a8ecf986
- Used at 112px for clock digits and 24px/16px for wholly ASCII metrics/labels. Mixed/Japanese strings retain M PLUS 1p, including missing-value em dash.
- Source does not come from an installed application. The license and upstream font are redistributed together; the font is not sold separately.

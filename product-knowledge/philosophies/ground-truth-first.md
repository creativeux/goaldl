<!--
GLaDOS-MANAGED PHILOSOPHY
Last Updated: 2026-07-04
Status: Confirmed by user 2026-07-04
-->
# Ground truth first

Every claim about the protocol or the data must be anchored to evidence from the real vehicle: the WinALDL log (`data/20250601_111156_LOG.txt`), the committed real captures, or a documented spec (`data/A033.ads`, Tech Edge aldl160). A model that merely "works on synthetic data" is unproven — synthetic data shares the decoder's assumptions and cannot falsify them.

Corollaries:
- Unverified assumptions are named explicitly and tracked (e.g. the `MapVoltsToKPa` transfer is flagged as the one BLM assumption not yet checked against WinALDL).
- When theory and capture disagree, the capture wins; re-derive the theory (this is how the months-long timing-decoder dead end was finally broken).

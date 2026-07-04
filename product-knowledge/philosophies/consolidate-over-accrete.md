<!--
GLaDOS-MANAGED PHILOSOPHY
Last Updated: 2026-07-04
Status: Confirmed by user 2026-07-04
-->
# Consolidate over accrete

Keep one working core; delete experiments once they've taught their lesson (git history is the archive). The 2026-07-03 consolidation cut 23 experimental tools, 6 dead decoders, and legacy subcommands (7214 → ~1775 lines) down to the validated pipeline — and the project got *more* capable afterwards, not less.

Corollaries:
- New capability grows as a consumer of the existing core (Session API), not as a parallel path.
- Historical documents that predate a corrected understanding are treated as record, not guidance (e.g. `DECODER_STATUS.md` in git history).
- Preserve knowledge in doc comments and CLAUDE.md where it's load-bearing; preserve code only while it earns its place.

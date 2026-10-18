# Harness abort — not a Phase 0 run

The commander started the Phase 0 driver as a child of a 30-minute monitor process. That would have killed any run
still in progress when the monitor expired. The commander stopped the driver's process group (SIGTERM) after about
10 seconds of wall time and restarted it detached (`nohup`). This first P-US C-whole r1 session had started and
produced tool calls, but wrote no assessment. It was stopped by the harness, not by an error, the budget, or the
investigator.

It is kept, unscored, and was not read for scoring. P-US C-whole r1 was run again from scratch, in a new random live
copy, as the first scored run. Disclosed in COMMANDER.md and GATE.md.

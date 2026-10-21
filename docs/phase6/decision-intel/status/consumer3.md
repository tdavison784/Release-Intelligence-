# Lane consumer3: status (Milestone 1 fix, ledger D-0006)

Branch `p6di/consumer3` (from `p6-decision-intel` @ 9296a213). Brief: `briefs/consumer3.md`.

## Done

- Golden graphs of consumer rules v1 and v2 (`relations/testdata/consumer-rules/`), written by the code before
  any v3 change, for three environments (plain, generated data, pod-spec paths), both sides.
  `TestConsumerRulesGolden` checks every later build against them byte for byte.

## In progress (exact next step)

- Rules v3 in `relations/consumers.go`: pod-spec paths (command, args, literal env values) matched against the same
  container's mounts, `reads-file` edges provider → workload.

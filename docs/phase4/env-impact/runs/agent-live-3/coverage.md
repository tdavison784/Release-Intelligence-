| # | item | agenda rule | disposition | examined | inspections | validation requests | records | summary |
|---|---|---|---|---|---|---|---|---|
| 1 | ci-0a5e2fb2331c | agenda:rendered-unexplained | raises-concern | false | 0 | 0 | 1 | Same 90.x credential change as ci-194d5b487655 for kube-controller-manager SM; env uses ignoreNamespaceSelectors: true so the chart-created Secret is in the wrong namespace. |
| 2 | ci-1871449ec07f | agenda:rendered-unexplained | raises-concern | false | 0 | 0 | 1 | coredns SM now uses authorization via the prometheus-token Secret; SM lives in kube-system while Secret is in monitoring. |
| 3 | ci-194d5b487655 | agenda:behaviour | raises-concern | true | 1 | 1 | 2 | Upstream states that with ignoreNamespaceSelectors the chart-created Secret is in the wrong namespace; env sets it true. Override-inspection found no pin. |
| 4 | ci-1e94c8421137 | agenda:rendered-unexplained | raises-concern | false | 0 | 0 | 1 | kube-scheduler SM: same credential/namespace mismatch risk. |
| 5 | ci-1f035001bfe6 | agenda:rendered-unexplained | raises-concern | false | 0 | 0 | 1 | kube-etcd SM: same credential/namespace mismatch risk. |
| 6 | ci-51933c83f5d4 | agenda:rendered-unexplained | raises-concern | false | 0 | 0 | 1 | kube-proxy SM: same credential/namespace mismatch risk. |
| 7 | ci-555bf9fefdf5 | agenda:rendered-unexplained | raises-concern | false | 0 | 0 | 1 | apiserver SM in namespace default references Secret rendered in monitoring. |
| 8 | ci-903ca29715e3 | agenda:rendered-unexplained | raises-concern | false | 0 | 0 | 1 | kubelet SM in kube-system references Secret kube-prometheus-stack-prometheus-token rendered in monitoring. |
| 9 | ci-a56630d1b23c | agenda:rendered-unexplained | not-examined | false | 0 | 0 | 0 | Grafana grafana.ini change appears only in chart-default render, not environment render; not examined. |
| 10 | ci-aaeb79bed47e | agenda:rendered-unexplained | raises-concern | false | 0 | 0 | 1 | Operator --secret-field-selector no longer excludes service-account-token Secrets, so the operator now watches them (needed for the new token Secret); broadens operator secret visibility. |
| 11 | ci-ac26042a3a1c | agenda:rendered-unexplained | raises-concern | false | 0 | 0 | 1 | New long-lived non-expiring SA-token Secret in monitoring; not usable by SMs in kube-system/default under ignoreNamespaceSelectors. |
| 12 | ci-0f8475f72a68 | agenda:pass-to-scrutinise | not-examined | true | 1 | 0 | 0 | Join says value unset in env (not-affected); not examined further. |
| 13 | ci-186eefc98ab9 | agenda:pass-to-scrutinise | not-examined | false | 0 | 0 | 0 | Join: value unset; not examined further. |
| 14 | ci-1b14cf0134ba | agenda:pass-to-scrutinise | not-examined | false | 0 | 0 | 0 | Join: value unset; not examined further. |
| 15 | ci-1d8a464ef1b7 | agenda:pass-to-scrutinise | not-examined | false | 0 | 0 | 0 | Join: value unset; not examined further. |
| 16 | ci-2ecd3769e47d | agenda:pass-to-scrutinise | not-examined | false | 0 | 0 | 0 | Join: value unset; not examined further. |
| 17 | ci-337251f802e5 | agenda:pass-to-scrutinise | not-examined | false | 0 | 0 | 0 | Join: value unset; not examined further. |
| 18 | ci-4fafc7794273 | agenda:pass-to-scrutinise | not-examined | false | 0 | 0 | 0 | Join: value unset; not examined further. |
| 19 | ci-5df55229fb90 | agenda:pass-to-scrutinise | not-examined | false | 0 | 0 | 0 | Join: value unset; not examined further. |
| 20 | ci-63ebfdbffcaf | agenda:pass-to-scrutinise | not-examined | false | 0 | 0 | 0 | Join: value unset; not examined further. |
| 21 | ci-779e8207da5f | agenda:pass-to-scrutinise | not-examined | false | 0 | 0 | 0 | Join: value unset; not examined further. |
| 22 | ci-93f70f86bcd3 | agenda:pass-to-scrutinise | not-examined | false | 0 | 0 | 0 | Join: value unset; not examined further. |
| 23 | ci-9d2d9680733c | agenda:pass-to-scrutinise | not-examined | false | 0 | 0 | 0 | Default change of secretFieldSelector is visible as the operator arg change (see ci-aaeb79bed47e); not separately examined. |
| 24 | ci-b798ed2dd1af | agenda:pass-to-scrutinise | not-examined | false | 0 | 0 | 0 | Join: value unset; not examined further. |
| 25 | ci-f08ea0fa28f6 | agenda:pass-to-scrutinise | not-examined | false | 0 | 0 | 0 | Join: value unset; not examined further. |
| 26 | ci-f8350fd0b224 | agenda:pass-to-scrutinise | not-examined | false | 0 | 0 | 0 | Join: value unset; not examined further. |

Totals: raises-concern 10; claims-mitigation 0; explains 0; examined-no-concern 0; not-examined 16; budget rule: agenda@v1 n=26: max(configured, 12+6n calls, 10+3n turns, 150000+30000n tokens, 1MiB+192KiB·n bytes, 300+60n s), capped at 400 calls/200 turns/3000000 tokens/16MiB/3600 s → 168 calls, 88 turns, 930000 tokens, 6160384 bytes, 1860 s

Tool calls: 12 (map[get_change_item:2 get_rendered_object:1 query_diff:1 read_artifact:1 record_assertion:1 record_assertion (rejected):1 request_validation:1 search_manifest:2 submit_findings:1 submit_findings (rejected):1])

## Validation requests (1)

- vrq-4b771f05c63f ci-194d5b487655 override-inspection subjects=[gn-004906487a76 gn-25954107a7c0] answers=[] → fulfilled vr-99475deee0c2 
  - record vr-99475deee0c2 validate.override-inspection@v1 not-run answers=[]: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item

## Answer links (0)


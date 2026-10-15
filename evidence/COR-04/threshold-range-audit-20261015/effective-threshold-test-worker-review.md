# Effective-threshold overflow regression review

## Scope

Added `dynamics/lif_effective_threshold_range_test.go` as a red-stage public
API regression only. The test does not change the LIF or Mixed implementation.
The fixture uses one zero-edge node, so the expected values do not depend on
an internal edge accumulator.

Let `A = 2^1023`, `ThetaMin = -A`, `ThetaMax = A`, `VReset = -MaxFloat64`,
and `ThetaRaw = 1000`. The finite endpoint interval is legal, while
`A + MaxFloat64` and `A + 1.5A` are both `+Inf` in binary64. The bounded
threshold base is the finite upper endpoint `A` at this raw value.

For adaptation, `DT = Ln2`, `TauSyn = TauAdapt = 1`, `Bias = initial =
MaxFloat64`, and `Beta = MaxFloat64`. Therefore `lambda = alpha = rho = 1/2`:
the first candidate is `MaxFloat64`, it fires, and the next adaptation is
`MaxFloat64`; the second step has a finite candidate but
`theta_effective = A + MaxFloat64 = +Inf`.

For homeostasis, `DT = TauSyn = TauRate = 1`, `Eta = MaxFloat64`,
`TargetRate = 0`, `HMax = 1.5A`, and `Bias = initial = HMax`. The first
candidate is finite and above `A`, so it fires. The rate update is `1`, the
offset update clamps to `HMax`, and the second step has
`theta_effective = A + 1.5A = +Inf`. Both fixtures are checked by the test
before the two-step rejection assertions.

## Public paths covered

Each mechanism has independent LIF and all-LIF Mixed tests. Every rejected
call checks an error, nil/zero result, unchanged parameter and input buffers,
and for continuation calls a deep copy of the supplied state. Each path first
executes a one-step success control, including state validation and one-step
backward where that API exists.

| Path | Adaptation | Homeostasis | Old-source result |
| --- | --- | --- | --- |
| LIF `Forward` with two rows | red failure | red failure | accepted the second row instead of rejecting `+Inf` |
| LIF `ForwardRecompute` with two rows | red failure | red failure | returned partial two-row outputs |
| LIF `BackwardRecompute` with two rows | passed rejection control | passed rejection control | already returned an error and zero gradient |
| LIF `NewState`/`Advance`, first row then continuation | red failure | red failure | accepted the continuation and returned a second state |
| all-LIF Mixed `Forward` with two rows | red failure | red failure | returned a partial two-row trace |
| all-LIF Mixed `NewState`/`Advance`, first row then continuation | red failure | red failure | accepted the continuation and returned a second state |

The single-step LIF and Mixed `Forward`, `Backward`, `ForwardRecompute`,
`BackwardRecompute`, and `Advance` controls passed for both mechanisms. The
non-nil coupled path was not duplicated here because the existing public
coupled implementation already rejects non-finite effective thresholds; this
task only adds the ordinary and persistent paths that reproduced the defect.

## Validation command and result

Ran exactly:

```sh
GOCACHE=/Users/timlai/Developer/coimnet/runs/.tmp/lif-threshold-range-20261015/go-cache GOMODCACHE=/Users/timlai/Developer/coimnet/runs/.tmp/lif-threshold-range-20261015/go-mod-cache GOTMPDIR=/Users/timlai/Developer/coimnet/runs/.tmp/lif-threshold-range-20261015/go-tmp TMPDIR=/Users/timlai/Developer/coimnet/runs/.tmp/lif-threshold-range-20261015/go-tmp GOPROXY=file:///Users/timlai/go/pkg/mod/cache/download GOSUMDB=off GOTOOLCHAIN=local GOMAXPROCS=2 go test -p=2 -count=1 ./dynamics -run '^TestEffectiveThresholdRange'
```

The command exited with status 1 as required for the red stage. The concrete
failures were:

- `TestEffectiveThresholdRangeLIFAdaptationRejectsOverflow/Forward`
- `.../Recompute/ForwardRecompute`
- `.../Advance`
- `TestEffectiveThresholdRangeLIFHomeostasisRejectsOverflow/Forward`
- `.../Recompute/ForwardRecompute`
- `.../Advance`
- `TestEffectiveThresholdRangeMixedAdaptationRejectsOverflow/Forward`
- `.../Advance`
- `TestEffectiveThresholdRangeMixedHomeostasisRejectsOverflow/Forward`
- `.../Advance`

The failure messages identify the old behavior as unexpectedly accepting an
infinite effective threshold. The BackwardRecompute rejection subtests and all
single-step controls produced no failure output.

## Limits

This validation intentionally did not run the full suite, race tests, build,
vet, training, downloads, or any external command. It does not claim anything
about existing OCR models or task quality.

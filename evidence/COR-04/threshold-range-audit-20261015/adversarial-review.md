## Diff Inspector

Scope: CLEAN

Reviewed: `dynamics/lif.go`, `dynamics/lif_coupled.go`, `dynamics/lif_segment.go`, `dynamics/lif_state.go`, and `dynamics/mixed.go` against `evidence/COR-04/threshold-range-audit-20261015/before/`; new `dynamics/lif_threshold_range_test.go`; `docs/tickets/103-lif-threshold-range.md`; and `ENG.md`.

Adversarial review: RUN — the change expands the accepted numeric range of a stateful spiking core and its gradient/recompute paths.

### Findings

- [P1] [CONFIRMED] `dynamics/lif.go:390-418`, `dynamics/lif_segment.go:197-226`, `dynamics/lif_state.go:301-336`, `dynamics/mixed.go:471-500`, and `dynamics/mixed_state.go:386-421` — after the new overflow-only transform admits a finite base threshold, adding a finite adaptation or homeostasis value can produce an unrepresentable effective threshold without an error. A concrete legal binary64 case is `A=2^1023`, `ThetaMin=-A`, `ThetaMax=A`, `VReset=-MaxFloat64`, `ThetaRaw=1000` (so `base=A`), `DT=ln(2)`, `TauSyn=1`, `TauAdapt=1`, adaptation `Beta=MaxFloat64`, `initial=MaxFloat64`, `Bias=MaxFloat64`, and two zero inputs. The independent Python calculation gives `lambda=alpha=rho=0.5`, step 0 `cand=MaxFloat64`, spike `1`, and adaptation `MaxFloat64`; step 1 `cand=0` but `theta=A+MaxFloat64=+Inf`, while voltage, synaptic output, and next adaptation remain finite. The listed ordinary paths only check drive/candidate/output/state finiteness, so they can return or commit a trajectory with an infinite effective threshold. `lif_coupled.go:239-281` and `mixed_coupled.go:186-227` already check `finite(theta)`, showing the intended rejection contract. Add the same effective-threshold finiteness check to every ordinary/recompute/persistent/mixed path before event evaluation and preserve failure atomicity.

### Needs investigation

- [NEEDS INVESTIGATION] Repository-wide paths outside the authorized `dynamics/` review scope were not assessed. Before declaring every CoImNet threshold transform covered, inspect any separate non-`dynamics` helper that computes `ThetaMax-ThetaMin` directly and decide whether it is a public corresponding path.

### Validation limitations

- The diff shows the original finite-span expressions and validation guard unchanged. `dynamics/lif.go:152-153` still requires finite values and `v_reset < theta_min < theta_max`, so the extreme finite range remains legal and equal/reversed/non-finite bounds remain rejected.
- Python standard-library `math`/`fractions` reference arithmetic produced `span=+Inf`, midpoint `0x0.0p+0`, slope `0x1.0000000000000p+1022`, and finite in-range values at raw `-1000`, `0`, and `1000`. The same reference arithmetic produced the confirmed downstream `theta=+Inf` case above.
- Go compilation, unit tests, race tests, vet, training, downloads, and runtime execution were intentionally not performed under the assigned validation restriction. Normal-range bit identity is established from the unchanged finite-span branches only; runtime compatibility evidence remains for Root's full-package run.

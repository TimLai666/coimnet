# Gradient audit reproduction

This audit is an independent finite-difference probe for the mixed-precision
Insyra bridge and the pure `float64` continuous core. It is evidence only and
is not part of the production build.

Run it from the repository root:

```sh
audit_dir=$(mktemp -d /private/tmp/coimnet-gradient-audit.XXXXXX)
cp evidence/gradient-audit-20260913/audit.go.txt "$audit_dir/audit.go"
go run "$audit_dir/audit.go" > "$audit_dir/report.json" 2> "$audit_dir/run.log"
```

The program writes its JSON report to stdout and writes
`gradient audit complete` to stderr. The saved report
`evidence/gradient-audit-20260913.json` records the exact inputs, tolerances,
observations, command, run log, and source fingerprints used for the checked
run. Each run gets a new temporary directory so a prior report is not
overwritten. The temporary copy is intentional because Go only treats `.go`
files as build inputs; the preserved `.txt` source remains outside the
production packages and `go test ./...` package discovery.

The probe uses central differences at epsilon
`1e-2, 2e-3, 1e-3, 1e-4`. Chapter 9.4 states a relative starting threshold of
`1e-4` for smooth float64 finite differences and a separate absolute check for
near-zero gradients. This audit operationalizes those two checks as
`absolute_error <= 1e-5 + 1e-4*abs(fd)`. The bridge test uses
`abs(fd-analytic) <= 2e-5 + 1e-3*abs(fd)` because encoder, readout, input, and
MSE operations cross Insyra float32 boundaries. A relative error above `1e-4`
can still satisfy the combined audit rule when the absolute error is small
enough for the additive `1e-5` term; both terms must be evaluated, rather than
rejecting on relative error alone.

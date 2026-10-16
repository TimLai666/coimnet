# Root implementation slot

Read plan.md. Root reviewed the complete public regression file and ran RED against unchanged production sources. All 162 leaves compiled and ran: 112 passing and 56 failing terminal events, 168 terminal events including parents. Failures reproduce receptor writes during Act and explicit gate conflicts during Update. Tests are frozen by frozen-checks.json.

Implement only learning/individual.go, learning/individual_online.go, learning/online.go. Preserve the original check and commit order and lock ownership. advanceVersioned already owns Individual.mu and must call the Locked helper; OnlineLearner.apply must call the helper that acquires that lock. Modify only the GateReceptor branch in advanceStepwise. The receptor-controlled eligibility window must remain active. Never mutate rule pointers or freeze flags.

Update the Act and advanceVersioned comments to describe the closed online gate. Also correct the directly related Advance comment: an enabled receptor declaration supplies its native gate, otherwise the implicit gate is zero. State the existing native behavior without changing it. No unrelated comment cleanup.

Do not edit the regression or any documentation. Do not run Go tests, build, vet, or module commands; Root owns execution. Run gofmt on only your three allowed files. If a test expectation seems wrong, stop and report it. Root will run green-online-ordinary/race, public reproductions, existing regressions and the full project gate.

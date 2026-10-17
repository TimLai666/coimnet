# Root review — Resample output byte capacity

The public one-sample request at rates 1→MaxInt reproduces a makeslice panic before any large backing allocation. The count-only helper correctly returns MaxInt. Current runtime multiplication checks and an independent big.Int byte product establish the safety of the probe.

The regression was frozen before the candidate. Root removed the worker's duplicate panic helper and reused the existing audio_test.go helper; assertions stayed unchanged. Original ordinary/race each have one failing capacity test and three passing normal/count-only events. Candidate ordinary/race each pass all four events. Entire audio package ordinary/race each pass 81 events, retaining the 77 current-source baseline events and adding only the four frozen events. Root independently parsed the receipt logs and verified SHA-256 values (dc2047).

The candidate adds one division-bound check after existing finite-input validation and before make. Float64 is eight bytes. No count formula, interpolation, report, normal sample, error precedence among earlier checks, model field, public signature or dependency changes. The dataset consumer already checks byte products and per-file/total limits; it needs no duplicated fix. Public ResampleOutputLength must retain count-only large-int queries.

The only source change is the candidate signal.go.txt, SHA 015579e25cb72b69e4d3f35691c5481c252865bee86bf4367034af7619464fb5. Formal signal.go remains d6a35017afefdb591c76cdbe8240ceb6f70b6a7f9fa5b04eeed8c6595c7d0bfb. The final frozen test is 0607b099a1a37704a6d3a4bbb3a2700e2dd83201851cd480116fe790a134d02f. Exact original/candidate diff was read by Root (0bbd1f). No further confirmed finding in that small diff.

The 10,459 bytes of owned probe/regression sources have been removed after actual runner termination, exact preserved-source comparison, open-handle checks and protected-input verification. Parent full-engineering cache stays in use. Original models, data, modules and unrelated work are preserved.

This phase is isolated validation, not formal adoption or complete engineering. It does not prove 32-bit runtime behavior, physical memory availability, model quality or absence of all bugs. Candidate adoption follows tickets142/143's frozen-source acceptance and verified delivery.

Subtraction: keep the check at the existing allocator, reuse the existing test helper, and preserve the independent count-only API. No new layer or arbitrary memory limit.

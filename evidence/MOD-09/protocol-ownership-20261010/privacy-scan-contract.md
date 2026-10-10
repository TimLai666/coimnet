# Root decision: project temporary source scanning

The full unit log records TestNoTelemetry failing while WalkDir traverses a Go temporary directory that another package removes. AGENTS.md requires temporary files under runs/.tmp, which is ignored. Scan only skips the exact module-root runs/.tmp subtree; source errors elsewhere must remain errors. Other runs directories, nested lookalike names and non-test production source remain scanned. No telemetry patterns or production exclusions are relaxed.

The existing scanner is extracted unchanged into a test-only helper first, then a six-case boundary regression is run RED. Only the exact temporary-root guard is added after RED. All BioInspired ownership regression files remain frozen. Full verification remains required on native mac-1 storage after SSH host trust and authentication succeed.

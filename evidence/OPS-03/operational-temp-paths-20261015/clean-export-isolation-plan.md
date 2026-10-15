# Clean export build metadata isolation

The new project-local workspace places the committed archive below a parent Git checkout. A minimal native Go build embeds the parent revision and dirty status. A process-local Git discovery ceiling did not prevent those stamps: Go 1.26.5 load/pkg.go calls vcs.FromDir, whose own filesystem walk continues into parent directories. The plain and ceiling probe metadata are both preserved; the ceiling-only repair is rejected.

Root contract: commands that build the committed export must not embed the parent working checkout's VCS status. Parent repository metadata/files, explicit workdir retention, source commit selection, other Go flags and output schema stay unchanged. The existing frozen path regression is not editable.

A separate additive regression runs the actual shell script in a tiny committed fixture. It stubs heavy Go operations, but its first child invokes the real installed Go tool once to build only a dependency-free fixture main package. The binary is retained outside the removed workspace, within the test's owned directory. Standard debug/buildinfo reads its actual settings: no vcs/vcs.* settings are allowed, and the pre-existing linker -s flag must remain enabled. The fixture deliberately sets buildvcs=true so the script must enforce clean-export semantics. Check successful default workspace cleanup, output fields, original HEAD/dirty files, siblings and existing Git ceiling entries. Root executes red before product repair.

The candidate repair appends -buildvcs=false to the shell process's GOFLAGS while preserving other options. This restores no-VCS build metadata from exports outside a checkout, does not change Git configuration, and works independently of path separators or Git's discovery rules. Native minimal comparison and the unchanged new regression must prove it before acceptance. Re-run the original path regression plus this new regression, normal and race, then all required engineering checks on the permanent final source set.

Keep the long LIF study and numerical sources unchanged. Do not run real-data OPS-10, download datasets or import the whole graph for this check.

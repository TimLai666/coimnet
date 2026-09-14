# Signal JSON review

Reviewed the five implementation files changed from `ed916bb`, the final `json_numbers_test.go`, and the four-source public example. Root review plus a Luna `max` adversarial review found no remaining confirmed failure in the strict runtime decoding paths.

The red control uses the final test file with the five original implementations restored. Its command, base commit and test hash are in `red-reproduction.json`. It reproduced null-to-zero conversion in ten numeric fields, including the first mapping index. The v20 signal logs exercise the same final test source after the fix.

Scalar wire fields reject explicit null before numeric decoding. Numeric arrays perform one null-literal scan and one standard float64-slice decode. Valid JSON numbers cannot contain that literal, and all other invalid element types remain rejected by the standard decoder. This avoids a decoder call and temporary scalar storage per element.

Tests verify optional null quality score/range, explicit zeros, omitted scalar defaults, numerical extremes, nested Observation/Target rejection and unchanged receivers on errors. The metadata-mutation assertion now proves the null fixture was actually injected. Existing duplicate-key, depth, unknown-field and size checks remain covered by the full signal suite.

Plain builder specs and individual MappingEntry values retain ordinary Go JSON behavior; strict persistence uses the documented Decode entry points. This patch does not extend their public construction contracts. The missing full SIG-04 mapping workflow is recorded in ticket 03 and remains unaccepted.

The evidence is software fixture validation. No biological task result, full-brain training, Windows runtime or GPU execution is claimed.

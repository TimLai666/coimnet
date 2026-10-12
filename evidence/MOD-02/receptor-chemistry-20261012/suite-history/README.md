# Earlier full-suite attempts

These failed attempts remain historical evidence. V1 was blocked by the sandbox process-substitution descriptor. V2 encountered module-cache symlinks rejected by module verification. V3 passed module verification, build and ordinary tests, then exposed the Mixed allocation assertion under race; Root stopped the remaining race processes to diagnose that failure. The abort receipt records that intentional interruption. None is a final acceptance run. See the parent verification.json for the final command and current source manifest.

V4 passed all 58 ordinary-test packages and the chemical/checkpoint race packages, then the experiment package hit the default ten-minute cumulative test timeout. The active test had been running for only three seconds. Remaining packages were interrupted. The final run keeps the same code and tests and sets `GOFLAGS="-p=2 -timeout=60m"`; no assertion or workload was reduced.

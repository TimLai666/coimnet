# Root review — audio field and allocation limits

Scope is clean: two production files and three independent frozen regressions. The formal diff was read in full against b315475977b46fc6014aace16609ec36e5ab3dc6. Exact verified candidate bytes were adopted; tests were not changed to fit the implementation.

WriteWAV validates positive channel count/rate and nonnegative frame count before its sole production wavOutputBytes call. The helper checks channel stride first, uses uint64 after that bound, divides before multiplying the payload, and bounds both RIFF data+36 and native file data+44. Buffer allocation and exclusive file opening follow these checks. Reader, pcm16, channel-count authority and NaN-to-silence behavior retain their contracts.

Generate checks the container element bytes after existing scalar validation. The rune guard checks the chosen length after the same PCG draw, so a merely wide declared range does not introduce an earlier rejection when a valid length is actually drawn. The validated min/max range keeps that integer expression representable. Zero-count output remains nonnil and bypasses the unused rune allocation.

Root independently parsed formal ordinary/race focused logs:41 pass events each,28 new regression events plus13 existing synthesis controls. The29121-byte nontrivial normal Generate/Synthesize payload has the same SHA-256 as the prior formal source within each build. WAV mono/stereo full bytes, empty output and existing-path controls passed.

The first formal focused attempt stopped before tests because Root omitted the declared GOTMPDIR. Its log/receipt are preserved; environment-setup.json records the project-local directory creation. Accepted retry81666 is terminal0,93fca9. No formal source was changed for that repair.

The independent readonly Luna/max review found no confirmed defect; Root checked its conclusions against the current diff and focused evidence. No32-bit runtime, arbitrary-size allocation success, model-quality improvement or biological validity is claimed. Full engineering74589, final governance, owned cleanup and verified delivery remain required.

Subtraction: one shared863-source gate covers both fixes; redundant helper checks implied by the validated caller/stride/rate bounds were removed before frozen candidate acceptance. No new format, public API, dependency or arbitrary capacity cap was added.

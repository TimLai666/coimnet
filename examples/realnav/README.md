# Real licensed fruit-fly navigation trajectory example

This example is a small, auditable behaviour-prediction run over the licensed
Dryad fruit-fly trajectory file. It imports the gzip CSV through the
`tasks/nav2d/trajectory` data contract, trains a continuous sparse CoImNet
core to predict the next `(dx, dy)` displacement, saves a training snapshot
outside Git, and loads that snapshot in a separate `infer` process for a
trial-held-out evaluation.

The model is an observer-position proxy. It is not a fruit-fly connectome or
a neural mechanism claim. The `infer` command is teacher-forced on recorded
observations: each row's current pose and already observed displacement history
predict only the next recorded displacement. The separate `rollout` command
feeds generated positions back into a frozen model in an engineering arena;
it does not establish animal control, learned homing, or path integration.

## Data and causal contract

The default source is kept outside this repository:

`/Users/timlai/Developer/coimnet-data/TSK-11/dryad-path-integration/all_ds_t01_d2_cm_no2.csv.gz`

The expected source SHA-256 is
`83e13b7057957cf41a45dc91996a4519be7066519242b6912c65b2418cb91670`.
It contains 39 `(fname, fly)` trials and 231,130 rows. The example accepts
only rows in `segment=after_relocation`. Samples are made from source-ordered
adjacent rows within one trial when the rows remain in that segment and their
timestamp gap is positive and no greater than the configured bound. The input
contains the current `(x_cm, y_cm)`, the previously observed displacement and
the observed time gap. The default sample contract uses a 0.5 second maximum
gap and a 5 cm maximum adjacent step. The target is the next row's
displacement.

`reward`, `fictive reward`, `condition`, `segment`, and future pose/target
columns never enter the model input. Trial splitting happens before sample
construction, so one trial cannot contribute rows to both training and held-out
sets. The network output is a learned residual added to the previous observed
direction at the training-set mean displacement, so the report can compare it
with the causal persistence predictor and the raw previous-displacement
predictor.

The source is Dryad DOI [10.5061/dryad.vdncjsz0b](https://doi.org/10.5061/dryad.vdncjsz0b),
with the author repository pinned to commit
`0eb07940a9ffdedd97ada81f75e9d5f7bface579`. Dryad publishes its datasets under
CC0; the report links the [official reuse guide](https://datadryad.org/help/guides/reuse).
The local author-repository copy matches Git blob
`b87c2ca52eb3030a02b8e14b6510b358fba92c69`, and its source README matches blob
`9dabe23037de5279a21ebed66464275d173966c4`. Byte-equivalence to the Dryad ZIP
was not verified because direct Dryad retrieval returned 403/401. The exact
source and license metadata, split trial IDs/counts, filtering rule version,
model fingerprints, metrics, update count, timing and RSS are emitted in the
run report. On Windows, the standard library has no `getrusage` equivalent, so
`runtime.max_rss_bytes` is `0` to mark RSS as unavailable.

The saved model includes the complete preprocessing contract: version, feature
rule, segment, sample limits, numeric scales, and feature lists. Inference
rejects a snapshot when any of these values differs from the current code. The
continuous core resets its recurrent state at the same 256-sample chunk
boundary during training, evaluation, and inference; the report records this
chunk size.

## Run

Keep data, snapshots, and reports outside the repository. Training creates a
new output directory and writes the exact snapshot there:

```sh
go run ./examples/realnav train \
  --data /Users/timlai/Developer/coimnet-data/TSK-11/dryad-path-integration/all_ds_t01_d2_cm_no2.csv.gz \
  --out /tmp/coimnet-realnav-run-01
```

Inference is a separate invocation. It loads the saved snapshot and refuses
to train:

```sh
go run ./examples/realnav infer \
  --data /Users/timlai/Developer/coimnet-data/TSK-11/dryad-path-integration/all_ds_t01_d2_cm_no2.csv.gz \
  --snapshot /tmp/coimnet-realnav-run-01/model.json \
  --out /tmp/coimnet-realnav-infer-01
```

The report includes held-out one-step displacement MSE and angular error,
continuous-direction, zero-output, and previous-displacement baselines. The
`train` and `infer` commands use the original `trajectory.Read` contract, which
keeps fictive-zone coordinates out of the imported rows and model input. Their
one-step reports include no predicted zone-distance change or return-success rate.
The infer report leaves pre-training metrics and the initial snapshot
fingerprint null because that process only receives the saved final snapshot.
The one-step metrics are teacher-forced and are not closed-loop success.

## Autonomous engineering rollout

A separate process can load the same saved model and run its original held-out
trials without using future recorded positions for subsequent actions:

```sh
go run ./examples/realnav rollout \
  --data /Users/timlai/Developer/coimnet-data/TSK-11/dryad-path-integration/all_ds_t01_d2_cm_no2.csv.gz \
  --snapshot /tmp/coimnet-realnav-run-01/model.json \
  --out /tmp/coimnet-realnav-rollout-01 \
  --steps 200
```

`--steps` defaults to 200 and accepts 1–4096 model decisions. These are not
animal seconds. The source was downsampled by both time and distance, and the
model predicts displacement rather than speed or the next time interval. The
initial previous-time feature comes from the earliest valid adjacent observed
pair; subsequent decisions use a fixed 0.1 second feature. The model receives
absolute observer coordinates and realised displacement, not animal sensory
observations or the fictive-zone center.

The circular engineering arena has a 30 cm radius. Actions longer than 5 cm
are clipped and counted. An endpoint outside the arena is rejected, counts as
a collision, and produces zero realised displacement for the next decision.
The neural state persists within each trial and resets every 256 decisions,
matching the existing training chunk rule. Parameters and optimizer state are
frozen. Direction persistence, zero displacement, and a seeded random-direction
control use the same initial observations, arena and decision limit, with
independent trial state and random streams.

`trajectory.ReadWithReturnTargets` extracts only the fixed
`estimated_food_x_cm` and `estimated_food_y_cm` center from `after_relocation`
rows, using the same verified source bytes as the position import. This metadata
is returned separately and is used only for scoring. The predeclared engineering
criterion is a 2 cm fictive-zone radius; initial inclusion, tangency and action
segments crossing the closed disk count as hits, and evaluation stops at the
first hit. The original experiment uses a different criterion and a 100 second
observation window; this rollout does not reproduce the experiment.

The independent `coimnet-realnav-rollout/v1` report records the effective
protocol and its hash, source and training-snapshot hashes, saved trial split, each
trial's target, strategy results and exclusions. It reports fictive-zone hit
rates separately for rewarded and non-rewarded trials, with initial hits counted
separately. `snapshot_sha256` identifies the complete neural training snapshot,
including optimizer state; the verification evidence separately fingerprints
the full saved model file, including preprocessing and split metadata. Source
rows are still scanned to verify that saved split and its sample counts; these
future observations never supply subsequent actions. A non-rewarded trial's
zone hit is hypothetical, not a return to a
previous reward. Poor model results are retained without changing the criterion.
Raw trajectories, saved models and complete reports remain outside Git; compact
verification evidence is linked from [ticket 32](../../docs/tickets/32-real-trajectory-autonomous-rollout.md).

The 2026-10-02 fixed-data run evaluated all 13 held-out trials (8 rewarded,
5 non-rewarded), with no exclusions or initial hits. The model and all three
controls hit zero fictive zones within 200 decisions. Two training processes
produced byte-identical saved models, and two rollout processes produced
identical results after removing runtime measurements. This establishes a
reproducible engineering evaluation, with no learned navigation result. See
[verification](../../evidence/TSK-11/autonomous-rollout-20261002/verification.json).

The test suite uses an in-memory synthetic trajectory and checks true gradient
updates, trial and segment isolation, causal feature construction, and
fresh-process snapshot inference. It does not download the Dryad source.

```sh
go test -count=1 ./examples/realnav
go test -count=1 -race ./examples/realnav
go vet ./examples/realnav
GOOS=windows GOARCH=amd64 go test -c -o /tmp/realnav.test.exe ./examples/realnav
```

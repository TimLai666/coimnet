# Real trajectory stimulus memory example

This example trains an eight-node recurrent engineering model on the complete trajectory history of each recorded fly trial. It predicts the next displacement from observed position, previous displacement, elapsed time and the delivered LED stimulus. It compares three separately trained models with identical capacity, initial parameters and update budgets: delivered stimulus, no stimulus, and delayed shuffled stimulus.

The network uses an artificial graph with 64 directed edges, including self-loops. It does not use MaleCNS wiring. Position is an explicit observation proxy, displacement is an imitation label, and recorded LED delivery is an instrument-level event. The dataset does not supply sensory observations, motor commands or neural reward arrival times. Results from this example cannot establish a fly neural mechanism.

## Source and split

The source is the authors' pinned mirror of Titova et al.'s displacement trajectories, associated with Dryad DOI `10.5061/dryad.vdncjsz0b` and CC0 1.0. The importer requires compressed-file SHA-256 `83e13b7057957cf41a45dc91996a4519be7066519242b6912c65b2418cb91670`; see the [existing source contract](../realnav/README.md#data-and-causal-contract). The mirror is not represented as a byte-identical copy of the Dryad archive.

`prepare` reads the existing v1 realnav model's original 26 training and 13 test trial IDs. It keeps those 13 test trials, then selects six validation trials from the original training group. Selection is stratified by condition using largest remainders, followed by the SHA-256 order of `20261003`, ASCII unit separator, and trial ID. The resulting 20 training trials and six validation trials are sorted by ID. All three groups must be disjoint and cover every imported trial.

The original test trials have been evaluated before. They are not a new untouched test set. This slice fixes all settings before training and reports validation scores without using them to select parameters. The independent [preregistration](../../evidence/TSK-11/stimulus-memory-20261003/preregistration.json) records the actual split, source fingerprints and settings.

## Run

Keep the source, original model and new trained models outside Git. Every output directory must be new. From the repository root:

```sh
go run ./examples/realnavmemory prepare \
  --input /path/to/all_ds_t01_d2_cm_no2.csv.gz \
  --base-model /path/to/realnav-v1/model.json \
  --out-dir /path/to/memory-plan

go run ./examples/realnavmemory train \
  --input /path/to/all_ds_t01_d2_cm_no2.csv.gz \
  --plan /path/to/memory-plan/plan.json \
  --out-dir /path/to/memory-training

go run ./examples/realnavmemory infer \
  --input /path/to/all_ds_t01_d2_cm_no2.csv.gz \
  --model /path/to/memory-training/bundle.json \
  --out-dir /path/to/memory-inference

go run ./examples/realnavmemory rollout \
  --input /path/to/all_ds_t01_d2_cm_no2.csv.gz \
  --model /path/to/memory-training/bundle.json \
  --out-dir /path/to/memory-rollout

go run ./examples/realnavmemory counterfactual \
  --input /path/to/all_ds_t01_d2_cm_no2.csv.gz \
  --model /path/to/memory-training/bundle.json \
  --out-dir /path/to/memory-counterfactual
```

The plan and model bundle have separate versioned contracts. Loading validates the source, feature rules, split, architecture, training options, seeds, controls and update budget, including every trainable parameter’s optimizer step count. Invalid inputs fail before an output directory is created. Existing output directories are never overwritten.

## History and learning

Each trial starts from zero state and retains every source row in order across all five phases. One row advances one `model_step`, regardless of the variable recorded time interval. There is no 256-row state reset or gradient truncation. Trials longer than 16,384 rows are rejected.

The six inputs are current x/30 and y/30, previous dx/2 and dy/2, previous elapsed seconds, and delivered stimulus encoded as 0 or 1. At the first row the previous displacement and elapsed time are zero. Previous displacement is also zero at a phase change, a gap longer than 0.5 seconds, or a displacement longer than 5 cm. The current observed position and elapsed time remain available, and the neural state continues through relocation. These are engineering assumptions.

Only `rewarded` rows with positive `led_1` provide delivered stimulus. Positive values in `non-rewarded` trials describe scheduled events with the LED unplugged; those events remain audit metadata and never become model inputs or reward. Trial IDs, condition names, phase names, relocation offsets and target coordinates are excluded from the input vector.

Loss is calculated only for the middle row of a valid three-row `after_relocation` window. Both adjacent time intervals must be at most 0.5 seconds and both adjacent displacements at most 5 cm. The target is the following observed displacement in cm. Every other row still participates in the forward and backward history, with zero direct loss upstream. `PredictAll` and `StepFrom` use the existing Insyra-backed encoder and readout.

All three controls use seeds 20261003, 20261004 and 20261005, ten epochs, one update per complete training trial per epoch, learning rate 0.001 and gradient clipping norm 1. All parameter groups are trainable. There is no weight decay, recomputation or truncation. Initial time constants are 2, 4, 8, 16, 64, 256, 1024 and 4096 model steps; the readout starts at zero.

The shuffled control delays stimulus by a 16-row block. Each new block receives a seeded Fisher–Yates permutation of the previous completed block; the first block is zero. It never reads future stimulus, including during autonomous continuation. This control changes both stimulus timing and its association with position, so a difference cannot be attributed solely to permutation.

## Frozen evaluation

Inference reports masked MSE before and after training for training, validation and test trials. Autonomous evaluation first replays the complete recorded prefix before the earliest valid relocation observation, then advances only with generated positions and displacements. The first decision consumes the recorded starting observation, including its actual elapsed-time feature. Subsequent generated observations use elapsed-time feature 0.1 seconds and zero delivered stimulus. Model steps are not interpreted as elapsed animal time.

The evaluator alone holds the return-region center. The engineering arena is a circle of radius 30 cm, the hit radius is 2 cm and the budget is 200 decisions. Out-of-arena moves leave position unchanged and count as collisions. An initial position within the hit region is recorded separately. Hits use the swept movement segment; `nearest_distance_cm` is the nearest recorded endpoint distance. Persistence and random controls use the mean displacement length of legal training labels.

Reports retain every valid test trial, control and seed, including unsuccessful walks. Invalid source or snapshot contracts, non-finite states and cancellation stop the command without publishing a partial report. Parameters and optimizer state stay frozen. An improved imitation loss does not by itself show autonomous navigation improvement. These results also do not complete the broader TSK-11 real-task acceptance criteria.

The existing [realnav v1 example](../realnav/README.md) keeps its own features, snapshots and behavior. The memory example uses a separate CLI and file format.

## Recorded evaluation (2026-10-03)

All nine models completed 200 updates on 20 training trials. The 6 validation and original 13 test trials were reported without retuning. Test MSE decreased from 0.02002106 cm² to 0.01689731–0.01750598 cm². Delivered-stimulus and no-stimulus differences were small and changed direction across seeds; this evaluation does not establish a stimulus-memory benefit.

All 18 model groups (three controls, three seeds, before and after training) hit 0 of 13 return regions. Persistence hit 0 of 13 for every seed. Random movement hit 0, 0 and 1 of 13 for seeds 20261003, 20261004 and 20261005. No trial was excluded, and every group retained 200 decisions per trial. Better imitation loss did not produce successful autonomous navigation.

Two independent inference processes and two independent rollout processes produced byte-identical reports. The source, original v1 model and trained bundle remained unchanged. See the [inference results](../../evidence/TSK-11/stimulus-memory-20261003/inference-report.json), [complete trial summaries](../../evidence/TSK-11/stimulus-memory-20261003/rollout-summary.json) and [training summary](../../evidence/TSK-11/stimulus-memory-20261003/training-summary.json). Full traces and model bundles remain in the external data directory recorded by the evidence.

## Frozen stimulus-history intervention (2026-10-03)

`counterfactual` evaluates the existing nine models before and after training on all 13 test trials, without training or retuning. Each pair starts from zero state. It first applies the original delivered, no-stimulus or delayed-shuffled input transform, then clears only the model-visible stimulus before the first source `relocation` row. All other recorded inputs, labels and later stimulus remain identical. Autonomous continuation keeps the original raw stimulus blocks, so later shuffled stimulus also remains identical when paths diverge.

The report uses schema `coimnet-realnav-memory-counterfactual/v1`. It retains every pair's masked MSE, output and action differences, position separation, hit/distance/collision summaries and complete trace fingerprints. Traces are omitted from the compact paired report. The fixed `1e-6 cm` scale is for engineering reporting; exact nonzero values below it are retained. It is not a significance or navigation-success threshold.

Both trained stimulus controls changed later outputs in the eight rewarded trials for every seed. The five non-rewarded trials, every no-stimulus model and every untrained zero-readout snapshot had exactly zero difference. Clearing history changed trajectories, but all 18 original and all 18 erased groups hit 0/13. The masked MSE effect changed direction across seeds, so this check establishes sensitivity to past stimulus without establishing a useful-memory or navigation benefit.

Two independent processes produced byte-identical reports for all 234 pairs. Every original rollout summary and complete trace fingerprint matches the prior evaluation, and weighted original MSE matches the prior inference results within 1e-15 cm². Source, parameters and saved optimizer remained unchanged. See the [paired report](../../evidence/TSK-11/stimulus-counterfactual-20261003/report.json), [summary](../../evidence/TSK-11/stimulus-counterfactual-20261003/summary.json) and [fixed protocol](../../evidence/TSK-11/stimulus-counterfactual-20261003/preregistration.json).

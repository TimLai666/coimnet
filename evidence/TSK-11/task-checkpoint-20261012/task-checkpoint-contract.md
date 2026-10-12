# Task checkpoint unit, root contract

Root scope: complete task model save/restore for OCR, whole-file ASR and byte text generation, then use these paths from each real-data example. Existing Snapshot APIs and CPU semantics stay unchanged. No task-quality threshold or requirement status is altered.

## Slot A: task APIs
- tasks/ocr/persistence.go: (*Recognizer).Save(ctx context.Context, path string) error; LoadRecognizer(ctx context.Context,path string)(*Recognizer,error).
- tasks/asr/persistence.go: same signatures.
- tasks/textgen/persistence.go: (*Model).Save(ctx,path) error; LoadModel(ctx,path)(*Model,error).
- Separate envelope schemas coimnet-ocr-checkpoint/v1, coimnet-asr-checkpoint/v1, coimnet-textgen-checkpoint/v1. Use directory bundles through checkpoint.SaveTaskTrainingBundle/LoadTaskTrainingBundle (new shared slot), so the task models retain network-disk support. No single-file hardlink publication. Each task config is strict JSON. Path denotes a NEW directory, with manifest written last. These task APIs have no existing published behavior. Bind task config, full TrainingSnapshot and, for text, actual ByteVocab config/hash. Custom vocabularies without reproducible config fail Save before file creation.
- Restore validates declared task config against exact network config/options expected by constructor, validates full training with learning.RestoreTrainer, owns all state, does not drop Adam moments/updates/accumulation. Avoid adding a second optimizer.
- Nil receiver/context, canceled context, empty path, existing destination, malformed/unknown/missing fields, config-training mismatch, corrupt bundle checksum/version/non-finite params all return error without publishing a file or replacing existing files.

## Slot B: real-example usage, after Slot A
- Optional --model-output and --model-input select training checkpoint or independent saved-model evaluation; no implicit persistence for old CLI invocations. Rebuild/fingerprint imported data for evaluation, never retrain in model-input mode. No raw data inside saved model.
- OCR/ASR/text report actual saved artifact digest and restored evaluation; keep old deterministic report behavior when flags omitted.
- Media reuse existing trained-model.json raw TrainingSnapshot format for compatibility, add bounded strict load + restore, use loaded model to recompute predictions and compare against memory results before artifact publication. Later add new-process evaluate path.

## Acceptance flows
1. Train -> predict -> Save -> Load -> identical predict/evaluate -> same next update and full training snapshot as uninterrupted reference. Execute a child process to Load for independently verified inference.
2. Saving twice leaves every file of the first bundle byte-for-byte unchanged. Corrupt/unknown/missing/semantically incompatible file and canceled context fail explicitly, never publish partial output.
3. Existing real-data runs retain data/split/seed/budget/negative quality evidence. New saved-model evaluation has no updates. Full package + full repo checks required before delivery.

Shared slot: checkpoint.TaskTrainingSnapshot{TaskSchema string json:task_schema, Config json.RawMessage json:config, Training learning.TrainingSnapshot json:training}; SaveTaskTrainingBundle(ctx context.Context,dir,taskSchema string,config []byte,snapshot learning.TrainingSnapshot) error; LoadTaskTrainingBundle(ctx context.Context,dir,taskSchema string)(TaskTrainingSnapshot,error). Generic writer validates schema nonempty, JSON config shape object, syntax/unicode/duplicate/null, snapshot RestoreTrainer; task caller validates config semantics. Reader validates bundle/task schema, strict fields, topology, complete training and config JSON before returning.

Files: testing agent owns tasks/{ocr,asr,textgen}/persistence_test.go and checkpoint/task_bundle_test.go only. Implementation agent later owns tasks/{ocr,asr,textgen}/persistence.go only and must not edit tests. Root owns examples and all docs/evidence.

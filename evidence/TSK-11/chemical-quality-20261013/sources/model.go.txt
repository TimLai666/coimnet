package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/TimLai666/coimnet/tasks/ocr"
)

// SHA256 hashes the canonical manifest that binds the model document and arrays.
type realOCRModelArtifact struct {
	Path    string `json:"path"`
	Mode    string `json:"mode"`
	SHA256  string `json:"sha256"`
	Updates uint64 `json:"updates"`
}

// RunWithModel trains and saves a new bundle, or evaluates an existing model
// without updates. Empty model paths retain the in-memory Run behavior.
func RunWithModel(ctx context.Context, dataset Dataset, config RunConfig, modelInput, modelOutput string) (Report, error) {
	if ctx == nil {
		return Report{}, errors.New("realocr: context is required")
	}
	if modelInput != "" && modelOutput != "" {
		return Report{}, errors.New("realocr: --model-input and --model-output are mutually exclusive")
	}
	return runWithModel(ctx, dataset, config, modelInput, modelOutput)
}

func realOCRArtifact(ctx context.Context, path, mode string, model *ocr.Recognizer) (*realOCRModelArtifact, error) {
	if ctx == nil {
		return nil, errors.New("realocr: context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	digest := model.LoadedCheckpointSHA256()
	if digest == "" {
		return nil, fmt.Errorf("realocr: model has no verified checkpoint origin")
	}
	return &realOCRModelArtifact{Path: path, Mode: mode, SHA256: digest, Updates: model.Snapshot().Updates}, nil
}

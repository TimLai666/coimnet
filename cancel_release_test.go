package coimnet_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/download"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/simulate"
	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/ipc"
	"github.com/apache/arrow/go/v17/arrow/memory"
)

// TestCancelReleases covers ticket 24, Root decision 5: each of the five SDK
// entry points returns context.Canceled after cancellation and leaves the
// process at the baseline goroutine count (within ±0) with HeapInuse back
// within 1.1x of the baseline after two GCs. Every case logs its before/after
// numbers. The 4 MiB pad is held alive only to give the heap comparison a
// meaningful floor; it never substitutes for a leak.
func TestCancelReleases(t *testing.T) {
	t.Run("simulate_Run", testCancelSimulateRun)
	t.Run("learning_Trainer_Step", testCancelTrainerStep)
	t.Run("learning_Individual_Advance", testCancelIndividualAdvance)
	t.Run("download_Fetch", testCancelDownloadFetch)
	t.Run("connectome_Build", testCancelConnectomeBuild)
}

const (
	cancelWaitDelay  = 50 * time.Millisecond
	reclaimAttempts  = 20
	reclaimSleep     = 50 * time.Millisecond
	operationTimeout = 30 * time.Second
	baselinePadBytes = 4 << 20
)

type baseline struct {
	goroutines int
	heap       uint64
}

// measureBaseline forces two GCs and records the goroutine count and HeapInuse
// right before the operation starts. The returned pad is a live 4 MiB slice the
// caller must keep reachable until after the reclaim check, so the heap
// comparison has a floor that scales with the operation instead of a few bytes.
func measureBaseline(t *testing.T) (baseline, *[]byte) {
	t.Helper()
	pad := make([]byte, baselinePadBytes)
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	b := baseline{goroutines: runtime.NumGoroutine(), heap: m.HeapInuse}
	t.Logf("before %s: goroutines=%d HeapInuse=%d", t.Name(), b.goroutines, b.heap)
	return b, &pad
}

// awaitReclaimed retries up to 20 times, GC-ing twice per attempt, until the
// goroutine count is exactly the baseline and HeapInuse is at most 1.1x it.
func awaitReclaimed(t *testing.T, b baseline, pad *[]byte) {
	t.Helper()
	for attempt := 0; attempt < reclaimAttempts; attempt++ {
		runtime.GC()
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		got := runtime.NumGoroutine()
		heapLimit := b.heap + b.heap/10
		t.Logf("after %s attempt %d: goroutines=%d (want %d) HeapInuse=%d (limit %d)", t.Name(), attempt, got, b.goroutines, m.HeapInuse, heapLimit)
		if got == b.goroutines && m.HeapInuse <= heapLimit {
			runtime.KeepAlive(pad)
			return
		}
		time.Sleep(reclaimSleep)
	}
	runtime.KeepAlive(pad)
	t.Errorf("%s: resources not released after cancellation", t.Name())
}

// runCanceled runs op on a fresh cancellable context, cancels after
// cancelWaitDelay, waits for the return and reports its error. A 30s bound
// keeps a genuinely blocked implementation from hanging the suite.
func runCanceled(t *testing.T, op func(context.Context) error) error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- op(ctx)
	}()
	time.Sleep(cancelWaitDelay)
	cancel()
	select {
	case err := <-done:
		return err
	case <-time.After(operationTimeout):
		t.Fatal("operation did not return after cancellation")
		return nil
	}
}

func testCancelSimulateRun(t *testing.T) {
	const nodes = 512
	const steps = 2000
	g := simulateFixtureGraph(t, nodes, nodes*nodes)
	protocol := lifProtocol(steps)
	params, err := simulate.UniformPositive(context.Background(), g, *protocol.Uniform)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := simulate.Build(context.Background(), g, params, protocol, simulate.Limits{MaxMemoryBytes: 1 << 30})
	if err != nil {
		t.Fatal(err)
	}
	stimulus := runner.Stimulus()
	b, pad := measureBaseline(t)
	err = runCanceled(t, func(ctx context.Context) error {
		_, err := runner.Run(ctx, stimulus)
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("simulate.Run error = %v, want context.Canceled", err)
	}
	awaitReclaimed(t, b, pad)
}

func testCancelTrainerStep(t *testing.T) {
	config, params := learnChain()
	trainer, err := learning.NewTrainer(config, params, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	input := learnInput(learnRows)
	b, pad := measureBaseline(t)
	err = runCanceled(t, func(ctx context.Context) error {
		_, err := trainer.Step(ctx, input, []float64{.5})
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Trainer.Step error = %v, want context.Canceled", err)
	}
	awaitReclaimed(t, b, pad)
}

func testCancelIndividualAdvance(t *testing.T) {
	config, params := learnChainWith(learnNodes, advanceEdges)
	ind, err := learning.NewIndividual(config, params, learning.DefaultOptions(), make([]float64, learnNodes))
	if err != nil {
		t.Fatal(err)
	}
	input := learnInput(advanceRows)
	b, pad := measureBaseline(t)
	err = runCanceled(t, func(ctx context.Context) error {
		_, err := ind.Advance(ctx, input)
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Individual.Advance error = %v, want context.Canceled", err)
	}
	awaitReclaimed(t, b, pad)
}

func testCancelDownloadFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "source.bin")
	b, pad := measureBaseline(t)
	err := runCanceled(t, func(ctx context.Context) error {
		_, err := download.Fetch(ctx, server.Client(), server.URL+"/source", path, download.Options{MaxBytes: 1 << 20})
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("download.Fetch error = %v, want context.Canceled", err)
	}
	awaitReclaimed(t, b, pad)
}

func testCancelConnectomeBuild(t *testing.T) {
	req := bigBuildRequest(t)
	b, pad := measureBaseline(t)
	err := runCanceled(t, func(ctx context.Context) error {
		_, err := connectome.Build(ctx, req)
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("connectome.Build error = %v, want context.Canceled", err)
	}
	awaitReclaimed(t, b, pad)
}

// lifProtocol is the LIF protocol from simulate/fixture_test.go expressed
// through exported names, driven by one two-unit pulse at step 0.
func lifProtocol(steps int) simulate.Protocol {
	return simulate.Protocol{
		SchemaVersion: simulate.ProtocolSchemaVersion,
		Core:          simulate.CoreLIF,
		LIF: &dynamics.LIFConfig{
			DT: 1, TauSyn: 1, ThetaMin: .5, ThetaMax: 1.5, VReset: -.5,
			RefractorySteps: 0,
			Surrogate:       dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
		},
		Injections: []simulate.Injection{{Channel: 0, Node: 0, Gain: 1}},
		Probes: []simulate.Probe{
			{Name: "all", Nodes: []int{0, 1, 2}, Reduce: simulate.ReduceSumOutput},
		},
		Stimulus:        simulate.StimulusSpec{Pulse: &simulate.Pulse{Channels: 1, Steps: steps, Channel: 0, Onset: 0, Duration: 1, Amplitude: 2}},
		ParameterSource: simulate.ParameterSourceUniform,
		Uniform:         &simulate.UniformParameters{Gain: 2, Bias: 0, LogTau: 0, ThetaRaw: 0},
	}
}

const (
	learnNodes      = 512
	learnChainEdges = 511
	learnRows       = 8000
	advanceRows     = 2048
	advanceEdges    = 32768
)

// learnChain builds the continuous-core model the Trainer case uses: one chain
// edge per node pair, one scalar input and output.
func learnChain() (learning.Config, learning.Parameters) {
	return learnChainWith(learnNodes, learnChainEdges)
}

// learnChainWith builds a continuous-core model with the given node and edge
// counts. The Individual.Advance case stays inside dynamics.MaxStateValues
// (rows*nodes) by keeping 2048x512 cells and instead adding enough edges that a
// single forward pass still needs tens of milliseconds, so the 50ms deadline
// reliably lands mid-run.
func learnChainWith(nodes, edges int) (learning.Config, learning.Parameters) {
	config := learning.Config{
		Dynamics:     dynamics.Config{Nodes: nodes, DT: .5, Activation: "tanh"},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{nodes - 1},
	}
	for i := 0; i < edges; i++ {
		config.Dynamics.Sources = append(config.Dynamics.Sources, i%nodes)
		config.Dynamics.Targets = append(config.Dynamics.Targets, (i+1)%nodes)
	}
	weights := make([]float64, edges)
	for i := range weights {
		weights[i] = .1 + float64(i%9)/100
	}
	params := learning.Parameters{
		Core:    dynamics.Parameters{Weights: weights, Bias: make([]float64, nodes), LogTau: make([]float64, nodes)},
		Encoder: make([]float64, nodes),
		Readout: []float64{.8},
	}
	return config, params
}

func learnInput(rows int) [][]float64 {
	input := make([][]float64, rows)
	for i := range input {
		input[i] = []float64{.1}
	}
	return input
}


// writeFeather writes one single-batch Feather V2 file, the same format the
// official MaleCNS sources use.
func writeFeather(t *testing.T, path string, schema *arrow.Schema, fill func(*array.RecordBuilder)) {
	t.Helper()
	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	fill(builder)
	record := builder.NewRecord()
	builder.Release()
	defer record.Release()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := ipc.NewFileWriter(file, ipc.WithSchema(schema))
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(record); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func fingerprintSource(t *testing.T, role connectome.FileRole, path string) connectome.SourceFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return connectome.SourceFile{
		Role: role, Path: path, Bytes: int64(len(data)),
		SHA256:     hex.EncodeToString(sum[:]),
		HashStatus: connectome.HashLocallyRecorded,
	}
}

var (
	weightsSchema = arrow.NewSchema([]arrow.Field{
		{Name: "body_pre", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "body_post", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "weight", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
	}, nil)
	ntSchema = arrow.NewSchema([]arrow.Field{
		{Name: "body", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "consensus_nt", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil)
	fullAnnotationSchema = arrow.NewSchema([]arrow.Field{
		{Name: "bodyId", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "status", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "class", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "superclass", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "somaSide", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil)
	trimAnnotationSchema = arrow.NewSchema([]arrow.Field{
		{Name: "bodyId", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "status", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil)
)

// simulateFixtureGraph writes the three source files for nodes selected rows
// and edges generated as pre=eight/nodes, post=eight%nodes in body ID space
// 1..nodes. It is the fixture from simulate/fixture_test.go generalised so the
// Run case has enough work to still be running at the cancel deadline.
func simulateFixtureGraph(t *testing.T, nodes, edges int) *connectome.Graph {
	t.Helper()
	dir := t.TempDir()
	writeFeather(t, filepath.Join(dir, "annotations.feather"), fullAnnotationSchema, func(b *array.RecordBuilder) {
		ids := b.Field(0).(*array.Int64Builder)
		status := b.Field(1).(*array.StringBuilder)
		class := b.Field(2).(*array.StringBuilder)
		super := b.Field(3).(*array.StringBuilder)
		side := b.Field(4).(*array.StringBuilder)
		for i := 0; i < nodes; i++ {
			ids.Append(int64(i + 1))
			status.Append("Traced")
			class.Append("ALIN")
			super.Append("cb_intrinsic")
			side.Append("L")
		}
	})
	writeFeather(t, filepath.Join(dir, "weights.feather"), weightsSchema, func(b *array.RecordBuilder) {
		pre := b.Field(0).(*array.Int64Builder)
		post := b.Field(1).(*array.Int64Builder)
		weight := b.Field(2).(*array.Int64Builder)
		for e := 0; e < edges; e++ {
			pre.Append(int64(e/nodes) + 1)
			post.Append(int64(e%nodes) + 1)
			weight.Append(int64(e%9) + 1)
		}
	})
	writeFeather(t, filepath.Join(dir, "nt.feather"), ntSchema, func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).Append(1)
		b.Field(1).(*array.StringBuilder).Append("acetylcholine")
	})
	manifest := cancellationManifest(t, dir, connectome.AnnotationFields{
		ID: "bodyId", Status: "status", Class: "class", Superclass: "superclass", SomaSide: "somaSide",
	})
	result, err := connectome.Build(context.Background(), connectome.BuildRequest{
		Manifest: manifest,
		Limits:   cancellationLimits(256 << 20),
		TempDir:  t.TempDir(),
		EdgeView: connectome.EdgeViewRows,
	})
	if err != nil {
		t.Fatalf("connectome.Build fixture graph: %v", err)
	}
	return result.Graph
}

func cancellationManifest(t *testing.T, dir string, annotations connectome.AnnotationFields) connectome.DatasetManifest {
	t.Helper()
	return connectome.DatasetManifest{
		SchemaVersion: connectome.ManifestSchemaVersion,
		Dataset:       "fixture",
		Namespace:     "cancel-release-fixture-v1",
		SourceVersion: "v1",
		License:       connectome.License{Name: "CC-BY-4.0", URL: "https://creativecommons.org/licenses/by/4.0/"},
		AcquiredAt:    "2026-09-14T00:00:00Z",
		Files: []connectome.SourceFile{
			fingerprintSource(t, connectome.RoleWeights, filepath.Join(dir, "weights.feather")),
			fingerprintSource(t, connectome.RoleAnnotations, filepath.Join(dir, "annotations.feather")),
			fingerprintSource(t, connectome.RoleNeurotransmitters, filepath.Join(dir, "nt.feather")),
		},
		FieldMapping: connectome.FieldMapping{
			Weights:           connectome.WeightsFields{Source: "body_pre", Target: "body_post", Value: "weight"},
			Annotations:       annotations,
			Neurotransmitters: connectome.NeurotransmitterFields{ID: "body", Consensus: "consensus_nt"},
		},
		Identity: connectome.IdentityMapping{
			WeightsEndpointsAreAnnotationIDs:    true,
			NeurotransmitterIDsAreAnnotationIDs: true,
			Evidence:                            "synthetic cancellation fixture",
		},
		Selection:          connectome.SelectionPredicate{Source: connectome.RoleAnnotations, Field: "status", Equals: "Traced", Label: "engineering selection"},
		DuplicateSemantics: connectome.DuplicateSemanticsUnknown,
		CoordinateUnit:     "unverified",
		TransformHistory:   []connectome.TransformStep{{Step: "generate", Description: "synthetic cancellation fixture", Version: "test"}},
	}
}

func cancellationLimits(maxMemory int64) connectome.ResourceLimits {
	return connectome.ResourceLimits{
		MaxMemoryBytes: maxMemory,
		MaxTempBytes:   1 << 30,
		MaxRunFiles:    1024,
		MaxArrowBytes:  64 << 20,
		MaxFooterBytes: 1 << 20,
		MaxRows:        1 << 23,
	}
}

const (
	bigAnnotations = 20000
	bigEdges       = 2_000_000
	bigBatch       = 250_000
)

// bigBuildRequest writes an annotations file of 20k selected IDs and a weights
// file of two million edges, large enough that a full Build takes far longer
// than the 50ms cancel deadline even before the external sort.
func bigBuildRequest(t *testing.T) connectome.BuildRequest {
	t.Helper()
	dir := t.TempDir()
	writeFeather(t, filepath.Join(dir, "annotations.feather"), trimAnnotationSchema, func(b *array.RecordBuilder) {
		ids := b.Field(0).(*array.Int64Builder)
		status := b.Field(1).(*array.StringBuilder)
		for i := 1; i <= bigAnnotations; i++ {
			ids.Append(int64(i))
			status.Append("Traced")
		}
	})
	writeWeightsBatched(t, filepath.Join(dir, "weights.feather"), bigEdges, bigBatch, func(i int64) (int64, int64, int64) {
		return i%bigAnnotations + 1, (i*31)%bigAnnotations + 1, i%10 + 1
	})
	writeFeather(t, filepath.Join(dir, "nt.feather"), ntSchema, func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).Append(1)
		b.Field(1).(*array.StringBuilder).Append("acetylcholine")
	})
	return connectome.BuildRequest{
		Manifest: cancellationManifest(t, dir, connectome.AnnotationFields{ID: "bodyId", Status: "status"}),
		Limits:   cancellationLimits(1 << 30),
		TempDir:  t.TempDir(),
		EdgeView: connectome.EdgeViewRows,
	}
}

func writeWeightsBatched(t *testing.T, path string, total, batchSize int, gen func(i int64) (int64, int64, int64)) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := ipc.NewFileWriter(file, ipc.WithSchema(weightsSchema))
	if err != nil {
		t.Fatal(err)
	}
	for start := 0; start < total; start += batchSize {
		size := batchSize
		if start+size > total {
			size = total - start
		}
		builder := array.NewRecordBuilder(memory.DefaultAllocator, weightsSchema)
		pre := builder.Field(0).(*array.Int64Builder)
		post := builder.Field(1).(*array.Int64Builder)
		weight := builder.Field(2).(*array.Int64Builder)
		for j := 0; j < size; j++ {
			p, q, v := gen(int64(start + j))
			pre.Append(p)
			post.Append(q)
			weight.Append(v)
		}
		record := builder.NewRecord()
		builder.Release()
		if err := writer.Write(record); err != nil {
			t.Fatal(err)
		}
		record.Release()
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

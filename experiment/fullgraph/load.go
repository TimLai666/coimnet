package fullgraph

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/internal/fileio"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/params"
	"github.com/TimLai666/coimnet/resources"
	"github.com/TimLai666/coimnet/simulate"
)

// LoadedModel is the full-graph learning model that Run trains, built from the three input files before any training.
type LoadedModel struct {
	Config     learning.Config
	Parameters learning.Parameters
	Options    learning.Options
	Files      map[string]string // "store", "params", "protocol" → SHA-256 of the file bytes
	Nodes      int
	Edges      int
	Sets       []simulate.ResolvedSet
	Timings    map[string]float64 // "load", "variant", "build" in milliseconds
	model      modelReport        // what Run writes as Report.Model
}

// LoadModel checks the Store, Params, Protocol, InputSet, ReadoutSet, Truncation, LearningRate and MaxMemoryMiB fields of
// o with the same rules and messages as Options.Validate (the other fields are ignored), then loads and builds exactly
// what Run trains: the store, the derived parameter set with its graph check, the compare protocol (a continuous tanh
// run block), the two named sets and the original variant, with every node and edge. It trains and writes nothing.
func LoadModel(ctx context.Context, o Options) (LoadedModel, error) {
	if ctx == nil {
		return LoadedModel{}, fmt.Errorf("fullgraph: nil context")
	}
	if err := ctx.Err(); err != nil {
		return LoadedModel{}, err
	}
	validation := DefaultOptions()
	validation.Store = o.Store
	validation.Params = o.Params
	validation.Protocol = o.Protocol
	validation.InputSet = o.InputSet
	validation.ReadoutSet = o.ReadoutSet
	validation.Truncation = o.Truncation
	validation.LearningRate = o.LearningRate
	validation.MaxMemoryMiB = o.MaxMemoryMiB
	validation.OutDir = "load-model-only"
	if err := validation.Validate(); err != nil {
		return LoadedModel{}, err
	}

	loaded := LoadedModel{Files: make(map[string]string, 3), Timings: make(map[string]float64, 3)}
	start := time.Now()
	for _, input := range []struct {
		name string
		path string
	}{{"store", o.Store}, {"params", o.Params}, {"protocol", o.Protocol}} {
		digest, err := sha256File(input.path)
		if err != nil {
			return LoadedModel{}, fmt.Errorf("fullgraph: hash %s: %w", input.name, err)
		}
		loaded.Files[input.name] = digest
	}
	memBytes := int64(o.MaxMemoryMiB) << 20
	graph, err := connectome.Load(ctx, o.Store, connectome.StoreLimits{
		MaxFileBytes: 64 << 30, MaxFooterBytes: 16 << 20, MaxMemoryBytes: memBytes,
	})
	if err != nil {
		return LoadedModel{}, fmt.Errorf("fullgraph: load store: %w", err)
	}
	paramSet, receipt, err := params.LoadWithReceipt(ctx, o.Params, params.LoadLimits{
		MaxFileBytes: 64 << 30, MaxFooterBytes: 16 << 20, MaxMemoryBytes: memBytes,
	})
	if err != nil {
		return LoadedModel{}, fmt.Errorf("fullgraph: load params: %w", err)
	}
	if err := paramSet.CheckGraph(graph); err != nil {
		return LoadedModel{}, fmt.Errorf("fullgraph: check graph: %w", err)
	}
	protocolBytes, err := fileio.ReadRegular(ctx, o.Protocol, simulate.MaxCompareProtocolBytes)
	if err != nil {
		return LoadedModel{}, fmt.Errorf("fullgraph: read protocol: %w", err)
	}
	protocol, err := simulate.DecodeCompareProtocol(bytes.NewReader(protocolBytes))
	if err != nil {
		return LoadedModel{}, fmt.Errorf("fullgraph: decode compare protocol: %w", err)
	}
	if protocol.Run.Core != simulate.CoreContinuous || protocol.Run.Continuous == nil || protocol.Run.Continuous.Activation != "tanh" {
		return LoadedModel{}, fmt.Errorf("fullgraph: protocol run block must declare a continuous core with tanh activation")
	}
	loaded.Timings["load"] = time.Since(start).Seconds() * 1000

	start = time.Now()
	var inputSet, readoutSet *simulate.NamedSet
	for i := range protocol.Sets {
		if protocol.Sets[i].Name == o.InputSet {
			inputSet = &protocol.Sets[i]
		}
		if protocol.Sets[i].Name == o.ReadoutSet {
			readoutSet = &protocol.Sets[i]
		}
	}
	if inputSet == nil {
		return LoadedModel{}, fmt.Errorf("fullgraph: protocol does not declare input set %q", o.InputSet)
	}
	if readoutSet == nil {
		return LoadedModel{}, fmt.Errorf("fullgraph: protocol does not declare readout set %q", o.ReadoutSet)
	}
	loaded.Sets, err = simulate.ResolveSets(ctx, graph, []simulate.NamedSet{*inputSet, *readoutSet})
	if err != nil {
		return LoadedModel{}, fmt.Errorf("fullgraph: resolve sets: %w", err)
	}
	variant, err := simulate.OriginalVariant(ctx, graph, paramSet, receipt.SHA256, protocol.Run, simulate.Limits{MaxMemoryBytes: memBytes})
	if err != nil {
		return LoadedModel{}, fmt.Errorf("fullgraph: original variant: %w", err)
	}
	loaded.Timings["variant"] = time.Since(start).Seconds() * 1000

	start = time.Now()
	loaded.Config, loaded.Parameters, loaded.Options, loaded.model, err = buildModel(variant, int(graph.NodeCount()), loaded.Sets[0].Nodes(), loaded.Sets[1].Nodes(), modelOptions{
		DT: protocol.Run.Continuous.DT, Truncation: o.Truncation, Rate: o.LearningRate,
	})
	if err != nil {
		return LoadedModel{}, fmt.Errorf("fullgraph: build model: %w", err)
	}
	loaded.Timings["build"] = time.Since(start).Seconds() * 1000
	loaded.Nodes = int(graph.NodeCount())
	loaded.Edges = int(graph.EdgeCount())
	return loaded, nil
}

// CheckModelMemory estimates, with the plan Run checks, the memory of steps rows with plasticEdges plastic edges and the
// given chemistry regions and channels on m, and refuses it above limitMiB; the report is returned either way.
func CheckModelMemory(m LoadedModel, steps, plasticEdges, regions, channels, limitMiB int) (resources.Report, error) {
	return checkMemory(memoryPlan(m.Config, m.Parameters, steps, plasticEdges, regions, channels), limitMiB)
}

package params

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/TimLai666/coimnet/internal/fileio"
)

const (
	// MaxMetaBytes bounds the Neuprint_Meta.csv read. The official v1.0 file
	// is 1,247,784 bytes; the limit leaves room for a later revision without
	// letting an unbounded file into memory.
	MaxMetaBytes = 8 << 20
	// maxROIHierarchyNodes bounds the ROI tree walk. The official v1.0
	// hierarchy has 5,637 nodes over five levels.
	maxROIHierarchyNodes = 1 << 20
	maxROIHierarchyDepth = 64
)

// roiNode is one node of the roiHierarchy JSON tree. The official file uses
// only "name" and "children"; any other key is ignored rather than rejected,
// because this is an external document, not a CoImNet file format.
type roiNode struct {
	Name     string    `json:"name"`
	Children []roiNode `json:"children"`
}

// readMeta parses the one data row of Neuprint_Meta.csv with a bounded read.
// The header uses the neo4j import form "name:type", so columns are matched on
// the part before the colon.
func readMeta(ctx context.Context, path string) (MetaSummary, error) {
	summary := MetaSummary{Status: StatusNotMeasured}
	data, err := fileio.ReadRegular(ctx, path, MaxMetaBytes)
	if err != nil {
		return summary, fmt.Errorf("params: read neuprint meta: %w", err)
	}
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = false
	header, err := reader.Read()
	if err != nil {
		return summary, fmt.Errorf("params: read neuprint meta header: %w", err)
	}
	row, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return summary, errors.New("params: neuprint meta has a header but no data row")
		}
		return summary, fmt.Errorf("params: read neuprint meta row: %w", err)
	}
	if _, err := reader.Read(); !errors.Is(err, io.EOF) {
		if err == nil {
			return summary, errors.New("params: neuprint meta has more than one data row")
		}
		return summary, fmt.Errorf("params: read neuprint meta: %w", err)
	}

	cells := map[string]string{}
	for i, name := range header {
		if i >= len(row) {
			break
		}
		if colon := strings.IndexByte(name, ':'); colon >= 0 {
			name = name[:colon]
		}
		if _, exists := cells[name]; exists {
			return summary, fmt.Errorf("params: neuprint meta has two %q columns", name)
		}
		cells[name] = row[i]
	}
	for _, target := range []struct {
		name  string
		value *float64
	}{
		{"postHighAccuracyThreshold", &summary.PostHighAccuracyThreshold},
		{"preHPThreshold", &summary.PreHPThreshold},
		{"postHPThreshold", &summary.PostHPThreshold},
	} {
		text, ok := cells[target.name]
		if !ok {
			return summary, fmt.Errorf("params: neuprint meta has no %s column", target.name)
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil {
			return summary, fmt.Errorf("params: neuprint meta %s = %q: %w", target.name, text, err)
		}
		*target.value = value
	}
	summary.Dataset = cells["dataset"]

	hierarchy, ok := cells["roiHierarchy"]
	if !ok {
		return summary, errors.New("params: neuprint meta has no roiHierarchy column")
	}
	var root roiNode
	decoder := json.NewDecoder(strings.NewReader(hierarchy))
	if err := decoder.Decode(&root); err != nil {
		return summary, fmt.Errorf("params: decode roiHierarchy: %w", err)
	}
	levels, nodes, err := walkROIHierarchy(root, 0, nil)
	if err != nil {
		return summary, err
	}
	summary.ROIRoot = root.Name
	summary.ROILevelCounts = levels
	summary.ROILevels = len(levels)
	summary.ROINodes = nodes
	summary.Status = StatusMeasured
	summary.ThresholdsApplied = false
	return summary, nil
}

// walkROIHierarchy counts the nodes per level of the tree.
func walkROIHierarchy(node roiNode, depth int, levels []int) ([]int, int, error) {
	if depth >= maxROIHierarchyDepth {
		return nil, 0, fmt.Errorf("params: roiHierarchy is deeper than %d levels", maxROIHierarchyDepth)
	}
	for len(levels) <= depth {
		levels = append(levels, 0)
	}
	levels[depth]++
	total := 1
	for _, child := range node.Children {
		var count int
		var err error
		levels, count, err = walkROIHierarchy(child, depth+1, levels)
		if err != nil {
			return nil, 0, err
		}
		total += count
		if total > maxROIHierarchyNodes {
			return nil, 0, fmt.Errorf("params: roiHierarchy has more than %d nodes", maxROIHierarchyNodes)
		}
	}
	return levels, total, nil
}

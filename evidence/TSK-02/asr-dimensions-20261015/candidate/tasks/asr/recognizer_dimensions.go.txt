package asr

import "fmt"

type recognizerDimensions struct {
	nodes   int
	edges   int
	classes int
	encoder int
	readout int
}

func deriveRecognizerDimensions(config RecognizerConfig) (recognizerDimensions, error) {
	mels := int64(config.FrontEnd.Mels)
	hidden := int64(config.Hidden)
	classes, ok := checkedAdd(int64(len(config.Alphabet)), 1)
	if !ok {
		return recognizerDimensions{}, errorsForRecognizerDimensions("classes")
	}
	nodes, ok := checkedAdd(mels, hidden)
	if !ok {
		return recognizerDimensions{}, errorsForRecognizerDimensions("nodes")
	}
	edges, ok := checkedProduct(nodes, hidden)
	if !ok {
		return recognizerDimensions{}, errorsForRecognizerDimensions("edges")
	}
	encoder, ok := checkedProduct(mels, mels)
	if !ok {
		return recognizerDimensions{}, errorsForRecognizerDimensions("encoder")
	}
	readout, ok := checkedProduct(hidden, classes)
	if !ok {
		return recognizerDimensions{}, errorsForRecognizerDimensions("readout")
	}

	maxInt := int64(^uint(0) >> 1)
	maxFloat64Elements := maxInt / 8
	for _, dimension := range []struct {
		name        string
		value       int64
		float64Data bool
	}{
		{name: "nodes", value: nodes, float64Data: true},
		{name: "edges", value: edges, float64Data: true},
		{name: "classes", value: classes},
		{name: "encoder", value: encoder, float64Data: true},
		{name: "readout", value: readout, float64Data: true},
	} {
		if dimension.value > maxInt {
			return recognizerDimensions{}, errorsForRecognizerDimensions(dimension.name)
		}
		if dimension.float64Data && dimension.value > maxFloat64Elements {
			return recognizerDimensions{}, fmt.Errorf("asr: %s float64 array has %d elements, exceeds platform allocation capacity", dimension.name, dimension.value)
		}
	}
	return recognizerDimensions{
		nodes:   int(nodes),
		edges:   int(edges),
		classes: int(classes),
		encoder: int(encoder),
		readout: int(readout),
	}, nil
}

func errorsForRecognizerDimensions(name string) error {
	return fmt.Errorf("asr: %s dimension cannot be represented", name)
}

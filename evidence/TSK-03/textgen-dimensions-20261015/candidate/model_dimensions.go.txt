package textgen

import "fmt"

const float64ParameterBytes = 8

// modelParameterLengths validates and computes the parameter array lengths
// without allocating storage.
func modelParameterLengths(vocabSize, embed, hidden int) (encoder, readout int, err error) {
	if vocabSize < 2 {
		return 0, 0, fmt.Errorf("vocabulary size must be at least 2")
	}
	if embed <= 0 || hidden <= 0 {
		return 0, 0, fmt.Errorf("embedding and hidden widths must be positive")
	}

	maxInt := int(^uint(0) >> 1)
	maxFloat64Elements := maxInt / float64ParameterBytes
	if vocabSize > maxFloat64Elements/embed {
		return 0, 0, fmt.Errorf("encoder parameter length is not representable")
	}
	encoder = vocabSize * embed
	if hidden > maxFloat64Elements/vocabSize {
		return 0, 0, fmt.Errorf("readout parameter length is not representable")
	}
	readout = hidden * vocabSize
	return encoder, readout, nil
}

// Package kmnist reads the official KMNIST IDX gzip files and fixes the image
// selections the benchmark examples share with their Python counterparts in
// scripts/bench, so both sides evaluate exactly the same images.
package kmnist

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

const (
	// Side is the image height and width.
	Side = 28
	// Classes is the number of KMNIST labels.
	Classes = 10
)

// Split is one IDX image and label pair as stored.
type Split struct {
	Labels []int
	pixels []byte
}

// Load reads one split: "train" or "t10k".
func Load(dir, split string) (Split, error) {
	labels, _, err := readIDX(filepath.Join(dir, split+"-labels-idx1-ubyte.gz"), 2049, 1)
	if err != nil {
		return Split{}, err
	}
	pixels, shape, err := readIDX(filepath.Join(dir, split+"-images-idx3-ubyte.gz"), 2051, 3)
	if err != nil {
		return Split{}, err
	}
	if int(shape[0]) != len(labels) || shape[1] != Side || shape[2] != Side {
		return Split{}, fmt.Errorf("kmnist: %s images have shape %v for %d labels", split, shape, len(labels))
	}
	out := Split{Labels: make([]int, len(labels)), pixels: pixels}
	for i, v := range labels {
		if int(v) >= Classes {
			return Split{}, fmt.Errorf("kmnist: %s label %d at index %d is outside the class count", split, v, i)
		}
		out.Labels[i] = int(v)
	}
	return out, nil
}

// Image returns image index scaled to [0, 1], or 1 minus that when invert is
// set.
func (s Split) Image(index int, invert bool) glyphs.Image {
	const n = Side * Side
	pixels := make([]float64, n)
	for j, b := range s.pixels[index*n : (index+1)*n] {
		v := float64(b) / 255
		if invert {
			v = 1 - v
		}
		pixels[j] = v
	}
	return glyphs.Image{Width: Side, Height: Side, Pixels: pixels}
}

// RoundRobin takes, for each position k in [from, from+count), the k-th index
// of every class in classes in turn, where a class's indices are its images in
// file order. It refuses a selection that runs past the images of any class.
func (s Split) RoundRobin(classes []int, from, count int) ([]int, error) {
	byClass := make([][]int, Classes)
	for i, l := range s.Labels {
		byClass[l] = append(byClass[l], i)
	}
	out := make([]int, 0, count*len(classes))
	for k := from; k < from+count; k++ {
		for _, c := range classes {
			if c < 0 || c >= Classes || k >= len(byClass[c]) {
				return nil, fmt.Errorf("kmnist: class %d has no image at position %d", c, k)
			}
			out = append(out, byClass[c][k])
		}
	}
	return out, nil
}

// Balanced takes the first index of each label in turn until limit indices,
// the selection examples/realocr uses. With a limit that is a multiple of the
// class count it equals RoundRobin over every class from position zero.
func (s Split) Balanced(limit int) ([]int, error) {
	byClass := make([][]int, Classes)
	for i, l := range s.Labels {
		byClass[l] = append(byClass[l], i)
	}
	out := make([]int, 0, limit)
	for round := 0; len(out) < limit; round++ {
		progress := false
		for c := 0; c < Classes && len(out) < limit; c++ {
			if round < len(byClass[c]) {
				out = append(out, byClass[c][round])
				progress = true
			}
		}
		if !progress {
			return nil, fmt.Errorf("kmnist: only %d balanced examples for the requested %d", len(out), limit)
		}
	}
	return out, nil
}

// SplitID names a selection: the SHA-256 of the indices joined by commas, with
// the policy and count in front. scripts/bench computes the same string.
func SplitID(policy string, indices []int) string {
	parts := make([]string, len(indices))
	for i, v := range indices {
		parts[i] = strconv.Itoa(v)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, ",")))
	return policy + ":" + strconv.Itoa(len(indices)) + ":" + hex.EncodeToString(sum[:])
}

// Alphabet reads the one-character class names from the dataset's
// metadata.json, in label order.
func Alphabet(dir string) ([]rune, error) {
	b, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return nil, err
	}
	var meta struct {
		Mapping []struct {
			Index int    `json:"index"`
			Char  string `json:"char"`
		} `json:"mapping"`
	}
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil, fmt.Errorf("kmnist: metadata.json: %w", err)
	}
	if len(meta.Mapping) != Classes {
		return nil, fmt.Errorf("kmnist: metadata.json maps %d classes, want %d", len(meta.Mapping), Classes)
	}
	out := make([]rune, Classes)
	for i, m := range meta.Mapping {
		r := []rune(m.Char)
		if m.Index != i || len(r) != 1 {
			return nil, fmt.Errorf("kmnist: metadata.json mapping row %d is not one character for label %d", i, i)
		}
		out[i] = r[0]
	}
	return out, nil
}

func readIDX(path string, magic uint32, dims int) ([]byte, []uint32, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	b, err := io.ReadAll(io.LimitReader(z, 64<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	head := 4 + 4*dims
	if len(b) < head || binary.BigEndian.Uint32(b) != magic {
		return nil, nil, fmt.Errorf("%s: not an IDX file with magic %d", path, magic)
	}
	shape := make([]uint32, dims)
	size := 1
	for i := range shape {
		shape[i] = binary.BigEndian.Uint32(b[4+4*i:])
		size *= int(shape[i])
	}
	if len(b)-head != size {
		return nil, nil, fmt.Errorf("%s: payload has %d bytes, header says %d", path, len(b)-head, size)
	}
	return b[head:], shape, nil
}

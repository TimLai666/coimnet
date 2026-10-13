package modulation

import (
	"fmt"
	"strconv"
)

const (
	float64Bytes     = 8
	sliceHeaderBytes = 3 * (strconv.IntSize / 8)
)

func checkFloat64Vector(name string, count int) error {
	if count < 0 {
		return fmt.Errorf("modulation: %s has a negative length %d", name, count)
	}
	maxInt := int(^uint(0) >> 1)
	if count > maxInt/float64Bytes {
		return fmt.Errorf("modulation: %s has %d float64 values, whose byte size is not representable", name, count)
	}
	return nil
}

func checkFloat64Grid(name string, rows, columns int) error {
	if rows < 0 || columns < 0 {
		return fmt.Errorf("modulation: %s has an invalid shape %d x %d", name, rows, columns)
	}
	maxInt := int(^uint(0) >> 1)
	if columns > (maxInt-sliceHeaderBytes)/float64Bytes {
		return fmt.Errorf("modulation: %s row with %d float64 values is not representable", name, columns)
	}
	rowBytes := columns*float64Bytes + sliceHeaderBytes
	if rows > maxInt/rowBytes {
		return fmt.Errorf("modulation: %s grid %d x %d is not representable", name, rows, columns)
	}
	return nil
}

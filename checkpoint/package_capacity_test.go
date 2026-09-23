package checkpoint

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// newPackageVectorModel is the C = 3 matrix-edge declaration the COR-06
// capacity constants are pinned against: 9 weights + 6 bias + 2 log_tau +
// 6 encoder + 3 readout = 26 parameters and 1*9 + 2*3 = 15 multiply-adds.
func newPackageVectorModel() (learning.Config, learning.Parameters) {
	c := learning.Config{
		Dynamics: dynamics.Config{
			Nodes:          2,
			Sources:        []int{0},
			Targets:        []int{1},
			Delays:         []int{0},
			DT:             .5,
			Activation:     "tanh",
			StateDimension: 3,
			EdgeShape:      "matrix",
		},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{1},
	}
	p := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{.3, -.1, .2, .15, .05, -.2, .1, .4, -.3}, Bias: []float64{.1, .2, .3, -.1, -.2, -.3}, LogTau: []float64{.05, -.05}},
		Encoder: []float64{.7, -.2, .3, .6, .1, -.4},
		Readout: []float64{.8, -.4, .2},
	}
	return c, p
}

// TestModelPackageCarriesCapacity checks the three legs of the COR-06 package
// contract: NewModelPackage fills Capacity from the same public API a caller
// can compute, Save/Load round trips the value unchanged, and a package
// written before the field existed still loads with a nil Capacity.
func TestModelPackageCarriesCapacity(t *testing.T) {
	c, p := newPackageVectorModel()
	pkg, err := NewModelPackage(c, p, packageUnits(), []string{"evidence/COR-06/verification.json"})
	if err != nil {
		t.Fatal(err)
	}
	network, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	want := network.Capacity(p)
	if pkg.Capacity == nil {
		t.Fatal("NewModelPackage left Capacity nil")
	}
	if *pkg.Capacity != want {
		t.Fatalf("package capacity %+v differs from Network.Capacity %+v", *pkg.Capacity, want)
	}
	if want.StateDimension != 3 || want.ParameterCount != 26 || want.FreeParameterCount != 26 || want.MultAddsPerStep != 15 {
		t.Fatalf("capacity constants = %+v, want C=3, 26 parameters, 15 multiply-adds", want)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "vector.coimpkg")
	if err := SaveModelPackage(context.Background(), path, pkg); err != nil {
		t.Fatal(err)
	}
	got, err := LoadModelPackage(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, pkg) {
		t.Fatalf("round trip changed the package:\n got %+v\nwant %+v", got, pkg)
	}

	legacy, err := LoadModelPackage(context.Background(), filepath.Join("..", "evidence", "OPS-03", "fixture", "tiny.coimpkg"))
	if err != nil {
		t.Fatalf("the pre-capacity fixture must still load: %v", err)
	}
	if legacy.Capacity != nil {
		t.Fatalf("old fixture carries capacity %+v, want nil", *legacy.Capacity)
	}
}

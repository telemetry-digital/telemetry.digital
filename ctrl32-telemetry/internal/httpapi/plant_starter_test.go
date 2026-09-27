package httpapi

import "testing"

// The demo mixing plant picture passes the server's own validation, with and without datastreams (spec 001, T026).
func TestPlantPictureValid(t *testing.T) {
	for _, ds := range [][]string{nil, {tagA}, {tagA, tagA, tagA}} {
		spec, err := validateScreenSpec(plantPicture(ds))
		if err != nil {
			t.Fatalf("%d datastreams: %v", len(ds), err)
		}
		if n := len(elementsOf(spec)); n < 40 {
			t.Errorf("only %d elements", n)
		}
	}
}

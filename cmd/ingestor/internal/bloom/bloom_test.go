package bloom

import (
	"fmt"
	"testing"
)

func TestFilterNoFalseNegatives(t *testing.T) {
	f := New(10_000, 0.01)
	for i := range 10_000 {
		f.Add(fmt.Sprintf("ticket-%d", i))
	}
	for i := range 10_000 {
		if !f.Test(fmt.Sprintf("ticket-%d", i)) {
			t.Fatalf("ticket-%d was added but Test returned false", i)
		}
	}
}

func TestFilterFalsePositiveRate(t *testing.T) {
	f := New(10_000, 0.01)
	for i := range 10_000 {
		f.Add(fmt.Sprintf("ticket-%d", i))
	}
	fp := 0
	for i := range 10_000 {
		if f.Test(fmt.Sprintf("other-%d", i)) {
			fp++
		}
	}
	if rate := float64(fp) / 10_000; rate > 0.02 {
		t.Fatalf("false positive rate %.4f, want <= 0.02", rate)
	}
}

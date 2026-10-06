package sysmem

import "testing"

func TestRead(t *testing.T) {
	r, err := Read()
	if err != nil {
		t.Fatal(err)
	}
	if r.Total < 256<<20 || r.Level == "" {
		t.Fatalf("reading %+v", r)
	}
}

func TestLevelOf(t *testing.T) {
	for _, c := range []struct {
		free uint64
		want Level
	}{{0, Critical}, {StopBytes - 1, Critical}, {StopBytes, Low}, {LowBytes - 1, Low}, {LowBytes, OK}} {
		if got := levelOf(c.free); got != c.want {
			t.Errorf("levelOf(%d) = %s; want %s", c.free, got, c.want)
		}
	}
}

func TestPressureLevel(t *testing.T) {
	for p, want := range map[uint32]Level{0: OK, 1: OK, 2: Low, 3: Low, 4: Critical, 5: Critical} {
		if got := pressureLevel(p); got != want {
			t.Errorf("pressureLevel(%d) = %s; want %s", p, got, want)
		}
	}
}

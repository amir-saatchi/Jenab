package sysmem

import "testing"

func TestParseMeminfo(t *testing.T) {
	r, err := parseMeminfo([]byte("MemTotal:       16000000 kB\nMemFree:          100000 kB\nMemAvailable:     800000 kB\n"))
	if err != nil || r.Total != 16000000*1024 || r.Free != 800000*1024 || r.Level != Low {
		t.Fatalf("%+v, %v", r, err)
	}
	if _, err := parseMeminfo([]byte("MemTotal: 1 kB\n")); err == nil {
		t.Error("no MemAvailable")
	}
}

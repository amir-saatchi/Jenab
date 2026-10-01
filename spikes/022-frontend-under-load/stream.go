package main

import (
	"encoding/json"
	rdebug "runtime/debug"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type floodCmd struct {
	ID   string `json:"id"`
	Rate int    `json:"rate"`
	N    int    `json:"n"`
}

// registerStreams adds a GoStream ("flood") as a comparison transport to events:
// the page sends {id, rate, n}; Go answers with n Tok frames then one {"end": stats} frame.
func registerStreams(app *application.App) {
	app.HandleStream("flood", func(c *application.StreamConn) {
		defer c.Close()
		for {
			var cmd floodCmd
			if err := c.ReceiveJSON(&cmd); err != nil {
				return
			}
			st := EmitStats{ID: cmd.ID, N: cmd.N, Rate: cmd.Rate}
			start := time.Now()
			st.StartEpoch = nowMs()
			for i := range cmd.N {
				if cmd.Rate > 0 {
					due := start.Add(time.Duration(float64(i) * float64(time.Second) / float64(cmd.Rate)))
					if d := time.Until(due); d > 0 {
						time.Sleep(d)
					} else {
						st.LateMaxMs = max(st.LateMaxMs, float64(-d.Microseconds())/1000)
					}
				}
				t0 := time.Now()
				b, _ := json.Marshal(Tok{I: i, T: "tok ", TS: nowMs()})
				if err := c.Send(b); err != nil {
					logf("stream send: %v", err)
					return
				}
				d := float64(time.Since(t0).Microseconds()) / 1000
				st.EmitTotalMs += d
				st.EmitMaxMs = max(st.EmitMaxMs, d)
			}
			st.EndEpoch = nowMs()
			st.AchievedHz = float64(cmd.N) / time.Since(start).Seconds()
			c.SendJSON(map[string]any{"end": st})
		}
	})
}

func wailsVersion() string {
	if bi, ok := rdebug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path == "github.com/wailsapp/wails/v3" {
				return d.Version
			}
		}
	}
	return "unknown"
}

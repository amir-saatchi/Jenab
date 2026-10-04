package agent

import (
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
)

type sent struct {
	at time.Time
	d  chat.Delta
}

func collector() (func(chat.Delta), func() []sent) {
	var mu sync.Mutex
	var out []sent
	return func(d chat.Delta) {
			mu.Lock()
			out = append(out, sent{time.Now(), d})
			mu.Unlock()
		}, func() []sent {
			mu.Lock()
			defer mu.Unlock()
			return append([]sent(nil), out...)
		}
}

func TestCoalescer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		send, got := collector()
		c := newCoalescer(deltaEvery, send)
		start := time.Now()
		// 100 deltas, one every millisecond.
		var want strings.Builder
		for i := range 100 {
			s := string(rune('a' + i%26))
			want.WriteString(s)
			c.add(chat.Delta{Message: "m", Kind: chat.PartText, Text: s, Seq: uint64(i)})
			time.Sleep(time.Millisecond)
		}
		synctest.Wait()
		time.Sleep(deltaEvery) // the timer sends the rest
		synctest.Wait()
		ds := got()
		var text strings.Builder
		for i, s := range ds {
			text.WriteString(s.d.Text)
			if i > 0 && s.at.Sub(ds[i-1].at) < deltaEvery {
				t.Errorf("deltas %d and %d are %s apart", i-1, i, s.at.Sub(ds[i-1].at))
			}
		}
		if text.String() != want.String() {
			t.Errorf("text %q, want %q", text.String(), want.String())
		}
		if n := len(ds); n < 6 || n > 8 {
			t.Errorf("%d deltas for 100 ms, want about 7", n)
		}
		if !ds[0].at.Equal(start) {
			t.Errorf("the first delta waited %s", ds[0].at.Sub(start))
		}
		if last := ds[len(ds)-1]; last.d.Seq != 99 || last.at.Sub(start) > 100*time.Millisecond+deltaEvery {
			t.Errorf("last delta %+v", last)
		}
	})
}

func TestCoalescerKeepsPartsApart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		send, got := collector()
		c := newCoalescer(deltaEvery, send)
		c.add(chat.Delta{Message: "m", Part: 0, Kind: chat.PartThinking, Text: "first"}) // sent at once
		c.add(chat.Delta{Message: "m", Part: 0, Kind: chat.PartThinking, Text: " think"})
		c.add(chat.Delta{Message: "m", Part: 1, Kind: chat.PartText, Text: "Hello"}) // sends " think" now
		c.add(chat.Delta{Message: "m", Part: 1, Kind: chat.PartText, Text: " there"})
		c.add(chat.Delta{Message: "m", Part: 2, Kind: chat.PartText, Text: "Next"})   // a text part after a tool call
		c.add(chat.Delta{Message: "retry", Part: 1, Kind: chat.PartText, Text: "Hi"}) // another try
		c.flush()
		c.flush() // nothing pending: sends nothing
		time.Sleep(time.Second)
		synctest.Wait()
		var parts []string
		for _, s := range got() {
			parts = append(parts, s.d.Text)
		}
		if strings.Join(parts, "|") != "first| think|Hello there|Next|Hi" {
			t.Errorf("deltas %q", parts)
		}
	})
}

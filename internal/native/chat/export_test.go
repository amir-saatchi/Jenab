package chat

import (
	"strings"

	"github.com/amir-saatchi/jenab/internal/id"
)

// Keeps tells how many things the screen keeps for chat ch, whose
// messages are ms: its view, chips and refresh button, and the keys of
// cards, undos and what is unfolded that name the chat or a message.
func (s *Screen) Keeps(ch id.Chat, ms []id.Message) int {
	n := 0
	if s.views[ch] != nil {
		n++
	}
	n += len(s.chips[ch])
	if s.refreshes[ch] != nil {
		n++
	}
	names := func(key string) bool {
		for _, seg := range strings.Split(key, ":") {
			if seg == string(ch) {
				return true
			}
			for _, m := range ms {
				if seg == string(m) {
					return true
				}
			}
		}
		return false
	}
	for k := range s.open {
		if names(k) {
			n++
		}
	}
	for k := range s.cards {
		if names(k) {
			n++
		}
	}
	for k := range s.undos {
		if names(k) {
			n++
		}
	}
	return n
}

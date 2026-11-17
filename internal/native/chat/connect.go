package chat

import (
	"errors"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/settings"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/store"
)

// The Connect card (ProviderCard, SPEC 3.9): a message sent with no
// provider set up is kept, and its turn stops; in place of the Retry card
// the chat shows Settings' form to connect one, with a provider running
// on this computer, such as Ollama, picked first. Once it is connected,
// the kept message is sent.

// connecting is the Connect card's form, made new for each chat it shows
// in.
type connecting struct {
	form *settings.ConnectForm
	chat id.Chat
}

// noProvider tells whether the settings, once read, have no provider.
func (s *Screen) noProvider() bool {
	v := s.st.Settings.View
	return v != nil && len(v.Settings.LLM.Providers) == 0
}

// keepConnect drops the Connect card's form, clearing the key typed in
// it, once the card no longer shows in chat ch: another chat shows, a
// provider was connected, or the turn went on.
func (s *Screen) keepConnect(ch id.Chat, shows bool) {
	if s.connect != nil && (s.connect.chat != ch || !shows) {
		s.connect.form.Clear()
		s.connect = nil
	}
}

// sendKept sends chat ch's kept message once a connect started from its
// card is done. It does so even when the card stopped showing meanwhile:
// the settings, with the new provider, are read before its models are, so
// a frame between the two drops the card.
//
// A refusal that means the message went or has nowhere to go is dropped:
// the chat is in a turn, its last turn no longer stopped on an error (the
// user pressed Retry or sent a message, so it went on), or the chat is
// gone. Any other refusal, such as a memory refresh running, is shown, and
// the Retry card, which shows in the Connect card's place once a provider
// is set up, still sends the message.
func (s *Screen) sendKept(p id.Project, ch id.Chat) {
	k := s.cardOf("failed:" + string(ch))
	k.busy = true
	s.st.Actions.Retry(p, ch, func(err error) {
		k.busy = false
		if keptGone(err) {
			return
		}
		s.st.Error(err)
	})
}

// keptGone tells whether a Retry refused with err leaves no kept message
// to send: it went with a turn, or the chat is gone.
func keptGone(err error) bool {
	return errors.Is(err, agent.ErrInTurn) || errors.Is(err, agent.ErrNoRetry) || errors.Is(err, store.ErrNotFound)
}

// connectCard is "Notes needs an AI model to answer" with the form.
func (s *Screen) connectCard(c *kit.Context, t *state.Thread) {
	p, ch := t.Project, t.Chat.ID
	if s.connect == nil || s.connect.chat != ch {
		s.keepConnect(ch, false)
		form := &settings.ConnectForm{Connected: func(app.ConnectResult) { s.sendKept(p, ch) }}
		s.connect = &connecting{form: form, chat: ch}
	}
	title := t.Chat.Title
	if title == "" {
		title = "This chat"
	}
	title += " needs an AI model to answer"
	kit.Card(c, title, func() {
		kit.CardTitle(c, title)
		kit.Muted(c, "Add a provider with your own key. Your message is kept and sent once it is connected.")
		s.connect.form.View(c, s.st)
	})
}

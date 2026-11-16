package state

import (
	"context"
	"errors"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

// ActionsAPI is what the chat screen asks the chat service to do;
// *app.ChatService has it.
type ActionsAPI interface {
	Send(ctx context.Context, p id.Project, c id.Chat, text string, quote *chat.Quote) (id.Message, error)
	Stop(ctx context.Context, p id.Project, c id.Chat) error
	Retry(ctx context.Context, p id.Project, c id.Chat) error
	Answer(ctx context.Context, p id.Project, c id.Chat, a agent.Answer) error
	SetModel(ctx context.Context, p id.Project, c id.Chat, model string) error
	SetLevel(ctx context.Context, p id.Project, c id.Chat, level string) error
	UndoState(ctx context.Context, p id.Project, c id.Chat, turn int) (agent.TurnUndoState, error)
	UndoTurn(ctx context.Context, p id.Project, c id.Chat, r agent.UndoRequest) (agent.TurnUndo, error)
	RewindState(ctx context.Context, p id.Project, c id.Chat, turn int) (agent.RewindState, error)
	Rewind(ctx context.Context, p id.Project, c id.Chat, turn int) (agent.RewindResult, error)
	UndoRewind(ctx context.Context, p id.Project, c id.Chat, rewind int64) error
	RewindUndoState(ctx context.Context, p id.Project, c id.Chat, rewind int64) (agent.TurnUndoState, error)
}

var _ ActionsAPI = (*app.ChatService)(nil)

// Uploader stores a file of the user's computer in a project's bucket, as
// the composer's attachments do (TASK-021); the native app gives one.
type Uploader func(ctx context.Context, p id.Project, path string) (app.Uploaded, error)

// Actions runs the chat screen's requests to the chat service off the UI
// thread and hands each result back on it: the React screens' ChatService
// calls. A nil then drops the result; an error with no then shows in a
// toast.
type Actions struct {
	api     ActionsAPI
	post    Post
	onError func(error)
	// Upload stores an attached file; nil means attachments are off.
	Upload Uploader
	// PickFiles asks the user for files to attach, blocking; nil means
	// the composer has no button for it.
	PickFiles func() []string
}

// NewActions makes the actions. onError shows the errors no caller takes.
func NewActions(api ActionsAPI, post Post, onError func(error)) *Actions {
	if onError == nil {
		onError = func(error) {}
	}
	return &Actions{api: api, post: post, onError: onError}
}

// call runs fn in a goroutine and gives its result to then on the UI
// thread.
func call[T any](a *Actions, fn func(ctx context.Context) (T, error), then func(T, error)) {
	go func() {
		v, err := fn(background)
		a.post(func() {
			if then != nil {
				then(v, err)
			} else if err != nil {
				a.onError(err)
			}
		})
	}()
}

func done(a *Actions, fn func(ctx context.Context) error, then func(error)) {
	var t func(struct{}, error)
	if then != nil {
		t = func(_ struct{}, err error) { then(err) }
	}
	call(a, func(ctx context.Context) (struct{}, error) { return struct{}{}, fn(ctx) }, t)
}

// Send sends a message, with the quote it replies to or nil.
func (a *Actions) Send(p id.Project, c id.Chat, text string, quote *chat.Quote, then func(id.Message, error)) {
	call(a, func(ctx context.Context) (id.Message, error) { return a.api.Send(ctx, p, c, text, quote) }, then)
}

// Stop cancels the chat's turn (SPEC 8.3).
func (a *Actions) Stop(p id.Project, c id.Chat) {
	done(a, func(ctx context.Context) error { return a.api.Stop(ctx, p, c) }, nil)
}

// Retry continues a turn that failed, or tries a waiting request now.
func (a *Actions) Retry(p id.Project, c id.Chat, then func(error)) {
	done(a, func(ctx context.Context) error { return a.api.Retry(ctx, p, c) }, then)
}

// Answer answers an approval card or an ask_user form.
func (a *Actions) Answer(p id.Project, c id.Chat, ans agent.Answer, then func(error)) {
	done(a, func(ctx context.Context) error { return a.api.Answer(ctx, p, c, ans) }, then)
}

// SetModel sets the chat's model.
func (a *Actions) SetModel(p id.Project, c id.Chat, model string, then func(error)) {
	done(a, func(ctx context.Context) error { return a.api.SetModel(ctx, p, c, model) }, then)
}

// SetLevel sets the project's approval level from the chat's composer
// (SPEC 8.8): strict, standard or auto. On success it is the opened
// project's level too, as the React composer kept it.
func (a *Actions) SetLevel(p id.Project, c id.Chat, level string, then func(error)) {
	done(a, func(ctx context.Context) error { return a.api.SetLevel(ctx, p, c, level) }, then)
}

// UndoState reads what *Undo turn* would revert, for a turn or, with
// rewind > 0, for the turns a rewind took out.
func (a *Actions) UndoState(p id.Project, c id.Chat, turn int, rewind int64, then func(agent.TurnUndoState, error)) {
	call(a, func(ctx context.Context) (agent.TurnUndoState, error) {
		if rewind > 0 {
			return a.api.RewindUndoState(ctx, p, c, rewind)
		}
		return a.api.UndoState(ctx, p, c, turn)
	}, then)
}

// UndoTurn undoes or redoes a turn's changes (SPEC 2.6).
func (a *Actions) UndoTurn(p id.Project, c id.Chat, r agent.UndoRequest, then func(agent.TurnUndo, error)) {
	call(a, func(ctx context.Context) (agent.TurnUndo, error) { return a.api.UndoTurn(ctx, p, c, r) }, then)
}

// RewindState reads what a rewind to turn would do, for its dialog.
func (a *Actions) RewindState(p id.Project, c id.Chat, turn int, then func(agent.RewindState, error)) {
	call(a, func(ctx context.Context) (agent.RewindState, error) { return a.api.RewindState(ctx, p, c, turn) }, then)
}

// Rewind takes the chat back to turn (TASK-018).
func (a *Actions) Rewind(p id.Project, c id.Chat, turn int, then func(agent.RewindResult, error)) {
	call(a, func(ctx context.Context) (agent.RewindResult, error) { return a.api.Rewind(ctx, p, c, turn) }, then)
}

// UndoRewind brings back the turns a rewind took out.
func (a *Actions) UndoRewind(p id.Project, c id.Chat, rewind int64, then func(error)) {
	done(a, func(ctx context.Context) error { return a.api.UndoRewind(ctx, p, c, rewind) }, then)
}

// Attach stores the file at path in the project's bucket.
func (a *Actions) Attach(p id.Project, path string, then func(app.Uploaded, error)) {
	call(a, func(ctx context.Context) (app.Uploaded, error) {
		if a.Upload == nil {
			return app.Uploaded{}, errors.New("attachments are off")
		}
		return a.Upload(ctx, p, path)
	}, then)
}

// Pick asks for files to attach, off the UI thread, and gives their paths
// to then on it.
func (a *Actions) Pick(then func([]string)) {
	if a.PickFiles == nil {
		return
	}
	go func() {
		paths := a.PickFiles()
		a.post(func() { then(paths) })
	}()
}

// Post runs fn on the UI thread, as the results are.
func (a *Actions) Post(fn func()) { a.post(fn) }

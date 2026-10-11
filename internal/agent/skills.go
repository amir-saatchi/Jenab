package agent

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/skill"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// Skills (SPEC 8.9). A chat loads a skill with load_skill, or with the
// first call of a tool in the skill's load_with. The name goes into
// chats.skills; the chip and the skill's text go into the tool message,
// and the request joins the text to the call's result (shape). At the
// next cut the text moves into block 1, and the result keeps a stub.

type loadArgs struct {
	Name string `json:"name"`
	File string `json:"file"`
}

// loadSkill is load_skill(name, file).
func loadSkill() tool.Tool {
	return tool.Func(tool.Spec{
		Name: "load_skill",
		Description: "Load a skill from the skill list by name. Its text arrives as the result and stays in this chat. " +
			"With file, read one of the skill's extra files instead.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"name": {"type": "string", "minLength": 1},
				"file": {"type": "string", "description": "An extra file of the skill, such as recipes.md."}
			},
			"required": ["name"],
			"additionalProperties": false
		}`),
	}, func(ctx context.Context, env *tool.Env, a loadArgs) (tool.Result, error) {
		if env.Skill == nil {
			return tool.Result{}, tool.Errorf("skills can't be loaded here")
		}
		return env.Skill(ctx, strings.TrimSpace(a.Name), strings.TrimSpace(a.File))
	})
}

// loadSkill loads a skill into the chat, or reads one of its extra files.
// Files don't count as loaded skills: they are previewed like any result.
func (r *runner) loadSkill(ctx context.Context, t *turn, tm *toolMsg, name, file string) (tool.Result, error) {
	set := r.o.skills()
	s, ok := set.Get(t.ch.Kind, name)
	if !ok {
		return tool.Result{}, tool.Errorf("there is no skill named %q; the skills are: %s", name, names(set.For(t.ch.Kind)))
	}
	if file != "" {
		text, ok := s.Files[file]
		switch {
		case ok:
			return tool.Result{Text: text}, nil
		case len(s.Files) == 0:
			return tool.Result{}, tool.Errorf("the skill %s has no extra files", name)
		}
		return tool.Result{}, tool.Errorf("the skill %s has no file %q; its files are: %s", name, file, strings.Join(s.FileNames(), ", "))
	}
	if slices.Contains(t.ch.Skills, name) {
		return tool.Result{Text: "The skill " + name + " is already loaded in this chat."}, nil
	}
	// The text is in the part after the chip; the result itself is empty.
	return tool.Result{Whole: true}, r.addSkill(ctx, t, tm, s)
}

// loadWith loads the skills whose load_with names the tool, at its first
// call in the chat; their text joins the call's result. It is a safety
// net for models that don't load skills themselves (8.9), so a skill over
// the chat's limit is left out.
func (r *runner) loadWith(ctx context.Context, t *turn, tm *toolMsg, name string) {
	for _, s := range r.o.skills().With(t.ch.Kind, name) {
		if slices.Contains(t.ch.Skills, s.Name) {
			continue
		}
		if err := r.addSkill(ctx, t, tm, s); err != nil {
			var te *tool.Error
			if !errors.As(err, &te) {
				r.o.d.Log.Error("agent: loading a skill", "skill", s.Name, "chat", t.ch.ID, "err", err)
			}
		}
	}
}

// addSkill records a skill as loaded in the chat and writes its chip and
// its text. A chat holds at most skill.MaxLoaded skills or
// skill.MaxLoadedTokens.
func (r *runner) addSkill(ctx context.Context, t *turn, tm *toolMsg, s skill.Skill) error {
	n, tokens := 1, skill.Tokens(s.Body)
	var loaded []string
	for _, name := range t.ch.Skills {
		if l, ok := r.o.skills().Get(t.ch.Kind, name); ok {
			n, tokens = n+1, tokens+skill.Tokens(l.Body)
			loaded = append(loaded, name)
		}
	}
	if n > skill.MaxLoaded || tokens > skill.MaxLoadedTokens {
		return tool.Errorf("can't load %s: a chat holds at most %d skills or %d tokens of them, and this chat has loaded %s",
			s.Name, skill.MaxLoaded, skill.MaxLoadedTokens, strings.Join(loaded, ", "))
	}
	skills := append(slices.Clone(t.ch.Skills), s.Name)
	chip := chat.Part{Kind: chat.PartNotice, Notice: &chat.Notice{Kind: chat.NoticeSkillLoaded, Text: "Skill loaded: " + s.Name, Skill: s.Name}}
	text := chat.Part{Kind: chat.PartNotice, Notice: &chat.Notice{Kind: chat.NoticeSkillText, Text: s.Text(), Skill: s.Name}}
	wctx, cancel := writing(ctx)
	defer cancel()
	r.cs.pub.Lock()
	defer r.cs.pub.Unlock()
	seq, err := r.p.Chats.SetSkills(wctx, t.ch.ID, skills)
	if err != nil {
		return err
	}
	r.cs.bumpSeq(seq)
	t.ch.Skills = skills
	i, chipSeq, err := tm.store(wctx, chip)
	if err != nil {
		return err
	}
	// The skills write has no part of its own, so the chip goes out with
	// its sequence number too: the frontend sees no gap.
	tm.publish(seq, i, chip)
	tm.publish(chipSeq, i, chip)
	i, textSeq, err := tm.store(wctx, text)
	if err != nil {
		return err
	}
	tm.publish(textSeq, i, text)
	return nil
}

// skills is the skill set without the skills that come with tools this
// build lacks (8.9).
func (o *Orchestrator) skills() *skill.Set {
	return o.d.Skills.Usable(func(name string) bool {
		_, ok := o.d.Tools.Get(name)
		return ok
	})
}

func names(ss []skill.Skill) string {
	if len(ss) == 0 {
		return "none"
	}
	ns := make([]string, len(ss))
	for i, s := range ss {
		ns[i] = s.Name
	}
	return strings.Join(ns, ", ")
}

package state

import "testing"

func TestMarkLinks(t *testing.T) {
	l := "jenab://" + string(projP) + "/docs/a.md"
	for src, want := range map[string]string{
		"See " + l + ".":                         "See <" + l + ">.",
		"فایل " + l:                              "فایل <" + l + ">",
		"[the report](" + l + ")":                "[the report](" + l + ")",
		"<" + l + ">":                            "<" + l + ">",
		"[" + l + "](" + l + ")":                 "[" + l + "](" + l + ")",
		"`" + l + "` and " + l:                   "`" + l + "` and <" + l + ">",
		"``a ` " + l + "`` " + l:                 "``a ` " + l + "`` <" + l + ">",
		"```\n" + l + "\n```\n" + l:              "```\n" + l + "\n```\n<" + l + ">",
		"[r]: " + l:                              "[r]: " + l,
		"jenab://nope/a.md":                      "jenab://nope/a.md",
		"no links":                               "no links",
		"two " + l + " and " + l + "\nnext " + l: "two <" + l + "> and <" + l + ">\nnext <" + l + ">",
	} {
		if got := MarkLinks(src); got != want {
			t.Errorf("MarkLinks(%q)\n = %q\nwant %q", src, got, want)
		}
	}
	if got, ok := CodeLink(" " + l + " "); !ok || got != l {
		t.Errorf("CodeLink = %q %v", got, ok)
	}
	if _, ok := CodeLink("go test " + l); ok {
		t.Error("code with more than a link is a link")
	}
}

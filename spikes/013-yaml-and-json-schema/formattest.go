package main

// Test 7: `format` support, built-in and custom.

import (
	"errors"
	"fmt"

	sjs "github.com/santhosh-tekuri/jsonschema/v6"
)

func formats() {
	h2("7. `format` checks")
	fmt.Println("Schema per row: `{\"type\": \"string\", \"format\": F}`. santhosh \"default\" = no `AssertFormat()` (2020-12 treats format as an annotation);")
	fmt.Println("\"assert\" = `AssertFormat()` plus the custom formats of formats.go. kaptinlin: `SetAssertFormat(true)` plus the same custom formats")
	fmt.Println("(its format callback returns only true/false, so the reason is lost). google has no format assertion and no custom formats.")
	fmt.Println("Bold = result differs from the expected one.")
	fmt.Println()
	cases := []struct {
		format, value string
		ok            bool
	}{
		{"date-time", "2026-09-27T08:00:03Z", true},
		{"date-time", "2026-09-27 08:00:03", false},
		{"date-time", "2026-02-30T08:00:00Z", false},
		{"date", "2026-09-28", true},
		{"date", "2026-9-28", false},
		{"uri", "https://api.example.com/btc?days=1", true},
		{"uri", "api.example.com/btc", false},
		{"uri", "https://exa mple.com/", false},
		{"email", "someone@example.com", true},
		{"email", "someone@", false},
		{"duration", "PT5M", true},
		{"duration", "5m", false},
		{"cron", "0 8 * * *", true},
		{"cron", "*/15 9-17 * * MON-FRI", true},
		{"cron", "0 8 * *", false},
		{"cron", "61 * * * *", false},
		{"iana-tz", "Europe/Berlin", true},
		{"iana-tz", "Asia/Tehran", true},
		{"iana-tz", "Europe/Berln", false},
		{"iana-tz", "Local", false},
		{"go-duration", "500ms", true},
		{"go-duration", "2h", true},
		{"go-duration", "2d", false},
		{"go-duration", "5 minutes", false},
		{"http-url", "https://api.example.com/btc/daily", true},
		{"http-url", "https://api.example.com/${{ inputs.coin }}", true},
		{"http-url", "${{ item.url }}", true},
		{"http-url", "ftp://api.example.com/btc", false},
		{"http-url", "file:///C:/Windows/win.ini", false},
	}
	head("format", "value", "expected", "santhosh default", "santhosh assert", "kaptinlin assert", "google")
	for _, c := range cases {
		schema := []byte(fmt.Sprintf(`{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "string", "format": %q}`, c.format))
		cell := func(valid bool, msg string) string {
			s := "valid"
			if !valid {
				s = "rejected: " + md(msg)
			}
			if valid != c.ok {
				return "**" + s + "**"
			}
			return s
		}
		var cells []any
		cells = append(cells, c.format, md(c.value), map[bool]string{true: "valid", false: "invalid"}[c.ok])
		for _, assert := range []bool{false, true} {
			sch := newSanthosh(schema, assert)
			if err := sch.Validate(c.value); err != nil {
				var out []VErr
				cells = append(cells, cell(false, firstMsg(err, &out)))
			} else {
				cells = append(cells, cell(true, ""))
			}
		}
		k := newKaptinlin(schema, true)
		if r := k.Validate(c.value); r.IsValid() {
			cells = append(cells, cell(true, ""))
		} else {
			var out []VErr
			flatKaptinlin(r, "", &out)
			m := "?"
			if len(out) > 0 {
				m = out[0].Msg
			}
			cells = append(cells, cell(false, m))
		}
		if err := newGoogle(schema).Validate(c.value); err != nil {
			cells = append(cells, cell(false, err.Error()))
		} else {
			cells = append(cells, cell(true, ""))
		}
		row(cells...)
	}
}

func firstMsg(err error, out *[]VErr) string {
	var ve *sjs.ValidationError
	if !errors.As(err, &ve) {
		return err.Error()
	}
	flatSanthosh(ve, out)
	if len(*out) == 0 {
		return err.Error()
	}
	return (*out)[0].Msg
}

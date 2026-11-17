package secret

import (
	"regexp"
	"strings"
)

// shapes find a key by how it looks, for one no longer in the keychain: a
// removed provider's key in a crash log written before it was removed.
// Each keeps its group 1 (the header or field name) and hides the rest.
// A value already replaced, [secret:NAME], is left: the values can't start
// with "[", and Shapes leaves one that starts with "secret:", as in
// "api_key: [secret:NAME]", where group 1 takes the "[".
var shapes = []*regexp.Regexp{
	// Authorization: Bearer <token>, also as a JSON or Go map field.
	regexp.MustCompile(`(?i)(\bauthorization["']?\s*[:=]\s*\[?["']?(?:bearer|basic|token)\s+)([A-Za-z0-9._~+/=-]{8,})`),
	regexp.MustCompile(`(?i)(\bbearer\s+)([A-Za-z0-9._~+/=-]{16,})`),
	// x-api-key: <key>, "api_key": "<key>", x-goog-api-key=<key>…
	regexp.MustCompile(`(?i)(\b(?:x-api-key|x-goog-api-key|api[-_]?key|access[-_]?token|secret[-_]?key)["']?\s*[:=]\s*\[?["']?)([^\s"'&,;\[\]][^\s"'&,;\]]{7,})`),
	// ?key=<key> in a URL, as Gemini takes it.
	regexp.MustCompile(`(?i)([?&](?:key|api[-_]?key|access_token|token)=)([^\s&#"'\[][^\s&#"']{7,})`),
	// Keys by their prefix: OpenAI and Anthropic, Groq, NVIDIA, xAI,
	// Hugging Face, Google, GitHub, GitLab.
	regexp.MustCompile(`()\b(?:sk-|gsk_|nvapi-|xai-|hf_|AIza|ghp_|github_pat_|glpat-)[A-Za-z0-9_-]{16,}`),
}

// Shapes hides whatever looks like a key in text, with [secret]: a bearer
// token, a field or header named like a key, a key in a URL, or a key with
// a known prefix (sk-, AIza…). It is the fallback for a key that can't be
// matched by value because the keychain no longer has it. A key with no
// such shape, sent in a header of another name, isn't found.
func Shapes(text string) string {
	for _, re := range shapes {
		text = re.ReplaceAllStringFunc(text, func(m string) string {
			g := re.FindStringSubmatch(m)
			if len(g) > 2 && strings.HasPrefix(g[2], "secret:") {
				return m
			}
			return g[1] + "[secret]"
		})
	}
	return text
}

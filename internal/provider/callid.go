package provider

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// NewCallID makes an ID for a tool call the provider sent without one.
// Each is new, so calls from different steps never share an ID.
func NewCallID() string {
	var b [8]byte
	rand.Read(b[:])
	return "call_" + hex.EncodeToString(b[:])
}

// CallIDRule is what one API kind accepts as a tool-call ID: at most Max
// bytes (0 for no limit), each one Allowed accepts (nil for any).
type CallIDRule struct {
	Max     int
	Allowed func(byte) bool
}

// Map turns a stored ID into one the API accepts, so a chat can move to
// another provider. An ID that fits is unchanged. Others become a hash of
// the ID: the same in every request, so the cached prefix holds and a
// result still matches its call.
func (r CallIDRule) Map(id string) string {
	if r.fits(id) {
		return id
	}
	h := sha256.Sum256([]byte(id))
	return "call_" + hex.EncodeToString(h[:12])
}

func (r CallIDRule) fits(id string) bool {
	if id == "" || r.Max > 0 && len(id) > r.Max {
		return false
	}
	if r.Allowed != nil {
		for i := range len(id) {
			if !r.Allowed(id[i]) {
				return false
			}
		}
	}
	return true
}

// IDChar accepts the bytes of `^[a-zA-Z0-9_-]+$`.
func IDChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

// ObjectArgs returns a call's arguments if they are a JSON object, and {}
// otherwise. Calls stored before ToolCall checked for an object may hold
// null or an array, which some APIs refuse in every later request.
func ObjectArgs(raw json.RawMessage) json.RawMessage {
	if isObject(raw) {
		return raw
	}
	return json.RawMessage(`{}`)
}

func isObject(raw []byte) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && t[0] == '{' && json.Valid(t)
}

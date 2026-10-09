package provider

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrorKind decides what happens after an error (SPEC 3.8).
type ErrorKind string

const (
	RateLimited ErrorKind = "rate_limited" // wait, then retry
	Overloaded  ErrorKind = "overloaded"   // wait, then retry
	Transport   ErrorKind = "transport"    // lost connection, cut-off or stalled stream: wait, then retry
	Quota       ErrorKind = "quota"        // daily quota or credit used up: no retry
	TooLarge    ErrorKind = "too_large"    // shrink the context; not retried as is
	BadRequest  ErrorKind = "request"      // 400, 401, 403, 404: no retry, the error is shown
)

// Error is a provider error. Message is safe to show and log: it never
// holds a key.
type Error struct {
	Kind     ErrorKind
	Provider string // the connection's name, e.g. "gemini"
	Status   int    // the HTTP status, 0 for transport errors
	// RetryAfter is how long to wait before trying again. For RateLimited
	// and Overloaded it is the end of the provider's pause. For Transport
	// it is 0: the caller waits Backoff(attempt).
	RetryAfter time.Duration
	Message    string
	Err        error // the SDK's error; not shown, may hold request details
}

func (e *Error) Error() string {
	s := string(e.Kind)
	if e.Provider != "" {
		s = e.Provider + ": " + s
	}
	if e.Status != 0 {
		s += fmt.Sprintf(" (%d)", e.Status)
	}
	if e.Message != "" {
		s += ": " + e.Message
	}
	return s
}

func (e *Error) Unwrap() error { return e.Err }

// Retryable reports whether the caller may try the same request again.
func (e *Error) Retryable() bool {
	return e.Kind == RateLimited || e.Kind == Overloaded || e.Kind == Transport
}

// maxRetryAfter caps the provider's own wait. A longer wait is treated as a
// quota: the model stops until the user acts (SPEC 3.8). A starting value.
const maxRetryAfter = 10 * time.Minute

// waits are the waits when the provider sends none (SPEC 3.8).
var waits = []time.Duration{10 * time.Second, 20 * time.Second, 40 * time.Second, 80 * time.Second, 100 * time.Second}

// Backoff is the wait before try attempt+1 (attempt counts from 0): 10 s,
// 20 s, 40 s, 80 s, then 100 s, each with up to 20 % added at random.
func Backoff(attempt int) time.Duration {
	d := waits[min(max(attempt, 0), len(waits)-1)]
	return jitter(d)
}

func jitter(d time.Duration) time.Duration {
	return d + time.Duration(rand.Int64N(int64(d)/5+1))
}

// quotaMarks are error texts that mean a daily limit or used-up credit,
// whatever the status. Gemini's per-minute limits also say "quota" and
// "check your plan and billing details", so neither word is enough
// (SPIKE-018).
var quotaMarks = []string{
	"perday", "per day", "(tpd)", "(rpd)", "weekly", "usage limit", "credit balance", "credits",
	"insufficient balance", "insufficient_quota", `"1113"`, `"1308"`, `"1310"`,
}

// Classify maps an HTTP error answer to an Error. body is the response
// body, which the caller has already redacted; header may be nil.
func Classify(provider string, status int, header http.Header, body string) *Error {
	e := &Error{Provider: provider, Status: status, Message: shorten(body, 600)}
	lower := strings.ToLower(body)
	switch {
	case status == http.StatusRequestEntityTooLarge:
		e.Kind = TooLarge
		return e
	case status >= 400 && status < 500 && containsAny(lower, quotaMarks):
		e.Kind = Quota
		return e
	case status == http.StatusTooManyRequests:
		e.Kind = RateLimited
	case status == http.StatusRequestTimeout:
		e.Kind = Transport
	case status >= 500: // 500, 502, 503, 504 and Anthropic's 529
		e.Kind = Overloaded
	default:
		e.Kind = BadRequest
		return e
	}
	if d, ok := retryAfter(header, body); ok {
		if d > maxRetryAfter {
			e.Kind = Quota
			e.Message = fmt.Sprintf("the provider asks to wait %s; %s", d.Round(time.Second), e.Message)
			return e
		}
		e.RetryAfter = d
	}
	return e
}

// TransportError wraps a lost connection, a cut-off stream or a stall.
func TransportError(provider, msg string, err error) *Error {
	return &Error{Kind: Transport, Provider: provider, Message: msg, Err: err}
}

// ErrCutOff is the cause of a stream that ended without its end marker.
var ErrCutOff = errors.New("the stream ended before the provider finished")

var (
	reRetryDelay = regexp.MustCompile(`"retryDelay"\s*:\s*"([0-9.]+)s"`)                // Gemini, in the body
	reTryAgainIn = regexp.MustCompile(`(?i)(?:try again|retry) in ([0-9.]+)(ms|s|m)\b`) // Groq and Gemini, in the message
)

// retryAfter reads the provider's wait: the retry-after-ms or retry-after
// header (seconds or a date), Gemini's retryDelay, or "try again in" or
// "retry in" in the body.
func retryAfter(h http.Header, body string) (time.Duration, bool) {
	if h != nil {
		if v := h.Get("retry-after-ms"); v != "" {
			if ms, err := strconv.ParseFloat(v, 64); err == nil && ms >= 0 {
				return time.Duration(ms * float64(time.Millisecond)), true
			}
		}
		if v := h.Get("retry-after"); v != "" {
			if s, err := strconv.ParseFloat(v, 64); err == nil && s >= 0 {
				return time.Duration(s * float64(time.Second)), true
			}
			if t, err := http.ParseTime(v); err == nil {
				return max(time.Until(t), 0), true
			}
		}
	}
	if m := reRetryDelay.FindStringSubmatch(body); m != nil {
		if s, err := strconv.ParseFloat(m[1], 64); err == nil {
			return time.Duration(s * float64(time.Second)), true
		}
	}
	if m := reTryAgainIn.FindStringSubmatch(body); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			unit := map[string]time.Duration{"ms": time.Millisecond, "s": time.Second, "m": time.Minute}[strings.ToLower(m[2])]
			return time.Duration(v * float64(unit)), true
		}
	}
	return 0, false
}

func containsAny(s string, subs []string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

// shorten collapses white space and cuts s to about n bytes, on a rune
// boundary.
func shorten(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}

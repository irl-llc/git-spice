// Package httptest provides utilities for HTTP testing.
// It includes helpers for the VCR library we use.
package httptest

import (
	"maps"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

// Sanitizer replaces sensitive or environment-specific values in recorded
// fixtures with canonical placeholders. This makes fixtures portable across
// different test environments.
type Sanitizer struct {
	// Replace is the string to search for in the fixture.
	Replace string
	// With is the canonical placeholder to substitute.
	With string
}

// TransportRecorderOptions contains options for creating a new
// [TransportRecorder].
type TransportRecorderOptions struct {
	// Update specifies whether the Recorder should update fixtures.
	Update func() bool

	// Matcher customizes how requests are matched to recorded interactions.
	Matcher func(*http.Request, cassette.Request) bool

	// Sanitizers are applied to recorded fixtures in update mode.
	// They replace environment-specific values with canonical placeholders.
	Sanitizers []Sanitizer
}

// NewTransportRecorder builds a new HTTP request recorder/replayer
// that will write fixtures to testdata/fixtures/<name>.
//
// The returned Recorder will be in recording mode if the -update flag is set,
// and in replay mode otherwise.
func NewTransportRecorder(
	t testing.TB,
	name string,
	opts TransportRecorderOptions,
) *recorder.Recorder {
	t.Helper()

	mode := recorder.ModeReplayOnly
	realTransport := http.DefaultTransport
	afterCaptureHook := func(*cassette.Interaction) error {
		return nil
	}

	if opts.Update != nil && opts.Update() {
		mode = recorder.ModeRecordOnly

		afterCaptureHook = func(i *cassette.Interaction) error {
			sanitizeHeaders(i)
			applySanitizers(i, opts.Sanitizers)
			return nil
		}
	}

	matcher := cassette.DefaultMatcher
	if opts.Matcher != nil {
		matcher = opts.Matcher
	}

	rec, err := recorder.New(filepath.Join("testdata", "fixtures", name),
		recorder.WithMode(mode),
		recorder.WithRealTransport(realTransport),
		recorder.WithSkipRequestLatency(true),
		recorder.WithHook(afterCaptureHook, recorder.AfterCaptureHook),
		recorder.WithMatcher(matcher),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, rec.Stop())
	})

	return rec
}

// sanitizeHeaders removes sensitive headers from the recorded interaction,
// keeping only an allowlist of safe headers.
func sanitizeHeaders(i *cassette.Interaction) {
	allHeaders := make(http.Header)
	maps.Copy(allHeaders, i.Request.Headers)
	maps.Copy(allHeaders, i.Response.Headers)

	var toRemove []string
	for k := range allHeaders {
		switch strings.ToLower(k) {
		case "content-type", "content-length", "user-agent",
			"x-next-page", "x-total-pages", "x-page":
			// ok
		default:
			toRemove = append(toRemove, k)
		}
	}

	for _, k := range toRemove {
		delete(i.Request.Headers, k)
		delete(i.Response.Headers, k)
	}
}

// applySanitizers replaces environment-specific values with canonical
// placeholders in URLs and bodies.
func applySanitizers(i *cassette.Interaction, sanitizers []Sanitizer) {
	for _, s := range sanitizers {
		i.Request.URL = strings.ReplaceAll(i.Request.URL, s.Replace, s.With)
		i.Request.Body = strings.ReplaceAll(i.Request.Body, s.Replace, s.With)
		i.Response.Body = strings.ReplaceAll(i.Response.Body, s.Replace, s.With)
	}
}

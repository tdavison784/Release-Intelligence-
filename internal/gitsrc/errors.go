package gitsrc

import (
	"context"
	"errors"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/fetch"
)

// Stderr fragments (git runs with LC_ALL=C) that tell what went wrong. They
// are matched case-insensitively. Not-found fragments are tested first
// because a 404 message also contains "unable to access".
var (
	notFoundHints = []string{
		"couldn't find remote ref",
		"could not find remote branch",
		"remote branch",        // "Remote branch X not found in upstream origin"
		"returned error: 404",  // dumb/smart HTTP 404
		"repository not found", // "remote: Repository not found."
		"does not appear to be a git repository",
		"unknown revision",
		"bad revision",
		"invalid reference",
		"not a valid object name",
		"not a valid ref",
		"did not match any",
	}
	unavailableHints = []string{
		"unable to access",
		"could not resolve host",
		"could not resolve proxy",
		"failed to connect",
		"couldn't connect",
		"connection refused",
		"connection reset",
		"connection timed out",
		"operation timed out",
		"network is unreachable",
		"ssl",
		"tls",
		"certificate",
		"proxy",
		"authentication failed",
		"could not read username",
		"terminal prompts disabled",
		"permission denied",
		"access denied",
		"returned error: 401",
		"returned error: 403",
		"returned error: 407",
		"returned error: 429",
		"returned error: 5",
		"early eof",
		"rpc failed",
		"unexpected disconnect",
		"remote end hung up", // also matches "the remote end hung up unexpectedly"
	}
)

// wrapErr converts an error from the Runner into one that carries the
// fetch sentinels. Errors that already carry a sentinel or a context error
// are only annotated; unclassifiable failures stay plain errors so callers
// report them as errors rather than as an unavailable source.
func wrapErr(repoURL, op string, err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, fetch.ErrNotFound), errors.Is(err, fetch.ErrUnavailable), errors.Is(err, fetch.ErrOffline),
		errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		var fe *fetch.Error
		if errors.As(err, &fe) {
			return err
		}
		return &fetch.Error{URL: repoURL, Err: err, Detail: op}
	}
	var re *RunError
	if !errors.As(err, &re) {
		return &fetch.Error{URL: repoURL, Err: err, Detail: op}
	}
	sentinel := classify(re)
	if sentinel == nil {
		return &fetch.Error{URL: repoURL, Err: err, Detail: op}
	}
	return &fetch.Error{URL: repoURL, Err: sentinel, Detail: op + ": " + re.detail()}
}

// classify maps a failed git invocation to a fetch sentinel, or nil.
func classify(re *RunError) error {
	switch {
	case re.NotInstalled, re.TimedOut:
		return fetch.ErrUnavailable
	}
	msg := strings.ToLower(re.Stderr)
	if msg == "" {
		return nil
	}
	for _, h := range notFoundHints {
		if strings.Contains(msg, h) {
			return fetch.ErrNotFound
		}
	}
	for _, h := range unavailableHints {
		if strings.Contains(msg, h) {
			return fetch.ErrUnavailable
		}
	}
	return nil
}

// offlineErr is returned when a request cannot be served from the cache in
// offline mode.
func offlineErr(repoURL, what string) error {
	return &fetch.Error{URL: repoURL, Err: fetch.ErrOffline, Detail: what}
}

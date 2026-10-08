package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/trackerprofile"
)

// Each profile rejection becomes the status the client needs to decide what to do:
// 413 for what exceeds the size, 409 for a file from a family the tracker isn't,
// 400 for the rest (including the future format version, which the code distinguishes).
func TestValidationStatus(t *testing.T) {
	cases := map[string]int{
		trackerprofile.CodeInvalid:            http.StatusBadRequest,
		trackerprofile.CodeUnsupportedVersion: http.StatusBadRequest,
		trackerprofile.CodeTooLarge:           http.StatusRequestEntityTooLarge,
		trackerprofile.CodeIncompatibleFamily: http.StatusConflict,
		"":                                    http.StatusBadRequest, // no code = invalid_profile
	}
	for code, want := range cases {
		ve := &trackerprofile.ValidationError{Path: "x", Msg: "y", Code: code}
		if got := validationStatus(ve); got != want {
			t.Errorf("code %q: status %d, want %d", code, got, want)
		}
	}
	if (&trackerprofile.ValidationError{}).ErrorCode() != trackerprofile.CodeInvalid {
		t.Error("rejection without a code must be returned as invalid_profile")
	}
}

// The third-party content notice accompanies EVERY preview, and states the two things that
// matter: the content hasn't been verified against the operator's site, and what a wrong id does.
func TestImportNotice(t *testing.T) {
	for _, want := range []string{"came from a file", "were not checked", "wrong category"} {
		if !strings.Contains(importNotice, want) {
			t.Errorf("importNotice without %q: %s", want, importNotice)
		}
	}
}

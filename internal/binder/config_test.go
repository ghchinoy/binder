package binder_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/ghchinoy/binder/internal/binder"
	"github.com/ghchinoy/binder/internal/okf/native"
)

// The typed config results must serialize byte-for-byte identically to the
// anonymous map[string]any literals cmd/config.go used before Phase 4. Go marshals
// struct fields in declaration order, so these goldens also pin the field order to
// the sorted-key order the maps produced (the binder.config/v1 wire contract).

func TestConfigGetEncodeJSON(t *testing.T) {
	svc := binder.New(native.New())
	res := svc.ConfigGet(binder.ConfigGetRequest{
		Version: "test", Key: "default_type", Value: "Note", Source: "default",
	})
	want := `{
  "binder": "binder/test",
  "command": "config get",
  "schema": "binder.config/v1",
  "result": {
    "key": "default_type",
    "source": "default",
    "value": "Note"
  }
}
`
	assertJSON(t, res.EncodeJSON, want)
}

func TestConfigSetEncodeJSON(t *testing.T) {
	svc := binder.New(native.New())
	res := svc.ConfigSet(binder.ConfigSetRequest{
		Version: "test", Key: "default_type", Value: "Guide", File: ".binder.yaml",
	})
	if res.Status != "updated" {
		t.Errorf("status = %q, want updated", res.Status)
	}
	want := `{
  "binder": "binder/test",
  "command": "config set",
  "schema": "binder.config/v1",
  "result": {
    "file": ".binder.yaml",
    "key": "default_type",
    "status": "updated",
    "value": "Guide"
  }
}
`
	assertJSON(t, res.EncodeJSON, want)
}

func TestConfigUnsetEncodeJSON(t *testing.T) {
	svc := binder.New(native.New())

	removed := svc.ConfigUnset(binder.ConfigUnsetRequest{
		Version: "test", Key: "default_type", File: ".binder.yaml", Existed: true,
	})
	if removed.Status != "removed" {
		t.Errorf("existed=true status = %q, want removed", removed.Status)
	}
	want := `{
  "binder": "binder/test",
  "command": "config unset",
  "schema": "binder.config/v1",
  "result": {
    "file": ".binder.yaml",
    "key": "default_type",
    "status": "removed"
  }
}
`
	assertJSON(t, removed.EncodeJSON, want)

	// The status vocabulary lives in the service: absent key => noop.
	noop := svc.ConfigUnset(binder.ConfigUnsetRequest{
		Version: "test", Key: "default_type", File: ".binder.yaml", Existed: false,
	})
	if noop.Status != "noop" {
		t.Errorf("existed=false status = %q, want noop", noop.Status)
	}
}

func assertJSON(t *testing.T, encode func(w io.Writer) error, want string) {
	t.Helper()
	var buf bytes.Buffer
	if err := encode(&buf); err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	if buf.String() != want {
		t.Errorf("JSON mismatch:\n--- got ---\n%s\n--- want ---\n%s", buf.String(), want)
	}
}

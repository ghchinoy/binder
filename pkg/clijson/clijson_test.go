package clijson

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// sample is a tiny report-like struct with json tags, standing in for a real
// command report in the encoder tests.
type sample struct {
	Name  string   `json:"name"`
	Count int      `json:"count"`
	Items []string `json:"items"`
}

func TestEncodeShapeAndProvenance(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, "0.1.0", "convert", sample{Name: "x", Count: 2, Items: []string{}}); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got := buf.String()
	for _, want := range []string{
		`"binder": "binder/0.1.0"`,
		`"command": "convert"`,
		`"schema": "binder.report/v1"`,
		`"result": {`,
		`"items": []`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("encoded envelope missing %q\n---\n%s", want, got)
		}
	}
	if !strings.HasSuffix(got, "}\n") {
		t.Errorf("encoded JSON must end with a trailing newline, got:\n%q", got)
	}
}

func TestEncodeDeterministic(t *testing.T) {
	enc := func() string {
		var buf bytes.Buffer
		if err := Encode(&buf, "0.1.0", "review", sample{Name: "a", Count: 1, Items: []string{"b", "a"}}); err != nil {
			t.Fatalf("Encode: %v", err)
		}
		return buf.String()
	}
	if a, b := enc(), enc(); a != b {
		t.Errorf("Encode is not deterministic:\n%s\n---\n%s", a, b)
	}
}

func TestEncodeNoHTMLEscape(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, "0.1.0", "convert", sample{Name: "a & b <c>", Items: []string{}}); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !strings.Contains(buf.String(), "a & b <c>") {
		t.Errorf("HTML escaping must be off; got:\n%s", buf.String())
	}
}

func TestExitCodeContract(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, ExitSuccess},
		{"usage", Usage(errors.New("bad flag")), ExitUsage},
		{"usage-wrapped", fmt.Errorf("ctx: %w", Usage(errors.New("bad flag"))), ExitUsage},
		{"findings", &FindingsError{Msg: "not conformant"}, ExitFindings},
		{"findings-wrapped", fmt.Errorf("ctx: %w", &FindingsError{Msg: "x"}), ExitFindings},
		{"io", errors.New("cannot read"), ExitIO},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ExitCode(c.err); got != c.want {
				t.Errorf("ExitCode(%v) = %d, want %d", c.err, got, c.want)
			}
		})
	}
}

func TestGateStrictSeam(t *testing.T) {
	// Hard non-conformance always gates, regardless of strict.
	if err := Gate(false, true, false, "nc"); err == nil {
		t.Error("hard non-conformance must gate (exit 1)")
	}
	// Advisories never gate while strict is false (the #13 default; #7 flips it).
	if err := Gate(false, false, true, "adv"); err != nil {
		t.Errorf("advisories must NOT gate under strict=false, got %v", err)
	}
	// Under strict, advisories gate — proving the seam #7 will wire.
	if err := Gate(true, false, true, "adv"); err == nil {
		t.Error("advisories must gate under strict=true")
	}
	// A clean run never gates.
	if err := Gate(false, false, false, "ok"); err != nil {
		t.Errorf("clean run must not gate, got %v", err)
	}
}

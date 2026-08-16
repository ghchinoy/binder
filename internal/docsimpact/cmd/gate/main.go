// Command gate is the CI entry point for the docs-impact check. It reads the PR
// BODY (never the diff — the template is filled in at PR-open time) from the
// PR_BODY environment variable and runs the shared docsimpact matcher against it.
//
// Exit codes deliberately separate a finding from a failure to run, so an
// absence can never be reported by a shell fallback and mistaken for "ran and
// found nothing" (issue #104, Design notes):
//
//	0  the docs-impact question is answered — pass
//	1  a finding: the section is missing or unanswered — the author must act
//	2  the gate could not run: PR_BODY was not provided (a CI configuration
//	   error, NOT an author error)
package main

import (
	"fmt"
	"os"

	"github.com/ghchinoy/binder/internal/docsimpact"
)

func main() {
	// Use LookupEnv, not Getenv: an unset variable (gate never received a body)
	// must be distinguishable from an empty body (author cleared the template).
	body, ok := os.LookupEnv("PR_BODY")
	if !ok {
		fmt.Fprintln(os.Stderr,
			"docs-impact gate: PR_BODY was not provided to the gate. This is a "+
				"CI configuration error, not a problem with the pull request.")
		os.Exit(2)
	}

	if err := docsimpact.Check(body); err != nil {
		fmt.Fprintln(os.Stderr, "docs-impact gate: "+err.Error())
		fmt.Fprintln(os.Stderr,
			"\nThis gate only checks that the question was answered, not that the "+
				"answer is correct. See .github/pull_request_template.md.")
		os.Exit(1)
	}

	fmt.Println("docs-impact gate: the Docs impact section is answered.")
}

package marketplace

import (
	"io"
	"os"
	"strings"
	"testing"
)

// extract_noun_test.go — `marketplace extract`'s REPORT names each artifact class separately.
//
// The defect (plugin-marketplace#37): one counter carried two classes while the verb's only output
// named one noun. Measured on layer-charly-internals-extra it printed "8 skill(s) written" for eight
// bodies of which six were sub-agent definitions; measured on pod-charly-automation it printed
// "1 skill(s) written" for a single agent body — 1 of 1, and the only line the verb emits.
//
// A number that mixes two nouns cannot be checked against either, which is what made this worth
// fixing rather than cosmetic: `marketplace extract` is the verb a corpus-generation run reports
// through, so its output is evidence, and evidence that names the wrong thing is not evidence.
//
// The test drives the REAL path (`extract(root)` over an authored fixture) and asserts the emitted
// LINE, not the internal counters — the counters are only observable through this message.

// TestExtract_ReportsEachArtifactClassSeparately is the regression test. An all-agent corpus is the
// sharpest case: the old message reported it as skills, and there was no other line to disagree with.
func TestExtract_ReportsEachArtifactClassSeparately(t *testing.T) {
	const node = "pr-validator"
	root := writeExtractFixture(t, agentFixture(node, "|", "# pr-validator\n\nFresh evaluation.\n"))

	stderr := captureExtractStderr(t, func() {
		if err := extract(root); err != nil {
			t.Fatalf("extract: %v", err)
		}
	})

	line := extractReportLine(stderr)
	if line == "" {
		t.Fatalf("extract printed no report line; stderr was:\n%s", stderr)
	}
	// The assertion that fails without the change: the old line was
	//   "marketplace extract: 1 skill(s) written under <root>"
	// which cannot satisfy either half of this — it names no agent, and it calls the body a skill.
	if !strings.Contains(line, "0 skill(s)") {
		t.Fatalf("report line does not report the skill count as 0 for an all-agent corpus: %q", line)
	}
	if !strings.Contains(line, "1 agent(s)") {
		t.Fatalf("report line does not report the agent count: %q\n(the defect this pins: one counter reported under the noun \"skill(s)\")", line)
	}
	// And the two counts must be distinct claims, not one number printed twice.
	if strings.Count(line, "1 ") != 1 {
		t.Fatalf("report line prints a count of 1 more than once, so the classes are not distinguished: %q", line)
	}
}

// TestExtract_ReportNamesNoClassItDidNotWrite is the paired negative: an empty corpus must report
// zeros for BOTH classes rather than falling silent or claiming work. Without this, a message that
// hard-coded "+ 0 agent(s)" would satisfy the test above while lying about a mixed corpus.
func TestExtract_ReportNamesNoClassItDidNotWrite(t *testing.T) {
	// A corpus whose only node is a skill: the opposite composition to the test above.
	root := writeExtractFixture(t, skillFixtureForReport("local-keeper", "A local fixture skill.\n"))

	stderr := captureExtractStderr(t, func() {
		if err := extract(root); err != nil {
			t.Fatalf("extract: %v", err)
		}
	})
	line := extractReportLine(stderr)
	if line == "" {
		t.Fatalf("extract printed no report line; stderr was:\n%s", stderr)
	}
	if !strings.Contains(line, "1 skill(s)") {
		t.Fatalf("report line does not report the skill count for a skills-only corpus: %q", line)
	}
	if !strings.Contains(line, "0 agent(s)") {
		t.Fatalf("report line does not report the agent count as 0: %q", line)
	}
}

// skillFixtureForReport builds one skill-kind node WITH an in-line body, so `extract` writes it and
// the skill arm of the counter runs. The authored shape matches the agent fixtures': node keyed by
// its own name, `skill:` as the kind discriminator, `content:` carrying the body.
func skillFixtureForReport(node, description string) string {
	return node + ":\n" +
		"    skill:\n" +
		"        description: " + description + "\n" +
		"        content: |\n" +
		"            # " + node + "\n" +
		"            \n" +
		"            A body the extractor moves to a file.\n"
}

// captureExtractStderr runs fn with os.Stderr redirected and returns everything written.
func captureExtractStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("closing the capture pipe: %v", err)
	}
	os.Stderr = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading the captured stderr: %v", err)
	}
	return string(out)
}

// extractReportLine returns the verb's own report line only, so an unrelated library message on the
// same stream cannot satisfy an assertion.
func extractReportLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "marketplace extract:") {
			return line
		}
	}
	return ""
}

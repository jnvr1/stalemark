package finding_test

import (
	"testing"

	"github.com/jnvr1/stalemark/internal/finding"
)

func TestFingerprint(t *testing.T) {
	base := finding.Finding{Rule: "SM-A001", Severity: finding.Error, Path: "docs/README.md", Line: 10, Subject: "build"}

	tests := []struct {
		name string
		f    finding.Finding
		same bool
	}{
		{"identical", base, true},
		{"different line", with(base, func(f *finding.Finding) { f.Line = 42 }), true},
		{"different col", with(base, func(f *finding.Finding) { f.Col = 7 }), true},
		{"different message", with(base, func(f *finding.Finding) { f.Message = "x" }), true},
		{"different subject", with(base, func(f *finding.Finding) { f.Subject = "test" }), false},
		{"different path", with(base, func(f *finding.Finding) { f.Path = "other.md" }), false},
		{"different rule", with(base, func(f *finding.Finding) { f.Rule = "SM-A002" }), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, want := tt.f.Fingerprint(), base.Fingerprint()
			if len(got) != 16 {
				t.Errorf("len(Fingerprint()) = %d, want 16: %q", len(got), got)
			}
			if (got == want) != tt.same {
				t.Errorf("Fingerprint() = %q, base = %q, want same=%v", got, want, tt.same)
			}
		})
	}
}

func TestFingerprintSeparatesFields(t *testing.T) {
	a := finding.Finding{Rule: "ab", Path: "c", Subject: "d"}
	b := finding.Finding{Rule: "a", Path: "bc", Subject: "d"}
	if a.Fingerprint() == b.Fingerprint() {
		t.Error("field boundaries are not separated in the hash input")
	}
}

func with(f finding.Finding, mut func(*finding.Finding)) finding.Finding {
	mut(&f)
	return f
}

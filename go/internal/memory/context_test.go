package memory

import (
	"os"
	"testing"
)

func TestBuildSignature(t *testing.T) {
	tests := []struct {
		name     string
		project  string
		dataset  string
		branch   string
		env      string
		expected string
	}{
		{
			name:     "full signature",
			project:  "urp-cli",
			dataset:  "UNSW-NB15",
			branch:   "master",
			env:      "fedora",
			expected: "urp-cli|UNSW-NB15|master|fedora",
		},
		{
			name:     "no dataset",
			project:  "urp-cli",
			dataset:  "",
			branch:   "main",
			env:      "local",
			expected: "urp-cli|main|local",
		},
		{
			name:     "defaults",
			project:  "",
			dataset:  "",
			branch:   "",
			env:      "",
			expected: "urp-cli|master|local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildSignature(tt.project, tt.dataset, tt.branch, tt.env)
			if result != tt.expected {
				t.Errorf("BuildSignature() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSignatureHash(t *testing.T) {
	sig := "urp-cli|master|local"
	hash := SignatureHash(sig)

	// Hash should be 8 hex chars
	if len(hash) != 8 {
		t.Errorf("SignatureHash length = %d, want 8", len(hash))
	}

	// Same input should give same hash
	hash2 := SignatureHash(sig)
	if hash != hash2 {
		t.Errorf("SignatureHash not deterministic: %s != %s", hash, hash2)
	}

	// Different input should give different hash
	hash3 := SignatureHash("different|sig")
	if hash == hash3 {
		t.Error("Different signatures should have different hashes")
	}
}

func TestIsCompatible(t *testing.T) {
	tests := []struct {
		name   string
		sigA   string
		sigB   string
		strict bool
		want   bool
	}{
		{
			name:   "exact match strict",
			sigA:   "urp-cli|master|local",
			sigB:   "urp-cli|master|local",
			strict: true,
			want:   true,
		},
		{
			name:   "different strict",
			sigA:   "urp-cli|master|local",
			sigB:   "urp-cli|main|local",
			strict: true,
			want:   false,
		},
		{
			name:   "same project loose",
			sigA:   "urp-cli|master|local",
			sigB:   "urp-cli|main|prod",
			strict: false,
			want:   true,
		},
		{
			name:   "different project loose",
			sigA:   "urp-cli|master|local",
			sigB:   "other-project|master|local",
			strict: false,
			want:   false,
		},
		{
			name:   "empty sig",
			sigA:   "",
			sigB:   "urp-cli",
			strict: false,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsCompatible(tt.sigA, tt.sigB, tt.strict)
			if result != tt.want {
				t.Errorf("IsCompatible(%q, %q, %v) = %v, want %v",
					tt.sigA, tt.sigB, tt.strict, result, tt.want)
			}
		})
	}
}

func TestNewContext(t *testing.T) {
	// Clear env vars for predictable test
	os.Unsetenv("URP_INSTANCE_ID")
	os.Unsetenv("URP_SESSION_ID")
	os.Unsetenv("URP_USER_ID")
	os.Unsetenv("URP_PROJECT")
	os.Unsetenv("URP_CONTEXT_SIGNATURE")

	ctx := NewContext()

	if ctx.InstanceID == "" {
		t.Error("InstanceID should not be empty")
	}

	if ctx.SessionID == "" {
		t.Error("SessionID should not be empty")
	}

	if ctx.UserID != "default" {
		t.Errorf("UserID = %q, want 'default'", ctx.UserID)
	}

	if ctx.Scope != "session" {
		t.Errorf("Scope = %q, want 'session'", ctx.Scope)
	}

	if ctx.StartedAt == "" {
		t.Error("StartedAt should not be empty")
	}
}

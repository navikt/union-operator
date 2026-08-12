package types

import (
	"strings"
	"testing"

	datanavnov1 "github.com/navikt/union-operator/api/v1alpha1"
)

func makeServiceAccount(name, domain, project, gcpProject string) ServiceAccount {
	return ServiceAccount{
		UnionServiceAccount: datanavnov1.UnionServiceAccount{Name: name},
		UnionEnv: &UnionEnv{
			Domain:         domain,
			Project:        project,
			GCPProjectName: gcpProject,
		},
	}
}

func TestGoogleServiceAccountName(t *testing.T) {
	tests := []struct {
		name     string
		saName   string
		domain   string
		project  string
		expected string
	}{
		{
			name:     "short name is not truncated",
			saName:   "sa",
			domain:   "dev",
			project:  "proj",
			expected: "sa-dev-proj-719a5",
		},
		{
			name:     "name at exactly 22 chars is not truncated",
			saName:   "abcdefg",
			domain:   "hijklmn",
			project:  "opqrst",
			expected: "abcdefg-hijklmn-opqrst-13085",
		},
		{
			name:     "long name is truncated to 22-char prefix",
			saName:   "dataplattform",
			domain:   "development",
			project:  "nav-data-union-restricted-dev",
			expected: "dataplattform-developm-a97a3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sa := makeServiceAccount(tt.saName, tt.domain, tt.project, "")
			got := sa.GoogleServiceAccountName()
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestGoogleServiceAccountNameFormat(t *testing.T) {
	sa := makeServiceAccount("sa", "dev", "proj", "")
	got := sa.GoogleServiceAccountName()

	// Must end with a 5-char hex suffix after a dash.
	parts := strings.Split(got, "-")
	suffix := parts[len(parts)-1]
	if len(suffix) != 5 {
		t.Errorf("expected 5-char hex suffix, got %q in %q", suffix, got)
	}
	for _, c := range suffix {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("suffix %q contains non-hex character %q", suffix, c)
		}
	}
}

func TestGoogleServiceAccountEmail(t *testing.T) {
	sa := makeServiceAccount("sa", "dev", "proj", "my-gcp-project")
	got := sa.GoogleServiceAccountEmail()
	expected := "sa-dev-proj-719a5@my-gcp-project.iam.gserviceaccount.com"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestGoogleServiceAccountEmailFormat(t *testing.T) {
	sa := makeServiceAccount("sa", "dev", "proj", "my-gcp-project")
	email := sa.GoogleServiceAccountEmail()

	if !strings.HasSuffix(email, "@my-gcp-project.iam.gserviceaccount.com") {
		t.Errorf("email %q does not end with expected domain", email)
	}

	gsaName := sa.GoogleServiceAccountName()
	if !strings.HasPrefix(email, gsaName+"@") {
		t.Errorf("email %q does not start with GSA name %q", email, gsaName)
	}
}

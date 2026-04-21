package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	datanavnov1 "github.com/navikt/union-operator/api/v1"
)

type OnpremHostMap map[string]OnpremHost

type OnpremHost struct {
	VIP      []string `json:"vip,omitempty"`
	Port     string   `json:"port"`
	Protocol string   `json:"protocol"`
}

type ServiceAccount struct {
	datanavnov1.UnionServiceAccount
	UnionEnv
}

type UnionEnv struct {
	Project        string
	Domain         string
	GCPProjectName string
}

func (u *UnionEnv) Namespace() string {
	return fmt.Sprintf("%s-%s", u.Project, u.Domain)
}

func (s *ServiceAccount) GoogleServiceAccountName() string {
	name := fmt.Sprintf("%s-%s-%s", s.Name, s.Domain, s.Project)
	hash := sha256.Sum256([]byte(name))

	prefixLength := min(23, len(name))
	return fmt.Sprintf("%s-%s", name[:prefixLength], hex.EncodeToString(hash[:])[:5])
}

func (s *ServiceAccount) GoogleServiceAccountEmail() string {
	return fmt.Sprintf("%s@%s.iam.gserviceaccount.com", s.GoogleServiceAccountName(), s.GCPProjectName)
}

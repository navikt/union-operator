package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	datanavnov1 "github.com/navikt/union-operator/api/v1"
)

type UnionEnv struct {
	Project         string
	Domain          string
	ServiceAccounts []datanavnov1.UnionServiceAccount
}

func (u *UnionEnv) Namespace() string {
	return fmt.Sprintf("%s-%s", u.Project, u.Domain)
}

func (u *UnionEnv) googleServiceAccountName(serviceAccountName string) string {
	name := fmt.Sprintf("%s-%s-%s", serviceAccountName, u.Domain, u.Project)
	hash := sha256.Sum256([]byte(name))

	prefixLength := 23
	if len(name) < prefixLength {
		prefixLength = len(name)
	}
	return fmt.Sprintf("%s-%s", name[:prefixLength], hex.EncodeToString(hash[:])[:5])
}

package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	datanavnov1 "github.com/navikt/union-operator/api/v1"
	"go.yaml.in/yaml/v2"
)

type UnionDataplaneConfig struct {
	GCPProjectName         string `yaml:"gcpProjectName"`
	FastRegistrationBucket string `yaml:"fastRegistrationBucket"`
	DataBucket             string `yaml:"dataBucket"`
	OnpremHostMapFilePath  string `yaml:"onpremHostMapFilePath"`
}

func (c *UnionDataplaneConfig) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, c)
}

type OnpremHostMap map[string]OnpremHost

type OnpremHost struct {
	VIP      []string `json:"vip,omitempty"`
	Port     string   `json:"port"`
	Protocol string   `json:"protocol"`
}

type UnionEnv struct {
	Project         string
	Domain          string
	ServiceAccounts []datanavnov1.UnionServiceAccount
	GCPProjectName  string
}

func (u *UnionEnv) Namespace() string {
	return fmt.Sprintf("%s-%s", u.Project, u.Domain)
}

func (u *UnionEnv) GoogleServiceAccountName(serviceAccountName string) string {
	name := fmt.Sprintf("%s-%s-%s", serviceAccountName, u.Domain, u.Project)
	hash := sha256.Sum256([]byte(name))

	prefixLength := min(23, len(name))
	return fmt.Sprintf("%s-%s", name[:prefixLength], hex.EncodeToString(hash[:])[:5])
}

func (u *UnionEnv) GoogleServiceAccountEmail(serviceAccountName string) string {
	return fmt.Sprintf("%s@%s.iam.gserviceaccount.com", u.GoogleServiceAccountName(serviceAccountName), u.GCPProjectName)
}

package persist

import (
	"context"
	"errors"

	"cloud.google.com/go/bigquery"
	datanavnov1 "github.com/navikt/union-operator/api/v1alpha1"
	"google.golang.org/api/googleapi"
)

// AllowlistPersister is the interface the controller depends on for persisting
// allowlist history. Using an interface allows tests to inject a no-op without
// requiring real GCP credentials.
type AllowlistPersister interface {
	PersistAllowlist(ctx context.Context, utsa *datanavnov1.UnionTeamServiceAccounts) error
}

type Persister struct {
	BigQuerySink BigQuery
}

type BigQuery struct {
	ProjectID string
	DatasetID string
	TableID   string
}

type allowListTableEntry struct {
	Team              string                 `json:"team"`
	Environment       string                 `json:"environment"`
	ServiceAccount    string                 `json:"serviceaccount"`
	InternalAllowlist []string               `json:"internalallowlist"`
	ExternalAllowlist []string               `json:"externalallowlist"`
	CloudSQLAllowlist []string               `json:"cloudsqlallowlist"`
	GithubActor       string                 `json:"githubactor"`
	GithubRepo        string                 `json:"githubrepo"`
	GithubHash        string                 `json:"githubhash"`
	CreatedTimestamp  bigquery.NullTimestamp `json:"createdtimestamp"`
}

func createAllowlistTableIfNotExists(ctx context.Context, bqClient *bigquery.Client, projectID, datasetID, tableID string) error {
	schema := bigquery.Schema{
		{Name: "team", Type: bigquery.StringFieldType, Required: true},
		{Name: "environment", Type: bigquery.StringFieldType, Required: true},
		{Name: "serviceaccount", Type: bigquery.StringFieldType, Required: true},
		{Name: "internalallowlist", Type: bigquery.StringFieldType, Repeated: true},
		{Name: "externalallowlist", Type: bigquery.StringFieldType, Repeated: true},
		{Name: "cloudsqlallowlist", Type: bigquery.StringFieldType, Repeated: true},
		{Name: "githubactor", Type: bigquery.StringFieldType},
		{Name: "githubrepo", Type: bigquery.StringFieldType},
		{Name: "githubhash", Type: bigquery.StringFieldType},
		{Name: "createdtimestamp", Type: bigquery.TimestampFieldType, Required: true},
	}

	metadata := &bigquery.TableMetadata{
		Schema: schema,
	}

	ds := bqClient.DatasetInProject(projectID, datasetID)
	err := ds.Table(tableID).Create(ctx, metadata)
	var e *googleapi.Error
	if ok := errors.As(err, &e); ok {
		if e.Code == 409 {
			// already exists
			return nil
		}
	}

	return err
}

func (p *Persister) PersistAllowlist(ctx context.Context, utsa *datanavnov1.UnionTeamServiceAccounts) (err error) {
	bqClient, err := bigquery.NewClient(ctx, p.BigQuerySink.ProjectID)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := bqClient.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if err := createAllowlistTableIfNotExists(ctx, bqClient, p.BigQuerySink.ProjectID, p.BigQuerySink.DatasetID, p.BigQuerySink.TableID); err != nil {
		return err
	}

	table := bqClient.DatasetInProject(p.BigQuerySink.ProjectID, p.BigQuerySink.DatasetID).Table(p.BigQuerySink.TableID)

	tableEntry := allowListTableEntry{
		Team:        utsa.Spec.Project,
		Environment: utsa.Spec.Domain,
	}

	if githubActor, ok := utsa.Annotations["data.nav.no/github.actor"]; ok {
		tableEntry.GithubActor = githubActor
	}

	if githubRepo, ok := utsa.Annotations["data.nav.no/github.repository"]; ok {
		tableEntry.GithubRepo = githubRepo
	}

	if githubHash, ok := utsa.Annotations["data.nav.no/github.sha"]; ok {
		tableEntry.GithubHash = githubHash
	}
	tableEntry.CreatedTimestamp = bigquery.NullTimestamp{Timestamp: utsa.CreationTimestamp.Time, Valid: true}

	for _, sa := range utsa.Spec.ServiceAccounts {
		entry := p.extractAllowlistForServiceAccount(tableEntry, sa)
		if err := table.Inserter().Put(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

func (p *Persister) extractAllowlistForServiceAccount(tableEntry allowListTableEntry, sa datanavnov1.UnionServiceAccount) allowListTableEntry {
	internalAllowList := make([]string, 0, len(sa.InternalAllowlist))
	externalAllowList := make([]string, 0, len(sa.ExternalAllowlist))
	cloudSQLAllowList := make([]string, 0, len(sa.CloudSQL))
	for _, host := range sa.InternalAllowlist {
		internalAllowList = append(internalAllowList, host.Host)
	}
	for _, host := range sa.ExternalAllowlist {
		externalAllowList = append(externalAllowList, host.Host)
	}
	for _, host := range sa.CloudSQL {
		cloudSQLAllowList = append(cloudSQLAllowList, host.IP)
	}

	return allowListTableEntry{
		Team:              tableEntry.Team,
		Environment:       tableEntry.Environment,
		ServiceAccount:    sa.Name,
		InternalAllowlist: internalAllowList,
		ExternalAllowlist: externalAllowList,
		CloudSQLAllowlist: cloudSQLAllowList,
		GithubActor:       tableEntry.GithubActor,
		GithubRepo:        tableEntry.GithubRepo,
		GithubHash:        tableEntry.GithubHash,
		CreatedTimestamp:  tableEntry.CreatedTimestamp,
	}
}

func NewPersister(sink BigQuery) *Persister {
	return &Persister{
		BigQuerySink: sink,
	}
}

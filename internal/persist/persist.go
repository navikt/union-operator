package persist

import (
	"context"
	"errors"

	"cloud.google.com/go/bigquery"
	datanavnov1 "github.com/navikt/union-operator/api/v1alpha1"
	"google.golang.org/api/googleapi"
)

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

func (p *Persister) PersistAllowlist(ctx context.Context, sink BigQuery, utsa *datanavnov1.UnionTeamServiceAccounts) error {
	bqClient, err := bigquery.NewClient(ctx, sink.ProjectID)
	if err != nil {
		return err
	}
	defer bqClient.Close()

	if err := createAllowlistTableIfNotExists(ctx, bqClient, sink.ProjectID, sink.DatasetID, sink.TableID); err != nil {
		return err
	}

	table := bqClient.DatasetInProject(sink.ProjectID, sink.DatasetID).Table(sink.TableID)

	tableEntry := allowListTableEntry{
		Team:        utsa.Spec.Project,
		Environment: utsa.Spec.Domain,
	}

	if githubActor, ok := utsa.ObjectMeta.Annotations["data.nav.no/github.actor"]; ok {
		tableEntry.GithubActor = githubActor
	}

	if githubRepo, ok := utsa.ObjectMeta.Annotations["data.nav.no/github.repository"]; ok {
		tableEntry.GithubRepo = githubRepo
	}

	if githubHash, ok := utsa.ObjectMeta.Annotations["data.nav.no/github.sha"]; ok {
		tableEntry.GithubHash = githubHash
	}
	tableEntry.CreatedTimestamp = bigquery.NullTimestamp{Timestamp: utsa.CreationTimestamp.Time, Valid: true}

	for _, sa := range utsa.Spec.ServiceAccounts {

		tableEntry, err := p.extractAllowlistForServiceAccount(tableEntry, sa)
		if err != nil {
			return err
		}
		err = table.Inserter().Put(ctx, tableEntry)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Persister) extractAllowlistForServiceAccount(tableEntry allowListTableEntry, sa datanavnov1.UnionServiceAccount) (allowListTableEntry, error) {
	internalAllowList := []string{}
	externalAllowList := []string{}
	for _, host := range sa.InternalAllowlist {
		internalAllowList = append(internalAllowList, host.Host)
	}
	for _, host := range sa.ExternalAllowlist {
		externalAllowList = append(externalAllowList, host.Host)
	}

	return allowListTableEntry{
		Team:              tableEntry.Team,
		Environment:       tableEntry.Environment,
		ServiceAccount:    sa.Name,
		InternalAllowlist: internalAllowList,
		ExternalAllowlist: externalAllowList,
		GithubActor:       tableEntry.GithubActor,
		GithubRepo:        tableEntry.GithubRepo,
		GithubHash:        tableEntry.GithubHash,
		CreatedTimestamp:  tableEntry.CreatedTimestamp,
	}, nil
}

func NewPersister(sink BigQuery) *Persister {
	return &Persister{
		BigQuerySink: sink,
	}
}

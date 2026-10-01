package ingest

// Contract stubs. Replace each with a real implementation (and delete it from
// this file) — see api.go for the documented behaviour.

import (
	"context"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

func (i *Ingester) listVersions(ctx context.Context, def *catalog.ProductDefinition) (*VersionList, error) {
	return nil, ErrNotImplemented
}

func (i *Ingester) ingestRelease(ctx context.Context, def *catalog.ProductDefinition, v domain.Version, known *VersionList) (*domain.Release, error) {
	return nil, ErrNotImplemented
}

func (i *Ingester) advisories(ctx context.Context, def *catalog.ProductDefinition) ([]domain.Advisory, []domain.Evidence, []domain.SourceStatus, error) {
	return nil, nil, nil, ErrNotImplemented
}

func (i *Ingester) checkRelationships(ctx context.Context, def *catalog.ProductDefinition, releases []domain.Version, known *VersionList) (*RelationshipReport, error) {
	return nil, ErrNotImplemented
}

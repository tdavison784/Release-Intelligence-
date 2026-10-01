package upgrade

// Contract stubs. Replace each with a real implementation (and delete it from
// this file) — see api.go for the documented behaviour.

import (
	"io"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func selectPath(versions []domain.Version, from, to domain.Version, lineage string) (*PathSelection, error) {
	return nil, ErrNotImplemented
}

func build(in Input) (*domain.UpgradeEdge, error) { return nil, ErrNotImplemented }

func renderText(w io.Writer, e *domain.UpgradeEdge, opts RenderOptions) error {
	return ErrNotImplemented
}

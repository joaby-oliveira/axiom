package local

import (
	"context"
	"encoding/hex"
	"path/filepath"

	"github.com/rgomids/axiom/internal/project"
	"github.com/rgomids/axiom/internal/projectapp"
)

// InspectRecordedSource observes the portable Project at an installed record's
// exact SourceLocation, which need not be <root>/<slug>. A source that is this
// store's own <root>/<slug> keeps the locked read with recovery detection; any
// other source uses the same safe absolute-source loader as project install
// (no links, private ownership, exactly one bounded manifest). Absence is
// reported as an absent observation, never as a candidate to create.
func (s PortableStore) InspectRecordedSource(ctx context.Context, source, slug string) (PortableObservation, error) {
	if err := ctx.Err(); err != nil {
		return PortableObservation{}, err
	}
	if !filepath.IsAbs(source) || filepath.Clean(source) != source || !project.ValidSlug(slug) {
		return PortableObservation{}, ErrUnsafe
	}
	canonical, err := trustedCanonical(source)
	if err != nil {
		return PortableObservation{}, ErrUnsafe
	}
	if canonical == filepath.Join(s.root, slug) {
		return s.Inspect(ctx, slug)
	}
	snapshot, result := portableSnapshot(ctx, source)
	switch {
	case result.Category == "cancelled":
		return PortableObservation{}, ctx.Err()
	case result.Category == "project_not_found":
		return PortableObservation{Revision: "absent"}, nil
	case result.Status == InstallationFailed:
		return PortableObservation{}, ErrUnsafe
	}
	digest, ok := snapshot.Revision().Digest()
	if !ok {
		return PortableObservation{}, ErrUnsafe
	}
	return PortableObservation{Exists: true, Revision: hex.EncodeToString(digest[:]), Snapshot: snapshot}, nil
}

// RecordCodec validates and encodes a complete application local candidate
// with the exact installation-record wire contract. It performs no I/O.
type RecordCodec struct{}

func (RecordCodec) EncodeLocal(state projectapp.LocalRecordState) ([]byte, []projectapp.Issue) {
	record, issues := NewRecord(RecordState(state))
	if len(issues) != 0 {
		return nil, invalidLocalCandidate()
	}
	wire, issues := EncodeRecord(record)
	if len(issues) != 0 {
		return nil, invalidLocalCandidate()
	}
	return wire, nil
}

// ApplicationRecord exposes a sealed record's complete state to the
// application layer without its wire format.
func ApplicationRecord(record Record) projectapp.LocalRecordState {
	return projectapp.LocalRecordState(record.State())
}

// DirectoryObserver observes only the explicit directory it is given, using the
// same non-following identity rule as initial Project setup.
type DirectoryObserver struct{}

func (DirectoryObserver) ObserveCheckout(ctx context.Context, request projectapp.CheckoutRequest) (projectapp.CheckoutFacts, []projectapp.Issue) {
	if ctx.Err() != nil {
		return projectapp.CheckoutFacts{}, []projectapp.Issue{{Phase: projectapp.LocalPhase, Field: projectapp.InstallationField, Code: projectapp.CancelledOperation}}
	}
	identity, err := DirectoryIdentity(filepath.Clean(request.ExplicitPath))
	if err != nil {
		return projectapp.CheckoutFacts{}, []projectapp.Issue{{Phase: projectapp.LocalPhase, Field: projectapp.InstallationField, Code: projectapp.InvalidSnapshot}}
	}
	return projectapp.CheckoutFacts{CanonicalIdentity: identity, Observation: projectapp.Observation{Availability: projectapp.Unverified, Basis: projectapp.NotChecked}}, nil
}

func invalidLocalCandidate() []projectapp.Issue {
	return []projectapp.Issue{{Phase: projectapp.LocalPhase, Field: projectapp.InstallationField, Code: projectapp.InvalidPreview}}
}

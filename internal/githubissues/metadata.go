package githubissues

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"github.com/rgomids/axiom/internal/workitem"
)

// MetadataEffect is adapter-owned. Concrete GitHub field names never enter the
// provider-neutral policy or Work Item domain.
type MetadataEffect struct {
	Field  string
	Values []string
}

type MetadataPreview struct {
	Surface           workitem.MetadataSurface `json:"surface"`
	Target            string                   `json:"target"`
	ObservationDigest string                   `json:"observationDigest"`
	Sources           map[string]string        `json:"sources"`
	Unsupported       []string                 `json:"unsupported,omitempty"`
	Effects           []MetadataEffect         `json:"effects"`
	Digest            string                   `json:"digest"`
}

type MetadataAuthority struct {
	Target, ObservationDigest, PreviewDigest string
}

func PrepareMetadataPreview(surface workitem.MetadataSurface, target, observationDigest string, resolution workitem.MetadataResolution) (MetadataPreview, error) {
	if target == "" || len(target) > 512 || len(observationDigest) != 64 || len(resolution.Prompts) != 0 {
		return MetadataPreview{}, errors.New("metadata preview unresolved")
	}
	if _, err := hex.DecodeString(observationDigest); err != nil {
		return MetadataPreview{}, errors.New("invalid metadata observation digest")
	}
	effects, err := MetadataEffects(surface, resolution)
	if err != nil {
		return MetadataPreview{}, err
	}
	preview := MetadataPreview{Surface: surface, Target: target, ObservationDigest: observationDigest, Sources: cloneMetadataSources(resolution.Sources), Unsupported: append([]string(nil), resolution.Unsupported...), Effects: effects}
	preview.Digest = metadataDigest(preview)
	return preview, nil
}

func (authority MetadataAuthority) Covers(preview MetadataPreview) bool {
	return authority.Target == preview.Target && authority.ObservationDigest == preview.ObservationDigest && authority.PreviewDigest == preview.Digest && preview.Digest == metadataDigest(MetadataPreview{Surface: preview.Surface, Target: preview.Target, ObservationDigest: preview.ObservationDigest, Sources: preview.Sources, Unsupported: preview.Unsupported, Effects: preview.Effects})
}

func cloneMetadataSources(input map[string]string) map[string]string {
	result := make(map[string]string, len(input))
	for name, source := range input {
		result[name] = source
	}
	return result
}

func metadataDigest(value any) string {
	wire, _ := json.Marshal(value)
	sum := sha256.Sum256(wire)
	return hex.EncodeToString(sum[:])
}

func MetadataEffects(surface workitem.MetadataSurface, resolution workitem.MetadataResolution) ([]MetadataEffect, error) {
	fields := map[string]string{"classification": "labels", "owner": "assignees"}
	if surface == workitem.WorkItemSurface {
		fields["delivery_target"] = "milestone"
		fields["tracking_state"] = "project_status"
	} else if surface == workitem.PullRequestSurface {
		fields["reviewers"] = "requested_reviewers"
	} else {
		return nil, errors.New("unsupported metadata surface")
	}
	names := make([]string, 0, len(resolution.Resolved))
	for name := range resolution.Resolved {
		if fields[name] == "" {
			return nil, errors.New("metadata intention unavailable on surface")
		}
		names = append(names, name)
	}
	sort.Strings(names)
	effects := make([]MetadataEffect, 0, len(names))
	for _, name := range names {
		effects = append(effects, MetadataEffect{Field: fields[name], Values: append([]string(nil), resolution.Resolved[name]...)})
	}
	return effects, nil
}

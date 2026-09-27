// Package runtimeprofile owns machine-local Runtime/Model Profile resolution.
// Concrete model names and credential references are configuration data, not
// Axiom domain identities.
package runtimeprofile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

const (
	FormatVersion         = 1
	MaxConfigurationBytes = 256 << 10
	maxRuntimes           = 8
	maxProfiles           = 32
	maxTextBytes          = 256
)

var (
	ErrInvalidConfiguration = errors.New("invalid runtime profile configuration")
	ErrStaleConfiguration   = errors.New("stale runtime profile configuration")
	ErrNoMatch              = errors.New("no allowed runtime profile match")
	ErrAmbiguousMatch       = errors.New("multiple allowed runtime profile matches")
)

type CapabilityStatus string

const (
	CapabilityDeclared CapabilityStatus = "declared"
	CapabilityProven   CapabilityStatus = "proven"
)

type Runtime struct {
	ID                    string   `json:"id"`
	Adapter               string   `json:"adapter"`
	Enabled               bool     `json:"enabled"`
	AllowlistedProfileIDs []string `json:"allowlistedProfileIds"`
	CredentialReference   string   `json:"credentialReference,omitempty"`
}

type ModelProfile struct {
	ID           string   `json:"id"`
	RuntimeID    string   `json:"runtimeId"`
	Model        string   `json:"model"`
	Capabilities []string `json:"capabilities"`
	Complexities []string `json:"complexities,omitempty"`
}

type Preference struct {
	Role           string `json:"role"`
	Complexity     string `json:"complexity"`
	ModelProfileID string `json:"modelProfileId"`
}

type Configuration struct {
	FormatVersion int            `json:"formatVersion"`
	Revision      uint64         `json:"revision"`
	Runtimes      []Runtime      `json:"runtimes"`
	ModelProfiles []ModelProfile `json:"modelProfiles"`
	Preferences   []Preference   `json:"preferences,omitempty"`
}

type Observation struct {
	RuntimeID        string                      `json:"runtimeId"`
	Adapter          string                      `json:"adapter"`
	Installed        bool                        `json:"installed"`
	Available        bool                        `json:"available"`
	Version          string                      `json:"version,omitempty"`
	Revision         uint64                      `json:"revision"`
	ObservedAt       time.Time                   `json:"observedAt"`
	CapabilityStatus map[string]CapabilityStatus `json:"capabilityStatus"`
}

type Observer interface {
	Observe(context.Context, string) (Observation, error)
}

type Request struct {
	ConfigurationRevision uint64   `json:"configurationRevision"`
	Role                  string   `json:"role"`
	Complexity            string   `json:"complexity"`
	Capabilities          []string `json:"capabilities"`
}

type Choice struct {
	RuntimeID             string   `json:"runtimeId"`
	Adapter               string   `json:"adapter"`
	ModelProfileID        string   `json:"modelProfileId"`
	Model                 string   `json:"model"`
	Capabilities          []string `json:"capabilities"`
	RuntimeVersion        string   `json:"runtimeVersion,omitempty"`
	ConfigurationRevision uint64   `json:"configurationRevision"`
	ObservationRevision   uint64   `json:"observationRevision"`
}

type Blocker struct {
	Code       string   `json:"code"`
	RuntimeIDs []string `json:"runtimeIds,omitempty"`
	ProfileIDs []string `json:"profileIds,omitempty"`
}

type Result struct {
	Choice  *Choice  `json:"choice,omitempty"`
	Blocker *Blocker `json:"blocker,omitempty"`
}

type Resolver struct{ observer Observer }

func NewResolver(observer Observer) Resolver { return Resolver{observer: observer} }

func (r Resolver) Resolve(ctx context.Context, cfg Configuration, request Request) (Result, error) {
	if err := Validate(cfg); err != nil {
		return blocked("invalid_configuration", nil, nil), err
	}
	if request.ConfigurationRevision != cfg.Revision {
		return blocked("stale_configuration", nil, nil), ErrStaleConfiguration
	}
	if !validToken(request.Role) || !validToken(request.Complexity) || !validTokens(request.Capabilities, false) {
		return blocked("invalid_request", nil, nil), ErrInvalidConfiguration
	}
	if r.observer == nil {
		return blocked("inventory_unavailable", nil, nil), ErrNoMatch
	}
	profiles := profileMap(cfg.ModelProfiles)
	candidates := make([]Choice, 0, len(cfg.ModelProfiles))
	seenRuntimes := make([]string, 0, len(cfg.Runtimes))
	for _, runtime := range sortedRuntimes(cfg.Runtimes) {
		seenRuntimes = append(seenRuntimes, runtime.ID)
		if !runtime.Enabled {
			continue
		}
		observation, err := r.observer.Observe(ctx, runtime.ID)
		if err != nil || !validObservation(runtime, cfg.Revision, observation) {
			continue
		}
		for _, profileID := range sortedStrings(runtime.AllowlistedProfileIDs) {
			profile := profiles[profileID]
			if profile.RuntimeID != runtime.ID || !supports(profile, observation, request) {
				continue
			}
			candidates = append(candidates, Choice{
				RuntimeID: runtime.ID, Adapter: runtime.Adapter, ModelProfileID: profile.ID,
				Model: profile.Model, Capabilities: sortedStrings(request.Capabilities), RuntimeVersion: observation.Version,
				ConfigurationRevision: cfg.Revision, ObservationRevision: observation.Revision,
			})
		}
	}
	preferred := preferredProfile(cfg.Preferences, request)
	if preferred != "" {
		filtered := candidates[:0]
		for _, candidate := range candidates {
			if candidate.ModelProfileID == preferred {
				filtered = append(filtered, candidate)
			}
		}
		candidates = filtered
	}
	if len(candidates) == 0 {
		return blocked("no_allowed_match", seenRuntimes, nil), ErrNoMatch
	}
	if len(candidates) != 1 {
		ids := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			ids = append(ids, candidate.ModelProfileID)
		}
		return blocked("ambiguous_match", nil, sortedStrings(ids)), ErrAmbiguousMatch
	}
	return Result{Choice: &candidates[0]}, nil
}

func Validate(cfg Configuration) error {
	if cfg.FormatVersion != FormatVersion || cfg.Revision == 0 || len(cfg.Runtimes) == 0 || len(cfg.Runtimes) > maxRuntimes || len(cfg.ModelProfiles) == 0 || len(cfg.ModelProfiles) > maxProfiles {
		return ErrInvalidConfiguration
	}
	runtimes := make(map[string]Runtime, len(cfg.Runtimes))
	for _, runtime := range cfg.Runtimes {
		if !validToken(runtime.ID) || (runtime.Adapter != "codex" && runtime.Adapter != "claude") || runtime.ID != runtime.Adapter || !validReference(runtime.CredentialReference) || len(runtime.AllowlistedProfileIDs) > maxProfiles {
			return ErrInvalidConfiguration
		}
		if _, exists := runtimes[runtime.ID]; exists || !validTokens(runtime.AllowlistedProfileIDs, false) {
			return ErrInvalidConfiguration
		}
		runtimes[runtime.ID] = runtime
	}
	profiles := make(map[string]ModelProfile, len(cfg.ModelProfiles))
	for _, profile := range cfg.ModelProfiles {
		if !validToken(profile.ID) || !validText(profile.Model) || !validTokens(profile.Capabilities, false) || !validTokens(profile.Complexities, true) {
			return ErrInvalidConfiguration
		}
		if _, exists := runtimes[profile.RuntimeID]; !exists {
			return ErrInvalidConfiguration
		}
		if _, exists := profiles[profile.ID]; exists {
			return ErrInvalidConfiguration
		}
		profiles[profile.ID] = profile
	}
	for _, runtime := range cfg.Runtimes {
		for _, profileID := range runtime.AllowlistedProfileIDs {
			if profile, exists := profiles[profileID]; !exists || profile.RuntimeID != runtime.ID {
				return ErrInvalidConfiguration
			}
		}
	}
	seenPreferences := map[string]bool{}
	for _, preference := range cfg.Preferences {
		key := preference.Role + "\x00" + preference.Complexity
		if !validToken(preference.Role) || !validToken(preference.Complexity) || !validToken(preference.ModelProfileID) || seenPreferences[key] {
			return ErrInvalidConfiguration
		}
		if _, exists := profiles[preference.ModelProfileID]; !exists {
			return ErrInvalidConfiguration
		}
		seenPreferences[key] = true
	}
	return nil
}

func Digest(cfg Configuration) (string, error) {
	if err := Validate(cfg); err != nil {
		return "", err
	}
	normalized := cfg
	normalized.Runtimes = sortedRuntimes(cfg.Runtimes)
	normalized.ModelProfiles = append([]ModelProfile(nil), cfg.ModelProfiles...)
	sort.Slice(normalized.ModelProfiles, func(i, j int) bool { return normalized.ModelProfiles[i].ID < normalized.ModelProfiles[j].ID })
	for index := range normalized.Runtimes {
		normalized.Runtimes[index].AllowlistedProfileIDs = sortedStrings(normalized.Runtimes[index].AllowlistedProfileIDs)
	}
	for index := range normalized.ModelProfiles {
		normalized.ModelProfiles[index].Capabilities = sortedStrings(normalized.ModelProfiles[index].Capabilities)
		normalized.ModelProfiles[index].Complexities = sortedStrings(normalized.ModelProfiles[index].Complexities)
	}
	sort.Slice(normalized.Preferences, func(i, j int) bool {
		left := normalized.Preferences[i].Role + "\x00" + normalized.Preferences[i].Complexity
		right := normalized.Preferences[j].Role + "\x00" + normalized.Preferences[j].Complexity
		return left < right
	})
	wire, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(wire)
	return hex.EncodeToString(digest[:]), nil
}

func Encode(cfg Configuration) ([]byte, error) {
	if err := Validate(cfg); err != nil {
		return nil, err
	}
	wire, err := json.Marshal(cfg)
	if err != nil || len(wire)+1 > MaxConfigurationBytes {
		return nil, ErrInvalidConfiguration
	}
	return append(wire, '\n'), nil
}

func Decode(wire []byte) (Configuration, error) {
	if len(wire) == 0 || len(wire) > MaxConfigurationBytes {
		return Configuration{}, ErrInvalidConfiguration
	}
	decoder := json.NewDecoder(strings.NewReader(string(wire)))
	decoder.DisallowUnknownFields()
	var cfg Configuration
	if err := decoder.Decode(&cfg); err != nil {
		return Configuration{}, ErrInvalidConfiguration
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) || Validate(cfg) != nil {
		return Configuration{}, ErrInvalidConfiguration
	}
	return cfg, nil
}

func supports(profile ModelProfile, observation Observation, request Request) bool {
	if len(profile.Complexities) > 0 && !contains(profile.Complexities, request.Complexity) {
		return false
	}
	for _, capability := range request.Capabilities {
		if !contains(profile.Capabilities, capability) || observation.CapabilityStatus[capability] != CapabilityProven {
			return false
		}
	}
	return true
}

func validObservation(runtime Runtime, revision uint64, observation Observation) bool {
	return observation.RuntimeID == runtime.ID && observation.Adapter == runtime.Adapter && observation.Revision == revision && observation.Installed && observation.Available && !observation.ObservedAt.IsZero()
}

func preferredProfile(preferences []Preference, request Request) string {
	for _, preference := range preferences {
		if preference.Role == request.Role && preference.Complexity == request.Complexity {
			return preference.ModelProfileID
		}
	}
	return ""
}

func blocked(code string, runtimes, profiles []string) Result {
	return Result{Blocker: &Blocker{Code: code, RuntimeIDs: runtimes, ProfileIDs: profiles}}
}

func profileMap(profiles []ModelProfile) map[string]ModelProfile {
	result := make(map[string]ModelProfile, len(profiles))
	for _, profile := range profiles {
		result[profile.ID] = profile
	}
	return result
}

func sortedRuntimes(values []Runtime) []Runtime {
	result := append([]Runtime(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func validTokens(values []string, allowEmpty bool) bool {
	if !allowEmpty && len(values) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if !validToken(value) || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func validToken(value string) bool {
	if value == "" || len(value) > maxTextBytes {
		return false
	}
	for _, char := range value {
		if !((char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.') {
			return false
		}
	}
	return true
}

func validText(value string) bool {
	return value != "" && len(value) <= maxTextBytes && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n")
}

func validReference(value string) bool {
	return value == "" || (validText(value) && !strings.Contains(value, "="))
}

func (b Blocker) Error() string { return fmt.Sprintf("runtime resolution blocked: %s", b.Code) }

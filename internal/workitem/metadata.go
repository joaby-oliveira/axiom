package workitem

import (
	"bytes"
	"errors"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

const MaxMetadataPolicyBytes = 64 * 1024

type MetadataSurface string

const (
	WorkItemSurface    MetadataSurface = "work_item"
	PullRequestSurface MetadataSurface = "pull_request"
)

type MetadataRequirement string

const (
	MetadataRequired MetadataRequirement = "required"
	MetadataOptional MetadataRequirement = "optional"
)

type MetadataRule struct {
	Requirement MetadataRequirement
	Default     []string
	Reference   string
}

type MetadataPolicy struct {
	WorkItem, PullRequest map[string]MetadataRule
}

type metadataPolicyDTO struct {
	Kind        string                     `yaml:"kind"`
	Version     int                        `yaml:"policyVersion"`
	WorkItem    map[string]metadataRuleDTO `yaml:"workItem,omitempty"`
	PullRequest map[string]metadataRuleDTO `yaml:"pullRequest,omitempty"`
}
type metadataRuleDTO struct {
	Requirement string    `yaml:"requirement"`
	Default     yaml.Node `yaml:"default,omitempty"`
	Reference   string    `yaml:"reference,omitempty"`
}

// DecodeMetadataPolicies selects exactly zero or one recognized policy from
// already containment-validated Project policy documents.
func DecodeMetadataPolicies(documents [][]byte) (*MetadataPolicy, error) {
	var selected *MetadataPolicy
	for _, document := range documents {
		var header struct {
			Kind string `yaml:"kind"`
		}
		if len(document) == 0 || len(document) > MaxMetadataPolicyBytes || yaml.Unmarshal(document, &header) != nil {
			return nil, errors.New("malformed policy document")
		}
		if header.Kind != "work-item-metadata" {
			continue
		}
		if selected != nil {
			return nil, errors.New("duplicate work-item-metadata policy")
		}
		policy, err := DecodeMetadataPolicy(document)
		if err != nil {
			return nil, err
		}
		selected = &policy
	}
	return selected, nil
}

func DecodeMetadataPolicy(input []byte) (MetadataPolicy, error) {
	if len(input) == 0 || len(input) > MaxMetadataPolicyBytes || !utf8.Valid(input) {
		return MetadataPolicy{}, errors.New("malformed metadata policy")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(input))
	decoder.KnownFields(true)
	var dto metadataPolicyDTO
	if err := decoder.Decode(&dto); err != nil {
		return MetadataPolicy{}, errors.New("malformed metadata policy")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return MetadataPolicy{}, errors.New("multiple metadata policy documents")
	}
	if dto.Kind != "work-item-metadata" || dto.Version != 1 {
		return MetadataPolicy{}, errors.New("unsupported metadata policy")
	}
	workItem, err := decodeMetadataRules(dto.WorkItem, map[string]bool{"classification": true, "owner": true, "delivery_target": true, "tracking_state": true}, false)
	if err != nil {
		return MetadataPolicy{}, err
	}
	pullRequest, err := decodeMetadataRules(dto.PullRequest, map[string]bool{"classification": true, "owner": true, "reviewers": true}, true)
	if err != nil {
		return MetadataPolicy{}, err
	}
	return MetadataPolicy{WorkItem: workItem, PullRequest: pullRequest}, nil
}

func decodeMetadataRules(values map[string]metadataRuleDTO, allowed map[string]bool, reviewersList bool) (map[string]MetadataRule, error) {
	result := make(map[string]MetadataRule, len(values))
	for name, dto := range values {
		if !allowed[name] || dto.Requirement != string(MetadataRequired) && dto.Requirement != string(MetadataOptional) || dto.Reference != "" && !validMetadataValue(dto.Reference) {
			return nil, errors.New("invalid metadata policy rule")
		}
		if dto.Reference != "" && dto.Default.Kind != 0 {
			return nil, errors.New("conflicting metadata policy rule")
		}
		defaults, err := decodeMetadataDefault(dto.Default, reviewersList && name == "reviewers")
		if err != nil {
			return nil, err
		}
		result[name] = MetadataRule{Requirement: MetadataRequirement(dto.Requirement), Default: defaults, Reference: dto.Reference}
	}
	return result, nil
}

func decodeMetadataDefault(node yaml.Node, list bool) ([]string, error) {
	if node.Kind == 0 {
		return nil, nil
	}
	if list {
		if node.Kind != yaml.SequenceNode || len(node.Content) == 0 || len(node.Content) > 16 {
			return nil, errors.New("invalid metadata default")
		}
		result := make([]string, 0, len(node.Content))
		for _, child := range node.Content {
			if child.Kind != yaml.ScalarNode || child.Tag != "!!str" || !validMetadataValue(child.Value) {
				return nil, errors.New("invalid metadata default")
			}
			result = append(result, child.Value)
		}
		return result, nil
	}
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" || !validMetadataValue(node.Value) {
		return nil, errors.New("invalid metadata default")
	}
	return []string{node.Value}, nil
}

type MetadataResolution struct {
	Resolved    map[string][]string
	Sources     map[string]string
	Prompts     []string
	Unsupported []string
}

// ResolveMetadata applies explicit -> policy -> validated context precedence.
func ResolveMetadata(surface MetadataSurface, policy MetadataPolicy, explicit, context map[string][]string, capabilities map[string]bool) (MetadataResolution, error) {
	rules := policy.WorkItem
	allowed := map[string]bool{"classification": true, "owner": true, "delivery_target": true, "tracking_state": true}
	if surface == PullRequestSurface {
		rules = policy.PullRequest
		allowed = map[string]bool{"classification": true, "owner": true, "reviewers": true}
	} else if surface != WorkItemSurface {
		return MetadataResolution{}, errors.New("unsupported metadata surface")
	}
	for name, values := range explicit {
		if !allowed[name] || len(values) == 0 || len(validMetadataValues(values)) == 0 {
			return MetadataResolution{}, errors.New("invalid explicit metadata")
		}
	}
	result := MetadataResolution{Resolved: map[string][]string{}, Sources: map[string]string{}}
	names := make(map[string]MetadataRule, len(rules)+len(explicit))
	for name, rule := range rules {
		names[name] = rule
	}
	for name := range explicit {
		if _, exists := names[name]; !exists {
			names[name] = MetadataRule{Requirement: MetadataOptional}
		}
	}
	for _, name := range sortedRuleNames(names) {
		rule := names[name]
		values, source := validMetadataValues(explicit[name]), "explicit"
		if len(values) == 0 {
			values, source = validMetadataValues(rule.Default), "policy"
		}
		if len(values) == 0 && rule.Reference != "" {
			values, source = validMetadataValues(context[rule.Reference]), "policy_reference"
		}
		if len(values) == 0 {
			values, source = validMetadataValues(context[name]), "context"
		}
		if !capabilities[name] {
			if rule.Requirement == MetadataRequired {
				return MetadataResolution{}, errors.New("required metadata capability unsupported")
			}
			result.Unsupported = append(result.Unsupported, name)
			continue
		}
		if len(values) == 0 {
			if rule.Requirement == MetadataRequired {
				result.Prompts = append(result.Prompts, name)
			}
			continue
		}
		result.Resolved[name], result.Sources[name] = values, source
	}
	return result, nil
}

func validMetadataValues(values []string) []string {
	if len(values) > 16 {
		return nil
	}
	result := make([]string, len(values))
	for i, value := range values {
		if !validMetadataValue(value) {
			return nil
		}
		result[i] = value
	}
	return result
}
func validMetadataValue(value string) bool {
	return value != "" && len(value) <= 256 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\r\n")
}
func sortedRuleNames(values map[string]MetadataRule) []string {
	result := make([]string, 0, len(values))
	for name := range values {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

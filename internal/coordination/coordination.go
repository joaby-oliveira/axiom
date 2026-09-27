// Package coordination owns bounded structured child coordination records.
package coordination

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rgomids/axiom/internal/executiongraph"
)

const (
	FormatVersion    = 1
	MaxRecordBytes   = 64 << 10
	maxFields        = 32
	maxFieldBytes    = 4096
	maxStreamRecords = 64
)

var (
	ErrInvalidRecord     = errors.New("invalid coordination record")
	ErrStaleRevision     = errors.New("stale coordination revision")
	ErrCapacity          = errors.New("coordination capacity exhausted")
	sensitiveAssignment  = regexp.MustCompile(`(?i)\b(?:password|passwd|token|api[_-]?key|client[_-]?secret|secret|authorization)[ \t]*[:=][ \t]*["']?[^ \t"']{4,}`)
	bearerCredential     = regexp.MustCompile(`(?i)\bauthorization[ \t]*:[ \t]*bearer[ \t]+[a-z0-9._~+/=-]{8,}`)
	privateKeyCredential = regexp.MustCompile(`(?i)-----begin (?:[a-z0-9]+ )*private key-----`)
	knownCredential      = regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{20,}|(?:AKIA|ASIA)[A-Z0-9]{16}|sk-(?:proj-)?[A-Za-z0-9_-]{20,})\b`)
)

type Kind string

const (
	QuestionRequest      Kind = "question_request"
	Answer               Kind = "answer"
	ContractProposal     Kind = "contract_proposal"
	ContractAcceptance   Kind = "contract_acceptance"
	Blocker              Kind = "blocker"
	DependencyResolution Kind = "dependency_resolution"
	ArtifactPublication  Kind = "artifact_publication"
	Progress             Kind = "progress"
	Result               Kind = "result"
)

type UsageStatus string

const (
	UsageMeasured    UsageStatus = "measured"
	UsagePartial     UsageStatus = "partial"
	UsageUnavailable UsageStatus = "unavailable"
)

type UsageObservation struct {
	Source string      `json:"source"`
	Unit   string      `json:"unit"`
	Value  string      `json:"value,omitempty"`
	Status UsageStatus `json:"status"`
}

type Provenance struct {
	Product, Version, Revision, SourceState string
}

type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Record struct {
	FormatVersion  int               `json:"formatVersion"`
	RecordID       string            `json:"recordId"`
	Kind           Kind              `json:"kind"`
	ParentID       string            `json:"parentId"`
	ChildID        string            `json:"childId"`
	GraphRevision  uint64            `json:"graphRevision"`
	AttemptID      string            `json:"attemptId,omitempty"`
	Revision       uint64            `json:"revision"`
	PreviousDigest string            `json:"previousDigest,omitempty"`
	Provenance     Provenance        `json:"provenance"`
	Fields         []Field           `json:"fields"`
	Usage          *UsageObservation `json:"usage,omitempty"`
	CreatedAt      time.Time         `json:"createdAt"`
	Digest         string            `json:"digest"`
}

type Stream struct {
	FormatVersion int      `json:"formatVersion"`
	ParentID      string   `json:"parentId"`
	ChildID       string   `json:"childId"`
	Records       []Record `json:"records"`
}

type Input struct {
	Kind             Kind
	ParentID         string
	ChildID          string
	GraphRevision    uint64
	AttemptID        string
	ExpectedRevision uint64
	PreviousDigest   string
	Provenance       Provenance
	Fields           []Field
	Usage            *UsageObservation
}

type Store interface {
	Latest(context.Context, string, string) (Record, bool, error)
	Publish(context.Context, Record) error
}

type Service struct {
	store      Store
	allocateID func() (string, error)
	now        func() time.Time
}

func New(store Store, allocateID func() (string, error), now func() time.Time) Service {
	if allocateID == nil {
		allocateID = randomID
	}
	if now == nil {
		now = time.Now
	}
	return Service{store: store, allocateID: allocateID, now: now}
}

func (s Service) Publish(ctx context.Context, graph executiongraph.Graph, input Input) (Record, error) {
	if s.store == nil || !executiongraph.ValidGraph(graph) || !validInput(graph, input) {
		return Record{}, ErrInvalidRecord
	}
	latest, exists, err := s.store.Latest(ctx, input.ParentID, input.ChildID)
	if err != nil {
		return Record{}, err
	}
	expected := uint64(0)
	previousDigest := ""
	if exists {
		expected = latest.Revision
		previousDigest = latest.Digest
	}
	if input.ExpectedRevision != expected || input.PreviousDigest != previousDigest {
		return Record{}, ErrStaleRevision
	}
	if expected >= maxStreamRecords {
		return Record{}, ErrCapacity
	}
	recordID, err := s.allocateID()
	if err != nil || !validID(recordID) {
		return Record{}, ErrInvalidRecord
	}
	record := Record{
		FormatVersion: FormatVersion, RecordID: recordID, Kind: input.Kind, ParentID: input.ParentID, ChildID: input.ChildID,
		GraphRevision: input.GraphRevision, AttemptID: input.AttemptID, Revision: expected + 1, PreviousDigest: previousDigest,
		Provenance: input.Provenance, Fields: sortedFields(input.Fields), Usage: input.Usage, CreatedAt: s.now().UTC(),
	}
	digest, err := recordDigest(record)
	if err != nil {
		return Record{}, err
	}
	record.Digest = digest
	if !ValidRecord(record) {
		return Record{}, ErrInvalidRecord
	}
	if err := s.store.Publish(ctx, record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func ValidRecord(record Record) bool {
	if record.FormatVersion != FormatVersion || !validID(record.RecordID) || !validKind(record.Kind) || !validID(record.ParentID) || !validID(record.ChildID) || record.GraphRevision == 0 || record.Revision == 0 || record.Revision > maxStreamRecords || record.CreatedAt.IsZero() || !validDigest(record.Digest) || !validProvenance(record.Provenance) || len(record.Fields) == 0 || len(record.Fields) > maxFields {
		return false
	}
	if record.Revision == 1 && record.PreviousDigest != "" || record.Revision > 1 && !validDigest(record.PreviousDigest) {
		return false
	}
	if record.AttemptID != "" && !validID(record.AttemptID) {
		return false
	}
	allowed := allowedFields(record.Kind)
	seen := map[string]bool{}
	for _, field := range record.Fields {
		if !allowed[field.Name] || seen[field.Name] || !validValue(field.Value) {
			return false
		}
		seen[field.Name] = true
	}
	if record.Usage != nil && !validUsage(*record.Usage) {
		return false
	}
	digest, err := recordDigest(record)
	return err == nil && digest == record.Digest
}

// AcceptanceEvidence projects a digest-verified record into the execution
// graph acceptance boundary, carrying the correlation fields that let
// BuildEvidence derive a question/answer or proposal/acceptance exchange.
func AcceptanceEvidence(record Record) (executiongraph.CoordinationEvidence, error) {
	if !ValidRecord(record) {
		return executiongraph.CoordinationEvidence{}, ErrInvalidRecord
	}
	evidence := executiongraph.CoordinationEvidence{RecordID: record.RecordID, Kind: string(record.Kind), ParentID: record.ParentID, GraphRevision: record.GraphRevision, ChildID: record.ChildID, AttemptID: record.AttemptID, Digest: record.Digest}
	for _, field := range record.Fields {
		switch {
		case record.Kind == Answer && field.Name == "question_reference", record.Kind == ContractAcceptance && field.Name == "contract_reference":
			evidence.Reference = field.Value
		case record.Kind == ContractAcceptance && field.Name == "decision":
			evidence.Decision = field.Value
		}
	}
	return evidence, nil
}

func Encode(record Record) ([]byte, error) {
	if !ValidRecord(record) {
		return nil, ErrInvalidRecord
	}
	wire, err := json.Marshal(record)
	if err != nil || len(wire)+1 > MaxRecordBytes {
		return nil, ErrInvalidRecord
	}
	return append(wire, '\n'), nil
}

func ValidStream(stream Stream) bool {
	if stream.FormatVersion != FormatVersion || !validID(stream.ParentID) || !validID(stream.ChildID) || len(stream.Records) == 0 || len(stream.Records) > maxStreamRecords {
		return false
	}
	previous := ""
	for index, record := range stream.Records {
		if !ValidRecord(record) || record.ParentID != stream.ParentID || record.ChildID != stream.ChildID || record.Revision != uint64(index+1) || record.PreviousDigest != previous {
			return false
		}
		previous = record.Digest
	}
	return true
}

func EncodeStream(stream Stream) ([]byte, error) {
	if !ValidStream(stream) {
		return nil, ErrInvalidRecord
	}
	wire, err := json.Marshal(stream)
	if err != nil || len(wire)+1 > MaxRecordBytes*maxStreamRecords {
		return nil, ErrInvalidRecord
	}
	return append(wire, '\n'), nil
}

func DecodeStream(wire []byte) (Stream, error) {
	if len(wire) == 0 || len(wire) > MaxRecordBytes*maxStreamRecords {
		return Stream{}, ErrInvalidRecord
	}
	decoder := json.NewDecoder(strings.NewReader(string(wire)))
	decoder.DisallowUnknownFields()
	var stream Stream
	if err := decoder.Decode(&stream); err != nil {
		return Stream{}, ErrInvalidRecord
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) || !ValidStream(stream) {
		return Stream{}, ErrInvalidRecord
	}
	return stream, nil
}

func validInput(graph executiongraph.Graph, input Input) bool {
	if input.ParentID != graph.Parent.ExecutionID || input.GraphRevision != graph.Parent.GraphRevision || !validKind(input.Kind) || !validProvenance(input.Provenance) || len(input.Fields) == 0 || len(input.Fields) > maxFields {
		return false
	}
	childFound := false
	attemptFound := input.AttemptID == ""
	for _, child := range graph.Children {
		if child.ExecutionID != input.ChildID {
			continue
		}
		childFound = true
		for _, attempt := range child.Attempts {
			if attempt.AttemptID == input.AttemptID {
				attemptFound = true
			}
		}
	}
	probe := Record{Kind: input.Kind, Fields: input.Fields, Usage: input.Usage}
	allowed := allowedFields(probe.Kind)
	seen := map[string]bool{}
	for _, field := range probe.Fields {
		if !allowed[field.Name] || seen[field.Name] || !validValue(field.Value) {
			return false
		}
		seen[field.Name] = true
	}
	return childFound && attemptFound && (input.Usage == nil || validUsage(*input.Usage))
}

func allowedFields(kind Kind) map[string]bool {
	common := map[string]bool{"summary": true, "reference": true, "digest": true, "status": true}
	specific := map[Kind][]string{
		QuestionRequest: {"question", "response_contract"}, Answer: {"answer", "question_reference"},
		ContractProposal: {"contract", "scope"}, ContractAcceptance: {"contract_reference", "decision"},
		Blocker: {"code", "required_action"}, DependencyResolution: {"dependency", "resolution"},
		ArtifactPublication: {"artifact_id", "artifact_digest", "media_type"}, Progress: {"phase", "percent"},
		Result: {"outcome", "validation_reference"},
	}
	result := map[string]bool{}
	for key, value := range common {
		result[key] = value
	}
	for _, key := range specific[kind] {
		result[key] = true
	}
	return result
}

func validKind(kind Kind) bool {
	switch kind {
	case QuestionRequest, Answer, ContractProposal, ContractAcceptance, Blocker, DependencyResolution, ArtifactPublication, Progress, Result:
		return true
	default:
		return false
	}
}

func validUsage(usage UsageObservation) bool {
	if !validToken(usage.Source) {
		return false
	}
	switch usage.Status {
	case UsageUnavailable:
		return usage.Unit == "" && usage.Value == ""
	case UsageMeasured, UsagePartial:
		return validToken(usage.Unit) && usage.Value != "" && len(usage.Value) <= 128 && !strings.ContainsAny(usage.Value, "\x00\r\n")
	default:
		return false
	}
}

func validProvenance(value Provenance) bool {
	return validToken(value.Product) && validToken(value.Version) && validToken(value.Revision) && validToken(value.SourceState)
}

func validValue(value string) bool {
	if value == "" || len(value) > maxFieldBytes || strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	lower := strings.ToLower(value)
	if privateKeyCredential.MatchString(value) || strings.Contains(lower, "<|assistant|>") || strings.Contains(lower, "<|user|>") || strings.Contains(lower, "<|system|>") {
		return false
	}
	return !sensitiveAssignment.MatchString(value) && !bearerCredential.MatchString(value) && !knownCredential.MatchString(value)
}

func validToken(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for _, char := range value {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' || char == ':') {
			return false
		}
	}
	return true
}

func validID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
			continue
		}
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

func validDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func sortedFields(fields []Field) []Field {
	result := append([]Field(nil), fields...)
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func recordDigest(record Record) (string, error) {
	record.Digest = ""
	wire, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(wire)
	return hex.EncodeToString(digest[:]), nil
}

func randomID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

// SPDX-License-Identifier: Apache-2.0
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const MaxRowBytes = 32 << 20

// PrivateVisibility identifies explicitly owner-private retained rows. Cast's
// source schema admits it without extending legacy HTTP Create/Update validators.
// New private creations belong in the client's encrypted repository. Neither org
// nor people-with-zero-recipients is an owner-only promise.
const PrivateVisibility = "private"

var (
	ErrSnapshot   = errors.New("cast: invalid or incomplete retained catalog")
	ErrShared     = errors.New("cast: publication and recipient access require encrypted ACLs")
	ErrLive       = errors.New("cast: live signaling requires an authorized private migration")
	ErrDeviceWork = errors.New("cast: active media processing requires an authorized device worker")
)

type Row struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	JSON []byte `json:"json"`
}

type relation struct {
	ID         string     `json:"id"`
	Tenant     string     `json:"tenant"`
	Owner      string     `json:"owner_sub"`
	Video      string     `json:"video_id"`
	User       string     `json:"user_id"`
	Identity   string     `json:"identity"`
	Author     string     `json:"author_sub"`
	Reporter   string     `json:"reporter_sub"`
	Blocker    string     `json:"blocker_sub"`
	Blocked    string     `json:"blocked_sub"`
	ReplyTo    *string    `json:"reply_to"`
	Seq        int        `json:"seq"`
	Kind       Kind       `json:"kind"`
	Status     string     `json:"status"`
	Visibility Visibility `json:"visibility"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
}

type Snapshot struct {
	scope     storage.Scope
	rows      map[string]Row
	relations map[string]relation
}

func SourceKind(kind string) bool {
	switch kind {
	case "video", "person", "job", "comment", "chat", "stream_key", "viewer", "presentation", "slide_mark", "report", "person_block", "external_access", "external_comment", "external_report", "media":
		return true
	default:
		return false
	}
}

func canonicalUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

func compositeID(left, right string) (string, error) {
	if left == "" || right == "" || !utf8.ValidString(left) || !utf8.ValidString(right) || strings.ContainsRune(left+right, 0) {
		return "", ErrSnapshot
	}
	data := strconv.Itoa(len(left)) + ":" + left + strconv.Itoa(len(right)) + ":" + right
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:]), nil
}

func rowIdentity(kind string, value relation) (string, error) {
	switch kind {
	case "media":
		if !storage.Identifier(value.ID) {
			return "", ErrSnapshot
		}
		return value.ID, nil
	case "video", "job", "comment", "chat", "report", "external_comment", "external_report":
		if !canonicalUUID(value.ID) {
			return "", ErrSnapshot
		}
		return value.ID, nil
	case "presentation":
		if !canonicalUUID(value.Video) {
			return "", ErrSnapshot
		}
		return value.Video, nil
	case "person", "external_access":
		return compositeID(value.Video, value.User)
	case "viewer":
		return compositeID(value.Video, value.Identity)
	case "slide_mark":
		return compositeID(value.Video, strconv.Itoa(value.Seq))
	case "stream_key":
		return compositeID(value.Tenant, value.Owner)
	case "person_block":
		return compositeID(value.Blocker, value.Blocked)
	default:
		return "", ErrSnapshot
	}
}

// RetainedRow derives routing from original columns and keeps their exact JSON.
// Both SQL capture and devices use this function for composite identifiers.
func RetainedRow(kind string, raw []byte) (Row, error) {
	var value relation
	if !SourceKind(kind) || len(raw) < 2 || len(raw) > MaxRowBytes || json.Unmarshal(raw, &value) != nil {
		return Row{}, ErrSnapshot
	}
	id, err := rowIdentity(kind, value)
	if err != nil {
		return Row{}, err
	}
	return Row{Kind: kind, ID: id, JSON: slices.Clone(raw)}, nil
}

func externalKind(kind string) bool { return strings.HasPrefix(kind, "external_") }

func ValidateRow(scope storage.Scope, row Row) error {
	derived, err := RetainedRow(row.Kind, row.JSON)
	if err != nil || !scope.Valid() || scope.Product != "cast" || derived.ID != row.ID {
		return ErrSnapshot
	}
	var value relation
	if json.Unmarshal(row.JSON, &value) != nil {
		return ErrSnapshot
	}
	if externalKind(row.Kind) {
		return validateExternalRow(scope, row.Kind, value)
	}
	if row.Kind == "person_block" {
		if value.Blocker == value.Blocked || (value.Blocker != scope.Subject && value.Blocked != scope.Subject) {
			return ErrSnapshot
		}
		return nil
	}
	if value.Tenant != scope.Tenant {
		return ErrSnapshot
	}
	if row.Kind != "video" && row.Kind != "stream_key" && !canonicalUUID(value.Video) {
		return ErrSnapshot
	}
	if row.Kind == "media" {
		return validateMediaRow(row.JSON)
	}
	if row.Kind == "video" {
		// The retained relation intentionally ignores fields outside its graph.
		// Validate the complete playback projection as well, so malformed values
		// cannot be silently discarded when the inventory is traversed.
		var video VideoRow
		if json.Unmarshal(row.JSON, &video) != nil {
			return ErrSnapshot
		}
	}
	return validateContentRow(row.Kind, value)
}

func validateContentRow(kind string, value relation) error {
	switch kind {
	case "video":
		return validateVideoRow(value)
	case "stream_key":
		if value.Owner == "" {
			return ErrSnapshot
		}
	case "job":
		if !JobKind(value.Kind).Valida() || !validJobStatus(value.Status) {
			return ErrSnapshot
		}
	case "comment", "chat":
		if value.Author == "" || value.Body == "" {
			return ErrSnapshot
		}
	case "slide_mark":
		if value.Seq < 1 || value.Seq > 2000 {
			return ErrSnapshot
		}
	}
	return nil
}

func validateExternalRow(scope storage.Scope, kind string, value relation) error {
	if value.Tenant == "" || !canonicalUUID(value.Video) && kind != "external_report" {
		return ErrSnapshot
	}
	switch kind {
	case "external_access":
		if value.User != scope.Subject {
			return ErrSnapshot
		}
	case "external_comment":
		if value.Author != scope.Subject {
			return ErrSnapshot
		}
	case "external_report":
		if value.Reporter != scope.Subject {
			return ErrSnapshot
		}
	default:
		return ErrSnapshot
	}
	return nil
}

func validateVideoRow(value relation) error {
	if value.Owner == "" || !value.Kind.Valida() || (!value.Visibility.Valida() && value.Visibility != PrivateVisibility) || value.Title == "" {
		return ErrSnapshot
	}
	switch Status(value.Status) {
	case StatusUploading, StatusProcessing, StatusReady, StatusFailed, StatusScheduled, StatusLive, StatusEnded:
		return nil
	default:
		return ErrSnapshot
	}
}

func validJobStatus(value string) bool {
	return value == string(JobPending) || value == string(JobRunning) || value == string(JobDone) || value == string(JobFailed)
}

func NewSnapshot(scope storage.Scope, rows []Row) (*Snapshot, error) {
	if !scope.Valid() || scope.Product != "cast" {
		return nil, ErrSnapshot
	}
	s := &Snapshot{scope: scope, rows: map[string]Row{}, relations: map[string]relation{}}
	for _, row := range rows {
		if err := ValidateRow(scope, row); err != nil {
			return nil, err
		}
		key := row.Kind + "\x00" + row.ID
		if _, exists := s.rows[key]; exists {
			return nil, ErrSnapshot
		}
		row.JSON = slices.Clone(row.JSON)
		s.rows[key] = row
		var value relation
		_ = json.Unmarshal(row.JSON, &value)
		s.relations[key] = value
	}
	if err := s.validateGraph(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Snapshot) validateGraph() error {
	for key, value := range s.relations {
		row := s.rows[key]
		if row.Kind == "video" || row.Kind == "stream_key" || row.Kind == "person_block" || externalKind(row.Kind) {
			continue
		}
		if value.Video != "" {
			if _, exists := s.rows["video\x00"+value.Video]; !exists {
				return ErrSnapshot
			}
		}
		if row.Kind == "media" && value.Owner != s.relations["video\x00"+value.Video].Owner {
			return ErrSnapshot
		}
		if row.Kind == "comment" && value.ReplyTo != nil {
			parent, exists := s.relations["comment\x00"+*value.ReplyTo]
			if !exists || parent.Video != value.Video || parent.ReplyTo != nil {
				return ErrSnapshot
			}
		}
	}
	return nil
}

// PersonalCompatible never reinterprets a previously published video as a
// private one. It is a preflight check to run before any retained-source fence.
func (s *Snapshot) PersonalCompatible() error {
	var shared, live, work bool
	for key, value := range s.relations {
		err := s.rowPolicy(s.rows[key].Kind, value)
		switch {
		case errors.Is(err, ErrShared):
			shared = true
		case errors.Is(err, ErrLive):
			live = true
		case errors.Is(err, ErrDeviceWork):
			work = true
		}
	}
	if shared {
		return ErrShared
	}
	if live {
		return ErrLive
	}
	if work {
		return ErrDeviceWork
	}
	return nil
}

func (s *Snapshot) rowPolicy(kind string, value relation) error {
	if externalKind(kind) {
		return ErrShared
	}
	switch kind {
	case "person", "viewer":
		return ErrShared
	case "video":
		return s.videoPolicy(value)
	case "stream_key", "presentation":
		return ErrLive
	case "job":
		if value.Status == string(JobPending) || value.Status == string(JobRunning) {
			return ErrDeviceWork
		}
	case "comment", "chat":
		if value.Author != s.scope.Subject {
			return ErrShared
		}
	case "person_block":
		if value.Blocker != s.scope.Subject {
			return ErrShared
		}
	}
	return nil
}

func (s *Snapshot) videoPolicy(value relation) error {
	if value.Owner != s.scope.Subject || value.Visibility != PrivateVisibility {
		return ErrShared
	}
	if value.Kind == KindLive && value.Status != string(StatusReady) {
		return ErrLive
	}
	if value.Status == string(StatusUploading) || value.Status == string(StatusProcessing) {
		return ErrDeviceWork
	}
	return nil
}

func (s *Snapshot) Rows() []Row {
	result := make([]Row, 0, len(s.rows))
	for _, row := range s.rows {
		row.JSON = slices.Clone(row.JSON)
		result = append(result, row)
	}
	slices.SortFunc(result, func(a, b Row) int {
		if compared := strings.Compare(a.Kind, b.Kind); compared != 0 {
			return compared
		}
		return strings.Compare(a.ID, b.ID)
	})
	return result
}

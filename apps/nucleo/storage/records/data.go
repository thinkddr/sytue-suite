// SPDX-License-Identifier: Apache-2.0
package records

import (
	"context"
	"encoding/json"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// ReadRaw retains the complete stored JSON, including unknown domain fields.
func (r *Repository) ReadRaw(parent context.Context, kind, id string) ([]byte, error) {
	ctx, done, err := r.Start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	version, err := r.ReadyVersion(kind, id)
	if err != nil {
		return nil, err
	}
	return r.readJSON(ctx, version)
}

func (r *Repository) Selected(kind string) ([]storage.ObjectVersion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, ErrClosed
	}
	if !r.ready {
		return nil, ErrNotReady
	}
	var out []storage.ObjectVersion
	for _, heads := range r.heads {
		for _, version := range heads {
			var header struct{ Kind, Entity string }
			if json.Unmarshal(version.Commit.Metadata, &header) != nil {
				return nil, ErrSource
			}
			if header.Kind == r.cfg.ReceiptKind {
				continue
			}
			if header.Kind != r.cfg.EntityKind {
				return nil, ErrSource
			}
			if header.Entity != kind {
				continue
			}
			if len(heads) != 1 {
				return nil, ErrConflict
			}
			out = append(out, cloneVersion(version))
		}
	}
	slices.SortFunc(out, func(a, b storage.ObjectVersion) int {
		if a.ObjectID < b.ObjectID {
			return -1
		}
		if a.ObjectID > b.ObjectID {
			return 1
		}
		return 0
	})
	return out, nil
}

type mutation struct {
	Version      int                        `json:"v"`
	Scope        storage.Scope              `json:"scope"`
	Operation    string                     `json:"operation"`
	Input        string                     `json:"input"`
	Entity       string                     `json:"entity"`
	ID           string                     `json:"id"`
	State        string                     `json:"state"`
	JSON         []byte                     `json:"json"`
	Content      bool                       `json:"content,omitempty"`
	Prepared     bool                       `json:"prepared,omitempty"`
	Target       *storage.PublicationTarget `json:"target,omitempty"`
	ContentInput []byte                     `json:"content_input,omitempty"`
	Accepted     *storage.Reference         `json:"accepted,omitempty"`
	Acknowledge  bool                       `json:"acknowledge,omitempty"`
	Observed     *mutationObservation       `json:"observed,omitempty"`
}

// mutationObservation pins the exact heads and target that prepare observed
// before the WAL was written. A retry before the publication plan exists must
// publish against these, never against whatever head or device is current:
// adopting a newer head would make stale intent a child of a later writer.
type mutationObservation struct {
	Target  storage.PublicationTarget `json:"target"`
	Parents []storage.Reference       `json:"parents"`
}

func (r *Repository) Mutate(parent context.Context, operation string, input any, prepare func(context.Context) (string, string, string, []byte, error)) ([]byte, error) {
	return r.mutate(parent, operation, input, prepare, MutationContent{}, nil)
}

func (r *Repository) validateMutation(ctx context.Context, work mutation, operation, fingerprint string) error {
	if work.Prepared && !work.Content || work.Version != 1 || work.Scope != r.cfg.Scope || work.Operation != operation || work.Input != fingerprint || len(work.JSON) > MaxEntityJSON || !json.Valid(work.JSON) {
		return ErrSource
	}
	if err := validateMutationContentIntent(work); err != nil {
		return err
	}
	if err := validateMutationObservation(work); err != nil {
		return err
	}
	if err := r.cfg.ValidateRow(ctx, work.Entity, work.ID, work.JSON); err != nil {
		return err
	}
	state, err := rawState(work.JSON)
	if err != nil || state != work.State {
		return ErrSource
	}
	return nil
}

func validateMutationObservation(work mutation) error {
	if work.Observed == nil {
		return nil
	}
	if !work.Observed.Target.Valid() || (work.Content || work.Acknowledge) && (work.Target == nil || *work.Target != work.Observed.Target) {
		return ErrSource
	}
	for _, parent := range work.Observed.Parents {
		if !parent.Valid() {
			return ErrSource
		}
	}
	return nil
}

func validateMutationContentIntent(work mutation) error {
	if work.Content || work.Acknowledge {
		if work.Content && work.Acknowledge || work.Target == nil || !work.Target.Valid() {
			return ErrSource
		}
		if work.Content && (len(work.ContentInput) > MaxEntityJSON || !json.Valid(work.ContentInput)) || !work.Content && len(work.ContentInput) != 0 {
			return ErrSource
		}
		if work.Accepted != nil && (work.Content && !work.Prepared || !work.Accepted.Valid() || work.Accepted.RoomID != work.Target.RoomID) {
			return ErrSource
		}
	} else if work.Target != nil || len(work.ContentInput) != 0 || work.Accepted != nil {
		return ErrSource
	}
	return nil
}

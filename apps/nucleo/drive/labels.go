// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var ErrInvalidLabel = errors.New("drive: label is not in this account's taxonomy")

// Taxonomy definitions are organization policy. The file's assignment and policy
// snapshot stay inside the private catalog; no legacy file write is performed.
func (r *Repository) SensitivityLabels(parent context.Context) ([]api.EtiquetaDrive, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	if err := r.requireReady(); err != nil {
		return nil, err
	}
	return r.readLabels(ctx)
}

func (r *Repository) readLabels(ctx context.Context) ([]api.EtiquetaDrive, error) {
	labels, err := r.cfg.Gateway.SensitivityLabelsForScope(ctx, r.cfg.Scope)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, label := range labels {
		if !storage.Identifier(label.ID) || strings.TrimSpace(label.Name) == "" || label.Rank < 0 || seen[label.ID] {
			return nil, ErrSource
		}
		seen[label.ID] = true
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return labels, nil
}

func (r *Repository) SetLabel(ctx context.Context, file, label string) error {
	label = strings.TrimSpace(label)
	if label != "" && !storage.Identifier(label) {
		return ErrInvalidLabel
	}
	return r.mutateMetadata(ctx, metadataRequest{Operation: "label", Kind: api.DriveFile, ID: file, Value: label})
}

type labelState struct {
	ID     string          `json:"label_id"`
	Name   string          `json:"label_name"`
	Rank   int             `json:"label_rank"`
	Block  bool            `json:"label_block_external_share"`
	Nested *api.DriveLabel `json:"label"`
}

func (s labelState) patch() map[string]any {
	return map[string]any{"label_id": s.ID, "label_name": s.Name, "label_rank": s.Rank, "label_block_external_share": s.Block, "label": nil}
}

func (s labelState) same(other labelState) bool {
	return s.ID == other.ID && s.Name == other.Name && s.Rank == other.Rank && s.Block == other.Block && s.Nested == nil
}

func (r *Repository) changedLabel(ctx context.Context, item summary, raw []byte, id string) ([]byte, error) {
	if item.SourceKind != "file" {
		return nil, ErrSource
	}
	if err := r.requireLive(ctx, item); err != nil {
		return nil, err
	}
	if _, err := fileView(raw, item.SourceID); err != nil {
		return nil, err
	}
	var current labelState
	if json.Unmarshal(raw, &current) != nil {
		return nil, ErrSource
	}
	next := labelState{}
	if id != "" {
		labels, err := r.readLabels(ctx)
		if err != nil {
			return nil, err
		}
		for _, label := range labels {
			if label.ID == id {
				next = labelState{ID: id, Name: label.Name, Rank: label.Rank, Block: label.BlockExternalShare}
				break
			}
		}
		if next.ID == "" {
			return nil, ErrInvalidLabel
		}
	}
	if current.same(next) {
		return nil, nil
	}
	// Classification does not change the content's modified time or version.
	return patchRaw(raw, next.patch())
}

// A durable retry retains its original resolved policy even if an admin changes
// the taxonomy later. Only the captured label fields may differ from its base.
func checkLabelTransition(original, updated []byte, id string) error {
	var next labelState
	if json.Unmarshal(updated, &next) != nil || next.ID != id || next.Nested != nil || next.Rank < 0 {
		return ErrSource
	}
	if id == "" {
		if !next.same(labelState{}) {
			return ErrSource
		}
	} else if strings.TrimSpace(next.Name) == "" {
		return ErrSource
	}
	expected, err := patchRaw(original, next.patch())
	if err != nil || !bytes.Equal(expected, updated) {
		return ErrSource
	}
	return nil
}

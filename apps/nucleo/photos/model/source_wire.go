// SPDX-License-Identifier: Apache-2.0
package model

import "git.sytue.eu/sytue/one/apps/nucleo/storage"

// SourcePlanReference is the only device-plan value retained by the server.
// The authenticated encrypted link resolves the complete private plan through
// the maintained SDK; this reference grants no access and contains no pin/key.
type SourcePlanReference struct {
	Epoch string            `json:"epoch"`
	Link  storage.Reference `json:"link"`
}

func (r SourcePlanReference) Valid() bool { return storage.MigrationEpoch(r.Epoch) && r.Link.Valid() }

type SourcePlanRouting struct {
	Scope storage.Scope        `json:"scope"`
	Plan  *SourcePlanReference `json:"plan,omitempty"`
}

func (r SourcePlanRouting) Validate(scope storage.Scope) error {
	if !scope.Valid() || scope.Product != "photos" || r.Scope != scope || r.Plan != nil && !r.Plan.Valid() {
		return ErrSnapshot
	}
	return nil
}

// SourcePreflight describes only existing legacy JSON and retained variants.
// Originals remain separate authorized Drive pins on the device. Raw SQL JSON
// is base64 on the wire so unknown integers and fields cannot be normalized.
type SourcePreflight struct {
	Scope        storage.Scope             `json:"scope"`
	Epoch        string                    `json:"epoch"`
	CAS          string                    `json:"cas"`
	Rows         []Row                     `json:"rows"`
	Backend      []storage.StreamSourceRow `json:"backend"`
	BackendBlobs int64                     `json:"backend_blobs"`
}

func (p SourcePreflight) Validate(scope storage.Scope, epoch string) error {
	if !scope.Valid() || scope.Product != "photos" || p.Scope != scope || !storage.MigrationEpoch(epoch) || p.Epoch != epoch || p.CAS == "" || len(p.CAS) > 4096 || p.BackendBlobs < 0 || p.BackendBlobs > int64(len(p.Backend)) || len(p.Rows) != len(p.Backend)-int(p.BackendBlobs) {
		return ErrSnapshot
	}
	for _, row := range p.Backend {
		if row.Validate() != nil {
			return ErrSnapshot
		}
	}
	return nil
}

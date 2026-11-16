// SPDX-License-Identifier: Apache-2.0
// Package webport is the bounded host IO contract for the shared Go/Wasm core.
// Hosts own SDK trust, encrypted persistence and network credentials. A port is
// an ephemeral capability tied to one immutable account/product and lifetime.
package webport

import (
	"context"
	"errors"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var ErrRejected = errors.New("webport: scoped host operation rejected")

const (
	Version    = 1
	ChunkBytes = 256 << 10
)

type Method string

const (
	Current               Method = "current"
	IdentityRoot          Method = "identity.root"
	LeaseAcquire          Method = "lease.acquire"
	LeaseRelease          Method = "lease.release"
	Authorize             Method = "authority.revalidate"
	JournalRead           Method = "journal.read"
	JournalWrite          Method = "journal.write"
	JournalDelete         Method = "journal.delete"
	JournalKeys           Method = "journal.keys"
	JournalLock           Method = "journal.lock"
	JournalUnlock         Method = "journal.unlock"
	StoragePrepare        Method = "storage.prepare"
	StorageBindings       Method = "storage.bindings"
	StorageRecoverable    Method = "storage.recoverable"
	StorageFrozen         Method = "storage.frozen"
	StorageTarget         Method = "storage.target"
	StorageReadRecord     Method = "storage.record.read"
	StoragePutRecord      Method = "storage.record.put"
	StorageReadObject     Method = "storage.object.read"
	StoragePutObject      Method = "storage.object.put"
	StorageHistory        Method = "storage.history"
	StorageFileInfo       Method = "storage.file.info"
	StorageFileCommitment Method = "storage.file.commitment"
	StorageFileDownload   Method = "storage.file.download"
	StorageFileRead       Method = "storage.file.read"
	StorageFileClose      Method = "storage.file.close"
	StorageFileUpload     Method = "storage.file.upload"
	StorageFileAck        Method = "storage.file.ack"
	StorageFileCopy       Method = "storage.file.copy"
	IngressSecretRead     Method = "ingress.secret.read"
	IngressSecretWrite    Method = "ingress.secret.write"
	IngressActionSign     Method = "ingress.action.sign"
	ServiceRequest        Method = "service.request"
	ServiceRead           Method = "service.read"
	ServiceClose          Method = "service.close"
	SourceCapabilities    Method = "source.capabilities"
	SourceBegin           Method = "source.begin"
	SourcePage            Method = "source.page"
	SourceOpen            Method = "source.open"
	SourceRead            Method = "source.read"
	SourceClose           Method = "source.close"
	CutoverState          Method = "cutover.state"
	CutoverBegin          Method = "cutover.begin"
	CutoverComplete       Method = "cutover.complete"
	SubmissionRegister    Method = "submission.register"
	SubmissionControl     Method = "submission.control"
	SubmissionReceipt     Method = "submission.receipt"
	SubmissionSend        Method = "submission.send"
)

type Bounds struct {
	Request  int `json:"request"`
	Response int `json:"response"`
}

var bounds = map[Method]Bounds{
	Current: {0, 64 << 10}, IdentityRoot: {0, 4096}, LeaseAcquire: {4096, 4096}, LeaseRelease: {4096, 4096}, Authorize: {4096, 4096},
	JournalRead: {8192, (8 << 20) + 1}, JournalWrite: {(8 << 20) + 8192, 4096}, JournalDelete: {8192, 4096}, JournalKeys: {8192, 1 << 20}, JournalLock: {8192, 4096}, JournalUnlock: {8192, 4096},
	StorageFrozen: {0, 256 << 10}, StoragePrepare: {8192, 64 << 10}, StorageBindings: {0, 64 << 10}, StorageRecoverable: {8192, 4096}, StorageTarget: {8192, 4096},
	StorageReadRecord: {64 << 10, 1 << 20}, StoragePutRecord: {1 << 20, 64 << 10}, StorageReadObject: {64 << 10, 1 << 20}, StoragePutObject: {1 << 20, 1 << 20}, StorageHistory: {64 << 10, 8 << 20},
	StorageFileInfo: {64 << 10, 64 << 10}, StorageFileCommitment: {64 << 10, 64 << 10}, StorageFileDownload: {64 << 10, 64 << 10}, StorageFileRead: {64 << 10, ChunkBytes}, StorageFileClose: {8192, 4096},
	StorageFileUpload: {64 << 10, 64 << 10}, StorageFileAck: {64 << 10, 4096}, StorageFileCopy: {64 << 10, 64 << 10},
	IngressActionSign: {16 << 10, 16 << 10}, IngressSecretRead: {8192, 2049}, IngressSecretWrite: {8192, 4096},
	ServiceRequest: {320 << 10, 64 << 10}, ServiceRead: {8192, ChunkBytes}, ServiceClose: {8192, 4096},
	SourceCapabilities: {8192, 64 << 10}, SourceBegin: {8192, 64 << 10}, SourcePage: {64 << 10, 1 << 20}, SourceOpen: {64 << 10, 64 << 10}, SourceRead: {64 << 10, ChunkBytes}, SourceClose: {8192, 4096},
	CutoverState: {8192, 64 << 10}, CutoverBegin: {8192, 64 << 10}, CutoverComplete: {64 << 10, 64 << 10},
	SubmissionRegister: {64 << 10, 64 << 10}, SubmissionControl: {8192, 64 << 10}, SubmissionReceipt: {8192, 64 << 10}, SubmissionSend: {64 << 10, 64 << 10},
}

func Limits(method Method) (Bounds, bool) { value, ok := bounds[method]; return value, ok }
func ValidID(id string) bool {
	if len(id) != 64 {
		return false
	}
	return strings.Trim(id, "0123456789abcdef") == ""
}

type Binding struct {
	MatrixUserID string `json:"matrix_user_id"`
	DeviceID     string `json:"device_id"`
}

type Identity struct {
	Version int           `json:"v"`
	Binding Binding       `json:"binding"`
	Scope   storage.Scope `json:"scope"`
}

func (i Identity) Validate() error {
	if i.Version != Version || !i.Scope.Valid() || !strings.HasPrefix(i.Binding.MatrixUserID, "@") || !strings.Contains(i.Binding.MatrixUserID, ":") || len(i.Binding.MatrixUserID) > 2048 || strings.ContainsAny(i.Binding.MatrixUserID, "\x00\r\n") || !storage.Identifier(i.Binding.DeviceID) {
		return ErrRejected
	}
	return nil
}

// Call never accepts a URL, token, arbitrary function name or implicit scope.
// Payloads are owned bytes; large originals use bounded streams, never one JSON
// string. A host must revalidate the lease before and after each actual IO and
// drain active work before key erasure. IO concurrency is explicit, not FIFO.
type Port interface {
	ID() string
	Current(context.Context) (Identity, error)
	Call(context.Context, storage.Scope, Method, []byte) ([]byte, error)
}

func Contract() map[string]Bounds {
	out := make(map[string]Bounds, len(bounds))
	for method, limit := range bounds {
		out[string(method)] = limit
	}
	return out
}

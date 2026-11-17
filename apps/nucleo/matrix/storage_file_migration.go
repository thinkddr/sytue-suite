// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"io"
	"slices"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// MigrateStorageFileContent copies an immutable application body from the old
// account into its isolated identity. Both devices must be verified and have
// their own owner key. The maintained uploader creates fresh ciphertext and
// checks its entire replacement; neither identity's recovery keys are shared.
//
// The caller retains the exact source receipt and job in its encrypted catalog
// migration intent. It must migrate ALL catalog revisions and their parent links
// before cutover. This method never deletes originals or acknowledges the upload
// journal: AcknowledgeStorageFile still requires its new catalog commit.
// File-manifest parents are rejected: Drive body history belongs to the catalog,
// and copying unknown links unchanged could silently depend on the old account.
func (c *Client) MigrateStorageFileContent(ctx context.Context, destination *Client, gateway *api.Drive, journal *offline.Store, job string, ref EventRef, object, revision string) (EventRef, error) {
	if _, err := c.fileMigrationScope(destination); err != nil {
		return EventRef{}, err
	}
	target, err := destination.StoragePublicationTarget()
	if err != nil {
		return EventRef{}, err
	}
	return c.MigrateStorageFileContentAt(ctx, destination, gateway, journal, job, target, ref, object, revision)
}

// MigrateStorageFileContentAt keeps a catalog migration's original destination
// even if its default room rotates before the first encrypted file publication.
func (c *Client) MigrateStorageFileContentAt(ctx context.Context, destination *Client, gateway *api.Drive, journal *offline.Store, job string, publication storage.PublicationTarget, ref EventRef, object, revision string) (EventRef, error) {
	scope, err := c.fileMigrationScope(destination)
	if err != nil {
		return EventRef{}, err
	}
	if err := destination.CheckStoragePublicationTarget(publication); err != nil {
		return EventRef{}, err
	}
	if _, err := c.PrepareStorage(ctx, scope); err != nil {
		return EventRef{}, err
	}
	if _, err := destination.PrepareStorage(ctx, scope); err != nil {
		return EventRef{}, err
	}
	source := storageFileIO{scope: scope, gateway: gateway, records: c, closed: c.closed.Load}
	records := storageFileTarget{storageRecordIO: destination, storagePublicationIO: destination, target: publication}
	target := storageFileIO{scope: scope, gateway: gateway, records: records, journal: journal, closed: func() bool { return c.closed.Load() || destination.closed.Load() }}
	return target.migrateContent(ctx, source, job, ref, object, revision)
}

func (c *Client) fileMigrationScope(destination *Client) (storage.Scope, error) {
	if c == nil || destination == nil || c == destination {
		return storage.Scope{}, ErrIdentity
	}
	c.stateMu.RLock()
	source, scope := c.cfg.Bootstrap, c.storageScope()
	c.stateMu.RUnlock()
	destination.stateMu.RLock()
	target, next := destination.cfg.Bootstrap, destination.storageScope()
	destination.stateMu.RUnlock()
	if !scope.Valid() || scope != next || source.StorageIdentityVersion.Normalized() != storage.LegacyIdentity || target.StorageIdentityVersion != storage.IsolatedIdentity || source.HomeserverURL != target.HomeserverURL || source.MatrixUserID == target.MatrixUserID || !validStorageBindings(source) || !validStorageBindings(target) {
		return storage.Scope{}, ErrIdentity
	}
	return scope, nil
}

func (f storageFileIO) migrateContent(ctx context.Context, source storageFileIO, job string, ref EventRef, object, revision string) (EventRef, error) {
	if f.scope != source.scope || !ref.Valid() {
		return EventRef{}, ErrIdentity
	}
	file, err := source.root(ctx, ref, object, revision)
	if err != nil {
		return EventRef{}, err
	}
	if len(file.Request.Previous) != 0 {
		return EventRef{}, storage.ErrRecord
	}
	reader := &storageFileReaderAt{ctx: ctx, source: source, reference: ref, request: file.Request, part: -1}
	defer reader.Close()
	request := file.Request
	request.Metadata = slices.Clone(request.Metadata)
	request.Previous = nil
	f.expectedSourceSHA256 = file.SHA256
	return f.upload(ctx, job, request, reader)
}

// Only one authenticated block is retained in memory. The existing uploader
// performs its full source hash, durable checkpoints and replacement verification;
// this adapter supplies random access without a plaintext temporary file.
type storageFileReaderAt struct {
	mu        sync.Mutex
	ctx       context.Context
	source    storageFileIO
	reference EventRef
	request   storage.FileRequest
	part      int
	block     []byte
	closed    bool
}

func (r *storageFileReaderAt) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	clear(r.block)
	r.block = nil
}

func (r *storageFileReaderAt) ReadAt(out []byte, offset int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, ErrClosed
	}
	if err := r.source.guard(r.ctx); err != nil {
		clear(r.block)
		r.block = nil
		r.part = -1
		return 0, err
	}
	if offset < 0 {
		return 0, storage.ErrRecord
	}
	if len(out) == 0 {
		return 0, nil
	}
	if offset >= r.request.SizeBytes {
		return 0, io.EOF
	}
	count := 0
	for count < len(out) && offset < r.request.SizeBytes {
		part := int(offset / storage.MaxCipherBlockBytes)
		if part != r.part {
			clear(r.block)
			r.block = nil
			r.part = -1
			data, err := r.source.readRange(r.ctx, r.reference, r.request.ObjectID, r.request.RevisionID, int64(part)*storage.MaxCipherBlockBytes, fileBlockBytes(r.request.SizeBytes, part))
			if err != nil {
				return count, err
			}
			r.block, r.part = data, part
		}
		begin := int(offset % storage.MaxCipherBlockBytes)
		copied := copy(out[count:], r.block[begin:])
		if copied == 0 {
			return count, storage.ErrRecord
		}
		count += copied
		offset += int64(copied)
	}
	if err := r.source.guard(r.ctx); err != nil {
		clear(out[:count])
		clear(r.block)
		r.block = nil
		r.part = -1
		return 0, err
	}
	if count != len(out) {
		return count, io.EOF
	}
	return count, nil
}

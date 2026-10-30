// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package matrix

import (
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func (f storageFileIO) leafAt(ctx context.Context, file storageFile, index int, cache map[EventRef]storageFilePage) (storageFileBlock, error) {
	page, level, position := file.Root, file.Root.Level, 0
	for level > 0 {
		span := filePageSpan(level - 1)
		child := (index - position*filePageSpan(level)) / span
		if child < 0 || child >= len(page.Nodes) || page.Nodes[child].Event == nil {
			return storageFileBlock{}, storage.ErrRecord
		}
		ref := *page.Nodes[child].Event
		next, ok := cache[ref]
		if !ok {
			rec, err := f.readRecord(ctx, ref, file.Request)
			if err != nil {
				return storageFileBlock{}, err
			}
			if json.Unmarshal(rec.Payload, &next) != nil {
				return storageFileBlock{}, storage.ErrRecord
			}
			cache[ref] = next
		}
		level--
		position = position*fileFanout + child
		if err := validateFilePage(next, file.Request, level, position); err != nil {
			return storageFileBlock{}, err
		}
		page = next
	}
	slot := index % fileFanout
	if slot >= len(page.Nodes) || page.Nodes[slot].Block == nil {
		return storageFileBlock{}, storage.ErrRecord
	}
	return *page.Nodes[slot].Block, nil
}

func (f storageFileIO) readRange(ctx context.Context, ref EventRef, object, revision string, offset, length int64) ([]byte, error) {
	if f.gateway == nil || f.records == nil || offset < 0 || length < 0 || length > storage.MaxCipherBlockBytes {
		return nil, storage.ErrRecord
	}
	file, err := f.root(ctx, ref, object, revision)
	if err != nil {
		return nil, err
	}
	if offset > file.Request.SizeBytes || length > file.Request.SizeBytes-offset {
		return nil, storage.ErrRecord
	}
	out := make([]byte, 0, int(length))
	cache := map[EventRef]storageFilePage{}
	end := offset + length
	for cursor := offset; cursor < end; {
		index := int(cursor / storage.MaxCipherBlockBytes)
		descriptor, err := f.leafAt(ctx, file, index, cache)
		block := descriptor.at(f.scope, object, revision, index)
		if err != nil {
			clear(out)
			return nil, err
		}
		if block.Blob.SizeBytes != fileBlockBytes(file.Request.SizeBytes, index) {
			clear(out)
			return nil, storage.ErrRecord
		}
		data, err := downloadStorageBlock(ctx, f.gateway, f.scope, block, object, revision, index)
		if err != nil {
			clear(out)
			return nil, err
		}
		if digestBytes(data) != descriptor.PlainSHA256 {
			clear(data)
			clear(out)
			return nil, ErrIdentity
		}
		if err := f.guard(ctx); err != nil {
			clear(data)
			clear(out)
			return nil, err
		}
		start := cursor - int64(index)*storage.MaxCipherBlockBytes
		count := min(int64(len(data))-start, end-cursor)
		if count <= 0 {
			clear(data)
			clear(out)
			return nil, storage.ErrRecord
		}
		out = append(out, data[start:start+count]...)
		clear(data)
		cursor += count
	}
	if err := f.guard(ctx); err != nil {
		clear(out)
		return nil, err
	}
	return out, nil
}

// ReadStorageFileRange supports media seeking and OS file providers without
// downloading an entire file. The authenticated manifest binds every selected
// block to its revision and position. A range is bounded to one block of output;
// the whole-file digest is checked by DownloadStorageFile, not by a range read.
func (c *Client) ReadStorageFileRange(ctx context.Context, gateway *api.Drive, ref EventRef, object, revision string, offset, length int64) ([]byte, error) {
	scope, err := c.storageBlockScope(ctx)
	if err != nil {
		return nil, err
	}
	return (storageFileIO{scope: scope, gateway: gateway, records: c, closed: c.closed.Load}).readRange(ctx, ref, object, revision, offset, length)
}

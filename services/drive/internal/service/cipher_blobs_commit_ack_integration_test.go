//go:build integration

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

type loseCipherCommit struct {
	cipherBlobStore
	proxy *pgtest.CommitAckProxy
}

func (s *loseCipherCommit) CreateCipherBlob(ctx context.Context, blob domain.CipherBlob) (domain.CipherBlob, error) {
	s.proxy.Arm()
	return s.cipherBlobStore.CreateCipherBlob(ctx, blob)
}

func TestCipherBlobRecoversAfterRealLostCommitAcknowledgement(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	proxy := pgtest.NewCommitAckProxy(t, h.AppDSN)
	pool, err := pg.Connect(t.Context(), pg.Config{DSN: proxy.DSN(), DirectDSN: proxy.DSN()})
	if err != nil {
		t.Fatal("connect through COMMIT proxy", err)
	}
	t.Cleanup(pool.Close)

	realStore := store.New(h.Pool)
	blobs := &fakeBlobs{}
	s := newSvc(&fakeStore{}, blobs, nil)
	s.store = realStore
	s.cipher = &loseCipherCommit{cipherBlobStore: store.New(pool), proxy: proxy}
	ctx, cancel := context.WithTimeout(store.WithOwner(tenantCtx(t), "alice"), time.Minute)
	defer cancel()
	id := strings.Repeat("A", 26)

	if _, err := s.PutCipherBlob(ctx, "mail", id, 3, strings.NewReader("abc")); err == nil {
		t.Fatal("lost COMMIT acknowledgement was not reported")
	}
	if err := proxy.WaitDropped(ctx); err != nil {
		t.Fatal("proxy did not observe PostgreSQL commit", err)
	}
	row, err := realStore.CipherBlob(ctx, "mail", id)
	if err != nil || len(blobs.delKeys) != 0 || string(blobs.putBody) != "abc" {
		t.Fatal("committed ciphertext was lost after the acknowledgement", err)
	}
	claimed, err := realStore.ReclamarSubidasCaducadas(ctx, time.Now().Add(10*time.Minute), 20)
	if err != nil || len(claimed) != 0 {
		t.Fatal("committed upload reservation remained available to the collector", claimed, err)
	}
	s.cipher = realStore
	blobs.getBody = "abc"
	if recovered, err := s.PutCipherBlob(ctx, "mail", id, 3, strings.NewReader("abc")); err != nil || recovered.StorageKey != row.StorageKey || len(blobs.delKeys) != 0 {
		t.Fatal("retry failed to recover the committed receipt", err)
	}
}

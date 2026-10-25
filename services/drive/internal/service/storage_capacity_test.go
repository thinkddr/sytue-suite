// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

type capacityBlobs struct {
	*fakeBlobs
	capacity objectstore.Capacity
	err      error
}

func (b capacityBlobs) Capacity(context.Context) (objectstore.Capacity, error) {
	return b.capacity, b.err
}

type quotaLimitStore struct {
	*fakeStore
	received int64
}

func (s *quotaLimitStore) SetQuotaLimit(_ context.Context, limit int64) (domain.Quota, error) {
	s.received = limit
	q := s.quota
	q.LimitBytes = limit
	return q, nil
}

func TestQuotaConectadaEsUnaAmpliacionYSeRetiraAlDesconectar(t *testing.T) {
	total, used, available := int64(1500), int64(600), int64(900)
	st := &fakeStore{quota: domain.Quota{LimitBytes: 500, UsedBytes: 200}}
	s := newSvc(st, &fakeBlobs{}, nil)
	s.router = newBlobRouter(&fakeBlobs{}, &fakeExternal{}, "", nil)
	s.router.cache["acme"] = entradaExterna{
		blobs: capacityBlobs{fakeBlobs: &fakeBlobs{}, capacity: objectstore.Capacity{
			TotalBytes: &total, UsedBytes: &used, AvailableBytes: &available,
		}},
		expires: time.Now().Add(time.Hour),
	}
	ctx := tenantCtx(t)
	for range 2 {
		q, err := s.Quota(ctx)
		if err != nil || !q.ConnectedStorage || q.LimitBytes != 2000 || q.UsedBytes != 800 || q.Available() != 1200 {
			t.Fatalf("cuota ampliada: %+v, %v", q, err)
		}
	}
	if st.quota.LimitBytes != 500 || st.quota.UsedBytes != 200 {
		t.Fatalf("la capacidad remota no puede alterar lo facturado: %+v", st.quota)
	}
	// Otro tenant nunca hereda la capacidad del cliente cacheado para acme.
	if q := s.quotaWithStorage(context.Background(), st.quota); q.ConnectedStorage || q.LimitBytes != 500 {
		t.Fatalf("sin tenant no hay ampliación: %+v", q)
	}
	beta := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "beta"})
	if q := s.quotaWithStorage(beta, st.quota); q.ConnectedStorage || q.LimitBytes != 500 {
		t.Fatalf("otro tenant no hereda el almacenamiento de acme: %+v", q)
	}
	s.ForgetExternalStorage("acme")
	q, err := s.Quota(ctx)
	if err != nil || q.ConnectedStorage || q.LimitBytes != 500 || q.UsedBytes != 200 || q.Available() != 300 {
		t.Fatalf("desconectado: %+v, %v", q, err)
	}
}

func TestQuotaSinCapacidadRemotaConservaElPlanYLaConexion(t *testing.T) {
	for _, b := range []blobStore{
		&fakeBlobs{}, // S3 no informa de capacidad finita estándar.
		capacityBlobs{fakeBlobs: &fakeBlobs{}, err: errors.New("permission denied")},
		capacityBlobs{fakeBlobs: &fakeBlobs{}},
	} {
		st := &fakeStore{quota: domain.Quota{LimitBytes: 500, UsedBytes: 200}}
		s := newSvc(st, &fakeBlobs{}, nil)
		s.router = newBlobRouter(&fakeBlobs{}, &fakeExternal{}, "", nil)
		s.router.cache["acme"] = entradaExterna{blobs: b, expires: time.Now().Add(time.Hour)}
		q, err := s.Quota(tenantCtx(t))
		if err != nil || !q.ConnectedStorage || q.StorageTotalBytes != nil || q.LimitBytes != 500 || q.Available() != 300 {
			t.Fatalf("una cuota desconocida no desconecta ni concede bytes ficticios: %+v, %v", q, err)
		}
	}
}

func TestCambiarCuotaBaseNoPersisteLaAmpliacion(t *testing.T) {
	total, used, available := int64(1500), int64(600), int64(900)
	st := &quotaLimitStore{fakeStore: &fakeStore{quota: domain.Quota{LimitBytes: 500, UsedBytes: 200}}}
	s := newSvc(st.fakeStore, &fakeBlobs{}, nil)
	s.store = st
	s.router = newBlobRouter(&fakeBlobs{}, &fakeExternal{}, "", nil)
	s.router.cache["acme"] = entradaExterna{
		blobs: capacityBlobs{fakeBlobs: &fakeBlobs{}, capacity: objectstore.Capacity{
			TotalBytes: &total, UsedBytes: &used, AvailableBytes: &available,
		}},
		expires: time.Now().Add(time.Hour),
	}
	q, err := s.SetQuotaLimit(tenantCtx(t), 700)
	if err != nil || st.received != 700 || q.BaseLimitBytes != 700 || q.LimitBytes != 2200 {
		t.Fatalf("se persiste sólo el plan y se devuelve el total: guardado=%d cuota=%+v err=%v", st.received, q, err)
	}
}

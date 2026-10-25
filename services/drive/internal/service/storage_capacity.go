// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// quotaWithStorage añade la capacidad física conectada a la vista de la
// cuenta. Las escrituras siguen reservando únicamente la cuota de Sytue en el
// store; sus blobs externos ya quedan fuera de esa reserva.
func (s *Service) quotaWithStorage(ctx context.Context, q domain.Quota) domain.Quota {
	q.BaseLimitBytes, q.BaseUsedBytes = q.LimitBytes, q.UsedBytes
	t, err := tenancy.From(ctx)
	if err != nil || s.router == nil {
		return q
	}
	// Un solo presupuesto para el almacén del tenant y las nubes del usuario:
	// una cuenta lenta no añade tres segundos por proveedor al arranque.
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var org, personal personalStorageCapacity
	var readers sync.WaitGroup
	readers.Go(func() {
		b, err := s.router.externo(bounded, t)
		if err != nil || b == nil {
			return
		}
		org.Connected = true
		if reader, ok := b.(objectstore.CapacityReader); ok {
			if reported, err := reader.Capacity(bounded); err == nil {
				org.Capacity = reported
			}
		}
	})
	actor, authenticated := authn.FromContext(ctx)
	if authenticated && actor.Subject != "" && !actor.Delegado && s.router.conn != nil {
		readers.Go(func() {
			if reported, err := s.router.conn.PersonalCapacity(bounded, t, actor.Subject); err == nil {
				personal = reported
			}
		})
	}
	readers.Wait()
	capacities := make([]objectstore.Capacity, 0, 2)
	for _, result := range []personalStorageCapacity{org, personal} {
		if result.Connected {
			capacities = append(capacities, result.Capacity)
		}
	}
	if len(capacities) == 0 {
		return q
	}
	capacity := objectstore.SumCapacities(capacities...)
	q = q.WithStorageExtension(capacity.TotalBytes, capacity.UsedBytes, capacity.AvailableBytes)
	q.StorageCapacityPartial = q.StorageCapacityPartial || capacity.Partial
	return q
}

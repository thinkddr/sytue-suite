// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"strconv"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// umbralCuota es el porcentaje a partir del cual el almacenamiento «se está llenando».
const umbralCuota = 90

// cruzaUmbral dice si sumar `sumados` bytes ha llevado el uso de la cuota por encima
// de umbralCuota, desde por debajo: una vez por cruce y no en cada subida posterior.
func cruzaUmbral(q domain.Quota, sumados int64) bool {
	if q.LimitBytes <= 0 || sumados <= 0 {
		return false
	}
	return q.UsedBytes*100 >= q.LimitBytes*umbralCuota && (q.UsedBytes-sumados)*100 < q.LimitBytes*umbralCuota
}

// avisarCuota deja en la campana de quien sube el «tu almacenamiento está casi lleno»
// cuando SU subida cruza el umbral. La cuota es de la organización (en una cuenta
// personal, de la persona), y avisa a quien la acaba de llenar, que es quien lo ve.
// Mejor esfuerzo: la subida ya está hecha.
func (s *Service) avisarCuota(ctx context.Context, sub string, sumados int64) {
	if s.hubNotifier == nil || sub == "" {
		return
	}
	q, err := s.store.EnsureQuota(ctx, s.cfg.DefaultQuotaBytes)
	if err != nil || !cruzaUmbral(q, sumados) {
		return
	}
	pct := strconv.FormatInt(q.UsedBytes*100/q.LimitBytes, 10)
	t, _ := tenancy.From(ctx)
	s.hubNotifier.SendAsync(ctx, hubnotify.Notification{
		Tenant:  t.Slug,
		Subject: sub,
		Source:  "drive",
		Type:    "drive.quota.warning",
		Level:   "warning",
		Title:   "Tu almacenamiento está al " + pct + " %",
		URL:     s.cfg.PublicURL,
		Params:  map[string]string{"percent": pct},
	})
}

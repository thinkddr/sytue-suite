// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// tenantQuota devuelve los límites del plan de la organización (free si falla la
// lectura cross-tenant).
func (c *Controller) tenantQuota(ctx context.Context, tenant string) domain.PlanQuota {
	plan, err := c.store.GetOrganizationPlanByTenant(ctx, tenant)
	if err != nil || !domain.ValidPlan(plan) {
		return domain.QuotasFor(domain.PlanFree)
	}
	return domain.QuotasFor(plan)
}

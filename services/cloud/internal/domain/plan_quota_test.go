// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

func TestQuotasForPlans(t *testing.T) {
	free := QuotasFor(PlanFree)
	if free.MaxProjects != 3 || free.MaxManagedServices != 2 {
		t.Fatalf("free: %+v", free)
	}
	pro := QuotasFor(PlanPro)
	if pro.MaxProjects != 10 || pro.PostgresPVCSize != "5Gi" {
		t.Fatalf("pro: %+v", pro)
	}
	team := QuotasFor(PlanTeam)
	if team.MaxPods != 100 || team.BackupRetentionDays != 30 {
		t.Fatalf("team: %+v", team)
	}
	if q := QuotasFor(Plan("unknown")); q.Plan != PlanFree {
		t.Fatalf("unknown plan should fall back to free, got %+v", q)
	}
	if free.MaxStorage != "10Gi" || free.MaxPVCs != 4 {
		t.Fatalf("free storage/pvc quota inesperada: %+v", free)
	}
	if pro.MaxStorage != "50Gi" || pro.MaxPVCs != 10 {
		t.Fatalf("pro storage/pvc quota inesperada: %+v", pro)
	}
	if team.MaxStorage != "200Gi" || team.MaxPVCs != 30 {
		t.Fatalf("team storage/pvc quota inesperada: %+v", team)
	}
}

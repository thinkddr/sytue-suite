// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

// PlanQuota fija los límites operativos de una organización según su plan de
// facturación. El controller los traduce a ResourceQuota/LimitRange de K8s; el
// servicio los usa para cuotas de proyectos y servicios gestionados.
type PlanQuota struct {
	Plan               Plan
	MaxProjects        int
	MaxManagedServices int
	// Recursos agregados del namespace tenant (ResourceQuota).
	RequestCPU    string
	RequestMemory string
	LimitCPU      string
	LimitMemory   string
	MaxPods       int
	// Tamaño del PVC de Postgres gestionado por instancia.
	PostgresPVCSize string
	// Tope de almacenamiento persistente total del namespace (requests.storage)
	// y de PersistentVolumeClaims (persistentvolumeclaims en ResourceQuota).
	MaxStorage string
	MaxPVCs    int
	// Días de retención de backups pg_dump en Object Storage.
	BackupRetentionDays int
}

// QuotasFor devuelve los límites del plan (free por defecto si plan inválido).
func QuotasFor(plan Plan) PlanQuota {
	switch plan {
	case PlanPro:
		return PlanQuota{
			Plan: PlanPro, MaxProjects: 10, MaxManagedServices: 5,
			RequestCPU: "4", RequestMemory: "8Gi", LimitCPU: "8", LimitMemory: "16Gi",
			MaxPods: 50, PostgresPVCSize: "5Gi", MaxStorage: "50Gi", MaxPVCs: 10, BackupRetentionDays: 14,
		}
	case PlanTeam:
		return PlanQuota{
			Plan: PlanTeam, MaxProjects: 25, MaxManagedServices: 15,
			RequestCPU: "8", RequestMemory: "16Gi", LimitCPU: "16", LimitMemory: "32Gi",
			MaxPods: 100, PostgresPVCSize: "10Gi", MaxStorage: "200Gi", MaxPVCs: 30, BackupRetentionDays: 30,
		}
	default:
		return PlanQuota{
			Plan: PlanFree, MaxProjects: 3, MaxManagedServices: 2,
			RequestCPU: "2", RequestMemory: "4Gi", LimitCPU: "4", LimitMemory: "8Gi",
			MaxPods: 25, PostgresPVCSize: "2Gi", MaxStorage: "10Gi", MaxPVCs: 4, BackupRetentionDays: 7,
		}
	}
}

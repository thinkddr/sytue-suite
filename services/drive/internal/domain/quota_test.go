// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"math"
	"testing"
)

// UsedBytes ya incluye lo externo: el desglose solo resta, y nunca por debajo de cero
// (una declaración que llegue antes de que el store recomponga el total no puede
// dejar a Drive con bytes negativos).
func TestQuotaDesgloseExterno(t *testing.T) {
	q := Quota{LimitBytes: 1000, UsedBytes: 500, ExternalUsage: map[string]int64{"cast": 300}}
	if q.ExternalBytes() != 300 || q.DriveBytes() != 200 {
		t.Fatalf("externo = %d, drive = %d; quiero 300 y 200", q.ExternalBytes(), q.DriveBytes())
	}
	if q.Available() != 500 || q.HasRoomFor(501) || !q.HasRoomFor(500) {
		t.Fatalf("el hueco tiene que salir del total: available = %d", q.Available())
	}
	sin := Quota{LimitBytes: 1000, UsedBytes: 500}
	if sin.ExternalBytes() != 0 || sin.DriveBytes() != 500 {
		t.Fatalf("sin externo: %d %d", sin.ExternalBytes(), sin.DriveBytes())
	}
	raro := Quota{LimitBytes: 1000, UsedBytes: 100, ExternalUsage: map[string]int64{"cast": 300}}
	if raro.DriveBytes() != 0 {
		t.Fatalf("drive nunca negativo: %d", raro.DriveBytes())
	}
}

func TestQuotaSumaLaCapacidadConectadaSinCambiarElPlan(t *testing.T) {
	total, used, available := int64(1500), int64(600), int64(900)
	base := Quota{LimitBytes: 500, UsedBytes: 200, ExternalUsage: map[string]int64{"cast": 50}}
	q := base.WithStorageExtension(&total, &used, &available)
	if q.LimitBytes != 2000 || q.UsedBytes != 800 || q.Available() != 1200 {
		t.Fatalf("capacidad combinada: %+v, libre=%d", q, q.Available())
	}
	if q.BaseLimitBytes != 500 || q.BaseUsedBytes != 200 || base.LimitBytes != 500 || base.UsedBytes != 200 {
		t.Fatalf("la ampliación no puede reescribir el plan: base=%+v vista=%+v", base, q)
	}
	if q.DriveBytes() != 150 || q.ExternalBytes() != 50 {
		t.Fatalf("el uso del proveedor no se atribuye a ficheros de Drive: %+v", q)
	}
	q = q.WithStorageExtension(&total, &used, &available)
	if q.LimitBytes != 2000 || q.UsedBytes != 800 || q.Available() != 1200 {
		t.Fatalf("refrescar no suma dos veces: %+v", q)
	}
}

func TestQuotaRespetaElLibreRealDeCadaAlmacen(t *testing.T) {
	total, used, available := int64(1000), int64(100), int64(25)
	q := (Quota{LimitBytes: 500, UsedBytes: 200}).WithStorageExtension(&total, &used, &available)
	if q.Available() != 325 || q.HasRoomFor(326) || !q.HasRoomFor(325) {
		t.Fatalf("el límite individual del proveedor debe respetarse: %+v, libre=%d", q, q.Available())
	}
	// Una cuota local sobrepasada no se come el espacio libre del proveedor.
	q = (Quota{LimitBytes: 500, UsedBytes: 700}).WithStorageExtension(&total, &used, &available)
	if q.Available() != 25 {
		t.Fatalf("el proveedor conserva sus 25 bytes libres: %d", q.Available())
	}
	available = 0
	q = (Quota{LimitBytes: 500, UsedBytes: 200}).WithStorageExtension(&total, &used, &available)
	if q.Available() != 300 || q.StorageAvailableBytes == nil || *q.StorageAvailableBytes != 0 {
		t.Fatalf("cero libre no equivale a capacidad desconocida: %+v", q)
	}
}

func TestQuotaNoInventaCapacidadAusenteOInvalida(t *testing.T) {
	value := func(n int64) *int64 { return &n }
	for _, tt := range []struct {
		name                   string
		total, used, available *int64
	}{
		{"sin datos", nil, nil, nil},
		{"límite desconocido", nil, value(100), nil},
		{"uso ausente", value(1000), nil, value(500)},
		{"negativo", value(-1), value(0), value(0)},
		{"libre imposible", value(100), value(0), value(101)},
		{"desbordamiento de límite", value(math.MaxInt64), value(0), value(0)},
		{"desbordamiento de uso", value(1000), value(math.MaxInt64), value(0)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := (Quota{LimitBytes: 500, UsedBytes: 200}).WithStorageExtension(tt.total, tt.used, tt.available)
			if !q.ConnectedStorage || q.LimitBytes != 500 || q.UsedBytes != 200 || q.Available() != 300 {
				t.Fatalf("conectado no significa una cifra inventada: %+v", q)
			}
		})
	}
}

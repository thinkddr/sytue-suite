// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

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

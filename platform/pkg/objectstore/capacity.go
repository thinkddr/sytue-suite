// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"context"
	"errors"
	"math"
	"net/http"
)

// Capacity describe la cuota del almacén conectado. Nil significa que el
// proveedor no comunica ese dato; un puntero a cero sí significa cero bytes.
// UsedBytes puede incluir datos ajenos a Sytue y AvailableBytes puede ser
// menor que TotalBytes-UsedBytes por límites adicionales de la cuenta.
type Capacity struct {
	TotalBytes     *int64 `json:"total_bytes"`
	UsedBytes      *int64 `json:"used_bytes"`
	AvailableBytes *int64 `json:"available_bytes"`
	// Partial conserva que otras fuentes conectadas no informan de su cuota.
	Partial bool `json:"partial,omitempty"`
}

// CapacityReader es opcional: conectar un BlobStore no implica conocer su
// cuota. S3 no ofrece una capacidad finita estándar y no implementa esta
// interfaz. Una lectura fallida de capacidad no invalida la conexión de blobs.
type CapacityReader interface {
	Capacity(context.Context) (Capacity, error)
}

// ProviderCapacityReader reutiliza los adaptadores de cuota con el transporte
// del llamador: la pasarela de integraciones conserva su política y auditoría
// de egress, sin exportar la credencial personal a Drive.
func ProviderCapacityReader(provider string, client *http.Client, token TokenSource) (CapacityReader, error) {
	if client == nil || token == nil {
		return nil, errors.New("capacidad: hacen falta transporte y credencial")
	}
	switch provider {
	case "google-drive", "gdrive":
		return &GoogleDrive{http: client, tok: token}, nil
	case "onedrive":
		return &OneDrive{http: client, tok: token}, nil
	case "dropbox":
		return &Dropbox{http: client, tok: token}, nil
	default:
		return nil, errors.New("capacidad: proveedor no admitido")
	}
}

// SumCapacities suma las fuentes que conocen sus tres cifras y señala las que
// no las conocen. Si ninguna es conocida, los bytes siguen siendo nil. Una
// suma no representable tampoco se presenta como cero ni se satura.
func SumCapacities(capacities ...Capacity) Capacity {
	if len(capacities) == 0 {
		return Capacity{}
	}
	var total, used, available int64
	known, partial := false, false
	for _, capacity := range capacities {
		partial = partial || capacity.Partial
		if capacity.TotalBytes == nil || capacity.UsedBytes == nil || capacity.AvailableBytes == nil {
			partial = true
			continue
		}
		t, u, a := *capacity.TotalBytes, *capacity.UsedBytes, *capacity.AvailableBytes
		if t < 0 || u < 0 || a < 0 || a > t {
			partial = true
			continue
		}
		if t > math.MaxInt64-total || u > math.MaxInt64-used || a > math.MaxInt64-available {
			return Capacity{Partial: true}
		}
		total, used, available = total+t, used+u, available+a
		known = true
	}
	if !known {
		return Capacity{Partial: partial}
	}
	return Capacity{TotalBytes: &total, UsedBytes: &used, AvailableBytes: &available, Partial: partial}
}

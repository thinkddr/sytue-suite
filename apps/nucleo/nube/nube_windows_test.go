// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package nube

import (
	"testing"
	"unsafe"
)

// TestLasEstructurasMidenLoDeCfapi: los tamaños de `cfapi.h` y `shellapi.h` en x64. Si uno
// se mueve, Windows lee los campos corridos y el fallo sale como un HRESULT sin explicación.
func TestLasEstructurasMidenLoDeCfapi(t *testing.T) {
	for _, c := range []struct {
		nombre     string
		tiene, cfp uintptr
	}{
		{"CF_SYNC_REGISTRATION", unsafe.Sizeof(registro{}), 72},
		{"CF_SYNC_POLICIES", unsafe.Sizeof(politicas{}), 24},
		{"CF_CALLBACK_REGISTRATION", unsafe.Sizeof(llamada{}), 16},
		{"CF_CALLBACK_INFO", unsafe.Sizeof(infoLlamada{}), 152},
		{"CF_OPERATION_INFO", unsafe.Sizeof(operacion{}), 48},
		{"CF_OPERATION_PARAMETERS(TransferData)", unsafe.Sizeof(opDatos{}), 40},
		{"CF_OPERATION_PARAMETERS(TransferPlaceholders)", unsafe.Sizeof(opMarcadores{}), 40},
		{"CF_PLACEHOLDER_CREATE_INFO", unsafe.Sizeof(marcador{}), 88},
		{"CF_CALLBACK_PARAMETERS(FetchData)", unsafe.Offsetof(paramsBajar{}.desde), 16},
		{"CF_CALLBACK_INFO.FileIdentity", unsafe.Offsetof(infoLlamada{}.identidad), 88},
		{"CF_CALLBACK_INFO.TransferKey", unsafe.Offsetof(infoLlamada{}.transferencia), 112},
		{"CF_CALLBACK_INFO.RequestKey", unsafe.Offsetof(infoLlamada{}.peticion), 144},
		{"CF_CALLBACK_INFO.VolumeDosName", unsafe.Offsetof(infoLlamada{}.volumen), 32},
		{"CF_CALLBACK_INFO.NormalizedPath", unsafe.Offsetof(infoLlamada{}.ruta), 104},
		{"CF_SYNC_REGISTRATION.ProviderId", unsafe.Offsetof(registro{}.proveedor), 52},
		{"CF_OPERATION_PARAMETERS(AckDelete/AckRename)", unsafe.Sizeof(opAck{}), 16},
		{"CF_CALLBACK_PARAMETERS(Rename).TargetPath", unsafe.Offsetof(paramsRenombrar{}.destino), 16},
		{"CF_CALLBACK_PARAMETERS(Delete/CloseCompletion).Flags", unsafe.Offsetof(paramsBanderas{}.banderas), 8},
		{"SHFILEOPSTRUCTW", unsafe.Sizeof(operacionShell{}), 56},
		{"SHFILEOPSTRUCTW.fFlags", unsafe.Offsetof(operacionShell{}.banderas), 32},
		{"SHFILEOPSTRUCTW.fAnyOperationsAborted", unsafe.Offsetof(operacionShell{}.abortada), 36},
	} {
		if c.tiene != c.cfp {
			t.Errorf("%s: %d, y en cfapi.h es %d", c.nombre, c.tiene, c.cfp)
		}
	}
}

// TestActualizarSinIdentidadNoSeCae: la raíz registrada por el Explorador no tiene
// identidad, y marcarla como sincronizada tumbaba el Hub con un índice fuera de rango.
// Con un manejador inválido Windows dice que no; lo que se prueba es que no hay pánico.
func TestActualizarSinIdentidadNoSeCae(t *testing.T) {
	if actualizar(0, `C:\no-existe`, "", nil, actualizarEnSync) {
		t.Fatal("un manejador inválido no puede actualizarse")
	}
}

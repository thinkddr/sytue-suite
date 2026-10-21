// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"context"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/peticion"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # La salida de la franja de solo lectura, en los tres editores
//
// Es `FranjaSoloLectura.tsx` de la web: quien solo puede leer pide acceso de edición
// (services/docs/internal/http/peticiones.go), el dueño lo ve en su franja y lo concede
// con un clic, y la franja de quien lo pidió se quita sola en cuanto se concede. Docus,
// Calcus y Diapos lo enganchan igual, así que vive aquí y no en cada app.

// EsperaCanalRol es la primera espera antes de reabrir el canal que escucha el rol; se
// dobla hasta un minuto.
var EsperaCanalRol = 2 * time.Second

// EngancharPeticionEdicion pone los botones de la franja según el rol. Llamarla en cada carga: rehace los campos de la vez anterior.
//
//   - Sin permiso de escritura y con la franja por PERMISO (sin MotivoSoloLectura):
//     «Pedir acceso de edición», o «Petición enviada» si ya se pidió.
//   - El dueño: la primera petición pendiente, con «Dar acceso» y «Rechazar».
//
// `escuchar` abre el canal de coedición solo para enterarse de que el rol ha cambiado.
// Pásalo a false si la app ya lo tiene abierto (Docus) y llama a `reabrir` desde su
// propio manejador del mensaje `role`. `reabrir` vuelve a cargar el documento: el rol
// nuevo decide todo lo demás (la cinta, el guardado, la franja).
func EngancharPeticionEdicion(c *Contexto, svc *api.Docs, id, rol string, escuchar bool, recomponer, reabrir func()) {
	DejarDeEscucharRol()
	c.Peticion = peticion.Estado{}
	if svc == nil || id == "" {
		return
	}
	sinPermiso := c.SoloLectura && c.MotivoSoloLectura == "" && !api.CanWrite(rol)
	peticion.Enganchar(&c.Peticion, svc.Peticiones(id), peticion.Opciones{
		SinPermiso:    sinPermiso,
		PuedeConceder: rol == "owner",
		YaPedida:      func(ctx context.Context) (bool, error) { return svc.EdicionPedida(ctx, id) },
		Recomponer:    recomponer,
		Fallo: func(texto string) {
			if c.Notificar != nil {
				c.Notificar(ui.AvisoPeligro, texto)
			}
		},
		T: func(k string, v i18n.Vars) string { return c.Traducir(k, v) },
	})
	if sinPermiso && escuchar {
		escucharRol(svc, id, reabrir)
	}
}

// ponytail: UNA escucha por proceso, que es lo que abre cada app (un documento a la
// vez). Si una app llega a tener dos documentos abiertos, pasa a ser por Contexto.
var (
	muRol     sync.Mutex
	cortarRol context.CancelFunc
)

// DejarDeEscucharRol corta la escucha del rol del documento que se deja (Volver).
func DejarDeEscucharRol() {
	muRol.Lock()
	defer muRol.Unlock()
	if cortarRol != nil {
		cortarRol()
		cortarRol = nil
	}
}

// escucharRol abre el canal de coedición del documento solo para enterarse de que el
// rol de quien lo tiene abierto ha cambiado, y llama a `reabrir` (en el hilo de la
// interfaz) cuando ya puede escribir. Es el canal que el servicio ya tiene, no un
// sondeo: Calcus y Diapos no coeditan todavía y no lo abrían.
func escucharRol(svc *api.Docs, id string, reabrir func()) {
	ctx, cancelar := context.WithCancel(context.Background())
	muRol.Lock()
	cortarRol = cancelar
	muRol.Unlock()
	go func() {
		espera := EsperaCanalRol
		for ctx.Err() == nil {
			// Desde una seq inalcanzable: solo se quiere el rol, no la repetición de todas
			// las operaciones del documento.
			_ = svc.Coeditar(ctx, id, 1<<62, func(m api.DocMensaje) {
				espera = EsperaCanalRol
				if m.Type == "role" && api.CanWrite(m.Role) {
					cancelar()
					fyne.Do(reabrir)
				}
			})
			select {
			case <-ctx.Done():
				return
			case <-time.After(espera):
			}
			espera = min(2*espera, time.Minute)
		}
	}()
}

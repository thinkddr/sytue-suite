// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"log/slog"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/matrix/cierre"
)

// prefijoSesionMatrix es la cuenta del llavero donde apps/nucleo/matrix guarda la sesión
// de cada dispositivo (Rooms y los de almacenamiento privado de cada app).
const prefijoSesionMatrix = "rooms-matrix-session-"

// Lo que las pruebas sustituyen: el cliente que llega a un homeserver de prueba y el plazo.
var (
	clienteCierreMatrix = func() *http.Client { return &http.Client{Transport: api.Transporte} }
	plazoCierreMatrix   = cierre.Plazo
)

// cerrarDispositivosMatrix cierra en el homeserver los dispositivos de Matrix de `sujeto`
// cuyas sesiones va a olvidar la salida. Va ANTES de olvidarlas —sin token ya no se
// pueden cerrar— y nunca la bloquea: con plazo corto y sin error, porque un dispositivo
// cuyo llavero se borra queda inservible aquí aunque el homeserver no conteste.
func cerrarDispositivosMatrix(ctx context.Context, sujeto string) {
	if sujeto == "" {
		return
	}
	cuentas, err := auth.CuentasCon(prefijoSesionMatrix)
	if err != nil || len(cuentas) == 0 {
		return
	}
	ctx, parar := context.WithTimeout(context.WithoutCancel(ctx), plazoCierreMatrix)
	defer parar()
	if total, cerrados := cierre.DeSujeto(ctx, clienteCierreMatrix(), sujeto, cuentas, auth.LeerSecreto); cerrados < total {
		slog.Warn("sesion: no se han podido cerrar en el homeserver todos los dispositivos de Matrix", "total", total, "cerrados", cerrados)
	}
}

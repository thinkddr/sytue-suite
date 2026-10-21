// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// ErrDestinoNoPermitido: el almacén de terceros apunta a una dirección que el servicio
// no debe alcanzar (red interna, o sin TLS).
var ErrDestinoNoPermitido = errors.New("service: la dirección del almacén tiene que ser https y pública")

// maxSaltos acota las redirecciones de un almacén de terceros.
const maxSaltos = 5

// clienteDeTerceros es el cliente HTTP de TODO almacén cuya dirección la pone la
// organización (WebDAV, NAS) o la devuelve un proveedor (sesiones de subida de
// Google Drive y OneDrive). La dirección la escribe un administrador de la
// organización —en una cuenta personal, la propia persona—, así que es
// entrada de fuera: el destino se valida al conectar (`ssrfhttp.SafeDialContext`,
// que resuelve, exige IP pública y conecta a ESA IP) y cada redirección se vuelve a
// validar y tiene que seguir siendo https. Sin esto, un almacén «de terceros»
// podía ser un servicio interno de la suite, o redirigir a uno, y su respuesta
// volvía como contenido del fichero.
//
// Sin plazo total: una subida o una descarga dura lo que dure el fichero. El que
// hay es el de la cabecera de respuesta.
func clienteDeTerceros(cabecera time.Duration) *http.Client {
	tr := ssrfhttp.Transport()
	tr.ResponseHeaderTimeout = cabecera
	return &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxSaltos {
				return fmt.Errorf("%w: demasiadas redirecciones", ErrDestinoNoPermitido)
			}
			if req.URL.Scheme != "https" {
				return fmt.Errorf("%w: la redirección sale de https", ErrDestinoNoPermitido)
			}
			return ssrfhttp.ValidateURL(req.Context(), req.URL.String())
		},
	}
}

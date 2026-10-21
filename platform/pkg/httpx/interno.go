// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// SecretoInterno protege una ruta de servicio a servicio (bajo /internal, que el
// Ingress no publica) con el secreto compartido de la suite en `Authorization:
// Bearer`, comparado en tiempo constante. Sin secreto configurado no deja pasar a
// nadie: una ruta interna abierta por olvido de un valor es peor que una caída.
//
// Es el único: TestNadieCopiaElChequeoDelSecretoInterno falla si reaparece una copia.
func SecretoInterno(secreto string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Sin el prefijo no hay Bearer: CutPrefix devolvería la cabecera entera, y
			// `Authorization: <secreto>` a secas pasaría.
			dado, conPrefijo := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !conPrefijo || !casa(dado, secreto) {
				Fail(w, r, ErrUnauthorized.WithMessage("Invalid internal secret."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecretoEnCabecera es SecretoInterno para una puerta que ya lleva sesión en
// `Authorization` y pide, ADEMÁS, un secreto compartido en otra cabecera (la verja
// `X-Sytue-Publicador` de las publicaciones de Pages: la persona sola no basta, tiene
// que llegar por el BFF). Misma regla: tiempo constante y, sin secreto configurado,
// no pasa nadie. Contesta 403 y no 401: la sesión es buena, y un 401 haría creer al
// cliente que ha caducado.
func SecretoEnCabecera(nombre, secreto string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !casa(r.Header.Get(nombre), secreto) {
				Fail(w, r, ErrForbidden.WithMessage("Invalid internal secret."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// casa compara en tiempo constante; un secreto vacío no casa con nada.
func casa(dado, secreto string) bool {
	return secreto != "" && subtle.ConstantTimeCompare([]byte(dado), []byte(secreto)) == 1
}

// SoloDesdeDentro es SecretoInterno para las rutas internas que el Ingress SÍ publica
// (su host lo manda todo al servicio): con X-Forwarded-For —lo pone Traefik, y los
// servicios llaman al Service sin pasar por él— contesta 404 ANTES de mirar el secreto,
// así que desde internet ni se confirma que la ruta existe ni se puede probar secretos.
// Es la misma frontera que ya usa HealthHandler para la versión.
func SoloDesdeDentro(secreto string) func(http.Handler) http.Handler {
	conSecreto := SecretoInterno(secreto)
	return func(next http.Handler) http.Handler {
		return SinPasarPorElIngress(conSecreto(next))
	}
}

// SinPasarPorElIngress es la frontera de SoloDesdeDentro sola, para una puerta
// interna con su propia credencial (el token de operador de Integraciones): lo que
// llega con X-Forwarded-For viene de internet por Traefik y recibe un 404.
func SinPasarPorElIngress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Forwarded-For") != "" {
			Fail(w, r, ErrNotFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

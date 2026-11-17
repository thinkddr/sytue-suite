// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Dispositivos cierra en el homeserver los dispositivos de Matrix que ha dado de alta una
// tirada de `scripts/ventana-real.sh`, para que la cuenta de auditoría no acumule uno
// nuevo por app en cada tirada: cada tirada estrena llavero y carpeta, así que no queda
// ningún dispositivo que reutilizar la vez siguiente.
//
// Las apps de cliente cifrado no llegan a hacerlo: el guion las mata sin cerrar la sesión
// de la suite. Aquí se leen del llavero DE LA TIRADA —el
// gnome-keyring de la sesión de D-Bus que abre el guion— las sesiones que guardó
// `apps/nucleo/matrix` (`rooms-matrix-session-*` y los cierres pendientes
// `matrix-logout-*`) y se cierra cada dispositivo con su propio token
// (`POST /_matrix/client/v3/logout`), y solo contra el homeserver de la suite. No escribe
// ningún token: solo cuántos dispositivos había y cuántos se cerraron.
//
//	go run ./herramientas/dispositivos    (desde apps/fyne, dentro de la sesión de D-Bus)
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// homeserver es el de la suite: un token no sale hacia ningún otro.
const homeserver = "https://matrix.sytue.eu"

// prefijos son las cuentas del llavero donde `apps/nucleo/matrix` guarda la sesión de un
// dispositivo (sessionKey) y la de uno que quedó a medio cerrar (logoutIntentKey).
var prefijos = []string{"rooms-matrix-session-", "matrix-logout-"}

// sesion es lo que hace falta de `credentials` (apps/nucleo/matrix/client.go).
type sesion struct {
	Homeserver  string `json:"homeserver"`
	DeviceID    string `json:"device_id"`
	AccessToken string `json:"access_token"`
}

// deSecreto lee la sesión de un secreto: tal cual, o dentro de un cierre pendiente.
func deSecreto(datos []byte, servidor string) (sesion, bool) {
	var envuelto struct {
		Session *sesion `json:"session"`
		sesion
	}
	if json.Unmarshal(datos, &envuelto) != nil {
		return sesion{}, false
	}
	s := envuelto.sesion
	if envuelto.Session != nil {
		s = *envuelto.Session
	}
	return s, s.Homeserver == servidor && s.DeviceID != "" && s.AccessToken != ""
}

// cerrar cierra cada dispositivo una vez (una sesión y su cierre pendiente son el mismo).
// Un 401 es un token que ya no vale: ese dispositivo ya está cerrado.
func cerrar(ctx context.Context, cuentas []string, leer func(string) ([]byte, error), cliente *http.Client, servidor string) (total, cerrados int) {
	tokens := map[string]string{}
	for _, c := range cuentas {
		datos, err := leer(c)
		if err != nil {
			continue
		}
		if s, ok := deSecreto(datos, servidor); ok {
			tokens[s.DeviceID] = s.AccessToken
		}
		clear(datos)
	}
	for _, token := range tokens {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, servidor+"/_matrix/client/v3/logout", bytes.NewReader([]byte("{}")))
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := cliente.Do(req)
		if err != nil {
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode/100 == 2 || resp.StatusCode == http.StatusUnauthorized {
			cerrados++
		}
	}
	return len(tokens), cerrados
}

func main() { os.Exit(principal()) }

func principal() int {
	var cuentas []string
	for _, p := range prefijos {
		c, err := auth.CuentasCon(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "dispositivos: no se pudo leer el llavero de la tirada:", err)
			return 1
		}
		cuentas = append(cuentas, c...)
	}
	ctx, parar := context.WithTimeout(context.Background(), time.Minute)
	defer parar()
	// Sin seguir redirecciones: el token va al homeserver y a ningún otro sitio.
	cliente := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	total, cerrados := cerrar(ctx, cuentas, auth.LeerSecreto, cliente, homeserver)
	fmt.Printf("dispositivos de Matrix de la tirada: %d; cerrados: %d\n", total, cerrados)
	if cerrados < total {
		return 1
	}
	return 0
}

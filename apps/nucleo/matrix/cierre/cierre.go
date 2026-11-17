// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package cierre cierra en el homeserver un dispositivo de Matrix con SU PROPIO token
// (`POST /_matrix/client/v3/logout`). Es la única implementación de ese cierre en las
// nativas: la usa el logout del dispositivo de Rooms (apps/nucleo/matrix, el que deja
// una intención pendiente si no hay red) y el cierre de sesión de la suite
// (apps/fyne/sesion), que antes solo olvidaba del llavero las sesiones de los
// dispositivos de almacenamiento privado de Mail, Hub, ERP… y los dejaba vivos en el
// homeserver: uno muerto por cada entrada, que además recibía cada clave de sala por
// to-device y seguía figurando como dispositivo de confianza.
//
// No importa el SDK de Matrix a propósito: la sesión de la suite lo enlaza en las trece
// apps y no puede arrastrar mautrix ni la criptografía para hacer un POST.
package cierre

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Plazo acota el cierre remoto al salir: sin homeserver se sale igual, y lo que se
// olvida después del llavero no se puede reutilizar.
const Plazo = 3 * time.Second

// ErrSinRecibo es una respuesta que no prueba que el dispositivo ya no exista.
var ErrSinRecibo = errors.New("matrix: el homeserver no confirmó el cierre del dispositivo")

// Sesion es lo que hace falta de la sesión guardada de un dispositivo (las
// `credentials` de apps/nucleo/matrix, que se guardan con estos mismos nombres).
type Sesion struct {
	Homeserver  string `json:"homeserver"`
	DeviceID    string `json:"device_id"`
	AccessToken string `json:"access_token"`
	Subject     string `json:"subject"`
}

// Dispositivo cierra el dispositivo dueño de `token`. Un 401 M_UNKNOWN_TOKEN sin
// soft_logout es un dispositivo que ya no existe; con soft_logout (o con un valor que
// no es booleano) solo caducó el token, y eso no es un recibo.
func Dispositivo(ctx context.Context, httpc *http.Client, homeserver, token string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(homeserver, "/")+"/_matrix/client/v3/logout", strings.NewReader("{}"))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	cuerpo, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode/100 == 2 {
		return nil
	}
	if resp.StatusCode == http.StatusUnauthorized {
		var fallo map[string]any
		if json.Unmarshal(cuerpo, &fallo) == nil && fallo["errcode"] == "M_UNKNOWN_TOKEN" {
			valor, presente := fallo["soft_logout"]
			blando, valido := valor.(bool)
			if !presente || (valido && !blando) {
				return nil
			}
		}
	}
	return fmt.Errorf("%w: HTTP %d", ErrSinRecibo, resp.StatusCode)
}

// DeSujeto cierra, una vez cada uno, los dispositivos guardados en `cuentas` que son de
// `sujeto`: los de otra persona que comparta el llavero no se tocan, y sin sujeto no se
// cierra nada. Cada token va solo al homeserver de su propia sesión y sin seguir
// redirecciones. Devuelve cuántos había y cuántos confirmó el homeserver.
func DeSujeto(ctx context.Context, httpc *http.Client, sujeto string, cuentas []string, leer func(string) ([]byte, error)) (total, cerrados int) {
	if sujeto == "" {
		return 0, 0
	}
	sesiones := map[string]Sesion{}
	for _, cuenta := range cuentas {
		datos, err := leer(cuenta)
		if err != nil {
			continue
		}
		var s Sesion
		valida := json.Unmarshal(datos, &s) == nil
		clear(datos)
		if !valida || s.Subject != sujeto || s.DeviceID == "" || s.AccessToken == "" || !homeserverValido(s.Homeserver) {
			continue
		}
		sesiones[s.Homeserver+"\x00"+s.DeviceID] = s
	}
	sinRedirecciones := *httpc
	sinRedirecciones.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	for _, s := range sesiones {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if Dispositivo(ctx, &sinRedirecciones, s.Homeserver, s.AccessToken) == nil {
				mu.Lock()
				cerrados++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return len(sesiones), cerrados
}

func homeserverValido(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

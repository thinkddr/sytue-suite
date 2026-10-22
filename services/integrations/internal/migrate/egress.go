// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

// maxJSONBytes acota la lectura de una página de metadatos (defensa anti-OOM). El
// CONTENIDO de los ficheros no pasa por aquí: se copia en streaming.
const maxJSONBytes = 32 << 20

// maxPages acota los saltos de paginación de un listado (defensa anti-bucle).
const maxPages = 5000

// getJSON hace un GET autenticado por rt.HTTP —el ÚNICO cliente de egress de la
// pasarela: SSRF, rate-limit, auditoría y política— y decodifica la respuesta.
func getJSON(ctx context.Context, rt connector.Runtime, rawURL, token string, dst any) error {
	return getJSONWith(ctx, rt, rawURL, token, nil, dst)
}

// getJSONWith es getJSON con cabeceras propias del proveedor (GitHub exige fijar
// la versión de su API REST; sin ella sirve «la última», que cambia bajo los pies
// de una integración que debe seguir funcionando dentro de dos años).
func getJSONWith(ctx context.Context, rt connector.Runtime, rawURL, token string, hdr http.Header, dst any) error {
	resp, err := openStreamWith(ctx, rt, rawURL, token, hdr)
	if err != nil {
		return err
	}
	defer resp.Close()
	body, err := io.ReadAll(io.LimitReader(resp, maxJSONBytes+1))
	if err != nil {
		return fmt.Errorf("leer respuesta: %w", err)
	}
	if len(body) > maxJSONBytes {
		return fmt.Errorf("respuesta demasiado grande (> %d bytes)", maxJSONBytes)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("la respuesta no es JSON: %w", err)
	}
	return nil
}

// openStream emite el GET y devuelve el CUERPO abierto si el status es 2xx (el
// llamante cierra). token vacío = sin Authorization: así se usan las URLs
// pre-autenticadas de descarga que emite Graph, que no admiten cabecera propia.
//
// «Sin Authorization» tiene que serlo de verdad: el egress de la pasarela añade el
// Bearer de la instancia a toda petición que no lleve uno, y estas URL las devuelve
// el tercero y viven en otro host. Sin marcarlas, el token guardado de la conexión
// viajaba a donde dijera la respuesta.
func openStream(ctx context.Context, rt connector.Runtime, rawURL, token string) (io.ReadCloser, error) {
	if token == "" {
		ctx = policy.SinCredencial(ctx)
	}
	return openStreamWith(ctx, rt, rawURL, token, nil)
}

// openStreamWith es openStream con cabeceras propias del proveedor (ver getJSONWith).
func openStreamWith(ctx context.Context, rt connector.Runtime, rawURL, token string, hdr http.Header) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	for k, vs := range hdr {
		req.Header[http.CanonicalHeaderKey(k)] = vs
	}
	req.Header.Set("User-Agent", "Sytue-Integration-Gateway/1")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := rt.HTTP.Do(req)
	if err != nil {
		// Preserva ssrfhttp.ErrBlocked y los errores de transporte para errors.Is.
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("el origen respondió HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return resp.Body, nil
}

// sameHostNext resuelve un enlace de página siguiente (absoluto o relativo) y
// exige que NO cambie de host: un salto a otro destino filtraría el token.
func sameHostNext(current, next string) (string, error) {
	nu, err := url.Parse(strings.TrimSpace(next))
	if err != nil {
		return "", fmt.Errorf("enlace de paginación inválido: %w", err)
	}
	cu, err := url.Parse(current)
	if err != nil {
		return "", fmt.Errorf("URL de página inválida: %w", err)
	}
	if !nu.IsAbs() {
		nu = cu.ResolveReference(nu)
	}
	if nu.Scheme != "http" && nu.Scheme != "https" {
		return "", fmt.Errorf("enlace de paginación con esquema no permitido")
	}
	if !strings.EqualFold(nu.Hostname(), cu.Hostname()) {
		return "", fmt.Errorf("el enlace de paginación cambia de host (%s → %s): bloqueado", cu.Hostname(), nu.Hostname())
	}
	return nu.String(), nil
}

// hostsOficialesGoogle y hostsOficialesM365 coinciden con los declarados en las
// semillas del catálogo (google-workspace.json y microsoft365.json).
var (
	hostsOficialesGoogle = []string{
		"www.googleapis.com",
		"people.googleapis.com",
		"gmail.googleapis.com",
		"admin.googleapis.com",
		"oauth2.googleapis.com",
		"accounts.google.com",
		"*.googleapis.com",
	}
	hostsOficialesM365 = []string{
		"graph.microsoft.com",
		"login.microsoftonline.com",
		"*.sharepoint.com",
		"*.1drv.com",
	}
	hostsOficialesGitHub = []string{
		"api.github.com",
		"github.com",
		"*.githubusercontent.com",
	}
)

func esLoopbackDePrueba(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func validarHostOficial(rawURL string, oficiales []string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return fmt.Errorf("%w: %q no es una URL http(s) válida", connector.ErrBadQuery, rawURL)
	}
	if esLoopbackDePrueba(u.Hostname()) || manifest.HostPermitido(u.Hostname(), oficiales) {
		return nil
	}
	return fmt.Errorf("%w: el servidor %q no está entre los hosts oficiales declarados en el manifiesto del proveedor", connector.ErrBadQuery, u.Hostname())
}

// baseURLOr valida y normaliza (sin barra final) una raíz de API configurable —
// existe para poder apuntar a los servidores simulados de los tests sin abrir la
// salida hacia hosts ajenos al manifiesto del proveedor.
func baseURLOr(raw, def string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def, nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%w: %q no es una URL http(s) válida", connector.ErrBadQuery, raw)
	}
	oficiales := hostsOficialesGoogle
	if strings.Contains(def, "github.com") {
		oficiales = hostsOficialesGitHub
	} else if strings.Contains(def, "microsoft") {
		oficiales = hostsOficialesM365
	}
	if err := validarHostOficial(raw, oficiales); err != nil {
		return "", err
	}
	return strings.TrimRight(raw, "/"), nil
}

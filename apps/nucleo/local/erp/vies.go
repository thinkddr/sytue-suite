// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
)

// # Comprobar un NIF-IVA en VIES
//
// La IA propone un cliente o un proveedor (o la persona lo escribe) y su NIF-IVA puede
// estar mal. VIES (VAT Information Exchange System) es el servicio OFICIAL de la Comisión
// Europea que dice si un NIF-IVA está dado de alta para operaciones dentro de la UE y, si
// su país lo publica, a nombre de quién y con qué dirección.
//
// NADA es automático: se consulta solo cuando la persona pulsa «Comprobar en VIES» (en la
// ficha del contacto o en la propuesta de Sytue AI), sale solo el código del país y el
// número, y lo contestado se enseña al lado de lo propuesto sin cambiar ningún campo. El
// cliente HTTP lo pone la app: en Sytue Office, el permiso puntual del portero de red con
// el propósito `vies` (`red.Portero.Puntual`), que solo abre el destino mientras dura la
// consulta. Sin cliente no sale nada.

// DireccionVIES es la API REST pública de VIES.
const DireccionVIES = "https://ec.europa.eu/taxation_customs/vies/rest-api/check-vat-number"

// Los fallos de una comprobación que la pantalla dice con su texto.
var (
	// ErrNoEsNIFIVA: no tiene forma de NIF-IVA de la UE (sin prefijo de un Estado
	// miembro ni país del contacto que lo dé, o con caracteres que no caben), o VIES lo
	// rechazó por eso (INVALID_INPUT).
	ErrNoEsNIFIVA = errors.New("erp: no es un NIF-IVA de la UE")
	// ErrVIESNoResponde: VIES, o el servicio del Estado miembro detrás, no contesta ahora
	// (MS_UNAVAILABLE, SERVICE_UNAVAILABLE, TIMEOUT, demasiadas consultas…).
	ErrVIESNoResponde = errors.New("erp: VIES no responde")
)

// ComprobacionVIES es lo que contesta VIES de un NIF-IVA.
type ComprobacionVIES struct {
	// Pais y Numero son lo preguntado: «ES» y «B12345678» (Grecia es «EL»).
	Pais, Numero string
	// Valido: dado de alta para operaciones dentro de la UE.
	Valido bool
	// Nombre y Direccion son los registrados, en una línea; vacíos si el país no los
	// publica (España, Alemania… contestan «---»).
	Nombre, Direccion string
	// Fecha es cuándo contestó VIES.
	Fecha time.Time
}

// VIES consulta el servicio.
type VIES struct {
	// HTTP es el cliente; nil no sale (no hay cliente por defecto: la red es del portero).
	HTTP *http.Client
	// URL es la de la API; vacía, DireccionVIES. Las pruebas ponen la de un VIES falso.
	URL string
}

// prefijosVIES son los de VIES: los de los Estados de la UE (Grecia es EL) más XI, Irlanda
// del Norte, que sigue en VIES para mercancías.
func esPrefijoVIES(p string) bool {
	switch p {
	case "EL", "XI":
		return true
	case "GR":
		return false
	}
	return dominio.MiembroUE(p)
}

// PartirNIFIVA separa un NIF-IVA en el prefijo de VIES y el número, sin espacios, puntos
// ni guiones: «ES B-12345678» da («ES», «B12345678»). Sin prefijo, lo pone el país del
// contacto (ISO 3166-1 alfa-2: «ES», «GR»…), si es de la UE.
func PartirNIFIVA(nif, pais string) (prefijo, numero string, err error) {
	var b strings.Builder
	for _, r := range strings.ToUpper(nif) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '+', r == '*':
			b.WriteRune(r)
		case r == ' ', r == '-', r == '.', r == '/', r == '\t':
		default:
			return "", "", fmt.Errorf("%w: %q", ErrNoEsNIFIVA, nif)
		}
	}
	limpio := b.String()
	if len(limpio) > 2 && esLetra(limpio[0]) && esLetra(limpio[1]) && esPrefijoVIES(limpio[:2]) {
		prefijo, numero = limpio[:2], limpio[2:]
	} else {
		pais = strings.ToUpper(strings.TrimSpace(pais))
		switch {
		case pais == "GR":
			prefijo = "EL"
		case esPrefijoVIES(pais):
			prefijo = pais
		default:
			return "", "", fmt.Errorf("%w: %q sin prefijo ni país de la UE", ErrNoEsNIFIVA, nif)
		}
		numero = limpio
	}
	// El patrón de VIES es [0-9A-Za-z+*.]{2,12}.
	if len(numero) < 2 || len(numero) > 12 {
		return "", "", fmt.Errorf("%w: %q", ErrNoEsNIFIVA, nif)
	}
	return prefijo, numero, nil
}

func esLetra(c byte) bool { return c >= 'A' && c <= 'Z' }

// Comprobar pregunta a VIES por nif (con el país del contacto para el que no lleve
// prefijo). Solo sale el prefijo y el número.
func (v VIES) Comprobar(ctx context.Context, nif, pais string) (ComprobacionVIES, error) {
	prefijo, numero, err := PartirNIFIVA(nif, pais)
	if err != nil {
		return ComprobacionVIES{}, err
	}
	if v.HTTP == nil {
		return ComprobacionVIES{}, errors.New("erp: VIES sin cliente de red")
	}
	cuerpo, _ := json.Marshal(map[string]string{"countryCode": prefijo, "vatNumber": numero})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cmp.Or(v.URL, DireccionVIES), bytes.NewReader(cuerpo))
	if err != nil {
		return ComprobacionVIES{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := v.HTTP.Do(req)
	if err != nil {
		return ComprobacionVIES{}, fmt.Errorf("erp: VIES: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return ComprobacionVIES{}, fmt.Errorf("erp: VIES: %w", err)
	}
	// Los nombres son los de la respuesta de la API REST de VIES de la Comisión Europea,
	// no los nuestros: se leen tal cual.
	var r struct {
		Valid         *bool  `json:"valid"`
		RequestDate   string `json:"requestDate"` //nolint:tagliatelle // contrato de la API de VIES
		Name          string `json:"name"`
		Address       string `json:"address"`
		ActionSucceed *bool  `json:"actionSucceed"` //nolint:tagliatelle // contrato de la API de VIES
		ErrorWrappers []struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		} `json:"errorWrappers"` //nolint:tagliatelle // contrato de la API de VIES
	}
	errJSON := json.Unmarshal(raw, &r)
	if len(r.ErrorWrappers) > 0 || (r.ActionSucceed != nil && !*r.ActionSucceed) {
		codigo := ""
		if len(r.ErrorWrappers) > 0 {
			codigo = r.ErrorWrappers[0].Error
		}
		if codigo == "INVALID_INPUT" {
			return ComprobacionVIES{}, fmt.Errorf("%w: VIES contesta %s", ErrNoEsNIFIVA, codigo)
		}
		return ComprobacionVIES{}, fmt.Errorf("%w: %s", ErrVIESNoResponde, cmp.Or(codigo, resp.Status))
	}
	if resp.StatusCode != http.StatusOK || errJSON != nil || r.Valid == nil {
		return ComprobacionVIES{}, fmt.Errorf("%w: %s", ErrVIESNoResponde, resp.Status)
	}
	c := ComprobacionVIES{
		Pais: prefijo, Numero: numero, Valido: *r.Valid,
		Nombre: limpiarVIES(r.Name), Direccion: limpiarVIES(r.Address),
	}
	c.Fecha, _ = time.Parse(time.RFC3339, r.RequestDate)
	return c, nil
}

// limpiarVIES deja un dato de VIES en una línea: «---» (el país no lo publica) es vacío,
// y los saltos de línea de la dirección, comas.
func limpiarVIES(s string) string {
	var partes []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			partes = append(partes, l)
		}
	}
	s = strings.Join(partes, ", ")
	if strings.Trim(s, "- ") == "" {
		return ""
	}
	return s
}

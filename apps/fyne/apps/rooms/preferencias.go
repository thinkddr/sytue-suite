// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"encoding/json"
	"slices"
	"time"
)

// Lo que la web guarda en `localStorage` y aquí va al almacén POR PERSONA de la sesión
// (`sesion.PorPersona`): las secciones por defecto plegadas, si la barra de formato se
// ve y cuándo caduca el estado personalizado. Las claves son las de la web.
const (
	ClavePlegadas = "sytue-rooms-collapsed-sections"
	ClaveFormato  = "sytue-rooms-formato"
	ClaveEstado   = "sytue-rooms:custom-status"
)

// Almacen es donde se guardan; lo cumple `sesion.PorPersona`. Sin él, viven lo que dure
// la ventana.
type Almacen interface {
	Leer(clave string) string
	Poner(clave, valor string)
}

// estadoGuardado es `StoredStatus`: el estado y su caducidad en milisegundos, o null.
type estadoGuardado struct {
	Status    string `json:"status"`
	ExpiresAt *int64 `json:"expiresAt"` //nolint:tagliatelle // Clave persistida compartida con la web.
}

// restaurarPreferencias lee lo guardado al arrancar: las secciones plegadas, la barra de
// formato y la caducidad del estado.
func (sa *Sala) restaurarPreferencias() {
	if sa.Almacen == nil {
		return
	}
	var plegadas []string
	if json.Unmarshal([]byte(sa.Almacen.Leer(ClavePlegadas)), &plegadas) == nil && len(plegadas) > 0 {
		l := sa.P.Lateral
		l.Plegadas = map[string]bool{}
		for _, k := range plegadas {
			l.Plegadas[k] = true
		}
	}
	if sa.Almacen.Leer(ClaveFormato) == "1" {
		sa.P.Canal.Compositor.FormatoVisible = true
		sa.P.Canal.Compositor.Refresh()
	}
	sa.restaurarCaducidad()
}

// guardarPlegadas guarda las secciones por defecto plegadas, en orden estable.
func (sa *Sala) guardarPlegadas() {
	if sa.Almacen == nil {
		return
	}
	var ks []string
	for k, si := range sa.P.Lateral.Plegadas {
		if si {
			ks = append(ks, k)
		}
	}
	slices.Sort(ks)
	datos, _ := json.Marshal(ks)
	if len(ks) == 0 {
		datos = nil
	}
	sa.Almacen.Poner(ClavePlegadas, string(datos))
}

// guardarFormato es el `localStorage.setItem(FORMATO_KEY, …)` del «Aa».
func (sa *Sala) guardarFormato(visible bool) {
	if sa.Almacen == nil {
		return
	}
	v := "0"
	if visible {
		v = "1"
	}
	sa.Almacen.Poner(ClaveFormato, v)
}

// guardarEstado es `saveCustomStatus`: el estado con su caducidad (el cero, que no
// caduca); sin estado se borra.
func (sa *Sala) guardarEstado(texto string, caduca time.Time) {
	if sa.Almacen == nil {
		return
	}
	if texto == "" {
		sa.Almacen.Poner(ClaveEstado, "")
		return
	}
	g := estadoGuardado{Status: texto}
	if !caduca.IsZero() {
		ms := caduca.UnixMilli()
		g.ExpiresAt = &ms
	}
	datos, _ := json.Marshal(g)
	sa.Almacen.Poner(ClaveEstado, string(datos))
}

// caducidadGuardada es `storedExpiry`: cuándo caduca el estado guardado, o el cero.
func (sa *Sala) caducidadGuardada() (string, time.Time) {
	if sa.Almacen == nil {
		return "", time.Time{}
	}
	var g estadoGuardado
	if json.Unmarshal([]byte(sa.Almacen.Leer(ClaveEstado)), &g) != nil || g.ExpiresAt == nil {
		return g.Status, time.Time{}
	}
	return g.Status, time.UnixMilli(*g.ExpiresAt)
}

// restaurarCaducidad es `loadCustomStatus` y el temporizador de la web: si el estado que
// se tiene es el guardado y ya venció, se borra; si no, se programa su borrado.
func (sa *Sala) restaurarCaducidad() {
	l := sa.P.Lateral
	texto, caduca := sa.caducidadGuardada()
	if texto == "" || caduca.IsZero() || texto != l.EstadoPersonal {
		return
	}
	if !caduca.After(sa.ahora()) {
		sa.CambiarPresencia(presenciaAPI(l.Presencia), "", true, time.Time{})
		return
	}
	if sa.pararCaducidad != nil {
		sa.pararCaducidad()
	}
	sa.pararCaducidad = sa.Esperar(caduca.Sub(sa.ahora()), func() {
		sa.pararCaducidad = nil
		sa.CambiarPresencia(presenciaAPI(sa.P.Lateral.Presencia), "", true, time.Time{})
	})
}

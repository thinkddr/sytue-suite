// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"bytes"
	"encoding/json"
	"slices"
	"time"
)

// La protección de Diapos, la parte PURA: la misma regla que `slides/proteger.ts` de la
// web y que cumple el servicio (`domain/proteccion_diapos.go`). La impone el servidor;
// lo de aquí evita mandar un cambio que va a rechazar y, sobre todo, deja abrir en
// edición una presentación protegida (antes `Fiel` no conocía `protect` y la dejaba en
// solo lectura, también a su dueño). Es la MISMA regla escrita aparte: si cambia una,
// cambia la otra, y los casos de `vectores/diapos-proteccion.json` los exigen las dos.
//
//   - `slides`: contenido entero fijo, y la diapositiva no se borra (sí se mueve).
//   - `boxes`: la caja queda como está y en su diapositiva.
//   - `master`: patrón, diseños, tema, temas propios y tamaño del lienzo fijos.
//   - Cambiar `protect` (o quitarlo) es también cambiar la protección.
//
// Gestionar la protección (bloquear, contraseña) es de la web: aquí no hay panel.

// Proteccion es `SlidesProtection`. Los dos booleanos van por puntero por lo mismo que
// los de `Caja`: un `false` explícito se perdería con `omitempty` y `Fiel` tumbaría el
// documento sin motivo.
//
//nolint:tagliatelle // las claves son las de `SlidesProtection` en la web
type Proteccion struct {
	// TieneClave lo fija el servidor (hay contraseña): no se reescribe ni cuenta al
	// comparar.
	TieneClave *bool    `json:"hasPassword,omitempty"`
	Diapos     []string `json:"slides,omitempty"`
	Cajas      []string `json:"boxes,omitempty"`
	Marco      *bool    `json:"master,omitempty"`
}

func verdad(b *bool) bool { return b != nil && *b }

// DiapoBloqueada es `diapoBloqueada`.
func (m *Modelo) DiapoBloqueada(id string) bool {
	return m.Proteccion != nil && slices.Contains(m.Proteccion.Diapos, id)
}

// CajaBloqueada es `cajaBloqueada`.
func (m *Modelo) CajaBloqueada(id string) bool {
	return m.Proteccion != nil && slices.Contains(m.Proteccion.Cajas, id)
}

// MarcoBloqueado dice si `master` fija patrón, diseños, tema y tamaño.
func (m *Modelo) MarcoBloqueado() bool {
	return m.Proteccion != nil && verdad(m.Proteccion.Marco)
}

// mismoJSON compara dos valores por su serialización. Los tipos son structs, así que el
// orden de las claves lo fija el tipo y no hace falta canonizar (en la web, `canonico`).
func mismoJSON(a, b any) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}

// sinClave es `protect` sin `hasPassword`, que lo fija el servidor.
func sinClave(p *Proteccion) Proteccion {
	q := *p
	q.TieneClave = nil
	return q
}

// marco es lo que `master` fija, comparable.
func marco(m *Modelo) any {
	return struct {
		Patron *Patron      `json:"master"`
		Tema   string       `json:"theme"`
		Temas  []TemaPropio `json:"themes"`
		Tamano *Tamano      `json:"size"`
	}{m.Patron, m.Tema, m.Temas, m.Tamano}
}

type cajaEn struct {
	diapo string
	caja  *Caja
}

func cajasPorID(m *Modelo) map[string][]cajaEn {
	out := map[string][]cajaEn{}
	for i := range m.Diapos {
		d := &m.Diapos[i]
		for j := range d.Cajas {
			out[d.Cajas[j].ID] = append(out[d.Cajas[j].ID], cajaEn{d.ID, &d.Cajas[j]})
		}
	}
	return out
}

// CambioProtegido es `cambioProtegido`: ¿el paso de `antes` a `despues` toca algo que la
// protección fija? Falso si `antes` no está protegido (proteger es de cualquiera).
func CambioProtegido(antes, despues *Modelo) bool {
	p := antes.Proteccion
	if p == nil {
		return false
	}
	if despues.Proteccion == nil || !mismoJSON(sinClave(p), sinClave(despues.Proteccion)) {
		return true
	}
	nuevas := map[string][]*Diapo{}
	for i := range despues.Diapos {
		d := &despues.Diapos[i]
		nuevas[d.ID] = append(nuevas[d.ID], d)
	}
	for _, id := range p.Diapos {
		i := slices.IndexFunc(antes.Diapos, func(d Diapo) bool { return d.ID == id })
		if i < 0 {
			continue // no hay nada que fijar
		}
		ahora := nuevas[id]
		if len(ahora) != 1 || !mismoJSON(ahora[0], &antes.Diapos[i]) {
			return true
		}
	}
	antesCajas, ahoraCajas := cajasPorID(antes), cajasPorID(despues)
	for _, id := range p.Cajas {
		a := antesCajas[id]
		if len(a) != 1 {
			continue // un id repetido no se puede fijar (como el servidor)
		}
		b := ahoraCajas[id]
		if len(b) != 1 || b[0].diapo != a[0].diapo || !mismoJSON(b[0].caja, a[0].caja) {
			return true
		}
	}
	return verdad(p.Marco) && !mismoJSON(marco(antes), marco(despues))
}

// clonar es una copia profunda por JSON: el modelo es JSON puro y `Fiel` garantiza que
// el viaje no pierde nada.
func (m *Modelo) clonar() *Modelo {
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	var c Modelo
	if json.Unmarshal(b, &c) != nil {
		return nil
	}
	return &c
}

// ---- en el editor ----

// esperaAviso es la de `avisoProtegidoPermitido`: un arrastre o un teclear insistente
// intentan el cambio una y otra vez, y el aviso sale una vez, no en cada paso.
const esperaAviso = 1500 * time.Millisecond

// vigilando dice si hay algo bloqueado que cuidar: presentación protegida y quien edita
// no es su dueño.
func (e *Editor) vigilando() bool { return !e.Propietario && e.m.Proteccion != nil }

// anotarBase guarda el modelo actual como el último aceptado, o lo suelta si no hay
// nada que vigilar.
func (e *Editor) anotarBase() {
	e.base = nil
	if e.vigilando() {
		e.base = e.m.clonar()
	}
}

// vetado es la comprobación de `apply` en la web, tras aplicar el cambio: si toca lo
// bloqueado, el modelo vuelve a como estaba (también lo que un arrastre movió en vivo),
// la diapositiva activa con él, y se avisa. `activa` es la de antes del cambio.
func (e *Editor) vetado(activa int) bool {
	if e.base == nil || e.Propietario || !CambioProtegido(e.base, e.m) {
		e.anotarBase()
		return false
	}
	if c := e.base.clonar(); c != nil {
		*e.m = *c // en el sitio: el lienzo y quien guarda miran este mismo puntero
	}
	e.activa = max(0, min(activa, len(e.m.Diapos)-1))
	e.recomponer()
	e.avisarProtegido()
	return true
}

// avisarProtegido es el aviso de la web (`slides.msg.protected`), espaciado.
func (e *Editor) avisarProtegido() {
	if ahora := time.Now(); ahora.Sub(e.ultimoAviso) > esperaAviso {
		e.ultimoAviso = ahora
		e.avisar("slides.msg.protected", "sheet.msg.protectionDenied")
	}
}

// diapoFija dice si la diapositiva activa está bloqueada para quien edita. En el
// patrón no cuenta: lo que se edita entonces es el marco.
func (e *Editor) diapoFija() bool {
	return !e.Propietario && !e.patron && e.m.valida(e.activa) && e.m.DiapoBloqueada(e.m.Diapos[e.activa].ID)
}

// cajaFija dice si la caja `id` de lo que se edita está bloqueada para quien edita: por
// ella misma, por su diapositiva o, en el patrón, por `master`.
func (e *Editor) cajaFija(id string) bool {
	if e.Propietario {
		return false
	}
	if e.patron {
		return e.m.MarcoBloqueado()
	}
	return e.diapoFija() || e.m.CajaBloqueada(id)
}

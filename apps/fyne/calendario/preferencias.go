// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"encoding/json"
	"slices"
)

// Lo que la web guarda en `localStorage` con el `id` de la persona en la clave
// (apps/calendar/src/lib/preferencias.ts) y aquí va al almacén POR PERSONA de la sesión
// (`sesion.PorPersona`), que ya pone a la persona: la vista con la que se quedó y los
// calendarios que ocultó. Se guardan los OCULTOS y no los visibles para que un
// calendario nuevo, o uno recién compartido, aparezca encendido.
const (
	ClaveVista   = "sytue.calendar.vista"
	ClaveOcultos = "sytue.calendar.ocultos"
)

// maxOcultos acota los guardados: los de calendarios borrados no deben crecer sin fin.
const maxOcultos = 200

// Almacen es donde se guardan; lo cumple `sesion.PorPersona`. Sin él, la vista y los
// visibles viven lo que dure la ventana, como siempre.
type Almacen interface {
	Leer(clave string) string
	Poner(clave, valor string)
}

// nombresVista son los valores de la web (`ViewMode`), para que lo guardado diga lo mismo
// en las dos plataformas.
var nombresVista = map[Vista]string{
	VistaMes: "month", VistaSemana: "week", VistaDia: "day", VistaAgenda: "agenda",
}

// restaurarPreferencias lee lo guardado de quien acaba de entrar: la vista (salvo que un
// enlace profundo ya haya puesto la suya, que manda) y los ocultos, que aplica la
// primera carga de la lista (`encender`).
func (a *App) restaurarPreferencias() {
	fijada := a.vistaDelEnlace
	a.vistaDelEnlace = false
	if a.Almacen == nil {
		return
	}
	if !fijada {
		// Sin nada guardado, el mes: quien entra en una ventana donde otra persona dejó
		// la semana no la hereda (en la web, otra cuenta es otra página).
		a.P.Vista = VistaMes
		guardada := a.Almacen.Leer(ClaveVista)
		for v, n := range nombresVista {
			if n == guardada {
				a.P.Vista = v
			}
		}
	}
	a.ocultos = map[string]bool{}
	var ids []string
	if json.Unmarshal([]byte(a.Almacen.Leer(ClaveOcultos)), &ids) == nil {
		for _, id := range ids {
			a.ocultos[id] = true
		}
	}
	// Otra persona: su lista es otra y su primera carga vuelve a ser la primera.
	a.conocidos = map[string]bool{}
}

// guardarVista recuerda la vista que la persona ha elegido.
func (a *App) guardarVista(v Vista) {
	if a.Almacen != nil {
		a.Almacen.Poner(ClaveVista, nombresVista[v])
	}
}

// guardarOcultos guarda los calendarios conocidos que no se ven, más los ya guardados
// que esta carga no trajo (un compartido que falló al listarse no olvida que estaba
// oculto). Sin ninguno, se borra.
func (a *App) guardarOcultos() {
	if a.Almacen == nil {
		return
	}
	var ids []string
	for id := range a.conocidos {
		if !a.P.visible(id) {
			ids = append(ids, id)
		}
	}
	for id := range a.ocultos {
		if !a.conocidos[id] {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	ids = ids[:min(len(ids), maxOcultos)]
	a.ocultos = map[string]bool{}
	for _, id := range ids {
		a.ocultos[id] = true
	}
	if len(ids) == 0 {
		a.Almacen.Poner(ClaveOcultos, "")
		return
	}
	datos, _ := json.Marshal(ids)
	a.Almacen.Poner(ClaveOcultos, string(datos))
}

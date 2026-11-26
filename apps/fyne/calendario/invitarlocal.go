// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Invitar desde el formulario en la edición sin cuenta (Sytue Office, A-11 y A-13). Con
// SinPersonas el formulario no enseña invitados: no hay servicio de Sytue que los
// avise. Pero un calendario de una CUENTA (CalDAV, Google, Microsoft) sí tiene quien
// mande la invitación —el proveedor, o el correo de la persona—, y en sus eventos
// Pantalla.Invitar vuelve a poner el campo de los invitados, con el autocompletado de la
// libreta de este equipo, una pista de por dónde saldrá la invitación y, en «buscar un
// hueco», la ocupación de los invitados que el proveedor sepa. En un calendario de este
// equipo, en su lugar, una frase dice dónde se puede invitar.
//
// Sin Invitar (nil), todo como hasta ahora; y la edición conectada (sin SinPersonas) no
// lo mira.

// Invitar es lo que la app da para invitar desde el formulario con SinPersonas.
type Invitar struct {
	// En dice si en un calendario se puede invitar (el de una cuenta que deja escribir).
	En func(calendario string) bool
	// Pista, si no es nil, es la frase bajo el campo en un calendario donde se puede
	// invitar: por dónde saldrá la invitación. Vacía, ninguna.
	Pista func(calendario string) string
	// PistaSin va en lugar del campo en un calendario donde no se puede, si hay alguno
	// donde sí («Para invitar, guarda el evento en un calendario de una cuenta»).
	PistaSin string
	// Sugerir es el autocompletado: hasta n direcciones de la libreta para lo escrito.
	Sugerir func(ctx context.Context, q string, n int) ([]SugerenciaInvitado, error)
	// Ocupacion son los ratos ocupados de los invitados entre desde y hasta, por
	// dirección (en minúsculas), según el proveedor del calendario; quien no sabe, fuera
	// del mapa. nil: «buscar un hueco» solo mira la ocupación propia.
	Ocupacion func(ctx context.Context, calendario string, correos []string, desde, hasta time.Time) (map[string][]api.Hueco, error)
	// ResumenOcupacion es la frase del panel de huecos: con la ocupación de quién se ha
	// contado y de quién no se sabe.
	ResumenOcupacion func(con, sin []string) string
}

// SugerenciaInvitado es una dirección de la libreta para el campo de invitados.
type SugerenciaInvitado struct{ Nombre, Correo string }

// MaxSugerenciasInvitado es cuántas se enseñan bajo el campo.
const MaxSugerenciasInvitado = 5

// invitarLocal es Invitar si se usa: con SinPersonas.
func (p *Pantalla) invitarLocal() *Invitar {
	if p.SinPersonas {
		return p.Invitar
	}
	return nil
}

// invitaAqui dice si el formulario enseña los invitados en el calendario elegido: en la
// conectada, siempre; sin personas, solo donde Invitar lo diga.
func (f *formulario) invitaAqui() bool {
	if !f.p.SinPersonas {
		return true
	}
	inv := f.p.Invitar
	return inv != nil && inv.En != nil && inv.En(f.e.CalendarioID)
}

// pistaSinInvitar es la frase de un calendario donde no se puede invitar, si en alguno
// de los del formulario se puede.
func (f *formulario) pistaSinInvitar() string {
	inv := f.p.invitarLocal()
	if inv == nil || inv.PistaSin == "" || inv.En == nil || f.e.SoloLectura || f.invitaAqui() {
		return ""
	}
	for _, c := range f.destinos {
		if inv.En(c.ID) {
			return inv.PistaSin
		}
	}
	return ""
}

// pistaInvitar es la frase bajo el campo: por dónde saldrá la invitación.
func (f *formulario) pistaInvitar() string {
	if inv := f.p.invitarLocal(); inv != nil && inv.Pista != nil && !f.e.SoloLectura {
		return inv.Pista(f.e.CalendarioID)
	}
	return ""
}

// sugerir pide las direcciones de la libreta para lo escrito (con dos letras, como el
// redactor de Mail). La respuesta que llega cuando ya se escribió otra cosa no vale.
func (f *formulario) sugerir(q string) {
	if f.a.Sugerir == nil {
		return
	}
	q = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(q), ",;"))
	f.turnoSug++
	turno := f.turnoSug
	if len([]rune(q)) < 2 {
		if len(f.sugerencias) > 0 {
			f.sugerencias, f.sugActiva = nil, 0
			f.rehacer()
		}
		return
	}
	f.a.Sugerir(q, func(l []SugerenciaInvitado) {
		if turno != f.turnoSug {
			return
		}
		var out []SugerenciaInvitado
		for _, s := range l {
			if s.Correo == "" || slices.ContainsFunc(f.e.Invitados, func(x string) bool { return strings.EqualFold(x, s.Correo) }) {
				continue
			}
			out = append(out, s)
			if len(out) >= MaxSugerenciasInvitado {
				break
			}
		}
		if len(out) == 0 && len(f.sugerencias) == 0 {
			return
		}
		f.sugerencias, f.sugActiva = out, 0
		f.rehacer()
	})
}

// elegirSugerencia añade la dirección y vacía la lista.
func (f *formulario) elegirSugerencia(s SugerenciaInvitado) {
	f.turnoSug++
	f.sugerencias, f.sugActiva = nil, 0
	f.correo.PonerTexto("")
	f.errCorreo, f.correo.Invalida = "", false
	f.anadirInvitado(s.Correo)
	f.rehacer()
}

// teclaSugerencias mueve la señalada con las flechas e Intro la elige. Dice si se quedó
// la tecla.
func (f *formulario) teclaSugerencias(k *fyne.KeyEvent) bool {
	if len(f.sugerencias) == 0 {
		return false
	}
	switch k.Name {
	case fyne.KeyDown:
		f.sugActiva = (f.sugActiva + 1) % len(f.sugerencias)
	case fyne.KeyUp:
		f.sugActiva = (f.sugActiva + len(f.sugerencias) - 1) % len(f.sugerencias)
	case fyne.KeyReturn, fyne.KeyEnter:
		// Una dirección entera escrita a mano se añade tal cual.
		if correoValido.MatchString(strings.TrimSpace(f.correo.Texto())) {
			return false
		}
		f.elegirSugerencia(f.sugerencias[min(f.sugActiva, len(f.sugerencias)-1)])
		return true
	case fyne.KeyEscape:
		f.sugerencias = nil
	default:
		return false
	}
	f.rehacer()
	return true
}

// listaSugerencias es la lista bajo el campo: «Nombre» y la dirección, la señalada con
// el fondo de `bg-subtle` (`border bg-surface-raised rounded-md`).
func (f *formulario) listaSugerencias() fyne.CanvasObject {
	if len(f.sugerencias) == 0 {
		return nil
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ancho := max(f.util()-2*(tema.Borde+espacio1)-2*tema.Espacio(2), 0)
	filas := make([]fyne.CanvasObject, 0, len(f.sugerencias))
	for i, s := range f.sugerencias {
		sug := s
		lineas := []fyne.CanvasObject{ui.NuevoParrafo(s.Correo, tema.TextoSM, ui.Normal, "text", v).A(ancho).Recortado(1)}
		nombre := s.Correo
		if s.Nombre != "" {
			nombre = s.Nombre + " <" + s.Correo + ">"
			lineas = []fyne.CanvasObject{
				ui.NuevoParrafo(s.Nombre, tema.TextoSM, ui.Medio, "text", v).A(ancho).Recortado(1),
				ui.NuevoParrafo(s.Correo, tema.TextoXS, ui.Normal, "text-muted", v).A(ancho).Recortado(1),
			}
		}
		fila := ui.NuevaFilaPulsable(nombre, container.New(ui.Columna{}, lineas...), func() { f.elegirSugerencia(sug) })
		fila.AltoMinimo = tema.Espacio(9)
		if i == f.sugActiva {
			fondo := canvas.NewRectangle(tema.Color("bg-subtle", v))
			fondo.CornerRadius = tema.RadioMD
			filas = append(filas, container.NewStack(fondo, fila))
			continue
		}
		filas = append(filas, fila)
	}
	caja := ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioMD)
	return container.New(&conCaja{caja: caja, X: espacio1, Y: espacio1}, append(caja.Objetos(), container.New(ui.Columna{}, filas...))...)
}

// RestarOcupados quita de los huecos libres los ratos ocupados de los invitados: lo que
// queda de cada hueco en el que aún cabe un evento de dur.
func RestarOcupados(libres, ocupados []api.Hueco, dur time.Duration) []api.Hueco {
	if len(ocupados) == 0 {
		return libres
	}
	ocupados = slices.Clone(ocupados)
	slices.SortFunc(ocupados, func(a, b api.Hueco) int { return a.Start.Compare(b.Start) })
	var out []api.Hueco
	for _, h := range libres {
		desde := h.Start
		for _, o := range ocupados {
			if !o.End.After(desde) || !o.Start.Before(h.End) {
				continue
			}
			if o.Start.After(desde) && o.Start.Sub(desde) >= dur {
				out = append(out, api.Hueco{Start: desde, End: o.Start})
			}
			if o.End.After(desde) {
				desde = o.End
			}
		}
		if h.End.Sub(desde) >= dur {
			out = append(out, api.Hueco{Start: desde, End: h.End})
		}
	}
	return out
}

// pedirOcupacion pide la ocupación de los invitados y, con ella, quita de los huecos
// libres los ratos en que alguno está ocupado. Quien el proveedor no conoce va en
// h.sin; si la consulta entera falla, todos.
func (f *formulario) pedirOcupacion(libres []api.Hueco, desde, hasta time.Time, minutos, turno int) {
	h := f.bh
	invitados := slices.Clone(f.e.Invitados)
	f.a.Ocupacion(f.e.CalendarioID, invitados, desde, hasta, func(m map[string][]api.Hueco, err error) {
		if turno != h.turno {
			return
		}
		h.cargado()
		h.con, h.sin = nil, nil
		var ocupados []api.Hueco
		for _, c := range invitados {
			t, ok := m[strings.ToLower(strings.TrimSpace(c))]
			if err != nil || !ok {
				h.sin = append(h.sin, c)
				continue
			}
			h.con = append(h.con, c)
			ocupados = append(ocupados, t...)
		}
		h.fallo, h.lista = false, RestarOcupados(libres, ocupados, time.Duration(minutos)*time.Minute)
		f.rehacer()
	})
}

// invitadosCambiaron vuelve a buscar huecos, si el panel está abierto, con los
// invitados de ahora (sin personas con Invitar: en la conectada la ocupación es la
// propia y no cambia).
func (f *formulario) invitadosCambiaron() {
	if f.p.invitarLocal() != nil && f.bh != nil && f.bh.abierto {
		f.pedirHuecos()
	}
}

// conOcupacionDeInvitados dice si «buscar un hueco» pregunta por los invitados.
func (f *formulario) conOcupacionDeInvitados() bool {
	return f.a.Ocupacion != nil && f.p.invitarLocal() != nil && f.invitaAqui() && len(f.e.Invitados) > 0
}

// resumenOcupacion es la frase del panel con quién se ha contado.
func (f *formulario) resumenOcupacion() string {
	inv := f.p.invitarLocal()
	if inv == nil || inv.ResumenOcupacion == nil || f.bh == nil || len(f.bh.con)+len(f.bh.sin) == 0 {
		return ""
	}
	return inv.ResumenOcupacion(f.bh.con, f.bh.sin)
}

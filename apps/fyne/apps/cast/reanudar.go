// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"cmp"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/video"
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

// Seguir donde se dejó un vídeo bajo demanda: la regla de lib/reanudar.ts de la web,
// con el mismo formato.
//
// Es una comodidad de cada dispositivo, no un dato de la cuenta: va en la carpeta de la
// suite (`ajustes.Carpeta`), separada por persona (quien comparte el ordenador no
// hereda lo que vio otra), y no se sincroniza; eso sería un dato del servicio. Sin
// sesión no se recuerda nada.
//
//   - Por debajo de 10 s no se guarda: es haber abierto el vídeo, no haberlo empezado.
//   - Desde el 95 % se da por visto y se olvida.
//   - Lo que lleva 90 días sin tocarse caduca, y se recuerdan como mucho 200 vídeos.
const (
	ReanudarUmbral    = 10 * time.Second
	ReanudarMaxFrac   = 0.95
	ReanudarCada      = 5 * time.Second
	ReanudarCaducidad = 90 * 24 * time.Hour
	ReanudarMax       = 200
)

// Punto es dónde se quedó un vídeo: segundo, duración (s) y cuándo se guardó (ms de
// época), como en la web.
type Punto struct {
	S  float64 `json:"s"`
	T  float64 `json:"t"`
	En int64   `json:"en"`
}

// Puntos son los de una persona, por id de vídeo.
type Puntos map[string]Punto

// Merece dice si una posición merece recordarse: pasados 10 s y sin llegar al 95 %.
func Merece(pos, total time.Duration) bool {
	return total > 0 && pos >= ReanudarUmbral && float64(pos)/float64(total) < ReanudarMaxFrac
}

func segundos(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

// Anotar devuelve los puntos con la posición de ese vídeo: si merece, se guarda; si no
// (se acaba de abrir, o ya está casi visto), se olvida la que hubiera. Y se poda.
func (p Puntos) Anotar(id string, pos, total time.Duration, ahora time.Time) Puntos {
	nuevos := maps.Clone(p)
	if nuevos == nil {
		nuevos = Puntos{}
	}
	if Merece(pos, total) {
		nuevos[id] = Punto{S: pos.Seconds(), T: total.Seconds(), En: ahora.UnixMilli()}
	} else {
		delete(nuevos, id)
	}
	return nuevos.Podar(ahora)
}

// Podar quita los caducados y los que no caben (se quedan los más recientes).
func (p Puntos) Podar(ahora time.Time) Puntos {
	ids := make([]string, 0, len(p))
	for id, x := range p {
		if ahora.UnixMilli()-x.En < ReanudarCaducidad.Milliseconds() {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b string) int { return cmp.Compare(p[b].En, p[a].En) })
	out := make(Puntos, min(len(ids), ReanudarMax))
	for _, id := range ids[:min(len(ids), ReanudarMax)] {
		out[id] = p[id]
	}
	return out
}

// Desde es dónde seguir ese vídeo; cero si no hay que seguir.
func (p Puntos) Desde(id string, ahora time.Time) time.Duration {
	x, ok := p[id]
	if !ok || ahora.UnixMilli()-x.En >= ReanudarCaducidad.Milliseconds() || !Merece(segundos(x.S), segundos(x.T)) {
		return 0
	}
	return segundos(x.S)
}

// Visto es lo visto de ese vídeo (de 0 a 1) para la barra de la tarjeta; cero, nada.
func (p Puntos) Visto(id string, ahora time.Time) float64 {
	if p.Desde(id, ahora) == 0 {
		return 0
	}
	return p[id].S / p[id].T
}

// ---- El almacén ----

// carpetaReanudar es dónde va; las pruebas la cambian por una temporal.
var carpetaReanudar = ajustes.Carpeta

const ficheroReanudar = "cast-reanudar.json"

// leerTodos son los puntos de todas las personas del fichero; vacío si no se puede.
func leerTodos() map[string]Puntos {
	dir, err := carpetaReanudar()
	if err != nil {
		return map[string]Puntos{}
	}
	datos, err := os.ReadFile(filepath.Join(dir, ficheroReanudar))
	var todos map[string]Puntos
	if err != nil || json.Unmarshal(datos, &todos) != nil || todos == nil {
		return map[string]Puntos{}
	}
	return todos
}

// LeerPuntos son los de esa persona; vacíos sin persona o sin fichero legible.
func LeerPuntos(persona string) Puntos {
	if persona == "" {
		return Puntos{}
	}
	return leerTodos()[persona].Podar(time.Now())
}

// GuardarPuntos guarda los de esa persona sin tocar los de las demás. Si no se puede
// escribir, no pasa nada: el recuerdo es una comodidad, no un dato.
func GuardarPuntos(persona string, p Puntos) {
	if persona == "" {
		return
	}
	dir, err := carpetaReanudar()
	if err != nil {
		return
	}
	todos := leerTodos()
	if len(p) == 0 {
		delete(todos, persona)
	} else {
		todos[persona] = p
	}
	datos, err := json.Marshal(todos)
	if err != nil {
		return
	}
	// Al lado y renombrado: otra ventana nunca lee medio fichero. Con 0600 (CreateTemp):
	// lo que alguien ha visto es suyo.
	tmp, err := os.CreateTemp(dir, ficheroReanudar+".*")
	if err != nil {
		return
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_, err = tmp.Write(datos)
	if cerr := tmp.Close(); err != nil || cerr != nil {
		return
	}
	_ = os.Rename(tmp.Name(), filepath.Join(dir, ficheroReanudar))
}

// ---- En la app ----

// persona es el `sub` de quien está dentro; "" sin sesión, que no recuerda nada.
func (a *App) persona() string {
	if a.Yo == nil || !a.dentro() {
		return ""
	}
	return a.Yo()
}

// misPuntos son los de quien está dentro, leídos una vez por persona.
func (a *App) misPuntos() Puntos {
	yo := a.persona()
	if yo == "" {
		return nil
	}
	if a.puntos == nil || a.puntosDe != yo {
		a.puntos, a.puntosDe = LeerPuntos(yo), yo
	}
	return a.puntos
}

// visto es lo visto de un vídeo, para la barra de su tarjeta.
func (a *App) visto(id string) float64 { return a.misPuntos().Visto(id, time.Now()) }

// prepararReanudo, al abrir el reproductor de la página de un vídeo bajo demanda,
// apunta qué vídeo se recuerda y devuelve dónde seguirlo (cero: desde el principio).
// Solo la primera vez: cambiar de calidad no vuelve atrás, y la gestión del póster
// (que usa el mismo reproductor) ni recuerda ni sigue.
func (a *App) prepararReanudo() time.Duration {
	vid := a.ver.video
	if a.ruta != RutaVer || vid.ID == "" || vid.EnDirecto() || a.persona() == "" {
		return 0
	}
	a.reanudo, a.guardadoEn = vid.ID, time.Now()
	if a.ver.reanudado {
		return 0
	}
	a.ver.reanudado = true
	a.ver.desde = a.misPuntos().Desde(vid.ID, time.Now())
	return a.ver.desde
}

// ofrecerInicio pone en el visor «Empezar desde el principio», que deshace el salto.
func (a *App) ofrecerInicio(r *video.Reproductor) {
	if a.visor == nil {
		return
	}
	a.visor.AlDesdeInicio = func() {
		a.visor.AlDesdeInicio = nil
		r.Buscar(0)
		// Volver al principio es olvidar dónde se estaba.
		a.anotarPosicion(true)
		r.Reproducir()
		a.visor.Rehacer()
	}
	a.visor.Rehacer()
}

// anotarPosicion guarda dónde va el vídeo de la página: cada 5 s mientras suena y,
// con `forzar`, al salir. Visto entero, se olvida.
func (a *App) anotarPosicion(forzar bool) {
	r := a.reproductor
	if a.reanudo == "" || r == nil {
		return
	}
	pos, total := r.Posicion(), r.Duracion()
	if r.Terminado() {
		pos = total
	}
	if x := a.visor; x != nil && x.AlDesdeInicio != nil && pos > a.ver.desde+15*time.Second {
		// El ofrecimiento de volver al principio sobra cuando ya se lleva un rato.
		x.AlDesdeInicio = nil
		x.Rehacer()
	}
	if !forzar && (r.Pausado() || time.Since(a.guardadoEn) < ReanudarCada) {
		return
	}
	yo := a.persona()
	if yo == "" {
		return
	}
	a.puntos = a.misPuntos().Anotar(a.reanudo, pos, total, time.Now())
	a.guardadoEn = time.Now()
	GuardarPuntos(yo, a.puntos)
}

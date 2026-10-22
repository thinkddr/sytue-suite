// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Las calidades en la app viva: la estimación al subir y al subir desde Drive, y la
// tarjeta de Gestionar. Las pantallas son funciones de sus datos (calidades.go); aquí
// se pide al servicio y se guarda la elección.

// estadoCalidades es lo de la tarjeta de Gestionar.
type estadoCalidades struct {
	datos     *api.CastVideoQualities
	error     bool
	elegidas  []int
	libres    *int64
	guardando bool
}

// TarjetaCalidades son los datos de la tarjeta de Gestionar (ManageQualities.tsx).
type TarjetaCalidades struct {
	Sel SelCalidades
	// Pendiente: lo generado aún no es lo elegido (se aplica en segundo plano).
	Pendiente bool
	// Cambiada: la elección difiere de la guardada (enciende Guardar).
	Cambiada, Guardando bool
	AlGuardar           func()
}

// tarjetaCalidades es la tarjeta de Gestionar: el selector, si hay cambios
// aplicándose y «Guardar calidades».
func tarjetaCalidades(v Vista, d TarjetaCalidades) fyne.CanvasObject {
	bloques := []ui.Bloque{{Obj: SelectorCalidades(v, d.Sel), Margen: tema.Espacio(3)}}
	if d.Pendiente {
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, v.Traduce("quality.pending"), tema.TextoXS, ui.Normal, "text-muted"), Margen: tema.Espacio(3)})
	}
	guardar := ui.NuevoBoton(v.Traduce("quality.save"), d.AlGuardar).Con(ui.Secundario, ui.TallaMD)
	guardar.Cargando = d.Guardando
	guardar.Inerte = d.Guardando || !d.Cambiada || len(d.Sel.Elegidas) == 0
	bloques = append(bloques, ui.Bloque{Obj: container.New(ui.FilaAuto{}, guardar)})
	return Recuadro(v, "", ui.PilaBloques(bloques...))
}

// ---- Subir ----

// preparar deja los ficheros en «Antes de subir» y pide la estimación de cada uno.
// Del fichero local solo se sabe el tamaño, así que la estimación sale aproximada
// (y lo dice). `// ponytail: leer duración y dimensiones del contenedor si la
// aproximación se queda corta; el demuxer WebM ya las sabe para un .webm`.
func (a *App) preparar(rutas []string) {
	for _, ruta := range rutas {
		a.sub.siguiente++
		p := Preparado{Clave: a.sub.siguiente, Ruta: ruta, Nombre: filepath.Base(ruta)}
		if fi, err := os.Stat(ruta); err == nil {
			p.Tamano = fi.Size()
		}
		a.sub.preparados = append(a.sub.preparados, p)
		if a.cli == nil {
			continue
		}
		gen, clave, tam := a.gen, p.Clave, p.Tamano
		a.lanzar(func() {
			c, fin := ctx()
			defer fin()
			e, err := a.cli.Estimate(c, tam, 0, 0, 0)
			a.hilo(gen, func() { a.estimado(clave, e, err) })
		})
	}
}

// estimado anota la estimación de un preparado y, si ya están todas y nadie ha
// tocado la elección, marca la recomendada.
func (a *App) estimado(clave int, e api.CastEstimate, err error) {
	for i := range a.sub.preparados {
		if a.sub.preparados[i].Clave != clave {
			continue
		}
		if err != nil {
			a.sub.preparados[i].Error = true
		} else {
			a.sub.preparados[i].Estimacion = &e
		}
	}
	if s := a.estimacionSubida(); s != nil && !a.sub.tocadas {
		a.sub.calidades = Recomendada(*s)
	}
	a.pintar()
}

// estimacionSubida es la de todo lo preparado, cuando han llegado todas.
func (a *App) estimacionSubida() *api.CastEstimate {
	es := make([]api.CastEstimate, 0, len(a.sub.preparados))
	for _, p := range a.sub.preparados {
		if p.Estimacion == nil {
			return nil
		}
		es = append(es, *p.Estimacion)
	}
	return SumarEstimaciones(es)
}

func (a *App) selCalidadesSubir() SelCalidades {
	s := SelCalidades{
		Estimacion: a.estimacionSubida(), Elegidas: a.sub.calidades, Avisar1080: true,
		AlCambiar: func(qs []int) { a.sub.calidades, a.sub.tocadas = qs, true; a.pintar() },
	}
	for _, p := range a.sub.preparados {
		s.Error = s.Error || p.Error
	}
	if a.sub.cuota != nil {
		libres := a.sub.cuota.AvailableBytes
		s.Libres = &libres
	}
	return s
}

// empezarSubida pasa lo preparado a la cola con la elección. Sin tocarla va vacía:
// el servicio da a cada vídeo la recomendada de SU original.
func (a *App) empezarSubida() {
	rutas := make([]string, 0, len(a.sub.preparados))
	for _, p := range a.sub.preparados {
		rutas = append(rutas, p.Ruta)
	}
	var cals []int
	if a.sub.tocadas {
		cals = slices.Clone(a.sub.calidades)
	}
	a.sub.preparados, a.sub.calidades, a.sub.tocadas = nil, nil, false
	a.encolar(rutas, cals)
}

func (a *App) descartarSubida() {
	a.sub.preparados, a.sub.calidades, a.sub.tocadas = nil, nil, false
	a.pintar()
}

// ---- Subir desde Drive ----

// estimarDrive pide la estimación del fichero de Drive (solo se sabe su tamaño) y lo
// que queda en el plan.
func (a *App) estimarDrive(tam int64) {
	if a.cli == nil {
		return
	}
	gen := a.gen
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		e, err := a.cli.Estimate(c, tam, 0, 0, 0)
		q, errQ := a.cli.Quota(c)
		a.hilo(gen, func() {
			if err != nil {
				a.dri.errorEstimacion = true
			} else {
				a.dri.estimacion = &e
				if !a.dri.tocadas {
					a.dri.calidades = Recomendada(e)
				}
			}
			if errQ == nil {
				a.dri.libres = q.AvailableBytes
			}
			a.pintar()
		})
	})
}

func (a *App) selCalidadesDrive() SelCalidades {
	s := SelCalidades{
		Estimacion: a.dri.estimacion, Error: a.dri.errorEstimacion, Elegidas: a.dri.calidades,
		Avisar1080: true, Deshabilitado: a.dri.enviando,
		AlCambiar: func(qs []int) { a.dri.calidades, a.dri.tocadas = qs, true; a.pintar() },
	}
	if a.dri.libres >= 0 {
		libres := a.dri.libres
		s.Libres = &libres
	}
	return s
}

// ---- Gestionar ----

// pedirCalidades lee las calidades del vídeo con su estimación, y lo que queda en el plan.
func (a *App) pedirCalidades() {
	a.ges.cal = estadoCalidades{}
	if a.cli == nil || !conCalidades(a.ges.video) {
		return
	}
	gen, id := a.gen, a.ges.id
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		q, err := a.cli.Qualities(c, id)
		cuota, errC := a.cli.Quota(c)
		a.hilo(gen, func() {
			if err != nil {
				a.ges.cal.error = true
			} else {
				a.ges.cal.datos, a.ges.cal.elegidas = &q, slices.Clone(q.Qualities)
			}
			if errC == nil {
				libres := cuota.AvailableBytes
				a.ges.cal.libres = &libres
			}
			a.pintar()
		})
	})
}

// conCalidades: un directo programado o en curso aún no tiene calidades que elegir.
func conCalidades(vid api.CastVideo) bool {
	return vid.Status != api.CastStatusScheduled && vid.Status != api.CastStatusLive
}

func (a *App) tarjetaCalidades() *TarjetaCalidades {
	if !conCalidades(a.ges.video) {
		return nil
	}
	c := a.ges.cal
	t := &TarjetaCalidades{
		Sel: SelCalidades{
			Error: c.error, Elegidas: c.elegidas, Libres: c.libres, Deshabilitado: c.guardando,
			AlCambiar: func(qs []int) { a.ges.cal.elegidas = qs; a.pintar() },
		},
		Guardando: c.guardando, AlGuardar: a.guardarCalidades,
	}
	if c.datos != nil {
		e := c.datos.CastEstimate
		t.Sel.Estimacion = &e
		t.Pendiente = c.datos.Pending
		t.Cambiada = !slices.Equal(Ordenadas(c.elegidas), Ordenadas(c.datos.Qualities))
		// Lo que tiene que caber: solo lo que se añade a lo que ya hay.
		anade := max(BytesDe(e, c.elegidas)-BytesDe(e, Generadas(*c.datos)), 0)
		t.Sel.AOcupar = &anade
	}
	return t
}

// guardarCalidades guarda la elección; si quita alguna, antes pregunta, porque se
// borra y no se deshace sin volver a generarla.
func (a *App) guardarCalidades() {
	c := a.ges.cal
	if c.datos == nil || c.guardando || len(c.elegidas) == 0 {
		return
	}
	if fuera := Quitadas(Generadas(*c.datos), c.elegidas); len(fuera) > 0 {
		nombres := make([]string, len(fuera))
		for i, q := range fuera {
			nombres[i] = Etiqueta(q)
		}
		a.confirmar(a.v.Traduce("quality.removeTitle"),
			a.v.Traduce("quality.removeDescription", i18n.Vars{"qualities": strings.Join(nombres, ", ")}),
			a.v.Traduce("quality.remove"), a.enviarCalidades)
		return
	}
	a.enviarCalidades()
}

func (a *App) enviarCalidades() {
	if a.cli == nil {
		return
	}
	gen, id, elegidas := a.gen, a.ges.id, slices.Clone(a.ges.cal.elegidas)
	a.ges.cal.guardando = true
	a.pintar()
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		q, err := a.cli.SetQualities(c, id, elegidas)
		a.hilo(gen, func() {
			a.ges.cal.guardando = false
			if err != nil {
				a.avisar(ui.AvisoPeligro, a.v.Traduce("manage.saveError"))
			} else {
				a.ges.cal.datos, a.ges.cal.elegidas = &q, slices.Clone(q.Qualities)
				a.avisar(ui.AvisoExito, a.v.Traduce("quality.saved"))
			}
			a.pintar()
		})
	})
}

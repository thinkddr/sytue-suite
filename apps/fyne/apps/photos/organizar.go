// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"slices"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # «Organizar en álbumes con IA»
//
// Espejo de `components/OrganizarConIA.tsx` y `lib/organizarIA.ts` de la web. En
// «Álbumes»: parte de las fotos que no están en ninguno de los álbumes de la persona
// (como mucho [api.TopeOrganizar] por pasada), describe con Sytue AI las que no tienen
// nada escrito (como mucho [TopeDescribir], porque cada una se cobra) y con lo visto le
// pide en UNA llamada de texto una propuesta de álbumes ([api.AgruparFotos], la misma
// instrucción y el mismo lector que la web). Antes de tocar nada la ENSEÑA: cada álbum
// con su título editable, cuántas fotos lleva, unas miniaturas y su casilla. «Crear
// álbumes» crea lo marcado —o lo añade al álbum que ya se llama igual— y el aviso trae
// «Deshacer» durante sus 10 s, como la papelera.
//
// LA GALERÍA CIFRADA NO ENTRA, con la misma regla que describir (ADR 0086): las fotos
// cifradas nunca pasan por Sytue AI, y si la persona tiene el Drive en la bóveda se le
// dice con el mismo aviso (`gallery.encrypted.note`).

// Cuántas páginas de la línea de tiempo se miran como mucho buscando fotos sin álbum, y
// de cuántas fotos (el máximo del servicio): `PAGINAS_ORGANIZAR` y
// `POR_PAGINA_ORGANIZAR` de la web.
const (
	PaginasOrganizar   = 10
	PorPaginaOrganizar = 200
)

// MiniaturasOrganizar son las fotos que se enseñan de cada álbum propuesto.
const MiniaturasOrganizar = 6

// ladoRuedaOrganizar es el `<Spinner size={16} />` del progreso.
const ladoRuedaOrganizar float32 = 16

// FaseOrganizar es en qué punto está «Organizar con IA».
type FaseOrganizar uint8

// Las fases, en el orden en que pasan.
const (
	OrgInactiva FaseOrganizar = iota
	OrgBuscando
	OrgDescribiendo
	OrgAgrupando
	OrgPropuesta
	OrgVacia
)

// propuestaAlbum es un álbum de la propuesta en la vista previa. El campo vive en el
// estado, como el del título de un álbum nuevo: la pantalla se rehace entera y lo
// escrito no se puede perder por eso.
type propuestaAlbum struct {
	titulo   *ui.Entrada
	fotos    []api.Photo
	incluido bool
}

// estadoOrganizar es lo que la pantalla de álbumes sabe de «Organizar con IA».
type estadoOrganizar struct {
	fase          FaseOrganizar
	hechas, total int
	propuesta     []propuestaAlbum
	existentes    []api.Album
	// topado: quedan fotos sin mirar; describirTopado: quedan sin describir.
	topado, describirTopado bool
	creando                 bool
	vacio, error            string
	// vez es la pasada en curso: lo que vuelve de una anterior —parada o sustituida—
	// se tira.
	vez int
	// parar corta la llamada de agrupar en vuelo; nil, no hay ninguna.
	parar func()
}

func (o *estadoOrganizar) trabajando() bool {
	return o.fase == OrgBuscando || o.fase == OrgDescribiendo || o.fase == OrgAgrupando
}

// ---- Lo que no es Sytue AI: de qué se parte, qué se crea y cómo se deshace ----

// fotosSinAlbum son las fotos que no están en ninguno de los álbumes de la persona, de
// la más reciente a la más antigua y [api.TopeOrganizar] como mucho. `topado` dice que
// quedan fotos sin mirar. Es `fotosSinAlbum` de `lib/organizarIA.ts`.
//
// SOLO SUS ÁLBUMES: los que le comparten para editar son de otra persona. Si uno no se
// puede leer, no se sigue: sin saber qué tiene se propondría ordenar fotos ya ordenadas.
// Y las cifradas no entran nunca, como en describir.
func fotosSinAlbum(ctx context.Context, cli *api.Photos) (albumes []api.Album, fotos []api.Photo, topado bool, err error) {
	albumes, err = cli.Albums(ctx)
	if err != nil {
		return nil, nil, false, err
	}
	var conFotos []api.Album
	for _, al := range albumes {
		// Los vacíos no se piden: no hay nada que leer en ellos.
		if al.Count > 0 {
			conFotos = append(conFotos, al)
		}
	}
	var mu sync.Mutex
	enAlbum := map[string]bool{}
	var fallo error
	_, fallidos := ui.EnTandas(conFotos, func(al api.Album) error {
		lista, err := cli.AlbumPhotos(ctx, al.ID)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			if fallo == nil {
				fallo = err
			}
			return err
		}
		for _, f := range lista {
			enAlbum[f.ID] = true
		}
		return nil
	}, ui.EnParalelo)
	if len(fallidos) > 0 {
		return nil, nil, false, fallo
	}
	cursor := ""
	for range PaginasOrganizar {
		pagina, siguiente, err := cli.Timeline(ctx, cursor, PorPaginaOrganizar, false, "")
		if err != nil {
			return nil, nil, false, err
		}
		var sueltas []api.Photo
		for _, f := range pagina {
			if !enAlbum[f.ID] && !EsCifrada(f.ID) {
				sueltas = append(sueltas, f)
			}
		}
		caben := api.TopeOrganizar - len(fotos)
		fotos = append(fotos, sueltas[:min(len(sueltas), caben)]...)
		cursor = siguiente
		if len(sueltas) > caben {
			return albumes, fotos, true, nil
		}
		if cursor == "" {
			return albumes, fotos, false, nil
		}
		if len(fotos) >= api.TopeOrganizar {
			break
		}
	}
	return albumes, fotos, true, nil
}

// AlbumElegido es un álbum de la propuesta tal como queda en la vista previa.
type AlbumElegido struct {
	Titulo   string
	IDs      []string
	Incluido bool
}

// PasoOrganizar es lo que se va a crear: en un álbum que ya existe, o en uno nuevo.
type PasoOrganizar struct {
	Titulo string
	// Existente es el álbum de la persona que se llama igual; nil, hay que crearlo.
	Existente *api.Album
	IDs       []string
}

// PlanDeAlbumes es `planDeAlbumes` de la web: lo que se crea con lo que se dejó marcado.
//
// SI YA HAY UN ÁLBUM QUE SE LLAMA IGUAL —sin contar mayúsculas ni acentos—, las fotos van
// a ese y no se crea otro: dos «Verano» en la lista es justo el desorden que se quería
// arreglar. Por lo mismo, dos propuestas que tras editar el título se llaman igual se
// juntan en una. Lo que queda sin título o sin fotos no se crea.
func PlanDeAlbumes(elegidos []AlbumElegido, existentes []api.Album) []PasoOrganizar {
	var plan []PasoOrganizar
	porTitulo := map[string]int{}
	for _, e := range elegidos {
		titulo := strings.TrimSpace(e.Titulo)
		if !e.Incluido || titulo == "" || len(e.IDs) == 0 {
			continue
		}
		clave := buscar.Plegar(titulo)
		if i, ya := porTitulo[clave]; ya {
			for _, id := range e.IDs {
				if !slices.Contains(plan[i].IDs, id) {
					plan[i].IDs = append(plan[i].IDs, id)
				}
			}
			continue
		}
		paso := PasoOrganizar{Titulo: titulo, Existente: albumLlamado(existentes, clave)}
		for _, id := range e.IDs {
			if !slices.Contains(paso.IDs, id) {
				paso.IDs = append(paso.IDs, id)
			}
		}
		porTitulo[clave] = len(plan)
		plan = append(plan, paso)
	}
	return plan
}

// albumLlamado es el álbum cuyo título plegado es `clave`, o nil.
func albumLlamado(albumes []api.Album, clave string) *api.Album {
	for i := range albumes {
		if buscar.Plegar(strings.TrimSpace(albumes[i].Title)) == clave {
			al := albumes[i]
			return &al
		}
	}
	return nil
}

// hechoOrganizar es lo que se hizo de verdad: con esto se deshace.
type hechoOrganizar struct {
	album api.Album
	// nuevo: se creó ahora, y deshacer lo borra entero (las fotos se quedan).
	nuevo bool
	// ids son las fotos que se metieron: deshacer las saca de un álbum que ya existía.
	ids []string
}

// crearAlbumes crea los álbumes del plan y les mete sus fotos, uno detrás de otro. Uno
// que falla no para el resto: lo hecho, hecho está, y se dice cuántos fallaron.
func crearAlbumes(ctx context.Context, cli *api.Photos, plan []PasoOrganizar) (hechos []hechoOrganizar, fallidos int) {
	for _, paso := range plan {
		nuevo := paso.Existente == nil
		var al api.Album
		if nuevo {
			var err error
			if al, err = cli.CreateAlbum(ctx, paso.Titulo); err != nil {
				fallidos++
				continue
			}
		} else {
			al = *paso.Existente
		}
		if _, err := cli.AddToAlbum(ctx, al.ID, paso.IDs); err != nil {
			fallidos++
			// Uno recién creado y vacío se queda para que «Deshacer» también lo quite.
			if nuevo {
				hechos = append(hechos, hechoOrganizar{album: al, nuevo: true})
			}
			continue
		}
		al.Count += len(paso.IDs)
		hechos = append(hechos, hechoOrganizar{album: al, nuevo: nuevo, ids: paso.IDs})
	}
	return hechos, fallidos
}

// deshacerOrganizados borra los álbumes que se crearon y saca de los que ya existían
// las fotos que se metieron. Las fotos no se tocan: un álbum solo las agrupa. Devuelve
// cuántas cosas no se pudieron deshacer.
func deshacerOrganizados(ctx context.Context, cli *api.Photos, hechos []hechoOrganizar) int {
	var tareas []func() error
	for _, h := range hechos {
		if h.nuevo {
			tareas = append(tareas, func() error { return cli.DeleteAlbum(ctx, h.album.ID) })
			continue
		}
		for _, id := range h.ids {
			tareas = append(tareas, func() error { return cli.RemoveFromAlbum(ctx, h.album.ID, id) })
		}
	}
	_, fallidas := ui.EnTandas(tareas, func(t func() error) error { return t() }, ui.EnParalelo)
	return len(fallidas)
}

// ---- El recorrido, en el hilo de la interfaz salvo las llamadas ----

// organizar es «Organizar con IA». Como describir: sin Sytue AI lo dice y no pide nada.
func (a *App) organizar() {
	o := &a.alb.org
	if o.trabajando() || o.creando || a.cli == nil {
		return
	}
	vez := o.vez + 1
	*o = estadoOrganizar{fase: OrgBuscando, vez: vez}
	if a.IA == nil {
		o.fase, o.error = OrgInactiva, a.falloIA(nil, "")
		a.pintar()
		return
	}
	a.pintar()
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		c, fin := a.ctxAlbum(vida)
		defer fin()
		albumes, fotos, topado, err := fotosSinAlbum(c, a.cli)
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			if o.vez != vez {
				return
			}
			if err != nil {
				o.fase, o.error = OrgInactiva, a.textoDeFallo(err)
				a.pintar()
				return
			}
			if len(fotos) == 0 {
				o.fase, o.vacio = OrgVacia, "albums.organize.nothing"
				a.pintar()
				return
			}
			// Se usa lo que Sytue AI ya vio; solo se describen las que no tienen nada.
			var sinNada []api.Photo
			for _, f := range fotos {
				if strings.TrimSpace(f.Description) == "" && len(f.Tags) == 0 {
					sinNada = append(sinNada, f)
				}
			}
			o.existentes, o.topado, o.describirTopado = albumes, topado, len(sinNada) > TopeDescribir
			a.organizarDescribiendo(gen, vez, fotos, sinNada[:min(len(sinNada), TopeDescribir)], 0)
		})
	})
}

// organizarDescribiendo describe la foto `i` del lote y, al volver, la siguiente: DE UNA
// EN UNA, como el lote de la selección, y «Parar» se lee entre una y otra. Cada una se
// guarda en cuanto vuelve, así que lo descrito se queda aunque se pare.
func (a *App) organizarDescribiendo(gen, vez int, fotos, lote []api.Photo, i int) {
	o := &a.alb.org
	if o.vez != vez {
		return
	}
	if i >= len(lote) {
		a.organizarAgrupando(gen, vez, fotos)
		return
	}
	o.fase, o.hechas, o.total = OrgDescribiendo, i, len(lote)
	a.pintar()
	f := lote[i]
	a.enSesion(func(vida context.Context) {
		d, et, err := a.describirYGuardar(vida, f.ID)
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			if o.vez != vez {
				return
			}
			if err != nil && api.IANoConfigurada(err) {
				// Sin Sytue AI no se sigue: las demás fallarían igual, una a una.
				o.fase, o.error = OrgInactiva, a.falloIA(err, "")
				a.pintar()
				return
			}
			if err == nil {
				a.ponerDescripcion(f.ID, d, et)
				for j := range fotos {
					if fotos[j].ID == f.ID {
						fotos[j].Description, fotos[j].Tags = d, et
					}
				}
			}
			// Una que falla no para el resto: se queda fuera de esta pasada.
			a.organizarDescribiendo(gen, vez, fotos, lote, i+1)
		})
	})
}

// organizarAgrupando le pide a Sytue AI la propuesta, con «Parar» cortando la llamada.
func (a *App) organizarAgrupando(gen, vez int, fotos []api.Photo) {
	o := &a.alb.org
	o.fase = OrgAgrupando
	// La llamada se corta desde el hilo de la interfaz con lo que arranque en la
	// goroutine: si «Parar» llega antes, la corta en cuanto empieza.
	var mu sync.Mutex
	parada := false
	var cortar context.CancelFunc
	o.parar = func() {
		mu.Lock()
		defer mu.Unlock()
		parada = true
		if cortar != nil {
			cortar()
		}
	}
	a.pintar()
	a.enSesion(func(vida context.Context) {
		c, fin := ctxIA(vida)
		defer fin()
		mu.Lock()
		cortar = fin
		if parada {
			fin()
		}
		mu.Unlock()
		propuesta, err := api.AgruparFotos(c, a.IA, fotos, a.idioma())
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			if o.vez != vez {
				return
			}
			o.parar = nil
			if err != nil {
				o.fase, o.error = OrgInactiva, a.falloIA(err, "albums.organize.failed")
			} else {
				a.ponerPropuesta(propuesta, fotos)
			}
			a.pintar()
		})
	})
}

// ponerPropuesta deja la propuesta en la vista previa, cada álbum con su campo de título
// y sus fotos. Sin ninguno que lleve fotos, es el vacío de «no hay grupos claros».
func (a *App) ponerPropuesta(propuesta []api.AlbumPropuesto, fotos []api.Photo) {
	o := &a.alb.org
	porID := make(map[string]api.Photo, len(fotos))
	for _, f := range fotos {
		porID[f.ID] = f
	}
	for _, p := range propuesta {
		pa := propuestaAlbum{titulo: ui.NuevaEntrada(a.v.Traduce("albums.title")), incluido: true}
		pa.titulo.PonerTexto(p.Titulo)
		for _, id := range p.IDs {
			if f, ok := porID[id]; ok {
				pa.fotos = append(pa.fotos, f)
			}
		}
		if len(pa.fotos) > 0 {
			o.propuesta = append(o.propuesta, pa)
		}
	}
	if len(o.propuesta) == 0 {
		o.fase, o.vacio = OrgVacia, "albums.organize.noGroups"
	} else {
		o.fase = OrgPropuesta
	}
}

// pararOrganizar es «Parar»: lo descrito hasta ahí ya está guardado; lo demás no se hace.
func (a *App) pararOrganizar() {
	o := &a.alb.org
	if o.parar != nil {
		o.parar()
	}
	*o = estadoOrganizar{vez: o.vez + 1}
	a.pintar()
}

// descartarOrganizar cierra la propuesta sin crear nada.
func (a *App) descartarOrganizar() {
	o := &a.alb.org
	if o.creando {
		return
	}
	*o = estadoOrganizar{vez: o.vez + 1}
	a.pintar()
}

// elegidos es la propuesta tal como está ahora en la vista previa.
func (o *estadoOrganizar) elegidos() []AlbumElegido {
	out := make([]AlbumElegido, len(o.propuesta))
	for i, p := range o.propuesta {
		out[i] = AlbumElegido{Titulo: p.titulo.Texto(), IDs: idsDe(p.fotos), Incluido: p.incluido}
	}
	return out
}

// crearOrganizados es «Crear álbumes». El aviso con «Deshacer» sale aunque entretanto se
// haya cambiado de pantalla, como el de la papelera: los álbumes ya están creados, y sin
// el aviso no habría con qué deshacerlo.
func (a *App) crearOrganizados() {
	o := &a.alb.org
	if o.fase != OrgPropuesta || o.creando || a.cli == nil {
		return
	}
	plan := PlanDeAlbumes(o.elegidos(), o.existentes)
	if len(plan) == 0 {
		return
	}
	o.creando = true
	a.pintar()
	gen, vez, identidad := a.gen, o.vez, a.identidad
	a.enSesion(func(vida context.Context) {
		c, fin := a.ctxAlbum(vida)
		defer fin()
		hechos, fallidos := crearAlbumes(c, a.cli, plan)
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		fyne.Do(func() {
			if a.cerrada || identidad != a.identidad {
				return
			}
			a.albumes = nil // la del selector se queda vieja
			if gen == a.gen && o.vez == vez {
				*o = estadoOrganizar{vez: vez + 1}
				if len(hechos) > 0 {
					a.pedirAlbumes()
				}
				a.pintar()
			}
			a.avisarOrganizados(hechos, identidad)
			if fallidos > 0 {
				a.avisar(ui.AvisoPeligro, a.v.T.T("albums.organize.createFailed", i18n.Vars{"count": fallidos}), "")
			}
		})
	})
}

// avisarOrganizados dice cuántos álbumes y cuántas fotos, durante los 10 s de
// ui.DuracionDeshacer, con «Deshacer».
func (a *App) avisarOrganizados(hechos []hechoOrganizar, identidad string) {
	if len(hechos) == 0 {
		return
	}
	fotos := 0
	for _, h := range hechos {
		fotos += len(h.ids)
	}
	av := ui.NuevoAviso(ui.AvisoExito, a.v.T.T("albums.organize.done", i18n.Vars{"count": len(hechos)}),
		a.v.T.T("albums.organize.donePhotos", i18n.Vars{"count": fotos}))
	av.Duracion = ui.DuracionDeshacer
	av.RotuloAccion = a.v.Traduce("gallery.undo")
	av.AlPulsarAccion = func() { a.deshacerOrganizar(hechos, identidad) }
	a.avisos.Notificar(av)
}

// deshacerOrganizar es el «Deshacer» del aviso. Con otra cuenta, esos álbumes son de la
// anterior: no se tocan.
func (a *App) deshacerOrganizar(hechos []hechoOrganizar, identidad string) {
	if a.cli == nil || a.identidad != identidad {
		return
	}
	a.enSesion(func(vida context.Context) {
		c, fin := a.ctxAlbum(vida)
		defer fin()
		fallidas := deshacerOrganizados(c, a.cli, hechos)
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		fyne.Do(func() {
			if a.cerrada || identidad != a.identidad {
				return
			}
			a.albumes = nil
			if a.ruta == RutaAlbumes {
				a.pedirAlbumes()
			}
			if fallidas > 0 {
				a.avisar(ui.AvisoPeligro, a.v.T.T("albums.organize.undoFailed", i18n.Vars{"count": fallidas}), "")
			} else {
				a.avisar(ui.AvisoExito, a.v.Traduce("albums.organize.undone"), "")
			}
		})
	})
}

// datosOrganizar son los datos de la sección para la pantalla.
func (a *App) datosOrganizar() *OrganizarIA {
	o := &a.alb.org
	d := &OrganizarIA{
		Fase: o.fase, Hechas: o.hechas, Total: o.total, Topado: o.topado,
		DescribirTopado: o.describirTopado, Creando: o.creando, Vacio: o.vacio, Error: o.error,
		EnBoveda:  a.cif.enBoveda,
		AlEmpezar: a.organizar, AlParar: a.pararOrganizar,
		AlCrear: a.crearOrganizados, AlDescartar: a.descartarOrganizar,
	}
	for i, p := range o.propuesta {
		d.Propuesta = append(d.Propuesta, PropuestaVista{
			Titulo: p.titulo, Fotos: p.fotos, Incluido: p.incluido,
			Existe: albumLlamado(o.existentes, buscar.Plegar(strings.TrimSpace(p.titulo.Texto()))) != nil,
			AlIncluir: func(si bool) {
				if i < len(o.propuesta) {
					o.propuesta[i].incluido = si
					a.pintar()
				}
			},
		})
	}
	return d
}

// ---- La sección ----

// OrganizarIA son los datos de «Organizar con IA» en «Álbumes».
type OrganizarIA struct {
	Fase          FaseOrganizar
	Hechas, Total int
	Propuesta     []PropuestaVista
	// Topado y DescribirTopado: quedan fotos sin mirar, o sin describir.
	Topado, DescribirTopado bool
	Creando                 bool
	// Vacio es la clave del estado vacío; Error, el texto del fallo.
	Vacio, Error string
	// EnBoveda: la persona tiene el Drive en la bóveda, y sus cifradas no entran.
	EnBoveda bool

	AlEmpezar, AlParar, AlCrear, AlDescartar func()
}

// PropuestaVista es un álbum propuesto tal como se pinta.
type PropuestaVista struct {
	Titulo   *ui.Entrada
	Fotos    []api.Photo
	Incluido bool
	// Existe: ya hay un álbum que se llama así, y las fotos irán a ese.
	Existe    bool
	AlIncluir func(bool)
}

// SeccionOrganizar es la `<section>` de OrganizarConIA.tsx: la cabecera con el botón, la
// pista, el progreso con «Parar», el fallo, el vacío y la vista previa.
func SeccionOrganizar(v Vista, d *OrganizarIA) fyne.CanvasObject {
	trabajando := d.Fase == OrgBuscando || d.Fase == OrgDescribiendo || d.Fase == OrgAgrupando
	// De fantasma, como «Buscar montones»: lo principal de la página es crear un álbum.
	empezar := ui.NuevoBoton(v.Traduce("albums.organize.start"), d.AlEmpezar).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.IA, 0)
	empezar.Inerte = trabajando || d.Creando
	cabecera := container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: 2, Partir: true, SaltoFila: tema.Espacio(2)},
		ParrafoEn(v.Traduce("albums.organize.title"), tema.TextoSM, ui.SemiNegrita, "text", v.Variante), empezar)
	bloques := []ui.Bloque{
		{Obj: cabecera, Margen: tema.Espacio(2)},
		bloqueLargo(ui.NuevoParrafo(v.Traduce("albums.organize.hint"), tema.TextoXS, ui.Normal, "text-muted", v.Variante), tema.Espacio(2)),
	}
	if d.EnBoveda {
		bloques = append(bloques, bloqueLargo(ui.NuevoParrafo(v.Traduce("gallery.encrypted.note"), tema.TextoXS, ui.Normal, "text-muted", v.Variante), tema.Espacio(2)))
	}
	if trabajando {
		// El progreso se lee siempre: es un botón que gasta dinero por foto, y «Parar»
		// tiene que estar a mano MIENTRAS corre.
		var progreso string
		switch d.Fase {
		case OrgDescribiendo:
			progreso = v.T.T("albums.organize.describing", i18n.Vars{"done": d.Hechas, "total": d.Total})
		case OrgAgrupando:
			progreso = v.Traduce("albums.organize.grouping")
		default:
			progreso = v.Traduce("albums.organize.looking")
		}
		// `<Spinner size={16} />`, la del progreso en línea.
		rueda := ui.NuevaRueda(ladoRuedaOrganizar, "text", v.Variante)
		rueda.Rotulo = progreso
		bloques = append(bloques, ui.Bloque{Obj: container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: 3, Partir: true, SaltoFila: tema.Espacio(2)},
			container.New(fijo{ancho: ladoRuedaOrganizar, alto: ladoRuedaOrganizar}, rueda),
			ParrafoEn(progreso, tema.TextoSM, ui.Normal, "text", v.Variante),
			ui.NuevoBoton(v.Traduce("gallery.describeStop"), d.AlParar).Con(ui.Fantasma, ui.TallaSM)), Margen: tema.Espacio(2)})
	}
	if d.Error != "" {
		bloques = append(bloques, bloqueLargo(ui.NuevoParrafo(d.Error, tema.TextoSM, ui.Normal, "danger-text", v.Variante), tema.Espacio(2)))
	}
	if d.Fase == OrgVacia && d.Vacio != "" {
		vacio := ui.NuevoVacio(v.Traduce(d.Vacio), "")
		vacio.Compacto = true
		vacio.Glifo = iconos.Trazo(iconos.Album, ui.LadoGlifoVacioCompacto, "text-subtle", v.Variante)
		bloques = append(bloques, ui.Bloque{Obj: vacio, Margen: tema.Espacio(2)})
	}
	if d.Fase == OrgPropuesta {
		bloques = append(bloques, vistaPreviaOrganizar(v, d)...)
	}
	bloques[len(bloques)-1].Margen = 0
	return ui.PilaBloques(bloques...)
}

// vistaPreviaOrganizar son los bloques de la propuesta: el recuento, una fila por álbum
// (`border rounded-md p-2`, como un montón sugerido) con su casilla, su título, las
// miniaturas, cuántas fotos y si va a un álbum que ya existe, las notas de lo que queda
// y los dos botones.
func vistaPreviaOrganizar(v Vista, d *OrganizarIA) []ui.Bloque {
	bloques := []ui.Bloque{{
		Obj:    ParrafoEn(v.T.T("albums.organize.proposal", i18n.Vars{"count": len(d.Propuesta)}), tema.TextoXS, ui.SemiNegrita, "text-muted", v.Variante),
		Margen: tema.Espacio(2),
	}}
	algo := false
	for _, p := range d.Propuesta {
		titulo := strings.TrimSpace(p.Titulo.Texto())
		algo = algo || (p.Incluido && titulo != "")
		casilla := ui.NuevaCasilla(v.T.T("albums.organize.include", i18n.Vars{"title": titulo}), p.AlIncluir)
		casilla.SinEtiqueta, casilla.Marcada = true, p.Incluido
		hijos := []fyne.CanvasObject{casilla, anchoFijo(p.Titulo, tema.Espacio(64))}
		for _, f := range p.Fotos[:min(len(p.Fotos), MiniaturasOrganizar)] {
			m := NuevaMiniatura(f, v.Teca)
			m.SinMarcas, m.Rotulo = true, textoAlternativo(f)
			hijos = append(hijos, anchoFijo(m, ladoMonton))
		}
		hijos = append(hijos, ParrafoEn(Recuento(v, "albums.count", len(p.Fotos)), tema.TextoSM, ui.Normal, "text-muted", v.Variante))
		if p.Existe {
			hijos = append(hijos, ParrafoEn(v.Traduce("albums.organize.existing"), tema.TextoXS, ui.Normal, "text-muted", v.Variante))
		}
		fila := container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: len(hijos), Partir: true, SaltoFila: tema.Espacio(2)}, hijos...)
		bloques = append(bloques, ui.Bloque{Obj: recuadro(v, "", "border", tema.RadioMD,
			ui.Margen{Izq: tema.Espacio(2), Der: tema.Espacio(2), Arr: tema.Espacio(2), Abj: tema.Espacio(2)}, fila), Margen: tema.Espacio(3)})
	}
	// Lo que queda para la pasada siguiente: fotos sin mirar, o sin describir.
	if d.Topado {
		bloques = append(bloques, bloqueLargo(ui.NuevoParrafo(v.T.T("albums.organize.capped", i18n.Vars{"count": api.TopeOrganizar}), tema.TextoXS, ui.Normal, "text-muted", v.Variante), tema.Espacio(2)))
	}
	if d.DescribirTopado {
		bloques = append(bloques, bloqueLargo(ui.NuevoParrafo(v.T.T("gallery.describeCapped", i18n.Vars{"count": TopeDescribir}), tema.TextoXS, ui.Normal, "text-muted", v.Variante), tema.Espacio(2)))
	}
	crear := ui.NuevoBoton(v.Traduce("albums.organize.create"), d.AlCrear).Con(ui.Primario, ui.TallaSM).ConIcono(iconos.Album, 0)
	crear.Inerte, crear.Cargando = !algo || d.Creando, d.Creando
	descartar := ui.NuevoBoton(v.Traduce("albums.organize.discard"), d.AlDescartar).Con(ui.Fantasma, ui.TallaSM)
	descartar.Inerte = d.Creando
	bloques = append(bloques, ui.Bloque{Obj: container.New(ui.Fila{Hueco: tema.Espacio(2)}, crear, descartar)})
	return bloques
}

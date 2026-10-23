// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Las pantallas que no son la línea de tiempo: Albums.tsx, AlbumView.tsx, Trash.tsx,
// Duplicadas.tsx, Similares.tsx y Cerca.tsx. Cada una pide UNA cosa al entrar y hace
// unas pocas; aquí están las dos mitades.

// pedir es el patrón de las seis: la llamada en segundo plano y el resultado en el
// hilo de la interfaz, tirado si entretanto se ha cambiado de pantalla.
func pedir[T any](a *App, llamada func(context.Context, *api.Photos) (T, error), listo func(T, error)) {
	if a.cli == nil {
		return
	}
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		c, fin := ctx(vida)
		defer fin()
		r, err := llamada(c, a.cli)
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			listo(r, err)
			a.pintar()
		})
	})
}

// ---- Álbumes ----

func (a *App) pedirAjustes() {
	pedir(a, func(x context.Context, c *api.Photos) (api.PhotosSettings, error) {
		return c.Settings(x)
	}, func(aj api.PhotosSettings, err error) {
		if err == nil {
			a.aj = aj
		}
	})
}

func (a *App) cambiarExclusionLote(excluir bool) {
	pedir(a, func(x context.Context, c *api.Photos) (api.PhotosSettings, error) {
		return c.SetExcludeAIBatch(x, excluir)
	}, func(aj api.PhotosSettings, err error) {
		if err != nil {
			if a.ruta == RutaRepetidas {
				a.rep.error = a.textoDeFallo(err)
			} else {
				a.alb.error = a.textoDeFallo(err)
			}
			return
		}
		a.aj = aj
	})
}

func (a *App) datosAlbumes() Albumes {
	return Albumes{
		Lista: a.alb.lista, Titulo: a.campoAlbum, Montones: a.alb.montones,
		Buscando: a.alb.buscando, Error: a.alb.error,
		ExcluidaLoteIA: a.aj.ExcludeAIBatch, PuedeGestionarLoteIA: a.aj.CanManageAIBatch,
		AlExcluirLoteIA: a.cambiarExclusionLote,
		AlAbrir:         func(al api.Album) { a.AbrirAlbum(al.ID) },
		AlIr:            a.irSeccion, AlCrear: a.crearAlbum, AlBuscar: a.buscarMontones,
		AlGuardar: a.guardarMonton,
		AlVer:     func(fotos []api.Photo) { a.AbrirParecidas(idsDe(fotos)) },
	}
}

func (a *App) pedirAlbumes() {
	pedir(a, func(x context.Context, c *api.Photos) ([]api.Album, error) { return c.Albums(x) }, func(l []api.Album, err error) {
		a.alb.cargando, a.alb.fallo = false, err
		if err != nil {
			a.alb.error = a.textoDeFallo(err)
			return
		}
		if l == nil {
			l = []api.Album{}
		}
		// El selector de «Añadir a un álbum…» lleva además los compartidos para editar:
		// se pide aparte (conAlbumes).
		a.alb.lista, a.albumes = l, nil
	})
}

// crearAlbum es el `<form onSubmit>` de «Crear».
func (a *App) crearAlbum(titulo string) {
	titulo = strings.TrimSpace(titulo)
	if titulo == "" {
		return
	}
	pedir(a, func(x context.Context, c *api.Photos) (api.Album, error) { return c.CreateAlbum(x, titulo) }, func(al api.Album, err error) {
		if err != nil {
			a.alb.error = a.textoDeFallo(err)
			return
		}
		a.alb.lista = append([]api.Album{al}, a.alb.lista...)
		a.albumes = nil // la del selector se queda vieja
		a.campoAlbum.PonerTexto("")
	})
}

// buscarMontones pide las sugerencias. SOLO a petición: recorrer los vectores de la
// biblioteca no es gratis, y nadie abre «Álbumes» para que le adivinen.
func (a *App) buscarMontones() {
	if a.alb.buscando {
		return
	}
	a.alb.buscando = true
	a.pintar()
	pedir(a, func(x context.Context, c *api.Photos) ([][]api.Photo, error) { return c.Themes(x) }, func(m [][]api.Photo, err error) {
		a.alb.buscando = false
		if err != nil {
			a.alb.error = a.textoDeFallo(err)
			return
		}
		if m == nil {
			m = [][]api.Photo{}
		}
		a.alb.montones = m
	})
}

// guardarMonton convierte una sugerencia en álbum, con el título propuesto por lo que
// Sytue AI vio en esas fotos y editable.
func (a *App) guardarMonton(fotos []api.Photo) {
	a.preguntar(a.v.Traduce("albums.suggestionSave"), a.v.T.T("gallery.newAlbumPrompt", i18n.Vars{"count": len(fotos)}),
		a.v.Traduce("gallery.newAlbumCreate"), TituloSugerido(fotos), func(titulo string) {
			a.crearCon(titulo, idsDe(fotos), func(al api.Album, err error) {
				if err != nil {
					a.alb.error = a.textoDeFallo(err)
					return
				}
				al.Count = len(fotos)
				a.alb.lista = append([]api.Album{al}, a.alb.lista...)
				for i, m := range a.alb.montones {
					if len(m) > 0 && len(fotos) > 0 && m[0].ID == fotos[0].ID {
						a.alb.montones = append(a.alb.montones[:i:i], a.alb.montones[i+1:]...)
						break
					}
				}
			})
		})
}

// crearCon crea un álbum y le mete esas fotos de una vez.
func (a *App) crearCon(titulo string, ids []string, listo func(api.Album, error)) {
	pedir(a, func(x context.Context, c *api.Photos) (api.Album, error) {
		al, err := c.CreateAlbum(x, strings.TrimSpace(titulo))
		if err == nil {
			_, err = c.AddToAlbum(x, al.ID, ids)
		}
		return al, err
	}, func(al api.Album, err error) {
		if err == nil {
			a.albumes = nil
		}
		listo(al, err)
	})
}

// ---- Un álbum ----

// AbrirAlbum va a la página de un álbum (`album?id=`).
func (a *App) AbrirAlbum(id string) {
	a.Ir(RutaAlbum)
	a.uno = estadoAlbum{id: id, estadoCarga: estadoCarga{cargando: true}}
	a.pintar()
	type par struct {
		al    api.Album
		fotos []api.Photo
	}
	pedir(a, func(x context.Context, c *api.Photos) (par, error) {
		al, err := c.Album(x, id)
		if err != nil {
			return par{}, err
		}
		fotos, err := c.AlbumPhotos(x, id)
		if fotos == nil {
			fotos = []api.Photo{}
		}
		return par{al, fotos}, err
	}, func(p par, err error) {
		a.uno.cargando, a.uno.fallo = false, err
		if err != nil {
			a.uno.error = a.textoDeFallo(err)
			return
		}
		a.uno.album, a.uno.fotos = p.al, p.fotos
	})
}

func (a *App) datosAlbum() AlbumAbierto {
	u := a.uno
	var conPersonas func()
	if a.Directorio != nil && u.album.ID != "" {
		conPersonas = a.compartirConPersonas
	}
	return AlbumAbierto{
		AlCompartirConPersonas: conPersonas,
		Album:                  u.album, Fotos: u.fotos, Aviso: u.aviso, Error: u.error,
		AlVolver:           func() { a.Ir(RutaAlbumes) },
		AlCompartir:        a.compartirAlbum,
		AlDejarDeCompartir: a.dejarDeCompartir,
		AlBorrar:           a.borrarAlbum,
		AlAbrir: func(f api.Photo) {
			if i := posicion(a.uno.fotos, f.ID); i >= 0 {
				a.abrirVisor(desdeAlbum, i)
			}
		},
		AlIr: a.irSeccion,
	}
}

// compartirAlbum crea el enlace y lo copia. El servicio solo guarda su huella:
// en un álbum ya compartido crea uno NUEVO y el anterior deja de valer, así que
// antes se pregunta (FILE-10, como la web).
func (a *App) compartirAlbum() {
	id := a.uno.id
	generar := func() {
		pedir(a, func(x context.Context, c *api.Photos) (string, error) { return c.ShareAlbum(x, id) }, func(tk string, err error) {
			if err != nil {
				a.uno.error = a.textoDeFallo(err)
				return
			}
			a.uno.album.Shared = true
			a.copiarEnlace(tk)
		})
	}
	if !a.uno.album.Shared {
		generar()
		return
	}
	a.confirmar(a.v.Traduce("album.newLink"), a.v.Traduce("album.newLinkConfirm"), a.v.Traduce("album.newLink"), generar)
}

// copiarEnlace deja el enlace público en el portapapeles. Si no hay a dónde copiar, lo
// enseña, que es lo que hace la web cuando `clipboard.writeText` falla.
func (a *App) copiarEnlace(tk string) {
	enlace := "/p/" + tk
	if a.WebDe != nil {
		enlace = a.WebDe(enlace)
	}
	if app := fyne.CurrentApp(); app != nil {
		app.Clipboard().SetContent(enlace)
		a.uno.aviso = a.v.Traduce("album.linkCopied")
	} else {
		a.uno.aviso = enlace
	}
	a.pintar()
}

func (a *App) dejarDeCompartir() {
	id := a.uno.id
	a.confirmar(a.v.Traduce("album.unshare"), a.v.Traduce("album.unshareConfirm"), a.v.Traduce("album.unshare"), func() {
		pedir(a, func(x context.Context, c *api.Photos) (struct{}, error) { return struct{}{}, c.UnshareAlbum(x, id) }, func(_ struct{}, err error) {
			if err != nil {
				a.uno.error = a.textoDeFallo(err)
				return
			}
			a.uno.album.Shared, a.uno.aviso = false, ""
		})
	})
}

func (a *App) borrarAlbum() {
	id := a.uno.id
	a.confirmar(a.v.Traduce("albums.delete"), a.v.Traduce("albums.deleteConfirm"), a.v.Traduce("albums.delete"), func() {
		pedir(a, func(x context.Context, c *api.Photos) (struct{}, error) { return struct{}{}, c.DeleteAlbum(x, id) }, func(_ struct{}, err error) {
			if err != nil {
				a.uno.error = a.textoDeFallo(err)
				return
			}
			a.albumes = nil
			// Fuera de este `listo`: Ir rehace la pantalla y cambia la generación.
			defer a.Ir(RutaAlbumes)
		})
	})
}

// ---- La papelera ----

func (a *App) pedirPapelera() {
	pedir(a, func(x context.Context, c *api.Photos) ([]api.Photo, error) { return c.Papelera(x) }, func(l []api.Photo, err error) {
		a.pap.cargando, a.pap.fallo = false, err
		if err != nil {
			a.pap.error = a.textoDeFallo(err)
			return
		}
		if l == nil {
			l = []api.Photo{}
		}
		a.pap.fotos = l
	})
}

func (a *App) datosPapelera() Papelera {
	return Papelera{
		Fotos: a.pap.fotos, Aviso: a.pap.aviso, Error: a.pap.error, AlIr: a.irSeccion,
		AlRestaurar: a.restaurar,
		AlPurgar: func(f api.Photo) {
			a.confirmar(a.v.Traduce("trash.purge"), a.v.Traduce("trash.purgeConfirm"), a.v.Traduce("trash.purge"),
				func() { a.purgar(f.ID) })
		},
		AlVaciar: func() {
			a.confirmar(a.v.Traduce("trash.empty"), a.v.T.T("trash.emptyConfirm", i18n.Vars{"count": len(a.pap.fotos)}),
				a.v.Traduce("trash.empty"), func() { a.purgar("") })
		},
	}
}

func (a *App) restaurar(f api.Photo) {
	pedir(a, func(x context.Context, c *api.Photos) (struct{}, error) { return struct{}{}, c.Restaurar(x, f.ID) }, func(_ struct{}, err error) {
		if err != nil {
			a.pap.error = a.textoDeFallo(err)
			return
		}
		a.pap.fotos = quitarFoto(a.pap.fotos, f.ID)
	})
}

// purgar borra para siempre una foto, o la papelera entera con id vacío.
func (a *App) purgar(id string) {
	pedir(a, func(x context.Context, c *api.Photos) (int, error) { return c.Purgar(x, id) }, func(n int, err error) {
		if err != nil {
			a.pap.error = a.textoDeFallo(err)
			return
		}
		if id == "" {
			a.pap.fotos = []api.Photo{}
		} else {
			a.pap.fotos, n = quitarFoto(a.pap.fotos, id), 1
		}
		a.pap.aviso = a.v.T.T("trash.purged", i18n.Vars{"count": n})
	})
}

// ---- Repetidas ----

func (a *App) pedirRepetidas() {
	pedir(a, func(x context.Context, c *api.Photos) ([][]api.Photo, error) { return c.Duplicates(x, 50) }, func(g [][]api.Photo, err error) {
		a.rep.cargando, a.rep.fallo = false, err
		if err != nil {
			a.rep.error = a.textoDeFallo(err)
			return
		}
		if g == nil {
			g = [][]api.Photo{}
		}
		a.rep.grupos = g
	})
}

func (a *App) datosRepetidas() Repetidas {
	return Repetidas{
		Grupos: a.rep.grupos, Aviso: a.rep.aviso, Error: a.rep.error, Trabajando: a.rep.trabajando,
		ExcluidaLoteIA: a.aj.ExcludeAIBatch, PuedeGestionarLoteIA: a.aj.CanManageAIBatch,
		AlExcluirLoteIA: a.cambiarExclusionLote,
		AlIr:            a.irSeccion, AlQuedarse: a.quedarse,
	}
}

// quedarse manda a la papelera las OTRAS fotos del grupo. Se PROPONE y no se ejecuta
// solo: quien decide cuál vale es quien mira.
func (a *App) quedarse(grupo int, elegida api.Photo) {
	if a.rep.trabajando || grupo >= len(a.rep.grupos) {
		return
	}
	sobran := quitarFoto(a.rep.grupos[grupo], elegida.ID)
	a.rep.trabajando = true
	a.pintar()
	pedir(a, func(x context.Context, c *api.Photos) (int, error) { return tirarVarias(x, c, idsDe(sobran)), nil }, func(bien int, _ error) {
		a.rep.trabajando = false
		a.rep.grupos = append(a.rep.grupos[:grupo:grupo], a.rep.grupos[grupo+1:]...)
		a.rep.aviso = a.v.T.T("dup.done", i18n.Vars{"count": bien})
		if bien < len(sobran) {
			a.rep.error = a.v.T.T("gallery.trashFailed", i18n.Vars{"count": len(sobran) - bien})
		}
	})
}

// tirarVarias manda esas fotos a la papelera y dice cuántas se fueron. El servicio no
// tiene un «borrar varias», y para unas decenas no hace falta.
func tirarVarias(x context.Context, c *api.Photos, ids []string) int {
	bien := 0
	for _, id := range ids {
		if c.Trash(x, id) == nil {
			bien++
		}
	}
	return bien
}

// ---- Parecidas y «por aquí» ----

// AbrirParecidas va a «Parecidas a esta» (una) o a «Más como estas» (varias, hasta doce).
func (a *App) AbrirParecidas(ids []string) {
	if len(ids) == 0 {
		return
	}
	ids = ids[:min(len(ids), TopeParecidas)]
	a.Ir(RutaParecidas)
	a.enc = estadoEncontradas{ids: ids, estadoCarga: estadoCarga{cargando: true}}
	a.pintar()
	type par struct{ refs, fotos []api.Photo }
	pedir(a, func(x context.Context, c *api.Photos) (par, error) {
		// Las referencias se ven, y pequeñas: quien llega por un enlace no sabe de qué
		// foto se parte. Tres como mucho, y la que falle no para las demás.
		var p par
		for _, id := range ids[:min(len(ids), 3)] {
			if f, err := c.Photo(x, id); err == nil {
				p.refs = append(p.refs, f)
			}
		}
		var err error
		if len(ids) == 1 {
			p.fotos, err = c.Similar(x, ids[0], 24)
		} else {
			p.fotos, err = c.SimilarMany(x, ids, 24)
		}
		if p.fotos == nil {
			p.fotos = []api.Photo{}
		}
		return p, err
	}, func(p par, err error) {
		a.enc.cargando, a.enc.fallo = false, err
		if err != nil {
			a.enc.error = a.textoDeFallo(err)
			return
		}
		a.enc.referencias, a.enc.fotos = p.refs, p.fotos
	})
}

// AbrirCerca va a «Fotos de por aquí» de esa foto.
func (a *App) AbrirCerca(id string) {
	a.Ir(RutaCerca)
	a.enc = estadoEncontradas{ids: []string{id}, estadoCarga: estadoCarga{cargando: true}}
	a.pintar()
	type par struct {
		ref   api.Photo
		fotos []api.Photo
	}
	pedir(a, func(x context.Context, c *api.Photos) (par, error) {
		ref, err := c.Photo(x, id)
		if err != nil {
			return par{}, err
		}
		fotos, err := c.Nearby(x, id, 48)
		if fotos == nil {
			fotos = []api.Photo{}
		}
		return par{ref, fotos}, err
	}, func(p par, err error) {
		a.enc.cargando, a.enc.fallo = false, err
		if err != nil {
			a.enc.error = a.textoDeFallo(err)
			return
		}
		a.enc.referencias, a.enc.fotos = []api.Photo{p.ref}, p.fotos
		if l := p.ref.Location; l != nil {
			a.enc.coordenadas = Coordenadas(a.v, *l)
		}
	})
}

func (a *App) datosEncontradas() Encontradas {
	e := a.enc
	d := Encontradas{
		Referencias: e.referencias, Fotos: e.fotos, Cerca: a.ruta == RutaCerca, Varias: len(e.ids) > 1,
		Coordenadas: e.coordenadas, Aviso: e.aviso, Error: e.error,
		AlIr: a.irSeccion, AlVolver: func() { a.Ir(RutaFotos) },
		AlAbrir: func(f api.Photo) {
			if i := posicion(a.enc.fotos, f.ID); i >= 0 {
				a.abrirVisor(desdeEncontradas, i)
			}
		},
	}
	if a.ruta == RutaParecidas {
		d.AlGuardar = a.guardarEncontradas
	}
	return d
}

// guardarEncontradas es «Guardar como álbum» de «parecidas».
func (a *App) guardarEncontradas() {
	fotos := a.enc.fotos
	if len(fotos) == 0 {
		return
	}
	a.preguntar(a.v.Traduce("similar.saveAlbum"), a.v.T.T("gallery.newAlbumPrompt", i18n.Vars{"count": len(fotos)}),
		a.v.Traduce("gallery.newAlbumCreate"), TituloSugerido(fotos), func(titulo string) {
			a.crearCon(titulo, idsDe(fotos), func(al api.Album, err error) {
				if err != nil {
					a.enc.error = a.textoDeFallo(err)
					return
				}
				a.enc.aviso = a.v.T.T("gallery.addedToAlbum", i18n.Vars{"count": len(fotos), "title": al.Title})
			})
		})
}

// idsDe son los ids de esas fotos, en su orden.
func idsDe(fotos []api.Photo) []string {
	ids := make([]string, len(fotos))
	for i, f := range fotos {
		ids[i] = f.ID
	}
	return ids
}

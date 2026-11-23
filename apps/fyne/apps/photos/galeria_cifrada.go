// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/galeriacifrada"
)

// # La galería cifrada en nativo (ADR 0086)
//
// Las fotos que viven cifradas en «Fotos» del Drive de la bóveda, con el MISMO índice
// cifrado que la web (`nucleo/photos/galeriacifrada`): fecha de captura, medidas,
// favorita, álbumes y miniatura, escritos y leídos solo en el dispositivo. Como en la
// web:
//
//   - Entran en la línea de tiempo por fecha, con un candado
//     ([galeriacifrada.MezclarLinea]: una cifrada solo aparece cuando la línea del
//     servidor ya ha llegado a su fecha), en favoritas (las marcadas) y en la búsqueda
//     (en el dispositivo, por nombre y por mes, delante de los resultados del servidor).
//   - Se abren en el visor a pantalla completa, como las del servidor (visor_fuente.go):
//     descifradas en memoria, pasando a la anterior y la siguiente de la línea mezclada,
//     con su favorita, sus álbumes cifrados, «Descargar» (solo a donde elige la
//     persona) y «Editar» (editor_cifrado.go), que guarda una versión nueva cifrada.
//   - En «Álbumes», una sección aparte con los álbumes cifrados: crear, abrir, y desde
//     la foto, meterla o sacarla.
//   - Lo que necesita al servidor (Sytue AI, parecidas, cerca, recuerdos, selección,
//     papelera, compartir) no las incluye, y se dice (`gallery.encrypted.note`).
//
// Al servidor no llega nada: ni la foto, ni su miniatura, ni su nombre ni su fecha. En
// la línea de tiempo una cifrada es un api.Photo con el id del fichero de Drive
// precedido de [prefijoCifrada], que ninguna llamada al servidor recibe: las miniaturas
// de esos ids las descifra [miniaturasCifradas], no la red.
//
// No se abre el Drive de la bóveda por su cuenta: si no está montado, la sección
// ofrece «Mostrar», que es lo que pide desbloquearlo (como subir).

// prefijoCifrada marca en la línea de tiempo las fotos de la galería cifrada. Los ids
// de Drive no llevan «:» (galeriacifrada solo admite letras, cifras, «-» y «_»), así
// que no se confunden con los del servidor.
const prefijoCifrada = "cifrada:"

// reconstruirPorVez es cuántas entradas que faltan se rehacen por apertura (cada una
// descifra el original).
const reconstruirPorVez = 10

// EsCifrada dice si un id de la línea de tiempo es de una foto de la galería cifrada.
func EsCifrada(id string) bool { return strings.HasPrefix(id, prefijoCifrada) }

func idDeLinea(fichero string) string { return prefijoCifrada + fichero }

// FotoDeLinea es una foto cifrada tal y como va en la línea de tiempo.
func FotoDeLinea(f galeriacifrada.Foto) api.Photo {
	return api.Photo{
		ID: idDeLinea(f.Fichero.ID), Name: nombreCifrada(f), ContentType: f.Fichero.ContentType,
		SizeBytes: f.Fichero.SizeBytes, Width: f.Entrada.Ancho, Height: f.Entrada.Alto,
		TakenAt: f.Entrada.Tomada, TakenFromEXIF: f.Entrada.Exif, Favorite: f.Entrada.Favorita,
	}
}

func nombreCifrada(f galeriacifrada.Foto) string {
	if f.Entrada.Nombre != "" {
		return f.Entrada.Nombre
	}
	return f.Fichero.Name
}

// estadoCifradas es lo que la galería sabe de la galería cifrada.
type estadoCifradas struct {
	pedida bool
	// enBoveda: el Drive de la persona está en la bóveda; sin ello no hay sección.
	enBoveda bool
	cerrada  bool
	cargando bool
	fallo    string
	galeria  *galeriacifrada.Galeria
	drive    *api.Drive
	indice   galeriacifrada.Indice
	// album es el álbum cifrado abierto en «Álbumes»; "", ninguno.
	album string
}

// miniaturasCifradas descifra, a petición de la Fototeca, la miniatura de una foto
// cifrada y, para el visor, su foto grande. Vive aparte del estado de la app porque la
// Fototeca pide desde sus propias goroutines; el estado se cambia en el hilo de la
// interfaz con [miniaturasCifradas.poner].
type miniaturasCifradas struct {
	mu      sync.Mutex
	galeria *galeriacifrada.Galeria
	fotos   map[string]galeriacifrada.Foto
}

func (m *miniaturasCifradas) poner(g *galeriacifrada.Galeria, indice galeriacifrada.Indice) {
	if m == nil {
		return
	}
	fotos := make(map[string]galeriacifrada.Foto, len(indice.Fotos))
	for _, f := range indice.Fotos {
		fotos[f.Fichero.ID] = f
	}
	m.mu.Lock()
	m.galeria, m.fotos = g, fotos
	m.mu.Unlock()
}

// de es la galería y la foto del id de la línea `id`.
func (m *miniaturasCifradas) de(id string) (*galeriacifrada.Galeria, galeriacifrada.Foto, bool) {
	if m == nil {
		return nil, galeriacifrada.Foto{}, false
	}
	fichero, _ := strings.CutPrefix(id, prefijoCifrada)
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.fotos[fichero]
	return m.galeria, f, ok && m.galeria != nil
}

// leer es la miniatura descifrada del id de la línea `id`.
func (m *miniaturasCifradas) leer(ctx context.Context, id string) ([]byte, error) {
	g, f, ok := m.de(id)
	if !ok || f.Entrada.Miniatura == "" {
		return nil, galeriacifrada.ErrSinFoto
	}
	return g.LeerMiniatura(ctx, f.Entrada)
}

// grande es la foto grande del visor de una cifrada: el original descifrado en memoria,
// orientado y reducido a [galeriacifrada.LadoVista], como la variante «grande» del
// servidor. Si el original no se puede (demasiado grande, un formato que aquí no se
// decodifica), la miniatura.
func (m *miniaturasCifradas) grande(ctx context.Context, id string) ([]byte, error) {
	g, f, ok := m.de(id)
	if !ok {
		return nil, galeriacifrada.ErrSinFoto
	}
	if datos, err := g.Vista(ctx, f.Fichero, galeriacifrada.LadoVista); err == nil {
		return datos, nil
	} else if ctx.Err() != nil {
		return nil, err
	}
	return m.leer(ctx, id)
}

// desviarCifradas hace que las Fototecas de la galería pidan las miniaturas de las fotos
// cifradas a [miniaturasCifradas] y nunca al servidor. Se llama al arrancar, antes de
// la primera descarga y después de que el binario ponga `Pedir`.
func (a *App) desviarCifradas() {
	if a.GaleriaCifrada == nil || a.cifDesviadas {
		return
	}
	a.cifDesviadas = true
	desviar := func(t *Fototeca, cifrada func(context.Context, string) ([]byte, error)) {
		if t == nil {
			return
		}
		red := t.Pedir
		t.Pedir = func(ctx context.Context, id, variante string) ([]byte, error) {
			if EsCifrada(id) {
				return cifrada(ctx, id)
			}
			if red == nil {
				return nil, errors.New("photos: sin red")
			}
			return red(ctx, id, variante)
		}
	}
	desviar(a.v.Teca, a.minisCif.leer)
	// La del visor enseña la foto grande, descifrada en el dispositivo; si es la misma
	// Fototeca que la de la galería, la miniatura.
	if a.v.TecaGrande != a.v.Teca {
		desviar(a.v.TecaGrande, a.minisCif.grande)
	}
}

// cargarCifradas lee la galería cifrada. Con `abrir`, pide desbloquear el Drive de la
// bóveda si aún no lo está.
func (a *App) cargarCifradas(abrir bool) {
	if a.GaleriaCifrada == nil || a.cif.cargando {
		return
	}
	album, antes := a.cif.album, sumasCifradas(a.cif.indice)
	a.cif.pedida, a.cif.cargando, a.cif.fallo = true, true, ""
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		c, fin := context.WithTimeout(vida, PlazoSubida(0))
		defer fin()
		drive, err := a.GaleriaCifrada(c, abrir)
		if drive == nil && err == nil {
			fyne.Do(func() { a.cif = estadoCifradas{pedida: true} })
			return
		}
		var leido estadoCifradas
		if err == nil {
			leido, err = leerCifradas(c, drive)
		}
		fyne.Do(func() {
			if vida.Err() != nil {
				return
			}
			a.cif = leido
			a.cif.pedida, a.cif.enBoveda, a.cif.album = true, true, album
			switch {
			case errors.Is(err, ErrDriveCifradoCerrado):
				a.cif.cerrada = true
			case err != nil:
				clave := motivoSubida(err)
				if clave == "" {
					clave = "gallery.encrypted.failed"
				}
				a.cif.fallo = a.v.Traduce(clave)
			}
			a.minisCif.poner(a.cif.galeria, a.cif.indice)
			// La miniatura de lo que cambió (otra versión, una que faltaba) se vuelve a
			// pedir; la de lo demás se queda en la caché.
			for id, suma := range sumasCifradas(a.cif.indice) {
				if previa, ok := antes[id]; ok && previa != suma && a.v.Teca != nil {
					a.v.Teca.Tirar(idDeLinea(id))
				}
			}
			if gen == a.gen {
				//nolint:contextcheck // Sólo reconstruye UI.
				a.pintar()
			}
		})
	})
}

// sumasCifradas es, por foto, lo que identifica su miniatura: la suma del contenido
// indexado y el fichero de la miniatura.
func sumasCifradas(indice galeriacifrada.Indice) map[string]string {
	out := make(map[string]string, len(indice.Fotos))
	for _, f := range indice.Fotos {
		out[f.Fichero.ID] = f.Entrada.Suma + "\x00" + f.Entrada.Miniatura
	}
	return out
}

// leerCifradas lee el índice y rehace unas pocas entradas pendientes. Las miniaturas
// se descifran después, las que se vayan a ver.
func leerCifradas(ctx context.Context, drive *api.Drive) (estadoCifradas, error) {
	g := galeriacifrada.Nueva(drive, nil)
	indice, err := g.Leer(ctx)
	if err != nil {
		return estadoCifradas{}, err
	}
	indice = reconstruirPendientes(ctx, g, indice)
	return estadoCifradas{galeria: g, drive: drive, indice: indice}, nil
}

// reconstruirPendientes rehace unas pocas entradas que faltan o son de otro contenido
// y vuelve a leer el índice si cambió algo. Lo que no se rehace sigue saliendo con lo
// que dice Drive: nunca se pierde nada.
func reconstruirPendientes(ctx context.Context, g *galeriacifrada.Galeria, indice galeriacifrada.Indice) galeriacifrada.Indice {
	hechas := 0
	for _, f := range indice.Fotos {
		if hechas >= reconstruirPorVez || ctx.Err() != nil {
			break
		}
		if f.Pendiente && g.Reconstruir(ctx, f.Fichero) == nil {
			hechas++
		}
	}
	if hechas == 0 {
		return indice
	}
	if nuevo, err := g.Leer(ctx); err == nil {
		return nuevo
	}
	return indice
}

// cifradasDeLaVista son las que van en la línea abierta: todas o, en favoritas, las
// marcadas; con una búsqueda, las que coinciden por nombre o por mes.
func (a *App) cifradasDeLaVista() []galeriacifrada.Foto {
	fotos := a.cif.indice.Fotos
	if a.soloFavoritas() {
		fotos = a.cif.indice.Favoritas()
	}
	var fecha func(time.Time) string
	if a.v.T != nil {
		locale := a.v.T.Locale()
		fecha = func(t time.Time) string { return i18n.FormatMonthYear(locale, t) }
	}
	return galeriacifrada.Buscar(fotos, a.g.texto, fecha)
}

// lineaMezclada es la línea de tiempo que se enseña: la del servidor con las cifradas
// por fecha (sin galería cifrada leída, la del servidor tal cual).
func (a *App) lineaMezclada() []api.Photo {
	if a.cif.galeria == nil || len(a.cif.indice.Fotos) == 0 {
		return a.g.fotos
	}
	l := galeriacifrada.MezclarLinea(a.g.fotos, func(p api.Photo) time.Time { return p.TakenAt }, a.cifradasDeLaVista(),
		galeriacifrada.OpcionesLinea{Fin: a.g.fin, Anyo: a.g.anyo, Busqueda: strings.TrimSpace(a.g.texto) != ""})
	out := make([]api.Photo, len(l))
	for i, e := range l {
		if e.EsCifrada {
			out[i] = FotoDeLinea(e.Cifrada)
		} else {
			out[i] = e.Servidor
		}
	}
	return out
}

// hayCifradas: la galería cifrada está leída y tiene fotos. Es cuando las pantallas
// que pasan por el servidor (parecidas, cerca) dicen que no las incluyen.
func (a *App) hayCifradas() bool { return a.cif.galeria != nil && len(a.cif.indice.Fotos) > 0 }

// cifradaDeLinea es la foto cifrada de un id de la línea.
func (a *App) cifradaDeLinea(id string) (galeriacifrada.Foto, bool) {
	fichero, ok := strings.CutPrefix(id, prefijoCifrada)
	if !ok {
		return galeriacifrada.Foto{}, false
	}
	for _, f := range a.cif.indice.Fotos {
		if f.Fichero.ID == fichero {
			return f, true
		}
	}
	return galeriacifrada.Foto{}, false
}

// abrirDeLinea abre una foto de la línea en el visor a pantalla completa: la del
// servidor y la cifrada, que el visor descifra en el dispositivo (visor_fuente.go).
func (a *App) abrirDeLinea(f api.Photo) { a.abrirEnGaleria(f.ID) }

// fotosAlbumCifrado son las fotos del álbum cifrado abierto en «Álbumes», como van en la
// línea de tiempo: la lista por la que pasa el visor abierto desde ahí.
func (a *App) fotosAlbumCifrado() []api.Photo {
	if a.cif.album == "" {
		return nil
	}
	fotos := a.cif.indice.DelAlbum(a.cif.album)
	out := make([]api.Photo, len(fotos))
	for i, f := range fotos {
		out[i] = FotoDeLinea(f)
	}
	return out
}

// favoritaDeLinea es el corazón de una tesela de la línea: la cifrada, solo en la
// bóveda; la del servidor, en el servidor.
func (a *App) favoritaDeLinea(f api.Photo) {
	if c, ok := a.cifradaDeLinea(f.ID); ok {
		a.favoritaCifrada(c, !c.Entrada.Favorita)
		return
	}
	a.cambiarFavorita(f, !f.Favorite)
}

// cambiarCifrada pone en el índice leído lo que devolvió una escritura.
func (a *App) cambiarCifrada(f galeriacifrada.Foto) {
	for i := range a.cif.indice.Fotos {
		if a.cif.indice.Fotos[i].Fichero.ID == f.Fichero.ID {
			a.cif.indice.Fotos[i] = f
		}
	}
	a.minisCif.poner(a.cif.galeria, a.cif.indice)
}

// seccionCifradas es la cabecera de la galería cifrada en la línea de tiempo; nil si la
// persona no tiene el Drive en la bóveda (o la app no sabe de él).
func (a *App) seccionCifradas() fyne.CanvasObject {
	if a.GaleriaCifrada == nil || !a.cif.enBoveda {
		return nil
	}
	return SeccionCifradas(a.v, a.datosCifradas())
}

func (a *App) datosCifradas() DatosCifradas {
	return DatosCifradas{
		Cerrada:   a.cif.cerrada,
		Cargando:  a.cif.cargando,
		Fallo:     a.cif.fallo,
		Vacia:     a.cif.galeria != nil && len(a.cif.indice.Fotos) == 0,
		AlMostrar: func() { a.cargarCifradas(true) },
	}
}

// DatosCifradas es lo que pinta [SeccionCifradas].
type DatosCifradas struct {
	Cerrada, Cargando bool
	Fallo             string
	// Vacia: el índice se leyó y no hay ninguna foto.
	Vacia     bool
	AlMostrar func()
}

// SeccionCifradas es «En tu Drive cifrado»: el estado de la galería cifrada (cerrada con
// «Mostrar», descifrando, el fallo o vacía) y el aviso de lo que no incluye a esas
// fotos (Sytue AI, parecidas, cerca, recuerdos). Las fotos van en la línea de tiempo.
func SeccionCifradas(v Vista, d DatosCifradas) fyne.CanvasObject {
	return recuadro(v, "", "border", tema.RadioMD,
		ui.Margen{Izq: tema.Espacio(4), Der: tema.Espacio(4), Arr: tema.Espacio(3), Abj: tema.Espacio(3)},
		ui.PilaBloques(bloquesCifradas(v, "gallery.encrypted.title", d, nil)...))
}

// bloquesCifradas son la cabecera (candado, título, «Mostrar» y `extra`), la nota y el
// estado.
func bloquesCifradas(v Vista, titulo string, d DatosCifradas, extra fyne.CanvasObject) []ui.Bloque {
	cabecera := []fyne.CanvasObject{
		iconos.Trazo(iconos.Candado, iconos.TallaSM, "text", v.Variante).Objeto(),
		ParrafoEn(v.Traduce(titulo), tema.TextoSM, ui.SemiNegrita, "text", v.Variante),
	}
	if d.Cerrada && d.AlMostrar != nil {
		cabecera = append(cabecera, ui.NuevoBoton(v.Traduce("gallery.encrypted.show"), d.AlMostrar).Con(ui.Fantasma, ui.TallaXS))
	} else if extra != nil {
		cabecera = append(cabecera, extra)
	}
	bloques := []ui.Bloque{
		{Obj: container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: 2}, cabecera...), Margen: tema.Espacio(1)},
		{Obj: ui.NuevoParrafo(v.Traduce("gallery.encrypted.note"), tema.TextoXS, ui.Normal, "text-muted", v.Variante)},
	}
	var estado fyne.CanvasObject
	switch {
	case d.Cargando:
		estado = ParrafoEn(v.Traduce("gallery.encrypted.opening"), tema.TextoSM, ui.Normal, "text-muted", v.Variante)
	case d.Fallo != "":
		estado = ui.NuevoParrafo(d.Fallo, tema.TextoSM, ui.Normal, "danger-text", v.Variante)
	case d.Cerrada:
	case d.Vacia:
		vacio := ui.NuevoVacio(v.Traduce("gallery.encrypted.empty"), "")
		vacio.Compacto = true
		estado = vacio
	}
	if estado != nil {
		bloques[len(bloques)-1].Margen = tema.Espacio(2)
		bloques = append(bloques, ui.Bloque{Obj: estado})
	}
	return bloques
}

// escribirCifrada corre una escritura en la galería cifrada y, al volver, pone la foto
// cambiada en el índice o dice que no se pudo.
func (a *App) escribirCifrada(escribir func(context.Context, *galeriacifrada.Galeria) (galeriacifrada.Foto, error)) {
	g := a.cif.galeria
	if g == nil {
		return
	}
	a.enSesion(func(vida context.Context) {
		c, fin := ctx(vida)
		defer fin()
		f, err := escribir(c, g)
		fyne.Do(func() {
			if vida.Err() != nil || a.cif.galeria != g {
				return
			}
			if err != nil {
				a.avisar(ui.AvisoPeligro, a.v.Traduce("gallery.encrypted.actionFailed"), "")
			} else if f.Fichero.ID != "" {
				a.cambiarCifrada(f)
			}
			a.pintar()
		})
	})
}

// favoritaCifrada marca o desmarca una foto cifrada, solo en la bóveda.
func (a *App) favoritaCifrada(f galeriacifrada.Foto, valor bool) {
	a.escribirCifrada(func(c context.Context, g *galeriacifrada.Galeria) (galeriacifrada.Foto, error) {
		e, err := g.Actualizar(c, f.Fichero, func(e *galeriacifrada.Entrada) { e.Favorita = valor })
		f.Entrada, f.ConEntrada = e, true
		return f, err
	})
}

// albumCifrado mete (`dentro`) o saca una foto de un álbum cifrado, solo en su entrada
// del índice.
func (a *App) albumCifrado(f galeriacifrada.Foto, album string, dentro bool) {
	a.escribirCifrada(func(c context.Context, g *galeriacifrada.Galeria) (galeriacifrada.Foto, error) {
		e, err := g.PonerEnAlbum(c, f.Fichero, album, dentro)
		f.Entrada, f.ConEntrada = e, true
		return f, err
	})
}

// nuevoAlbumCifrado pregunta el título y crea un álbum cifrado; con `con`, mete esa foto.
// El álbum nuevo queda abierto en «Álbumes».
func (a *App) nuevoAlbumCifrado(con *galeriacifrada.Foto) {
	g := a.cif.galeria
	if g == nil {
		return
	}
	a.preguntar(a.v.Traduce("gallery.encrypted.newAlbum"), a.v.Traduce("albums.title"), a.v.Traduce("gallery.newAlbumCreate"), "", func(titulo string) {
		a.enSesion(func(vida context.Context) {
			c, fin := ctx(vida)
			defer fin()
			album, err := g.CrearAlbum(c, titulo)
			var foto galeriacifrada.Foto
			if err == nil && con != nil {
				foto = *con
				foto.Entrada, err = g.PonerEnAlbum(c, con.Fichero, album.ID, true)
				foto.ConEntrada = true
			}
			fyne.Do(func() {
				if vida.Err() != nil || a.cif.galeria != g {
					return
				}
				if album.ID != "" {
					a.cif.indice.Albumes = append(a.cif.indice.Albumes, album)
					slices.SortFunc(a.cif.indice.Albumes, func(x, y galeriacifrada.Album) int {
						return strings.Compare(strings.ToLower(x.Titulo), strings.ToLower(y.Titulo))
					})
					a.cif.album = album.ID
				}
				if err != nil {
					a.avisar(ui.AvisoPeligro, a.v.Traduce("gallery.encrypted.actionFailed"), "")
				} else if foto.Fichero.ID != "" {
					a.cambiarCifrada(foto)
				}
				a.pintar()
			})
		})
	})
}

// ---- Los álbumes cifrados ----

// DatosAlbumesCifrados es lo que pinta [SeccionAlbumesCifrados].
type DatosAlbumesCifrados struct {
	Estado  DatosCifradas
	Albumes []api.Album
	// Elegido es el álbum abierto; Fotos, las suyas (como van en la línea de tiempo).
	Elegido *api.Album
	Fotos   []api.Photo
	// AlNuevo crea un álbum cifrado; nil, todavía no se puede (sin índice leído).
	AlNuevo  func()
	AlElegir func(id string)
	AlAbrir  func(api.Photo)
	// AlFavorita marca o desmarca una foto del álbum abierto.
	AlFavorita func(api.Photo)
}

// SeccionAlbumesCifrados es «Álbumes cifrados» en la página de álbumes, como la web
// (AlbumesCifrados.tsx): el estado de la galería cifrada, las tarjetas y, del elegido,
// sus fotos. Los álbumes se crean y se llenan solo en la bóveda.
func SeccionAlbumesCifrados(v Vista, d DatosAlbumesCifrados) fyne.CanvasObject {
	var nuevo fyne.CanvasObject
	if d.AlNuevo != nil {
		nuevo = ui.NuevoBoton(v.Traduce("gallery.encrypted.newAlbum"), d.AlNuevo).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Mas, 0)
	}
	bloques := bloquesCifradas(v, "gallery.encrypted.albums", d.Estado, nuevo)
	if len(d.Albumes) > 0 {
		tarjetas := make([]fyne.CanvasObject, len(d.Albumes))
		for i, al := range d.Albumes {
			tarjetas[i] = TarjetaAlbum(v, al, func(al api.Album) {
				if d.AlElegir != nil {
					d.AlElegir(al.ID)
				}
			})
		}
		r := ui.RejillaAlbumes()
		r.Hueco = tema.Espacio(6)
		bloques[len(bloques)-1].Margen = tema.Espacio(4)
		bloques = append(bloques, ui.Bloque{
			Obj:  container.New(r, tarjetas...),
			Alto: func(ancho float32) float32 { return r.MinSizeCon(tarjetas, ancho).Height },
		})
	}
	if d.Elegido != nil {
		bloques[len(bloques)-1].Margen = tema.Espacio(6)
		bloques = append(bloques, ui.Bloque{
			Obj:    ParrafoEn(d.Elegido.Title, tema.TextoSM, ui.SemiNegrita, "text", v.Variante),
			Margen: tema.Espacio(2),
		})
		if len(d.Fotos) == 0 {
			vacio := ui.NuevoVacio(v.Traduce("gallery.encrypted.albumEmpty"), "")
			vacio.Compacto = true
			bloques = append(bloques, ui.Bloque{Obj: vacio})
		} else {
			bloques = append(bloques, ui.Bloque{Obj: rejillaDeFotos(v, d.Fotos, Galeria{AlAbrir: d.AlAbrir, AlFavorita: d.AlFavorita})})
		}
	}
	return ui.PilaBloques(bloques...)
}

// seccionAlbumesCifrados es la sección de la página de álbumes; nil sin galería cifrada.
func (a *App) seccionAlbumesCifrados() fyne.CanvasObject {
	if a.GaleriaCifrada == nil || !a.cif.enBoveda {
		return nil
	}
	d := DatosAlbumesCifrados{
		Estado:   a.datosCifradas(),
		AlElegir: a.elegirAlbumCifrado,
		AlAbrir: func(f api.Photo) {
			if i := posicion(a.fotosAlbumCifrado(), f.ID); i >= 0 {
				a.abrirVisor(desdeAlbumCifrado, i)
			}
		},
		AlFavorita: func(f api.Photo) {
			if c, ok := a.cifradaDeLinea(f.ID); ok {
				a.favoritaCifrada(c, !c.Entrada.Favorita)
			}
		},
	}
	// Sin fotos el estado lo dice la línea de tiempo: aquí basta con los álbumes.
	d.Estado.Vacia = false
	if a.cif.galeria != nil {
		d.AlNuevo = func() { a.nuevoAlbumCifrado(nil) }
	}
	cuantas, portadas := map[string]int{}, map[string]string{}
	for _, f := range a.cif.indice.Fotos {
		for _, al := range f.Entrada.Albumes {
			cuantas[al]++
			if _, ya := portadas[al]; !ya && f.Entrada.Miniatura != "" {
				portadas[al] = idDeLinea(f.Fichero.ID)
			}
		}
	}
	for _, al := range a.cif.indice.Albumes {
		album := api.Album{ID: al.ID, Title: al.Titulo, Count: cuantas[al.ID], CoverAssetID: portadas[al.ID]}
		d.Albumes = append(d.Albumes, album)
		if al.ID == a.cif.album {
			d.Elegido = &album
		}
	}
	if d.Elegido != nil {
		d.Fotos = a.fotosAlbumCifrado()
	}
	return SeccionAlbumesCifrados(a.v, d)
}

// elegirAlbumCifrado abre un álbum cifrado o, si ya lo estaba, lo cierra.
func (a *App) elegirAlbumCifrado(id string) {
	if a.cif.album == id {
		a.cif.album = ""
	} else {
		a.cif.album = id
	}
	a.pintar()
}

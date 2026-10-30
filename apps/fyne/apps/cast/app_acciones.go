// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"context"
	"errors"
	"image"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/fyne/llamada/camara"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// iconoBuscar es la lupa del buscador de Explorar.
const iconoBuscar = iconos.Buscar

// ---- Subir ----

func (a *App) pedirCuota() {
	if a.cli == nil || !a.dentro() {
		return
	}
	gen := a.gen
	view := a.Widget.View()
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		q, err := a.cli.Quota(c)
		if err == nil {
			if counter, valid := widgets.StoragePercent(q.UsedBytes, q.LimitBytes); valid && q.ExternalStorage != nil && !*q.ExternalStorage {
				_ = view.Publicar(c, counter)
			} else {
				// An older backend cannot distinguish plan usage from an external
				// store. Unknown/unlimited capacity also retires the previous percent.
				_ = view.Publicar(c)
			}
		}
		a.hilo(gen, func() {
			if err == nil {
				a.sub.cuota, a.sub.errorCuota = &q, false
			} else {
				a.sub.errorCuota = true
			}
			a.pintar()
		})
	})
}

func (a *App) pantallaSubir() fyne.CanvasObject {
	if !a.dentro() {
		return PantallaSubir(a.sinSesion(), Subir{})
	}
	s := a.sub
	return PantallaSubir(a.v, Subir{
		Cuota: s.cuota, ErrorCuota: s.errorCuota, Visibilidad: s.visibilidad,
		Arrastrando: a.arrastrando, Cola: s.cola,
		Preparados: s.preparados, Calidades: a.selCalidadesSubir(),
		AlEmpezar: a.empezarSubida, AlDescarte: a.descartarSubida,
		AlElegir:      a.elegirFicheros,
		AlVisibilidad: func(vis string) { a.sub.visibilidad = vis; a.pintar() },
		AlCancelar:    a.cancelarEnvio,
		AlQuitar:      a.quitarEnvio,
		AlEditar:      a.AbrirGestionar,
	})
}

// Arrastrando es lo que avisa `soltar.AlArrastrar`: la zona se resalta.
func (a *App) Arrastrando(si bool) {
	if a.arrastrando != si {
		a.arrastrando = si
		if a.ruta == RutaSubir {
			a.pintar()
		}
	}
}

func (a *App) elegirFicheros() {
	if a.Elegir == nil {
		return
	}
	a.lanzar(func() {
		rutas, err := a.Elegir()
		if err != nil || len(rutas) == 0 {
			return
		}
		a.enLaInterfaz(func() { a.anadir(rutas) })
	})
}

// tituloDe es el título que nace de un nombre de fichero: sin la extensión y sin los
// guiones bajos (tituloDe de cola.ts).
func tituloDe(nombre string) string {
	sin := strings.TrimSuffix(nombre, filepath.Ext(nombre))
	sin = strings.Join(strings.Fields(strings.ReplaceAll(sin, "_", " ")), " ")
	if sin == "" {
		sin = nombre
	}
	if r := []rune(sin); len(r) > 200 {
		sin = string(r[:200])
	}
	return sin
}

// anadir mete ficheros en la cola: lo que no es vídeo o pasa de 8 GiB entra ya
// rechazado, sin gastar un byte; lo demás espera en «Antes de subir» a que se
// confirmen las calidades (preparar).
func (a *App) anadir(rutas []string) {
	var listos []string
	for _, ruta := range rutas {
		nombre := filepath.Base(ruta)
		var tam int64
		if fi, err := os.Stat(ruta); err == nil {
			tam = fi.Size()
		}
		if esVideo(nombre) && tam <= TopeVideo {
			listos = append(listos, ruta)
			continue
		}
		a.encolar([]string{ruta}, nil)
	}
	a.preparar(listos)
	a.pintar()
}

// encolar mete ficheros en la cola de subida con esas calidades (vacía, la
// recomendada) y arranca la cola.
func (a *App) encolar(rutas []string, calidades []int) {
	for _, ruta := range rutas {
		a.sub.siguiente++
		clave := a.sub.siguiente
		nombre := filepath.Base(ruta)
		var tam int64
		if fi, err := os.Stat(ruta); err == nil {
			tam = fi.Size()
		}
		campo := ui.NuevaEntrada(a.v.Traduce("upload.titleField")).Con(ui.CampoSM)
		campo.PonerTexto(tituloDe(nombre))
		campo.AlTeclear = alIntro(func() { a.guardarTitulo(clave) })
		e := Envio{Clave: clave, Ruta: ruta, Nombre: nombre, Tamano: tam, Titulo: campo, visibilidad: a.sub.visibilidad, calidades: calidades}
		switch {
		case !esVideo(nombre):
			e.Fase, e.Motivo = Fallida, a.v.Traduce("upload.error.tipo")
		case tam > TopeVideo:
			e.Fase, e.Motivo = Fallida, a.v.Traduce("upload.error.grande")
		}
		if e.Fase == Fallida {
			a.soltarCopia(ruta) // no se va a subir: la copia del teléfono sobra ya
		}
		a.sub.cola = append(a.sub.cola, e)
	}
	a.pintar()
	a.arrancarCola()
}

// envio devuelve el fichero de la cola con esa clave, o nil.
func (a *App) envio(clave int) *Envio {
	for i := range a.sub.cola {
		if a.sub.cola[i].Clave == clave {
			return &a.sub.cola[i]
		}
	}
	return nil
}

// arrancarCola sube el siguiente que espera, si no hay ninguno subiendo: de uno en
// uno, como la web (el ancho de banda es el mismo y así el primero acaba antes).
func (a *App) arrancarCola() {
	if a.cli == nil {
		return
	}
	for _, e := range a.sub.cola {
		if e.Fase == Subiendo {
			return
		}
	}
	for i := range a.sub.cola {
		e := &a.sub.cola[i]
		if e.Fase != Esperando {
			continue
		}
		e.Fase = Subiendo
		c, cancelar := context.WithCancel(context.Background())
		a.sub.cancelar[e.Clave] = cancelar
		clave, ruta, titulo, vis, cals := e.Clave, e.Ruta, strings.TrimSpace(e.Titulo.Texto()), e.visibilidad, e.calidades
		if titulo == "" {
			titulo = tituloDe(e.Nombre)
		}
		a.pintar()
		a.lanzar(func() { a.enviar(c, clave, ruta, titulo, vis, cals) })
		return
	}
}

// actualizarEnvio cambia un fichero de la cola desde la rutina que sube.
func (a *App) actualizarEnvio(clave int, f func(e *Envio)) {
	a.enLaInterfaz(func() {
		if e := a.envio(clave); e != nil {
			f(e)
		}
		if a.ruta == RutaSubir {
			a.pintar()
		}
	})
}

// enviar da de alta el vídeo y sube el original en streaming; corre fuera de la
// interfaz. Un registro sin original no sirve: si la subida falla, se borra.
//
//nolint:contextcheck // lo que avisa a la interfaz repinta la pantalla, que no es del contexto de la subida
func (a *App) enviar(c context.Context, clave int, ruta, titulo, vis string, calidades []int) {
	abrir := a.AbrirFichero
	if abrir == nil {
		abrir = abrirLocal
	}
	cc, fin := context.WithTimeout(c, time.Minute)
	vid, err := a.cli.Create(cc, api.CastNew{Title: titulo, Visibility: vis, Qualities: calidades})
	fin()
	if err == nil {
		a.actualizarEnvio(clave, func(e *Envio) { e.VideoID = vid.ID })
	}
	var f io.ReadCloser
	var tam int64
	if err == nil {
		f, tam, err = abrir(ruta)
	}
	if err == nil {
		var enviado atomic.Int64
		parar := make(chan struct{})
		go a.progreso(clave, &enviado, parar)
		tipo := mime.TypeByExtension(strings.ToLower(filepath.Ext(ruta)))
		if !strings.HasPrefix(tipo, "video/") {
			tipo = "video/mp4"
		}
		// Sin tope de tiempo: 8 GiB por una conexión doméstica son horas.
		_, err = a.cli.Upload(c, vid.ID, &contador{r: f, n: &enviado}, tam, tipo)
		close(parar)
		_ = f.Close()
	}
	if err != nil {
		motivo := a.motivoDeFallo(c, err)
		if vid.ID != "" {
			bc, bfin := ctx()
			_ = a.cli.Delete(bc, vid.ID) //nolint:contextcheck // borrar el registro no depende de la subida cancelada
			bfin()
		}
		a.actualizarEnvio(clave, func(e *Envio) { e.Fase, e.Motivo, e.VideoID = Fallida, motivo, "" })
	} else {
		a.actualizarEnvio(clave, func(e *Envio) { e.Fase, e.Enviado = Procesando, tam })
	}
	a.soltarCopia(ruta)
	a.enLaInterfaz(func() {
		delete(a.sub.cancelar, clave)
		if err == nil {
			a.guardarTitulo(clave) // si cambió mientras subía
			if a.ruta == RutaSubir {
				a.pedirCuota()
			}
		}
		a.arrancarCola()
	})
}

// motivoDeFallo es el `upload.error.*` que le toca a un fallo de subida.
func (a *App) motivoDeFallo(c context.Context, err error) string {
	var e *api.Error
	switch {
	case c.Err() != nil:
		return a.v.Traduce("upload.error.cancelada")
	case errors.As(err, &e) && e.Status == 507:
		if quedan, ok := api.CastQuotaAvailable(err); ok && quedan >= 0 {
			return a.v.Traduce("upload.error.cuotaQuedan", i18n.Vars{"available": Tamano(a.v, quedan)})
		}
		return a.v.Traduce("upload.error.cuota")
	case errors.As(err, &e) && e.Status == 413:
		return a.v.Traduce("upload.error.grande")
	case errors.As(err, &e):
		return a.v.Traduce("upload.error.servicio")
	}
	return a.v.Traduce("upload.error.red")
}

// progreso pone al día la barra cuatro veces por segundo mientras se sube.
func (a *App) progreso(clave int, enviado *atomic.Int64, parar chan struct{}) {
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-parar:
			return
		case <-t.C:
			n := enviado.Load()
			a.actualizarEnvio(clave, func(e *Envio) { e.Enviado = n })
		}
	}
}

// guardarTitulo guarda el título escrito en la fila, si el vídeo ya existe.
func (a *App) guardarTitulo(clave int) {
	e := a.envio(clave)
	if e == nil || e.VideoID == "" || a.cli == nil {
		return
	}
	titulo := strings.TrimSpace(e.Titulo.Texto())
	if titulo == "" || titulo == e.guardado {
		return
	}
	e.guardado = titulo
	id := e.VideoID
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		_, _ = a.cli.Update(c, id, api.CastChanges{Title: &titulo})
	})
}

// cancelarEnvio corta la subida en curso o saca de la espera.
func (a *App) cancelarEnvio(clave int) {
	if cancelar, ok := a.sub.cancelar[clave]; ok {
		cancelar()
		return
	}
	if e := a.envio(clave); e != nil && e.Fase == Esperando {
		a.soltarCopia(e.Ruta)
		e.Fase, e.Motivo = Fallida, a.v.Traduce("upload.error.cancelada")
		a.pintar()
	}
}

// quitarEnvio saca de la lista lo que ya no sube.
func (a *App) quitarEnvio(clave int) {
	cola := a.sub.cola[:0:0]
	for _, e := range a.sub.cola {
		if e.Clave != clave {
			cola = append(cola, e)
		}
	}
	a.sub.cola = cola
	a.pintar()
}

// abrirLocal abre un fichero del disco con su tamaño.
func abrirLocal(ruta string) (io.ReadCloser, int64, error) {
	f, err := os.Open(ruta) //nolint:gosec // G304: es el fichero que eligió la persona
	if err != nil {
		return nil, 0, err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	return f, fi.Size(), nil
}

// contador cuenta lo leído: es el progreso de la subida.
type contador struct {
	r io.Reader
	n *atomic.Int64
}

func (c *contador) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))
	return n, err
}

// ---- Subir desde Sytue Drive ----

// AbrirDesdeDrive enseña «Subir a Cast» del fichero de Drive `id` (lo que abre la
// acción del menú de Drive, `/upload?drive=<id>`).
func (a *App) AbrirDesdeDrive(id string) {
	a.apuntar(lugar{ruta: RutaDesdeDrive, param: id})
	a.salirDeVer()
	a.ruta = RutaDesdeDrive
	a.nuevaPantalla()
	a.scroll.Offset = fyne.NewPos(0, 0)
	a.dri = estadoDrive{id: IDDeDrive(id), visibilidad: api.CastOrg, libres: -1}
	a.tituloDrive.PonerTexto("")
	a.pintar()
	a.pedirDesdeDrive()
}

// pedirDesdeDrive lee el fichero: su nombre, tamaño y tipo, y si se puede copiar.
func (a *App) pedirDesdeDrive() {
	if a.cli == nil || !a.dentro() || a.dri.id == "" {
		return
	}
	gen, id := a.gen, a.dri.id
	a.dri.fichero, a.dri.fallo, a.dri.cargando = nil, nil, true
	a.pintar()
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		f, err := a.cli.DriveFile(c, id)
		a.hilo(gen, func() {
			a.dri.cargando, a.dri.fallo = false, err
			if err != nil {
				if quedan, ok := api.CastQuotaAvailable(err); ok {
					a.dri.libres = quedan
				}
			} else {
				a.dri.fichero = &f
				a.tituloDrive.PonerTexto(f.Title)
				a.estimarDrive(f.SizeBytes)
			}
			a.pintar()
		})
	})
}

func (a *App) pantallaDesdeDrive() fyne.CanvasObject {
	if !a.dentro() {
		return PantallaDesdeDrive(a.sinSesion(), DesdeDrive{ID: a.dri.id})
	}
	d := a.dri
	motivo := DriveBien
	if d.fallo != nil {
		motivo = motivoDrive(d.fallo)
	}
	v := a.v
	v.AlReintentar = a.pedirDesdeDrive
	return PantallaDesdeDrive(v, DesdeDrive{
		ID: d.id, Fichero: d.fichero, Motivo: motivo, Libres: d.libres,
		Titulo: a.tituloDrive, Visibilidad: d.visibilidad, Enviando: d.enviando, ErrorEnvio: d.errorEnvio,
		Calidades:     a.selCalidadesDrive(),
		AlVisibilidad: a.cambiarVisDrive,
		AlSubir:       a.subirDesdeDrive,
		AlReintentar:  a.pedirDesdeDrive,
		AlAbrirDrive: func() {
			if a.AbrirDrive != nil {
				a.AbrirDrive()
			}
		},
		AlOtro: func() { a.Ir(RutaSubir) },
	})
}

func (a *App) cambiarVisDrive(vis string) { a.dri.visibilidad = vis; a.pintar() }

// subirDesdeDrive pide la copia y, en cuanto el servicio la acepta, lleva a la gestión
// del vídeo, que lo enseña «subiendo» hasta que la copia llega.
func (a *App) subirDesdeDrive() {
	titulo := strings.TrimSpace(a.tituloDrive.Texto())
	if a.cli == nil || a.dri.fichero == nil || a.dri.enviando || titulo == "" {
		return
	}
	gen, id, vis := a.gen, a.dri.fichero.FileID, a.dri.visibilidad
	var cals []int
	if a.dri.tocadas {
		cals = a.dri.calidades
	}
	a.dri.enviando, a.dri.errorEnvio = true, ""
	a.pintar()
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		vid, err := a.cli.FromDrive(c, api.CastFromDrive{FileID: id, Title: titulo, Visibility: vis, Qualities: cals})
		a.hilo(gen, func() {
			a.dri.enviando = false
			if err != nil {
				libres := int64(-1)
				if quedan, ok := api.CastQuotaAvailable(err); ok {
					libres = quedan
				}
				a.dri.errorEnvio = textoMotivo(a.v, motivoDrive(err), libres)
				a.pintar()
				return
			}
			a.AbrirGestionar(vid.ID)
		})
	})
}

// ---- Gestionar ----

// AbrirGestionar enseña «Gestionar» de un vídeo.
func (a *App) AbrirGestionar(id string) {
	a.apuntar(lugar{ruta: RutaGestionar, param: id})
	a.salirDeVer()
	a.ruta = RutaGestionar
	a.nuevaPantalla()
	a.scroll.Offset = fyne.NewPos(0, 0)
	a.ges = estadoGestionar{id: id, estadoCarga: estadoCarga{cargando: true}}
	a.visor = nil
	a.pintar()
	a.pedirGestionar()
}

func (a *App) pedirGestionar() {
	if a.cli == nil || !a.dentro() {
		return
	}
	gen, id := a.gen, a.ges.id
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		vid, err := a.cli.Video(c, id)
		var personas []api.CastPerson
		if err == nil && !vid.Can.Edit {
			err = &api.Error{Status: 403}
		}
		if err == nil && vid.Visibility == api.CastPeople {
			personas, _ = a.cli.People(c, id)
		}
		var play api.CastPlay
		errPlay := errSinReproduccion
		if err == nil && vid.Status == api.CastStatusReady {
			play, errPlay = a.cli.Play(c, id)
		}
		a.hilo(gen, func() {
			a.ges.cargando, a.ges.fallo = false, err
			if err == nil {
				a.cargarGestionar(vid)
				a.ges.personas = personas
				a.pedirCalidades()
				if errPlay == nil && play.WebMURL != "" {
					a.visorGestionar(play.WebMURL)
				}
			}
			a.pintar()
		})
	})
}

// errSinReproduccion: el vídeo aún no se puede ver, así que no se pide.
var errSinReproduccion = errors.New("cast: el vídeo aún no se puede reproducir")

// visorGestionar monta el reproductor de la tarjeta del póster (el Player de la web
// con `alVideo`): con él se va al fotograma que se quiere de póster.
func (a *App) visorGestionar(ruta string) {
	a.visor = NuevoVisor(a.v)
	a.visor.AlAmpliar = a.pantallaCompleta
	a.visor.AlSilencio = a.alternarSilencio
	a.visor.Rehacer()
	a.cargarPosterEnVisor(a.ges.video.PosterURL)
	a.abrirReproductor(ruta)
}

// cargarGestionar pone en los campos lo que dice el servicio.
func (a *App) cargarGestionar(vid api.CastVideo) {
	a.ges.video = vid
	a.gTitulo.PonerTexto(vid.Title)
	a.gDescripcion.PonerTexto(vid.Description)
	a.recordarPosters([]api.CastVideo{vid})
}

// detallesCambiados dice si el título o la descripción difieren de lo guardado (lo
// que enciende Guardar; lo demás se guarda al momento).
func (a *App) detallesCambiados() bool {
	vid := a.ges.video
	return strings.TrimSpace(a.gTitulo.Texto()) != vid.Title || strings.TrimSpace(a.gDescripcion.Texto()) != strings.TrimSpace(vid.Description)
}

// pintarSiGestionar repinta al teclear en los campos cuando Guardar cambia de estado.
func (a *App) pintarSiGestionar() {
	if a.ruta == RutaGestionar && a.ges.video.ID != "" {
		cambiado := a.detallesCambiados() && strings.TrimSpace(a.gTitulo.Texto()) != ""
		if cambiado != a.ges.cambiadoPintado {
			a.ges.cambiadoPintado = cambiado
			a.pintar()
		}
	}
}

func (a *App) pantallaGestionar() fyne.CanvasObject {
	if !a.dentro() {
		return PantallaGestionar(a.sinSesion(), Gestionar{})
	}
	g := a.ges
	v := a.vistaDe(g.estadoCarga, g.video.ID == "", false, a.pedirGestionar)
	a.ges.cambiadoPintado = a.detallesCambiados() && strings.TrimSpace(a.gTitulo.Texto()) != ""
	var visor fyne.CanvasObject
	if a.visor != nil {
		visor = a.visor.Objeto()
	}
	return PantallaGestionar(v, Gestionar{
		Video: g.video, Titulo: a.gTitulo, Descripcion: a.gDescripcion, Persona: a.gPersona, Visor: visor,
		Personas: g.personas, ErrorPersona: g.errorPersona, Guardando: g.guardando, Cambiado: a.detallesCambiados(),
		PosterOcupado:   g.posterOcupado,
		Calidades:       a.tarjetaCalidades(),
		AlVisibilidad:   func(vis string) { a.cambiar(api.CastChanges{Visibility: &vis}) },
		AlComentarios:   func(b bool) { a.cambiar(api.CastChanges{CommentsEnabled: &b}) },
		AlChat:          func(b bool) { a.cambiar(api.CastChanges{ChatEnabled: &b}) },
		AlDescargas:     func(b bool) { a.cambiar(api.CastChanges{DownloadEnabled: &b}) },
		AlAnadirPersona: a.anadirPersona, AlQuitarPersona: a.quitarPersona,
		AlSubirPoster: a.subirPoster, AlFotograma: a.posterDeFotograma,
		AlGuardar: a.guardar, AlBorrar: a.confirmarBorrar,
		AlVer: func() { a.AbrirVideo(g.id) },
	})
}

// aplicar pone en `vid` los cambios, para enseñarlos antes de que conteste el servicio.
func aplicar(vid api.CastVideo, c api.CastChanges) api.CastVideo {
	if c.Title != nil {
		vid.Title = *c.Title
	}
	if c.Description != nil {
		vid.Description = *c.Description
	}
	if c.Visibility != nil {
		vid.Visibility = *c.Visibility
	}
	if c.CommentsEnabled != nil {
		vid.CommentsEnabled = *c.CommentsEnabled
	}
	if c.ChatEnabled != nil {
		vid.ChatEnabled = *c.ChatEnabled
	}
	if c.DownloadEnabled != nil {
		vid.DownloadEnabled = *c.DownloadEnabled
	}
	return vid
}

// cambiar es el `cambiar` de ManageApp.tsx: se enseña ya, se guarda y, si el servicio
// no lo acepta, se vuelve a lo de antes con «No se ha podido guardar». `hecho` se
// llama con el resultado (en el hilo de la interfaz).
func (a *App) cambiar(c api.CastChanges, hecho ...func(ok bool)) {
	if a.cli == nil {
		return
	}
	antes := a.ges.video
	a.ges.video = aplicar(antes, c)
	a.pintar()
	gen, id := a.gen, a.ges.id
	a.lanzar(func() {
		ct, fin := ctx()
		defer fin()
		vid, err := a.cli.Update(ct, id, c)
		a.hilo(gen, func() {
			if err != nil {
				a.ges.video = antes
				a.avisar(ui.AvisoPeligro, a.v.Traduce("manage.saveError"))
			} else {
				a.ges.video = vid
				a.recordarPosters([]api.CastVideo{vid})
				if vid.Visibility == api.CastPeople && antes.Visibility != api.CastPeople {
					a.pedirPersonas()
				}
			}
			for _, f := range hecho {
				f(err == nil)
			}
			a.pintar()
		})
	})
}

// guardar es el formulario de detalles: título y descripción.
func (a *App) guardar() {
	titulo := strings.TrimSpace(a.gTitulo.Texto())
	if !a.detallesCambiados() || titulo == "" || a.ges.guardando {
		return
	}
	desc := strings.TrimSpace(a.gDescripcion.Texto())
	a.ges.guardando = true
	a.cambiar(api.CastChanges{Title: &titulo, Description: &desc}, func(ok bool) {
		a.ges.guardando = false
		if ok {
			a.avisar(ui.AvisoExito, a.v.Traduce("manage.saved"))
		}
	})
}

func (a *App) pedirPersonas() {
	gen, id := a.gen, a.ges.id
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		ps, err := a.cli.People(c, id)
		a.hilo(gen, func() {
			if err == nil {
				a.ges.personas = ps
			}
			a.pintar()
		})
	})
}

// anadirPersona da acceso a quien se escribió (el PeoplePicker, que solo se ve con la
// visibilidad `people` ya guardada).
func (a *App) anadirPersona() {
	consulta := strings.TrimSpace(a.gPersona.Texto())
	if consulta == "" || a.cli == nil || a.ges.video.Visibility != api.CastPeople {
		return
	}
	gen, id := a.gen, a.ges.id
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		p, err := a.cli.AddPerson(c, id, consulta)
		a.hilo(gen, func() {
			if err != nil {
				a.ges.errorPersona = a.textoPersona(err)
			} else {
				a.ges.errorPersona = ""
				a.ges.personas = append(quitarPersona(a.ges.personas, p.UserID), p)
				a.gPersona.PonerTexto("")
			}
			a.pintar()
		})
	})
}

// textoPersona: un 404 al compartir es «no hay nadie con ese @usuario».
func (a *App) textoPersona(err error) string {
	var e *api.Error
	if errors.As(err, &e) && e.Status == 404 {
		return a.v.Traduce("manage.people.notFound", i18n.Vars{"query": strings.TrimSpace(a.gPersona.Texto())})
	}
	return textoDeFallo(a.v, err)
}

func quitarPersona(ps []api.CastPerson, id string) []api.CastPerson {
	var out []api.CastPerson
	for _, p := range ps {
		if p.UserID != id {
			out = append(out, p)
		}
	}
	return out
}

func (a *App) quitarPersona(p api.CastPerson) {
	if a.cli == nil {
		return
	}
	gen, id := a.gen, a.ges.id
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		err := a.cli.RemovePerson(c, id, p.UserID)
		a.hilo(gen, func() {
			if err != nil {
				a.ges.errorPersona = textoDeFallo(a.v, err)
			} else {
				a.ges.personas = quitarPersona(a.ges.personas, p.UserID)
			}
			a.pintar()
		})
	})
}

// Tope del póster que se sube: el MAX_POSTER de la web.
const maxPoster = 5 << 20

// subirPoster sube una imagen elegida como póster.
func (a *App) subirPoster() {
	if a.ElegirImagen == nil || a.cli == nil || a.ges.posterOcupado {
		return
	}
	gen, id := a.gen, a.ges.id
	a.lanzar(func() {
		ruta, err := a.ElegirImagen()
		if err != nil || ruta == "" {
			return
		}
		datos, err := os.ReadFile(ruta) //nolint:gosec // G304: la imagen que eligió la persona
		if err == nil && int64(len(datos)) > maxPoster {
			a.hilo(gen, func() { a.avisar(ui.AvisoPeligro, a.v.Traduce("manage.posterTooBig")) })
			return
		}
		a.hilo(gen, func() { a.ges.posterOcupado = true; a.pintar() })
		var vid api.CastVideo
		if err == nil {
			c, fin := ctx()
			tipo := mime.TypeByExtension(strings.ToLower(filepath.Ext(ruta)))
			vid, err = a.cli.UploadPoster(c, id, datos, tipo)
			fin()
		}
		a.hilo(gen, func() {
			a.ges.posterOcupado = false
			if err != nil {
				a.avisar(ui.AvisoPeligro, a.v.Traduce("manage.saveError"))
			} else {
				a.nuevoPoster(vid)
				a.avisar(ui.AvisoExito, a.v.Traduce("manage.posterDone"))
			}
			a.pintar()
		})
	})
}

// nuevoPoster tira el póster viejo de la caché para que se pida el nuevo.
func (a *App) nuevoPoster(vid api.CastVideo) {
	if a.v.Teca != nil {
		a.v.Teca.Tirar(vid.ID)
	}
	a.ges.video.PosterURL = vid.PosterURL
	a.recordarPosters([]api.CastVideo{vid})
}

// posterDeFotograma es «Usar este fotograma»: el instante en el que está el
// reproductor. El servicio lo saca en segundo plano y el póster cambia al rato.
func (a *App) posterDeFotograma() {
	if a.cli == nil || a.reproductor == nil || a.ges.posterOcupado {
		return
	}
	en := a.reproductor.Posicion()
	gen, id := a.gen, a.ges.id
	a.ges.posterOcupado = true
	a.pintar()
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		_, err := a.cli.PosterFrame(c, id, en.Milliseconds())
		a.hilo(gen, func() {
			a.ges.posterOcupado = false
			if err != nil {
				a.avisar(ui.AvisoPeligro, a.v.Traduce("manage.saveError"))
			} else {
				a.avisar(ui.AvisoExito, a.v.Traduce("manage.posterQueued"))
			}
			a.pintar()
		})
	})
}

func (a *App) confirmarBorrar() {
	a.confirmar(a.v.Traduce("manage.deleteTitle"), a.v.Traduce("manage.deleteDescription"), a.v.Traduce("manage.delete"), a.borrar)
}

func (a *App) borrar() {
	if a.cli == nil {
		return
	}
	gen, id := a.gen, a.ges.id
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		err := a.cli.Delete(c, id)
		a.hilo(gen, func() {
			if err != nil {
				a.avisar(ui.AvisoPeligro, a.v.Traduce("errors.title"))
				return
			}
			a.avisar(ui.AvisoExito, a.v.Traduce("manage.deleted"))
			a.Ir(RutaInicio)
		})
	})
}

// ---- Emitir ----

func (a *App) pedirEmitir() {
	if a.cli == nil || !a.dentro() {
		return
	}
	gen := a.gen
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		clave, errClave := a.cli.StreamKey(c)
		a.hilo(gen, func() {
			if errClave != nil {
				a.emi.errorClave = textoDeFallo(a.v, errClave)
			} else {
				a.emi.clave, a.emi.errorClave = &clave, ""
			}
			a.pintar()
		})
	})
	a.pedirDirectos()
}

// pedirDirectos lee mis directos (la biblioteca «mine» y de ella los live).
func (a *App) pedirDirectos() {
	if a.cli == nil || !a.dentro() {
		return
	}
	gen := a.gen
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		mios, err := a.cli.Videos(c, api.CastMine, "", "", "", 50)
		a.hilo(gen, func() {
			switch {
			case err == nil:
				a.emi.directos, a.emi.sinConexion, a.emi.estado = directosDe(mios.Videos), false, Lista
			case estadoDeFallo(err) == SinPermiso:
				a.emi.estado = SinPermiso
			case estadoDeFallo(err) == SinConexion:
				a.emi.sinConexion = true
			}
			a.pintar()
		})
	})
}

// directosDe deja los directos de una lista (nunca nil: vacía es «aún no hay»).
func directosDe(vs []api.CastVideo) []api.CastVideo {
	out := []api.CastVideo{}
	for _, x := range vs {
		if x.Kind == "live" {
			out = append(out, x)
		}
	}
	return out
}

func (a *App) pintarSiEmitir() {
	if a.ruta == RutaEmitir {
		a.pintar()
	}
}

func (a *App) pantallaEmitir() fyne.CanvasObject {
	if !a.dentro() {
		return PantallaEmitir(a.sinSesion(), Emitir{})
	}
	e := a.emi
	v := a.v
	v.Estado = e.estado
	if a.previa == nil {
		a.previa = NuevaPrevia(a.v)
	}
	ah := e.ahora
	// En directo se enseña el título de lo que se emite, no los campos para programar.
	if ah.elegido == nil && ah.video.ID != "" && ah.fase == EmisionEnDirecto {
		ah.elegido = &ah.video
	}
	return PantallaEmitir(v, Emitir{
		Clave: e.clave, ClaveVisible: e.visible, ErrorClave: e.errorClave, Regenerando: e.regenerando,
		Emision: Emision{
			Elegido: ah.elegido, Titulo: a.tituloAhora, Visibilidad: ah.visibilidad, Previa: a.previa.Objeto(),
			Fase: ah.fase, Camara: ah.camara, Pantalla: ah.pantalla, Micro: ah.micro, Denegado: ah.denegado,
			AlVisibilidad: func(vis string) { a.emi.ahora.visibilidad = vis; a.pintar() },
			AlCamara:      a.fuenteCamara(), AlPantalla: a.fuentePantalla(), AlMicro: a.alternarMicro,
			AlEmpezar: a.empezarEmision, AlTerminar: a.confirmarAcabar,
			AlCambiarCamara: a.cambioDeCamara(),
		},
		Titulo: a.tituloDirecto, Descripcion: a.descDirecto, Visibilidad: e.visibilidad, Fuente: e.fuente,
		Programando: e.programando,
		Directos:    e.directos, SinConexion: e.sinConexion,
		AlVerClave:       func() { a.emi.visible = !a.emi.visible; a.pintar() },
		AlCopiarServidor: func() { a.copiarEstudio(e.clave.RTMPURL, "studio.obs.copied") },
		AlCopiarClave:    func() { a.copiarEstudio(e.clave.StreamKey, "studio.obs.copied") },
		AlRegenerar:      a.confirmarRegenerar,
		AlVisibilidad:    func(vis string) { a.emi.visibilidad = vis; a.pintar() },
		AlFuente:         func(f string) { a.emi.fuente = f; a.pintar() },
		AlProgramar:      a.programar,
		AlEmitirAqui:     a.elegirParaEmitir,
		AlAbrir:          a.abrir,
		AlTerminar:       a.confirmarTerminarDeLista,
	})
}

func (a *App) copiarEstudio(texto, clave string) {
	if a.w != nil && ui.Copiar(fyne.CurrentApp().Clipboard(), texto) {
		a.avisar(ui.AvisoExito, a.v.Traduce(clave))
	} else {
		a.avisar(ui.AvisoPeligro, a.v.Traduce("errors.title"))
	}
}

func (a *App) confirmarRegenerar() {
	a.confirmar(a.v.Traduce("studio.obs.rotateTitle"), a.v.Traduce("studio.obs.rotateDescription"), a.v.Traduce("studio.obs.rotate"), a.regenerar)
}

func (a *App) regenerar() {
	if a.cli == nil {
		return
	}
	gen := a.gen
	a.emi.regenerando = true
	a.pintar()
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		k, err := a.cli.RotateStreamKey(c)
		a.hilo(gen, func() {
			a.emi.regenerando = false
			if err != nil {
				a.avisar(ui.AvisoPeligro, a.v.Traduce("errors.title"))
			} else {
				a.emi.clave = &k
				a.avisar(ui.AvisoExito, a.v.Traduce("studio.obs.rotated"))
			}
			a.pintar()
		})
	})
}

// confirmarTerminarDeLista es «Terminar» en una fila de mis directos.
func (a *App) confirmarTerminarDeLista(x api.CastVideo) {
	a.confirmar(a.v.Traduce("studio.browser.stopTitle"), a.v.Traduce("studio.browser.stopDescription"), a.v.Traduce("studio.lives.end"),
		func() { a.terminar(x) })
}

// programar da de alta un directo con el formulario. Desde OBS queda programado y la
// clave lo lleva a su sala; «desde aquí» queda además elegido en el panel de emitir,
// listo para «Emitir ahora».
func (a *App) programar() {
	titulo := strings.TrimSpace(a.tituloDirecto.Texto())
	if titulo == "" || a.cli == nil {
		return
	}
	gen, vis, origen := a.gen, a.emi.visibilidad, a.emi.fuente
	desc := strings.TrimSpace(a.descDirecto.Texto())
	a.emi.programando = true
	a.pintar()
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		vid, _, err := a.cli.ScheduleLive(c, api.CastNew{Title: titulo, Description: desc, Visibility: vis, Source: origen})
		a.hilo(gen, func() {
			a.emi.programando = false
			if err != nil {
				a.avisar(ui.AvisoPeligro, a.v.Traduce("errors.title"))
				a.pintar()
				return
			}
			a.tituloDirecto.PonerTexto("")
			a.descDirecto.PonerTexto("")
			a.emi.directos = append([]api.CastVideo{vid}, a.emi.directos...)
			a.avisar(ui.AvisoExito, a.v.Traduce("studio.schedule.done"))
			if origen == "webrtc" {
				a.elegirParaEmitir(vid)
			}
			a.pintar()
		})
	})
}

// elegirParaEmitir pone un directo programado en el panel de emitir desde aquí.
func (a *App) elegirParaEmitir(x api.CastVideo) {
	if a.emi.ahora.fase == EmisionConectando || a.emi.ahora.fase == EmisionEnDirecto {
		return
	}
	a.emi.ahora.elegido = &x
	a.pintar()
}

// ---- La vista previa (antes de emitir) ----

// fuenteCamara es el conmutador de la cámara: antes de emitir abre o suelta la de la
// vista previa; en directo la publica o la apaga en la sala. nil si no hay cámara.
func (a *App) fuenteCamara() func() {
	if a.Camara == nil {
		return nil
	}
	return func() { a.alternarFuente(false) }
}

func (a *App) fuentePantalla() func() {
	if a.Pantalla == nil {
		return nil
	}
	return func() { a.alternarFuente(true) }
}

func (a *App) alternarFuente(pantalla bool) {
	ah := &a.emi.ahora
	encendida := ah.camara
	if pantalla {
		encendida = ah.pantalla
	}
	switch ah.fase {
	case EmisionConectando:
		return
	case EmisionEnDirecto:
		a.alternarEnSala(pantalla, encendida)
		return
	case EmisionPreparando, EmisionFallida:
	}
	if encendida {
		if pantalla {
			ah.pantalla = false
			a.soltarDePrevia(true)
		} else {
			ah.camara = false
			a.soltarDePrevia(false)
		}
		a.mirarPrevia()
		a.pintar()
		return
	}
	abrir := a.Camara
	if pantalla {
		abrir = a.Pantalla
	}
	gen := a.gen
	a.lanzar(func() {
		f, err := abrir()
		a.enLaInterfaz(func() { a.fuenteAbierta(gen, pantalla, f, err) })
	})
}

// alternarEnSala enciende o apaga una fuente en la sala en la que se emite.
func (a *App) alternarEnSala(pantalla, encendida bool) {
	ah := &a.emi.ahora
	if a.emiSala == nil {
		return
	}
	var err error
	if pantalla {
		err = a.emiSala.Pantalla(!encendida)
	} else {
		err = a.emiSala.Camara(!encendida)
	}
	switch {
	case err != nil:
		ah.denegado = !pantalla
	case pantalla:
		ah.pantalla = !encendida
	default:
		ah.camara = !encendida
	}
	a.pintar()
}

// fuenteAbierta recibe la cámara o la pantalla de la vista previa. Si mientras se
// abría se empezó a emitir o se cerró la app, se suelta.
func (a *App) fuenteAbierta(gen int, pantalla bool, f camara.Fuente, err error) {
	ah := &a.emi.ahora
	if err != nil || ah.fase == EmisionConectando || ah.fase == EmisionEnDirecto || gen != a.gen {
		if f != nil {
			f.Cerrar()
		}
		if err != nil && gen == a.gen {
			// Cancelar el selector de pantalla no es un error que enseñar.
			ah.denegado = !pantalla
			if !pantalla && errors.Is(err, camara.ErrSinPermiso) {
				a.esperarPermisoCamara(gen)
			}
			a.pintar()
		}
		return
	}
	ah.denegado = false
	if !pantalla {
		// Con la cámara ya concedida se pregunta por el micrófono: dos diálogos a la
		// vez en Android se pisan, y sin permiso el directo saldría mudo sin avisar.
		a.pedirMicro()
	}
	if pantalla {
		a.soltarDePrevia(true)
		a.previaPant, ah.pantalla = f, true
	} else {
		a.soltarDePrevia(false)
		a.previaCam, ah.camara = f, true
	}
	a.mirarPrevia()
	a.pintar()
}

// soltarDePrevia cierra la pantalla o la cámara de la vista previa, si está abierta.
func (a *App) soltarDePrevia(pantalla bool) {
	f := &a.previaCam
	if pantalla {
		f = &a.previaPant
	}
	if *f != nil {
		(*f).Cerrar()
		*f = nil
	}
}

// Quince fotogramas por segundo en la vista previa: los de la cámara al emitir.
const ritmoPrevia = time.Second / 15

// mirarPrevia (re)arranca el latido que pinta la vista previa: la pantalla si se
// comparte, si no la cámara (en espejo). Sin ninguna, vuelve el aviso.
func (a *App) mirarPrevia() {
	if a.pararPrevia != nil {
		close(a.pararPrevia)
		a.pararPrevia = nil
	}
	if a.previa == nil {
		a.previa = NuevaPrevia(a.v)
	}
	f, espejo := a.previaPant, false
	if f == nil {
		f, espejo = a.previaCam, a.camaraDelantera()
	}
	if f == nil {
		a.previa.Imagen(nil, false)
		return
	}
	parar := make(chan struct{})
	a.pararPrevia = parar
	go func() {
		t := time.NewTicker(ritmoPrevia)
		defer t.Stop()
		for {
			select {
			case <-parar:
				return
			case <-t.C:
				img, nueva := f.Leer()
				if !nueva || img == nil {
					continue
				}
				a.enLaInterfaz(func() {
					select {
					case <-parar:
					default:
						a.previa.Imagen(img, espejo)
					}
				})
			}
		}
	}()
}

// soltarPrevia cierra la cámara y la pantalla de la vista previa.
func (a *App) soltarPrevia() {
	if a.pararPrevia != nil {
		close(a.pararPrevia)
		a.pararPrevia = nil
	}
	a.soltarDePrevia(false)
	a.soltarDePrevia(true)
}

// reabrirPrevia vuelve a abrir, para la vista previa, las fuentes que estaban elegidas
// (tras terminar o si no se pudo empezar), como la web, que conserva sus pistas.
func (a *App) reabrirPrevia() {
	ah := &a.emi.ahora
	cam, pant := ah.camara, ah.pantalla
	ah.camara, ah.pantalla = false, false
	if a.previa != nil {
		a.previa.Imagen(nil, false)
	}
	if cam && a.Camara != nil {
		a.alternarFuente(false)
	}
	if pant && a.Pantalla != nil {
		a.alternarFuente(true)
	}
}

func (a *App) alternarMicro() {
	ah := &a.emi.ahora
	if !ah.micro {
		a.pedirMicro()
	}
	if ah.fase == EmisionEnDirecto && a.emiSala != nil {
		if err := a.emiSala.Micro(!ah.micro); err != nil {
			ah.denegado = true
			a.pintar()
			return
		}
	}
	ah.micro = !ah.micro
	a.pintar()
}

// ---- Emitir desde aquí ----

// empezarEmision es «Emitir ahora»: consigue la conexión de emisor —la del directo
// elegido o la de uno que se programa con el título y la visibilidad—, entra en la
// sala y publica lo elegido.
func (a *App) empezarEmision() {
	ah := &a.emi.ahora
	titulo := strings.TrimSpace(a.tituloAhora.Texto())
	if a.cli == nil || ah.fase == EmisionConectando || ah.fase == EmisionEnDirecto || (!ah.camara && !ah.pantalla) ||
		(ah.elegido == nil && titulo == "") {
		return
	}
	ah.fase, ah.denegado = EmisionConectando, false
	a.pintar()
	elegido, vis := ah.elegido, ah.visibilidad
	gen := a.gen
	a.lanzar(func() {
		c, fin := ctx()
		defer fin()
		var vid api.CastVideo
		var pub *api.CastLiveConn
		var err error
		if elegido != nil {
			var conn api.CastLiveConn
			vid = *elegido
			if conn, err = a.cli.PublishToken(c, elegido.ID); err == nil {
				pub = &conn
			}
		} else {
			vid, pub, err = a.cli.ScheduleLive(c, api.CastNew{Title: titulo, Visibility: vis, Source: "webrtc"})
		}
		a.hilo(gen, func() {
			if err == nil && pub == nil {
				err = llamada.ErrSinSala
			}
			if err != nil {
				a.emi.ahora.fase = EmisionFallida
				a.pintar()
				return
			}
			if elegido == nil {
				a.tituloAhora.PonerTexto("")
				a.pedirDirectos()
			}
			a.entrarAEmitir(vid, *pub)
		})
	})
}

// entrarAEmitir suelta la vista previa (la sala abre la cámara y la pantalla por su
// cuenta), entra con el token de emisor y enciende lo elegido.
func (a *App) entrarAEmitir(vid api.CastVideo, pub api.CastLiveConn) {
	ah := &a.emi.ahora
	ah.video = vid
	quiereCam, quierePant, micro := ah.camara, ah.pantalla, ah.micro
	a.soltarPrevia()
	if a.Conectar == nil {
		ah.fase = EmisionFallida
		a.reabrirPrevia()
		a.pintar()
		return
	}
	gen := a.gen
	sala := &salaCompartida{}
	o := llamada.Opciones{Micro: micro, Audio: a.Audio, Camara: a.Camara, Pantalla: a.Pantalla}
	o.AlFotograma = func() {
		a.enLaInterfaz(func() {
			if a.emi.ahora.fase != EmisionEnDirecto || a.previa == nil {
				return
			}
			if img, espejo := imagenPropia(sala.leer()); img != nil {
				a.previa.Imagen(img, espejo && a.camaraDelantera())
			}
		})
	}
	o.AlSalir = func(string) {
		a.enLaInterfaz(func() {
			if a.emi.ahora.fase == EmisionEnDirecto && a.emiSala != nil && a.emiSala == sala.leer() {
				a.emiSala = nil
				a.emi.ahora.fase = EmisionFallida
				a.previa.EnDirecto(false)
				a.reabrirPrevia()
				a.pintar()
			}
		})
	}
	a.lanzar(func() {
		s, err := a.Conectar(pub.ServerURL, pub.Token, o)
		sala.poner(s)
		var errCam, errPant error
		if err == nil && quiereCam {
			errCam = s.Camara(true)
		}
		if err == nil && quierePant {
			errPant = s.Pantalla(true)
		}
		a.enLaInterfaz(func() {
			a.enSala(gen, s, err, quiereCam && errCam == nil, quierePant && errPant == nil, errCam != nil || errPant != nil)
		})
	})
}

// enSala recibe la sala en la que se emite (o por qué no se entró).
func (a *App) enSala(gen int, s Sala, err error, cam, pant, denegado bool) {
	ah := &a.emi.ahora
	if gen != a.gen || ah.fase != EmisionConectando {
		if s != nil {
			go s.Colgar()
		}
		return
	}
	if err != nil {
		ah.fase = EmisionFallida
		a.reabrirPrevia()
		a.pintar()
		return
	}
	a.emiSala = s
	ah.fase, ah.camara, ah.pantalla, ah.denegado = EmisionEnDirecto, cam, pant, denegado
	a.previa.EnDirecto(true)
	a.pintar()
}

// imagenPropia es lo que sale de aquí: la pantalla si se comparte, si no la cámara
// (que se ve en espejo).
func imagenPropia(s Sala) (image.Image, bool) {
	if s == nil {
		return nil, false
	}
	for _, p := range s.Participantes() {
		if !p.Yo {
			continue
		}
		if p.Compartida != nil {
			return p.Compartida, false
		}
		return p.Video, true
	}
	return nil, false
}

// confirmarAcabar es «Terminar el directo» en el panel: cuelga, avisa al servicio (que
// para la grabación y la encola) y vuelve a preparar, con las fuentes de antes.
func (a *App) confirmarAcabar() {
	a.confirmar(a.v.Traduce("studio.browser.stopTitle"), a.v.Traduce("studio.browser.stopDescription"), a.v.Traduce("studio.browser.stop"), a.acabar)
}

func (a *App) acabar() {
	ah := &a.emi.ahora
	if ah.fase != EmisionEnDirecto {
		return
	}
	if a.emiSala != nil {
		s := a.emiSala
		a.emiSala = nil
		go s.Colgar()
	}
	vid := ah.video
	ah.fase, ah.elegido, ah.video = EmisionPreparando, nil, api.CastVideo{}
	if a.previa != nil {
		a.previa.EnDirecto(false)
	}
	a.reabrirPrevia()
	a.terminar(vid)
	a.pintar()
}

// Cerrar lo suelta todo al cerrar la ventana: reproductor, salas y chat.
func (a *App) Cerrar() {
	a.nuevaPantalla() // lo que esté en vuelo o sondeando ya no es de ninguna pantalla
	if a.esperaBusqueda != nil {
		a.esperaBusqueda.Stop()
		a.esperaBusqueda = nil
	}
	// La ventana primero: si su vídeo está en la página, vuelve a ella y salirDeVer lo
	// suelta con lo demás.
	a.cerrarPiP(false)
	a.salirDeVer()
	if p := a.pendiente; p != nil {
		a.pendiente = nil
		p.r.Cerrar()
		_ = p.f.Close()
	}
	a.soltarPrevia()
	if a.emiSala != nil {
		a.emiSala.Colgar()
		a.emiSala = nil
	}
	a.actualizarEncendido()
}

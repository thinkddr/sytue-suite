// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/fsnotify/fsnotify"

	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/local/imagenes"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/carrete"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
)

// # «Importar» (FO-04, FO-09)
//
// El botón «Importar» de la barra de la galería abre un diálogo que depende del equipo:
//
//   - En el MÓVIL, «Importar del carrete»: «Importar ahora» trae a la fototeca las fotos
//     y los vídeos de la galería del sistema que aún no estén (COPIADOS: allí no hay una
//     ruta estable a la que enlazar), y la casilla «Importar automáticamente»
//     lo hace solo al abrir la app, al volver a ella y cada cuarto de hora mientras está
//     abierta (la regla de la copia de la conectada, CadaRondaCopia).
//   - En el ESCRITORIO, «Importar una carpeta…» (una vez) y las CARPETAS VIGILADAS: lo que
//     aparece en ellas se importa solo, al abrir la app y en cuanto llega (fsnotify).
//
// Lo de una carpeta se ENLAZA (FO-09): el original se queda donde está, sin copiarlo, y la
// fototeca guarda sus miniaturas; fotos y vídeos. Si un fichero de una vigilada se mueve
// o se renombra dentro de ellas, la foto lo vuelve a encontrar por su huella (Recolocar);
// si desaparece, la galería lo dice. La casilla «Copiar a la fototeca» copia las fotos
// como antes (los vídeos se enlazan siempre: copiarlos a la fototeca no aporta nada).

// esperaVigiladas es lo que se espera tras el último cambio en una carpeta vigilada: una
// cámara o un copiado de muchas fotos llega a ráfagas.
const esperaVigiladas = 3 * time.Second

// importador es el estado de «Importar».
type importador struct {
	l    *local.App
	w    fyne.Window
	p    *photos.App
	repo *fototeca.Fototeca
	cli  *api.Photos
	heic bool

	carrete *photos.Copia // la galería del sistema (móvil)
	vigila  *photos.Copia // las carpetas vigiladas (escritorio)
	// abiertos es lo que dijo el carrete de los vídeos que tiene abiertos (copiar.go).
	abiertos *elementosAbiertos

	mu        sync.Mutex
	carpetas  []string
	copiar    bool
	bajas     bool // algo desapareció de una vigilada desde la última ronda
	reloj     *time.Timer
	vigilante *fsnotify.Watcher
	estado    *widget.Label
	dialogo   *ui.Dialogo
}

// ajustesImportar es lo que se guarda: las carpetas vigiladas y si las fotos de una
// carpeta se copian a la fototeca en vez de enlazarse.
type ajustesImportar struct {
	Vigiladas []string `json:"vigiladas,omitempty"`
	Copiar    bool     `json:"copiar,omitempty"`
}

func nuevoImportador(l *local.App, w fyne.Window, p *photos.App, repo *fototeca.Fototeca, cli *api.Photos) *importador {
	im := &importador{l: l, w: w, p: p, repo: repo, cli: cli, heic: imagenes.HayConversorHEIC(), abiertos: &elementosAbiertos{}}
	if esMovil() && carrete.Disponible() {
		// Fotos y vídeos (FO-09): los vídeos se copian con su póster y su duración.
		im.carrete = photos.NuevaCopia(soloImportables{GaleriaSistema: photos.CarreteDelSistema{}, heic: im.heic, videos: true, abiertos: im.abiertos},
			l.Espacio.Ajuste("photos-carrete.json"))
	}
	var aj ajustesImportar
	_ = espacio.LeerJSON(im.rutaAjustes(), &aj)
	im.carpetas, im.copiar = aj.Vigiladas, aj.Copiar
	im.vigila = photos.NuevaCopia(soloImportables{GaleriaSistema: galeriaCarpetas{carpetas: im.vigiladas}, heic: im.heic, videos: true},
		l.Espacio.Ajuste("photos-vigiladas.json"))
	return im
}

func esMovil() bool { return runtime.GOOS == "ios" || runtime.GOOS == "android" }

func (im *importador) rutaAjustes() string { return im.l.Espacio.Ajuste("photos-importar.json") }

// vigiladas son las carpetas vigiladas que existen ahora.
func (im *importador) vigiladas() []string {
	im.mu.Lock()
	defer im.mu.Unlock()
	return slices.DeleteFunc(slices.Clone(im.carpetas), func(c string) bool {
		fi, err := os.Stat(c)
		return err != nil || !fi.IsDir()
	})
}

func (im *importador) guardar() {
	im.mu.Lock()
	aj := ajustesImportar{Vigiladas: slices.Clone(im.carpetas), Copiar: im.copiar}
	im.mu.Unlock()
	if err := espacio.EscribirJSON(im.rutaAjustes(), aj); err != nil {
		slog.Warn("photos: no se pudieron guardar las carpetas vigiladas", "error", err)
	}
}

// accion es el botón de la barra.
func (im *importador) accion() *photos.AccionExtra {
	return &photos.AccionExtra{Rotulo: texto(im.l, "importar"), Al: im.abrir}
}

// arrancar hace lo automático: el carrete si está encendido, las vigiladas, y su
// vigilante. Devuelve con qué pararlo.
func (im *importador) arrancar() func() {
	im.vigilar()
	im.automatico()
	var reloj *time.Ticker
	if im.carrete != nil {
		reloj = time.NewTicker(photos.CadaRondaCopia)
		go func() {
			for range reloj.C {
				im.automatico()
			}
		}()
	}
	return func() {
		if reloj != nil {
			reloj.Stop()
		}
		im.mu.Lock()
		if im.vigilante != nil {
			_ = im.vigilante.Close()
		}
		im.mu.Unlock()
	}
}

// automatico es lo que corre solo: al abrir, al volver a la ventana y con el reloj.
func (im *importador) automatico() {
	if im.carrete != nil && im.carrete.Ajustes().Activa {
		go im.ronda(im.carrete, nil, false)
	}
	if len(im.vigiladas()) > 0 {
		go im.ronda(im.vigila, im.vigiladas, false)
	}
}

// destino es la fototeca como destino de una ronda: con las carpetas del origen, lo de
// dentro se enlaza (salvo «Copiar a la fototeca»).
func (im *importador) destino(carpetas func() []string) *destinoLocal {
	d := nuevoDestinoLocal(im.repo, im.cli)
	im.mu.Lock()
	d.carpetas, d.copiar = carpetas, im.copiar
	im.mu.Unlock()
	d.medir, d.abiertos = medirVideo, im.abiertos
	return d
}

// ronda importa lo que falte de un origen; carpetas son las suyas si es de carpetas (nil
// en el carrete). `aMano` dice si lo pidió la persona: entonces se dice también cuando no
// había nada nuevo. Corre fuera de la interfaz.
func (im *importador) ronda(c *photos.Copia, carpetas func() []string, aMano bool) {
	if c == im.carrete && esMovil() {
		pctx, fin := context.WithTimeout(context.Background(), 5*time.Minute)
		permiso, _ := photos.CarreteDelSistema{}.PedirPermiso(pctx)
		fin()
		if !permiso.Lee() {
			im.decir(true, texto(im.l, "importar.sinPermiso"), "")
			return
		}
	}
	ctx, cancelar := context.WithTimeout(context.Background(), time.Hour)
	defer cancelar()
	p, err := c.Ronda(ctx, "local", im.destino(carpetas), func(p photos.ProgresoCopia) {
		if p.Total > 0 {
			im.ponerEstado(fmt.Sprintf(texto(im.l, "importar.avance"), p.Hechas, p.Total))
		}
	})
	if c == im.vigila {
		// Lo movido o renombrado dentro de las vigiladas se recoloca por su huella, y lo
		// que desapareció se ve en la galería (con su aviso) sin esperar a recargarla.
		if im.recolocar(ctx) > 0 || im.tomarBajas() {
			fyne.Do(im.p.RecargarGaleria)
		}
	}
	switch {
	case errors.Is(err, photos.ErrCopiaEnMarcha):
		return
	case err != nil:
		slog.Warn("photos: importación", "error", err)
		im.ponerEstado("")
		im.decir(true, texto(im.l, "importar.fallo"), err.Error())
	case p.Subidas > 0:
		im.ponerEstado(fmt.Sprintf(texto(im.l, "importar.hecho"), p.Subidas))
		im.decir(false, fmt.Sprintf(texto(im.l, "importar.hecho"), p.Subidas), "")
		fyne.Do(im.p.RecargarGaleria)
	case aMano:
		im.ponerEstado(texto(im.l, "importar.nada"))
	default:
		im.ponerEstado("")
	}
}

// importarCarpeta importa UNA vez lo que haya en una carpeta (y sus subcarpetas), con un
// registro en memoria: lo que ya esté en la fototeca no se duplica igualmente, por su
// huella.
func (im *importador) importarCarpeta(dir string) {
	carpetas := func() []string { return []string{dir} }
	c := photos.NuevaCopia(soloImportables{GaleriaSistema: galeriaCarpetas{carpetas: carpetas}, heic: im.heic, videos: true}, "")
	go im.ronda(c, carpetas, true)
}

// recolocar busca en las vigiladas los originales enlazados que ya no están en su sitio
// (FO-09) y dice cuántos ha encontrado.
func (im *importador) recolocar(ctx context.Context) int {
	lista, err := (galeriaCarpetas{carpetas: im.vigiladas}).Listar(ctx)
	if err != nil {
		return 0
	}
	rutas := make([]string, 0, len(lista))
	for _, e := range lista {
		rutas = append(rutas, e.ID)
	}
	n, err := im.repo.Recolocar(ctx, rutas)
	if err != nil {
		slog.Warn("photos: recolocar los originales enlazados", "error", err)
	}
	return n
}

// tomarBajas dice si algo desapareció de una vigilada desde la última vez, y lo olvida.
func (im *importador) tomarBajas() bool {
	im.mu.Lock()
	defer im.mu.Unlock()
	b := im.bajas
	im.bajas = false
	return b
}

func (im *importador) decir(peligro bool, titulo, detalle string) {
	fyne.Do(func() { im.p.Avisar(peligro, titulo, detalle) })
}

func (im *importador) ponerEstado(s string) {
	fyne.Do(func() {
		if im.estado != nil {
			im.estado.SetText(s)
			im.estado.Hidden = s == ""
			if im.dialogo != nil {
				im.dialogo.Recolocar()
			}
		}
	})
}

// ----- Carpetas vigiladas -----

// vigilar pone el vigilante sobre las carpetas vigiladas (y sus subcarpetas).
func (im *importador) vigilar() {
	im.mu.Lock()
	if im.vigilante != nil {
		_ = im.vigilante.Close()
		im.vigilante = nil
	}
	im.mu.Unlock()
	dirs := im.vigiladas()
	if len(dirs) == 0 {
		return
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Warn("photos: sin vigilante de carpetas", "error", err)
		return
	}
	for _, d := range dirs {
		vigilarArbol(w, d)
	}
	im.mu.Lock()
	im.vigilante = w
	im.mu.Unlock()
	go im.atender(w)
}

// atender programa una ronda con cada cambio que avisa el vigilante, hasta que se
// cierre; una carpeta nueva se vigila también.
func (im *importador) atender(w *fsnotify.Watcher) {
	for {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if ev.Has(fsnotify.Create) {
				if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
					vigilarArbol(w, ev.Name)
				}
			}
			if ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename) {
				im.mu.Lock()
				im.bajas = true
				im.mu.Unlock()
			}
			if ev.Has(fsnotify.Create) || ev.Has(fsnotify.Write) || ev.Has(fsnotify.Rename) || ev.Has(fsnotify.Remove) {
				im.pronto()
			}
		case _, ok := <-w.Errors:
			if !ok {
				return
			}
		}
	}
}

// vigilarArbol pone el vigilante sobre raiz y sus subcarpetas (fsnotify no baja solo),
// menos las ocultas. Lo que no se puede leer se queda sin vigilar.
func vigilarArbol(w *fsnotify.Watcher, raiz string) {
	_ = filepath.WalkDir(raiz, func(p string, e fs.DirEntry, err error) error {
		if err == nil && e.IsDir() {
			if p != raiz && strings.HasPrefix(e.Name(), ".") {
				return fs.SkipDir
			}
			_ = w.Add(p)
		}
		return nil
	})
}

// pronto programa una ronda de las vigiladas tras esperaVigiladas sin cambios.
func (im *importador) pronto() {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.reloj != nil {
		im.reloj.Stop()
	}
	im.reloj = time.AfterFunc(esperaVigiladas, func() { im.ronda(im.vigila, im.vigiladas, false) })
}

func (im *importador) anadirVigilada(dir string) {
	dir = filepath.Clean(dir)
	im.mu.Lock()
	if slices.Contains(im.carpetas, dir) {
		im.mu.Unlock()
		return
	}
	im.carpetas = append(im.carpetas, dir)
	im.mu.Unlock()
	im.guardar()
	im.vigilar()
	go im.ronda(im.vigila, im.vigiladas, true)
}

func (im *importador) quitarVigilada(dir string) {
	im.mu.Lock()
	im.carpetas = slices.DeleteFunc(im.carpetas, func(c string) bool { return c == dir })
	im.mu.Unlock()
	im.guardar()
	im.vigilar()
}

// ----- El diálogo -----

// abrir enseña el diálogo de «Importar». Va en el hilo de la interfaz.
func (im *importador) abrir() {
	t := func(k string) string { return texto(im.l, k) }
	parrafo := func(s string) *widget.Label {
		l := widget.NewLabel(s)
		l.Wrapping = fyne.TextWrapWord
		return l
	}
	im.estado = parrafo("")
	im.estado.Hidden = true
	var cuerpo []fyne.CanvasObject
	var pie fyne.CanvasObject
	var d *ui.Dialogo
	cerrar := ui.NuevoBoton(im.l.T("cerrar"), func() { d.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	if im.carrete != nil {
		cuerpo = append(cuerpo, parrafo(t("importar.carreteTexto")))
		auto := widget.NewCheck(t("importar.automatico"), func(si bool) {
			_ = im.carrete.CambiarAjustes(func(aj *photos.AjustesCopia) { aj.Activa = si })
			if si {
				go im.ronda(im.carrete, nil, false)
			}
		})
		auto.Checked = im.carrete.Ajustes().Activa
		cuerpo = append(cuerpo, auto)
		ahora := ui.NuevoBoton(t("importar.ahora"), func() { go im.ronda(im.carrete, nil, true) }).Con(ui.Primario, ui.TallaMD)
		pie = container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cerrar, ahora)
	} else {
		cuerpo = append(cuerpo, parrafo(t("importar.carpetaTexto")))
		lista := container.NewVBox()
		var pintarLista func()
		pintarLista = func() {
			lista.Objects = nil
			im.mu.Lock()
			cs := slices.Clone(im.carpetas)
			im.mu.Unlock()
			if len(cs) == 0 {
				lista.Add(parrafo(t("importar.sinVigiladas")))
			}
			for _, c := range cs {
				quitar := ui.NuevoBoton(t("importar.dejar"), func() { im.quitarVigilada(c); pintarLista() }).Con(ui.Fantasma, ui.TallaSM)
				// Una ruta larga se corta con «…» en vez de montarse sobre el botón.
				ruta := widget.NewLabel(c)
				ruta.Truncation = fyne.TextTruncateEllipsis
				lista.Add(container.NewBorder(nil, nil, nil, quitar, ruta))
			}
			lista.Refresh()
			if d != nil {
				d.Recolocar()
			}
		}
		pintarLista()
		vigilar := ui.NuevoBoton(t("importar.vigilar"), func() {
			local.ElegirCarpeta(t("importar.vigilar"), func(dir string) { im.anadirVigilada(dir); pintarLista() })
		}).Con(ui.Secundario, ui.TallaSM)
		cuerpo = append(cuerpo, widget.NewLabelWithStyle(t("importar.vigiladas"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			parrafo(t("importar.vigiladasTexto")), lista, container.NewHBox(vigilar))
		// Enlazar (lo de siempre ahora) o copiar a la fototeca, para lo que entre desde
		// aquí en adelante; lo ya enlazado sigue enlazado.
		copiar := widget.NewCheck(t("importar.copiar"), func(si bool) {
			im.mu.Lock()
			im.copiar = si
			im.mu.Unlock()
			im.guardar()
		})
		im.mu.Lock()
		copiar.Checked = im.copiar
		im.mu.Unlock()
		cuerpo = append(cuerpo, copiar, parrafo(t("importar.copiarTexto")))
		carpeta := ui.NuevoBoton(t("importar.carpeta"), func() {
			local.ElegirCarpeta(t("importar.carpeta"), im.importarCarpeta)
		}).Con(ui.Primario, ui.TallaMD)
		pie = container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cerrar, carpeta)
	}
	if !im.heic {
		cuerpo = append(cuerpo, parrafo(t("importar.sinHEIC")))
	}
	cuerpo = append(cuerpo, im.estado)
	titulo := t("importar")
	if im.carrete != nil {
		titulo = t("importar.carrete")
	}
	d = ui.NuevoDialogo(titulo, container.NewVBox(cuerpo...)).Con(ui.DialogoMD, pie)
	d.RotuloCerrar = im.l.T("cerrar")
	im.dialogo = d
	d.Mostrar(im.w.Canvas())
}

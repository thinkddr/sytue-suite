// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command drive es Sytue Drive de Sytue Office: la misma pantalla de Drive de la suite
// sobre una carpeta del disco, en claro, sin cuenta y sin servidor
// (apps/fyne/PLAN-EDICION-LOCAL.md §6.4).
//
// Comparado con `cmd/drive`: no hay `sesion.Abrir` (ni OIDC ni llavero de sesión), ni
// `cifrado` (ni Matrix ni dispositivo verificado), ni Directorio, Grupos, integraciones o
// cuenta. La fachada `api.Drive` va enchufada con `WithPrivateData` a `local/archivos`, así
// que todo lo que la pantalla pide se resuelve en el disco y lo que no tiene camino local
// falla cerrado, sin caer a ningún servicio.
//
//   - La raíz es `~/Sytue Office/Drive` (en iOS, `Drive` en los Documentos de la app, que
//     «Archivos» enseña; en Android, la carpeta de la app o la que la persona elige en
//     Ajustes con el selector del sistema, carpeta.go; DR-12). Papelera, versiones e índice
//     van en su `.sytue`.
//   - Compartir, «Compartido conmigo», unidades de equipo, «Documentos», las nubes de la
//     organización y clasificar no se montan (DR-05): `drive.EnEquipo`.
//   - «Nuevo → Documento, Hoja, Presentación» deja un .docx, .xlsx o .pptx en la carpeta y
//     lo abre en Docus, Calcus o Diapos de Sytue Office; abrir un fichero va a su app por
//     `sytueoffice-<producto>:/abrir?local=…` y, si no está, a la del sistema (DR-06/07).
//   - «Descargar» es «Copiar a…» y «Mostrar en la carpeta» (DR-08).
//   - «Resumir» usa la IA que la persona configura en Ajustes (DR-10).
//   - En el hueco de «Clasificar», etiquetas libres de la persona (DR-09).
//   - Las cuentas de Ajustes → Nube (Sytue Drive/WebDAV, Google Drive, OneDrive) salen en
//     el lateral como unidades: se navegan, se abren (bajadas a una caché; lo guardado
//     sube solo, sin pisar lo que otro cambió) y aceptan subir y soltar (DR-13).
package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"

	"git.sytue.eu/sytue/one/apps/fyne/apps/drive"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/archivos"
)

// Medidas de la ventana al abrir, las de la edición conectada.
const (
	anchoInicial = 1280
	altoInicial  = 800
)

func main() {
	a := local.NuevaApp("")
	l, err := local.Abrir(a, "drive")
	if err != nil {
		slog.Error("drive", "error", err)
		os.Exit(1)
	}
	tema.ElegirCromo(tema.CromoPapel)
	tr, err := l.Traductor(catalogos.Drive())
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}
	persona := l.Espacio.Persona()
	opciones := archivos.Opciones{
		Instalacion: l.Espacio.ID, Autor: persona.Nombre,
		Miniaturas: filepath.Join(l.Espacio.Rutas.Cache, "drive", "miniaturas"),
	}
	// En Android, la carpeta puede ser una elegida con el selector del sistema (DR-12).
	var carpeta *carpetaDrive
	var disco *archivos.Archivos
	perdida := ""
	if runtime.GOOS == "android" {
		carpeta = nuevaCarpetaDrive(l.Espacio.Rutas.Datos, raizDrive(a, l))
		disco, perdida, err = carpeta.abrirDisco(opciones)
	} else {
		disco, err = archivos.Abrir(raizDrive(a, l), opciones)
	}
	if err != nil {
		slog.Error("no se pudo abrir la carpeta de Drive", "error", err)
		os.Exit(1)
	}
	// Las cuentas de Ajustes → Nube, como unidades más (DR-13).
	rem := abrirRemotas(l, disco)
	// Las raíces no se usan: WithPrivateData cierra todo lo que iría por HTTP.
	svc, err := api.NewDrive("https://sin-servidor.invalid", nil, nil).WithPrivateData(rem)
	if err != nil {
		slog.Error("drive", "error", err)
		os.Exit(1)
	}

	w := a.NewWindow(a.Metadata().Name)
	w.SetPadded(false)
	t := textos(l)
	p := drive.NuevaPantalla(tr, svc, nil, w)
	p.EnEquipo = enEquipo(a, disco, rem, t, carpeta, l.Idioma())
	if carpeta != nil {
		l.AjustesDeLaApp(t("carpetaTitulo"), carpeta.seccion(p, t))
	}
	sinc := &sincronizador{r: rem, p: p, t: t}
	l.AlCerrar(sinc.vigilar())
	p.IA = drive.FuncionIA(l.Asistir())
	l.AlCambiarIA(func() { fyne.Do(func() { p.IA = drive.FuncionIA(l.Asistir()) }) })
	// «Combinar PDF», con el mismo motor de CertiPDF en el dispositivo.
	if pdf, err := api.NewPDF("https://sin-servidor.invalid", "https://sin-servidor.invalid", nil, nil).WithPrivateDrive(disco); err == nil {
		p.ConPDF(pdf)
	}
	p.ElegirFicheros = func(listo func([]drive.Entrada)) {
		local.ElegirFichero(func(rutas []string) { listo(drive.Rutas(rutas)) })
	}
	switch {
	case !local.Tactil():
		p.ElegirCarpeta = func(titulo string, listo func([]drive.Entrada)) {
			local.ElegirCarpeta(titulo, func(ruta string) { listo(drive.Rutas([]string{ruta})) })
		}
	case carpeta != nil:
		// En Android, «Subir carpeta» con el selector de carpetas del sistema (arbol.go).
		p.ElegirCarpeta = func(_ string, listo func([]drive.Entrada)) {
			go func() {
				entradas, err := carpeta.entradasDe()
				fyne.Do(func() {
					if err != nil {
						local.Info(w, t("noSeSube"), err.Error())
						return
					}
					if len(entradas) > 0 {
						listo(entradas)
					}
				})
			}()
		}
	}

	// Soltar en la ventana sube a la carpeta abierta, como en Drive.
	w.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) { p.Subir(rutasLocales(uris)) })
	soltar.AlArrastrar(w, p.Arrastrando)
	ahora := func(f func(*drive.Pantalla)) { f(p) }
	drive.EngancharAtajos(w.Canvas(), ahora, ahora)
	// Lo que cambió otro programa se ve al volver a la ventana (§6.4, «Refresco»).
	ultima := time.Now()
	a.Lifecycle().SetOnEnteredForeground(func() {
		if time.Since(ultima) > 2*time.Second {
			ultima = time.Now()
			p.RefrescarUnidades()
			p.Recargar()
			go sinc.ahora()
		}
	})
	// Lo que llega de fuera («Nube → Abrir de la nube…», los argumentos) se guarda en la
	// carpeta abierta: en Drive, abrir un fichero de otro sitio es traerlo.
	l.AlAbrirFichero(func(_ context.Context, ap api.Apertura) error {
		return traer(p, disco.Raiz(), ap) //nolint:contextcheck // traer entrega el fichero a la pantalla, y lo que esta carga después vive con ella
	}, func(err error) { l.Fallo(w, err) })
	l.AlCerrar(p.Cerrar)

	w.SetContent(l.Montar(w, iconos.Drive, p.Objeto()))
	w.Resize(fyne.NewSize(anchoInicial, altoInicial))
	p.Arrancar(true)
	if perdida != "" {
		local.Info(w, t("carpetaTitulo"), strings.ReplaceAll(t("carpetaPerdida"), "{name}", perdida))
	}
	// Una carpeta o un fichero de Drive por su ruta desde la raíz (`?clase=carpeta&id=/Proyectos`,
	// `?clase=fichero&id=/Proyectos/plan.pdf`): se entra en su carpeta y el fichero se abre.
	l.AlAbrirElemento(func(clase, id string) {
		switch clase {
		case "carpeta":
			p.IrAEnlace(id, "", nil)
		case "fichero":
			p.IrAEnlace(path.Dir(id), id, nil)
		}
	})
	w.ShowAndRun()
}

// raizDrive es la carpeta de Drive. En escritorio y Android, `<datos>/Drive`
// (`~/Sytue Office/Drive`). En iOS, `Drive` directamente en los Documentos de la app, que
// es lo que la app «Archivos» enseña de ella (DR-12).
func raizDrive(a fyne.App, l *local.App) string {
	if runtime.GOOS == "ios" {
		// La raíz de Fyne en iOS es `<Documentos>/fyne`.
		return filepath.Join(filepath.Dir(a.Storage().RootURI().Path()), "Drive")
	}
	return filepath.Join(l.Espacio.Rutas.Datos, "Drive")
}

// rutasLocales se queda con lo que es del disco: un fichero o una carpeta.
func rutasLocales(uris []fyne.URI) []string {
	var rutas []string
	for _, u := range uris {
		if u == nil || u.Scheme() != "file" {
			continue
		}
		if _, err := os.Stat(u.Path()); err == nil {
			rutas = append(rutas, u.Path())
		}
	}
	return rutas
}

// traer sube a la carpeta abierta un fichero que llega de fuera. Se llama fuera de la
// interfaz.
func traer(p *drive.Pantalla, raiz string, ap api.Apertura) error {
	if u, err := storage.ParseURI(ap.Local); err == nil && u.Scheme() == "file" {
		// Un fichero que YA está en Drive (lo abre el Hub, que no tiene otra app para
		// él) se enseña en su carpeta; subirlo lo duplicaría.
		if rel, ok := dentroDeDrive(raiz, u.Path()); ok {
			fyne.Do(func() { p.IrAEnlace(path.Dir(rel), rel, nil) })
			return nil
		}
		fyne.Do(func() { p.Subir([]string{u.Path()}) })
		return nil
	}
	nombre, datos, err := local.LeerLocal(ap.Local)
	if err != nil {
		return err
	}
	e := drive.Fluido(nombre, int64(len(datos)), io.NopCloser(bytes.NewReader(datos)))
	fyne.Do(func() { p.SubirEntradas([]drive.Entrada{e}) })
	return nil
}

// ctxCorto es el plazo de lo que se hace en el disco a petición de la persona.
func ctxCorto() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Minute)
}

// dentroDeDrive da la ruta de un fichero de dentro de la raíz de Drive tal como la
// entiende IrAEnlace («/Proyectos/plan.pdf»), o false si está fuera.
func dentroDeDrive(raiz, ruta string) (string, bool) {
	if raiz == "" {
		return "", false
	}
	rel, err := filepath.Rel(filepath.Clean(raiz), filepath.Clean(filepath.FromSlash(ruta)))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return "/" + filepath.ToSlash(rel), true
}

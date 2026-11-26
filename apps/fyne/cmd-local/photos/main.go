// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command photos es Sytue Photos de Sytue Office: una fototeca en este equipo, con sus
// fotos en claro, sin cuenta y sin servidor (apps/fyne/PLAN-EDICION-LOCAL.md §6.8).
//
// «Abrir con» una imagen (del sistema o desde Drive) la abre directamente en el editor
// (FO-06); «Importar» trae las fotos y los vídeos del carrete en el móvil y, en el
// escritorio, ENLAZA sin copiar las fotos y los vídeos de una carpeta o de las carpetas
// vigiladas (FO-04, FO-09; los vídeos se ven en el visor, video.go); «Añadir fotos» y
// soltar en la ventana admiten también vídeos, que se copian (copiar.go); las HEIC entran
// convertidas a JPEG por el sistema (heic.go). Vaciar la papelera libera el disco en el
// acto (fototeca.go).
//
// Comparado con `cmd/photos`: no hay `sesion.Abrir` (ni OIDC ni llavero de sesión), ni
// `cifrado.NewPrivateApplication` (ni Matrix ni dispositivo verificado), ni el segundo
// repositorio de Drive donde la conectada deja los originales. Hay UNA fototeca, en
// SQLite con sus índices (`apps/nucleo/local/fototeca`, el plan B de §5.6: con el motor de
// registros, 50.000 fotos tardaban 27 s en abrir), con las reglas del repositorio de la
// conectada (`apps/nucleo/photos`), y la del almacén de antes se migra la primera vez que
// se abre, enseñando cómo va. Los originales entran por la ingesta directa
// (`ImportarFoto`, FO-01): «Añadir fotos», soltarlas en la ventana, «Abrir con» o
// pasarlas en la línea de órdenes (FO-06), también en WebP, AVIF y BMP (FO-03). Los años,
// los recuerdos, las repetidas (por huella), «Por aquí», vaciar la papelera y la portada
// de un álbum se calculan en la fototeca (FO-02), y
// «Exportar la fototeca» saca los originales a `AAAA/MM/` (FO-05). Compartir, enlaces
// públicos, «Compartido conmigo» y el directorio no se montan.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/local"

	// Registra AVIF (libavif en WASM, sin CGO), TIFF y BMP para la ingesta y las derivadas,
	// y convierte las HEIC con el sistema: lo mismo que leen las notas de imagen de Notepads.
	"git.sytue.eu/sytue/one/apps/fyne/local/imagenes"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/carrete"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// Ventana inicial: la misma que la conectada.
const (
	anchoInicial = 1280
	altoInicial  = 800
)

// sinRed es la raíz que se le da a la fachada: nunca se llama, porque `WithPrivateData`
// cambia el transporte por uno que falla cerrado sin salir del proceso.
const sinRed = "https://sin-red.invalid"

func main() {
	a := local.NuevaApp("")
	l, err := local.Abrir(a, "photos")
	if err != nil {
		slog.Error("photos", "error", err)
		os.Exit(1)
	}
	tema.ElegirCromo(tema.CromoPapel)
	tr, err := photos.Traductor(l.Idioma())
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}
	nombre := "Sytue Photos Office"
	if m := a.Metadata(); m.Name != "" {
		nombre = m.Name
	}
	w := a.NewWindow(nombre)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(anchoInicial, altoInicial))

	// La primera vez con la fototeca del almacén de antes, se pasa a SQLite (§5.6, plan B)
	// enseñando cómo va: con muchas fotos tarda (nucleo/local/fototeca/migrar.go).
	if fototeca.HayQueMigrar(l.Espacio.Rutas.Producto("photos")) {
		avance := pantallaDeMigrar(l, w)
		go func() {
			repo, cli, copias, err := abrirFototeca(l, avance)
			fyne.Do(func() {
				if err != nil {
					fallaLaFototeca(l, w, tr, err)
					return
				}
				montar(a, l, w, tr, repo, cli, copias)
			})
		}()
		w.ShowAndRun()
		return
	}
	repo, cli, copias, err := abrirFototeca(l, nil)
	if err != nil {
		fallaLaFototeca(l, w, tr, err)
		w.ShowAndRun()
		return
	}
	montar(a, l, w, tr, repo, cli, copias)
	w.ShowAndRun()
}

// fallaLaFototeca dice en la ventana que la fototeca no se pudo abrir y por qué.
func fallaLaFototeca(l *local.App, w fyne.Window, tr *i18n.Translator, err error) {
	slog.Error("photos: no se pudo abrir la fototeca", "error", err)
	detalle := err.Error()
	if errors.Is(err, fototeca.ErrEnUso) {
		detalle = texto(l, "migrar.enUso")
	}
	w.SetContent(l.Montar(w, iconos.Photos, ui.NuevoVacio(tr.T("matrix.failed", nil), detalle)))
}

// pantallaDeMigrar pone en la ventana que se está poniendo al día la fototeca, con una
// barra, y da cómo moverla.
func pantallaDeMigrar(l *local.App, w fyne.Window) func(fototeca.Avance) {
	barra := widget.NewProgressBarInfinite()
	medida := widget.NewProgressBar()
	medida.Hide()
	cuerpo := container.NewVBox(ui.NuevoVacio(texto(l, "migrar"), texto(l, "migrar.texto")), container.NewStack(barra, medida))
	w.SetContent(l.Montar(w, iconos.Photos, container.NewCenter(container.NewGridWrap(fyne.NewSize(520, cuerpo.MinSize().Height), cuerpo))))
	return func(a fototeca.Avance) {
		if a.Fase != fototeca.FaseCopiar || a.Total == 0 {
			return
		}
		fyne.Do(func() {
			if barra.Visible() {
				barra.Stop()
				barra.Hide()
				medida.Show()
			}
			medida.TextFormatter = func() string { return fmt.Sprintf(texto(l, "exportar.avance"), a.Hechas, a.Total) }
			medida.SetValue(float64(a.Hechas) / float64(a.Total))
		})
	}
}

// montar pone la fototeca abierta en la ventana: la pantalla compartida de Photos con lo
// que esta edición le añade.
func montar(a fyne.App, l *local.App, w fyne.Window, tr *i18n.Translator, repo *fototeca.Fototeca, cli *api.Photos, copias *copiasParaVer) {
	miniaturas, grandes := photos.NuevaFototeca(), photos.NuevaFototeca()
	grandes.Variante = photos.VarianteGrande
	vista := photos.Vista{
		T: tr, Teca: miniaturas, TecaGrande: grandes, Ventana: anchoInicial,
		Variante: tema.De(a.Settings().ThemeVariant()), SinServidor: true, Textos: textosDeVista(l),
	}
	p := photos.Nueva(cli, vista, w)
	// Sin Drive: «Añadir fotos» importa directamente a la fototeca (CanImportPrivatePhotos).
	// Sin directorio ni web: no se comparte con personas ni se abre nada fuera.
	p.Elegir = func() ([]string, error) {
		// Fotos y vídeos (FO-09): en iOS, el selector de la fototeca (PHPicker) con los dos;
		// en el resto, el de ficheros filtrado por ellas.
		rutas, err := ficheros.ElegirFotosYVideos(tr.T("gallery.add", nil), extensionesParaAnadir()...)
		if err != nil {
			return nil, err
		}
		// Las HEIC entran ya en JPEG, con lo que el sistema tenga (heic.go).
		return conHEICConvertidas(rutas), nil
	}
	// Los vídeos elegidos o soltados se copian a la fototeca, con su póster (FO-09).
	p.ImportarVideo = func(ctx context.Context, ruta string) error { return importarVideoElegido(ctx, repo, ruta) }
	p.Guardar = func(nombre string, datos []byte) (string, error) {
		if runtime.GOOS == "ios" || runtime.GOOS == "android" {
			if err := guardarEnGaleria(p, nombre, datos); !errors.Is(err, carrete.ErrNoDisponible) {
				return "", err
			}
		}
		carpeta, err := ficheros.Carpeta()
		if err != nil {
			return "", err
		}
		return ficheros.Guardar(carpeta, nombre, bytes.NewReader(datos))
	}
	miniaturas.Pedir = func(ctx context.Context, id, variante string) ([]byte, error) {
		return leer(cli.Variant(ctx, id, variante))
	}
	grandes.Pedir = func(ctx context.Context, id, variante string) ([]byte, error) {
		return leer(cli.VariantDe(ctx, id, variante, p.Huella(id)))
	}
	p.Galeria = photos.CarreteDelSistema{}
	switch runtime.GOOS {
	case "darwin":
		p.DestinoGaleria = photos.GaleriaFotos
	case "windows", "linux":
		p.DestinoGaleria = photos.GaleriaImagenes
	}
	p.SeguirEnFondo = sistema.SeguirEnFondo
	prepararVideos(l, p, cli, copias)
	// Lo que se veía de los vídeos que ya no están (su enlace o su copia en la caché).
	copias.podarEnFondo()
	// La IA con clave propia, si la persona la ha configurado: describir y preguntar
	// (visión), editar con IA y ampliar la búsqueda (FO-07). Apagada, nil, y la pantalla
	// lo dice.
	p.IA = asistente(l)
	l.AlCambiarIA(func() {
		ia := asistente(l)
		fyne.Do(func() { p.IA = ia })
	})
	// «Exportar en formatos abiertos» (Ajustes → Copia): los originales tal cual, en AAAA/MM.
	l.AlExportarTodo(func(ctx context.Context, carpeta string) error {
		_, err := repo.ExportarFototeca(ctx, carpeta, func(int, int) {})
		return err
	})
	p.ExportarFototeca = &photos.AccionExtra{Rotulo: texto(l, "exportar"), Al: func() { exportarFototeca(l, w, repo) }}
	// «Importar»: del carrete en el móvil; de una carpeta y las vigiladas en el escritorio
	// (FO-04, FO-09), con el motor de la copia de la conectada.
	im := nuevoImportador(l, w, p, repo, cli)
	p.ImportarFototeca = im.accion()
	p.CambiarSesion("local\x00" + l.Espacio.ID)
	l.AlCerrar(p.Cerrar)

	// Un fichero de imagen del equipo («Abrir con», la línea de órdenes, la nube): se
	// importa si no estaba ya (por su huella) y se abre en el visor.
	l.AlAbrirFichero(func(ctx context.Context, ap api.Apertura) error {
		return abrirFichero(ctx, repo, cli, p, ap)
	}, func(err error) { l.Fallo(w, err) })
	soltar.AlArrastrar(w, p.Arrastrando)

	w.SetContent(l.Montar(w, iconos.Photos, p.Objeto()))
	// Arrancar pone su propio receptor de ficheros soltados: importa a la galería.
	p.Arrancar()
	l.AlCerrar(im.arrancar())
	ultima := time.Now()
	a.Lifecycle().SetOnEnteredForeground(func() {
		if time.Since(ultima) > 2*time.Second {
			ultima = time.Now()
			im.automatico()
		}
	})
	// Una foto (`?clase=foto&id=…`) en el visor, o un álbum (`?clase=album&id=…`).
	l.AlAbrirElemento(func(clase, id string) {
		switch clase {
		case "album":
			p.AbrirAlbum(id)
		case "foto":
			go func() {
				if f, err := repo.Photo(context.Background(), id); err == nil {
					fyne.Do(func() { p.AbrirFoto(f) })
				}
			}()
		}
	})
}

// formatosAdmitidos son los que la fototeca local admite además de JPEG, PNG y GIF, como
// los nombra `image.RegisterFormat`: los que se miden y derivan en el dispositivo.
var formatosAdmitidos = []string{"webp", "avif", "bmp"}

// abrirFototeca abre la fototeca de SQLite de la carpeta de Photos (y la migra del
// almacén de antes si hace falta, diciendo cómo va a avance) y le pone delante la fachada
// de las pantallas, y da con qué ver los vídeos copiados a ella. La fototeca se cierra con
// la app; mientras está abierta, la copia de seguridad la copia en caliente.
func abrirFototeca(l *local.App, avance func(fototeca.Avance)) (*fototeca.Fototeca, *api.Photos, *copiasParaVer, error) {
	// Se mide y se deriva en el dispositivo: WebP (con `miniatura`), AVIF y BMP (con los
	// decodificadores registrados arriba) entran como los demás.
	repo, err := fototeca.Abrir(context.Background(), l.Espacio.Rutas.Producto("photos"), l.Espacio.ID, fototeca.Opciones{Formatos: formatosAdmitidos, Avance: avance})
	if err != nil {
		return nil, nil, nil, err
	}
	l.AlCerrar(repo.Close)
	l.AlCerrar(l.Copia.BaseAbierta(repo.Ruta(), repo.CopiarEnCaliente))
	copias := nuevasCopiasParaVer(filepath.Join(l.Espacio.Rutas.Cache, "photos-videos"), repo, repo.RutaEnClaro)
	cli, err := api.NewPhotos(sinRed, nil, nil).WithPrivateData(&fototecaLocal{Fototeca: repo, copias: copias})
	return repo, cli, copias, err
}

// abrirFichero importa (o reencuentra) la imagen de ap y la abre directamente en el
// editor (FO-06): es lo que se quiere al abrir una foto del sistema o de Drive con Photos.
// Una HEIC se pasa antes a JPEG con lo que tenga el sistema (heic.go).
func abrirFichero(ctx context.Context, repo *fototeca.Fototeca, cli *api.Photos, p *photos.App, ap api.Apertura) error {
	if ap.Local == "" {
		return local.ErrSinFichero
	}
	nombre, datos, err := local.LeerLocal(ap.Local)
	if err != nil {
		return err
	}
	if imagenes.EsHEIC(nombre) {
		if nombre, datos, err = imagenes.AJPEG(ctx, nombre, datos); err != nil {
			return err
		}
	}
	suma := sha256.Sum256(datos)
	foto, ya, err := repo.FotoPorHuella(ctx, hex.EncodeToString(suma[:]))
	if err != nil {
		return err
	}
	if !ya {
		nombre = filepath.Base(nombre)
		tipo := mime.TypeByExtension(strings.ToLower(filepath.Ext(nombre)))
		if !strings.HasPrefix(tipo, "image/") {
			return errors.New("photos: " + nombre + " no es una imagen")
		}
		c, fin := context.WithTimeout(ctx, photos.PlazoSubida(int64(len(datos))))
		defer fin()
		foto, err = cli.ImportarFoto(c, uuid.NewString(), nombre, tipo, bytes.NewReader(datos), int64(len(datos)))
		if err != nil {
			return err
		}
	}
	fyne.Do(func() {
		// La línea de tiempo se vuelve a pedir (la nueva cae en su fecha) y encima se
		// abre la foto en el editor; «Volver» deja en la galería.
		p.Ir(photos.RutaFotos)
		p.AbrirEditor(foto, "")
	})
	return nil
}

// guardarEnGaleria deja los bytes en un temporal y de ahí en la galería del sistema.
func guardarEnGaleria(p *photos.App, nombre string, datos []byte) error {
	f, err := os.CreateTemp("", "sytue-guardar-*"+filepath.Ext(nombre))
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	_, err = f.Write(datos)
	if cerrar := f.Close(); err == nil {
		err = cerrar
	}
	if err != nil {
		return err
	}
	c, fin := context.WithTimeout(context.Background(), 2*time.Minute)
	defer fin()
	return p.GuardarEnGaleria(c, f.Name(), nombre)
}

// leer trae entera una variante: la Fototeca guarda los bytes, no el flujo.
func leer(r io.ReadCloser, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return io.ReadAll(r)
}

// asistente es la IA de la cáscara como la pide la pantalla; nil con la IA apagada.
func asistente(l *local.App) api.Asistente {
	if f := l.Asistir(); f != nil {
		return api.AsistenteFunc(f)
	}
	return nil
}

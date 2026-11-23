// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/carrete"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/galeriacifrada"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/render"
)

// # La copia de seguridad en la app
//
// El motor está en copia.go; esto es cuándo corre con la app abierta y qué se ve. Con la
// app cerrada corre copia_fondo.go. Es solo de la app nativa: la web no tiene galería del
// sistema que leer.
//
//   - **Se activa por dispositivo** (AjustesCopia, en el registro): en el teléfono sí y
//     en el portátil del trabajo no, por ejemplo. Activarla pide el permiso de la
//     fototeca, si hace falta. Por defecto solo con Wi-Fi y con vídeos.
//   - **Corre sola** al abrir la app, al volver a ella y cada [CadaRondaCopia] mientras
//     está abierta; con la app cerrada, cuando el sistema la despierta (AlCambiarCopia
//     le dice cuándo). Una ronda que no tiene nada que subir solo lee la galería.
//   - **Se ve** en la línea de estado de la galería mientras sube («Copiando 3 de 120…»),
//     con el número de fotos nuevas al terminar, y en el diálogo de la nube de la barra.
//   - **Es de la cuenta abierta**: cambiar de cuenta corta la ronda (la vida de la sesión,
//     `enSesion`), y cada cuenta tiene su registro.
//   - **Termina lo empezado**: si la app pasa a segundo plano a media subida, pide al
//     sistema unos segundos para acabarla (AlFondoCopia).

// CadaRondaCopia es cada cuánto se mira la galería con la app abierta.
const CadaRondaCopia = 15 * time.Minute

// estadoCopia es lo que la interfaz sabe de la copia.
type estadoCopia struct {
	// progreso es el de la ronda en marcha; nil, ninguna.
	progreso *ProgresoCopia
	// fallo es cómo acabó la última ronda, si acabó mal.
	fallo error
	// reloj es el de las rondas con la app abierta.
	reloj *time.Ticker
	// dialogo y texto son el diálogo de la copia, si está abierto, y su línea de estado.
	dialogo *ui.Dialogo
	texto   *ui.Parrafo
}

// HayCopia dice si este dispositivo puede hacer copia: hay galería del sistema y a
// dónde subir.
func (a *App) HayCopia() bool {
	return a.Copia != nil && a.Galeria != nil && a.Galeria.Disponible() && a.Drive != nil && a.cli != nil
}

// CopiaActiva dice si la copia está encendida en este dispositivo.
func (a *App) CopiaActiva() bool { return a.HayCopia() && a.Copia.Ajustes().Activa }

// iniciarCopia pone en marcha el reloj de las rondas y la primera, y vuelve a decirle al
// sistema cuándo despertar a la app. En Arrancar.
func (a *App) iniciarCopia() {
	if !a.HayCopia() {
		return
	}
	a.avisarCambioCopia()
	if a.cop.reloj != nil {
		return
	}
	a.cop.reloj = time.NewTicker(CadaRondaCopia)
	reloj := a.cop.reloj
	go func() {
		for range reloj.C {
			fyne.Do(a.rondaCopia)
		}
	}()
	a.rondaCopia()
}

// avisarCambioCopia le pasa los ajustes a quien programa la copia con la app cerrada.
func (a *App) avisarCambioCopia() {
	if a.AlCambiarCopia != nil && a.Copia != nil {
		a.AlCambiarCopia(a.Copia.Ajustes())
	}
}

// rondaCopia lanza una ronda si toca: encendida, con sesión, con permiso y sin otra en
// marcha. Corre en el hilo de la interfaz y no bloquea.
func (a *App) rondaCopia() {
	if !a.CopiaActiva() || a.identidad == "" || a.cop.progreso != nil || a.Copia.EnMarcha() {
		return
	}
	if !a.Galeria.Permiso().Lee() {
		a.cop.fallo = carrete.ErrSinPermiso
		a.pintarCopia()
		return
	}
	cuenta := a.identidad
	a.cop.progreso, a.cop.fallo = &ProgresoCopia{}, nil
	a.pintarCopia()
	a.enSesion(func(vida context.Context) {
		d, err := a.destinoSubida(vida)
		if err != nil {
			//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
			fyne.Do(func() { a.finRondaCopia(cuenta, ProgresoCopia{}, err) })
			return
		}
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		p, err := a.Copia.Ronda(vida, cuenta, d, func(p ProgresoCopia) {
			fyne.Do(func() {
				if a.identidad == cuenta && a.cop.progreso != nil {
					*a.cop.progreso = p
					a.pintarCopia()
				}
			})
		})
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		fyne.Do(func() { a.finRondaCopia(cuenta, p, err) })
	})
}

// finRondaCopia recoge el final de una ronda en el hilo de la interfaz.
func (a *App) finRondaCopia(cuenta string, p ProgresoCopia, err error) {
	if a.terminarFondo != nil {
		a.terminarFondo()
		a.terminarFondo = nil
	}
	if a.identidad != cuenta {
		a.rondaCopia() // la de otra cuenta, ya cortada: ahora le toca a la nueva
		return
	}
	a.cop.progreso = nil
	if errors.Is(err, context.Canceled) || errors.Is(err, ErrCopiaEnMarcha) {
		err = nil
	}
	a.cop.fallo = err
	if p.Subidas > 0 {
		// El progreso va en línea; cómo acabó, en el aviso, como «Añadir fotos».
		a.avisar(ui.AvisoExito, a.v.T.T("backup.done", i18n.Vars{"count": p.Subidas}), "")
		// Las nuevas caen donde les toque por fecha. Solo si no se está haciendo otra
		// cosa con la galería, para no quitarle a nadie lo que mira.
		if a.ruta == RutaFotos && a.g.subida == nil && a.g.seleccion == nil && a.vis == nil && a.ed == nil && a.g.texto == "" {
			a.gen++
			a.g = estadoGaleria{}
			a.cargarMas()
		}
	}
	a.pintarCopia()
}

// DriveCifradoAbierto es el Drive de la bóveda recién abierto en esta app (lo llama
// quien la monta, en el hilo de la interfaz). La copia que esperaba por él sigue ya, sin
// esperar a la ronda siguiente —también la que dejó esperando la copia con la app
// cerrada—, y la galería cifrada que estaba cerrada se lee.
func (a *App) DriveCifradoAbierto() {
	if a.CopiaActiva() && a.identidad != "" &&
		(errors.Is(a.cop.fallo, ErrDriveCifradoCerrado) || a.Copia.EsperandoBoveda(a.identidad)) {
		a.cop.fallo = nil
		a.rondaCopia()
	}
	if a.cif.pedida && a.cif.cerrada && !a.cif.cargando {
		a.cargarCifradas(false)
	}
}

// AlFondoCopia es la app pasando a segundo plano: si hay una ronda a medias, pide al
// sistema tiempo para acabar la subida en curso (SeguirEnFondo, en iOS). La ronda no se
// corta: si el sistema suspende la app, sigue al volver.
func (a *App) AlFondoCopia() {
	if a.cop.progreso != nil && a.SeguirEnFondo != nil && a.terminarFondo == nil {
		a.terminarFondo = a.SeguirEnFondo("sytue-photos-copia")
	}
}

// pintarCopia repinta lo que enseña la copia: la galería (su línea de estado) y el
// diálogo, si está abierto.
func (a *App) pintarCopia() {
	if a.cop.texto != nil {
		a.cop.texto.Texto = a.textoCopia()
		a.cop.texto.Refresh()
		if a.cop.dialogo != nil {
			a.cop.dialogo.Recolocar()
		}
	}
	if a.ruta == RutaFotos && a.vis == nil && a.ed == nil {
		a.pintar()
	}
}

// avisoCopia es la línea de estado de la galería mientras la copia sube algo.
func (a *App) avisoCopia() string {
	if p := a.cop.progreso; p != nil && p.Total > 0 {
		return a.v.T.T("backup.running", i18n.Vars{"done": p.Hechas, "total": p.Total})
	}
	return ""
}

// textoCopia es lo que dice el diálogo de cómo está la copia.
func (a *App) textoCopia() string {
	switch {
	case !a.CopiaActiva():
		return a.v.Traduce("backup.off")
	case a.cop.progreso != nil && a.cop.progreso.Total > 0:
		return a.avisoCopia()
	case errors.Is(a.cop.fallo, carrete.ErrSinPermiso) || a.Galeria.Permiso() == carrete.Denegado:
		return a.v.Traduce("backup.noPermission")
	case errors.Is(a.cop.fallo, ErrCopiaEsperaWifi):
		return a.v.Traduce("backup.waitingWifi")
	case a.cop.fallo != nil:
		return a.v.Traduce("backup.paused")
	case a.Galeria.Permiso() == carrete.Limitado:
		return a.v.Traduce("backup.limited")
	case a.cop.progreso == nil && !a.Copia.Ultima(a.identidad).IsZero():
		return a.v.Traduce("backup.upToDate")
	default:
		return a.v.Traduce("backup.waiting")
	}
}

// abrirCopia es el diálogo de la nube de la barra: qué es, cómo va, sus dos ajustes y
// encenderla o apagarla.
func (a *App) abrirCopia() {
	if !a.HayCopia() || a.w == nil {
		return
	}
	verbo := "backup.turnOn"
	if a.CopiaActiva() {
		verbo = "backup.turnOff"
	}
	ancho := anchoCuerpoDialogo(a)
	escalon := tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}
	parrafo := func(clave, token string) *ui.Parrafo {
		return ui.NuevoParrafo(a.v.Traduce(clave), escalon, ui.Normal, token, a.v.Variante)
	}
	casilla := func(clave string, marcada bool, cambiar func(*AjustesCopia, bool)) fyne.CanvasObject {
		c := ui.NuevaCasilla(a.v.Traduce(clave), func(si bool) { a.CambiarAjustesCopia(func(aj *AjustesCopia) { cambiar(aj, si) }) })
		c.Marcada, c.Escalon, c.AnchoEtiqueta = marcada, escalon, ancho-ui.MedidaEleccion("").Width
		return c
	}
	aj := a.Copia.Ajustes()
	estado := parrafo("", "text-muted")
	estado.Texto = a.textoCopia()
	// Con la app cerrada sigue en el móvil; en el ordenador, mientras está abierta.
	cerrada := "backup.whileOpen"
	if a.FondoDisponible {
		cerrada = "backup.closed"
	}
	bloques := []ui.Bloque{
		{Obj: ui.AlAncho(ancho, parrafo("backup.description", "text")), Margen: tema.Espacio(2)},
		{Obj: ui.AlAncho(ancho, parrafo("backup.driveOnly", "text-muted")), Margen: tema.Espacio(3)},
		{Obj: ui.AlAncho(ancho, parrafo(cerrada, "text-muted")), Margen: tema.Espacio(3)},
		{Obj: casilla("backup.mobileData", aj.ConDatos, func(aj *AjustesCopia, si bool) { aj.ConDatos = si }), Margen: tema.Espacio(1)},
		{Obj: casilla("backup.videos", !aj.SinVideos, func(aj *AjustesCopia, si bool) { aj.SinVideos = !si }), Margen: tema.Espacio(3)},
		{Obj: ui.AlAncho(ancho, estado)},
	}
	var d *ui.Dialogo
	cerrar := func() {
		d.Cerrar()
		a.cop.dialogo, a.cop.texto = nil, nil
	}
	d = ui.Confirmacion(a.v.Traduce("backup.title"), a.v.Traduce("ui.close"), a.v.Traduce(verbo), false,
		ui.PilaBloques(bloques...), cerrar, func() {
			cerrar()
			a.CambiarCopia(!a.CopiaActiva())
		})
	d.RotuloCerrar = a.v.Traduce("ui.close")
	a.cop.dialogo, a.cop.texto = d, estado
	d.Mostrar(a.w.Canvas())
}

// CambiarAjustesCopia cambia los ajustes de la copia de este dispositivo, se lo dice a
// quien la programa con la app cerrada y, si ahora se puede, lanza una ronda (permitir los
// datos móviles con la copia esperando a una Wi-Fi, por ejemplo).
func (a *App) CambiarAjustesCopia(f func(*AjustesCopia)) {
	if !a.HayCopia() {
		return
	}
	_ = a.Copia.CambiarAjustes(f)
	a.avisarCambioCopia()
	a.cop.fallo = nil
	a.pintarCopia()
	a.rondaCopia()
}

// CambiarCopia enciende o apaga la copia en este dispositivo. Encenderla pide antes el
// permiso de la fototeca (el diálogo del sistema), y sin él no se enciende.
func (a *App) CambiarCopia(encender bool) {
	if !a.HayCopia() {
		return
	}
	if !encender {
		a.CambiarAjustesCopia(func(aj *AjustesCopia) { aj.Activa = false })
		return
	}
	go func() {
		c, fin := context.WithTimeout(context.Background(), 5*time.Minute)
		defer fin()
		permiso, _ := a.Galeria.PedirPermiso(c)
		fyne.Do(func() {
			if !permiso.Lee() {
				a.cop.fallo = carrete.ErrSinPermiso
				a.avisar(ui.AvisoPeligro, a.v.Traduce("backup.noPermission"), "")
				a.pintarCopia()
				return
			}
			a.CambiarAjustesCopia(func(aj *AjustesCopia) { aj.Activa = true })
			a.iniciarCopia()
		})
	}()
}

// destinoDrive es «/Fotos» de Drive y el índice de syphotos, para la copia de seguridad
// y para «Añadir fotos». Lo usa también la copia con la app cerrada, sin App.
type destinoDrive struct {
	drive   *api.Drive
	fotos   *api.Photos
	carpeta string
	// nombres es lo que ya hay en «/Fotos» de un Drive privado, para no pisar nada:
	// ahí subir con un nombre que ya está crea una versión nueva de ese fichero, y no
	// hay `keep_both` que lo evite. Se lee una vez por destino.
	nombres map[string]bool
	// cifrada es la galería cifrada del Drive privado (ADR 0086), donde se apunta cada
	// foto que se sube ahí. Se crea con la primera.
	cifrada *galeriacifrada.Galeria
}

// privado: el Drive es el privado (la bóveda o Matrix), y la galería del servidor no
// puede indexar lo que se sube ahí.
func (d *destinoDrive) privado() bool {
	return d.drive.Privado() && (d.fotos == nil || !d.fotos.CanImportPrivatePhotos())
}

func nuevoDestinoDrive(drive *api.Drive, fotos *api.Photos) *destinoDrive {
	return &destinoDrive{drive: drive, fotos: fotos}
}

// destinoSubida es a dónde suben «Añadir fotos» y la copia: el Drive de siempre o,
// si el del servidor ya no admite escrituras, el privado ([App.DriveDeSubida]). Se
// pregunta en cada subida: la persona puede pasar su Drive a la bóveda con la
// galería abierta.
func (a *App) destinoSubida(vida context.Context) (*destinoDrive, error) {
	if a.DriveDeSubida == nil || (a.cli != nil && a.cli.CanImportPrivatePhotos()) {
		return nuevoDestinoDrive(a.Drive, a.cli), nil
	}
	drive, err := a.DriveDeSubida(vida)
	if err != nil {
		return nil, err
	}
	return nuevoDestinoDrive(drive, a.cli), nil
}

func (d *destinoDrive) carpetaFotos(vida context.Context) (string, error) {
	if d.fotos != nil && d.fotos.CanImportPrivatePhotos() {
		return "", nil
	}
	if d.carpeta != "" {
		return d.carpeta, nil
	}
	id, err := carpetaFotosEn(vida, d.drive)
	if err == nil {
		d.carpeta = id
	}
	return id, err
}

// Contenido lee «/Fotos» entera: las sumas para no subir dos veces y los nombres para no
// pisar ninguno. Solo la primera ronda y una vez al día (RefrescoContenidoCopia).
func (d *destinoDrive) Contenido(vida context.Context) (map[string]bool, map[string]bool, error) {
	carpeta, err := d.carpetaFotos(vida)
	if err != nil {
		return nil, nil, err
	}
	sumas, nombres := map[string]bool{}, map[string]bool{}
	for cursor := ""; ; {
		c, fin := ctx(vida)
		hijos, err := d.drive.Children(c, carpeta, cursor)
		fin()
		if err != nil {
			return nil, nil, err
		}
		for _, f := range hijos.Files {
			nombres[f.Name] = true
			if f.Checksum != "" {
				sumas[f.Checksum] = true
			}
		}
		for _, f := range hijos.Folders {
			nombres[f.Name] = true
		}
		if cursor = hijos.NextCursor; cursor == "" {
			return sumas, nombres, nil
		}
	}
}

// Subir deja el fichero en «/Fotos» sin sustituir ninguno (`keep_both`), con un plazo a
// la medida del fichero: un vídeo de 2 GB no sube en diez minutos.
func (d *destinoDrive) Subir(vida context.Context, ruta, nombre, tipo string) (Subido, error) {
	carpeta, err := d.carpetaFotos(vida)
	if err != nil {
		return Subido{}, err
	}
	f, err := os.Open(ruta) //nolint:gosec // la copia local que ha dejado la galería, o lo que la persona ha elegido
	if err != nil {
		return Subido{}, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return Subido{}, err
	}
	c, fin := context.WithTimeout(vida, PlazoSubida(info.Size()))
	defer fin()
	if d.fotos != nil && d.fotos.CanImportPrivatePhotos() {
		foto, err := d.fotos.ImportarFoto(c, uuid.NewString(), nombre, tipo, f, info.Size())
		if err != nil {
			return Subido{}, err
		}
		return Subido{ID: foto.ID, Nombre: foto.Name}, nil
	}
	if d.privado() {
		if d.nombres == nil {
			if _, d.nombres, err = d.Contenido(vida); err != nil {
				return Subido{}, err
			}
		}
		nombre = nombreLibre(nombre, d.nombres)
	}
	fichero, err := d.drive.UploadConservando(c, carpeta, nombre, tipo, f, info.Size())
	if err != nil {
		return Subido{}, err
	}
	if d.nombres != nil {
		d.nombres[fichero.Name] = true
	}
	if d.privado() {
		d.indexarCifrada(c, ruta, fichero)
	}
	return Subido{ID: fichero.ID, Nombre: fichero.Name}, nil
}

// indexarCifrada apunta la foto recién subida al Drive privado en el índice de la
// galería cifrada (fecha EXIF, medidas, miniatura cifrada), leyendo la copia local: el
// original no vuelve a bajar. Es lo mejor posible: si falla, la foto ya está a salvo y
// su entrada se reconstruye al leer la galería (aquí o en la web).
func (d *destinoDrive) indexarCifrada(ctx context.Context, ruta string, fichero api.File) {
	if !strings.HasPrefix(strings.ToLower(fichero.ContentType), "image/") || fichero.SizeBytes > render.MaxOriginal {
		return
	}
	datos, err := os.ReadFile(ruta) //nolint:gosec // la misma copia local que se acaba de subir
	if err != nil {
		return
	}
	defer clear(datos)
	if d.cifrada == nil {
		d.cifrada = galeriacifrada.Nueva(d.drive, nil)
	}
	_ = d.cifrada.Indexar(ctx, fichero, datos)
}

// Indexar apunta en la galería lo subido. Un 400 es que syphotos no sabe medirlo.
func (d *destinoDrive) Indexar(vida context.Context, id string) error {
	if d.fotos != nil && d.fotos.CanImportPrivatePhotos() {
		return nil
	}
	if d.privado() {
		// El id es del Drive privado: syphotos no puede leer el fichero.
		return errors.Join(ErrNoIndexable, ErrEnBoveda)
	}
	c, fin := ctx(vida)
	defer fin()
	_, err := d.fotos.Indexar(c, id)
	if e := (*api.Error)(nil); errors.As(err, &e) && e.Status == http.StatusBadRequest {
		return errors.Join(ErrNoIndexable, err)
	}
	return err
}

// PlazoSubida es lo que se espera a una subida de `bytes`: diez minutos como poco, y a
// razón de 100 KB/s, que es una conexión mala pero no rota.
func PlazoSubida(bytes int64) time.Duration {
	const porSegundo = 100 * 1024
	return max(10*time.Minute, time.Duration(bytes/porSegundo)*time.Second)
}

// GuardarEnGaleria deja `ruta` en la galería del sistema (el álbum «Sytue Photos») y la
// marca para que la copia no la vuelva a subir. Es a donde van las descargas en el
// móvil, como en cualquier app de fotos. Si la galería no sabe guardar,
// [carrete.ErrNoDisponible] y quien llama la deja en descargas.
func (a *App) GuardarEnGaleria(ctx context.Context, ruta, nombre string) error {
	if a.Galeria == nil || !a.Galeria.Disponible() {
		return carrete.ErrNoDisponible
	}
	id, err := a.Galeria.Guardar(ctx, ruta, nombre)
	if err != nil {
		return err
	}
	if a.Copia != nil {
		cuenta, _ := a.cuenta.Load().(string)
		a.Copia.Marcar(cuenta, id)
	}
	return nil
}

// DestinoGaleria es a qué galería del sistema lleva «Añadir a Fotos» / «Guardar en
// Imágenes» en el escritorio. En el móvil no hace falta: lo descargado ya va a la galería.
type DestinoGaleria uint8

// Los tres.
const (
	// SinDestinoGaleria: no se ofrece (el móvil, o sin galería del sistema).
	SinDestinoGaleria DestinoGaleria = iota
	// GaleriaFotos: la app Fotos de macOS (PhotoKit, álbum «Sytue Photos»).
	GaleriaFotos
	// GaleriaImagenes: Imágenes › Sytue Photos, la que enseñan la app Fotos de Windows y
	// las de Linux.
	GaleriaImagenes
)

// claves son las del mando y las del aviso de hecho.
func (d DestinoGaleria) claves() (mando, hecho string) {
	if d == GaleriaFotos {
		return "library.addToPhotos", "library.addedToPhotos"
	}
	return "library.saveToPictures", "library.savedToPictures"
}

// ofreceGaleria dice si este equipo ofrece guardar en su galería.
func (a *App) ofreceGaleria() bool {
	return a.DestinoGaleria != SinDestinoGaleria && a.Galeria != nil && a.Galeria.Disponible() && a.cli != nil
}

// guardarEnSistema exporta las fotos con su receta (como «Descargar» en JPEG) y las deja
// en la galería del sistema, una a una, y al acabar dice cuántas y cuáles no. Lo guardado
// queda marcado: la copia de seguridad no lo vuelve a subir.
func (a *App) guardarEnSistema(fotos []api.Photo) {
	if !a.ofreceGaleria() || len(fotos) == 0 {
		return
	}
	_, hecho := a.DestinoGaleria.claves()
	a.enSesion(func(vida context.Context) {
		var fallidas []string
		for _, f := range fotos {
			if vida.Err() != nil {
				return
			}
			if err := a.exportarAGaleria(vida, f); err != nil {
				fallidas = append(fallidas, f.Name)
			}
		}
		hechas := len(fotos) - len(fallidas)
		fyne.Do(func() {
			if len(fallidas) > 0 {
				a.avisar(ui.AvisoPeligro, a.v.T.T("library.saveFailed", i18n.Vars{"name": strings.Join(fallidas, ", ")}), "")
			}
			if hechas > 0 {
				a.avisar(ui.AvisoExito, a.v.T.T(hecho, i18n.Vars{"count": hechas}), "")
			}
		})
	})
}

// exportarAGaleria trae una exportación JPEG y la guarda en la galería del sistema.
// Corre FUERA del hilo de la interfaz.
func (a *App) exportarAGaleria(vida context.Context, f api.Photo) error {
	c, fin := ctx(vida)
	defer fin()
	r, err := a.cli.Export(c, f.ID, "jpeg")
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	tmp, err := os.CreateTemp("", "sytue-galeria-*.jpg")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_, err = io.Copy(tmp, r)
	if cerrar := tmp.Close(); err == nil {
		err = cerrar
	}
	if err != nil {
		return err
	}
	return a.GuardarEnGaleria(c, tmp.Name(), nombreDescarga(f.Name, "jpeg"))
}

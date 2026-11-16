// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/carrete"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// driveFalso es «/Fotos» de Drive y el índice de syphotos: lo que subió, por nombre, y
// cuántas veces se indexó. Lo demás lo contesta el syphotos falso de app_test.go. Como el
// Drive de verdad, con `on_conflict=keep_both` guarda «nombre (2).ext»; sin él,
// SUSTITUIRÍA, y eso aquí es un fallo de la prueba.
type driveFalso struct {
	mu          sync.Mutex
	t           *testing.T
	resto       http.Handler
	subidos     map[string]string
	indexados   int
	yaHabia     map[string]string // nombre → checksum de lo que estaba antes
	noIndexable bool              // syphotos contesta 400: no sabe medirlo
}

func (d *driveFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	escribir := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case r.URL.Path == "/api/v1/folders/root":
		escribir(api.Folder{ID: "raiz", IsRoot: true})
	case r.URL.Path == "/api/v1/folders/raiz/children":
		escribir(api.Children{Folders: []api.Folder{{ID: "fotos", Name: CarpetaFotos}}})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/folders/fotos/children":
		var c api.Children
		for n, s := range d.yaHabia {
			c.Files = append(c.Files, api.File{ID: n, Name: n, Checksum: s})
		}
		escribir(c)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/folders/fotos/files":
		datos, _ := io.ReadAll(r.Body)
		nombre := r.URL.Query().Get("name")
		_, ya := d.subidos[nombre]
		_, habia := d.yaHabia[nombre]
		if ya || habia {
			if r.URL.Query().Get("on_conflict") != "keep_both" {
				d.t.Errorf("se ha sustituido %q en Drive", nombre)
			}
			ext := path.Ext(nombre)
			nombre = strings.TrimSuffix(nombre, ext) + " (2)" + ext
		}
		d.subidos[nombre] = string(datos)
		escribir(api.File{ID: "f-" + nombre, Name: nombre})
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/assets":
		if d.noIndexable {
			w.WriteHeader(http.StatusBadRequest)
			escribir(map[string]string{"code": "bad_request"})
			return
		}
		d.indexados++
		escribir(api.Photo{ID: "nueva"})
	default:
		d.mu.Unlock()
		d.resto.ServeHTTP(w, r)
		d.mu.Lock()
	}
}

// appConCopia monta la app con una galería de mentira y Drive.
func appConCopia(t *testing.T, g *galeriaDeMentira, d *driveFalso) *App {
	t.Helper()
	s := &servicioFalso{fotos: fotosDelServicio(3), porPagina: 100}
	d.resto, d.subidos, d.t = s, map[string]string{}, t
	srv := httptest.NewServer(d)
	t.Cleanup(srv.Close)
	w := test.NewWindow(nil)
	w.SetPadded(false)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(ventanaAncho, ventanaAlto))
	a := Nueva(api.NewPhotos(srv.URL, nil, nil), vistaDePrueba(t, NuevaFototeca()), w)
	a.Drive = api.NewDrive(srv.URL, nil, nil)
	a.lanzar = func(f func()) { f() }
	a.espera = 0
	a.Galeria, a.Copia = g, NuevaCopia(g, "")
	a.CambiarSesion("ana")
	w.SetContent(a.Objeto())
	t.Cleanup(func() {
		if a.cop.reloj != nil {
			a.cop.reloj.Stop()
		}
	})
	return a
}

// esperar deja que lleguen los `fyne.Do` hasta que se cumpla `listo` (en el hilo de la
// interfaz).
func esperar(t *testing.T, que string, listo func() bool) {
	t.Helper()
	for range 400 {
		var hecho bool
		fyne.DoAndWait(func() { hecho = listo() })
		if hecho {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(que)
}

// esperarCopia espera a que no haya ronda.
func esperarCopia(t *testing.T, a *App) {
	t.Helper()
	esperar(t, "la ronda de la copia no termina", func() bool { return a.cop.progreso == nil && !a.Copia.EnMarcha() })
}

// TestLaCopiaSoloCorreEncendida: con la galería del sistema, la barra ofrece la nube; la
// copia no sube nada hasta que se enciende, y encendida sube lo que falta con la fecha
// delante —convirtiendo a JPEG lo que la galería no sabe enseñar—, lo indexa y dice
// cuántas fotos nuevas hay. Cada cambio de ajustes se le pasa a quien programa la copia
// con la app cerrada.
func TestLaCopiaSoloCorreEncendida(t *testing.T) {
	g, d := nuevaGaleria(t), &driveFalso{yaHabia: map[string]string{"playa.jpg": suma("la playa")}}
	g.poner("1", "IMG_0001.JPG", "la playa", tomaCopia)
	g.poner("2", "IMG_0002.HEIC", "el monte", tomaCopia.Add(time.Minute))
	a := appConCopia(t, g, d)
	var programada []AjustesCopia
	a.AlCambiarCopia = func(aj AjustesCopia) { programada = append(programada, aj) }
	a.Arrancar()
	esperarCopia(t, a)
	if len(d.subidos) != 0 {
		t.Fatalf("la copia ha subido sin encenderla: %v", d.subidos)
	}
	if len(programada) != 1 || programada[0].Activa {
		t.Fatalf("al arrancar no se le ha dicho (apagada) a quien la programa: %+v", programada)
	}
	_, gal := a.datosGaleria()
	if gal.AlCopia == nil {
		t.Fatal("con galería del sistema, la barra no ofrece la copia de seguridad")
	}

	a.CambiarCopia(true)
	esperar(t, "con permiso, encender no la ha encendido", a.CopiaActiva)
	esperarCopia(t, a)
	if len(d.subidos) != 1 || d.subidos["2024-05-01 12.01.00 IMG_0002.jpg"] != "jpeg:el monte" || d.indexados != 1 {
		t.Fatalf("subidas %v, indexadas %d", d.subidos, d.indexados)
	}
	if got := ultimoAviso(a).Titulo; got != a.v.T.T("backup.done", i18n.Vars{"count": 1}) {
		t.Errorf("el aviso es %q", got)
	}
	if a.textoCopia() != a.v.Traduce("backup.upToDate") {
		t.Errorf("el diálogo dice %q", a.textoCopia())
	}
	if ultima := programada[len(programada)-1]; !ultima.Activa || ultima.ConDatos {
		t.Errorf("encendida, se ha programado %+v", ultima)
	}

	a.CambiarCopia(false)
	g.poner("3", "IMG_0003.JPG", "el río", tomaCopia.Add(2*time.Minute))
	a.AlVolver()
	esperarCopia(t, a)
	if len(d.subidos) != 1 {
		t.Errorf("apagada, la copia ha seguido subiendo: %v", d.subidos)
	}
	if ultima := programada[len(programada)-1]; ultima.Activa {
		t.Errorf("apagada, sigue programada: %+v", ultima)
	}
}

// TestLaCopiaEsperaALaWifiYLoDice: con datos móviles no sube nada y el diálogo lo dice;
// permitir los datos lanza la ronda.
func TestLaCopiaEsperaALaWifiYLoDice(t *testing.T) {
	g, d := nuevaGaleria(t), &driveFalso{}
	g.poner("1", "IMG_0001.JPG", "uno", tomaCopia)
	g.medida = true
	a := appConCopia(t, g, d)
	_ = a.Copia.CambiarAjustes(func(aj *AjustesCopia) { aj.Activa = true })
	a.Arrancar()
	esperarCopia(t, a)
	if len(d.subidos) != 0 || a.textoCopia() != a.v.Traduce("backup.waitingWifi") {
		t.Fatalf("con datos: subidas %v, el diálogo dice %q", d.subidos, a.textoCopia())
	}
	a.CambiarAjustesCopia(func(aj *AjustesCopia) { aj.ConDatos = true })
	esperarCopia(t, a)
	if len(d.subidos) != 1 {
		t.Fatalf("con los datos permitidos no ha subido: %v", d.subidos)
	}
}

// TestLaCopiaSinPermisoNoSeEnciende: si la persona dice que no al diálogo del sistema,
// la copia no se enciende y la app lo dice.
func TestLaCopiaSinPermisoNoSeEnciende(t *testing.T) {
	g, d := nuevaGaleria(t), &driveFalso{}
	g.permiso = carrete.Denegado
	a := appConCopia(t, g, d)
	a.Arrancar()
	a.CambiarCopia(true)
	esperar(t, "sin permiso, la app no dice nada", func() bool { return ultimoAviso(a).Titulo != "" })
	if a.CopiaActiva() || ultimoAviso(a).Titulo != a.v.Traduce("backup.noPermission") {
		t.Fatalf("activa=%v, aviso %q", a.CopiaActiva(), ultimoAviso(a).Titulo)
	}
}

// TestAnyadirFotosNoSustituyeNada: «Añadir fotos» con el nombre de una foto que ya está
// en «/Fotos» no la sustituye (`keep_both`): las dos se quedan. Y lo que la galería no
// sabe enseñar (un 400 del índice) se dice aparte: está en Drive, no es un fallo.
func TestAnyadirFotosNoSustituyeNada(t *testing.T) {
	g, d := nuevaGaleria(t), &driveFalso{yaHabia: map[string]string{"IMG_0001.jpg": suma("la vieja")}}
	a := appConCopia(t, g, d)
	a.Arrancar()
	ruta := filepath.Join(t.TempDir(), "IMG_0001.jpg")
	if err := os.WriteFile(ruta, []byte("la nueva"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.subir([]string{ruta})
	esperar(t, "la subida no termina", func() bool { return a.g.subida == nil && len(a.avisos.Avisos()) > 0 })
	if d.subidos["IMG_0001 (2).jpg"] != "la nueva" || d.indexados != 1 {
		t.Fatalf("subidas %v, indexadas %d", d.subidos, d.indexados)
	}
	d.mu.Lock()
	d.noIndexable = true
	d.mu.Unlock()
	a.subir([]string{ruta})
	quiero := a.v.T.T("gallery.uploadedNotShown", i18n.Vars{"name": "IMG_0001.jpg"})
	esperar(t, "no se dice que no sale en la galería", func() bool { return ultimoAviso(a).Titulo == quiero })
}

// TestSinGaleriaNoHayNube: sin galería del sistema (el navegador) no se ofrece la copia.
func TestSinGaleriaNoHayNube(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(3), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	if _, gal := a.datosGaleria(); gal.AlCopia != nil {
		t.Fatal("sin galería del sistema, la barra ofrece la copia")
	}
	if strings.Contains(strings.Join(s.vistas(), "\n"), "/folders") {
		t.Error("sin galería, la app ha ido a Drive")
	}
}

// TestGuardarEnLaGaleriaDelOrdenador: en el escritorio, la selección (y el visor) ofrecen
// «Guardar en Imágenes» / «Añadir a Fotos»; exporta cada foto en JPEG, la deja en la
// galería del sistema, la marca para que la copia no la vuelva a subir y lo dice. Sin
// destino (el móvil, donde lo descargado ya va a la galería), no se ofrece.
func TestGuardarEnLaGaleriaDelOrdenador(t *testing.T) {
	g, d := nuevaGaleria(t), &driveFalso{}
	a := appConCopia(t, g, d)
	a.Arrancar()
	esperar(t, "la galería no carga", func() bool { return len(a.g.fotos) > 0 })
	a.entrarEnSeleccion()
	a.alternar(a.g.fotos[0])
	if _, gal := a.datosGaleria(); gal.Lote.AlSistema != nil {
		t.Fatal("sin destino, la selección ofrece guardar en la galería")
	}

	a.DestinoGaleria = GaleriaImagenes
	_, gal := a.datosGaleria()
	if gal.Lote.AlSistema == nil || gal.Lote.ClaveSistema != "library.saveToPictures" {
		t.Fatalf("con destino Imágenes: %q", gal.Lote.ClaveSistema)
	}
	gal.Lote.AlSistema()
	quiero := a.v.T.T("library.savedToPictures", i18n.Vars{"count": 1})
	esperar(t, "no dice que se ha guardado", func() bool { return ultimoAviso(a).Titulo == quiero })
	if len(g.guardadas) != 1 || g.guardadas[0] != nombreDescarga(a.g.fotos[0].Name, "jpeg") {
		t.Fatalf("guardadas %q", g.guardadas)
	}
	// Lo guardado no lo vuelve a subir la copia.
	g.poner("propia-"+g.guardadas[0], g.guardadas[0], "exportada", tomaCopia)
	_ = a.Copia.CambiarAjustes(func(aj *AjustesCopia) { aj.Activa = true })
	a.rondaCopia()
	esperarCopia(t, a)
	if len(d.subidos) != 0 {
		t.Fatalf("la copia ha subido lo que se guardó desde Sytue: %v", d.subidos)
	}

	a.DestinoGaleria = GaleriaFotos
	if _, gal := a.datosGaleria(); gal.Lote.ClaveSistema != "library.addToPhotos" {
		t.Fatalf("en macOS el mando es «Añadir a Fotos», no %q", gal.Lote.ClaveSistema)
	}
}

// TestSubirFotosSeQuedaEnElAnyoYRefrescaLaBarra: tras saltar a 2019, subir fotos vuelve a
// pedir la línea desde ese año (subir no mueve la vista) y la barra de años, que las
// subidas pueden ser de un año que aún no tenía. Como la web.
func TestSubirFotosSeQuedaEnElAnyoYRefrescaLaBarra(t *testing.T) {
	g, d := nuevaGaleria(t), &driveFalso{}
	a := appConCopia(t, g, d)
	s := d.resto.(*servicioFalso)
	s.mu.Lock()
	s.anyos = []api.TimelineYear{{Year: 2024, Count: 2}, {Year: 2019, Count: 1}}
	s.mu.Unlock()
	a.Arrancar()
	a.saltarAAnyo(2019)
	antes := contar(s, "GET /api/v1/timeline/years")
	ruta := filepath.Join(t.TempDir(), "IMG_0002.jpg")
	if err := os.WriteFile(ruta, []byte("nueva"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.subir([]string{ruta})
	esperar(t, "la subida no termina", func() bool { return a.g.subida == nil && len(a.avisos.Avisos()) > 0 })
	if a.g.anyo != 2019 {
		t.Errorf("tras subir, la galería ha vuelto a %d", a.g.anyo)
	}
	if n := contar(s, "GET /api/v1/timeline?desde=2019"); n != 2 {
		t.Errorf("tras subir, la línea no se pide desde 2019 (%d veces): %v", n, s.vistas())
	}
	if contar(s, "GET /api/v1/timeline/years") != antes+1 {
		t.Errorf("tras subir, la barra de años no se refresca: %v", s.vistas())
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// ventanaQueSuelta recuerda a quién llamar al soltar: la ventana de pruebas de Fyne se
// traga `SetOnDropped`, y sin esto soltar un fichero no se puede ver.
type ventanaQueSuelta struct {
	fyne.Window
	soltar func(fyne.Position, []fyne.URI)
}

func (v *ventanaQueSuelta) SetOnDropped(f func(fyne.Position, []fyne.URI)) { v.soltar = f }

// fichero escribe uno en una carpeta de la prueba y devuelve su ruta.
func fichero(t *testing.T, nombre, contenido string) string {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), nombre)
	if err := os.WriteFile(ruta, []byte(contenido), 0o600); err != nil {
		t.Fatal(err)
	}
	return ruta
}

// TestElFicheroDeLaMaquinaEntraPorElMismoSitio: un `.docx` del escritorio, soltado en la
// ventana, llega al MISMO `cargar` que abre lo de Drive, con su URI y sin referencia de
// Drive; y lo que ese `cargar` lee es el fichero, entero. Lo que no es de este producto
// —una imagen soltada en Docus— no llega.
func TestElFicheroDeLaMaquinaEntraPorElMismoSitio(t *testing.T) {
	aparte(t)
	_ = ajustes.Cambiar(func(a *ajustes.Ajustes) { a.SubirAlAbrir = true })
	a := test.NewApp()
	defer a.Quit()
	s := Abrir(a, "sytue.eu")
	espia := nuevaEspia(a)
	s.app = espia

	w := &ventanaQueSuelta{Window: a.NewWindow("docus")}
	s.aceptarFicheros(w, func() { t.Error("no ha fallado nada y se ha avisado de un fallo") })
	vistas := make(chan api.Apertura, 2)
	s.AlAbrirFichero("docs", func(_ context.Context, ap api.Apertura) error { vistas <- ap; return nil })

	acta := fichero(t, "acta.docx", "PK\x03\x04 el contenido")
	foto := fichero(t, "foto.png", "\x89PNG")
	w.soltar(fyne.NewPos(10, 10), []fyne.URI{storage.NewFileURI(foto), storage.NewFileURI(acta)})

	visto := esperar(t, vistas)
	if visto.Ref != (api.Referencia{}) || !strings.HasSuffix(visto.Local, "/acta.docx") {
		t.Fatalf("apertura = %+v", visto)
	}
	nombre, datos, err := leerLocal(visto.Local)
	if err != nil || nombre != "acta.docx" || string(datos) != "PK\x03\x04 el contenido" {
		t.Errorf("leído %q = %q, %v", nombre, datos, err)
	}
	if otro, ok := recibir(vistas, 200*time.Millisecond); ok {
		t.Errorf("la imagen no es de Docus y ha llegado: %+v", otro)
	}
	espia.nadaEnLaWeb(t)
}

// TestArrancarConElFicheroLoAbre: «Abrir con» con la app cerrada. El fichero llega antes
// de que el `main` se apunte, y los `main` llaman a AlAbrirFichero ANTES que a
// AceptarFicheros: se perdía, y la app abría la portada. Visto en rojo quitando la rama
// de `localesAntes` en abrirLocal.
func TestArrancarConElFicheroLoAbre(t *testing.T) {
	aparte(t)
	_ = ajustes.Cambiar(func(a *ajustes.Ajustes) { a.SubirAlAbrir = true })
	a := test.NewApp()
	defer a.Quit()
	s := Abrir(a, "sytue.eu")
	espia := nuevaEspia(a)
	s.app = espia

	u, _ := url.Parse(storage.NewFileURI(fichero(t, "cuentas.xlsx", "PK")).String())
	s.recibirURL(u) // lo que entrega el sistema al arrancar
	vistas := make(chan api.Apertura, 1)
	s.AlAbrirFichero("sheets", func(_ context.Context, ap api.Apertura) error { vistas <- ap; return nil })
	s.aceptarFicheros(a.NewWindow("calcus"), func() { t.Error("no ha fallado nada y se ha avisado de un fallo") })
	if visto := esperar(t, vistas); !strings.HasSuffix(visto.Local, "/cuentas.xlsx") {
		t.Fatalf("apertura = %+v", visto)
	}
	espia.nadaEnLaWeb(t)
}

// TestSinAceptarFicherosNoSeAbreNingunoSuelto: SyPhotos y CertiPDF registran su
// `cargar`, pero el suyo abre por identificador de Drive. Un fichero suelto que les
// llegara acabaría en un error: sin la llamada explícita, se ignora.
func TestSinAceptarFicherosNoSeAbreNingunoSuelto(t *testing.T) {
	aparte(t)
	_ = ajustes.Cambiar(func(a *ajustes.Ajustes) { a.SubirAlAbrir = true })
	a := test.NewApp()
	defer a.Quit()
	s := Abrir(a, "sytue.eu")
	espia := nuevaEspia(a)
	s.app = espia

	llamado := make(chan struct{}, 1)
	s.AlAbrirFichero("docs", func(context.Context, api.Apertura) error { llamado <- struct{}{}; return nil })
	u, _ := url.Parse(storage.NewFileURI(fichero(t, "acta.docx", "x")).String())
	s.recibirURL(u)
	if _, ok := recibir(llamado, 200*time.Millisecond); ok {
		t.Error("una app que no acepta ficheros sueltos ha intentado abrir uno")
	}
	espia.nadaEnLaWeb(t)
}

// TestUnFicheroQueNoSeAbreSeDiceYNoVaALaWeb: el navegador no ve el disco de nadie, así
// que el respaldo de Drive —la misma página en la web— aquí sería una página vacía. Se
// avisa a quien lo pidió y no se abre nada.
func TestUnFicheroQueNoSeAbreSeDiceYNoVaALaWeb(t *testing.T) {
	aparte(t)
	_ = ajustes.Cambiar(func(a *ajustes.Ajustes) { a.SubirAlAbrir = true })
	a := test.NewApp()
	defer a.Quit()
	s := Abrir(a, "sytue.eu")
	espia := nuevaEspia(a)
	s.app = espia

	fallos := make(chan struct{}, 1)
	s.aceptarFicheros(a.NewWindow("calcus"), func() { fallos <- struct{}{} })
	s.AlAbrirFichero("sheets", func(context.Context, api.Apertura) error { return errors.New("401: sin sesión") })
	u, _ := url.Parse(storage.NewFileURI(fichero(t, "cuentas.xlsx", "x")).String())
	s.recibirURL(u)
	esperar(t, fallos)
	espia.nadaEnLaWeb(t)
}

// TestLosArgumentosQueSonFicheros: lo que Linux y Windows pasan al abrir un documento, y
// lo que no hay que confundir con él.
func TestLosArgumentosQueSonFicheros(t *testing.T) {
	acta := fichero(t, "acta.odt", "x")
	if f, ok := ficheroDeArg(acta); !ok || f != storage.NewFileURI(acta).String() {
		t.Errorf("una ruta que existe = %q, %v", f, ok)
	}
	if f, ok := ficheroDeArg("file:///home/ada/acta.odt"); !ok || f != "file:///home/ada/acta.odt" {
		t.Errorf("un file:// = %q, %v", f, ok)
	}
	for _, no := range []string{"--depurar", filepath.Join(t.TempDir(), "no-existe.odt"), t.TempDir(), "sytue-docs:/open-from-drive?file=a", "https://sytue.eu"} {
		if f, ok := ficheroDeArg(no); ok {
			t.Errorf("%q no es un fichero que abrir y ha salido %q", no, f)
		}
	}
	// Y lo que la segunda instancia reenvía a la que ya corre lleva el fichero como URL.
	if got := urlsDeArgs([]string{"--depurar", acta, "sytue-docs:/x"}); len(got) != 2 || !strings.HasPrefix(got[0], "file://") {
		t.Errorf("urlsDeArgs = %q", got)
	}

	// Lo que el fork entrega de Windows: la unidad como esquema y el resto opaco.
	u, err := url.Parse(`C:\Docs\acta.docx`)
	if err != nil {
		t.Fatal(err)
	}
	if uri, ok := ficheroLocal(u); !ok || uri.Scheme() != "file" {
		t.Errorf("una ruta de Windows = %v, %v", uri, ok)
	}
	for _, no := range []string{"sytue-docs:/open-from-drive?file=a", "https://sytue.eu/x.docx", "sytue-docs://auth/callback?code=x"} {
		u, _ := url.Parse(no)
		if _, ok := ficheroLocal(u); ok {
			t.Errorf("%q no es un fichero de la máquina", no)
		}
	}
}

// TestAbrirLocalPreguntaAntesDeSubirYRecuerdaEnElDispositivo (FYP-02): un fichero del
// equipo abierto desde el sistema («Abrir con», argumento o arrastrar a la ventana) no
// se sube al servicio sin que la persona dé su conformidad antes, y puede elegir recordar
// esa decisión en el dispositivo.
func TestAbrirLocalPreguntaAntesDeSubirYRecuerdaEnElDispositivo(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()
	s := Abrir(a, "sytue.eu")
	espia := nuevaEspia(a)
	s.app = espia

	w := &ventanaQueSuelta{Window: a.NewWindow("docus")}
	s.aceptarFicheros(w, func() { t.Error("no ha fallado nada") })
	vistas := make(chan api.Apertura, 4)
	s.AlAbrirFichero("docs", func(_ context.Context, ap api.Apertura) error { vistas <- ap; return nil })

	// 1. Sin diálogo de conformidad ni preferencia recordada, no se sube el fichero.
	acta := fichero(t, "acta.docx", "PK\x03\x04 contenido")
	w.soltar(fyne.NewPos(10, 10), []fyne.URI{storage.NewFileURI(acta)})
	if ap, ok := recibir(vistas, 200*time.Millisecond); ok {
		t.Fatalf("sin conformidad se ha subido el fichero local: %+v", ap)
	}

	// 2. Si la persona cancela la pregunta, tampoco se sube.
	preguntas := make(chan string, 4)
	s.AlConfirmarSubidaLocal(func(_ fyne.Window, nombre string, _ func(bool)) {
		preguntas <- nombre
	})
	w.soltar(fyne.NewPos(10, 10), []fyne.URI{storage.NewFileURI(acta)})
	if got := esperar(t, preguntas); got != "acta.docx" {
		t.Fatalf("nombre preguntado = %q, quiero acta.docx", got)
	}
	if ap, ok := recibir(vistas, 200*time.Millisecond); ok {
		t.Fatalf("tras cancelar se ha subido el fichero local: %+v", ap)
	}

	// 3. Si acepta sin recordar, se sube esta vez y la siguiente vuelve a preguntar.
	s.AlConfirmarSubidaLocal(func(_ fyne.Window, nombre string, aceptar func(bool)) {
		preguntas <- nombre
		aceptar(false)
	})
	w.soltar(fyne.NewPos(10, 10), []fyne.URI{storage.NewFileURI(acta)})
	esperar(t, preguntas)
	if visto := esperar(t, vistas); !strings.HasSuffix(visto.Local, "/acta.docx") {
		t.Fatalf("apertura tras aceptar = %+v", visto)
	}
	if ajustes.Leer().SubirAlAbrir {
		t.Fatal("aceptar(false) no debía recordar la elección en el dispositivo")
	}

	// 4. Si acepta recordando en este dispositivo, queda guardado y otra app de la suite
	// en el mismo dispositivo ya no vuelve a preguntar.
	s.AlConfirmarSubidaLocal(func(_ fyne.Window, nombre string, aceptar func(bool)) {
		preguntas <- nombre
		aceptar(true)
	})
	w.soltar(fyne.NewPos(10, 10), []fyne.URI{storage.NewFileURI(acta)})
	esperar(t, preguntas)
	esperar(t, vistas)
	if !ajustes.Leer().SubirAlAbrir {
		t.Fatal("aceptar(true) debía recordar SubirAlAbrir en los ajustes del dispositivo")
	}

	otra := Abrir(a, "sytue.eu")
	otra.app = espia
	wOtra := &ventanaQueSuelta{Window: a.NewWindow("calcus")}
	otra.aceptarFicheros(wOtra, func() { t.Error("no ha fallado nada") })
	otra.AlConfirmarSubidaLocal(func(_ fyne.Window, nombre string, _ func(bool)) {
		t.Errorf("con la elección recordada en el dispositivo no debía volver a preguntar (%q)", nombre)
	})
	otra.AlAbrirFichero("sheets", func(_ context.Context, ap api.Apertura) error { vistas <- ap; return nil })
	hoja := fichero(t, "cuentas.xlsx", "PK")
	wOtra.soltar(fyne.NewPos(10, 10), []fyne.URI{storage.NewFileURI(hoja)})
	if visto := esperar(t, vistas); !strings.HasSuffix(visto.Local, "/cuentas.xlsx") {
		t.Fatalf("apertura con elección recordada = %+v", visto)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package indice_test

import (
	"bytes"
	"database/sql"
	"image"
	"image/png"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/bases"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/correo"
	"git.sytue.eu/sytue/one/apps/nucleo/local/indice"
	"git.sytue.eu/sytue/one/apps/nucleo/notes"
	"git.sytue.eu/sytue/one/apps/nucleo/photos"
)

type secretosEnMemoria struct{}

func (secretosEnMemoria) Leer(string) (string, error)  { return "", nil }
func (secretosEnMemoria) Guardar(string, string) error { return nil }
func (secretosEnMemoria) Borrar(string) error          { return nil }

// hoyDePrueba es una carpeta de datos con lo de un día: eventos (uno que se repite cada
// día desde hace una semana, uno de todo el día, uno de mañana), tareas y recordatorios,
// correo sin leer, documentos con contenido, fotos en un álbum y una base. Las apps se
// quedan ABIERTAS mientras el Hub lee.
func hoyDePrueba(t *testing.T, ahora time.Time) string {
	t.Helper()
	datos := t.TempDir()
	ctx := t.Context()
	abrir := func(producto string) *almacen.Recursos {
		rec, err := almacen.Abrir(ctx, filepath.Join(datos, "Datos", producto), producto, "instalacion")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(rec.Cerrar)
		return rec
	}
	falla := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	k, err := calendar.New(abrir("calendar").Config)
	falla(err)
	falla(almacen.Preparar(ctx, k))
	cal, err := k.CreateCalendar(ctx, api.CalendarInput{Name: "casa", DisplayName: "Casa"})
	falla(err)
	diez := []int{10}
	hoy9 := time.Date(ahora.Year(), ahora.Month(), ahora.Day(), 9, 0, 0, 0, ahora.Location())
	_, err = k.CreateEvent(ctx, cal.ID, api.EventInput{Summary: "Pasear al perro", Start: hoy9.AddDate(0, 0, -7), End: hoy9.AddDate(0, 0, -7).Add(30 * time.Minute), RRule: "FREQ=DAILY", Reminders: &diez})
	falla(err)
	_, err = k.CreateEvent(ctx, cal.ID, api.EventInput{
		Summary: "Cumpleaños de Ana", AllDay: true,
		Start: time.Date(ahora.Year(), ahora.Month(), ahora.Day(), 0, 0, 0, 0, time.UTC), End: time.Date(ahora.Year(), ahora.Month(), ahora.Day()+1, 0, 0, 0, 0, time.UTC),
	})
	falla(err)
	_, err = k.CreateEvent(ctx, cal.ID, api.EventInput{Summary: "Dentista", Location: "Calle Mayor 3", Start: hoy9.AddDate(0, 0, 1), End: hoy9.AddDate(0, 0, 1).Add(time.Hour)})
	falla(err)

	n, err := notes.New(abrir("notes").Config)
	falla(err)
	t.Cleanup(n.Close)
	falla(almacen.Preparar(ctx, n))
	libro, err := n.CreateNotebook(ctx, api.NotebookInput{Title: "Casa"})
	falla(err)
	pan, err := n.CreateNote(ctx, libro.ID, api.NoteInput{Title: "Comprar pan", Kind: "todo"})
	falla(err)
	_, err = n.CreateNote(ctx, libro.ID, api.NoteInput{Title: "Regar las plantas", Kind: "todo"})
	falla(err)
	si := true
	_, err = n.CreateNote(ctx, libro.ID, api.NoteInput{Title: "Ya hecha", Kind: "todo", Checked: &si})
	falla(err)
	llamar, err := n.CreateNote(ctx, libro.ID, api.NoteInput{Title: "Llamar al banco", Kind: "text"})
	falla(err)
	_, err = n.SetReminder(ctx, pan.ID, api.NoteReminderInput{DueAt: hoy9.Add(8 * time.Hour), TZID: "UTC"})
	falla(err)
	_, err = n.SetReminder(ctx, llamar.ID, api.NoteReminderInput{DueAt: hoy9.Add(2 * time.Hour), TZID: "UTC"})
	falla(err)

	// Mail abierto: su índice lo crea él; los sobres se escriben como los dejaría su
	// sincronizador.
	buzon, err := correo.Abrir(filepath.Join(datos, "Correo"), secretosEnMemoria{}, correo.Red{})
	falla(err)
	t.Cleanup(buzon.Cerrar)
	db, err := sql.Open("sqlite3", filepath.Join(datos, "Correo", "indice.sqlite")+"?_busy_timeout=5000")
	falla(err)
	defer db.Close()
	_, err = db.Exec(`INSERT INTO carpetas (cuenta, nombre, rol) VALUES ('c1', 'INBOX', 'inbox'), ('c1', 'Papelera', 'trash')`)
	falla(err)
	for _, m := range []struct {
		carpeta, asunto, de, marcas, buscable string
		hace                                  time.Duration
	}{
		{"INBOX", "Factura de la luz", "Iberdrola", `[]`, "factura de la luz iberdrola", time.Hour},
		{"INBOX", "Cena del viernes", "Íñigo", `[]`, "cena del viernes inigo reforma", 2 * time.Hour},
		{"INBOX", "Leído ya", "Ana", `["\\Seen"]`, "leido ya ana", 3 * time.Hour},
		{"Papelera", "Reforma tirada", "Spam", `[]`, "reforma tirada spam", 4 * time.Hour},
	} {
		_, err = db.Exec(`INSERT INTO mensajes (cuenta, carpeta, hilo, asunto, de_nombre, marcas, buscable, fecha) VALUES ('c1', ?, ?, ?, ?, ?, ?, ?)`,
			m.carpeta, "hilo-"+m.asunto, m.asunto, m.de, m.marcas, m.buscable, ahora.Add(-m.hace).Unix())
		falla(err)
	}

	// Documentos con contenido: el texto de un documento y las celdas de una hoja.
	bib, err := biblioteca.Abrir(filepath.Join(datos, "Documentos"), "docs", biblioteca.Persona{ID: "yo"})
	falla(err)
	d, _, err := bib.CreateDocumentFromSource(ctx, api.DocText, "Acta", "")
	falla(err)
	_, err = bib.SaveDocumentContent(ctx, d.ID, api.DocContentKey(api.DocText), "<p>Se aprueba la <b>derrama</b> del tejado</p>", api.DocDot{}, 1, false, 0)
	falla(err)
	hojas, err := biblioteca.Abrir(filepath.Join(datos, "Documentos"), "sheets", biblioteca.Persona{ID: "yo"})
	falla(err)
	imp, err := biblioteca.Importar("gastos.csv", []byte("Concepto,Importe\nFontanero,120\nPintura,=B2*2\n"), false)
	falla(err)
	h, _, err := hojas.CreateDocumentFromSource(ctx, api.DocSheet, "Gastos", "")
	falla(err)
	_, err = hojas.SaveDocumentContent(ctx, h.ID, api.DocContentKey(api.DocSheet), imp.Content, api.DocDot{}, 1, false, 0)
	falla(err)

	// Una foto en el álbum «Bilbao 2024».
	p, err := photos.New(abrir("photos").Config)
	falla(err)
	falla(almacen.Preparar(ctx, p))
	cli, err := api.NewPhotos("https://sin-red.invalid", nil, nil).WithPrivateData(p)
	falla(err)
	var buf bytes.Buffer
	falla(png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 64, 48))))
	foto, err := cli.ImportarFoto(ctx, "0b8a9f8e-1c2d-4e5f-8a9b-0c1d2e3f4a5b", "IMG_0001.png", "image/png", bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	falla(err)
	album, err := cli.CreateAlbum(ctx, "Bilbao 2024")
	falla(err)
	_, err = cli.AddToAlbum(ctx, album.ID, []string{foto.ID})
	falla(err)

	// Una base con un registro.
	b, err := bases.New(abrir("bases").Config)
	falla(err)
	falla(almacen.Preparar(ctx, b))
	base, err := b.CreateBase(ctx, "Clientes")
	falla(err)
	tabla, err := b.CreateTable(ctx, base.ID, "Personas")
	falla(err)
	campo, err := b.CreateField(ctx, tabla.ID, api.BasesNewField{Name: "Nombre", Kind: "text"})
	falla(err)
	ciudad, err := b.CreateField(ctx, tabla.ID, api.BasesNewField{Name: "Ciudad", Kind: "text"})
	falla(err)
	_, err = b.CreateRecord(ctx, tabla.ID, map[string]any{campo.ID: "Ana Ruiz", ciudad.ID: "Zamora"}, nil)
	falla(err)
	return datos
}

// HU-04: lo de hoy sale de los almacenes de Calendar y Notepads y del índice de Mail,
// con las repeticiones expandidas, sin lo hecho ni lo leído ni lo tirado.
func TestHoyJuntaAgendaTareasRecordatoriosYCorreo(t *testing.T) {
	ahora := time.Date(2026, 11, 24, 7, 30, 0, 0, time.UTC)
	h := indice.LeerHoy(hoyDePrueba(t, ahora), ahora)
	if len(h.Fallos) != 0 {
		t.Fatal("fuentes que fallan:", h.Fallos)
	}
	eventos := make([]string, 0, len(h.Eventos))
	for _, e := range h.Eventos {
		eventos = append(eventos, e.Titulo)
	}
	if !slices.Equal(eventos, []string{"Cumpleaños de Ana", "Pasear al perro"}) {
		t.Fatalf("eventos de hoy = %q", eventos)
	}
	if !h.Eventos[0].TodoElDia || h.Eventos[1].Fecha.Hour() != 9 || h.Eventos[1].Fecha.Day() != 24 {
		t.Errorf("eventos = %+v", h.Eventos)
	}
	if got := titulos(h.Tareas); !slices.Equal(got, []string{"Comprar pan", "Regar las plantas"}) || h.TotalTareas != 2 {
		t.Errorf("tareas = %q (%d)", got, h.TotalTareas)
	}
	if got := titulos(h.Recordatorios); !slices.Equal(got, []string{"Llamar al banco", "Comprar pan"}) {
		t.Errorf("recordatorios = %q", got)
	}
	if h.SinLeer != 2 || !slices.Equal(titulos(h.Correos), []string{"Factura de la luz", "Cena del viernes"}) {
		t.Errorf("correo sin leer = %d %q", h.SinLeer, titulos(h.Correos))
	}
	if h.Vacio() {
		t.Error("con cosas, no está vacío")
	}
	// En otra zona horaria el cumpleaños sigue siendo del 24, no del 23.
	ny, _ := time.LoadLocation("America/New_York")
	tarde := time.Date(2026, 11, 23, 20, 0, 0, 0, ny)
	for _, e := range indice.LeerHoy(hoyDePrueba(t, ahora), tarde).Eventos {
		if e.Titulo == "Cumpleaños de Ana" {
			t.Error("el de todo el día del 24 sale el 23 en Nueva York")
		}
	}
}

// HU-06: los recordatorios que el Hub avisa con las apps cerradas son los mismos que
// programan ellas: cada repetición del evento con su VALARM, y el de cada nota.
func TestLosAvisosSonLosDeCalendarYNotepads(t *testing.T) {
	ahora := time.Date(2026, 11, 24, 7, 30, 0, 0, time.UTC)
	por, fallos := indice.Avisos(hoyDePrueba(t, ahora), ahora)
	if len(fallos) != 0 {
		t.Fatal(fallos)
	}
	cal := por["calendar"]
	if len(cal) < 30 {
		t.Fatalf("una serie diaria da un aviso por día: %d", len(cal))
	}
	primero := cal[0]
	if primero.Titulo != "Pasear al perro" || !primero.Cuando.Equal(time.Date(2026, 11, 24, 8, 50, 0, 0, time.UTC)) ||
		!strings.HasPrefix(primero.Enlace, "sytueoffice-calendar:/abrir?") || !strings.HasSuffix(primero.Clave, "/10") {
		t.Errorf("primer aviso de Calendar = %+v", primero)
	}
	notas := make([]string, 0, len(por["notes"]))
	for _, a := range por["notes"] {
		notas = append(notas, a.Titulo)
	}
	if !slices.Equal(notas, []string{"Llamar al banco", "Comprar pan"}) {
		t.Errorf("avisos de Notepads = %q", notas)
	}
}

// La búsqueda entra en el contenido de los documentos, en el correo (sin lo tirado), en
// las fotos (por su álbum, su fecha) y en las bases (nombres y celdas de texto).
func TestBuscaEnContenidoCorreoFotosYBases(t *testing.T) {
	ahora := time.Now()
	s := indice.Leer(hoyDePrueba(t, ahora))
	if len(s.Fallos) != 0 {
		t.Fatal("fuentes que fallan:", s.Fallos)
	}
	for consulta, quiero := range map[string]string{
		"derrama":       "Acta",
		"fontanero":     "Gastos",
		"cena viernes":  "Cena del viernes",
		"bilbao":        "Bilbao 2024",
		"zamora":        "Ana Ruiz",
		"clientes":      "Clientes",
		"personas":      "Personas",
		"img_0001":      "IMG_0001.png",
		"iberdrola luz": "Factura de la luz",
	} {
		got := s.Buscar(consulta, 10)
		if !slices.Contains(titulos(got), quiero) {
			t.Errorf("«%s» = %q, falta %q", consulta, titulos(got), quiero)
		}
	}
	// Bases abre una tabla por `<base>/<tabla>` y un registro por `<base>/<tabla>/<registro>`.
	for consulta, clase := range map[string]string{"personas": indice.Tabla, "zamora": indice.Registro} {
		partes := 2
		if clase == indice.Registro {
			partes = 3
		}
		got := s.Buscar(consulta, 10)
		i := slices.IndexFunc(got, func(e indice.Elemento) bool { return e.Clase == clase })
		if i < 0 || len(strings.Split(got[i].ID, "/")) != partes || strings.Contains(got[i].ID, "//") {
			t.Errorf("«%s»: el enlace a Bases no lleva base y tabla: %+v", consulta, got)
		}
	}
	if got := s.Buscar("tirada", 10); len(got) != 0 {
		t.Errorf("encuentra el correo de la papelera: %q", titulos(got))
	}
	if got := s.Buscar("b2", 10); len(got) != 0 {
		t.Errorf("busca en las fórmulas de una hoja: %q", titulos(got))
	}
	// La foto, por su fecha (sin hora de toma, la de cuando entró), con el nombre del mes.
	meses := []string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}
	got := s.Buscar(meses[ahora.Month()-1]+" "+ahora.Format("2006"), 10)
	if !slices.ContainsFunc(got, func(e indice.Elemento) bool { return e.Clase == indice.Foto }) {
		t.Errorf("la foto no sale por su fecha: %+v", got)
	}
	if c := s.Buscar("cena", 1); len(c) != 1 || c[0].Producto != "mail" || c[0].Clase != indice.Hilo || c[0].ID != "hilo-Cena del viernes" ||
		c[0].Enlace() != "sytueoffice-mail:/abrir?clase=hilo&id=hilo-Cena+del+viernes" {
		t.Errorf("correo = %+v", c)
	}
}

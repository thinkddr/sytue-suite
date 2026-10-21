// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package nube

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type doble struct {
	paginas map[string]api.Children
	hechos  *[]string
}

func (d doble) apuntar(s string) error {
	if d.hechos != nil {
		*d.hechos = append(*d.hechos, s)
	}
	return nil
}

func (d doble) File(_ context.Context, id string) (api.File, error) {
	return api.File{ID: id, FolderID: "c1", Name: "enero.pdf", ContentType: "application/pdf"}, nil
}

func (d doble) Upload(_ context.Context, carpeta, nombre, tipo string, cuerpo io.Reader, _ int64) (api.File, error) {
	b, _ := io.ReadAll(cuerpo)
	return api.File{ID: "f9"}, d.apuntar("subir " + carpeta + "/" + nombre + " " + tipo + " " + string(b))
}

func (d doble) CreateFolder(_ context.Context, padre, nombre string) (api.Folder, error) {
	if nombre == "Fotos" {
		return api.Folder{}, &api.Error{Status: http.StatusConflict}
	}
	return api.Folder{ID: "c9"}, d.apuntar("carpeta " + padre + "/" + nombre)
}

func (d doble) Rename(_ context.Context, k api.DriveKind, id, nombre string) error {
	return d.apuntar("renombrar " + string(k) + " " + id + " " + nombre)
}

func (d doble) Move(_ context.Context, k api.DriveKind, id, destino string) error {
	return d.apuntar("mover " + string(k) + " " + id + " " + destino)
}

func (d doble) Delete(_ context.Context, k api.DriveKind, id string) error {
	switch id {
	case "ya-no":
		return &api.Error{Status: http.StatusNotFound}
	case "prohibido":
		return &api.Error{Status: http.StatusForbidden}
	}
	return d.apuntar("borrar " + string(k) + " " + id)
}

func (d doble) Root(context.Context) (api.Folder, error) {
	return api.Folder{ID: "r", IsRoot: true}, nil
}

func (d doble) Children(_ context.Context, carpeta, cursor string) (api.Children, error) {
	h, ok := d.paginas[carpeta+"|"+cursor]
	if !ok {
		return api.Children{}, errors.New("no existe")
	}
	return h, nil
}

func (d doble) Download(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

// TestLosMarcadoresSonLosDeLaCarpetaEntera: la raíz es «Mi unidad», se siguen todas las
// páginas, cada marcador lleva su identidad con su clase, y los nombres que Windows no
// admite se quedan fuera en vez de tumbar la carpeta.
func TestLosMarcadoresSonLosDeLaCarpetaEntera(t *testing.T) {
	d := doble{paginas: map[string]api.Children{
		"r|": {Folders: []api.Folder{{ID: "c1", Name: "Facturas"}, {ID: "c2", Name: "a:b"}}, NextCursor: "2"},
		"r|2": {Files: []api.File{
			{ID: "f1", Name: "informe.pdf", SizeBytes: 7}, {ID: "f2", Name: "fin. "}, {ID: "f3", Name: "con?"},
		}},
		"c1|": {Files: []api.File{{ID: "f4", Name: "enero.pdf"}}},
	}}
	nombres := func(es []Entrada) (out []string) {
		for _, e := range es {
			out = append(out, base(e.Identidad)+"="+e.Nombre)
		}
		return out
	}
	es, _, err := listar(context.Background(), d, raiz)
	if err != nil {
		t.Fatal(err)
	}
	if got := nombres(es); !slices.Equal(got, []string{"c:c1=Facturas", "f:f1=informe.pdf"}) {
		t.Errorf("la raíz da %q", got)
	}
	if !es[0].Carpeta || es[1].Carpeta || es[1].Tamano != 7 {
		t.Errorf("clase o tamaño mal: %+v", es)
	}
	if es, _, _ := listar(context.Background(), d, "c:c1"); !slices.Equal(nombres(es), []string{"f:f4=enero.pdf"}) {
		t.Errorf("una subcarpeta da %q", nombres(es))
	}
	for _, mal := range []string{"f:f1", "x:1", ""} {
		if _, _, err := listar(context.Background(), d, mal); err == nil {
			t.Errorf("listar(%q) no falla", mal)
		}
	}
	if id, err := ficheroDe("f:f1"); err != nil || id != "f1" {
		t.Errorf("ficheroDe = %q, %v", id, err)
	}
	if _, err := ficheroDe("c:c1"); err == nil {
		t.Error("una carpeta pasa por fichero")
	}
}

// TestLoDelExploradorLlegaADrive: renombrar, mover, las dos cosas, sacarlo de la
// carpeta (la papelera de reciclaje), guardar encima, crear un fichero y una carpeta,
// y que los temporales de los editores no suban.
func TestLoDelExploradorLlegaADrive(t *testing.T) {
	var hechos []string
	d := doble{hechos: &hechos}
	ctx := context.Background()
	for _, c := range []struct {
		identidad string
		m         Movimiento
	}{
		{"f:f1", planRenombre(true, "raiz", "raiz", "a.txt", "b.txt")},
		{"f:f1", planRenombre(true, "raiz", "c:c1", "a.txt", "a.txt")},
		{"c:c2", planRenombre(true, "c:c1", "raiz", "Viejo", "Nuevo")},
		{"f:f1", planRenombre(false, "raiz", "", "a.txt", "a.txt")},
		{"f:f1", planRenombre(true, "raiz", "raiz", "a.txt", "a.txt")}, // nada que hacer
	} {
		if err := aplicarMovimiento(ctx, d, c.identidad, c.m); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := subirVersion(ctx, d, "f:f1", "", strings.NewReader("v2"), 2); err != nil {
		t.Fatal(err)
	}
	if x, err := subirNuevo(ctx, d, "c:c1", "nuevo.txt", "text/plain", strings.NewReader("hola"), 4); err != nil || x.ID != "f9" {
		t.Fatalf("subirNuevo = %+v, %v", x, err)
	}
	if id, err := crearCarpeta(ctx, d, "raiz", "Proyectos"); err != nil || id != "c:c9" {
		t.Fatalf("crearCarpeta = %q, %v", id, err)
	}
	quiero := []string{
		"renombrar files f1 b.txt",
		"mover files f1 c1",
		"mover folders c2 r", "renombrar folders c2 Nuevo",
		"borrar files f1",
		"subir c1/enero.pdf application/pdf v2", // versión nueva: su carpeta, su nombre y su tipo
		"subir c1/nuevo.txt text/plain hola",
		"carpeta r/Proyectos",
	}
	if !slices.Equal(hechos, quiero) {
		t.Errorf("en Drive:\n%q\nquiero:\n%q", hechos, quiero)
	}
	if err := aplicarMovimiento(ctx, d, "x:1", Movimiento{Borrar: true}); err == nil {
		t.Error("una identidad desconocida no falla")
	}
	// Lo que Drive ya no tiene se deja borrar; lo que Drive no deja borrar, no.
	if err := aplicarMovimiento(ctx, d, "f:ya-no", Movimiento{Borrar: true}); err != nil {
		t.Errorf("borrar lo que Drive ya no tiene = %v", err)
	}
	if err := aplicarMovimiento(ctx, d, "f:prohibido", Movimiento{Borrar: true}); err == nil {
		t.Error("un 403 de Drive deja borrar en el disco")
	}
	for n, quiere := range map[string]bool{
		"~$informe.docx": true, "A1B2C3.tmp": true, ".~lock.hoja.ods#": true, "informe.docx": false, "tmp.txt": false,
	} {
		if temporal(n) != quiere {
			t.Errorf("temporal(%q) = %v", n, !quiere)
		}
	}
}

// TestElRefrescoComparaPorIdentidad: lo nuevo de Drive entra, lo que Drive ya no tiene
// sale, una carpeta renombrada en la web se renombra (no se borra y se crea), un fichero
// con otra versión en Drive se actualiza, y lo que tiene cambios sin subir no se toca.
func TestElRefrescoComparaPorIdentidad(t *testing.T) {
	remotas := []Entrada{
		{Identidad: "c:a", Nombre: "Nombre nuevo", Carpeta: true},
		{Identidad: "f:b@2", Nombre: "b.txt"}, // otra versión en Drive
		{Identidad: "f:c@1", Nombre: "c.txt"}, // la misma
		{Identidad: "f:d@2", Nombre: "d.txt"}, // otra versión, pero hay cambios locales
		{Identidad: "f:n@1", Nombre: "nuevo.txt"},
		{Identidad: "f:o@1", Nombre: "ocupado.txt"}, // nombre ocupado por algo local
	}
	locales := []Local{
		{Identidad: "c:a", Nombre: "Nombre viejo", Carpeta: true, EnSync: true},
		{Identidad: "f:b@1", Nombre: "b.txt", EnSync: true},
		{Identidad: "f:c@1", Nombre: "c.txt", EnSync: true},
		{Identidad: "f:d@1", Nombre: "d.txt", EnSync: false},
		{Identidad: "f:x@1", Nombre: "borrado en la web.txt", EnSync: true},
		{Identidad: "f:y@1", Nombre: "borrado pero cambiado.txt", EnSync: false},
		{Nombre: "ocupado.txt"}, // no es de Drive todavía
	}
	c := comparar(remotas, true, locales)
	ids := func(es []Entrada) (out []string) {
		for _, e := range es {
			out = append(out, e.Identidad)
		}
		return out
	}
	if got := ids(c.Nuevas); !slices.Equal(got, []string{"f:n@1"}) {
		t.Errorf("nuevas = %q", got)
	}
	if got := ids(c.Actualizar); !slices.Equal(got, []string{"f:b@2"}) {
		t.Errorf("actualizar = %q (d.txt tiene cambios sin subir: gana lo local)", got)
	}
	if len(c.Renombrar) != 1 || c.Renombrar[0].Local.Identidad != "c:a" || c.Renombrar[0].Nombre != "Nombre nuevo" {
		t.Errorf("renombrar = %+v", c.Renombrar)
	}
	if len(c.Quitar) != 1 || c.Quitar[0].Identidad != "f:x@1" {
		t.Errorf("quitar = %+v (lo que tiene cambios sin subir no se quita)", c.Quitar)
	}
}

// TestLoGuardadoNoPisaLoDeDrive: un fichero que Windows da por no sincronizado solo sube
// si cambió en el disco; si además cambió en Drive, sube APARTE (conflicto) y no encima;
// si solo cambió en Drive, se baja lo de Drive. Medido en la VM: un renombre desde la web
// dejaba el marcador «no sincronizado» y subía lo viejo encima de la versión nueva.
func TestLoGuardadoNoPisaLoDeDrive(t *testing.T) {
	t0 := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	disco := IdentidadFichero(api.File{ID: "f1", UpdatedAt: t0})
	igual := api.File{ID: "f1", UpdatedAt: t0, SizeBytes: 10}
	nuevaEnDrive := api.File{ID: "f1", UpdatedAt: t1, SizeBytes: 12}
	ahora := hora(t1.Add(time.Minute))
	for _, c := range []struct {
		nombre       string
		drive        api.File
		fecha, largo int64
		quiero       Subida
	}{
		{"renombrado, sin cambios", igual, hora(t0), 10, SoloMarcar},
		{"guardado en el disco", igual, ahora, 11, Subir},
		{"mismo tamaño, fecha nueva", igual, ahora, 10, Subir},
		{"solo cambió en Drive", nuevaEnDrive, hora(t0), 10, TomarDrive},
		{"cambió en los dos", nuevaEnDrive, ahora, 11, Conflicto},
	} {
		if got := decidirSubida(disco, c.drive, c.fecha, c.largo); got != c.quiero {
			t.Errorf("%s: %d, quiero %d", c.nombre, got, c.quiero)
		}
	}
	if decidirSubida("f:f1", nuevaEnDrive, ahora, 1) != Subir {
		t.Error("una identidad sin versión no sube")
	}
	if got := nombreConflicto("informe.final.docx"); got != "informe.final (conflicto de Windows).docx" {
		t.Errorf("nombreConflicto = %q", got)
	}
	if base(disco) != "f:f1" || ficheroDe2(disco) != "f1" {
		t.Errorf("la identidad %q no da su id", disco)
	}
}

func ficheroDe2(identidad string) string {
	id, _ := ficheroDe(identidad)
	return id
}

// TestUnaCarpetaQueYaEstaEnDriveSeAdopta: crear «Fotos» donde ya la hay da 409; se usa
// la que existe en vez de dejar la del disco sin sincronizar.
func TestUnaCarpetaQueYaEstaEnDriveSeAdopta(t *testing.T) {
	d := doble{paginas: map[string]api.Children{"r|": {Folders: []api.Folder{{ID: "c3", Name: "fotos"}}}}}
	if id, err := crearCarpeta(t.Context(), d, "raiz", "Fotos"); err != nil || id != "c:c3" {
		t.Fatalf("crearCarpeta = %q, %v; quiero c:c3", id, err)
	}
	d.paginas["r|"] = api.Children{}
	if _, err := crearCarpeta(t.Context(), d, "raiz", "Fotos"); err == nil {
		t.Fatal("sin carpeta que adoptar, el 409 tiene que salir")
	}
}

// TestLaCarpetaNoPoneNombresQueWindowsNoTrataComoFicheros (FYS-08): los dispositivos
// de Windows y los nombres con giro de escritura no llegan a la carpeta.
func TestLaCarpetaNoPoneNombresQueWindowsNoTrataComoFicheros(t *testing.T) {
	for _, n := range []string{"CON", "nul.txt", "COM1.pdf", "lpt9", "Aux.", "factura\u202Efdp.exe"} {
		if nombreValido(n) {
			t.Errorf("%q pasa a la carpeta", n)
		}
	}
	for _, n := range []string{"CONTRATO.pdf", "com.txt", "COM10.txt", "informe.pdf", "año.txt"} {
		if !nombreValido(n) {
			t.Errorf("%q no pasa a la carpeta", n)
		}
	}
}

// TestUnaListaCortadaNoQuitaNada (FYS-03): `listar` se corta en `maxEntradas`, y lo que
// no se ha visto no es que Drive no lo tenga. Con la lista cortada no se quita nada.
func TestUnaListaCortadaNoQuitaNada(t *testing.T) {
	pagina := api.Children{NextCursor: "2"}
	for i := range maxEntradas {
		pagina.Files = append(pagina.Files, api.File{ID: strconv.Itoa(i), Name: strconv.Itoa(i) + ".txt"})
	}
	d := doble{paginas: map[string]api.Children{"r|": pagina, "r|2": {Files: []api.File{{ID: "z", Name: "z.txt"}}}}}
	es, completa, err := listar(t.Context(), d, raiz)
	if err != nil || completa || len(es) != maxEntradas {
		t.Fatalf("listar = %d entradas, completa=%v, %v; quiero %d y cortada", len(es), completa, err, maxEntradas)
	}
	fuera := []Local{{Identidad: "f:z@1", Nombre: "z.txt", EnSync: true}}
	if c := comparar(es, completa, fuera); len(c.Quitar) != 0 {
		t.Errorf("con la lista cortada se quita %+v", c.Quitar)
	}
	if c := comparar(es, true, fuera); len(c.Quitar) != 1 {
		t.Errorf("con la lista entera no se quita lo que Drive ya no tiene: %+v", c.Quitar)
	}
}

// TestUnRenombreDeDriveNoPisaLoDelDisco (FYS-03, decidido por el dueño): si lo renombrado
// en Drive se llama como otra cosa del disco —sin distinguir mayúsculas, como Windows—,
// lo nuestro toma un nombre libre, que tampoco usa nada de Drive, y se queda ahí en las
// vueltas siguientes; y si Drive trae otra versión, se vacía con ese nombre.
func TestUnRenombreDeDriveNoPisaLoDelDisco(t *testing.T) {
	remotas := []Entrada{
		{Identidad: "f:a@1", Nombre: "B.docx"},
		{Identidad: "f:c@1", Nombre: "B (2).docx"}, // otro de Drive, todavía sin marcador
		{Identidad: "f:m@1", Nombre: "Mayúsculas.txt"},
		{Identidad: "c:i", Nombre: "Informe.2026", Carpeta: true},
	}
	locales := []Local{
		{Identidad: "f:a@1", Nombre: "A.docx", EnSync: true},
		{Nombre: "b.docx"}, // del disco, sin subir
		{Identidad: "f:m@1", Nombre: "mayúsculas.txt", EnSync: true},
		{Identidad: "c:i", Nombre: "Viejo", Carpeta: true, EnSync: true},
		{Nombre: "informe.2026", Carpeta: true},
	}
	c := comparar(remotas, true, locales)
	got := make([]string, 0, len(c.Renombrar))
	for _, r := range c.Renombrar {
		got = append(got, r.Local.Nombre+" -> "+r.Nombre)
	}
	quiero := []string{"A.docx -> B (3).docx", "mayúsculas.txt -> Mayúsculas.txt", "Viejo -> Informe.2026 (2)"}
	if !slices.Equal(got, quiero) {
		t.Errorf("renombrar = %q\nquiero %q", got, quiero)
	}
	// La vuelta siguiente: ya se llama «B (3).docx» y no se mueve; Drive trae otra versión.
	remotas[0].Identidad = "f:a@2"
	locales[0].Nombre = "B (3).docx"
	c = comparar(remotas, true, locales[:2])
	if len(c.Renombrar) != 0 || len(c.Actualizar) != 1 || c.Actualizar[0].Nombre != "B (3).docx" {
		t.Errorf("la vuelta siguiente: renombrar %+v, actualizar %+v", c.Renombrar, c.Actualizar)
	}
}

// TestQuitarVaALaPapeleraYNoTocaLoPendiente (FYS-03, decidido por el dueño: «Papelera y
// no tocar lo pendiente»), contra un disco de mentira: lo que Drive ya no tiene va a la
// papelera de reciclaje; si dentro hay algo sin subir, con cambios o de los que no suben,
// se queda entero; y aunque Windows avise de cada hijo que sale, ninguno llega a Drive
// (una carpeta que solo se movió en la web vaciaba su contenido en la papelera de Drive).
func TestQuitarVaALaPapeleraYNoTocaLoPendiente(t *testing.T) {
	clientes := Local{Identidad: "c:cl", Nombre: "Clientes", Carpeta: true, EnSync: true}
	limpio := map[string][]Local{
		"D/Clientes": {
			{Identidad: "f:a@1", Nombre: "a.pdf", EnSync: true},
			{Identidad: "c:s", Nombre: "Sub", Carpeta: true}, // una carpeta «no sincronizada» es solo un atributo
		},
		"D/Clientes/Sub": {
			{Identidad: "f:b@1", Nombre: "b.pdf", EnSync: true, SinBajar: true},
			{Identidad: "c:v", Nombre: "Sin abrir", Carpeta: true, SinBajar: true},
		},
	}
	con := func(ruta string, l Local) map[string][]Local {
		d := map[string][]Local{}
		for k, v := range limpio {
			d[k] = slices.Clone(v)
		}
		d[ruta] = append(d[ruta], l)
		return d
	}
	for _, c := range []struct {
		nombre    string
		disco     map[string][]Local
		pendiente bool
	}{
		{"todo en Drive", limpio, false},
		{"un temporal de Word", con("D/Clientes/Sub", Local{Nombre: "~$b.docx"}), true},
		{"un fichero sin subir", con("D/Clientes", Local{Nombre: "nuevo.txt"}), true},
		{"una carpeta sin subir", con("D/Clientes/Sub", Local{Nombre: "Nueva", Carpeta: true}), true},
		{"cambios sin subir", con("D/Clientes", Local{Identidad: "f:x@1", Nombre: "x.pdf"}), true},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			leer := func(ruta string) ([]Local, error) {
				if strings.HasSuffix(ruta, "Sin abrir") {
					t.Error("entra en una carpeta sin poblar: la poblaría")
				}
				return c.disco[ruta], nil
			}
			var enPapelera, aDrive []string
			// Lo que haría Windows si avisara de cada elemento que sale: lo que no está en
			// `propios` iría a Drive (`alBorrar`).
			var avisar func(ruta, identidad string)
			avisar = func(ruta, identidad string) {
				if _, ok := propios.Load(identidad); !ok {
					aDrive = append(aDrive, identidad)
				}
				for _, h := range c.disco[ruta] {
					avisar(filepath.Join(ruta, h.Nombre), h.Identidad)
				}
			}
			err := quitar("D/Clientes", clientes, leer, func(ruta string) error {
				enPapelera = append(enPapelera, ruta)
				avisar(ruta, clientes.Identidad)
				return nil
			})
			switch {
			case c.pendiente && (!errors.Is(err, errPendiente) || len(enPapelera) > 0):
				t.Errorf("con algo pendiente dentro: %v, a la papelera %q", err, enPapelera)
			case !c.pendiente && (err != nil || !slices.Equal(enPapelera, []string{"D/Clientes"})):
				t.Errorf("sin nada pendiente: %v, a la papelera %q", err, enPapelera)
			}
			if len(aDrive) > 0 {
				t.Errorf("se borraría en Drive %q", aDrive)
			}
			propios.Range(func(k, _ any) bool {
				t.Errorf("%v se queda en propios", k)
				return true
			})
		})
	}
	// Si no se puede leer lo de dentro, se queda.
	err := quitar("D/Clientes", clientes, func(string) ([]Local, error) { return nil, errors.New("acceso denegado") },
		func(string) error { t.Error("a la papelera sin saber qué hay dentro"); return nil })
	if err == nil {
		t.Error("sin poder leer lo de dentro, no falla")
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// fichero deja un fichero de prueba y devuelve su URI, como la que entrega soltarlo.
func fichero(t *testing.T, nombre, contenido string) fyne.URI {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), nombre)
	if err := os.WriteFile(ruta, []byte(contenido), 0o600); err != nil {
		t.Fatal(err)
	}
	return storage.NewFileURI(ruta)
}

// sobreFila es el punto del lienzo sobre la fila `fila` (desde 0) de la rejilla, en su
// primera columna de datos.
func sobreFila(p *Pantalla, fila int) fyne.Position {
	g := p.abierta.rejilla
	a := fyne.CurrentApp().Driver().AbsolutePositionForObject(g)
	return a.Add(fyne.NewPos(g.r.ancho0+4, altoCabeza+(float32(fila)+0.5)*g.r.alto))
}

// conAdjuntosEnPersonas añade a «Personas» un campo de adjuntos.
func conAdjuntosEnPersonas(f *falso) {
	t := f.tabla("t1")
	t["fields"] = append(t["fields"].([]m), campoDe("adj", "t1", "Ficheros", "attachment", nil))
}

// TestSoltarFicherosLosAdjunta: soltados sobre una fila son adjuntos de esa fila;
// sobre la ficha abierta, de su registro. La ficha los enseña, los descarga y los borra,
// y dice por qué no ha subido uno que el servicio no admite.
func TestSoltarFicherosLosAdjunta(t *testing.T) {
	f := nuevoFalso(2)
	conAdjuntosEnPersonas(f)
	p, w := abrir(t, f)
	w.Resize(fyne.NewSize(1100, 760))
	p.Repintar()
	var guardado string
	p.Guardar = func(n string, r io.Reader) (string, error) {
		datos, err := io.ReadAll(r)
		guardado = n + ":" + string(datos)
		return "/descargas/" + n, err
	}

	// Mientras hay ficheros encima, la rejilla se recuadra.
	p.Arrastrando(true)
	if !p.abierta.rejilla.resaltada {
		t.Error("con ficheros encima, la rejilla no dice que se pueden soltar")
	}
	p.Soltado(sobreFila(p, 1), []fyne.URI{fichero(t, "factura.txt", "hola")})
	if p.abierta.rejilla.resaltada {
		t.Error("al soltar, el recuadro no se apaga")
	}
	if got := f.peticiones("POST /records/"); len(got) != 1 || !strings.HasPrefix(got[0], "POST /records/r002/attachments?field=adj&name=factura.txt hola") {
		t.Fatalf("lo soltado sobre la fila 2 no se ha adjuntado a ella: %q", got)
	}
	if a := p.abierta.rejilla.Registros[1].Attachments["adj"]; len(a) != 1 || a[0].Name != "factura.txt" {
		t.Fatalf("la rejilla no enseña el adjunto nuevo: %+v", a)
	}

	// En la ficha: lo soltado va a su registro, y cada fichero se baja y se borra.
	pulsar(t, w, "Fila 1")
	p.Soltado(fyne.NewPos(10, 10), []fyne.URI{fichero(t, "foto.txt", "píxeles")})
	capturar(t, w, "ficha-adjuntos")
	if !strings.Contains(textos(w), "foto.txt") || len(f.peticiones("POST /records/r001/attachments")) != 1 {
		t.Fatalf("lo soltado sobre la ficha no se ha adjuntado a su registro:\n%s", textos(w))
	}
	pulsar(t, w, "Descargar foto.txt")
	if guardado != "foto.txt:contenido del adjunto" || !strings.Contains(textos(w), "foto.txt — /descargas/foto.txt") {
		t.Errorf("descargar no deja el fichero o no dice dónde: %q", guardado)
	}
	p.Soltado(fyne.NewPos(10, 10), []fyne.URI{fichero(t, "virus.exe", "MZ")})
	if !strings.Contains(textos(w), "Este tipo de fichero no se admite") {
		t.Errorf("un tipo que el servicio rechaza no se explica:\n%s", textos(w))
	}
	pulsar(t, w, "Borrar foto.txt")
	pulsar(t, w, "Borrar")
	if len(f.peticiones("DELETE /records/r001/attachments/")) != 1 || len(p.abierta.rejilla.Registros[0].Attachments["adj"]) != 0 {
		t.Fatalf("borrar un adjunto no lo quita: %q", f.peticiones("DELETE"))
	}
}

// TestUnFicheroQuePasaDelTopeNoSeSube: se avisa sin mandarlo (el servicio daría 413).
func TestUnFicheroQuePasaDelTopeNoSeSube(t *testing.T) {
	f := nuevoFalso(1)
	conAdjuntosEnPersonas(f)
	p, w := abrir(t, f)
	ruta := filepath.Join(t.TempDir(), "enorme.bin")
	g, err := os.Create(ruta)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Truncate(api.MaxBytesAdjuntoBases + 1); err != nil {
		t.Fatal(err)
	}
	_ = g.Close()
	p.Soltado(sobreFila(p, 0), []fyne.URI{storage.NewFileURI(ruta)})
	if len(f.peticiones("POST /records/")) != 0 || !strings.Contains(textos(w), "El fichero pasa de 20 MB.") {
		t.Fatalf("un fichero de más de 20 MB se ha mandado o no se avisa:\n%s", textos(w))
	}
}

// TestSoltarUnCSVLoImporta: un CSV soltado sobre la base abre «Importar» con el nombre
// del fichero, y la tabla nueva se abre al importarla. También desde el menú.
func TestSoltarUnCSVLoImporta(t *testing.T) {
	f := nuevoFalso(1)
	p, w := abrir(t, f)
	if slices.Contains(elementosDe(t, w, "Clientes"), "Importar CSV") {
		t.Error("sin selector de ficheros, el menú ofrece importar")
	}
	csv := fichero(t, "Proveedores 2026.csv", "Cliente\nAcme\n")
	p.Soltado(sobreFila(p, 0), []fyne.URI{csv})
	capturar(t, w, "dialogo-importar")
	if got := campoDeDialogo(t, w, "Nombre de la nueva tabla").Texto(); got != "Proveedores 2026" {
		t.Fatalf("el nombre de la tabla no sale del fichero: %q", got)
	}
	pulsar(t, w, "Importar")
	if !f.vio("POST /bases/b1/import?name=Proveedores+2026 Cliente\nAcme") {
		t.Fatalf("no se ha importado el CSV: %q", f.peticiones("POST /bases"))
	}
	if t3 := p.abierta.tabla(); t3 == nil || t3.Name != "Proveedores 2026" {
		t.Fatalf("la tabla importada no se abre: activa %q", p.abierta.Activa)
	}

	// Desde el menú, con el selector del sistema.
	p.Elegir = func(listo func([]string)) { listo([]string{csv.Path()}) }
	pulsarEnMenu(t, w, "Clientes", "Importar CSV")
	if campoDeDialogo(t, w, "Nombre de la nueva tabla") == nil {
		t.Fatal("«Importar CSV» del menú no abre el diálogo")
	}
}

// portapapeles es un portapapeles de prueba.
type portapapeles struct{ texto string }

func (c *portapapeles) Content() string     { return c.texto }
func (c *portapapeles) SetContent(s string) { c.texto = s }

// TestPegarUnBloqueDeUnaHoja: lo copiado de una hoja de cálculo entra desde la celda con
// el foco, cada valor en su campo; las filas que sobran son registros nuevos, y un valor
// que no encaja se deja y se avisa. Ctrl+C copia la celda.
func TestPegarUnBloqueDeUnaHoja(t *testing.T) {
	f := nuevoFalso(2)
	p, w := abrir(t, f)
	g := p.abierta.rejilla
	w.Canvas().Focus(g)
	clip := &portapapeles{}
	g.TypedShortcut(&fyne.ShortcutCopy{Clipboard: clip})
	if clip.texto != "Persona 1" {
		t.Fatalf("copiar la celda deja %q", clip.texto)
	}

	clip.texto = "Ana\t30\r\nBea\tcuarenta\r\nCarla\t52,5\r\n"
	g.TypedShortcut(&fyne.ShortcutPaste{Clipboard: clip})
	for _, quiero := range []string{
		`PUT /records/r001/cells/nom?v=1 {"expected":"Persona 1","value":"Ana"}`,
		`PUT /records/r001/cells/eda?v=1 {"expected":21,"value":30}`,
		`PUT /records/r002/cells/nom?v=1 {"expected":"Persona 2","value":"Bea"}`,
		`POST /tables/t1/records {"data":{"eda":52.5,"nom":"Carla"}}`,
	} {
		if !f.vio(quiero) {
			t.Errorf("no se ha enviado %s; se envió:\n%s", quiero, strings.Join(append(f.peticiones("PUT"), f.peticiones("POST")...), "\n"))
		}
	}
	if len(f.peticiones("PUT /records/r002/cells/eda")) != 0 {
		t.Error("«cuarenta» se ha mandado como edad")
	}
	if len(g.Registros) != 3 || !strings.Contains(textos(w), "Ese valor no encaja en este campo.") {
		t.Fatalf("el registro que sobraba no se ha añadido (%d) o no se avisa del valor que no encaja", len(g.Registros))
	}

	// Soltar el mismo texto sobre una celda lo pega desde ella.
	p.SoltarTexto("Dani", sobreFila(p, 2))
	if !f.vio(`PUT /records/` + g.Registros[2].ID + `/cells/nom?v=1 {"expected":"Carla","value":"Dani"}`) {
		t.Fatalf("soltar texto sobre una celda no lo escribe ahí: %q", f.peticiones("PUT"))
	}
}

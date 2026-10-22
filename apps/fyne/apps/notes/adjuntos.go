// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Paridad web: los ADJUNTOS de una nota —`adjuntos`, `alSubirArchivos` y
// `eliminarAdjunto` de NoteCard.tsx—. Van dentro de `data.attachments` como URL de
// datos: el fichero ENTERO viaja con la nota, y por eso el servicio los topa con el
// resto de `data`.

// Adjunto es `NoteAttachment`.
type Adjunto struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Type    string `json:"type"`
	DataURL string `json:"dataUrl"` //nolint:tagliatelle // el nombre lo fija la web
}

// MaxDatosNota es el tope del servicio para `data` (services/notes, domain.go): un
// fichero mayor no cabe, y se dice antes de leerlo entero en memoria.
const MaxDatosNota = 5 << 20

var errAdjunto = errors.New("notes: el adjunto no es una URL de datos en base64")

// adjuntosDe lee `data.attachments`. Lo que no sea una lista de adjuntos no es ninguno.
func adjuntosDe(data json.RawMessage) []Adjunto {
	var d struct {
		Attachments []Adjunto `json:"attachments"`
	}
	if json.Unmarshal(data, &d) != nil {
		return nil
	}
	return d.Attachments
}

// conAdjuntos es `data` con la lista cambiada y lo demás intacto (`...dataActual`).
func conAdjuntos(data json.RawMessage, as []Adjunto) (json.RawMessage, error) {
	var m map[string]json.RawMessage
	if len(data) > 0 {
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, err
		}
	}
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	if as == nil {
		as = []Adjunto{} // `[]`, como el `filter` de la web, y no `null`
	}
	lista, err := json.Marshal(as)
	if err != nil {
		return nil, err
	}
	m["attachments"] = lista
	return api.CodificarDatosNotas(m)
}

// contenido son los bytes del fichero, sacados de su URL de datos.
func (a Adjunto) contenido() ([]byte, error) {
	cab, cuerpo, ok := strings.Cut(a.DataURL, ",")
	if !ok || !strings.HasPrefix(cab, "data:") || !strings.HasSuffix(cab, ";base64") {
		return nil, errAdjunto
	}
	return base64.StdEncoding.DecodeString(cuerpo)
}

// leerAdjunto es el `FileReader.readAsDataURL` de la web: el tipo sale de la extensión,
// como el `File.type` del navegador, y sin tipo la URL dice `application/octet-stream`.
func leerAdjunto(ruta string, ahora time.Time) (Adjunto, error) {
	info, err := os.Stat(ruta)
	if err != nil {
		return Adjunto{}, err
	}
	if info.Size() > MaxDatosNota {
		return Adjunto{}, fmt.Errorf("notes: %s no cabe en una nota", filepath.Base(ruta))
	}
	b, err := os.ReadFile(ruta)
	if err != nil {
		return Adjunto{}, err
	}
	tipo, _, _ := mime.ParseMediaType(mime.TypeByExtension(filepath.Ext(ruta)))
	enURL := tipo
	if enURL == "" {
		enURL = "application/octet-stream"
	}
	return Adjunto{
		ID:   strconv.FormatInt(ahora.UnixMilli(), 10) + "-" + strings.ToLower(rand.Text()[:6]),
		Name: filepath.Base(ruta), Size: int64(len(b)), Type: tipo,
		DataURL: "data:" + enURL + ";base64," + base64.StdEncoding.EncodeToString(b),
	}, nil
}

// adjuntar es «Adjuntar archivos…»: el selector del sistema y, con lo elegido, los
// adjuntos nuevos al final de los que ya tenga.
func (p *Pantalla) adjuntar(n Nota) {
	go func() {
		rutas, err := ficheros.Elegir()
		if err != nil || len(rutas) == 0 {
			return // cancelar el selector no es un error
		}
		nuevos := make([]Adjunto, 0, len(rutas))
		for _, r := range rutas {
			a, err := leerAdjunto(r, time.Now())
			if err != nil {
				p.avisar(ui.AvisoPeligro, p.tr.T("note.saveFailed", nil), "")
				return
			}
			nuevos = append(nuevos, a)
		}
		_ = p.cambiarAdjuntos(n.ID, func(as []Adjunto) []Adjunto { return append(as, nuevos...) })
	}()
}

// quitarAdjunto es `eliminarAdjunto`.
func (p *Pantalla) quitarAdjunto(n Nota, a Adjunto) {
	lanzar(func() error {
		return p.cambiarAdjuntos(n.ID, func(as []Adjunto) []Adjunto {
			return slices.DeleteFunc(slices.Clone(as), func(x Adjunto) bool { return x.ID == a.ID })
		})
	})
}

// cambiarAdjuntos reescribe la lista sobre el `data` que la nota tiene AHORA, no el de
// cuando se abrió el selector: lo demás de `data` viaja entero, porque el PUT sustituye.
func (p *Pantalla) cambiarAdjuntos(id string, f func([]Adjunto) []Adjunto) error {
	var datos json.RawMessage
	hay := false
	fyne.DoAndWait(func() {
		if n, ok := p.nota(id); ok {
			datos, hay = n.Datos, true
		}
	})
	if !hay {
		return nil
	}
	nuevo, err := conAdjuntos(datos, f(adjuntosDe(datos)))
	if err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("note.saveFailed", nil), "")
		return err
	}
	return p.editar(id, Parche{Datos: nuevo}, true)
}

// descargarAdjunto es el `<a download>`: el fichero, a la carpeta de descargas.
func (p *Pantalla) descargarAdjunto(a Adjunto) {
	go func() {
		b, err := a.contenido()
		if err != nil {
			p.avisar(ui.AvisoPeligro, p.tr.T("error.loadFailed", nil), "")
			return
		}
		p.descargar(a.Name, bytes.NewReader(b))
	}()
}

// Medidas del bloque de adjuntos, de sus clases.
const (
	// MargenAdjuntos es el `mt-2 … pt-2` del bloque, a cada lado del `border-t`.
	MargenAdjuntos = 2 * 0.25 * tema.Rem
	// LadoAccionAdjunto es el `h-6 w-6` de descargar y quitar: 21, sin el `min-h-[24px]`
	// que sí lleva `IconButton`.
	LadoAccionAdjunto = 6 * 0.25 * tema.Rem
	// GlifoAdjunto es el `size={13}` de los tres glifos.
	GlifoAdjunto = 13
	// NombreAdjunto y NombreAdjuntoSM son `max-w-44 sm:max-w-64`.
	NombreAdjunto   = 44 * 0.25 * tema.Rem
	NombreAdjuntoSM = 64 * 0.25 * tema.Rem
	// trackingWider es el `tracking-wider` (0,05 em) del rótulo.
	trackingWider = 0.05
)

// escalon10 es `text-[10px]`: el tamaño cambia y la altura de línea es la proporción de
// `text-xs` que hereda.
var escalon10 = tema.Escalon{Tam: 10, Linea: 10 * tema.TextoXS.Linea / tema.TextoXS.Tam}

// bloqueAdjuntos es `border-t pt-2 grid gap-1.5`: el rótulo con la cuenta y una fila por
// adjunto. El `mt-2` lo pone quien lo coloca.
func (t *Tarjeta) bloqueAdjuntos(v tema.Variante) fyne.CanvasObject {
	as := t.nota.Adjuntos
	rotulo := t.tr.T("note.attachments", nil) + " (" + strconv.Itoa(len(as)) + ")"
	vs := ui.NuevoVersalCon(rotulo, tema.TextoXS, ui.SemiNegrita, tema.Color("text-muted", v), trackingWider)
	filas := make([]fyne.CanvasObject, 0, len(as))
	for _, a := range as {
		filas = append(filas, t.filaAdjunto(a, v))
	}
	raya := canvas.NewRectangle(tema.Color("border", v))
	raya.SetMinSize(fyne.NewSize(0, tema.Borde))
	return container.New(ui.Margen{Arr: MargenAdjuntos}, container.New(ui.Columna{},
		raya,
		container.New(ui.Margen{Arr: MargenAdjuntos}, container.New(ui.Columna{Hueco: 1.5 * 0.25 * tema.Rem},
			container.New(&conVersal{v: vs, e: tema.TextoXS}, vs.Objetos()...),
			container.New(ui.Columna{Hueco: 1 * 0.25 * tema.Rem}, filas...)))))
}

// filaAdjunto es una fila: `rounded-md border bg-bg-subtle/50 px-2 py-1`, con el glifo,
// el nombre recortado y el tamaño a la izquierda y descargar y quitar a la derecha.
func (t *Tarjeta) filaAdjunto(a Adjunto, v tema.Variante) fyne.CanvasObject {
	accion := func(d, rotulo, trazo, encima string, f func()) *ui.BotonIcono {
		b := ui.NuevoBotonIcono(iconos.Ninguno, rotulo, f)
		b.Glifo = Dibujo(d, GlifoAdjunto, trazo, v)
		b.Lado, b.TamIcono, b.Radio = LadoAccionAdjunto, GlifoAdjunto, tema.RadioSM
		b.Trazo, b.TrazoEncima = trazo, encima
		return b
	}
	der := []fyne.CanvasObject{accion(BajarD, t.tr.T("note.downloadAttachment", nil), "primary-text", "primary",
		func() { llamar1(t.AlDescargar, a) })}
	if !t.SinAcciones {
		der = append(der, accion(PapeleraD, t.tr.T("note.deleteAttachment", nil), "text-muted", "danger-text",
			func() { llamar1(t.AlQuitarAdjunto, a) }))
	}
	maximo := float32(NombreAdjunto)
	if anchoVentana([]fyne.CanvasObject{t}) >= ui.CorteSM {
		maximo = NombreAdjuntoSM
	}
	f := &filaAdj{nombre: a.Name, maximo: maximo}
	f.texto = ui.Texto(a.Name, tema.TextoXS, ui.Medio, tema.Color("text", v))
	objs := []fyne.CanvasObject{
		Glifo(FicheroD, GlifoAdjunto, "text-subtle", v),
		f.texto,
		ui.Texto(i18n.FormatBytes(t.tr.Locale(), float64(a.Size)), escalon10, ui.Normal, tema.Color("text-muted", v)),
		container.New(ui.Fila{Hueco: 1 * 0.25 * tema.Rem}, der...),
	}
	caja := ui.NuevaCaja(tokens.SobreSRGB(velo(tema.Color("bg-subtle", v), 0.5), tema.Color("surface", v)), tema.Color("border", v), tema.RadioMD)
	px, py := 2*0.25*tema.Rem+tema.Borde, 1*0.25*tema.Rem+tema.Borde
	return container.New(&enCaja{caja: caja}, append(caja.Objetos(),
		container.New(ui.Margen{Izq: px, Der: px, Arr: py, Abj: py}, container.New(f, objs...)))...)
}

// filaAdj coloca [glifo, nombre, tamaño, acciones]: `justify-between gap-2`, con el
// nombre `truncate` hasta su `max-w` o hasta donde quepa, lo que sea menos.
type filaAdj struct {
	nombre string
	maximo float32
	texto  *canvas.Text
}

const (
	huecoFilaAdj  = 2 * 0.25 * tema.Rem   // gap-2
	huecoNombreAj = 1.5 * 0.25 * tema.Rem // gap-1.5
)

func (f *filaAdj) MinSize(objs []fyne.CanvasObject) fyne.Size {
	alto := tema.TextoXS.Linea
	for _, o := range objs {
		alto = max(alto, o.MinSize().Height)
	}
	return fyne.NewSize(0, alto)
}

func (f *filaAdj) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	glifo, tam, der := objs[0], objs[2], objs[3]
	mg, mt, md := glifo.MinSize(), ui.Medir(tamano0(tam), escalon10, ui.Normal), der.MinSize()
	cabe := t.Width - md.Width - huecoFilaAdj - mg.Width - 2*huecoNombreAj - mt.Width
	ancho := ui.Truncar(f.texto, f.nombre, tema.TextoXS, ui.Medio, min(f.maximo, max(cabe, 0)))
	x := float32(0)
	glifo.Move(fyne.NewPos(x, (t.Height-mg.Height)/2))
	glifo.Resize(mg)
	x += mg.Width + huecoNombreAj
	ui.ColocarTexto(f.texto, tema.TextoXS, x, (t.Height-tema.TextoXS.Linea)/2)
	x += ancho + huecoNombreAj
	if c, ok := tam.(*canvas.Text); ok {
		ui.ColocarTexto(c, escalon10, x, (t.Height-escalon10.Linea)/2)
	}
	der.Move(fyne.NewPos(t.Width-md.Width, (t.Height-md.Height)/2))
	der.Resize(md)
}

// tamano0 es el texto del tamaño, para medirlo.
func tamano0(o fyne.CanvasObject) string {
	if c, ok := o.(*canvas.Text); ok {
		return c.Text
	}
	return ""
}

// conVersal es un rótulo en versalitas en su caja de línea.
type conVersal struct {
	v *ui.Versal
	e tema.Escalon
}

func (c *conVersal) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(c.v.Ancho(), c.e.Linea)
}

func (c *conVersal) Layout([]fyne.CanvasObject, fyne.Size) { c.v.Colocar(0, 0) }

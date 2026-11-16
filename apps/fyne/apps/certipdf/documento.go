// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"strconv"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

/*
El DOCUMENTO abierto: dónde vive en Drive, sus bytes y la contraseña con que se abrió.

En la web el BFF vuelve a bajar el PDF de Drive en CADA llamada —el navegador no lo
tiene—. Aquí se baja UNA vez y se guarda en memoria: el motor lo necesita entero en cada
operación y subirlo desde aquí es lo mismo que subirlo desde el BFF, pero sin el viaje de
ida a Drive. Al cambiarlo, lo que devuelve el motor pasa a ser el documento: no hace
falta bajarlo otra vez para seguir.
*/

// Documento es un PDF abierto.
type Documento struct {
	pdf *api.PDF

	mu      sync.Mutex
	ruta    string
	fichero api.File
	datos   []byte
	clave   string
	// recargar dice que en Drive hay otro contenido (se acaba de restaurar una versión)
	// y lo que hay en memoria ya no es el documento: lo próximo que lo lea lo baja antes.
	recargar bool
}

// AbrirDocumento trae de Drive el PDF de esa RUTA, que es como se abre CertiPDF en la web
// (`/pdf?path=…`). Un fichero que no existe es un 404 del servicio.
func AbrirDocumento(ctx context.Context, pdf *api.PDF, ruta string) (*Documento, error) {
	f, err := pdf.PorRuta(ctx, ruta)
	if err != nil {
		return nil, err
	}
	datos, err := pdf.Descargar(ctx, f.ID)
	if err != nil {
		return nil, err
	}
	return &Documento{pdf: pdf, ruta: ruta, fichero: f, datos: datos}, nil
}

// SubirYAbrir guarda en la carpeta «PDF» de Drive un fichero de la máquina y lo abre. Es
// lo que hace la web con «abrir un PDF del ordenador» (`pdf-abrir.ts`): el documento se
// anota sobre Drive, con su historial, y no sobre una copia suelta.
func SubirYAbrir(ctx context.Context, pdf *api.PDF, nombre string, datos []byte) (*Documento, error) {
	f, err := pdf.Subir(ctx, nombre, bytes.NewReader(datos), int64(len(datos)))
	if err != nil {
		return nil, err
	}
	return &Documento{pdf: pdf, ruta: "/" + api.PDFCarpeta + "/" + f.Name, fichero: f, datos: datos}, nil
}

// Ruta es la ruta de Drive del documento.
func (d *Documento) Ruta() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ruta
}

// Nombre es el del fichero, que es lo que va en el título de la ventana.
func (d *Documento) Nombre() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.fichero.Name
}

// PonerClave fija la contraseña de apertura con que se mandan las operaciones.
func (d *Documento) PonerClave(c string) {
	d.mu.Lock()
	d.clave = c
	d.mu.Unlock()
}

func (d *Documento) paraElMotor() (api.PDFFichero, string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return api.PDFFichero{Nombre: d.fichero.Name, Datos: d.datos}, d.clave
}

// PrepararFirma es el primer paso de la firma con el certificado del sistema.
func (d *Documento) PrepararFirma(ctx context.Context, opts api.PDFFirmaOpciones, cadena [][]byte) (api.PDFFirmaPreparada, error) {
	doc, clave := d.paraElMotor()
	return d.pdf.PrepararFirma(ctx, doc, clave, opts, cadena)
}

// GuardarFirmado deja el firmado como copia al lado del original, como toda firma.
func (d *Documento) GuardarFirmado(ctx context.Context, datos []byte, sufijo string) (api.File, error) {
	d.mu.Lock()
	original := d.fichero
	d.mu.Unlock()
	return d.pdf.GuardarCopia(ctx, original, sufijo, datos)
}

// Leer manda una operación de LECTURA al motor y decodifica su JSON en `out`. Son las
// trece de `pdf.ts`; lo que las distingue son la ruta y los campos.
func (d *Documento) Leer(ctx context.Context, ruta string, campos map[string]string, out any) error {
	r, err := d.Binario(ctx, ruta, campos)
	if err != nil {
		return err
	}
	return json.Unmarshal(r, out)
}

// Binario es Leer sin decodificar: un adjunto, el XFDF exportado.
func (d *Documento) Binario(ctx context.Context, ruta string, campos map[string]string) ([]byte, error) {
	doc, clave := d.paraElMotor()
	c := map[string]string{"clave": clave}
	for k, v := range campos {
		c[k] = v
	}
	r, err := d.pdf.Motor(ctx, ruta, c, nil, doc)
	return r.Datos, err
}

// DiferenciaPagina es lo que cambió en el texto de una página al comparar.
type DiferenciaPagina struct {
	Pagina    int      `json:"pagina"`
	Anadido   []string `json:"anadido"`
	Quitado   []string `json:"quitado"`
	SoloEnUno bool     `json:"solo_en_uno"`
}

// Comparacion es la respuesta de `/pdf/compare`.
type Comparacion struct {
	Diferencias []DiferenciaPagina `json:"diferencias"`
	Iguales     bool               `json:"iguales"`
}

// TraerOtro baja de Drive el documento con el que se compara, por su ruta.
func (d *Documento) TraerOtro(ctx context.Context, otra string) (api.PDFFichero, error) {
	f, err := d.pdf.PorRuta(ctx, otra)
	if err != nil {
		return api.PDFFichero{}, err
	}
	datos, err := d.pdf.Descargar(ctx, f.ID)
	if err != nil {
		return api.PDFFichero{}, err
	}
	return api.PDFFichero{Nombre: f.Name, Datos: datos}, nil
}

// Comparar es `op: comparar` con otro documento de Drive, por su ruta: el texto (para los
// píxeles, [Documento.CompararVisual]). Como en la web, sin contraseña.
func (d *Documento) Comparar(ctx context.Context, otra string) (Comparacion, error) {
	var c Comparacion
	otro, err := d.TraerOtro(ctx, otra)
	if err != nil {
		return c, err
	}
	doc, _ := d.paraElMotor()
	r, err := d.pdf.Motor(ctx, "/pdf/compare", nil, nil, doc, otro)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(r.Datos, &c)
	return c, err
}

// Contenido es `op: contenido`: todo lo que la pantalla necesita saber del documento.
func (d *Documento) Contenido(ctx context.Context) (*Contenido, error) {
	if err := d.alDia(ctx); err != nil {
		return nil, err
	}
	var c Contenido
	if err := d.Leer(ctx, "/pdf/content", nil, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Texto es `op: texto`: los fragmentos de cada página, para buscar y para el asistente.
func (d *Documento) Texto(ctx context.Context) ([][]Fragmento, error) {
	var r struct {
		Paginas [][]Fragmento `json:"paginas"`
	}
	err := d.Leer(ctx, "/pdf/text", nil, &r)
	return r.Paginas, err
}

// SVG es la página `n` (1-based) dibujada como la ve la web: con el texto como `<text>`.
// No se pinta —el pintor de Fyne no sabe de `<text>`—: se lee para seleccionar texto.
func (d *Documento) SVG(ctx context.Context, n int) (string, error) {
	doc, clave := d.paraElMotor()
	return d.pdf.Render(ctx, doc, n, 1, clave)
}

// Imagenes dibuja las páginas `desde`..`hasta` (1-based) a `escala` —72 ppp por uno— y
// las devuelve decodificadas. Una que el motor no supo dibujar NO está en el mapa.
func (d *Documento) Imagenes(ctx context.Context, desde, hasta int, escala float64) (map[int]image.Image, error) {
	doc, clave := d.paraElMotor()
	rango := strconv.Itoa(desde)
	if hasta > desde {
		rango += "-" + strconv.Itoa(hasta)
	}
	crudos, err := d.pdf.RenderPNG(ctx, doc, rango, escala, clave)
	if err != nil {
		return nil, err
	}
	out := make(map[int]image.Image, len(crudos))
	for n, b := range crudos {
		if img, err := png.Decode(bytes.NewReader(b)); err == nil {
			out[n] = img
		}
	}
	return out, nil
}

// Aplicar cambia el documento (`/api/pdf-aplicar`). Si el resultado SUSTITUYE al original,
// sus bytes pasan a ser el documento; si es una copia, el documento sigue siendo el que
// era y la copia se abre aparte.
func (d *Documento) Aplicar(ctx context.Context, c api.PDFCambio) (api.PDFAplicado, error) {
	// Cambiar sobre lo que había antes de restaurar desharía la restauración.
	if err := d.alDia(ctx); err != nil {
		return api.PDFAplicado{}, err
	}
	d.mu.Lock()
	f := d.fichero
	d.mu.Unlock()
	doc, clave := d.paraElMotor()
	r, err := d.pdf.Aplicar(ctx, f, doc, clave, c)
	if err != nil {
		return r, err
	}
	if !r.Copia {
		d.mu.Lock()
		d.fichero, d.datos = r.Fichero, r.Datos
		d.mu.Unlock()
	}
	return r, nil
}

// RutaDe es la ruta de Drive de un fichero que acaba de dejar una operación: al lado del
// documento, en su misma carpeta.
func (d *Documento) RutaDe(f api.File) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	i := len(d.ruta) - len(d.fichero.Name)
	if i < 0 || d.ruta[i:] != d.fichero.Name {
		return "/" + f.Name
	}
	return d.ruta[:i] + f.Name
}

// asistir keeps text and page images out of the legacy AI service when the
// document came from private Drive, including requests queued before a UI change.
func (d *Documento) asistir(ctx context.Context, client *api.IA, request api.PeticionIA) (string, error) {
	if d == nil || client == nil {
		return "", api.ErrPrivatePDFOperation
	}
	if err := d.pdf.CheckRemoteProcessing(ctx); err != nil {
		return "", err
	}
	return client.Asistir(ctx, request)
}

// Versiones es una página del historial del documento en Drive, la actual primero.
func (d *Documento) Versiones(ctx context.Context, cursor string) ([]api.DriveVersion, string, error) {
	d.mu.Lock()
	id := d.fichero.ID
	d.mu.Unlock()
	return d.pdf.Versiones(ctx, id, cursor)
}

// Version trae los bytes de una versión archivada, para verla aparte.
func (d *Documento) Version(ctx context.Context, n int) ([]byte, error) {
	d.mu.Lock()
	id := d.fichero.ID
	d.mu.Unlock()
	return d.pdf.DescargarVersion(ctx, id, n)
}

// Restaurar hace actual la versión `n` en Drive. Lo que hay en memoria es la versión
// que se acaba de dejar, y seguir anotando sobre ella pisaría lo restaurado con el
// siguiente cambio: queda marcado para bajarlo otra vez, y eso lo hace [Documento.Contenido],
// que es lo primero que pide el visor. Así, si la red cae justo después de restaurar, no se
// dice «no se ha podido restaurar» de algo que sí se restauró, y «Reintentar» del visor
// vuelve a intentar la bajada en vez de enseñar el documento viejo.
func (d *Documento) Restaurar(ctx context.Context, n int) error {
	d.mu.Lock()
	id := d.fichero.ID
	d.mu.Unlock()
	if err := d.pdf.RestaurarVersion(ctx, id, n); err != nil {
		return err
	}
	d.mu.Lock()
	d.recargar = true
	d.mu.Unlock()
	return nil
}

// alDia vuelve a traer de Drive el documento si se restauró una versión desde la última
// vez: el fichero (su versión nueva, que el siguiente cambio necesita) y sus bytes.
func (d *Documento) alDia(ctx context.Context) error {
	d.mu.Lock()
	toca, ruta := d.recargar, d.ruta
	d.mu.Unlock()
	if !toca {
		return nil
	}
	f, err := d.pdf.PorRuta(ctx, ruta)
	if err != nil {
		return err
	}
	datos, err := d.pdf.Descargar(ctx, f.ID)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.fichero, d.datos, d.recargar = f, datos, false
	d.mu.Unlock()
	return nil
}

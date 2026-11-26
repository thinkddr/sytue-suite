// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package biblioteca

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
)

// DOCUMENTOS ANCLADOS A UN FICHERO DEL DISCO (PLAN-EDICION-LOCAL O-06 a O-08).
//
// Abrir un `.docx` o un `.docus` del disco trae su contenido a la biblioteca como un
// documento con `source_ref = file://<ruta>` (el mismo anclaje que `drive:<id>` en la
// edición conectada) y apunta la HUELLA del fichero (su SHA-256) tal como se leyó. Así:
//
//   - abrir el mismo fichero otra vez abre el MISMO documento, no una copia;
//   - Ctrl/Cmd+S lo escribe de vuelta en su sitio ([Biblioteca.GuardarEnFichero]): un
//     fichero nativo, tal cual; uno ajeno, convertido, pero solo tras preguntar la
//     primera vez y guardando antes una copia del original, `nombre (original).ext`;
//   - si el fichero cambió por fuera desde que se leyó (otra huella), no se pisa: se
//     devuelve [ErrCambioFuera] y la pantalla pregunta;
//   - lo que vino recortado o de una plantilla no se escribe de vuelta nunca: solo
//     «Guardar como» ([Biblioteca.GuardarComo]).
//
// Toda escritura es a un temporal en la misma carpeta y renombrado encima (R-NO-CORROMPE).

// Motivos por los que un documento no se guarda de vuelta en su fichero.
const (
	// BloqueoRecortado: el fichero no cabía entero y se importó recortado.
	BloqueoRecortado = "recortado"
	// BloqueoPlantilla: era una plantilla (.dotx, .ott…): se abre como documento nuevo.
	BloqueoPlantilla = "plantilla"
	// BloqueoSinVuelta: el formato se lee pero no se escribe (.csv, .txt, .pages…).
	BloqueoSinVuelta = "sin_vuelta"
)

var (
	// ErrSinFichero: el documento no está anclado a ningún fichero del disco.
	ErrSinFichero = errors.New("biblioteca: el documento no está anclado a un fichero")
	// ErrGuardadoBloqueado: el documento no se puede guardar de vuelta (ver Bloqueo*).
	ErrGuardadoBloqueado = errors.New("biblioteca: este documento no se puede guardar en su fichero de origen")
	// ErrConfirmarConversion: el fichero es de un formato ajeno y la persona aún no ha
	// aceptado que guardar en él es convertir ([Biblioteca.AceptarConversion]).
	ErrConfirmarConversion = errors.New("biblioteca: guardar en ese formato es una conversión y hay que confirmarlo")
	// ErrCambioFuera: el fichero ha cambiado desde que se leyó o se guardó por última vez.
	ErrCambioFuera = errors.New("biblioteca: el fichero ha cambiado fuera de la app")
	// ErrOtroTipo: el fichero es de otro editor (un `.calcus` abierto en Docus).
	ErrOtroTipo = errors.New("biblioteca: el fichero es de otro tipo de documento")
	// ErrFormatoSinVuelta: «Guardar como» a un formato que no se sabe escribir.
	ErrFormatoSinVuelta = errors.New("biblioteca: ese formato no se puede escribir")
)

// Fichero es el ancla de un documento a un fichero del disco.
type Fichero struct {
	// Ruta es la ruta absoluta del fichero.
	Ruta string `json:"ruta"`
	// Huella es el SHA-256 del fichero cuando se leyó o se escribió por última vez.
	Huella string `json:"huella,omitempty"`
	// Nativo: `.docus`, `.calcus` o `.diapos`; se guarda sin pérdida y sin preguntar.
	Nativo bool `json:"nativo,omitempty"`
	// Bloqueo es el motivo por el que no se guarda de vuelta; "" se puede.
	Bloqueo string `json:"bloqueo,omitempty"`
	// Aceptado: la persona ya aceptó que guardar en un formato ajeno es convertir.
	Aceptado bool `json:"aceptado,omitempty"`
	// Copia es la ruta de la copia del original que se hizo antes del primer guardado en
	// un formato ajeno ("" si no hacía falta o aún no se ha hecho).
	Copia string `json:"copia,omitempty"`
	// Copiado: ya no hace falta copiar el original (se hizo, o el fichero lo escribió esta
	// app con «Guardar como»).
	Copiado bool `json:"copiado,omitempty"`
	// Seq es la secuencia del documento cuando coincidía con el fichero.
	Seq int64 `json:"seq"`
}

// Abierto es lo que devuelve [Biblioteca.AbrirFichero].
type Abierto struct {
	Documento api.Document
	Fichero   Fichero
	// Nuevo: el documento se creó ahora.
	Nuevo bool
	// Recargado: ya existía, el fichero había cambiado por fuera y el documento no tenía
	// cambios propios, así que se ha puesto al día con el fichero (con una versión antes).
	Recargado bool
	// CambioFuera: ya existía, el fichero cambió por fuera Y el documento tiene cambios
	// que el fichero no tiene. Se abre el documento; guardar preguntará.
	CambioFuera bool
}

// FuenteDe es la `source_ref` de un fichero del disco: su URI `file://` absoluta.
func FuenteDe(ruta string) (string, error) {
	abs, err := filepath.Abs(ruta)
	if err != nil {
		return "", err
	}
	p := filepath.ToSlash(abs)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // C:/… en Windows
	}
	return (&url.URL{Scheme: "file", Path: p}).String(), nil
}

// RutaDe es la ruta de una URI `file://` (la de `api.Apertura.Local`); false si no es un
// fichero de este equipo (un `content://` de Android).
func RutaDe(uri string) (string, bool) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" || (u.Host != "" && u.Host != "localhost") {
		return "", false
	}
	p := u.Path
	if runtime.GOOS == "windows" {
		p = strings.TrimPrefix(p, "/")
	}
	return filepath.FromSlash(p), p != ""
}

func huella(datos []byte) string {
	s := sha256.Sum256(datos)
	return hex.EncodeToString(s[:])
}

// formatoDe es el formato con que se escribe un fichero de ese nombre para ese tipo: ""
// para el nativo, y error si no se sabe escribir.
func formatoDe(nombre string, kind api.DocKind) (model.BlobFormat, bool, error) {
	if k, ok := model.KindNativo(nombre); ok {
		if api.DocKind(k) != kind {
			return "", false, ErrOtroTipo
		}
		return "", true, nil
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(nombre), "."))
	switch {
	case ext == "docx" && kind == api.DocText, ext == "xlsx" && kind == api.DocSheet, ext == "pptx" && kind == api.DocSlides:
		return model.FormatOOXML, false, nil
	case ext == "odt" && kind == api.DocText, ext == "ods" && kind == api.DocSheet, ext == "odp" && kind == api.DocSlides:
		return model.FormatODF, false, nil
	case (ext == "md" || ext == "markdown") && kind == api.DocText:
		return model.FormatMarkdown, false, nil
	case (ext == "html" || ext == "htm") && kind == api.DocText:
		return model.FormatHTML, false, nil
	}
	return "", false, ErrFormatoSinVuelta
}

// leerFichero lee un fichero de la persona con el tope de importación.
func leerFichero(ruta string) ([]byte, error) {
	st, err := os.Stat(ruta)
	if err != nil {
		return nil, err
	}
	if st.Size() > api.MaxImportar {
		return nil, fmt.Errorf("biblioteca: %q supera los %d MiB que se importan", filepath.Base(ruta), api.MaxImportar>>20)
	}
	return os.ReadFile(ruta) //nolint:gosec // G304: es el fichero que la persona ha abierto
}

// contenidoDeFichero es lo que trae un fichero: el nativo, tal cual; el ajeno, importado.
type contenidoDeFichero struct {
	titulo      string
	contenido   string
	nativo      bool
	bloqueo     string
	comentarios []api.DocComment          // los del nativo, enteros
	importados  []api.ImportadoComentario // los del ajeno, para ImportComments
	imp         api.Importado
}

func (b *Biblioteca) leerContenido(ruta string, datos []byte, comentarios bool) (contenidoDeFichero, error) {
	nombre := filepath.Base(ruta)
	if _, ok := model.KindNativo(nombre); ok || model.EsNativo(datos) {
		n, err := model.LeerNativo(datos)
		if err != nil {
			return contenidoDeFichero{}, err
		}
		if api.DocKind(n.Kind) != b.tipo {
			return contenidoDeFichero{}, ErrOtroTipo
		}
		c := contenidoDeFichero{titulo: n.Titulo, contenido: n.Contenido, nativo: true}
		if c.titulo == "" {
			c.titulo = strings.TrimSuffix(nombre, filepath.Ext(nombre))
		}
		for _, m := range n.Comentarios {
			padre := ""
			if m.ParentID != nil {
				padre = *m.ParentID
			}
			c.comentarios = append(c.comentarios, api.DocComment{
				ID: m.ID, ParentID: padre, AuthorSub: m.AuthorSub, AuthorName: m.AuthorName,
				Body: m.Body, Resolved: m.Resolved, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, Anchor: m.Anchor,
			})
		}
		return c, nil
	}
	if c, ok := jsonDeDiapos(nombre, datos, b.tipo); ok {
		return c, nil
	}
	imp, err := Importar(nombre, datos, comentarios)
	if err != nil {
		return contenidoDeFichero{}, err
	}
	if imp.Kind != b.tipo {
		return contenidoDeFichero{}, ErrOtroTipo
	}
	c := contenidoDeFichero{titulo: imp.Title, contenido: imp.Content, importados: imp.Comments, imp: imp}
	switch _, _, errF := formatoDe(nombre, b.tipo); {
	case imp.Template:
		c.bloqueo = BloqueoPlantilla
	case imp.Truncated:
		c.bloqueo = BloqueoRecortado
	case errF != nil:
		c.bloqueo = BloqueoSinVuelta
	}
	return c, nil
}

// actorFichero es un actor nuevo para la operación que siembra o recarga el contenido.
func actorFichero() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "fichero-" + hex.EncodeToString(b)
}

// AbrirFichero trae a la biblioteca el fichero de `ruta` y devuelve su documento: el que
// ya estaba anclado a él, si lo hay, y si no uno nuevo. Con `comentarios`, los de un
// fichero ajeno se importan también (los de un nativo vienen siempre).
func (b *Biblioteca) AbrirFichero(ctx context.Context, ruta string, comentarios bool) (Abierto, error) {
	abs, err := filepath.Abs(ruta)
	if err != nil {
		return Abierto{}, err
	}
	fuente, err := FuenteDe(abs)
	if err != nil {
		return Abierto{}, err
	}
	b.ficheros.Lock()
	defer b.ficheros.Unlock()
	datos, err := leerFichero(abs)
	if err != nil {
		return Abierto{}, err
	}
	h := huella(datos)

	if doc, ok, err := b.DocumentBySource(ctx, fuente); err != nil {
		return Abierto{}, err
	} else if ok {
		return b.reabrir(ctx, doc.ID, abs, datos, h, comentarios)
	}

	c, err := b.leerContenido(abs, datos, comentarios)
	if err != nil {
		return Abierto{}, err
	}
	if c.bloqueo == BloqueoPlantilla {
		fuente = "" // una plantilla da un documento nuevo cada vez
	}
	b.mu.Lock()
	doc, err := b.nuevoCon(b.tipo, c.titulo, fuente, nil)
	if err == nil {
		b.docs[doc.ID].Comentarios = c.comentarios
	}
	b.mu.Unlock()
	if err != nil {
		return Abierto{}, err
	}
	contenido := c.contenido
	if len(c.importados) > 0 {
		anclados, errCom := b.ImportComments(ctx, doc.ID, c.importados)
		if errCom != nil {
			anclados = nil
		}
		contenido = c.imp.ContenidoConMarks(anclados)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if contenido != "" {
		actor := actorFichero()
		ops, err := opsDeRegistro(api.DocContentKey(b.tipo), contenido, api.DocDot{Actor: actor, Counter: 1}, 1, false)
		if err != nil {
			return Abierto{}, err
		}
		if _, err := b.aplicar(doc.ID, ops, actor, 0, false); err != nil {
			return Abierto{}, err
		}
	}
	r := b.docs[doc.ID]
	r.Fichero = &Fichero{Ruta: abs, Huella: h, Nativo: c.nativo, Bloqueo: c.bloqueo, Seq: r.Documento.Seq}
	if err := b.guardar(r); err != nil {
		return Abierto{}, err
	}
	return Abierto{Documento: r.Documento, Fichero: *r.Fichero, Nuevo: true}, nil
}

// reabrir es abrir un fichero que ya tiene documento. Con b.ficheros.
func (b *Biblioteca) reabrir(ctx context.Context, id, abs string, datos []byte, h string, comentarios bool) (Abierto, error) {
	b.mu.Lock()
	r, err := b.vivo(id)
	if err != nil {
		b.mu.Unlock()
		return Abierto{}, err
	}
	if r.Fichero == nil {
		r.Fichero = &Fichero{Ruta: abs, Huella: h, Seq: r.Documento.Seq}
		_, r.Fichero.Nativo = model.KindNativo(abs)
	}
	f := *r.Fichero
	doc := r.Documento
	b.mu.Unlock()
	if f.Huella == h {
		return Abierto{Documento: doc, Fichero: f}, nil
	}
	if doc.Seq != f.Seq {
		// Los dos han cambiado: se abre lo de la app y guardar preguntará.
		return Abierto{Documento: doc, Fichero: f, CambioFuera: true}, nil
	}
	// Solo cambió el fichero: se pone al día, con una versión de lo de antes.
	c, err := b.leerContenido(abs, datos, comentarios)
	if err != nil {
		return Abierto{}, err
	}
	if _, err := b.CreateDocumentVersion(ctx, id, "Antes de recargar el fichero"); err != nil {
		return Abierto{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	r, err = b.vivo(id)
	if err != nil {
		return Abierto{}, err
	}
	st, err := r.cargarEstado()
	if err != nil {
		return Abierto{}, err
	}
	lam := st.Lamport + 1
	actor := actorFichero()
	ops, err := opsDeRegistro(api.DocContentKey(b.tipo), c.contenido, api.DocDot{Actor: actor, Counter: lam}, lam, b.tipo == api.DocText)
	if err != nil {
		return Abierto{}, err
	}
	if _, err := b.aplicar(id, ops, actor, 0, false); err != nil {
		return Abierto{}, err
	}
	if c.nativo {
		r.Comentarios = c.comentarios
	}
	r.Fichero.Huella, r.Fichero.Seq, r.Fichero.Bloqueo = h, r.Documento.Seq, cmp.Or(c.bloqueo, r.Fichero.Bloqueo)
	if err := b.guardar(r); err != nil {
		return Abierto{}, err
	}
	return Abierto{Documento: r.Documento, Fichero: *r.Fichero, Recargado: true}, nil
}

// EstadoFichero es el ancla del documento id; false si no tiene.
func (b *Biblioteca) EstadoFichero(id string) (Fichero, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, err := b.vivo(id)
	if err != nil || r.Fichero == nil {
		return Fichero{}, false
	}
	return *r.Fichero, true
}

// AceptarConversion apunta que la persona sabe que guardar el documento id en su
// fichero de formato ajeno es convertirlo. Se pregunta una vez por documento.
func (b *Biblioteca) AceptarConversion(id string) error {
	return b.cambiar(id, func(r *registro) {
		if r.Fichero != nil {
			r.Fichero.Aceptado = true
		}
	})
}

// GuardarEnFichero escribe el documento id en el fichero al que está anclado. Sin
// `forzar`, si el fichero cambió por fuera devuelve ErrCambioFuera sin tocarlo.
func (b *Biblioteca) GuardarEnFichero(ctx context.Context, id string, forzar bool) (Fichero, error) {
	b.ficheros.Lock()
	defer b.ficheros.Unlock()
	f, ok := b.EstadoFichero(id)
	switch {
	case !ok:
		return Fichero{}, ErrSinFichero
	case f.Bloqueo != "":
		return f, fmt.Errorf("%w (%s)", ErrGuardadoBloqueado, f.Bloqueo)
	}
	formato, nativo, err := formatoDe(f.Ruta, b.tipo)
	if err != nil {
		return f, fmt.Errorf("%w: %w", ErrGuardadoBloqueado, err)
	}
	if !nativo && !f.Aceptado {
		return f, ErrConfirmarConversion
	}
	actual, errLeer := os.ReadFile(f.Ruta)
	if errLeer != nil && !errors.Is(errLeer, fs.ErrNotExist) {
		return f, errLeer
	}
	if errLeer == nil && huella(actual) != f.Huella && !forzar {
		return f, ErrCambioFuera
	}
	copia := ""
	if !nativo && !f.Copiado && errLeer == nil {
		if copia, err = guardarOriginal(f.Ruta, actual); err != nil {
			return f, err
		}
	}
	datos, seq, err := b.bytesDe(ctx, id, formato, nativo)
	if err != nil {
		return f, err
	}
	if err := escribirComo(f.Ruta, datos, f.Ruta); err != nil {
		return f, err
	}
	var out Fichero
	err = b.cambiar(id, func(r *registro) {
		if r.Fichero == nil {
			r.Fichero = &f
		}
		r.Fichero.Huella, r.Fichero.Seq = huella(datos), seq
		if !nativo {
			r.Fichero.Copiado = true
			if copia != "" {
				r.Fichero.Copia = copia
			}
		}
		out = *r.Fichero
	})
	return out, err
}

// guardarOriginal: la primera vez que se convierte encima de un fichero ajeno, su
// original se guarda al lado (`informe (original).docx`). Devuelve dónde.
func guardarOriginal(ruta string, actual []byte) (string, error) {
	copia, err := rutaLibre(ruta, "original")
	if err != nil {
		return "", err
	}
	return copia, escribirComo(copia, actual, ruta)
}

// GuardarComo escribe el documento id en `ruta` (nativo o convertido, por la extensión) y
// lo ancla a ese fichero: desde ahora, Ctrl/Cmd+S guarda ahí. Si otro documento estaba
// anclado al mismo fichero, se desancla: el fichero ya es de este.
func (b *Biblioteca) GuardarComo(ctx context.Context, id, ruta string) (Fichero, error) {
	abs, err := filepath.Abs(ruta)
	if err != nil {
		return Fichero{}, err
	}
	fuente, err := FuenteDe(abs)
	if err != nil {
		return Fichero{}, err
	}
	formato, nativo, err := formatoDe(abs, b.tipo)
	if err != nil {
		return Fichero{}, err
	}
	b.ficheros.Lock()
	defer b.ficheros.Unlock()
	datos, seq, err := b.bytesDe(ctx, id, formato, nativo)
	if err != nil {
		return Fichero{}, err
	}
	if err := escribirComo(abs, datos, abs); err != nil {
		return Fichero{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	r, err := b.vivo(id)
	if err != nil {
		return Fichero{}, err
	}
	for _, otro := range b.docs {
		if otro != r && otro.Documento.SourceRef == fuente {
			otro.Documento.SourceRef, otro.Fichero = "", nil
			if err := b.guardar(otro); err != nil {
				return Fichero{}, err
			}
		}
	}
	r.Documento.SourceRef = fuente
	r.Fichero = &Fichero{Ruta: abs, Huella: huella(datos), Nativo: nativo, Aceptado: true, Copiado: true, Seq: seq}
	if err := b.guardar(r); err != nil {
		return Fichero{}, err
	}
	return *r.Fichero, nil
}

// bytesDe son los bytes del documento id en ese formato (o el nativo) y la secuencia que
// llevan.
func (b *Biblioteca) bytesDe(ctx context.Context, id string, formato model.BlobFormat, nativo bool) ([]byte, int64, error) {
	if !nativo {
		b.mu.Lock()
		r, err := b.vivo(id)
		seq := int64(0)
		if err == nil {
			seq = r.Documento.Seq
		}
		b.mu.Unlock()
		if err != nil {
			return nil, 0, err
		}
		datos, err := b.ExportDocument(ctx, id, string(formato), false, nil)
		return datos, seq, err
	}
	b.mu.Lock()
	r, err := b.vivo(id)
	if err != nil {
		b.mu.Unlock()
		return nil, 0, err
	}
	st, err := r.cargarEstado()
	if err != nil {
		b.mu.Unlock()
		return nil, 0, err
	}
	n := model.Nativo{
		Kind: model.Kind(r.Documento.Kind), Titulo: r.Documento.Title, Contenido: st.Contenido(model.Kind(r.Documento.Kind)),
		Creado: r.Documento.CreatedAt, Modificado: r.Documento.UpdatedAt,
	}
	for _, c := range r.Comentarios {
		var padre *string
		if c.ParentID != "" {
			p := c.ParentID
			padre = &p
		}
		n.Comentarios = append(n.Comentarios, model.Comment{
			ID: c.ID, DocumentID: id, ParentID: padre, AuthorSub: c.AuthorSub, AuthorName: c.AuthorName,
			Anchor: c.Anchor, Body: c.Body, Resolved: c.Resolved, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		})
	}
	seq := r.Documento.Seq
	b.mu.Unlock()
	datos, err := model.EscribirNativo(n)
	return datos, seq, err
}

// rutaLibre es `nombre (etiqueta).ext` al lado de ruta, o `nombre (etiqueta 2).ext`… si
// ya existe.
func rutaLibre(ruta, etiqueta string) (string, error) {
	dir, base := filepath.Split(ruta)
	ext := filepath.Ext(base)
	nombre := strings.TrimSuffix(base, ext)
	for i := 1; i < 1000; i++ {
		e := etiqueta
		if i > 1 {
			e = fmt.Sprintf("%s %d", etiqueta, i)
		}
		p := filepath.Join(dir, nombre+" ("+e+")"+ext)
		if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
			return p, nil
		}
	}
	return "", fmt.Errorf("biblioteca: no hay nombre libre para la copia de %q", base)
}

// antesDeRenombrar es un gancho de las pruebas: corre con el temporal ya escrito y antes
// de ponerlo en su sitio, que es donde un corte haría más daño.
var antesDeRenombrar func(tmp string) error

// escribirComo escribe datos en ruta con temporal y renombrado, con los permisos que
// tenga `como` (el fichero que se sustituye) o 0644 si no existe: un fichero de la
// persona no pasa a ser 0600 por guardarlo.
func escribirComo(ruta string, datos []byte, como string) error {
	modo := fs.FileMode(0o644)
	if st, err := os.Stat(como); err == nil {
		modo = st.Mode().Perm()
	}
	dir := filepath.Dir(ruta)
	f, err := os.CreateTemp(dir, "."+filepath.Base(ruta)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op tras el renombrado
	if _, err := f.Write(datos); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, modo); err != nil {
		return err
	}
	if antesDeRenombrar != nil {
		if err := antesDeRenombrar(tmp); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, ruta); err != nil {
		return err
	}
	// La fecha de modificación es la de ahora aunque el sistema conserve la del temporal.
	ahora := time.Now()
	_ = os.Chtimes(ruta, ahora, ahora)
	return nil
}

// jsonDeDiapos es el formato de fichero ANTIGUO de Diapos (O-13): el JSON del modelo tal
// cual (`{"slides":[…]}`), el que `cmd/diapos` abre por la línea de órdenes. Se sigue
// leyendo, sin pérdida porque es el modelo, pero no se escribe: se guarda en `.diapos`
// con «Guardar como».
func jsonDeDiapos(nombre string, datos []byte, tipo api.DocKind) (contenidoDeFichero, bool) {
	if tipo != api.DocSlides || !strings.EqualFold(filepath.Ext(nombre), ".json") {
		return contenidoDeFichero{}, false
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(datos, &m) != nil || m["slides"] == nil {
		return contenidoDeFichero{}, false
	}
	return contenidoDeFichero{
		titulo: strings.TrimSuffix(nombre, filepath.Ext(nombre)), contenido: string(datos), bloqueo: BloqueoSinVuelta,
	}, true
}

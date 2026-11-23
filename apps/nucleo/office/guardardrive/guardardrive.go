// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package guardardrive es «Guardar en Drive» de Docus, Calcus y Diapos nativos, con
// las mismas reglas que la web (`packages/office/src/editor/state/driveCifrado.ts`,
// `anclaDrive.ts` y `pages/api/save-to-drive.ts`).
//
// EL ANCLA de un documento a su fichero (`source_ref`) es una de dos, y no se mezclan:
//
//   - «drive:<uuid>»: un fichero del Drive del SERVIDOR (lo pone el servicio al crear o
//     al abrir un fichero de Drive con Sytue Docs).
//   - «vault:<uuid>»: un fichero del Drive de la BÓVEDA post-cuántica (ADR 0086),
//     cifrado de extremo a extremo. Lo escribe el cliente y el servicio solo guarda el
//     ancla.
//
// DÓNDE SE GUARDA:
//
//   - Con el Drive en el servidor (fuente `legacy`) y un «drive:», en ese fichero, como
//     versión nueva y en el formato de su extensión (lo que hace el BFF de la web).
//   - Con el Drive en la bóveda (o un «vault:»), cifrado en ESTE dispositivo:
//     al fichero anclado si sigue vivo allí («drive:» de antes de pasar a la bóveda
//     conserva su id), como versión nueva ligada a la versión leída; si no, a un
//     fichero NUEVO en «Sytue Docs» de la raíz, en ODF, sin pisar ningún nombre. El
//     dueño ancla entonces el documento a él si podía (SePuedeAnclarEnBoveda).
//
// Lo que NO se hace nunca: mandar el fichero en claro al Drive del servidor cuando el
// Drive está en la bóveda (se mira ANTES de subir, sin claves), ni abrir la bóveda sin
// que la persona lo haya pedido (eso lo decide quien da `Destinos.Boveda`).
//
// Sin a medias: el documento se exporta ANTES de escribir nada; si la exportación o la
// subida fallan, no queda fichero ni ancla nuevos. Lo único que puede quedarse es la
// carpeta «Sytue Docs», que es la de siempre. Que el ancla no se pueda poner (otra
// ventana ancló antes, sin red) no deshace lo guardado: el fichero ya está en la
// bóveda, y el siguiente guardado lo intenta otra vez.
package guardardrive

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
)

const (
	// PrefijoDrive es el del ancla a un fichero del Drive del servidor.
	PrefijoDrive = "drive:"
	// PrefijoBoveda es el del ancla a un fichero del Drive de la bóveda.
	PrefijoBoveda = "vault:"
	// CarpetaDocumentos es la carpeta de la raíz donde van los ficheros nuevos: la
	// misma que en el Drive del servidor (`drive.CarpetaDocumentos` del servicio).
	CarpetaDocumentos = "Sytue Docs"
)

// Donde dice en qué Drive está el fichero de un ancla.
type Donde uint8

const (
	// EnServidor: «drive:<uuid>».
	EnServidor Donde = iota + 1
	// EnBoveda: «vault:<uuid>».
	EnBoveda
)

// Ancla es un `source_ref` reconocido.
type Ancla struct {
	Donde Donde
	ID    string
}

// uuid canónico (minúsculas): los ids de Drive, del servidor y de la bóveda.
var reUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// LeerAncla lee un `source_ref`; false si no ancla a ningún fichero reconocible.
func LeerAncla(ref string) (Ancla, bool) {
	if id, ok := strings.CutPrefix(ref, PrefijoDrive); ok && reUUID.MatchString(id) {
		return Ancla{Donde: EnServidor, ID: id}, true
	}
	if id, ok := strings.CutPrefix(ref, PrefijoBoveda); ok && reUUID.MatchString(id) {
		return Ancla{Donde: EnBoveda, ID: id}, true
	}
	return Ancla{}, false
}

// ErrAnclaNoValida: el id del fichero no es un uuid canónico.
var ErrAnclaNoValida = errors.New("guardardrive: vault_anchor_invalid")

// AnclaDeBoveda es el ancla de un fichero de la bóveda.
func AnclaDeBoveda(id string) (string, error) {
	if !reUUID.MatchString(id) {
		return "", ErrAnclaNoValida
	}
	return PrefijoBoveda + id, nil
}

// EsAnclaDeBoveda dice si `ref` es «vault:<uuid>».
func EsAnclaDeBoveda(ref string) bool {
	a, ok := LeerAncla(ref)
	return ok && a.Donde == EnBoveda
}

// SePuedeAnclarEnBoveda dice si el ancla `antes` se puede cambiar por una de la
// bóveda: solo si no había o si ya era de la bóveda. Un «drive:» no se sustituye (el
// servicio lo usa para la papelera de Drive). El servicio aplica la misma regla.
func SePuedeAnclarEnBoveda(antes string) bool {
	return antes == "" || EsAnclaDeBoveda(antes)
}

// FormatoPorExtension es la extensión del fichero → la familia de exportación de Docs
// (la tabla de `save-to-drive.ts`). Solo los formatos con ida y vuelta: un .pdf o un
// .txt no se reescriben, porque sería cambiarle el formato a espaldas de su dueño.
var FormatoPorExtension = map[string]string{
	"docx": "ooxml", "xlsx": "ooxml", "pptx": "ooxml",
	"odt": "odf", "ods": "odf", "odp": "odf",
	"md": "markdown", "markdown": "markdown",
	"html": "html", "htm": "html",
}

// tipoPorExtension es el tipo MIME con que se sube cada extensión: fijo, para no
// depender de la tabla del sistema.
var tipoPorExtension = map[string]string{
	"docx":     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"xlsx":     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"pptx":     "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	"odt":      "application/vnd.oasis.opendocument.text",
	"ods":      "application/vnd.oasis.opendocument.spreadsheet",
	"odp":      "application/vnd.oasis.opendocument.presentation",
	"md":       "text/markdown",
	"markdown": "text/markdown",
	"html":     "text/html",
	"htm":      "text/html",
}

func extension(nombre string) string {
	i := strings.LastIndexByte(nombre, '.')
	if i < 0 {
		return ""
	}
	return strings.ToLower(nombre[i+1:])
}

// FormatoDe es la familia con que se reescribe un fichero de ese nombre.
func FormatoDe(nombre string) (string, bool) {
	f, ok := FormatoPorExtension[extension(nombre)]
	return f, ok
}

var (
	// ErrFormatoSinVuelta: el fichero anclado no tiene ida y vuelta (.pdf, .txt).
	ErrFormatoSinVuelta = errors.New("guardardrive: unsupported_format")
	// ErrSinFichero: el documento no tiene fichero en Drive y su Drive está en el
	// servidor (o su ancla no es de Drive): no hay dónde guardar.
	ErrSinFichero = errors.New("guardardrive: not_from_drive")
	// ErrBovedaCerrada: el Drive está en la bóveda y este dispositivo aún no la ha
	// abierto. Quien da Destinos.Boveda ya ha ofrecido desbloquearla.
	ErrBovedaCerrada = errors.New("guardardrive: el Drive de la bóveda está cerrado")
	// ErrBovedaPendiente: el Drive aún no ha terminado de pasar a la bóveda.
	ErrBovedaPendiente = errors.New("guardardrive: el Drive aún está pasando a la bóveda")
	// ErrBovedaNoDisponible: este dispositivo no puede abrir el Drive de la bóveda.
	ErrBovedaNoDisponible = errors.New("guardardrive: este dispositivo no puede abrir el Drive de la bóveda")
)

// Repositorio es lo que guardar usa de un Drive: el de la bóveda (drive.Repository)
// o el del servidor (api.Drive). ReplaceFile escribe una versión nueva ligada a la
// versión leída: en la bóveda, si otro dispositivo publicó entretanto, falla en vez de
// pisarla.
type Repositorio interface {
	Root(ctx context.Context) (api.Folder, error)
	Children(ctx context.Context, carpeta, cursor string) (api.Children, error)
	CreateFolder(ctx context.Context, padre, nombre string) (api.Folder, error)
	Upload(ctx context.Context, carpeta, nombre, tipo string, cuerpo io.Reader, tamano int64) (api.File, error)
	File(ctx context.Context, id string) (api.File, error)
	ReplaceFile(ctx context.Context, original api.File, tipo string, cuerpo io.Reader, tamano int64) (api.File, error)
}

var (
	_ Repositorio = (*privateDrive.Repository)(nil)
	_ Repositorio = (*api.Drive)(nil)
)

// Exportar devuelve los bytes del documento en esa familia («ooxml», «odf»…). El
// servicio serializa desde SU estado: quien llama guarda lo pendiente antes.
type Exportar func(ctx context.Context, formato string) ([]byte, error)

// Guardado es lo que se escribió.
type Guardado struct {
	// Nombre con el que quedó el fichero.
	Nombre string
	// Ancla del fichero escrito: la que ya tenía el documento si era el anclado.
	Ancla string
	// Nuevo: un fichero nuevo, y no una versión nueva del anclado.
	Nuevo bool
	// Cifrado: escrito en la bóveda, cifrado en este dispositivo.
	Cifrado bool
}

// noEsta dice si el error es que el fichero ya no está (o está en la papelera).
func noEsta(err error) bool {
	var e *api.Error
	return errors.Is(err, privateDrive.ErrNotFound) || errors.As(err, &e) && e.Status == http.StatusNotFound
}

// enFichero escribe una versión nueva de `f` en el formato de su extensión.
func enFichero(ctx context.Context, repo Repositorio, f api.File, exportar Exportar) (api.File, error) {
	formato, ok := FormatoDe(f.Name)
	if !ok || f.FolderID == "" {
		return api.File{}, ErrFormatoSinVuelta
	}
	datos, err := exportar(ctx, formato)
	if err != nil {
		return api.File{}, err
	}
	tipo := f.ContentType
	if tipo == "" {
		tipo = tipoPorExtension[extension(f.Name)]
	}
	return repo.ReplaceFile(ctx, f, tipo, bytes.NewReader(datos), int64(len(datos)))
}

// EnElServidor guarda en el fichero «drive:» del Drive del servidor (el BFF
// `save-to-drive.ts` de la web). Solo con el Drive en el servidor: quien llama lo
// comprueba antes, para no mandarle el fichero en claro a un Drive en la bóveda.
func EnElServidor(ctx context.Context, repo Repositorio, ancla string, exportar Exportar) (Guardado, error) {
	a, ok := LeerAncla(ancla)
	if !ok || a.Donde != EnServidor || repo == nil {
		return Guardado{}, ErrSinFichero
	}
	f, err := repo.File(ctx, a.ID)
	if err != nil {
		return Guardado{}, err
	}
	escrito, err := enFichero(ctx, repo, f, exportar)
	if err != nil {
		return Guardado{}, err
	}
	return Guardado{Nombre: nombreDe(escrito, f.Name), Ancla: ancla}, nil
}

func nombreDe(f api.File, reserva string) string {
	if f.Name != "" {
		return f.Name
	}
	return reserva
}

// EnLaBoveda escribe el documento en el Drive de la bóveda: al fichero anclado si
// sigue vivo allí, o a uno nuevo en «Sytue Docs» (ver el comentario del paquete).
func EnLaBoveda(ctx context.Context, repo Repositorio, ancla, titulo string, kind api.DocKind, exportar Exportar) (Guardado, error) {
	if repo == nil {
		return Guardado{}, ErrBovedaCerrada
	}
	if a, ok := LeerAncla(ancla); ok {
		f, err := repo.File(ctx, a.ID)
		switch {
		case err == nil:
			escrito, err := enFichero(ctx, repo, f, exportar)
			if err != nil {
				return Guardado{}, err
			}
			return Guardado{Nombre: nombreDe(escrito, f.Name), Ancla: ancla, Cifrado: true}, nil
		case !noEsta(err):
			return Guardado{}, err
		}
		// Ya no está (o está en la papelera): a uno nuevo, como la web.
	}
	// Primero los bytes: si exportar falla, no se ha escrito nada.
	datos, err := exportar(ctx, "odf")
	if err != nil {
		return Guardado{}, err
	}
	carpeta, err := carpetaDocumentos(ctx, repo)
	if err != nil {
		return Guardado{}, err
	}
	ocupados, err := nombresEn(ctx, repo, carpeta)
	if err != nil {
		return Guardado{}, err
	}
	nombre := NombreLibre(api.ExportFilename(titulo, kind, "odf"), ocupados)
	f, err := repo.Upload(ctx, carpeta, nombre, tipoPorExtension[extension(nombre)], bytes.NewReader(datos), int64(len(datos)))
	if err != nil {
		return Guardado{}, err
	}
	nueva, err := AnclaDeBoveda(f.ID)
	if err != nil {
		return Guardado{}, err
	}
	return Guardado{Nombre: nombreDe(f, nombre), Ancla: nueva, Nuevo: true, Cifrado: true}, nil
}

// carpetaDocumentos es el id de «Sytue Docs» en la raíz, creándola si no está.
func carpetaDocumentos(ctx context.Context, repo Repositorio) (string, error) {
	raiz, err := repo.Root(ctx)
	if err != nil {
		return "", err
	}
	cursor := ""
	for {
		hijos, err := repo.Children(ctx, raiz.ID, cursor)
		if err != nil {
			return "", err
		}
		for _, c := range hijos.Folders {
			if c.Name == CarpetaDocumentos && c.DeletedAt == nil {
				return c.ID, nil
			}
		}
		if hijos.NextCursor == "" || hijos.NextCursor == cursor {
			break
		}
		cursor = hijos.NextCursor
	}
	c, err := repo.CreateFolder(ctx, raiz.ID, CarpetaDocumentos)
	if err != nil {
		return "", err
	}
	return c.ID, nil
}

// nombresEn son todos los nombres de la carpeta (ficheros y carpetas).
func nombresEn(ctx context.Context, repo Repositorio, carpeta string) (map[string]bool, error) {
	ocupados := map[string]bool{}
	cursor := ""
	for {
		hijos, err := repo.Children(ctx, carpeta, cursor)
		if err != nil {
			return nil, err
		}
		for _, c := range hijos.Folders {
			ocupados[c.Name] = true
		}
		for _, f := range hijos.Files {
			ocupados[f.Name] = true
		}
		if hijos.NextCursor == "" || hijos.NextCursor == cursor {
			return ocupados, nil
		}
		cursor = hijos.NextCursor
	}
}

// NombreLibre es `freePrivateDriveName` de la web: «Informe.odt», «Informe (2).odt»…
// Exacto, como la bóveda compara los nombres: subir con uno que ya está sería una
// versión nueva de OTRO fichero.
func NombreLibre(nombre string, ocupados map[string]bool) string {
	if !ocupados[nombre] {
		return nombre
	}
	base, ext := nombre, ""
	if i := strings.LastIndexByte(nombre, '.'); i > 0 {
		base, ext = nombre[:i], nombre[i:]
	}
	for n := 2; ; n++ {
		candidato := base + " (" + strconv.Itoa(n) + ")" + ext
		if !ocupados[candidato] {
			return candidato
		}
	}
}

// Documento es lo que guardar necesita saber del documento abierto.
type Documento struct {
	// Ancla es su `source_ref` («drive:», «vault:» o "").
	Ancla string
	// EsDueno: solo el dueño ancla (el servicio también lo exige).
	EsDueno bool
	Titulo  string
	Kind    api.DocKind
}

// Destinos son los dos Drive y el servicio de Docs.
type Destinos struct {
	// DriveEnBoveda dice, SIN claves, si el Drive del servidor de quien edita ya no
	// admite escrituras (su fuente no está en `legacy`).
	DriveEnBoveda func(context.Context) (bool, error)
	// Servidor es el Drive del servidor, para un «drive:» con el Drive allí.
	Servidor Repositorio
	// Boveda es el Drive de la bóveda, abierto. Si no lo está, devuelve
	// ErrBovedaCerrada (tras ofrecer desbloquearla), ErrBovedaPendiente o
	// ErrBovedaNoDisponible.
	Boveda func(context.Context) (Repositorio, error)
	// Anclar pone `nueva` como ancla del documento si sigue siendo `antes`.
	Anclar func(ctx context.Context, nueva, antes string) error
}

// Resultado es Guardado más el ancla que el documento tiene ahora.
type Resultado struct {
	Guardado
	AnclaDelDocumento string
}

// Ofrecer dice si «Guardar en Drive» tiene sentido: con un ancla, o con el Drive en
// la bóveda (el primer guardado crea el fichero y ancla el documento a él).
func Ofrecer(ancla string, driveEnBoveda bool) bool { return ancla != "" || driveEnBoveda }

// GuardarDocumento es todo «Guardar en Drive» (`saveToDrive` de `useDocument.ts`): con
// un «drive:» y el Drive en el servidor, en ese fichero; si no, cifrado en la bóveda,
// y si el fichero es nuevo y quien guarda es el dueño, ancla el documento a él.
func GuardarDocumento(ctx context.Context, d Destinos, doc Documento, exportar Exportar) (Resultado, error) {
	if d.DriveEnBoveda == nil || d.Boveda == nil || exportar == nil {
		return Resultado{}, ErrSinFichero
	}
	// Ante la duda (sin red, la fuente no contesta) NO es la bóveda: entonces el
	// Drive del servidor, si lo intenta, contesta 409 y se va a la bóveda.
	enBoveda, err := d.DriveEnBoveda(ctx)
	if err != nil {
		enBoveda = false
	}
	if doc.Ancla != "" && !EsAnclaDeBoveda(doc.Ancla) && !enBoveda {
		g, err := EnElServidor(ctx, d.Servidor, doc.Ancla, exportar)
		if err == nil || !api.EsUnidadCifrada(err) {
			return Resultado{Guardado: g, AnclaDelDocumento: doc.Ancla}, err
		}
		// El Drive pasó a la bóveda entretanto: como la web, a la bóveda.
		enBoveda = true
	}
	if doc.Ancla == "" && !enBoveda {
		return Resultado{}, ErrSinFichero
	}
	repo, err := d.Boveda(ctx)
	if err != nil {
		return Resultado{}, err
	}
	g, err := EnLaBoveda(ctx, repo, doc.Ancla, doc.Titulo, doc.Kind, exportar)
	if err != nil {
		return Resultado{}, err
	}
	r := Resultado{Guardado: g, AnclaDelDocumento: doc.Ancla}
	if g.Nuevo && doc.EsDueno && g.Ancla != doc.Ancla && SePuedeAnclarEnBoveda(doc.Ancla) && d.Anclar != nil {
		if d.Anclar(ctx, g.Ancla, doc.Ancla) == nil {
			r.AnclaDelDocumento = g.Ancla
		}
	}
	return r, nil
}

// Motivo es la clave del catálogo de la ofimática con que se dice un fallo.
func Motivo(err error) string {
	switch {
	case errors.Is(err, ErrBovedaCerrada):
		return "doc.msg.saveToDriveVaultLocked"
	case errors.Is(err, ErrBovedaPendiente):
		return "doc.msg.saveToDriveVaultNotReady"
	case errors.Is(err, ErrFormatoSinVuelta):
		return "doc.msg.saveToDriveUnsupported"
	case errors.Is(err, ErrBovedaNoDisponible), api.EsUnidadCifrada(err):
		return "doc.msg.saveToDriveVault"
	default:
		return "doc.msg.saveToDriveFailed"
	}
}

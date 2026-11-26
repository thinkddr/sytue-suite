// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"cmp"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Drive es el cliente de Sytue Drive. La API del servicio va por IDENTIFICADOR de
// carpeta y de fichero; el cliente de la web (packages/api-clients/src/drive) le monta
// encima una capa de rutas («/Contabilidad/2026») porque la web navega por URL. Una app
// nativa navega por carpetas: aquí se porta el núcleo por id y no la capa de rutas.
type Drive struct {
	c    *Client
	data DriveData
	// avisar se llama tras cada escritura con éxito en los datos privados (ConAviso): la
	// de HTTP ya la ve el transporte (ConAvisoDeEscritura), la del repositorio cifrado no.
	avisar func()
}

// NewDrive crea el cliente sobre la raíz del servicio (ServiceURL(apex, "drive")).
func NewDrive(servicio string, s Session, h *http.Client) *Drive {
	return &Drive{c: New(servicio+"/api/v1", s, h)}
}

// Folder es una carpeta.
type Folder struct {
	ID       string  `json:"id"`
	ParentID *string `json:"parent_id,omitempty"`
	Name     string  `json:"name"`
	IsRoot   bool    `json:"is_root"`
	IsGroup  bool    `json:"is_group"`
	// OwnerSub es el `sub` de quien la creó; CreatedAt, cuándo. Los pinta «Detalles».
	OwnerSub  string    `json:"owner_sub,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// DeletedAt solo viene en la papelera.
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	// Path solo viene en lo que CRUZA el árbol —una búsqueda—, donde sin ruta no se
	// sabría de dónde es cada resultado.
	Path string `json:"path,omitempty"`
}

// File es un fichero.
type File struct {
	ID          string `json:"id"`
	FolderID    string `json:"folder_id"`
	Name        string `json:"name"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentType string `json:"content_type"`
	Version     int    `json:"version"`
	// OwnerSub es quien lo subió por primera vez y ModifiedBy quien subió el contenido
	// ACTUAL (ambos `sub`); CreatedAt, cuándo nació. Son lo que enseña «Detalles».
	OwnerSub   string    `json:"owner_sub,omitempty"`
	ModifiedBy string    `json:"modified_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	// Checksum es el SHA-256 en hexadecimal del contenido, que el servicio calcula
	// al subir. Es con lo que `GuardarAdjunto` sabe que un adjunto ya está guardado
	// y no lo guarda dos veces.
	Checksum string `json:"checksum,omitempty"`
	// DeletedAt y Path: como en Folder.
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	Path      string     `json:"path,omitempty"`
	// Label es la etiqueta de sensibilidad, si lleva alguna.
	Label *DriveLabel `json:"label,omitempty"`
}

// DriveLabel es lo que de una etiqueta de sensibilidad le importa a quien comparte.
type DriveLabel struct {
	Name string `json:"name"`
	// BlockExternalShare: el servicio rechazará (403) cualquier enlace público.
	BlockExternalShare bool `json:"block_external_share"`
}

// Children es una página del contenido de una carpeta.
type Children struct {
	Folders    []Folder `json:"folders"`
	Files      []File   `json:"files"`
	NextCursor string   `json:"next_cursor,omitempty"`
}

// Root es la carpeta raíz de la persona («Mi unidad»).
func (d *Drive) Root(ctx context.Context) (Folder, error) {
	if d.data != nil {
		return d.data.Root(ctx)
	}
	return JSON[Folder](ctx, d.c, Request{Path: "/folders/root"})
}

// Children lista una página de una carpeta en el orden de siempre. cursor vacío es la
// primera.
func (d *Drive) Children(ctx context.Context, carpeta, cursor string) (Children, error) {
	return d.ChildrenEn(ctx, carpeta, cursor, OrdenListado{})
}

// OrdenListado es el `?orden=&sentido=` de un listado de carpeta: Campo es `nombre`,
// `modificado` o `tamano`, y Sentido `asc` o `desc`. El cero es el orden de siempre
// (nombre ascendente) y no añade parámetros, así que la URL —y con ella la caché sin
// conexión— es la de antes.
type OrdenListado struct {
	Campo, Sentido string
}

// ordenar aplica el orden a una página ya en memoria: carpetas y ficheros por separado,
// como el servicio, con el nombre de desempate.
func (o OrdenListado) ordenar(ch *Children) {
	if o.Campo == "" && o.Sentido == "" {
		return
	}
	desc := o.Sentido == "desc"
	compara := func(na, nb string, ta, tb time.Time, sa, sb int64) int {
		r := 0
		switch o.Campo {
		case "modificado":
			r = ta.Compare(tb)
		case "tamano":
			r = cmp.Compare(sa, sb)
		}
		if r == 0 {
			r = strings.Compare(strings.ToLower(na), strings.ToLower(nb))
		}
		if desc {
			r = -r
		}
		return r
	}
	slices.SortStableFunc(ch.Folders, func(a, b Folder) int {
		return compara(a.Name, b.Name, a.UpdatedAt, b.UpdatedAt, 0, 0)
	})
	slices.SortStableFunc(ch.Files, func(a, b File) int {
		return compara(a.Name, b.Name, a.UpdatedAt, b.UpdatedAt, a.SizeBytes, b.SizeBytes)
	})
}

func (o OrdenListado) poner(q url.Values) {
	if o.Campo != "" {
		q.Set("orden", o.Campo)
	}
	if o.Sentido != "" {
		q.Set("sentido", o.Sentido)
	}
}

// ChildrenEn lista una página de una carpeta en el orden pedido, que aplica el servicio
// con keyset por columna. El cursor lleva su orden: la página siguiente se pide en el
// MISMO, o el servicio contesta 400.
//
// En el almacenamiento privado los nombres van cifrados y el servicio no puede
// ordenarlos: la página se ordena aquí, ya descifrada.
func (d *Drive) ChildrenEn(ctx context.Context, carpeta, cursor string, o OrdenListado) (Children, error) {
	if d.data != nil {
		ch, err := d.data.Children(ctx, carpeta, cursor)
		if err == nil {
			o.ordenar(&ch)
		}
		return ch, err
	}
	q := url.Values{}
	o.poner(q)
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	return JSON[Children](ctx, d.c, Request{Path: "/folders/" + url.PathEscape(carpeta) + "/children", Query: q})
}

// CreateFolder crea una carpeta dentro de otra.
func (d *Drive) CreateFolder(ctx context.Context, padre, nombre string) (Folder, error) {
	if d.data != nil {
		return conAviso[Folder](d)(d.data.CreateFolder(ctx, padre, nombre))
	}
	return PostJSON[Folder](ctx, d.c, "/folders", map[string]string{"parent_id": padre, "name": nombre})
}

// ExportBasesView deja en la carpeta un CSV con la vista de Sytue Bases que abarca el
// token de una conexión de solo lectura concedida a Sytue Drive, y devuelve el fichero.
// Es `importBasesView` del cliente de la web. El token va en el cuerpo, nunca en la URL,
// y el servicio no lo guarda: refrescar la vista es volver a exportarla. Si la conexión
// no vale (revocada, caducada, de otro producto u otra organización) contesta 422 con
// el código CodigoBasesConexionInvalida; si no cabe, 507.
func (d *Drive) ExportBasesView(ctx context.Context, carpeta, token string) (File, error) {
	r, err := PostJSON[struct {
		File File `json:"file"`
	}](ctx, d.c, "/folders/"+url.PathEscape(carpeta)+"/imports/bases", map[string]string{"token": token})
	return r.File, err
}

// CodigoBasesConexionInvalida es el `code` del 422 de ExportBasesView.
const CodigoBasesConexionInvalida = "bases_connection_invalid"

// Download abre el contenido de un fichero. Cerrarlo es cosa de quien llama.
func (d *Drive) Download(ctx context.Context, fichero string) (io.ReadCloser, error) {
	if d.data != nil {
		return d.data.Download(ctx, fichero)
	}
	resp, err := d.c.Do(ctx, Request{Path: "/files/" + url.PathEscape(fichero) + "/content"})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// Thumbnail abre la miniatura de una imagen (512 px de lado como mucho, JPEG o PNG). El
// servicio solo las hace de JPEG, PNG y GIF; de lo demás contesta 415.
func (d *Drive) Thumbnail(ctx context.Context, fichero string) (io.ReadCloser, error) {
	if d.data != nil {
		return d.data.Thumbnail(ctx, fichero)
	}
	resp, err := d.c.Do(ctx, Request{Path: "/files/" + url.PathEscape(fichero) + "/thumbnail"})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// Upload sube un fichero a una carpeta, sin pasarlo por memoria. El nombre va en la
// query y no en `X-File-Name`: una cabecera no lleva bien «Factura nº 3 — señal.pdf».
// Si ya hay un fichero con ese nombre en la carpeta, lo SUSTITUYE (una versión nueva).
func (d *Drive) Upload(ctx context.Context, carpeta, nombre, tipo string, cuerpo io.Reader, tamano int64) (File, error) {
	if d.data != nil {
		return conAviso[File](d)(d.data.Upload(ctx, carpeta, nombre, tipo, cuerpo, tamano))
	}
	return d.subir(ctx, carpeta, url.Values{"name": {nombre}}, tipo, cuerpo, tamano)
}

// UploadConservando es Upload sin sustituir nada: con un fichero del mismo nombre en la
// carpeta, este se guarda como «nombre (2).ext» (`on_conflict=keep_both`) y el File que
// vuelve dice cuál. Es la de las fotos, donde «IMG_0001.jpg» se repite en cada teléfono.
// Un Drive anterior a la opción la ignora y sustituye: quien no pueda permitírselo
// sigue eligiendo un nombre libre antes de subir.
func (d *Drive) UploadConservando(ctx context.Context, carpeta, nombre, tipo string, cuerpo io.Reader, tamano int64) (File, error) {
	if d.data != nil {
		return conAviso[File](d)(d.data.Upload(ctx, carpeta, nombre, tipo, cuerpo, tamano))
	}
	return d.subir(ctx, carpeta, url.Values{"name": {nombre}, "on_conflict": {"keep_both"}}, tipo, cuerpo, tamano)
}

func (d *Drive) subir(ctx context.Context, carpeta string, q url.Values, tipo string, cuerpo io.Reader, tamano int64) (File, error) {
	if tipo == "" {
		tipo = "application/octet-stream"
	}
	resp, err := d.c.Stream(ctx, http.MethodPost, "/folders/"+url.PathEscape(carpeta)+"/files",
		q, http.Header{"Content-Type": {tipo}}, cuerpo, tamano)
	if err != nil {
		return File{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var f File
	err = json.NewDecoder(resp.Body).Decode(&f)
	return f, err
}

// DriveKind dice si un elemento es un fichero o una carpeta. Es el segmento de su ruta
// en la API: renombrar, mover, borrar y restaurar son la misma petición para los dos.
type DriveKind string

// Las dos clases de elemento.
const (
	DriveFile   DriveKind = "files"
	DriveFolder DriveKind = "folders"
)

// mandar ejecuta una escritura de la que solo importa si ha salido bien.
func (d *Drive) mandar(ctx context.Context, metodo, ruta string, cuerpo any) error {
	r := Request{Method: metodo, Path: ruta}
	if cuerpo != nil {
		datos, err := json.Marshal(cuerpo)
		if err != nil {
			return err
		}
		r.Body, r.Header = datos, http.Header{"Content-Type": {"application/json"}}
	}
	resp, err := d.c.Do(ctx, r)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func elemento(k DriveKind, id, resto string) string {
	return "/" + string(k) + "/" + url.PathEscape(id) + resto
}

// Rename cambia el nombre. El servicio contesta 409 si ya hay otro igual en su carpeta.
func (d *Drive) Rename(ctx context.Context, k DriveKind, id, nombre string) error {
	if d.data != nil {
		return d.avisado(d.data.Rename(ctx, k, id, nombre))
	}
	return d.mandar(ctx, http.MethodPatch, elemento(k, id, ""), map[string]string{"name": nombre})
}

// Move lleva el elemento a otra carpeta. El PATCH es el de Rename, y el servicio exige
// que lleve UNA de las dos cosas: nombre o destino.
func (d *Drive) Move(ctx context.Context, k DriveKind, id, destino string) error {
	if d.data != nil {
		return d.avisado(d.data.Move(ctx, k, id, destino))
	}
	campo := "new_folder_id"
	if k == DriveFolder {
		campo = "new_parent_id"
	}
	return d.mandar(ctx, http.MethodPatch, elemento(k, id, ""), map[string]string{campo: destino})
}

// Delete manda el elemento a la papelera; una carpeta, con todo lo que lleva dentro.
func (d *Drive) Delete(ctx context.Context, k DriveKind, id string) error {
	if d.data != nil {
		return d.avisado(d.data.Delete(ctx, k, id))
	}
	return d.mandar(ctx, http.MethodDelete, elemento(k, id, ""), nil)
}

// Restore lo devuelve de la papelera a donde estaba.
func (d *Drive) Restore(ctx context.Context, k DriveKind, id string) error {
	if d.data != nil {
		return d.avisado(d.data.Restore(ctx, k, id))
	}
	return d.mandar(ctx, http.MethodPost, elemento(k, id, "/restore"), nil)
}

// Purge lo borra de verdad: contenido e historial de versiones. No tiene vuelta atrás.
func (d *Drive) Purge(ctx context.Context, k DriveKind, id string) error {
	if x, ok := d.data.(DrivePurger); ok {
		return x.Purge(ctx, k, id)
	}
	return d.mandar(ctx, http.MethodDelete, elemento(k, id, "/purge"), nil)
}

// CopyFile hace una copia junto al original. No mueve bytes: el servicio apunta la
// copia al mismo contenido, y el nombre («Copia de …») lo pone él.
func (d *Drive) CopyFile(ctx context.Context, id string) (File, error) {
	if d.data != nil {
		return conAviso[File](d)(d.data.CopyFile(ctx, id))
	}
	return JSON[File](ctx, d.c, Request{Method: http.MethodPost, Path: elemento(DriveFile, id, "/copy")})
}

// CopyFolder copia una carpeta con todo su subárbol junto a la original. Tampoco mueve
// bytes: cada fichero de la copia apunta al contenido del suyo.
func (d *Drive) CopyFolder(ctx context.Context, id string) (Folder, error) {
	if x, ok := d.data.(DriveFolderCopier); ok {
		return x.CopyFolder(ctx, id)
	}
	return JSON[Folder](ctx, d.c, Request{Method: http.MethodPost, Path: elemento(DriveFolder, id, "/copy")})
}

// DriveTrash es la papelera: lo borrado de primer nivel y cuánto lo guarda el servicio
// antes de purgarlo solo (0: no lo purga).
type DriveTrash struct {
	Children
	RetentionSeconds int64 `json:"retention_seconds"`
}

// Trash lista la papelera.
func (d *Drive) Trash(ctx context.Context) (DriveTrash, error) {
	if d.data != nil {
		return d.data.Trash(ctx)
	}
	return JSON[DriveTrash](ctx, d.c, Request{Path: "/trash"})
}

// Compartidos es la vista plana —fichero o carpeta, con su ruta— con la que el servicio
// contesta a lo que CRUZA el árbol (la búsqueda, lo reciente, los favoritos) y a lo que
// hay dentro de un share. Es la misma forma en los cuatro sitios; lo que cambia es que
// en un share vienen además el permiso y quién compartió.
type Compartidos []Compartido

// Hijos reparte la lista en lo que el resto del cliente ya entiende. `Path` se conserva
// tal cual lo dio el servicio: absoluto en una búsqueda y RELATIVO a la raíz dentro de
// un share, que es como allí se nombra cada elemento.
func (c Compartidos) Hijos() Children {
	var out Children
	for _, x := range c {
		if x.IsDir {
			out.Folders = append(out.Folders, Folder{ID: x.ID, Name: x.Name, UpdatedAt: x.UpdatedAt, Path: x.Path})
		} else {
			out.Files = append(out.Files, x.File)
		}
	}
	return out
}

// Search busca por nombre en TODA la unidad. Va por QUERY: lo que alguien busca no
// tiene por qué quedarse en el log de acceso de cada proxy del camino.
func (d *Drive) Search(ctx context.Context, texto string) (Children, error) {
	if d.data != nil {
		return d.data.Search(ctx, texto)
	}
	r, err := Consulta[struct {
		Files Compartidos `json:"files"` // la clave es "files" aunque sean nodos: la comparte con el Hub
	}](ctx, d.c, Request{Path: "/search", Query: url.Values{"q": {texto}}})
	return r.Files.Hijos(), err
}

// Recent es una página de lo subido o cambiado hace poco, lo más nuevo primero, en el
// ORDEN del servicio. `cursor` vacío es la primera; el de la siguiente vuelve aparte,
// vacío si no hay más.
func (d *Drive) Recent(ctx context.Context, cursor string) (Compartidos, string, error) {
	if d.data != nil {
		return d.data.Recent(ctx, cursor)
	}
	q := url.Values{}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	r, err := JSON[struct {
		Items      Compartidos `json:"items"`
		NextCursor string      `json:"next_cursor"`
	}](ctx, d.c, Request{Path: "/recent", Query: q})
	return r.Items, r.NextCursor, err
}

// Favorites son los ficheros y carpetas que la persona ha marcado con la estrella, lo
// último marcado primero. Van MEZCLADOS y en ese orden: repartirlos en carpetas y
// ficheros, como `Hijos`, cambiaría lo que la web enseña.
func (d *Drive) Favorites(ctx context.Context) (Compartidos, error) {
	if d.data != nil {
		return d.data.Favorites(ctx)
	}
	r, err := JSON[struct {
		Items Compartidos `json:"items"`
	}](ctx, d.c, Request{Path: "/favorites"})
	return r.Items, err
}

// SetFavorite pone o quita la estrella. Es de la persona, no del fichero: nadie más la ve.
func (d *Drive) SetFavorite(ctx context.Context, k DriveKind, id string, favorito bool) error {
	if d.data != nil {
		return d.avisado(d.data.SetFavorite(ctx, k, id, favorito))
	}
	campo := "file_id"
	if k == DriveFolder {
		campo = "folder_id"
	}
	return d.mandar(ctx, http.MethodPost, "/favorites", map[string]any{campo: id, "favorite": favorito})
}

// File trae los datos de un fichero por su identificador, con su ruta si el servicio la
// resuelve. Es el `statById` de la web: lo que se sabe de un fichero cuando lo único
// que llegó fue su id, como en un enlace de apertura.
func (d *Drive) File(ctx context.Context, id string) (File, error) {
	if d.data != nil {
		return d.data.File(ctx, id)
	}
	return JSON[File](ctx, d.c, Request{Path: elemento(DriveFile, id, "")})
}

// Folder trae una carpeta por su identificador. Sirve para saber de quién cuelga: un
// favorito llega sin su camino, y se sube de padre en padre hasta la raíz.
func (d *Drive) Folder(ctx context.Context, id string) (Folder, error) {
	if d.data != nil {
		return d.data.Folder(ctx, id)
	}
	return JSON[Folder](ctx, d.c, Request{Path: elemento(DriveFolder, id, "")})
}

// DriveExternalCapacity conserva los valores desconocidos del proveedor como nil.
type DriveExternalCapacity struct {
	TotalBytes     *int64 `json:"total_bytes"`
	UsedBytes      *int64 `json:"used_bytes"`
	AvailableBytes *int64 `json:"available_bytes"`
	Partial        bool   `json:"partial,omitempty"`
}

// Complete dice si se puede mostrar una cifra de ampliación, también si vale cero.
func (c *DriveExternalCapacity) Complete() bool {
	return c != nil && c.TotalBytes != nil && *c.TotalBytes >= 0 &&
		c.UsedBytes != nil && *c.UsedBytes >= 0 &&
		c.AvailableBytes != nil && *c.AvailableBytes >= 0
}

// DriveQuota trae los totales del servicio, ampliados con las fuentes externas
// que informan su capacidad completa. La cuota base de Sytue se conserva aparte.
type DriveQuota struct {
	LimitBytes       int64                  `json:"limit_bytes"`
	UsedBytes        int64                  `json:"used_bytes"`
	AvailableBytes   int64                  `json:"available_bytes"`
	BaseLimitBytes   *int64                 `json:"base_limit_bytes,omitempty"`
	BaseUsedBytes    *int64                 `json:"base_used_bytes,omitempty"`
	ExternalStorage  bool                   `json:"external_storage"`
	ExternalCapacity *DriveExternalCapacity `json:"external_capacity,omitempty"`
}

// Quota dice cuánto espacio hay y cuánto se ha gastado.
func (d *Drive) Quota(ctx context.Context) (DriveQuota, error) {
	if x, ok := d.data.(DriveQuotaReader); ok {
		return x.Quota(ctx)
	}
	return JSON[DriveQuota](ctx, d.c, Request{Path: "/quota"})
}

// DriveShare es una compartición viva. Un enlace NO trae su dirección: el servicio la
// da una sola vez, al crearlo.
type DriveShare struct {
	ID   string `json:"id"`
	Kind string `json:"kind"` // "link", "user" o "group"
	// Subject es a quién: el `sub` de una persona o el nombre de un grupo. Vacío en un
	// enlace.
	Subject     string     `json:"subject,omitempty"`
	Permission  string     `json:"permission"`
	HasPassword bool       `json:"has_password,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Shares lista las comparticiones de un fichero o de una carpeta.
func (d *Drive) Shares(ctx context.Context, k DriveKind, id string) ([]DriveShare, error) {
	r, err := JSON[struct {
		Shares []DriveShare `json:"shares"`
	}](ctx, d.c, Request{Path: elemento(k, id, "/shares")})
	return r.Shares, err
}

// ShareLink crea un enlace público y devuelve su dirección, que no se vuelve a poder
// pedir. `escribir` deja además añadir y reemplazar contenido; `clave` vacía es sin
// contraseña y `caduca` a cero, sin caducidad.
func (d *Drive) ShareLink(ctx context.Context, k DriveKind, id string, escribir bool, clave string, caduca time.Time) (DriveShare, string, error) {
	cuerpo := cuerpoShare(k, id, "link", escribir)
	if clave != "" {
		cuerpo["password"] = clave
	}
	if !caduca.IsZero() {
		cuerpo["expires_at"] = caduca.UTC().Format(time.RFC3339)
	}
	r, err := PostJSON[struct {
		Share DriveShare `json:"share"`
		URL   string     `json:"url"`
	}](ctx, d.c, "/shares", cuerpo)
	return r.Share, r.URL, err
}

// ShareWith comparte con una persona de la organización (`tipo` "user", `sujeto` su
// `sub`, que da `Directorio.PorCorreo`) o con un grupo ("group", su nombre).
func (d *Drive) ShareWith(ctx context.Context, k DriveKind, id, tipo, sujeto string, escribir bool) (DriveShare, error) {
	cuerpo := cuerpoShare(k, id, tipo, escribir)
	cuerpo["subject"] = sujeto
	r, err := PostJSON[struct {
		Share DriveShare `json:"share"`
	}](ctx, d.c, "/shares", cuerpo)
	return r.Share, err
}

func cuerpoShare(k DriveKind, id, tipo string, escribir bool) map[string]string {
	campo := "file_id"
	if k == DriveFolder {
		campo = "folder_id"
	}
	permiso := "read"
	if escribir {
		permiso = "write"
	}
	return map[string]string{"kind": tipo, "permission": permiso, campo: id}
}

// Revoke mata una compartición: quien tuviera el enlace deja de entrar.
func (d *Drive) Revoke(ctx context.Context, compartida string) error {
	return d.mandar(ctx, http.MethodDelete, "/shares/"+url.PathEscape(compartida), nil)
}

// Compartido es algo que otra persona ha compartido CONTIGO: una raíz del listado de
// «Compartido conmigo», o un elemento de dentro de una.
//
// Dentro de una raíz, `Path` es RELATIVO a ella y es la única forma de nombrar el
// elemento: el servicio navega por (raíz, subcamino) y no por identificador, porque la
// verja de acceso es del share y no del nodo.
type Compartido struct {
	File
	IsDir bool `json:"is_directory"`
	// Permission ("view" o "edit") y SharedBy solo vienen en el listado de raíces; el
	// permiso de lo que hay DENTRO lo da CompartidoListado.
	Permission string `json:"permission,omitempty"`
	SharedBy   string `json:"shared_by,omitempty"`
	// EditRequested: en lectura, ya ha pedido editarlo (la franja lo dice).
	EditRequested bool `json:"edit_requested,omitempty"`
}

// Peticiones es la ruta de «pedir acceso de edición» de algo compartido (su raíz).
func (d *Drive) Peticiones(id string) Peticiones {
	return NuevasPeticiones(d.c, "/shared/"+url.PathEscape(id)+"/edit-requests")
}

// SharedWithMe lista las raíces que otras personas han compartido contigo. Sin ninguna,
// lista vacía: el servicio deniega por defecto y no filtra lo que existe.
func (d *Drive) SharedWithMe(ctx context.Context) ([]Compartido, error) {
	r, err := JSON[struct {
		Items []Compartido `json:"items"`
	}](ctx, d.c, Request{Path: "/shared-with-me"})
	return r.Items, err
}

// CompartidoListado es lo que hay dentro de una raíz compartida. IsDir dice si la raíz
// es una carpeta; si no lo es, Items trae solo el metadato del fichero.
type CompartidoListado struct {
	Permission string       `json:"permission"`
	IsDir      bool         `json:"is_dir"`
	More       bool         `json:"more"`
	Items      []Compartido `json:"items"`
}

// Editable: el share deja además subir y reemplazar. Lo decide el SERVICIO (manda "view"
// o "edit"); aquí solo se usa para no ofrecer un botón que acabaría en 403.
func (l CompartidoListado) Editable() bool { return l.Permission == "edit" }

// compartida arma la petición a una raíz compartida. El subcamino va por QUERY y no en
// el path porque es una ruta con barras: en el path lo partiría el enrutador.
func compartida(raiz, resto, subcamino string) Request {
	q := url.Values{}
	if subcamino != "" {
		q.Set("subpath", subcamino)
	}
	return Request{Path: "/shared/" + url.PathEscape(raiz) + resto, Query: q}
}

// SharedList navega dentro de una raíz compartida; `subcamino` vacío es la raíz misma.
// Sin share vigente el servicio contesta 404, no 403: no dice ni que exista.
func (d *Drive) SharedList(ctx context.Context, raiz, subcamino string) (CompartidoListado, error) {
	return JSON[CompartidoListado](ctx, d.c, compartida(raiz, "/list", subcamino))
}

// SharedContent abre el contenido de un fichero compartido. Si la raíz ES el fichero,
// `subcamino` va vacío. Cerrarlo es cosa de quien llama.
func (d *Drive) SharedContent(ctx context.Context, raiz, subcamino string) (io.ReadCloser, error) {
	resp, err := d.c.Do(ctx, compartida(raiz, "/content", subcamino))
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// SharedUpload sube un fichero a una CARPETA compartida, sin pasarlo por memoria. Con
// permiso de solo lectura el servicio contesta 403; sin share vigente, 404.
func (d *Drive) SharedUpload(ctx context.Context, raiz, subcamino, nombre, tipo string, cuerpo io.Reader, tamano int64) (File, error) {
	if tipo == "" {
		tipo = "application/octet-stream"
	}
	q := url.Values{"name": {nombre}}
	if subcamino != "" {
		q.Set("subpath", subcamino)
	}
	resp, err := d.c.Stream(ctx, http.MethodPost, "/shared/"+url.PathEscape(raiz)+"/files", q,
		http.Header{"Content-Type": {tipo}}, cuerpo, tamano)
	if err != nil {
		return File{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var f File
	err = json.NewDecoder(resp.Body).Decode(&f)
	return f, err
}

// DriveVersion es una entrada del historial de un fichero. La actual también sale, la primera.
type DriveVersion struct {
	Version   int   `json:"version"`
	SizeBytes int64 `json:"size_bytes"`
	// AuthorSub es el `sub` de quien la subió; la web lo enseña tal cual.
	AuthorSub string    `json:"author_sub,omitempty"`
	IsCurrent bool      `json:"is_current"`
	CreatedAt time.Time `json:"created_at"`
}

func version(fichero string, n int, resto string) string {
	return elemento(DriveFile, fichero, "/versions/"+strconv.Itoa(n)+resto)
}

// Versions es una página del historial de un fichero, lo más nuevo primero, y el cursor
// de la siguiente ("" si no hay más). `cursor` vacío es la primera.
func (d *Drive) Versions(ctx context.Context, fichero, cursor string) ([]DriveVersion, string, error) {
	if d.data != nil {
		return d.data.Versions(ctx, fichero, cursor)
	}
	q := url.Values{}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	r, err := JSON[struct {
		Versions   []DriveVersion `json:"versions"`
		NextCursor string         `json:"next_cursor"`
	}](ctx, d.c, Request{Path: elemento(DriveFile, fichero, "/versions"), Query: q})
	return r.Versions, r.NextCursor, err
}

// DownloadVersion abre el contenido de una versión ARCHIVADA; la actual se baja con Download.
func (d *Drive) DownloadVersion(ctx context.Context, fichero string, n int) (io.ReadCloser, error) {
	if d.data != nil {
		return d.data.DownloadVersion(ctx, fichero, n)
	}
	resp, err := d.c.Do(ctx, Request{Path: version(fichero, n, "/content")})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// RestoreVersion hace actual una versión archivada. No se pierde nada: lo que había pasa
// a ser una versión más del historial.
func (d *Drive) RestoreVersion(ctx context.Context, fichero string, n int) error {
	if d.data != nil {
		return d.avisado(d.data.RestoreVersion(ctx, fichero, n))
	}
	return d.mandar(ctx, http.MethodPost, version(fichero, n, "/restore"), nil)
}

// DriveLote es lo que ha movido o tirado una operación en lote. El servicio la aplica
// por ítem y se salta lo ajeno (RLS), así que puede mover menos de lo que se le pide:
// los contadores son la única forma de saber qué pasó de verdad.
type DriveLote struct {
	Ficheros int
	Carpetas int
}

// Total es cuántos elementos se movieron o se tiraron.
func (l DriveLote) Total() int { return l.Ficheros + l.Carpetas }

// BatchMove mueve ficheros y carpetas bajo otra carpeta en UNA petición. `destino`
// vacío es la raíz («Mi unidad»), que es lo que hace el servicio.
func (d *Drive) BatchMove(ctx context.Context, ficheros, carpetas []string, destino string) (DriveLote, error) {
	if x, ok := d.data.(DriveBatcher); ok {
		return x.BatchMove(ctx, ficheros, carpetas, destino)
	}
	r, err := PostJSON[struct {
		Ficheros int `json:"moved_files"`
		Carpetas int `json:"moved_folders"`
	}](ctx, d.c, "/batch/move", map[string]any{
		"file_ids": ficheros, "folder_ids": carpetas, "target_folder_id": destino,
	})
	return DriveLote{Ficheros: r.Ficheros, Carpetas: r.Carpetas}, err
}

// BatchTrash manda ficheros y carpetas a la papelera en UNA petición.
func (d *Drive) BatchTrash(ctx context.Context, ficheros, carpetas []string) (DriveLote, error) {
	if x, ok := d.data.(DriveBatcher); ok {
		return x.BatchTrash(ctx, ficheros, carpetas)
	}
	r, err := PostJSON[struct {
		Ficheros int `json:"trashed_files"`
		Carpetas int `json:"trashed_folders"`
	}](ctx, d.c, "/batch/trash", map[string]any{"file_ids": ficheros, "folder_ids": carpetas})
	return DriveLote{Ficheros: r.Ficheros, Carpetas: r.Carpetas}, err
}

// Zip abre una selección empaquetada en ZIP, en streaming. Cerrarlo es de quien llama.
//
// `contenido` es lo que suman sus ficheros SIN comprimir (la cabecera
// `X-Zip-Content-Bytes`), o 0 si el servicio no lo dice: el ZIP va comprimido y en
// streaming, así que su propio tamaño no se sabe hasta el final. Con él, quien lo lee
// puede enseñar el porcentaje (`ficheros` cuenta lo descomprimido a medida que llega).
func (d *Drive) Zip(ctx context.Context, ficheros, carpetas []string, nombre string) (cuerpo io.ReadCloser, contenido int64, err error) {
	if d.data != nil {
		return d.data.Zip(ctx, ficheros, carpetas, nombre)
	}
	q := url.Values{"file_ids": {strings.Join(ficheros, ",")}, "folder_ids": {strings.Join(carpetas, ",")}}
	if nombre != "" {
		q.Set("name", nombre)
	}
	resp, err := d.c.Do(ctx, Request{Path: "/batch/zip", Query: q})
	if err != nil {
		return nil, 0, err
	}
	contenido, _ = strconv.ParseInt(resp.Header.Get("X-Zip-Content-Bytes"), 10, 64)
	return resp.Body, max(contenido, 0), nil
}

// ConAviso es una copia de este cliente que llama a `avisar` tras cada escritura con
// éxito en los datos PRIVADOS (WithPrivateData): subir, crear, renombrar, mover, borrar,
// restaurar, copiar, sustituir, destacar y etiquetar. Es el ClienteConAviso del Drive en
// la bóveda o en Matrix, que no pasa por HTTP: con él el widget de Drive y el de CertiPDF
// se vuelven a publicar tras cada escritura (ADR 0078, enmienda del 2026-11-24). Las
// lecturas no avisan. `avisar` corre en la rutina de la escritura: tiene que volver
// enseguida. nil o sin datos privados, el mismo cliente.
func (d *Drive) ConAviso(avisar func()) *Drive {
	if d == nil || d.data == nil || avisar == nil {
		return d
	}
	copia := *d
	copia.avisar = avisar
	return &copia
}

// avisado avisa de una escritura privada si fue bien, y devuelve su error.
func (d *Drive) avisado(err error) error {
	if err == nil && d.avisar != nil {
		d.avisar()
	}
	return err
}

// conAviso es avisado para las escrituras que devuelven algo además del error.
func conAviso[T any](d *Drive) func(T, error) (T, error) {
	return func(v T, err error) (T, error) { return v, d.avisado(err) }
}

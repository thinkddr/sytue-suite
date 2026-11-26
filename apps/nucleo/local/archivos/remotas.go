// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package archivos

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
)

// # Ubicaciones remotas (DR-13)
//
// Las cuentas de nube que la persona conectó en Ajustes (`local/nube`: Sytue Drive o
// cualquier WebDAV, Google Drive, OneDrive) salen en Drive como UNIDADES más, junto a la
// carpeta del disco: se navegan, se abren y aceptan subir y soltar. [Remotas] envuelve la
// raíz local: lo que es de una unidad lo atiende el proveedor de la cuenta, y lo demás
// sigue yendo al disco tal cual.
//
//   - La red la pone quien abre el proveedor (`nube.Registro`, con el cliente del portero
//     de red, `local/red`): aquí no se crea ningún cliente HTTP.
//   - Abrir un fichero lo DESCARGA a una caché (`<caché>/drive/nube/<cuenta>/…`, con su
//     nombre) y lo abre desde ahí; cada descarga apunta su suma y la VERSIÓN remota (la
//     fecha de modificación y el tamaño que da el proveedor: `nube.Elemento` no trae ETag).
//   - [Remotas.Sincronizar] sube lo que se cambió en la caché (Docus guardando, por ejemplo).
//     Antes de subir vuelve a preguntar la versión remota: si alguien la cambió mientras
//     tanto, NO se pisa: lo de aquí sube al lado como «nombre (conflicto AAAA-MM-DD).ext»
//     (R-NO-CORROMPE), y lo dice.
//   - Crear carpetas, renombrar, mover dentro de la unidad y tirar los hace el proveedor
//     (`nube.Gestor`); mover a OTRA unidad (o al disco, o desde él) es copiar, comprobar
//     y quitar el origen (remotas_mover.go). Favoritos y etiquetas se guardan en este
//     equipo (remotas_marcas.go). Lo que los proveedores no exponen (versiones,
//     miniaturas) falla con ErrNoDisponible sobre una unidad, y la pantalla no lo ofrece.
//
// El identificador de un elemento remoto es `nube~<cuenta>~<id del proveedor en base64>`:
// el `~` no es del alfabeto de los identificadores locales, así que una operación local
// que reciba uno falla cerrada («no está»).
//
// Nombres repetidos: Google Drive admite dos documentos (o carpetas) con el mismo nombre en
// una carpeta, y un WebDAV puede dar el mismo `displayname` a dos recursos (OneDrive no:
// el nombre es único en su carpeta). Se ven TODOS: el primero de los que se llaman igual
// (`nube.Ordenar`: el más antiguo) con su nombre y los demás como «nombre (2).ext»,
// «nombre (3).ext»… (el primero libre en la carpeta), solo en la vista y en la caché; en
// la nube no se renombra nada. Abrir, copiar fuera, renombrar, mover y tirar van por el
// identificador del proveedor, y lo que se guarda en la caché sube encima del suyo por su
// identificador (`nube.Reemplazador`), no por el nombre.

// prefijoRemoto marca los identificadores de una unidad.
const prefijoRemoto = "nube~"

// Unidad es una cuenta de nube vista como unidad de Drive.
type Unidad struct {
	// ID es el de la carpeta raíz de la unidad, para navegar a ella.
	ID     string
	Nombre string
	Tipo   string
}

// Remotas es la raíz local más las unidades de las cuentas de nube.
type Remotas struct {
	*Archivos

	cuentas   func() []nube.Cuenta
	proveedor func(cuenta string) (nube.Proveedor, error)
	cache     string
	ahora     func() time.Time

	mu        sync.Mutex
	conocidos map[string]conocido
	estado    map[string]*descarga // ruta de la caché → lo descargado
	// Favoritos y etiquetas de lo que está en la nube (remotas_marcas.go); sin ruta, no
	// se admiten.
	rutaMarcas string
	marcas     []marca
}

// conocido es un elemento remoto ya listado: hace falta entero para descargarlo (Google
// necesita su tipo) y para saber de qué carpeta cuelga.
type conocido struct {
	Cuenta string
	E      nube.Elemento
	Padre  string // id del proveedor de la carpeta; "" la raíz
	// Vista es el nombre con que se ve: el suyo o, si otro de su carpeta se llama igual,
	// «nombre (2).ext»… ([nombresDeLaVista]). Vacío es el suyo.
	Vista string
}

// nombre es el nombre con que se ve.
func (k conocido) nombre() string {
	if k.Vista != "" {
		return k.Vista
	}
	return k.E.Nombre
}

// descarga es lo que se sabe de un fichero bajado a la caché.
type descarga struct {
	Cuenta  string    `json:"cuenta"`
	ID      string    `json:"id"`
	Carpeta string    `json:"carpeta"`
	Nombre  string    `json:"nombre"`
	Tipo    string    `json:"tipo,omitempty"`
	Suma    string    `json:"suma"`
	Version string    `json:"version"`
	Bajado  time.Time `json:"bajado"`
	// Marca es la fecha y el tamaño del fichero de la caché cuando se apuntó: si no han
	// cambiado, no hace falta leerlo entero para saber que sigue igual.
	Marca string `json:"marca,omitempty"`
	// Vista es el nombre con que se veía al bajarlo: si ahora se ve con otro (se fue o
	// llegó otro que se llama igual), se vuelve a bajar con el nuevo.
	Vista string `json:"vista,omitempty"`
	// PorID: el fichero de la caché ES el de la nube (no una exportación de un documento
	// de Google), así que sube encima del suyo por su identificador. Remoto es su nombre
	// en la nube. Lo apuntado antes de que existieran, sin ellos, sube como antes.
	PorID  bool   `json:"por_id,omitempty"`
	Remoto string `json:"remoto,omitempty"`
}

// marcaDe es la fecha de modificación y el tamaño de un fichero; "" si no está.
func marcaDe(ruta string) string {
	fi, err := os.Stat(ruta)
	if err != nil {
		return ""
	}
	return strconv.FormatInt(fi.ModTime().UnixNano(), 10) + "|" + strconv.FormatInt(fi.Size(), 10)
}

// cambiado dice si el fichero de la caché ya no es lo que se bajó o se subió.
func (d *descarga) cambiado(ruta string) bool {
	m := marcaDe(ruta)
	if m == "" || (d.Marca != "" && m == d.Marca) {
		return false
	}
	s, err := suma(ruta)
	return err == nil && s != d.Suma
}

// Subida es lo que hizo Sincronizar con un fichero cambiado.
type Subida struct {
	Nombre string
	// Conflicto: la versión remota había cambiado y lo de aquí subió con otro nombre.
	Conflicto bool
	// Como es el nombre con que quedó en la nube.
	Como string
	Err  error
}

// ConRemotas envuelve a con las unidades de las cuentas que dé `cuentas`; `proveedor`
// abre una (`nube.Registro.Proveedor`) y `cache` es donde se bajan los ficheros.
func ConRemotas(a *Archivos, cuentas func() []nube.Cuenta, proveedor func(string) (nube.Proveedor, error), cache string) *Remotas {
	r := &Remotas{
		Archivos: a, cuentas: cuentas, proveedor: proveedor, cache: cache, ahora: a.ahora,
		conocidos: map[string]conocido{}, estado: map[string]*descarga{},
	}
	if raw, err := os.ReadFile(r.rutaEstado()); err == nil {
		_ = json.Unmarshal(raw, &r.estado)
		if r.estado == nil {
			r.estado = map[string]*descarga{}
		}
	}
	return r
}

var _ api.DriveData = (*Remotas)(nil)

// Unidades son las cuentas conectadas ahora mismo.
func (r *Remotas) Unidades() []Unidad {
	cs := r.cuentas()
	out := make([]Unidad, 0, len(cs))
	for _, c := range cs {
		out = append(out, Unidad{ID: idRemoto(c.ID, ""), Nombre: c.Rotulo, Tipo: c.Tipo})
	}
	return out
}

// EsRemoto dice si un identificador es de una unidad.
func EsRemoto(id string) bool { return strings.HasPrefix(id, prefijoRemoto) }

func idRemoto(cuenta, id string) string {
	return prefijoRemoto + cuenta + "~" + base64.RawURLEncoding.EncodeToString([]byte(id))
}

func partirRemoto(id string) (cuenta, remoto string, ok bool) {
	resto, ok := strings.CutPrefix(id, prefijoRemoto)
	if !ok {
		return "", "", false
	}
	cuenta, cod, ok := strings.Cut(resto, "~")
	if !ok || cuenta == "" {
		return "", "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(cod)
	if err != nil {
		return "", "", false
	}
	return cuenta, string(raw), true
}

func (r *Remotas) cuenta(id string) (nube.Cuenta, bool) {
	for _, c := range r.cuentas() {
		if c.ID == id {
			return c, true
		}
	}
	return nube.Cuenta{}, false
}

// versionRemota es la que da el proveedor (ETag, versión de Google) y, si no da
// ninguna, la fecha y el tamaño.
func versionRemota(e nube.Elemento) string {
	if e.Version != "" {
		return "v:" + e.Version
	}
	return e.Modificado.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(e.Tamano, 10)
}

// ----- Tipos del API -----

func (r *Remotas) carpetaRemota(k conocido) api.Folder {
	p := idRemoto(k.Cuenta, k.Padre)
	return api.Folder{ID: idRemoto(k.Cuenta, k.E.ID), ParentID: &p, Name: k.nombre(), CreatedAt: k.E.Modificado, UpdatedAt: k.E.Modificado}
}

func (r *Remotas) ficheroRemoto(k conocido) api.File {
	tipo := k.E.Tipo
	if tipo == "" {
		tipo = tipoDe(k.E.Nombre)
	}
	return api.File{
		ID: idRemoto(k.Cuenta, k.E.ID), FolderID: idRemoto(k.Cuenta, k.Padre), Name: k.nombre(), SizeBytes: k.E.Tamano,
		ContentType: tipo, CreatedAt: k.E.Modificado, UpdatedAt: k.E.Modificado,
	}
}

// nombresDeLaVista da a cada elemento de una carpeta (por su id) el nombre con que se ve:
// el suyo o, si otro de la carpeta se llama igual, «nombre (2).ext», «nombre (3).ext»…
// (sin separar la extensión en una carpeta), el primero que no sea ya el nombre de otro.
// El primero de los que se llaman igual en el orden de [nube.Ordenar] (el más antiguo)
// se queda el suyo: es el mismo cada vez que se lista.
func nombresDeLaVista(es []nube.Elemento) map[string]string {
	orden := slices.Clone(es)
	nube.Ordenar(orden)
	cogidos := make(map[string]bool, len(es))
	for _, e := range es {
		cogidos[e.Nombre] = true
	}
	vistos := make(map[string]bool, len(es))
	out := make(map[string]string, len(es))
	for _, e := range orden {
		if !vistos[e.Nombre] {
			vistos[e.Nombre] = true
			out[e.ID] = e.Nombre
			continue
		}
		base, ext := e.Nombre, ""
		if !e.Carpeta {
			ext = filepath.Ext(e.Nombre)
			base = strings.TrimSuffix(e.Nombre, ext)
			if base == "" { // «.bashrc»
				base, ext = e.Nombre, ""
			}
		}
		for n := 2; ; n++ {
			if c := base + " (" + strconv.Itoa(n) + ")" + ext; !cogidos[c] {
				cogidos[c] = true
				out[e.ID] = c
				break
			}
		}
	}
	return out
}

// anotar apunta lo listado de una carpeta, cada uno con el nombre con que se ve, y lo
// devuelve en el mismo orden.
func (r *Remotas) anotar(cuenta, carpeta string, es []nube.Elemento) []conocido {
	vistas := nombresDeLaVista(es)
	out := make([]conocido, 0, len(es))
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range es {
		k := conocido{Cuenta: cuenta, E: e, Padre: carpeta, Vista: vistas[e.ID]}
		r.conocidos[idRemoto(cuenta, e.ID)] = k
		out = append(out, k)
	}
	r.alListar(cuenta, carpeta, es)
	return out
}

// ocupados son los nombres de una carpeta: los suyos y los que se ven.
func ocupados(es []nube.Elemento) map[string]bool {
	out := map[string]bool{}
	for _, v := range nombresDeLaVista(es) {
		out[v] = true
	}
	for _, e := range es {
		out[e.Nombre] = true
	}
	return out
}

// ----- Lectura -----

// Folder: la raíz de una unidad es una carpeta sin padre con el nombre de la cuenta.
func (r *Remotas) Folder(ctx context.Context, id string) (api.Folder, error) {
	cuenta, remoto, ok := partirRemoto(id)
	if !ok {
		return r.Archivos.Folder(ctx, id)
	}
	if remoto == "" {
		c, ok := r.cuenta(cuenta)
		if !ok {
			return api.Folder{}, noEsta(id)
		}
		return api.Folder{ID: id, Name: c.Rotulo}, nil
	}
	r.mu.Lock()
	k, ok := r.conocidos[id]
	r.mu.Unlock()
	if !ok || !k.E.Carpeta {
		return api.Folder{}, noEsta(id)
	}
	return r.carpetaRemota(k), nil
}

// Children lista una carpeta de la unidad en el proveedor.
func (r *Remotas) Children(ctx context.Context, id, cursor string) (api.Children, error) {
	cuenta, remoto, ok := partirRemoto(id)
	if !ok {
		return r.Archivos.Children(ctx, id, cursor)
	}
	p, err := r.proveedor(cuenta)
	if err != nil {
		return api.Children{}, err
	}
	es, err := p.Listar(ctx, remoto)
	if err != nil {
		return api.Children{}, err
	}
	var out api.Children
	for _, k := range r.anotar(cuenta, remoto, es) {
		if k.E.Carpeta {
			out.Folders = append(out.Folders, r.carpetaRemota(k))
		} else {
			out.Files = append(out.Files, r.ficheroRemoto(k))
		}
	}
	return out, nil
}

// File baja el fichero a la caché (si no estaba ya, o si cambió en la nube) y lo
// devuelve con la ruta de la caché en Path: es lo que se abre en su app.
func (r *Remotas) File(ctx context.Context, id string) (api.File, error) {
	if !EsRemoto(id) {
		return r.Archivos.File(ctx, id)
	}
	ruta, k, err := r.bajar(ctx, id)
	if err != nil {
		return api.File{}, err
	}
	f := r.ficheroRemoto(k)
	f.Path = ruta
	r.mu.Lock()
	if d := r.estado[ruta]; d != nil {
		f.Checksum = d.Suma
	}
	r.mu.Unlock()
	return f, nil
}

// Download sirve el fichero desde la caché, bajándolo antes si hace falta.
func (r *Remotas) Download(ctx context.Context, id string) (io.ReadCloser, error) {
	if !EsRemoto(id) {
		return r.Archivos.Download(ctx, id)
	}
	ruta, _, err := r.bajar(ctx, id)
	if err != nil {
		return nil, err
	}
	return os.Open(ruta)
}

// rutaCache es dónde se baja un elemento: una carpeta por elemento (por la suma de su
// id, para que dos ficheros con el mismo nombre no se pisen) con el fichero dentro con
// su nombre, que es lo que verá la app que lo abra.
func (r *Remotas) rutaCache(cuenta, remoto, nombre string) string {
	h := sha256.Sum256([]byte(remoto))
	n, err := nombreValido(filepath.Base(nombre))
	if err != nil {
		n = "fichero"
	}
	return filepath.Join(r.cache, cuenta, hex.EncodeToString(h[:8]), n)
}

func (r *Remotas) rutaEstado() string { return filepath.Join(r.cache, "estado.json") }

// guardarEstado se llama con r.mu tomado.
func (r *Remotas) guardarEstado() error {
	if err := os.MkdirAll(r.cache, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(r.estado, "", "  ")
	if err != nil {
		return err
	}
	return biblioteca.EscribirAtomico(r.rutaEstado(), append(raw, '\n'))
}

// bajar deja el fichero en la caché y dice dónde. Si ya estaba, con la versión remota y
// el nombre de la vista de ahora, no se vuelve a bajar; si se cambió aquí y aún no se ha
// subido, tampoco (se perdería el cambio): primero sube Sincronizar. Lo bajado antes con
// otro nombre (el de la vista cambió) se suelta.
func (r *Remotas) bajar(ctx context.Context, id string) (string, conocido, error) {
	cuenta, remoto, _ := partirRemoto(id)
	r.mu.Lock()
	k, ok := r.conocidos[id]
	previa, d := r.bajadoDe(cuenta, remoto)
	r.mu.Unlock()
	if !ok || k.E.Carpeta {
		return "", conocido{}, noEsta(id)
	}
	if d != nil {
		if _, err := os.Stat(previa); err == nil && (d.cambiado(previa) || d.Version == versionRemota(k.E) && cmpTexto(d.Vista, k.E.Nombre) == k.nombre()) {
			return previa, k, nil
		}
	}
	p, err := r.proveedor(cuenta)
	if err != nil {
		return "", conocido{}, err
	}
	datos, nombre, err := p.Descargar(ctx, k.E)
	if err != nil {
		return "", conocido{}, err
	}
	// Con el nombre de la vista; un documento de Google exportado a OOXML, con la
	// extensión que le pone la exportación.
	local, porID := k.nombre(), true
	if nombre != "" && nombre != k.E.Nombre {
		local, porID = nombre, false
		if resto, ok := strings.CutPrefix(nombre, k.E.Nombre); ok {
			local = k.nombre() + resto
		}
	}
	ruta := r.rutaCache(cuenta, remoto, local)
	if err := os.MkdirAll(filepath.Dir(ruta), 0o700); err != nil {
		return "", conocido{}, err
	}
	if err := biblioteca.EscribirAtomico(ruta, datos); err != nil {
		return "", conocido{}, err
	}
	h := sha256.Sum256(datos)
	r.mu.Lock()
	defer r.mu.Unlock()
	if previa != "" && previa != ruta {
		delete(r.estado, previa)
		_ = os.Remove(previa)
	}
	enLaNube := ""
	if porID {
		enLaNube = k.E.Nombre
	}
	r.estado[ruta] = &descarga{
		Cuenta: cuenta, ID: remoto, Carpeta: k.Padre, Nombre: filepath.Base(ruta), Tipo: k.E.Tipo,
		Suma: hex.EncodeToString(h[:]), Version: versionRemota(k.E), Bajado: r.ahora(), Marca: marcaDe(ruta),
		Vista: k.nombre(), PorID: porID, Remoto: enLaNube,
	}
	return ruta, k, r.guardarEstado()
}

// bajadoDe es dónde está en la caché el elemento remoto de la cuenta, si se bajó; con
// r.mu tomado.
func (r *Remotas) bajadoDe(cuenta, remoto string) (string, *descarga) {
	for ruta, d := range r.estado {
		if d.Cuenta == cuenta && d.ID == remoto {
			return ruta, d
		}
	}
	return "", nil
}

// cmpTexto es el primero de los dos que no está vacío.
func cmpTexto(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ----- Escritura -----

// Upload sube un fichero a una carpeta de la unidad (subir y soltar). Si ya hay UN
// fichero con ese nombre, se sustituye (por su identificador, como en Drive); si hay dos o
// más (Google Drive lo admite), no se adivina cuál: falla con un conflicto y no sube nada.
// La pantalla pregunta antes ([Remotas.Homonimos]) y sustituye el elegido con ReplaceFile.
func (r *Remotas) Upload(ctx context.Context, carpeta, nombre, tipo string, cuerpo io.Reader, tamano int64) (api.File, error) {
	cuenta, remoto, ok := partirRemoto(carpeta)
	if !ok {
		return r.Archivos.Upload(ctx, carpeta, nombre, tipo, cuerpo, tamano)
	}
	if _, err := nombreValido(nombre); err != nil {
		return api.File{}, err
	}
	datos, err := io.ReadAll(io.LimitReader(cuerpo, maxSubida+1))
	if err != nil {
		return api.File{}, err
	}
	if len(datos) > maxSubida {
		return api.File{}, fmt.Errorf("archivos: el fichero supera los %d MiB que admite la nube", maxSubida>>20)
	}
	p, err := r.proveedor(cuenta)
	if err != nil {
		return api.File{}, err
	}
	es, err := p.Listar(ctx, remoto)
	if err != nil {
		return api.File{}, err
	}
	var iguales []nube.Elemento
	for _, e := range es {
		if !e.Carpeta && e.Nombre == nombre {
			iguales = append(iguales, e)
		}
	}
	rp, sabe := p.(nube.Reemplazador)
	var e nube.Elemento
	switch {
	case len(iguales) > 1:
		return api.File{}, conflicto(nombre)
	case len(iguales) == 1 && sabe:
		e, err = rp.Reemplazar(ctx, iguales[0], datos)
	default:
		e, err = p.Subir(ctx, remoto, nombre, datos)
	}
	if err != nil {
		return api.File{}, err
	}
	return r.ficheroRemoto(r.trasSubir(ctx, p, cuenta, remoto, e)), nil
}

// Homonimos son los ficheros de una carpeta de la unidad que subir con ese nombre
// alcanzaría: los que se llaman así en la nube y el que se VE con ese nombre («acta
// (2).txt»), con el nombre con que se ven; y libre es el primer «nombre (n).ext» que no
// coge nadie, para guardar los dos. En el disco no hay (subir sustituye guardando la
// versión anterior).
func (r *Remotas) Homonimos(ctx context.Context, carpeta, nombre string) ([]api.File, string, error) {
	cuenta, remoto, ok := partirRemoto(carpeta)
	if !ok {
		return nil, "", nil
	}
	p, err := r.proveedor(cuenta)
	if err != nil {
		return nil, "", err
	}
	es, err := p.Listar(ctx, remoto)
	if err != nil {
		return nil, "", err
	}
	var out []api.File
	for _, k := range r.anotar(cuenta, remoto, es) {
		if !k.E.Carpeta && (k.E.Nombre == nombre || k.nombre() == nombre) {
			out = append(out, r.ficheroRemoto(k))
		}
	}
	libre := nombre
	if cogidos := ocupados(es); cogidos[nombre] {
		libre = nombreCopia(nombre, cogidos)
	}
	return out, libre, nil
}

// maxSubida es lo que admite una subida a la nube (lo que `nube` admite al bajar).
const maxSubida = 100 << 20

// releer vuelve a listar la carpeta para saber la versión que quedó en la nube: lo que
// devuelve Subir no siempre la trae.
func (r *Remotas) releer(ctx context.Context, p nube.Proveedor, cuenta, carpeta string, subido nube.Elemento) (nube.Elemento, bool) {
	es, err := p.Listar(ctx, carpeta)
	if err != nil {
		return nube.Elemento{}, false
	}
	var hallado nube.Elemento
	ok := false
	for _, k := range r.anotar(cuenta, carpeta, es) {
		if e := k.E; !e.Carpeta && ((subido.ID != "" && e.ID == subido.ID) || (!ok && e.Nombre == subido.Nombre)) {
			hallado, ok = e, true
		}
	}
	return hallado, ok
}

// trasSubir es cómo se ve lo que acaba de subir: vuelve a listar su carpeta (para saber la
// versión que quedó y el nombre con que se ve) y, si no puede, lo que devolvió Subir.
func (r *Remotas) trasSubir(ctx context.Context, p nube.Proveedor, cuenta, carpeta string, e nube.Elemento) conocido {
	if releido, ok := r.releer(ctx, p, cuenta, carpeta, e); ok {
		e = releido
	} else {
		r.apuntar(cuenta, e, carpeta)
	}
	k, err := r.conocidoDe(idRemoto(cuenta, e.ID))
	if err != nil {
		return conocido{Cuenta: cuenta, E: e, Padre: carpeta}
	}
	return k
}

// ReplaceFile guarda encima de un fichero de la unidad, con la misma comprobación de
// versión que Sincronizar.
//
// Un original sin suma, versión ni fecha (solo su id) es «Sustituir» al subir uno que se
// llama igual: va encima de ese por su identificador, sin bajarlo antes.
func (r *Remotas) ReplaceFile(ctx context.Context, original api.File, tipo string, cuerpo io.Reader, tamano int64) (api.File, error) {
	if !EsRemoto(original.ID) {
		return r.Archivos.ReplaceFile(ctx, original, tipo, cuerpo, tamano)
	}
	if original.Checksum == "" && original.Version == 0 && original.UpdatedAt.IsZero() {
		return r.sustituir(ctx, original.ID, cuerpo)
	}
	ruta, _, err := r.bajar(ctx, original.ID)
	if err != nil {
		return api.File{}, err
	}
	datos, err := io.ReadAll(io.LimitReader(cuerpo, maxSubida+1))
	if err != nil {
		return api.File{}, err
	}
	if err := biblioteca.EscribirAtomico(ruta, datos); err != nil {
		return api.File{}, err
	}
	s := r.subirUno(ctx, ruta)
	if s.Err != nil {
		return api.File{}, s.Err
	}
	if s.Conflicto {
		return api.File{}, ErrDesfasado
	}
	return r.File(ctx, original.ID)
}

// sustituir sube cuerpo encima del fichero id de una unidad, por su identificador.
func (r *Remotas) sustituir(ctx context.Context, id string, cuerpo io.Reader) (api.File, error) {
	k, err := r.conocidoDe(id)
	if err != nil {
		return api.File{}, err
	}
	if k.E.Carpeta {
		return api.File{}, noEsta(id)
	}
	datos, err := io.ReadAll(io.LimitReader(cuerpo, maxSubida+1))
	if err != nil {
		return api.File{}, err
	}
	if len(datos) > maxSubida {
		return api.File{}, errDemasiado
	}
	p, err := r.proveedor(k.Cuenta)
	if err != nil {
		return api.File{}, err
	}
	rp, ok := p.(nube.Reemplazador)
	if !ok {
		return api.File{}, errRemoto
	}
	e, err := rp.Reemplazar(ctx, k.E, datos)
	if err != nil {
		return api.File{}, err
	}
	return r.ficheroRemoto(r.trasSubir(ctx, p, k.Cuenta, k.Padre, e)), nil
}

// Sincronizar sube los ficheros de la caché que se cambiaron desde que se bajaron. Es lo
// que llama la app al volver a la ventana y cuando ve cambiar la caché.
func (r *Remotas) Sincronizar(ctx context.Context) []Subida {
	r.mu.Lock()
	var rutas []string
	for ruta, d := range r.estado {
		if d.cambiado(ruta) {
			rutas = append(rutas, ruta)
		}
	}
	r.mu.Unlock()
	slices.Sort(rutas)
	var out []Subida
	for _, ruta := range rutas {
		if ctx.Err() != nil {
			break
		}
		out = append(out, r.subirUno(ctx, ruta))
	}
	return out
}

// Cache es la carpeta donde se bajan los ficheros: la que hay que vigilar.
func (r *Remotas) Cache() string { return r.cache }

// subirUno sube un fichero cambiado de la caché comprobando antes la versión remota.
func (r *Remotas) subirUno(ctx context.Context, ruta string) Subida {
	r.mu.Lock()
	d0 := r.estado[ruta]
	var d descarga
	if d0 != nil {
		d = *d0
	}
	r.mu.Unlock()
	if d0 == nil {
		return Subida{Nombre: filepath.Base(ruta), Err: noEsta(ruta)}
	}
	s := Subida{Nombre: d.Nombre, Como: d.Nombre}
	datos, err := os.ReadFile(ruta)
	if err != nil {
		s.Err = err
		return s
	}
	p, err := r.proveedor(d.Cuenta)
	if err != nil {
		s.Err = err
		return s
	}
	es, err := p.Listar(ctx, d.Carpeta)
	if err != nil {
		s.Err = err
		return s
	}
	e, err := r.subirSegun(ctx, p, &d, es, datos, &s)
	if err != nil {
		s.Err = err
		return s
	}
	if releido, ok := r.releer(ctx, p, d.Cuenta, d.Carpeta, e); ok {
		e = releido
	}
	h := sha256.Sum256(datos)
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.Conflicto {
		// La copia de aquí es ahora la del fichero de conflicto: lo que se siga
		// cambiando sube a ese, no encima del de la otra persona.
		delete(r.estado, ruta)
		nueva := r.rutaCache(d.Cuenta, e.ID, s.Como)
		if os.MkdirAll(filepath.Dir(nueva), 0o700) == nil && biblioteca.EscribirAtomico(nueva, datos) == nil {
			_ = os.Remove(ruta)
			ruta = nueva
		}
	}
	vista := d.Vista
	if s.Conflicto {
		vista = s.Como
	}
	// Lo de la caché es ya un fichero de la nube tal cual (también lo que fue una
	// exportación de Google): lo que se siga cambiando sube encima, por su identificador.
	r.estado[ruta] = &descarga{
		Cuenta: d.Cuenta, ID: e.ID, Carpeta: d.Carpeta, Nombre: filepath.Base(ruta), Tipo: d.Tipo,
		Suma: hex.EncodeToString(h[:]), Version: versionRemota(e), Bajado: r.ahora(), Marca: marcaDe(ruta),
		Vista: vista, PorID: true, Remoto: cmpTexto(e.Nombre, s.Como),
	}
	if err := r.guardarEstado(); err != nil {
		s.Err = err
	}
	return s
}

// subirSegun sube datos según lo que hay ahora en la carpeta remota (es): encima del
// suyo, con su nombre o, si alguien lo cambió o su nombre es ya de otro, como fichero de
// conflicto. Deja en s el nombre con que sube y si es un conflicto.
func (r *Remotas) subirSegun(ctx context.Context, p nube.Proveedor, d *descarga, es []nube.Elemento, datos []byte, s *Subida) (nube.Elemento, error) {
	actual := ""
	var suyo nube.Elemento
	for _, e := range es {
		if e.ID == d.ID {
			actual, suyo = versionRemota(e), e
		}
	}
	cogidos := ocupados(es)
	// El nombre con que está (o estaba) en la nube: el de la caché puede ser el de la
	// vista («nombre (2).ext») o el de una exportación de Google («Plan.docx»).
	enLaNube := d.Nombre
	if d.PorID && d.Remoto != "" {
		enLaNube = d.Remoto
	}
	rp, sabeReemplazar := p.(nube.Reemplazador)
	switch {
	case actual != "" && actual != d.Version:
		// Alguien la cambió mientras tanto: no se pisa.
		s.Conflicto = true
		s.Como = nombreConflicto(d.Nombre, r.ahora(), cogidos)
		return p.Subir(ctx, d.Carpeta, s.Como, datos)
	case actual != "" && sabeReemplazar && (d.PorID || suyo.Nombre == d.Nombre):
		// Encima del suyo, por su identificador: otro de la carpeta puede llamarse igual.
		s.Como = suyo.Nombre
		return rp.Reemplazar(ctx, suyo, datos)
	case actual == "" && cogidos[enLaNube]:
		// Ya no está y su nombre es ahora de otro: subirlo con él lo pisaría.
		s.Conflicto = true
		s.Como = nombreConflicto(d.Nombre, r.ahora(), cogidos)
		return p.Subir(ctx, d.Carpeta, s.Como, datos)
	default:
		// Si en la nube ya no está, se vuelve a crear con su nombre (y una exportación de
		// Google sube a su lado como fichero).
		s.Como = enLaNube
		return p.Subir(ctx, d.Carpeta, s.Como, datos)
	}
}

// nombreConflicto es «nombre (conflicto AAAA-MM-DD).ext», con « (n)» si ya existe.
func nombreConflicto(nombre string, cuando time.Time, cogidos map[string]bool) string {
	ext := filepath.Ext(nombre)
	base := strings.TrimSuffix(nombre, ext) + " (conflicto " + cuando.Format("2006-01-02") + ")"
	c := base + ext
	for n := 2; cogidos[c]; n++ {
		c = base + " (" + strconv.Itoa(n) + ")" + ext
	}
	return c
}

// ----- Gestión de carpetas en la unidad -----

// errRemoto es lo que contesta una operación que el proveedor no expone.
var errRemoto = fmt.Errorf("archivos: una unidad de la nube no admite esta operación: %w", ErrNoDisponible)

// gestor es el de la cuenta, o errRemoto si su proveedor no gestiona carpetas.
func (r *Remotas) gestor(cuenta string) (nube.Proveedor, nube.Gestor, error) {
	p, err := r.proveedor(cuenta)
	if err != nil {
		return nil, nil, err
	}
	g, ok := p.(nube.Gestor)
	if !ok {
		return nil, nil, errRemoto
	}
	return p, g, nil
}

// conocidoDe es el elemento ya listado con ese id.
func (r *Remotas) conocidoDe(id string) (conocido, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k, ok := r.conocidos[id]
	if !ok {
		return conocido{}, noEsta(id)
	}
	return k, nil
}

// subirPendientes sube lo cambiado aquí de una cuenta antes de mover o renombrar en
// ella: después, la caché ya no sabría dónde subirlo.
func (r *Remotas) subirPendientes(ctx context.Context, cuenta string) error {
	r.mu.Lock()
	var rutas []string
	for ruta, d := range r.estado {
		if d.Cuenta == cuenta && d.cambiado(ruta) {
			rutas = append(rutas, ruta)
		}
	}
	r.mu.Unlock()
	slices.Sort(rutas)
	for _, ruta := range rutas {
		s := r.subirUno(ctx, ruta)
		if s.Err != nil {
			return s.Err
		}
		if s.Conflicto {
			return ErrDesfasado
		}
	}
	return nil
}

// cuelgaDe dice si el id del proveedor es viejo o cuelga de él (los proveedores con rutas
// como id, WebDAV).
func cuelgaDe(id, viejo string) bool {
	return id == viejo || strings.HasPrefix(id, strings.TrimSuffix(viejo, "/")+"/")
}

// olvidar quita de la caché y de lo listado un elemento que se ha movido, renombrado o
// tirado, y lo que cuelga de él: se vuelve a listar y a bajar donde esté ahora. Lo
// pendiente de subir ya subió (subirPendientes) salvo al tirar, que es lo que se pidió.
func (r *Remotas) olvidar(cuenta, viejo string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, k := range r.conocidos {
		if k.Cuenta == cuenta && (cuelgaDe(k.E.ID, viejo) || cuelgaDe(k.Padre, viejo)) {
			delete(r.conocidos, id)
		}
	}
	cambio := false
	for ruta, d := range r.estado {
		if d.Cuenta == cuenta && (cuelgaDe(d.ID, viejo) || cuelgaDe(d.Carpeta, viejo)) {
			delete(r.estado, ruta)
			_ = os.Remove(ruta)
			_ = os.Remove(filepath.Dir(ruta))
			cambio = true
		}
	}
	if cambio {
		_ = r.guardarEstado()
	}
}

// apuntar deja como listado un elemento que acaba de crear o cambiar el proveedor.
func (r *Remotas) apuntar(cuenta string, e nube.Elemento, padre string) {
	r.mu.Lock()
	r.conocidos[idRemoto(cuenta, e.ID)] = conocido{Cuenta: cuenta, E: e, Padre: padre}
	r.mu.Unlock()
}

// CreateFolder crea la carpeta en el proveedor si sabe (nube.Gestor).
func (r *Remotas) CreateFolder(ctx context.Context, padre, nombre string) (api.Folder, error) {
	cuenta, remoto, ok := partirRemoto(padre)
	if !ok {
		return r.Archivos.CreateFolder(ctx, padre, nombre)
	}
	nombre, err := nombreValido(strings.TrimSpace(nombre))
	if err != nil {
		return api.Folder{}, err
	}
	_, g, err := r.gestor(cuenta)
	if err != nil {
		return api.Folder{}, err
	}
	e, err := g.CrearCarpeta(ctx, remoto, nombre)
	if err != nil {
		return api.Folder{}, err
	}
	e.Carpeta = true
	r.apuntar(cuenta, e, remoto)
	return r.carpetaRemota(conocido{Cuenta: cuenta, E: e, Padre: remoto}), nil
}

// Rename renombra en el proveedor; lo cambiado aquí sube antes.
func (r *Remotas) Rename(ctx context.Context, k api.DriveKind, id, nombre string) error {
	cuenta, _, ok := partirRemoto(id)
	if !ok {
		return r.Archivos.Rename(ctx, k, id, nombre)
	}
	nombre, err := nombreValido(strings.TrimSpace(nombre))
	if err != nil {
		return err
	}
	_, g, err := r.gestor(cuenta)
	if err != nil {
		return err
	}
	c, err := r.conocidoDe(id)
	if err != nil {
		return err
	}
	if err := r.subirPendientes(ctx, cuenta); err != nil {
		return err
	}
	e, err := g.Renombrar(ctx, c.E, nombre)
	if err != nil {
		return err
	}
	r.olvidar(cuenta, c.E.ID)
	e.Carpeta = c.E.Carpeta
	r.apuntar(cuenta, e, c.Padre)
	r.trasCambiar(cuenta, c.E, e, c.Padre)
	return nil
}

// Move dentro de una misma unidad lo hace el proveedor (o el disco); entre unidades, o
// entre una y el disco, es un traslado: copiar, comprobar y quitar el origen
// ([Remotas.Trasladar]).
func (r *Remotas) Move(ctx context.Context, k api.DriveKind, id, destino string) error {
	if !EsRemoto(id) && !EsRemoto(destino) {
		return r.Archivos.Move(ctx, k, id, destino)
	}
	cuenta, _, ok1 := partirRemoto(id)
	cuentaDestino, remotoDestino, ok2 := partirRemoto(destino)
	if !ok1 || !ok2 || cuenta != cuentaDestino {
		return r.moverEntre(ctx, id, destino)
	}
	_, g, err := r.gestor(cuenta)
	if err != nil {
		return err
	}
	c, err := r.conocidoDe(id)
	if err != nil {
		return err
	}
	if c.Padre == remotoDestino {
		return nil
	}
	if c.E.Carpeta && cuelgaDe(remotoDestino, c.E.ID) {
		return fmt.Errorf("archivos: una carpeta no se mueve dentro de sí misma: %w", api.ErrPrivateDriveOperation)
	}
	if err := r.subirPendientes(ctx, cuenta); err != nil {
		return err
	}
	e, err := g.Mover(ctx, c.E, remotoDestino)
	if err != nil {
		return err
	}
	r.olvidar(cuenta, c.E.ID)
	e.Carpeta = c.E.Carpeta
	r.apuntar(cuenta, e, remotoDestino)
	r.trasCambiar(cuenta, c.E, e, remotoDestino)
	return nil
}

// CopyFile en una unidad baja el fichero y lo vuelve a subir al lado como «nombre (2)».
func (r *Remotas) CopyFile(ctx context.Context, id string) (api.File, error) {
	cuenta, _, ok := partirRemoto(id)
	if !ok {
		return r.Archivos.CopyFile(ctx, id)
	}
	ruta, c, err := r.bajar(ctx, id)
	if err != nil {
		return api.File{}, err
	}
	datos, err := os.ReadFile(ruta)
	if err != nil {
		return api.File{}, err
	}
	p, err := r.proveedor(cuenta)
	if err != nil {
		return api.File{}, err
	}
	es, err := p.Listar(ctx, c.Padre)
	if err != nil {
		return api.File{}, err
	}
	// La copia parte de su nombre en la nube, no del de la vista («nombre (2).ext»), y no
	// coge ninguno de los que se ven.
	base := filepath.Base(ruta)
	if v := c.nombre(); v != c.E.Nombre {
		if resto, ok := strings.CutPrefix(base, v); ok {
			base = c.E.Nombre + resto
		}
	}
	nombre := nombreCopia(base, ocupados(es))
	e, err := p.Subir(ctx, c.Padre, nombre, datos)
	if err != nil {
		return api.File{}, err
	}
	return r.ficheroRemoto(r.trasSubir(ctx, p, cuenta, c.Padre, e)), nil
}

// nombreCopia es el primer «base (n).ext» libre entre los nombres de la carpeta.
func nombreCopia(nombre string, cogidos map[string]bool) string {
	ext := filepath.Ext(nombre)
	base := strings.TrimSuffix(nombre, ext)
	for n := 2; ; n++ {
		if c := base + " (" + strconv.Itoa(n) + ")" + ext; !cogidos[c] {
			return c
		}
	}
}

// Delete en una unidad va a la papelera del proveedor (Google, OneDrive y Nextcloud la
// tienen): de allí se recupera.
func (r *Remotas) Delete(ctx context.Context, k api.DriveKind, id string) error {
	cuenta, remoto, ok := partirRemoto(id)
	if !ok {
		return r.Archivos.Delete(ctx, k, id)
	}
	if remoto == "" {
		return errRemoto // la raíz de la unidad es la cuenta: se quita en Ajustes
	}
	_, g, err := r.gestor(cuenta)
	if err != nil {
		return err
	}
	c, err := r.conocidoDe(id)
	if err != nil {
		return err
	}
	if err := g.Borrar(ctx, c.E); err != nil {
		return err
	}
	r.olvidar(cuenta, c.E.ID)
	r.soltarMarcas(cuenta, c.E)
	return nil
}

// Thumbnail: no se baja un fichero entero para su miniatura; la que ya está en la caché,
// sí.
func (r *Remotas) Thumbnail(ctx context.Context, id string) (io.ReadCloser, error) {
	if !EsRemoto(id) {
		return r.Archivos.Thumbnail(ctx, id)
	}
	return nil, errRemoto
}

// Versions: el historial es el del proveedor, que no se expone.
func (r *Remotas) Versions(ctx context.Context, id, cursor string) ([]api.DriveVersion, string, error) {
	if EsRemoto(id) {
		return nil, "", nil
	}
	return r.Archivos.Versions(ctx, id, cursor)
}

// CopiarFuera deja una copia de un elemento en una carpeta del equipo («Copiar a…»).
func (r *Remotas) CopiarFuera(ctx context.Context, id, carpeta string) (string, error) {
	return r.CopiarFueraEn(ctx, id, DelDisco, carpeta)
}

// CopiarFueraEn es CopiarFuera hacia una carpeta de otro sistema de ficheros
// ([Archivos.CopiarFueraEn]): lo de una unidad, bajado antes a la caché.
func (r *Remotas) CopiarFueraEn(ctx context.Context, id string, destino Sistema, carpeta string) (string, error) {
	if !EsRemoto(id) {
		return r.Archivos.CopiarFueraEn(ctx, id, destino, carpeta)
	}
	ruta, _, err := r.bajar(ctx, id)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(carpeta, filepath.Base(ruta))
	if _, err := destino.Lstat(dst); err == nil {
		dst = nombreLibreEn(destino, carpeta, filepath.Base(ruta))
	}
	datos, err := os.ReadFile(ruta)
	if err != nil {
		return "", err
	}
	return dst, destino.EscribirAtomico(dst, datos)
}

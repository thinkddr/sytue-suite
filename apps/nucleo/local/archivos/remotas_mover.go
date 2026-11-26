// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package archivos

import (
	"bytes"
	"context"
	"crypto/md5"  //nolint:gosec // es la huella que da Google Drive, no una firma
	"crypto/sha1" //nolint:gosec // es la huella que da OneDrive, no una firma
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
)

// # Mover entre unidades
//
// Dentro de una misma unidad mueve quien la guarda (el disco renombra; la nube, su
// `nube.Gestor`). Entre dos unidades distintas —el disco y una nube, o dos nubes— no hay
// quien mueva: [Remotas.Trasladar] COPIA cada fichero, COMPRUEBA que lo copiado llegó
// entero (el tamaño y, si el destino da huella —MD5 en Google Drive, SHA-1 en OneDrive
// personal—, la huella; si no la da, lo vuelve a leer y compara; en el disco, su SHA-256)
// y solo entonces quita el origen, a la papelera de su unidad (`.sytue/papelera` en el
// disco; la del proveedor en la nube, que en WebDAV es la del servidor si la tiene).
//
//   - Una carpeta va entera: se recorre, se recrea en el destino y se copian sus ficheros
//     de uno en uno, con su avance. Al final se tira de una vez lo que quedó copiado
//     entero (la carpeta, si todo lo de dentro llegó; si no, cada fichero que sí).
//   - Si algo falla a medias, o se cancela, lo que no llegó entero sigue en el origen tal
//     cual y la copia a medias se retira; el resultado dice qué se movió y qué no, y por
//     qué.
//   - En el destino no se pisa nada: si ya hay algo con ese nombre, eso no se mueve.
//   - Un documento de Google (Docs, Hojas, Presentaciones) no tiene fichero: bajarlo es
//     exportarlo, así que no se mueve (con «Copiar a…» se trae exportado).
//   - Lo oculto del disco («.DS_Store») no viaja, como al copiar; una carpeta que lo
//     tiene se queda en el origen con ello dentro.
//   - Favoritos y etiquetas van con lo que se mueve.
//
// Lo copiado se lee entero en memoria, como cualquier subida a la nube: hasta los
// 100 MiB que admite una (maxSubida).

// MotivoTraslado es por qué algo no se movió.
type MotivoTraslado int

// Los motivos.
const (
	// FalloCopia: no se pudo leer del origen o escribir en el destino.
	FalloCopia MotivoTraslado = iota
	// FalloExiste: en el destino ya hay algo con ese nombre; no se pisa.
	FalloExiste
	// FalloComprobacion: lo copiado no llegó entero; la copia se retiró.
	FalloComprobacion
	// FalloDocumentoGoogle: un documento de Google no tiene fichero que mover.
	FalloDocumentoGoogle
	// FalloOrigen: se copió y se comprobó, pero no se pudo quitar del origen: está en
	// los dos sitios.
	FalloOrigen
	// FalloCancelado: se canceló antes de que llegara.
	FalloCancelado
	// FalloTamano: pasa de lo que admite una subida a la nube.
	FalloTamano
)

// NoMovido es lo que se quedó en el origen.
type NoMovido struct {
	// Nombre es su ruta desde lo que se movió («Informes/acta.txt»).
	Nombre string
	Motivo MotivoTraslado
	Err    error
}

// Traslado es lo que pasó al mover entre unidades.
type Traslado struct {
	// Total es cuántos ficheros (y carpetas vacías) había que mover.
	Total int
	// Movidos son los que ya solo están en el destino, por su ruta («Informes/acta.txt»).
	Movidos   []string
	NoMovidos []NoMovido
	Cancelado bool
}

// AvanceTraslado es por dónde va un traslado.
type AvanceTraslado struct {
	// Hechos son los ficheros ya tratados (copiados o no) de Total.
	Hechos, Total int
	// Nombre es el que va ahora.
	Nombre string
}

// MaxTraslado es lo más grande que se mueve entre unidades: lo que admite una subida a la
// nube (y una bajada de `nube`).
const MaxTraslado = maxSubida

// errDemasiado es un fichero que pasa de lo que admite una subida a la nube.
var errDemasiado = fmt.Errorf("archivos: el fichero supera los %d MiB que admite la nube", maxSubida>>20)

// UnidadDe es la unidad de un identificador: "" el disco, o el id de la raíz de su
// unidad de la nube.
func UnidadDe(id string) string {
	if c, _, ok := partirRemoto(id); ok {
		return idRemoto(c, "")
	}
	return ""
}

// MismaUnidad dice si dos identificadores son de la misma unidad.
func MismaUnidad(a, b string) bool { return UnidadDe(a) == UnidadDe(b) }

// nodo es un fichero o una carpeta de un lado del traslado.
type nodo struct {
	id      string // el de la fachada: idDe(ruta) en el disco, idRemoto(…) en una unidad
	nombre  string // con el que se ve (y con el que llega al destino)
	carpeta bool
	tamano  int64
	google  bool          // documento de Google: sin fichero
	e       nube.Elemento // en una unidad, el del proveedor
}

func (n nodo) clase() api.DriveKind {
	if n.carpeta {
		return api.DriveFolder
	}
	return api.DriveFile
}

// lado es una unidad vista desde el traslado.
type lado interface {
	// nodoDe es el elemento con ese id.
	nodoDe(ctx context.Context, id string) (nodo, error)
	// hijos lista una carpeta; ocultos dice si había algo que no viaja.
	hijos(ctx context.Context, carpeta string) (hijos []nodo, ocultos bool, err error)
	leer(ctx context.Context, n nodo) ([]byte, error)
	// ocupado dice si en la carpeta ya hay algo con ese nombre.
	ocupado(ctx context.Context, carpeta, nombre string) (bool, error)
	crearCarpeta(ctx context.Context, padre, nombre string) (nodo, error)
	// escribir deja datos como fichero NUEVO en la carpeta (que no tiene ese nombre).
	escribir(ctx context.Context, carpeta, nombre string, datos []byte) (nodo, error)
	// comprobar mira que lo escrito llegó entero.
	comprobar(ctx context.Context, carpeta string, n nodo, datos []byte) error
	// tirar manda a la papelera de la unidad.
	tirar(ctx context.Context, n nodo) error
	// retirar quita lo que este traslado dejó a medias con ese nombre en la carpeta.
	retirar(ctx context.Context, carpeta, nombre string)
}

// ladoDe es la unidad de un id, lista para un traslado. Una unidad de la nube tiene que
// gestionar carpetas (tirar el origen, crear las del destino).
func (r *Remotas) ladoDe(id string) (lado, error) {
	cuenta, _, ok := partirRemoto(id)
	if !ok {
		return ladoDisco{r.Archivos}, nil
	}
	p, g, err := r.gestor(cuenta)
	if err != nil {
		return nil, err
	}
	return ladoNube{r: r, cuenta: cuenta, p: p, g: g}, nil
}

// ----- El disco -----

type ladoDisco struct{ a *Archivos }

func (d ladoDisco) nodoDe(_ context.Context, id string) (nodo, error) {
	r, err := d.a.ruta(id)
	if err != nil {
		return nodo{}, err
	}
	if r == d.a.raiz() {
		return nodo{}, ErrNombre
	}
	fi, err := d.a.fs().Stat(r)
	if err != nil {
		return nodo{}, noEsta(id)
	}
	return nodo{id: idDe(r), nombre: fi.Name(), carpeta: fi.IsDir(), tamano: fi.Size()}, nil
}

func (d ladoDisco) hijos(_ context.Context, carpeta string) ([]nodo, bool, error) {
	r, err := d.a.ruta(carpeta)
	if err != nil {
		return nil, false, err
	}
	es, err := d.a.fs().ReadDir(r)
	if err != nil {
		return nil, false, err
	}
	var out []nodo
	ocultos := false
	for _, e := range es {
		if oculto(e.Name()) {
			ocultos = true
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		hijo := filepath.Join(r, e.Name())
		switch {
		case fi.IsDir():
			out = append(out, nodo{id: idDe(hijo), nombre: e.Name(), carpeta: true})
		case fi.Mode().IsRegular():
			out = append(out, nodo{id: idDe(hijo), nombre: e.Name(), tamano: fi.Size()})
		default:
			ocultos = true // un enlace simbólico no viaja
		}
	}
	return out, ocultos, nil
}

func (d ladoDisco) leer(_ context.Context, n nodo) ([]byte, error) {
	r, err := d.a.ruta(n.id)
	if err != nil {
		return nil, err
	}
	fi, err := d.a.fs().Stat(r)
	if err != nil {
		return nil, err
	}
	if fi.Size() > maxSubida {
		return nil, errDemasiado
	}
	return d.a.fs().ReadFile(r)
}

func (d ladoDisco) ocupado(_ context.Context, carpeta, nombre string) (bool, error) {
	r, err := d.a.ruta(carpeta)
	if err != nil {
		return false, err
	}
	_, err = d.a.fs().Lstat(filepath.Join(r, nombre))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (d ladoDisco) crearCarpeta(ctx context.Context, padre, nombre string) (nodo, error) {
	f, err := d.a.CreateFolder(ctx, padre, nombre)
	if err != nil {
		return nodo{}, err
	}
	return nodo{id: f.ID, nombre: f.Name, carpeta: true}, nil
}

func (d ladoDisco) escribir(ctx context.Context, carpeta, nombre string, datos []byte) (nodo, error) {
	dir, err := d.a.ruta(carpeta)
	if err != nil {
		return nodo{}, err
	}
	if nombre, err = nombreValido(nombre); err != nil {
		return nodo{}, err
	}
	r := filepath.Join(dir, nombre)
	if _, err := d.a.fs().Lstat(r); err == nil {
		return nodo{}, conflicto(nombre)
	}
	if err := d.a.escribir(ctx, r, bytes.NewReader(datos), int64(len(datos))); err != nil {
		return nodo{}, err
	}
	return nodo{id: idDe(r), nombre: nombre, tamano: int64(len(datos))}, nil
}

// comprobar relee lo escrito en el disco: el tamaño y su SHA-256.
func (d ladoDisco) comprobar(_ context.Context, _ string, n nodo, datos []byte) error {
	r, err := d.a.ruta(n.id)
	if err != nil {
		return err
	}
	fi, err := d.a.fs().Stat(r)
	if err != nil {
		return err
	}
	if fi.Size() != int64(len(datos)) {
		return fmt.Errorf("archivos: la copia mide %d bytes y el original %d", fi.Size(), len(datos))
	}
	s, err := sumaEn(d.a.fs(), r)
	if err != nil {
		return err
	}
	if h := sha256.Sum256(datos); s != hex.EncodeToString(h[:]) {
		return errors.New("archivos: la copia no es igual que el original")
	}
	return nil
}

func (d ladoDisco) tirar(ctx context.Context, n nodo) error { return d.a.Delete(ctx, n.clase(), n.id) }

func (d ladoDisco) retirar(_ context.Context, carpeta, nombre string) {
	dir, err := d.a.ruta(carpeta)
	if err != nil {
		return
	}
	_ = d.a.fs().RemoveAll(filepath.Join(dir, nombre))
}

// ----- Una unidad de la nube -----

type ladoNube struct {
	r      *Remotas
	cuenta string
	p      nube.Proveedor
	g      nube.Gestor
}

// esDocumentoGoogle: los de Google (salvo las carpetas) no tienen bytes.
func esDocumentoGoogle(e nube.Elemento) bool {
	return !e.Carpeta && strings.HasPrefix(e.Tipo, "application/vnd.google-apps.")
}

func (n ladoNube) nodoDe(_ context.Context, id string) (nodo, error) {
	_, remoto, _ := partirRemoto(id)
	if remoto == "" {
		return nodo{}, errRemoto // la raíz de la unidad es la cuenta
	}
	k, err := n.r.conocidoDe(id)
	if err != nil {
		return nodo{}, err
	}
	return n.nodo(k), nil
}

func (n ladoNube) nodo(k conocido) nodo {
	return nodo{id: idRemoto(k.Cuenta, k.E.ID), nombre: k.nombre(), carpeta: k.E.Carpeta, tamano: k.E.Tamano, google: esDocumentoGoogle(k.E), e: k.E}
}

func (n ladoNube) listar(ctx context.Context, carpeta string) (string, []nube.Elemento, error) {
	_, remoto, ok := partirRemoto(carpeta)
	if !ok {
		return "", nil, noEsta(carpeta)
	}
	es, err := n.p.Listar(ctx, remoto)
	return remoto, es, err
}

func (n ladoNube) hijos(ctx context.Context, carpeta string) ([]nodo, bool, error) {
	remoto, es, err := n.listar(ctx, carpeta)
	if err != nil {
		return nil, false, err
	}
	var out []nodo
	for _, k := range n.r.anotar(n.cuenta, remoto, es) {
		out = append(out, n.nodo(k))
	}
	return out, false, nil
}

func (n ladoNube) leer(ctx context.Context, x nodo) ([]byte, error) {
	if x.tamano > maxSubida {
		return nil, errDemasiado
	}
	datos, _, err := n.p.Descargar(ctx, x.e)
	return datos, err
}

func (n ladoNube) ocupado(ctx context.Context, carpeta, nombre string) (bool, error) {
	_, es, err := n.listar(ctx, carpeta)
	if err != nil {
		return false, err
	}
	return ocupados(es)[nombre], nil
}

func (n ladoNube) crearCarpeta(ctx context.Context, padre, nombre string) (nodo, error) {
	_, remoto, ok := partirRemoto(padre)
	if !ok {
		return nodo{}, noEsta(padre)
	}
	nombre, err := nombreValido(nombre)
	if err != nil {
		return nodo{}, err
	}
	e, err := n.g.CrearCarpeta(ctx, remoto, nombre)
	if err != nil {
		return nodo{}, err
	}
	e.Carpeta = true
	n.r.apuntar(n.cuenta, e, remoto)
	return nodo{id: idRemoto(n.cuenta, e.ID), nombre: e.Nombre, carpeta: true, e: e}, nil
}

func (n ladoNube) escribir(ctx context.Context, carpeta, nombre string, datos []byte) (nodo, error) {
	_, remoto, ok := partirRemoto(carpeta)
	if !ok {
		return nodo{}, noEsta(carpeta)
	}
	nombre, err := nombreValido(nombre)
	if err != nil {
		return nodo{}, err
	}
	if len(datos) > maxSubida {
		return nodo{}, errDemasiado
	}
	e, err := n.p.Subir(ctx, remoto, nombre, datos)
	if err != nil {
		return nodo{}, err
	}
	n.r.apuntar(n.cuenta, e, remoto)
	return nodo{id: idRemoto(n.cuenta, e.ID), nombre: nombre, tamano: int64(len(datos)), e: e}, nil
}

// comprobar vuelve a listar la carpeta para ver lo que quedó en la nube: el tamaño tiene
// que ser el del original y, si el proveedor da huella, la huella también; si no la da,
// se baja otra vez y se compara entero.
func (n ladoNube) comprobar(ctx context.Context, carpeta string, x nodo, datos []byte) error {
	remoto, es, err := n.listar(ctx, carpeta)
	if err != nil {
		return err
	}
	var hallado *nube.Elemento
	for i, e := range es {
		if !e.Carpeta && ((x.e.ID != "" && e.ID == x.e.ID) || (x.e.ID == "" && e.Nombre == x.nombre)) {
			hallado = &es[i]
			break
		}
	}
	if hallado == nil {
		return errors.New("archivos: la copia no aparece en la nube")
	}
	n.r.anotar(n.cuenta, remoto, es)
	if hallado.Tamano != int64(len(datos)) {
		return fmt.Errorf("archivos: la copia mide %d bytes en la nube y el original %d", hallado.Tamano, len(datos))
	}
	alg, huella, _ := strings.Cut(hallado.Huella, ":")
	var mia string
	switch alg {
	case "md5":
		h := md5.Sum(datos) //nolint:gosec // la huella que da Google
		mia = hex.EncodeToString(h[:])
	case "sha1":
		h := sha1.Sum(datos) //nolint:gosec // la huella que da OneDrive
		mia = hex.EncodeToString(h[:])
	}
	if mia != "" {
		if !strings.EqualFold(mia, huella) {
			return errors.New("archivos: la huella de la copia en la nube no es la del original")
		}
		return nil
	}
	leidos, _, err := n.p.Descargar(ctx, *hallado)
	if err != nil {
		return err
	}
	if !bytes.Equal(leidos, datos) {
		return errors.New("archivos: la copia en la nube no es igual que el original")
	}
	return nil
}

func (n ladoNube) tirar(ctx context.Context, x nodo) error {
	if err := n.g.Borrar(ctx, x.e); err != nil {
		return err
	}
	n.r.olvidar(n.cuenta, x.e.ID)
	n.r.soltarMarcas(n.cuenta, x.e)
	return nil
}

func (n ladoNube) retirar(ctx context.Context, carpeta, nombre string) {
	_, es, err := n.listar(ctx, carpeta)
	if err != nil {
		return
	}
	for _, e := range es {
		if e.Nombre == nombre {
			_ = n.g.Borrar(ctx, e)
			n.r.olvidar(n.cuenta, e.ID)
		}
	}
}

// ----- El traslado -----

// paso es un elemento de lo que se mueve, en orden: cada carpeta antes que lo suyo.
type paso struct {
	n       nodo
	rel     string // su ruta desde el elemento movido, con barras
	padre   int    // el paso de su carpeta; -1, el de arriba
	hijos   []int  // los pasos de lo que tiene dentro
	ocultos bool   // carpeta con algo que no viaja: no se tira
	// Lo que pasó.
	copia    nodo // lo creado en el destino
	creado   bool // carpeta creada en el destino, o fichero copiado y comprobado
	completa bool // carpeta: todo lo de dentro llegó
	tirado   bool
	fallo    *NoMovido
	// porPadre: falló porque falló su carpeta; se dice la carpeta, no cada uno.
	porPadre bool
}

// cuenta dice si el paso cuenta como un elemento movido: un fichero o una carpeta vacía.
func (p paso) cuenta() bool { return !p.n.carpeta || len(p.hijos) == 0 }

// Trasladar lleva ids (ficheros o carpetas, de cualquier unidad) a la carpeta destino de
// OTRA unidad: copia, comprueba y solo entonces quita el origen (ver arriba). avance, si no
// es nil, se llama antes de cada fichero y al terminar. Un id de la misma unidad que el
// destino se mueve como siempre (Move). Devuelve error solo si no se pudo empezar (una
// unidad que no gestiona carpetas, un destino que no está); lo que falle después va en
// NoMovidos. Cancelar ctx para entre fichero y fichero (el que va termina): lo ya copiado
// se queda movido y el resto, en el origen.
func (r *Remotas) Trasladar(ctx context.Context, ids []string, destino string, avance func(AvanceTraslado)) (Traslado, error) {
	var out Traslado
	if destino == "" {
		destino = idDe(r.raiz())
	}
	ld, err := r.ladoDe(destino)
	if err != nil {
		return out, err
	}
	if _, remoto, ok := partirRemoto(destino); !ok || remoto != "" {
		if _, err := r.Folder(ctx, destino); err != nil {
			return out, err
		}
	}
	type plan struct {
		lo    lado
		pasos []paso
	}
	var planes []plan
	for _, id := range ids {
		lo, err := r.ladoDe(id)
		if err != nil {
			return Traslado{}, err
		}
		if MismaUnidad(id, destino) {
			r.moverEnUnidad(ctx, lo, id, destino, &out)
			continue
		}
		pasos, err := recorrer(ctx, lo, id)
		if err != nil {
			out.Total++
			out.NoMovidos = append(out.NoMovidos, NoMovido{Nombre: r.nombreDe(id), Motivo: motivoDe(err), Err: err})
			continue
		}
		for _, p := range pasos {
			if p.cuenta() {
				out.Total++
			}
		}
		planes = append(planes, plan{lo: lo, pasos: pasos})
	}
	hechos := len(out.Movidos) + len(out.NoMovidos)
	for _, pl := range planes {
		r.copiar(ctx, pl.lo, ld, destino, pl.pasos, func(nombre string) {
			if avance != nil {
				avance(AvanceTraslado{Hechos: hechos, Total: out.Total, Nombre: nombre})
			}
			hechos++
		})
		// Lo que haya que quitar se quita aunque se haya cancelado: lo copiado ya está
		// comprobado en el destino.
		limpio, soltar := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		r.quitarOrigen(limpio, pl.lo, ld, destino, pl.pasos)
		soltar()
		out.apuntar(pl.pasos)
	}
	if avance != nil {
		avance(AvanceTraslado{Hechos: out.Total, Total: out.Total})
	}
	out.Cancelado = ctx.Err() != nil
	return out, nil
}

// moverEnUnidad mueve id a destino dentro de la misma unidad (Move) y lo apunta en out.
func (r *Remotas) moverEnUnidad(ctx context.Context, lo lado, id, destino string, out *Traslado) {
	out.Total++
	n, err := lo.nodoDe(ctx, id)
	if err == nil {
		err = r.Move(ctx, n.clase(), id, destino)
	}
	if err != nil {
		out.NoMovidos = append(out.NoMovidos, NoMovido{Nombre: r.nombreDe(id), Motivo: motivoDe(err), Err: err})
		return
	}
	out.Movidos = append(out.Movidos, n.nombre)
}

// apuntar pasa a t lo que acabó pasando con cada paso de un plan ya copiado y quitado
// del origen: de lo que falló por su carpeta se dice solo la carpeta.
func (t *Traslado) apuntar(pasos []paso) {
	for _, p := range pasos {
		switch {
		case p.fallo != nil && !p.porPadre:
			t.NoMovidos = append(t.NoMovidos, *p.fallo)
		case p.fallo == nil && p.tirado && p.cuenta():
			t.Movidos = append(t.Movidos, p.rel)
		}
	}
}

// nombreDe es el nombre con que se ve un id, para decirlo; el id si no se sabe.
func (r *Remotas) nombreDe(id string) string {
	if EsRemoto(id) {
		if k, err := r.conocidoDe(id); err == nil {
			return k.nombre()
		}
		return id
	}
	if p, err := r.ruta(id); err == nil {
		return filepath.Base(p)
	}
	return id
}

// motivoDe clasifica un error de la copia.
func motivoDe(err error) MotivoTraslado {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return FalloCancelado
	case errors.Is(err, ErrExiste) || codigo(err) == http.StatusConflict:
		return FalloExiste
	case errors.Is(err, errDemasiado):
		return FalloTamano
	}
	return FalloCopia
}

// codigo es el estado HTTP de un error de la fachada, o 0.
func codigo(err error) int {
	var e *api.Error
	if errors.As(err, &e) {
		return e.Status
	}
	return 0
}

// recorrer da los pasos de un elemento: él y, si es carpeta, todo lo de dentro, cada
// carpeta antes que su contenido.
func recorrer(ctx context.Context, lo lado, id string) ([]paso, error) {
	n, err := lo.nodoDe(ctx, id)
	if err != nil {
		return nil, err
	}
	pasos := []paso{{n: n, rel: n.nombre, padre: -1}}
	for i := 0; i < len(pasos); i++ {
		if !pasos[i].n.carpeta {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		hs, ocultos, err := lo.hijos(ctx, pasos[i].n.id)
		if err != nil {
			return nil, err
		}
		pasos[i].ocultos = ocultos
		for _, h := range hs {
			pasos[i].hijos = append(pasos[i].hijos, len(pasos))
			pasos = append(pasos, paso{n: h, rel: path.Join(pasos[i].rel, h.nombre), padre: i})
		}
	}
	return pasos, nil
}

// copiar crea en el destino las carpetas y copia y comprueba los ficheros, en orden. Lo
// que falla se apunta en su paso; lo que cuelga de una carpeta que no se pudo crear
// falla con ella.
//
// Cancelar ctx para ANTES del siguiente: el fichero que va termina (se copia, se comprueba
// y luego se quita del origen). Cortar una subida a medias no es seguro: el servidor
// puede acabar de guardarla después de que aquí se diera por fallida y se buscara para
// retirarla, y quedaría una copia en el destino con el original aún en su sitio.
func (r *Remotas) copiar(ctx context.Context, lo, ld lado, destino string, pasos []paso, empezar func(string)) {
	op := context.WithoutCancel(ctx)
	for i := range pasos {
		p := &pasos[i]
		if p.cuenta() {
			empezar(p.rel)
		}
		donde := destino
		if p.padre >= 0 {
			padre := pasos[p.padre]
			if !padre.creado {
				m := FalloCopia
				if padre.fallo != nil && (padre.fallo.Motivo == FalloCancelado || padre.fallo.Motivo == FalloExiste) {
					m = padre.fallo.Motivo
				}
				p.fallar(m, padre.errDe())
				p.porPadre = true
				continue
			}
			donde = padre.copia.id
		}
		if err := ctx.Err(); err != nil {
			p.fallar(FalloCancelado, err)
			continue
		}
		c, m, err := copiarUno(op, lo, ld, donde, p.n, p.padre < 0)
		if err != nil {
			p.fallar(m, err)
			continue
		}
		p.copia, p.creado = c, true
	}
}

// copiarUno crea en donde la carpeta n, o copia el fichero n y comprueba que llegó entero.
// arriba es lo que se mueve: no pisa nada de lo que ya haya (dentro, las carpetas son
// nuevas). Si falla, dice el motivo. op no se cancela: lo que empieza, termina.
func copiarUno(op context.Context, lo, ld lado, donde string, n nodo, arriba bool) (nodo, MotivoTraslado, error) {
	if arriba {
		ya, err := ld.ocupado(op, donde, n.nombre)
		if err != nil {
			return nodo{}, motivoDe(err), err
		}
		if ya {
			return nodo{}, FalloExiste, conflicto(n.nombre)
		}
	}
	if n.carpeta {
		c, err := ld.crearCarpeta(op, donde, n.nombre)
		if err != nil {
			return nodo{}, motivoDe(err), err
		}
		return c, FalloCopia, nil
	}
	if n.google {
		return nodo{}, FalloDocumentoGoogle, errors.New("archivos: un documento de Google no tiene fichero que mover")
	}
	datos, err := lo.leer(op, n)
	if err != nil {
		return nodo{}, motivoDe(err), err
	}
	retirar := func() {
		limpio, soltar := context.WithTimeout(op, time.Minute)
		defer soltar()
		ld.retirar(limpio, donde, n.nombre)
	}
	c, err := ld.escribir(op, donde, n.nombre, datos)
	if err != nil {
		// Lo que dejara a medias, fuera; lo que ya estaba (un conflicto), no se toca.
		if m := motivoDe(err); m != FalloExiste {
			retirar()
		}
		return nodo{}, motivoDe(err), err
	}
	if err := ld.comprobar(op, donde, c, datos); err != nil {
		retirar()
		return nodo{}, FalloComprobacion, err
	}
	return c, FalloCopia, nil
}

// fallar apunta en el paso por qué no llegó.
func (p *paso) fallar(m MotivoTraslado, err error) {
	p.fallo = &NoMovido{Nombre: p.rel, Motivo: m, Err: err}
}

// errDe es el error del fallo de un paso, o nil.
func (p paso) errDe() error {
	if p.fallo != nil {
		return p.fallo.Err
	}
	return nil
}

// llego dice si el paso llegó entero al destino: un fichero copiado y comprobado, o una
// carpeta creada con todo lo suyo.
func (p paso) llego() bool { return p.creado && (!p.n.carpeta || p.completa) }

// quitarOrigen tira del origen lo que llegó entero, de arriba abajo: una carpeta entera
// de una vez si todo lo suyo llegó (y no tenía nada que no viaje); si no, cada fichero
// que sí. Antes, sus favoritos y etiquetas pasan a la copia. Lo que no se puede tirar se
// queda en los dos sitios y se dice. Las carpetas creadas en el destino en las que no
// llegó nada se retiran.
func (r *Remotas) quitarOrigen(ctx context.Context, lo, ld lado, destino string, pasos []paso) {
	marcarCompletas(pasos)
	for i := range pasos {
		p := &pasos[i]
		if p.padre >= 0 && pasos[p.padre].tirado {
			p.tirado = true // se fue con su carpeta
			continue
		}
		if p.padre >= 0 && pasos[p.padre].fallo != nil && pasos[p.padre].fallo.Motivo == FalloOrigen && p.fallo == nil {
			p.fallo, p.porPadre = &NoMovido{Nombre: p.rel, Motivo: FalloOrigen, Err: pasos[p.padre].fallo.Err}, true
			continue
		}
		if !p.llego() {
			continue
		}
		// La estrella y las etiquetas, a la copia: las suyas y las de lo que lleva dentro.
		r.llevarMarcasDe(ctx, pasos, i)
		if err := lo.tirar(ctx, p.n); err != nil {
			p.fallo = &NoMovido{Nombre: p.rel, Motivo: FalloOrigen, Err: err}
			continue
		}
		p.tirado = true
	}
	retirarVacias(ctx, ld, destino, pasos)
}

// marcarCompletas dice de cada carpeta si llegó con todo lo suyo, de abajo arriba (los
// hijos van siempre detrás de su carpeta).
func marcarCompletas(pasos []paso) {
	for i := len(pasos) - 1; i >= 0; i-- {
		p := &pasos[i]
		if !p.n.carpeta {
			continue
		}
		p.completa = p.creado && !p.ocultos
		for _, h := range p.hijos {
			if !pasos[h].llego() {
				p.completa = false
			}
		}
	}
}

// llevarMarcasDe pasa a la copia la estrella y las etiquetas del paso i y de lo que
// lleva dentro.
func (r *Remotas) llevarMarcasDe(ctx context.Context, pasos []paso, i int) {
	if pasos[i].creado {
		r.llevarMarcas(ctx, pasos[i].n.id, pasos[i].copia.id)
	}
	for _, h := range pasos[i].hijos {
		r.llevarMarcasDe(ctx, pasos, h)
	}
}

// retirarVacias quita del destino las carpetas creadas en las que no llegó nada; solo
// la más alta, que se lleva las de dentro.
func retirarVacias(ctx context.Context, ld lado, destino string, pasos []paso) {
	var llegoAlgo func(i int) bool
	llegoAlgo = func(i int) bool {
		if pasos[i].cuenta() {
			return pasos[i].creado
		}
		for _, h := range pasos[i].hijos {
			if llegoAlgo(h) {
				return true
			}
		}
		return false
	}
	for i := range pasos {
		p := pasos[i]
		if !p.n.carpeta || !p.creado || llegoAlgo(i) {
			continue
		}
		if p.padre >= 0 && !llegoAlgo(p.padre) {
			continue // la retira la de arriba
		}
		donde := destino
		if p.padre >= 0 {
			donde = pasos[p.padre].copia.id
		}
		ld.retirar(ctx, donde, p.copia.nombre)
	}
}

// ----- Mover y los lotes, con las unidades -----

// moverEntre es Move entre dos unidades: un traslado de un elemento. Si no se movió,
// el error dice por qué (con un 409 si en el destino ya había algo con ese nombre).
func (r *Remotas) moverEntre(ctx context.Context, id, destino string) error {
	t, err := r.Trasladar(ctx, []string{id}, destino, nil)
	if err != nil {
		return err
	}
	if len(t.NoMovidos) == 0 {
		return nil
	}
	f := t.NoMovidos[0]
	if f.Motivo == FalloExiste {
		return conflicto(f.Nombre)
	}
	if f.Err != nil {
		return f.Err
	}
	return fmt.Errorf("archivos: no se pudo mover «%s»", f.Nombre)
}

// BatchMove mueve en lote: dentro del disco como siempre; con algo de una unidad, uno a
// uno con Move (dentro de una unidad lo mueve el proveedor; entre unidades, un traslado).
// Mismas reglas que el del disco: sigue si uno falla y el primer error vuelve si no se
// movió nada.
func (r *Remotas) BatchMove(ctx context.Context, ficheros, carpetas []string, destino string) (api.DriveLote, error) {
	if !algunoRemoto(destino, ficheros, carpetas) {
		return r.Archivos.BatchMove(ctx, ficheros, carpetas, destino)
	}
	return enLote(ficheros, carpetas, func(k api.DriveKind, id string) error { return r.Move(ctx, k, id, destino) })
}

// BatchTrash tira en lote: lo de una unidad, a la papelera de su proveedor.
func (r *Remotas) BatchTrash(ctx context.Context, ficheros, carpetas []string) (api.DriveLote, error) {
	if !algunoRemoto("", ficheros, carpetas) {
		return r.Archivos.BatchTrash(ctx, ficheros, carpetas)
	}
	return enLote(ficheros, carpetas, func(k api.DriveKind, id string) error { return r.Delete(ctx, k, id) })
}

func algunoRemoto(destino string, listas ...[]string) bool {
	if EsRemoto(destino) {
		return true
	}
	for _, l := range listas {
		for _, id := range l {
			if EsRemoto(id) {
				return true
			}
		}
	}
	return false
}

func enLote(ficheros, carpetas []string, hacer func(api.DriveKind, string) error) (api.DriveLote, error) {
	var lote api.DriveLote
	var primero error
	for _, id := range ficheros {
		if err := hacer(api.DriveFile, id); err != nil {
			primero = cmpErr(primero, err)
			continue
		}
		lote.Ficheros++
	}
	for _, id := range carpetas {
		if err := hacer(api.DriveFolder, id); err != nil {
			primero = cmpErr(primero, err)
			continue
		}
		lote.Carpetas++
	}
	if lote.Total() == 0 && primero != nil {
		return lote, primero
	}
	return lote, nil
}

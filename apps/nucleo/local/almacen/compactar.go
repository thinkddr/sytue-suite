// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package almacen

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// # Compactar (PLAN-EDICION-LOCAL §5.6)
//
// El diario solo crece: cada versión de cada objeto, y lo «borrado del todo» (una nota
// purgada, una foto vaciada de la papelera) sigue en `registros.jsonl` y en `objetos/`
// aunque la app ya no lo enseñe. [Puerto.Compactar] reescribe el diario con lo VIVO y
// quita de `objetos/` lo que ya no cita nadie.
//
// Qué es lo vivo:
//
//   - de cada objeto, su ÚNICA cabeza (la revisión que ninguna otra tiene de padre), que
//     se reescribe sin padres: el motor exige que todo padre citado exista
//     (`storage.CatalogHistory`), así que una cabeza sin padres es una raíz válida. Un
//     objeto con dos cabezas (un conflicto sin resolver) se queda entero, con su historia;
//   - el contenido al que apunta cada índice vivo (un registro `content` o un fichero);
//   - y, de forma transitiva, todo registro o fichero cuya referencia (`"$…"`) aparezca
//     en un contenido vivo que sea JSON: así se conservan los ficheros retenidos de
//     Photos y de Bases (el original, las derivadas, las páginas de un álbum…) que el
//     dominio cita desde su fila, sin que este paquete conozca a ningún dominio. Lo que
//     la fila viva ya no cita (el original de una foto purgada que soltó su referencia,
//     las versiones viejas de una nota) se va.
//
// Lo que NO se compacta (ErrNoCompactable): un diario con lotes atómicos
// (`org.sytue.object.prepared.v1`/`batch.v1`) o con papeles de registro que no son
// `index` ni `content`. Hoy ningún repositorio los escribe en local; si llegan, mejor
// no tocar que adivinar.
//
// # Seguro frente a otra instancia
//
// El almacén no tenía cerrojo: dos instancias de la misma app podían abrirlo a la vez y
// las dos añadían al diario (cada una sin ver lo de la otra hasta reabrir). Ahora cada
// [AbrirPuerto] toma un cerrojo COMPARTIDO del sistema sobre `<dir>/cerrojo` (flock en
// Unix, LockFileEx en Windows: los de `nucleo/ajustes`), y Compactar lo pide EXCLUSIVO
// sin esperar: si hay otra instancia abierta, no compacta ([ErrEnUso]) y no pasa nada.
// Mientras una compacta, otra que abre espera ([EsperaCerrojo]) en vez de leer a medias.
// El Hub y la copia de seguridad leen sin cerrojo (almacen.Leer): ven el diario de antes
// o el de después, los dos enteros; en Windows, un lector con el fichero abierto hace
// fallar el renombrado y la compactación se deja para otra vez.
//
// # Atómica
//
// El diario nuevo se escribe en un temporal de la misma carpeta, se sincroniza y se
// renombra encima; solo después se borran los objetos huérfanos. Un corte antes del
// renombrado deja el diario de siempre y un temporal que se tira al abrir o al compactar
// la próxima vez; uno después deja huérfanos que la próxima compactación barre.
//
// Lo que tiene que cumplir quien llama: que el motor no tenga trabajo a medias en su
// diario transitorio (una mutación cuyo contenido aún no cita nadie): [Recursos.Compactar]
// lo mira y, si hay, no compacta ([ErrPendiente]). Tras compactar, el motor abierto en el
// mismo proceso conserva en memoria la historia vieja: no la vuelve a leer (sus cabezas
// siguen existiendo con la misma referencia), pero conviene refrescarlo
// (`RefreshPrivateData`) o compactar al cerrar ([Recursos.CompactarAlCerrar]).

var (
	// ErrEnUso: otra instancia tiene el almacén abierto, o este proceso está subiendo un
	// fichero: no se compacta ahora.
	ErrEnUso = errors.New("almacen: el almacén está en uso; no se compacta ahora")
	// ErrPendiente: el motor tiene trabajo a medias en su diario transitorio.
	ErrPendiente = errors.New("almacen: hay escrituras a medias; no se compacta ahora")
	// ErrNoCompactable: el diario tiene formas que este paquete no sabe compactar.
	ErrNoCompactable = errors.New("almacen: el historial tiene formas que no se compactan")
	// ErrCerrado: el puerto ya está cerrado.
	ErrCerrado = errors.New("almacen: el almacén está cerrado")
)

// EsperaCerrojo es cuánto espera AbrirPuerto a que otra instancia acabe de compactar.
var EsperaCerrojo = 30 * time.Second

// esperaExclusivo es cuánto insiste Compactar en el cerrojo exclusivo: si otra instancia
// está abierta, no se va a soltar enseguida, y no se espera.
const esperaExclusivo = 100 * time.Millisecond

const (
	ficheroCerrojo  = "cerrojo"
	ficheroDiario   = "registros.jsonl"
	prefijoTemporal = ".compactando-"
	prefijoSubida   = ".subida-"
)

// tomarCerrojo toma el cerrojo de dir, compartido o exclusivo, esperando hasta espera.
// Donde el sistema no da cerrojos (no hay a quién excluir), devuelve nil sin error en
// el compartido; el exclusivo falla con ErrEnUso, y así nunca se compacta a ciegas.
//
//nolint:contextcheck // la espera lleva su propio plazo (espera), no el de quien la pide: recuperar el compartido tras compactar no se puede cancelar
func tomarCerrojo(dir string, compartido bool, espera time.Duration) (func(), error) {
	f, err := os.OpenFile(filepath.Join(dir, ficheroCerrojo), os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // G304: la carpeta del almacén
	if err != nil {
		return nil, err
	}
	ctx, fin := context.WithTimeout(context.Background(), espera)
	defer fin()
	var soltar func()
	if compartido {
		soltar, err = ajustes.CerrojoFicheroCompartido(ctx, f)
	} else {
		soltar, err = ajustes.CerrojoFichero(ctx, f)
	}
	switch {
	case err == nil:
		return soltar, nil
	case errors.Is(err, context.DeadlineExceeded):
		if compartido {
			return nil, fmt.Errorf("almacen: otra instancia lleva más de %s compactando %s: %w", espera, dir, ErrEnUso)
		}
		return nil, ErrEnUso
	case compartido:
		slog.Debug("almacen: sin cerrojo del sistema", "dir", dir, "error", err)
		return nil, nil //nolint:nilnil // sin cerrojos del sistema no hay nada que soltar (ver arriba): se abre igual
	}
	return nil, ErrEnUso
}

// limpiarTemporales tira los temporales de una compactación cortada a medias. Solo se
// llama con un cerrojo cogido: nadie está escribiendo uno ahora.
func limpiarTemporales(dir string) {
	ms, _ := filepath.Glob(filepath.Join(dir, prefijoTemporal+"*"))
	for _, m := range ms {
		_ = os.Remove(m)
	}
}

// Compactado dice lo que hizo una compactación.
type Compactado struct {
	// Lineas son las del diario antes y después.
	LineasAntes, LineasDespues int
	// Bytes son los del diario antes y después.
	BytesAntes, BytesDespues int64
	// Objetos son los ficheros de `objetos/` que se han ido, y Liberados sus bytes.
	Objetos   int
	Liberados int64
}

// Algo dice si la compactación cambió algo.
func (c Compactado) Algo() bool {
	return c.LineasDespues != c.LineasAntes || c.Objetos > 0
}

// antesDeBarrer es un gancho de las pruebas: lo que pasa entre renombrar el diario y
// borrar los objetos huérfanos (un corte de luz, por ejemplo).
var antesDeBarrer = func() error { return nil }

// Compactar reescribe el diario con lo vivo y quita de `objetos/` lo que ya no cita
// nadie (ver arriba). No compacta, y lo dice con ErrEnUso, si otra instancia tiene el
// almacén abierto o este proceso está a mitad de subir un fichero. Mientras dura, el
// puerto no acepta escrituras.
func (p *Puerto) Compactar(ctx context.Context) (Compactado, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.diario == nil {
		return Compactado{}, ErrCerrado
	}
	if p.subiendo > 0 {
		return Compactado{}, ErrEnUso
	}
	// El compartido propio se suelta para poder pedir el exclusivo (son dos descriptores
	// del mismo fichero: el sistema no los distingue de dos procesos).
	p.soltarCerrojo()
	exclusivo, err := tomarCerrojo(p.dir, false, esperaExclusivo)
	if err != nil {
		p.recuperarCompartido()
		return Compactado{}, err
	}
	res, err := p.compactar(ctx)
	exclusivo()
	p.recuperarCompartido()
	// Con el cerrojo compartido de vuelta, el diario abierto y lo que hay en memoria
	// pasan a ser los del disco (también si otra instancia escribió antes del exclusivo).
	if rerr := p.recargar(); rerr != nil {
		err = errors.Join(err, rerr)
	}
	return res, err
}

// recuperarCompartido vuelve a coger el cerrojo compartido. Con p.mu.
func (p *Puerto) recuperarCompartido() {
	soltar, err := tomarCerrojo(p.dir, true, EsperaCerrojo)
	if err != nil {
		slog.Warn("almacen: no se pudo recuperar el cerrojo compartido", "dir", p.dir, "error", err)
	}
	p.soltar = soltar
}

// recargar vuelve a leer el diario y a abrirlo para añadir. Con p.mu.
func (p *Puerto) recargar() error {
	ruta := filepath.Join(p.dir, ficheroDiario)
	nuevo := &Puerto{records: map[storage.Reference]storage.Record{}, txns: map[string]storage.Reference{}, ficheros: map[storage.Reference]fichero{}, trabajos: map[string]storage.Reference{}}
	if err := nuevo.cargar(ruta); err != nil {
		return err
	}
	f, err := os.OpenFile(ruta, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // G304: el diario del almacén
	if err != nil {
		return err
	}
	if p.diario != nil {
		_ = p.diario.Close()
	}
	p.diario = f
	p.records, p.orden, p.txns, p.ficheros, p.trabajos, p.leido = nuevo.records, nuevo.orden, nuevo.txns, nuevo.ficheros, nuevo.trabajos, nuevo.leido
	// Lo leído puede ser otra cosa (lo que escribió otra instancia): el sello cambia, y
	// sigue subiendo, para que el motor vuelva a cargarlo.
	p.sello += nuevo.sello + 1
	p.generacion++
	return nil
}

// compactar hace el trabajo con el cerrojo exclusivo cogido. Con p.mu.
func (p *Puerto) compactar(ctx context.Context) (Compactado, error) {
	limpiarTemporales(p.dir)
	ruta := filepath.Join(p.dir, ficheroDiario)
	lineas, bytesAntes, err := leerDiario(ruta)
	if err != nil {
		return Compactado{}, err
	}
	res := Compactado{LineasAntes: len(lineas), BytesAntes: bytesAntes}
	pl, err := planificar(ctx, lineas, p.scope, p.rutaObjeto)
	if err != nil {
		return res, err
	}
	res.LineasDespues = len(pl.lineas)
	if len(pl.lineas) != len(lineas) || pl.reescritas > 0 {
		n, err := p.escribirDiario(ruta, pl.lineas)
		if err != nil {
			return res, err
		}
		res.BytesDespues = n
	} else {
		res.BytesDespues = bytesAntes
	}
	if err := antesDeBarrer(); err != nil {
		return res, err
	}
	res.Objetos, res.Liberados = p.barrerObjetos(pl.objetos)
	return res, ctx.Err()
}

// leerDiario lee las líneas enteras del diario (una última a medias se ignora, como al
// cargar) y su tamaño.
func leerDiario(ruta string) ([]linea, int64, error) {
	f, err := os.Open(ruta) //nolint:gosec // G304: el diario del almacén
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	var (
		out   []linea
		total int64
	)
	lector := bufio.NewReader(f)
	for {
		raw, err := lector.ReadBytes('\n')
		total += int64(len(raw))
		if len(raw) > 0 && raw[len(raw)-1] == '\n' {
			l, ok := leerLinea(raw)
			if !ok {
				return nil, 0, fmt.Errorf("almacen: %s está dañado", ruta)
			}
			out = append(out, l)
		}
		if errors.Is(err, io.EOF) {
			return out, total, nil
		}
		if err != nil {
			return nil, 0, err
		}
	}
}

// escribirDiario deja las líneas en el diario con temporal, sincronizado y renombrado.
// El diario abierto se cierra antes de renombrar (Windows no renombra encima de un
// fichero abierto) y lo vuelve a abrir recargar. Con p.mu.
func (p *Puerto) escribirDiario(ruta string, lineas []linea) (int64, error) {
	tmp, err := os.CreateTemp(p.dir, prefijoTemporal+"*")
	if err != nil {
		return 0, err
	}
	defer os.Remove(tmp.Name())
	w := bufio.NewWriterSize(tmp, 1<<20)
	var n int64
	for _, l := range lineas {
		raw, err := json.Marshal(l)
		if err != nil {
			tmp.Close()
			return 0, err
		}
		raw = append(raw, '\n')
		if _, err := w.Write(raw); err != nil {
			tmp.Close()
			return 0, err
		}
		n += int64(len(raw))
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		return 0, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return 0, err
	}
	if p.diario != nil {
		_ = p.diario.Close()
		p.diario = nil
	}
	if err := os.Rename(tmp.Name(), ruta); err != nil {
		return 0, err
	}
	sincronizarCarpeta(p.dir)
	return n, nil
}

// sincronizarCarpeta hace durable el renombrado donde el sistema lo permite.
func sincronizarCarpeta(dir string) {
	if d, err := os.Open(dir); err == nil { //nolint:gosec // G304: la carpeta del almacén
		_ = d.Sync()
		_ = d.Close()
	}
}

// barrerObjetos quita de `objetos/` lo que no está en vivos, y los restos de subidas
// cortadas (con el exclusivo y sin subidas en marcha, ninguna es de ahora).
func (p *Puerto) barrerObjetos(vivos map[string]bool) (int, int64) {
	var (
		n         int
		liberados int64
	)
	raiz := filepath.Join(p.dir, "objetos")
	_ = filepath.WalkDir(raiz, func(ruta string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return nil //nolint:nilerr // lo que no se lee no se borra
		}
		nombre := e.Name()
		if !strings.HasPrefix(nombre, prefijoSubida) && vivos[nombre] {
			return nil
		}
		if !strings.HasPrefix(nombre, prefijoSubida) && !esHuella(nombre) {
			return nil // algo que no es de este paquete: no se toca
		}
		info, ierr := e.Info()
		if os.Remove(ruta) == nil { //nolint:gosec // G122: objetos/ del almacén con el cerrojo exclusivo cogido: nadie más escribe ahí
			n++
			if ierr == nil {
				liberados += info.Size()
			}
		}
		return nil
	})
	return n, liberados
}

func esHuella(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// ----- El plan -----

// planCompactacion es el diario nuevo y las huellas de `objetos/` que siguen vivas.
type planCompactacion struct {
	lineas     []linea
	objetos    map[string]bool
	reescritas int
}

// planificar decide qué queda (ver arriba). leerObjeto da la ruta de un objeto por su
// huella, para buscar referencias dentro de los que son JSON.
//
//nolint:gocognit,gocyclo // cinco fases de corrido sobre los mismos índices del diario, cada una con su comentario: repartirlas exigiría pasar esos índices de mano en mano
func planificar(ctx context.Context, lineas []linea, scope storage.Scope, rutaObjeto func(string) string) (planCompactacion, error) {
	registros := map[storage.Reference]int{}
	ficheros := map[storage.Reference]int{}
	porEvento := map[string]storage.Reference{}
	acusados := map[string]bool{}
	for i, l := range lineas {
		switch l.Tipo {
		case "registro":
			if l.Registro == nil {
				continue
			}
			registros[l.Ref] = i
			porEvento[l.Ref.EventID] = l.Ref
		case "fichero":
			if l.Fichero == nil {
				continue
			}
			ficheros[l.Ref] = i
			porEvento[l.Ref.EventID] = l.Ref
		case "acuse":
			acusados[l.Trabajo] = true
		}
	}

	// Las versiones de cada objeto y sus cabezas.
	type version struct {
		ref    storage.Reference
		commit storage.ObjectCommit
	}
	porObjeto := map[string][]version{}
	for ref, i := range registros {
		r := lineas[i].Registro
		switch r.Role {
		case "index":
		case "content":
			continue
		default:
			return planCompactacion{}, ErrNoCompactable
		}
		var tipo struct {
			Kind string `json:"kind"`
		}
		if json.Unmarshal(r.Payload, &tipo) != nil || tipo.Kind != storage.ObjectKind {
			return planCompactacion{}, ErrNoCompactable
		}
		v, err := storage.DecodeObject(storage.RecordEntry{Reference: ref, Record: *r}, scope)
		if err != nil {
			return planCompactacion{}, fmt.Errorf("almacen: índice ilegible %s: %w", ref.EventID, err)
		}
		porObjeto[v.ObjectID] = append(porObjeto[v.ObjectID], version{ref, v.Commit})
	}

	vivos := map[storage.Reference]bool{}
	reescribir := map[storage.Reference]bool{}
	var pendientes []storage.Reference
	marcar := func(ref storage.Reference) {
		if vivos[ref] {
			return
		}
		if _, ok := registros[ref]; !ok {
			if _, ok := ficheros[ref]; !ok {
				return
			}
		}
		vivos[ref] = true
		pendientes = append(pendientes, ref)
	}
	for _, vs := range porObjeto {
		padres := map[storage.Reference]bool{}
		for _, v := range vs {
			for _, padre := range v.commit.Parents {
				padres[padre] = true
			}
		}
		var cabezas []version
		for _, v := range vs {
			if !padres[v.ref] {
				cabezas = append(cabezas, v)
			}
		}
		conservar := cabezas
		if len(cabezas) != 1 {
			conservar = vs // un conflicto (o un ciclo): se queda entero
		} else if len(cabezas[0].commit.Parents) > 0 {
			reescribir[cabezas[0].ref] = true
		}
		for _, v := range conservar {
			marcar(v.ref)
		}
	}

	// Lo que cita cada cosa viva, de forma transitiva.
	for len(pendientes) > 0 {
		if err := ctx.Err(); err != nil {
			return planCompactacion{}, err
		}
		ref := pendientes[len(pendientes)-1]
		pendientes = pendientes[:len(pendientes)-1]
		if i, ok := registros[ref]; ok {
			r := lineas[i].Registro
			if r.Role == "index" {
				var c storage.ObjectCommit
				if json.Unmarshal(r.Payload, &c) == nil && c.Content != nil {
					marcar(c.Content.Reference)
				}
				continue
			}
			for _, ev := range citasDeContenido(r.Payload) {
				if otra, ok := porEvento[ev]; ok {
					marcar(otra)
				}
			}
			continue
		}
		if i, ok := ficheros[ref]; ok {
			evs, err := citasDeObjeto(rutaObjeto(lineas[i].Fichero.SHA256))
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return planCompactacion{}, err
			}
			for _, ev := range evs {
				if otra, ok := porEvento[ev]; ok {
					marcar(otra)
				}
			}
		}
	}

	// El diario nuevo, en el orden de siempre. Solo cuenta la última línea de cada
	// referencia, como al cargar.
	pl := planCompactacion{objetos: map[string]bool{}}
	for i, l := range lineas {
		switch l.Tipo {
		case "registro":
			if l.Registro == nil || !vivos[l.Ref] || registros[l.Ref] != i {
				continue
			}
			if reescribir[l.Ref] {
				r, err := sinPadres(*l.Registro)
				if err != nil {
					return planCompactacion{}, err
				}
				l.Registro = &r
				pl.reescritas++
			}
			pl.lineas = append(pl.lineas, l)
		case "fichero":
			if l.Fichero == nil || !vivos[l.Ref] || ficheros[l.Ref] != i {
				continue
			}
			if acusados[l.Trabajo] {
				l.Trabajo = ""
			}
			pl.objetos[l.Fichero.SHA256] = true
			pl.lineas = append(pl.lineas, l)
		}
	}
	return pl, nil
}

// sinPadres es la cabeza r convertida en raíz: el mismo compromiso sin padres.
func sinPadres(r storage.Record) (storage.Record, error) {
	var c storage.ObjectCommit
	if err := json.Unmarshal(r.Payload, &c); err != nil {
		return r, err
	}
	c.Parents = nil
	payload, err := json.Marshal(c)
	if err != nil {
		return r, err
	}
	r.Payload = payload
	r.Previous = nil
	return r, nil
}

// citasDeContenido son las referencias que cita la carga de un registro `content`: el
// fichero de un enlace (`storage.FileLink`) o las que aparezcan en el JSON del dominio.
func citasDeContenido(payload []byte) []string {
	var envoltorio struct {
		Kind    string                  `json:"kind"`
		Content *storage.ContentPointer `json:"content"`
		JSON    []byte                  `json:"json"`
	}
	if json.Unmarshal(payload, &envoltorio) != nil {
		return citas(payload)
	}
	out := citas(payload)
	if envoltorio.Kind == storage.FileLinkKind && envoltorio.Content != nil {
		out = append(out, envoltorio.Content.Reference.EventID)
	}
	return append(out, citas(envoltorio.JSON)...)
}

// citasDeObjeto son las referencias que aparecen en un objeto que sea JSON (el contenido
// grande de una fila, el índice de un álbum de Photos…). Un binario (una foto, un
// adjunto) no empieza por «{» ni por «[» y no se lee entero.
func citasDeObjeto(ruta string) ([]string, error) {
	f, err := os.Open(ruta) //nolint:gosec // G304: objeto por su huella
	if err != nil {
		return nil, err
	}
	defer f.Close()
	lector := bufio.NewReaderSize(f, 64<<10)
	for {
		b, err := lector.ReadByte()
		if err != nil {
			return nil, nil //nolint:nilerr // vacío o ilegible: no cita nada
		}
		if b == ' ' || b == '\n' || b == '\r' || b == '\t' {
			continue
		}
		if b != '{' && b != '[' {
			return nil, nil
		}
		break
	}
	return citasDe(lector)
}

// citas son las cadenas JSON que empiezan por «$» en raw: las referencias de evento del
// puerto («$<uuid>»). Una coincidencia de más solo conserva algo de más.
func citas(raw []byte) []string {
	out, _ := citasDe(bufio.NewReader(bytes.NewReader(raw)))
	return out
}

func citasDe(r *bufio.Reader) ([]string, error) {
	var out []string
	var anterior byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return out, err
		}
		if anterior == '"' && b == '$' {
			var cita bytes.Buffer
			cita.WriteByte('$')
			for cita.Len() <= 255 {
				c, err := r.ReadByte()
				if err != nil || c == '"' || c == '\\' {
					break
				}
				cita.WriteByte(c)
			}
			out = append(out, cita.String())
			anterior = 0
			continue
		}
		anterior = b
	}
}

// ----- Desde los recursos -----

// Compactar compacta el almacén si el motor no tiene trabajo a medias en su diario
// transitorio (si lo tiene, ErrPendiente: se hará otra vez).
func (r *Recursos) Compactar(ctx context.Context) (Compactado, error) {
	if r == nil || r.Puerto == nil {
		return Compactado{}, ErrCerrado
	}
	if r.Config.Journal != nil {
		foto, err := r.Config.Journal.Snapshot(ctx)
		if err != nil {
			return Compactado{}, err
		}
		if foto.Records > 0 {
			return Compactado{}, ErrPendiente
		}
	}
	return r.Puerto.Compactar(ctx)
}

// ----- En el acto -----

// EsperaCompactarYa es cuánto se espera desde la última petición de CompactarYa antes de
// compactar: vaciar una papelera borra una cosa tras otra, y así se compacta una vez al
// acabar y no a cada paso (cada compactación para las escrituras mientras dura).
var EsperaCompactarYa = 2 * time.Second

// compactador lleva las compactaciones en el acto de unos Recursos: como mucho una
// rutina a la vez, y lo que se pide mientras tanto se junta en una vuelta más.
type compactador struct {
	mu        sync.Mutex
	pedida    time.Time // la última petición sin atender; cero, ninguna
	refrescar func(context.Context) error
	corre     bool
	parado    bool
	parar     chan struct{}
	vivo      sync.WaitGroup
	// hechas cuenta las compactaciones intentadas (para las pruebas).
	hechas int
}

// CompactarYa compacta el almacén EN EL ACTO tras «borrar del todo» (§5.6), en segundo
// plano: lo borrado se va del disco ya y no al cerrar la app. Las peticiones seguidas se
// juntan (EsperaCompactarYa). Si otra instancia tiene el almacén abierto o el motor tiene
// trabajo a medias (ErrEnUso, ErrPendiente), no pasa nada: se hará al cerrar
// (CompactarAlCerrar). Si cambió algo, refrescar —el RefreshPrivateData del repositorio
// abierto; nil, nada— pone su catálogo al día, para que no recuerde versiones que ya no
// están. No bloquea.
func (r *Recursos) CompactarYa(refrescar func(context.Context) error) {
	if r == nil {
		return
	}
	c := &r.ya
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.parado {
		return
	}
	c.pedida, c.refrescar = time.Now(), refrescar
	if c.corre {
		return
	}
	if c.parar == nil {
		c.parar = make(chan struct{})
	}
	c.corre = true
	c.vivo.Add(1)
	go r.compactarYa()
}

// compactarYa es la rutina de CompactarYa: espera a que dejen de llegar peticiones,
// compacta y repite mientras haya alguna sin atender.
func (r *Recursos) compactarYa() {
	c := &r.ya
	defer c.vivo.Done()
	for {
		c.mu.Lock()
		if c.parado || c.pedida.IsZero() {
			c.corre = false
			c.mu.Unlock()
			return
		}
		if falta := EsperaCompactarYa - time.Since(c.pedida); falta > 0 {
			parar := c.parar
			c.mu.Unlock()
			t := time.NewTimer(falta)
			select {
			case <-parar:
				t.Stop()
			case <-t.C:
			}
			continue
		}
		c.pedida, c.hechas = time.Time{}, c.hechas+1
		refrescar := c.refrescar
		c.mu.Unlock()
		r.compactarUnaVez(refrescar)
	}
}

func (r *Recursos) compactarUnaVez(refrescar func(context.Context) error) {
	producto := r.Config.Scope.Product
	ctx, fin := context.WithTimeout(context.Background(), 5*time.Minute)
	defer fin()
	res, err := r.Compactar(ctx)
	switch {
	case errors.Is(err, ErrEnUso) || errors.Is(err, ErrPendiente):
		slog.Debug("almacen: se compactará al cerrar", "producto", producto, "motivo", err)
		return
	case err != nil:
		slog.Warn("almacen: no se pudo compactar", "producto", producto, "error", err)
		return
	}
	if !res.Algo() {
		return
	}
	slog.Info("almacen: compactado en el acto", "producto", producto,
		"lineas", res.LineasAntes, "quedan", res.LineasDespues, "objetos", res.Objetos, "liberados", res.Liberados)
	if refrescar != nil {
		if err := refrescar(ctx); err != nil {
			slog.Warn("almacen: refrescar tras compactar", "producto", producto, "error", err)
		}
	}
}

// PararCompactar deja de compactar en el acto: lo pedido y aún sin empezar se olvida (lo
// hará CompactarAlCerrar) y se espera a la compactación en marcha. Se llama antes de
// cerrar el repositorio, que la compactación refresca; Cerrar también la llama.
func (r *Recursos) PararCompactar() {
	if r == nil {
		return
	}
	c := &r.ya
	c.mu.Lock()
	if !c.parado {
		c.parado = true
		if c.parar != nil {
			close(c.parar)
		}
	}
	c.mu.Unlock()
	c.vivo.Wait()
}

// CerrojoCompartido toma el cerrojo compartido del almacén de dir, el mismo que tiene
// cada instancia abierta: mientras se tiene, nadie lo compacta (y quien compacte deja
// de hacerlo con ErrEnUso). Es para quien copia el diario y los objetos sin abrir el
// almacén —la copia de seguridad—, para que no se lleve un objeto que una compactación
// borra a mitad. Hay que llamar a soltar al acabar.
func CerrojoCompartido(dir string) (soltar func(), err error) {
	s, err := tomarCerrojo(dir, true, EsperaCerrojo)
	if s == nil {
		s = func() {}
	}
	return s, err
}

// EsAlmacen dice si dir es la carpeta de un almacén (tiene su diario).
func EsAlmacen(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, ficheroDiario))
	return err == nil && fi.Mode().IsRegular()
}

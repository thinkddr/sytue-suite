// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"bufio"
	"compress/flate"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

// # Subir y bajar, con su progreso
//
// El trozo del sistema —dónde dejar una descarga sin pisar nada, cómo enseñarla en el
// gestor de ficheros y cómo abrirla con el visor asociado— es `apps/nucleo/ficheros`, y
// se IMPORTA: está hecho una vez para las quince apps de escritorio.
//
// El progreso se lleva en `Progreso`, y lleva cerrojo a propósito: quien avanza es la
// rutina de la transferencia y quien lo pinta es la principal. El fallo que esto evita
// está medido en este repositorio —`fyne.Do` no marshalla nada en el driver de pruebas,
// así que un temporizador que toca estado pintado es carrera de verdad y tumba a un test
// cualquiera, no al suyo—.

// Instantanea es la foto del progreso en un momento, ya sin cerrojo, para pintarla.
type Instantanea struct {
	// Actual es el que va, contando desde 1 («Subiendo 1/3»), y Completos cuántos han
	// terminado, que es lo que cuenta la barra (el `done` de la web).
	Actual, Total, Completos int
	Nombre                   string
	// Parte es lo que lleva hecho el que está en curso, de 0 a 1.
	Parte float32
}

// Fraccion es lo que la barra tiene que rellenar: los completos más lo que lleva el de
// ahora, sobre el total. Es la cuenta de la web,
// `(current + (fraction ?? 0)) / total`.
func (i Instantanea) Fraccion() float32 {
	if i.Total <= 0 {
		return 0
	}
	return (float32(i.Completos) + i.Parte) / float32(i.Total)
}

// Progreso es el estado compartido de una transferencia.
type Progreso struct {
	// Bajada dice que es una descarga: la franja enseña el nombre y no «Subiendo n/m».
	// Se fija al crearlo y no cambia, así que no necesita el cerrojo.
	Bajada bool

	mu     sync.Mutex
	actual int
	total  int
	nombre string
	parte  float32
}

// NuevoProgreso arranca el progreso de `total` ficheros.
func NuevoProgreso(total int) *Progreso { return &Progreso{total: total} }

// Empezar pasa al fichero `i` (contando desde 0) y pone su parte a cero.
func (p *Progreso) Empezar(i int, nombre string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.actual, p.nombre, p.parte = i, nombre, 0
}

// Avanzar fija lo que lleva hecho el fichero en curso.
func (p *Progreso) Avanzar(parte float32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.parte = min(max(parte, 0), 1)
}

// Terminar da por hecho el fichero en curso.
func (p *Progreso) Terminar() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.actual, p.parte = p.actual+1, 0
}

// Instantanea es la foto para pintar. Se llama desde la rutina principal.
func (p *Progreso) Instantanea() Instantanea {
	p.mu.Lock()
	defer p.mu.Unlock()
	// `current` se enseña contando desde 1: «Subiendo 1/3», no «0/3».
	return Instantanea{Actual: min(p.actual+1, p.total), Total: p.total, Completos: p.actual, Nombre: p.nombre, Parte: p.parte}
}

// Hechos es cuántos van completos, para el resumen del final.
func (p *Progreso) Hechos() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.actual
}

// avanzar adapta la fracción de ficheros.ConProgreso a la barra de la transferencia.
func avanzar(p *Progreso) func(float64) { return func(f float64) { p.Avanzar(float32(f)) } }

// Entrada es un fichero que se va a subir y la subcarpeta —relativa a la de destino, con
// barras— en la que tiene que quedar. Vacía: en la de destino.
//
// Es una RUTA del disco o un FLUJO: lo que da el selector del móvil, que en Android es un
// `content://` que no se abre con `os.Open` sino por el proveedor de contenido. Con Flujo,
// Nombre es el que dice el proveedor y Tamano lo que mide, o -1 si no lo dice; con -1 la
// subida va troceada y la barra no inventa un avance.
type Entrada struct {
	Ruta string
	Dir  string

	Flujo  io.ReadCloser
	Nombre string
	Tamano int64
}

// Fluido es una entrada que llega como flujo, sin ruta.
func Fluido(nombre string, tamano int64, r io.ReadCloser) Entrada {
	return Entrada{Flujo: r, Nombre: nombre, Tamano: tamano}
}

// Rutas son entradas de ficheros del disco, sin recorrer carpetas.
func Rutas(rutas []string) []Entrada {
	out := make([]Entrada, 0, len(rutas))
	for _, r := range rutas {
		out = append(out, Entrada{Ruta: r})
	}
	return out
}

// nombre es el que se queda el fichero en Drive.
func (e Entrada) nombre() string {
	if e.Flujo != nil {
		return filepath.Base(e.Nombre)
	}
	return filepath.Base(e.Ruta)
}

// abrir da el contenido y lo que mide (-1: no se sabe). Cerrarlo es de quien llama.
func (e Entrada) abrir() (io.ReadCloser, int64, error) {
	if e.Flujo != nil {
		return e.Flujo, e.Tamano, nil
	}
	f, err := os.Open(e.Ruta) //nolint:gosec // la ruta la eligió la persona en el diálogo del sistema
	if err != nil {
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	return f, info.Size(), nil
}

// cerrar suelta los flujos de unas entradas que no se van a subir.
func cerrar(es []Entrada) {
	for _, e := range es {
		if e.Flujo != nil {
			_ = e.Flujo.Close()
		}
	}
}

// Subir sube los ficheros del equipo a una carpeta de Drive, uno detrás de otro.
//
// De uno en uno y no en paralelo a propósito: el progreso que la web enseña es «1 de 3»
// con el nombre del que va, y con tres subidas a la vez ese texto no significa nada.
// Además una subida en paralelo con una cuota casi llena falla por la mitad y deja al
// usuario sin saber cuáles entraron.
//
// Devuelve cuántos subieron y el primer error, si lo hubo: la web tiene una clave para
// la subida parcial (`files.upload.partial`) justamente porque esto puede pasar.
func Subir(ctx context.Context, svc *api.Drive, carpeta string, rutas []string, p *Progreso) (int, error) {
	return SubirArbol(ctx, svc, carpeta, Rutas(rutas), p)
}

// SubirArbol es Subir con subcarpetas: lo que hace la web al soltar una CARPETA, que
// recrea su árbol dentro de Drive. Las subcarpetas se crean antes, de las someras a las
// profundas, y una que ya existe (409) se reutiliza: soltar dos veces lo mismo no es un
// fallo. Si una no se puede crear, sus ficheros fallan y lo dice el resumen.
func SubirArbol(ctx context.Context, svc *api.Drive, carpeta string, entradas []Entrada, p *Progreso) (int, error) {
	if carpeta == "" {
		raiz, err := svc.Root(ctx)
		if err != nil {
			cerrar(entradas)
			return 0, err
		}
		carpeta = raiz.ID
	}
	ids := map[string]string{"": carpeta}
	var primero error
	for _, dir := range Subcarpetas(entradas) {
		padre, ok := ids[path.Dir(dir)]
		if path.Dir(dir) == "." {
			padre, ok = carpeta, true
		}
		if !ok {
			continue
		}
		id, err := AsegurarCarpeta(ctx, svc, padre, path.Base(dir))
		if err != nil {
			if primero == nil {
				primero = err
			}
			continue
		}
		ids[dir] = id
	}
	subidos := 0
	for i, e := range entradas {
		p.Empezar(i, e.nombre())
		destino, ok := ids[e.Dir]
		err := primero
		if ok {
			err = subirUno(ctx, svc, destino, e, p) // cierra el flujo
		} else if e.Flujo != nil {
			_ = e.Flujo.Close()
		}
		p.Terminar()
		if err != nil || !ok {
			if primero == nil {
				primero = err
			}
			continue
		}
		subidos++
	}
	if subidos == len(entradas) {
		return subidos, nil
	}
	return subidos, primero
}

// Subcarpetas son las carpetas que hay que crear para unas entradas, con sus antecesoras
// y sin repetir, de las someras a las profundas.
func Subcarpetas(entradas []Entrada) []string {
	vistas := map[string]bool{}
	var out []string
	for _, e := range entradas {
		acc := ""
		for _, seg := range strings.Split(e.Dir, "/") {
			if seg == "" {
				continue
			}
			acc = path.Join(acc, seg)
			if !vistas[acc] {
				vistas[acc] = true
				out = append(out, acc)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b string) int { return strings.Count(a, "/") - strings.Count(b, "/") })
	return out
}

// AsegurarCarpeta crea una carpeta o, si ya hay una con ese nombre (409), devuelve la
// que hay. Las carpetas llegan enteras en la primera página de la carpeta padre.
func AsegurarCarpeta(ctx context.Context, svc *api.Drive, padre, nombre string) (string, error) {
	f, err := svc.CreateFolder(ctx, padre, nombre)
	if err == nil {
		return f.ID, nil
	}
	var e *api.Error
	if !errors.As(err, &e) || e.Status != http.StatusConflict {
		return "", err
	}
	hijos, err := svc.Children(ctx, padre, "")
	if err != nil {
		return "", err
	}
	for _, c := range hijos.Folders {
		if c.Name == nombre {
			return c.ID, nil
		}
	}
	return "", err
}

// EntradasDe convierte lo soltado en la ventana en entradas: un fichero va a la carpeta
// abierta, y una CARPETA se recorre entera y cada fichero lleva su subcarpeta, empezando
// por el nombre de la carpeta soltada.
func EntradasDe(rutas []string) []Entrada {
	var out []Entrada
	for _, r := range rutas {
		info, err := os.Stat(r)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			out = append(out, Entrada{Ruta: r})
			continue
		}
		base := filepath.Dir(r)
		_ = filepath.WalkDir(r, func(ruta string, d fs.DirEntry, err error) error {
			// Lo que no se puede leer se salta sin parar el paseo: es lo que hace la
			// web con las entradas que el navegador no deja abrir.
			// Solo ficheros NORMALES: un enlace simbólico dentro de la carpeta no se
			// sigue. «notas.txt -> ../../.ssh/id_ed25519» subía la clave privada con
			// otro nombre (FYS-01). Es lo que hace Chrome desde CVE-2022-3656.
			if err == nil && d.Type().IsRegular() {
				if rel, e := filepath.Rel(base, filepath.Dir(ruta)); e == nil {
					out = append(out, Entrada{Ruta: ruta, Dir: filepath.ToSlash(rel)})
				}
			}
			return nil
		})
	}
	return out
}

func subirUno(ctx context.Context, svc *api.Drive, carpeta string, e Entrada, p *Progreso) error {
	r, tamano, err := e.abrir()
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	// Sin tamaño no avisa: la barra se llena al terminar este fichero. Y un aviso por
	// punto porcentual, no uno por bloque leído, que inundaba la rutina de la interfaz.
	cuerpo := ficheros.ConProgreso(r, tamano, avanzar(p))
	nombre := e.nombre()
	_, err = svc.Upload(ctx, carpeta, nombre, TipoDe(nombre), cuerpo, tamano)
	return err
}

// TipoDe adivina el `Content-Type` por la extensión. Sin extensión conocida va el
// genérico, que es lo que hace el cliente: el servicio lo vuelve a mirar por su cuenta.
func TipoDe(nombre string) string {
	if t := mime.TypeByExtension(filepath.Ext(nombre)); t != "" {
		return t
	}
	return "application/octet-stream"
}

// Descargar trae un fichero a la carpeta de descargas del sistema y devuelve dónde ha
// quedado.
//
// No pisa nada: `ficheros.Guardar` renombra a «informe (1).pdf» si ya existe. Es la
// mitad de la promesa de que los documentos de la gente no se corrompen nunca —la otra
// mitad es que esta app no escribe encima de un fichero de Drive que no entienda—.
func Descargar(ctx context.Context, svc *api.Drive, el Elemento, p *Progreso) (string, error) {
	cuerpo, err := svc.Download(ctx, el.ID)
	if err != nil {
		return "", err
	}
	defer func() { _ = cuerpo.Close() }()

	carpeta, err := ficheros.Carpeta()
	if err != nil {
		return "", err
	}
	var r io.Reader = cuerpo
	if p != nil && el.Tamano > 0 {
		r = ficheros.ConProgreso(cuerpo, el.Tamano, avanzar(p))
	}
	return ficheros.Guardar(carpeta, el.Nombre, r)
}

// NombreZip es el nombre con el que el servicio sirve un ZIP (`zipDownloadName` de
// services/drive): la base recortada, 120 bytes como mucho, «sytue-drive» si no hay
// ninguna, y siempre con su `.zip`. Se repite aquí porque la descarga se guarda con
// ESTE nombre y el cliente no lee la cabecera.
func NombreZip(base string) string {
	base = strings.TrimSuffix(strings.TrimSpace(base), ".zip")
	if base == "" {
		base = "sytue-drive"
	}
	if len(base) > 120 {
		base = base[:120]
	}
	return base + ".zip"
}

// DescargarZip trae una selección empaquetada en ZIP a la carpeta de descargas y
// devuelve dónde ha quedado. Sin pisar nada, como Descargar.
//
// El ZIP va comprimido y en streaming: su tamaño no se sabe hasta el final. Lo que sí da
// el servicio es cuánto suman sus ficheros SIN comprimir, así que el progreso se cuenta
// por ahí: mientras se guarda, una copia del flujo se descomprime aparte y se va sumando
// (`ContarZip`). Sin ese total, la barra se queda vacía, que es no inventarse nada.
func DescargarZip(ctx context.Context, svc *api.Drive, idsFicheros, idsCarpetas []string, nombre string, p *Progreso) (string, error) {
	cuerpo, contenido, err := svc.Zip(ctx, idsFicheros, idsCarpetas, nombre)
	if err != nil {
		return "", err
	}
	defer func() { _ = cuerpo.Close() }()
	carpeta, err := ficheros.Carpeta()
	if err != nil {
		return "", err
	}
	if p == nil || contenido <= 0 {
		return ficheros.Guardar(carpeta, NombreZip(nombre), cuerpo)
	}
	lector, escritor := io.Pipe()
	hecho := make(chan struct{})
	go func() {
		defer close(hecho)
		var leido int64
		// Un ZIP que no se deja contar no es un fallo de la descarga: se deja de contar
		// y se sigue guardando.
		_ = ContarZip(lector, func(n int64) {
			leido += n
			p.Avanzar(float32(leido) / float32(contenido))
		})
		_, _ = io.Copy(io.Discard, lector)
	}()
	ruta, err := ficheros.Guardar(carpeta, NombreZip(nombre), io.TeeReader(cuerpo, escritor))
	_ = escritor.CloseWithError(err)
	<-hecho
	return ruta, err
}

// Firmas de un ZIP (APPNOTE 4.3.7, 4.3.9).
const (
	firmaLocal       = 0x04034b50
	firmaDescriptor  = 0x08074b50
	tope32           = 1<<32 - 1
	metodoGuardado   = 0
	metodoDesinflado = 8
)

// errZipIncontable es un ZIP que no se puede seguir en streaming: un fichero sin
// comprimir y sin tamaño en su cabecera no dice dónde acaba.
var errZipIncontable = errors.New("drive: el ZIP no se puede contar en streaming")

// ContarZip lee un ZIP en streaming, de cabecera local en cabecera local, y va diciendo
// cuántos bytes DESCOMPRIMIDOS lleva cada tramo. Se para en el directorio central.
//
// El servicio escribe cada fichero comprimido y con descriptor de datos detrás
// (`archive/zip`), así que el tamaño comprimido no está en la cabecera: se sabe dónde
// acaba descomprimiéndolo. `flate` no lee ni un byte de más si se le da un
// `io.ByteReader`, que es lo que hace que el descriptor y la cabecera siguiente se
// encuentren donde están.
func ContarZip(r io.Reader, avanzar func(int64)) error {
	br := bufio.NewReader(r)
	var cab [30]byte
	contar := contarEscritor(avanzar)
	for {
		if _, err := io.ReadFull(br, cab[:4]); err != nil {
			return err
		}
		if binary.LittleEndian.Uint32(cab[:4]) != firmaLocal {
			return nil // el directorio central: ya no hay más ficheros
		}
		if _, err := io.ReadFull(br, cab[4:]); err != nil {
			return err
		}
		opciones := binary.LittleEndian.Uint16(cab[6:])
		metodo := binary.LittleEndian.Uint16(cab[8:])
		comprimido := int64(binary.LittleEndian.Uint32(cab[18:]))
		saltar := int64(binary.LittleEndian.Uint16(cab[26:])) + int64(binary.LittleEndian.Uint16(cab[28:]))
		if _, err := br.Discard(int(saltar)); err != nil {
			return err
		}
		conDescriptor := opciones&0x8 != 0
		var n int64
		var err error
		switch {
		case metodo == metodoDesinflado:
			n, err = io.Copy(contar, flate.NewReader(br))
		case metodo == metodoGuardado && !conDescriptor:
			n, err = io.CopyN(contar, br, comprimido)
		default:
			return errZipIncontable
		}
		if err != nil {
			return err
		}
		if conDescriptor {
			if err := saltarDescriptor(br, n); err != nil {
				return err
			}
		}
	}
}

// saltarDescriptor se come el descriptor de datos que va detrás de un fichero: la firma
// (opcional según la norma, y que `archive/zip` siempre pone), el CRC y los dos tamaños,
// de 8 bytes cada uno si el fichero pasa de 4 GB.
func saltarDescriptor(br *bufio.Reader, descomprimido int64) error {
	var b [4]byte
	if _, err := io.ReadFull(br, b[:]); err != nil {
		return err
	}
	resto := 8
	if descomprimido >= tope32 {
		resto = 16
	}
	if binary.LittleEndian.Uint32(b[:]) == firmaDescriptor {
		resto += 4 // era la firma: falta el CRC
	}
	_, err := br.Discard(resto)
	return err
}

// contarEscritor tira lo que recibe y dice cuánto era.
type contarEscritor func(int64)

func (c contarEscritor) Write(b []byte) (int, error) {
	c(int64(len(b)))
	return len(b), nil
}

// DescargarCompartido baja un fichero de una raíz compartida contigo —el propio fichero
// si la raíz lo es, con `subcamino` vacío— a la carpeta de descargas, sin pisar nada.
func DescargarCompartido(ctx context.Context, svc *api.Drive, raiz, subcamino, nombre string) (string, error) {
	cuerpo, err := svc.SharedContent(ctx, raiz, subcamino)
	if err != nil {
		return "", err
	}
	defer func() { _ = cuerpo.Close() }()
	carpeta, err := ficheros.Carpeta()
	if err != nil {
		return "", err
	}
	return ficheros.Guardar(carpeta, nombre, cuerpo)
}

// subirACompartidaUno sube un fichero a una carpeta compartida.
func subirACompartidaUno(ctx context.Context, svc *api.Drive, raiz, subcamino string, e Entrada) error {
	r, tamano, err := e.abrir()
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	nombre := e.nombre()
	_, err = svc.SharedUpload(ctx, raiz, subcamino, nombre, TipoDe(nombre), r, tamano)
	return err
}

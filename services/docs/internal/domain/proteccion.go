// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

// Protección de hoja y de libro de Calcus CUMPLIDA EN EL SERVIDOR (ROADMAP-OFFICE
// permisos-1). Hasta ahora SheetProtection/WorkbookProtection solo las aplicaba el
// editor: cualquier editor las saltaba con un POST directo a /ops. Aquí vive la regla
// pura; el almacén la ejecuta dentro de la transacción que bloquea el documento
// (store.AppendOps), de modo que POST /ops, WebSocket y la restauración de versiones
// —las tres únicas vías de escritura— pasan por la misma frontera.
//
// Decisión del dueño (2026-10-01):
//   - el propietario siempre puede quitar o cambiar la protección;
//   - un editor solo con la contraseña, verificada AQUÍ;
//   - la contraseña se guarda solo como hash con sal. Nunca vuelve al cliente: ni en
//     el estado, ni en las operaciones que se sirven (Redactar*).
//
// Hash: el esquema de OOXML (algorithmName SHA-512, saltValue, spinCount) para que
// viaje tal cual en un XLSX (<sheetProtection algorithmName=… hashValue=…
// saltValue=… spinCount=…/>). No es un KDF de memoria dura, pero 100 000 vueltas de
// SHA-512 (~40 ms) frenan el intento en línea, y la verificación está limitada por
// documento y persona (limitadorClaves). Elegirlo evita mantener dos formatos.
//
// Alcance de las reglas, deliberadamente el MISMO que aplica el editor
// (SheetCanvas.tsx: PROTECT_CONTENT_CMDS / PROTECT_FORMAT_CMDS /
// PROTECT_PERMISSION_CMDS): contenido de celdas bloqueadas, bloqueo y ocultación de
// celdas, formato (si no se concedió), y los campos que gobierna cada permiso del
// cuadro «Proteger hoja». El resto del modelo (congelar, esquema, estilo de tabla…)
// es vista y queda libre, como en el editor.

var (
	// ErrProtegido: la escritura toca algo protegido y quien escribe no es propietario
	// ni presentó la contraseña. El mensaje no revela contenido ni nombres.
	ErrProtegido = errors.New("docs: el contenido está protegido")
	// ErrClaveProteccion: se presentó una contraseña y no es la de la protección.
	ErrClaveProteccion = errors.New("docs: la contraseña de protección no es correcta")
	// ErrProteccionPendiente: el documento aún no tiene su estado de protección
	// inicializado; el servicio lo inicializa y reintenta (no llega al cliente).
	ErrProteccionPendiente = errors.New("docs: estado de protección sin inicializar")
)

// claveHoja es el registro LWW que guarda el modelo completo de Calcus
// (JSON.stringify(SheetModel)).
const claveHoja = "doc:sheet"

// AmbitoLibro identifica la protección de estructura del libro.
const AmbitoLibro = "libro"

// AmbitoHoja identifica la protección de una hoja por su nombre.
func AmbitoHoja(nombre string) string { return "hoja:" + nombre }

// Proteccion es lo que presenta quien escribe, FUERA de las operaciones (para que la
// contraseña jamás entre en el registro de operaciones): la contraseña vigente con la
// que se autoriza y las contraseñas nuevas por ámbito. Viaja en el cuerpo de
// POST /ops, del mensaje WS «ops» y de la restauración de versiones.
type Proteccion struct {
	Clave  string            `json:"password,omitempty"`
	Nuevas map[string]string `json:"new,omitempty"`
	// Hashes son contraseñas ya derivadas por ámbito: las que trae un fichero importado
	// (sheetProtection de un .xlsx exportado por Sytue). Equivale a poner la contraseña
	// de la que salió el hash, con las mismas reglas que Nuevas: de cualquier editor en un
	// ámbito sin proteger, y del propietario o quien presenta la contraseña en uno protegido.
	Hashes map[string]ClaveWire `json:"hashes,omitempty"`
	// Quien es la persona que escribe, para los rangos editables por persona o grupo.
	// La pone el almacén desde la sesión; nunca se lee del cuerpo de la petición.
	Quien Subject `json:"-"`
	// SinIntentos: esa persona ha agotado sus intentos de contraseña en este documento
	// (limitadorClaves) y en este lote no se comprueba ninguna, venga por donde venga.
	// Lo pone el servicio; nunca se lee del cuerpo de la petición.
	SinIntentos bool `json:"-"`
}

type ctxProteccion struct{}

// ConProteccion adjunta al contexto lo que presenta quien escribe.
func ConProteccion(ctx context.Context, p *Proteccion) context.Context {
	if p == nil || (p.Clave == "" && len(p.Nuevas) == 0 && len(p.Hashes) == 0) {
		return ctx
	}
	return context.WithValue(ctx, ctxProteccion{}, *p)
}

// ProteccionDe devuelve lo adjuntado con ConProteccion (cero si nada).
func ProteccionDe(ctx context.Context) Proteccion {
	p, _ := ctx.Value(ctxProteccion{}).(Proteccion)
	return p
}

// --- Hash con sal (esquema OOXML) ---

const (
	claveAlgoritmo  = "SHA-512"
	claveVueltas    = 100000
	claveSalBytes   = 16
	claveMaxVueltas = 10_000_000 // tope defensivo ante una fila corrupta
)

// ClaveHash es la contraseña ya derivada. Los nombres de campo siguen a OOXML.
type ClaveHash struct {
	Algoritmo string // algorithmName
	Sal       []byte // saltValue
	Vueltas   int    // spinCount
	Hash      []byte // hashValue
}

// HashClave deriva la contraseña con sal aleatoria nueva.
func HashClave(clave string) (ClaveHash, error) {
	sal := make([]byte, claveSalBytes)
	if _, err := rand.Read(sal); err != nil {
		return ClaveHash{}, fmt.Errorf("docs: sal de protección: %w", err)
	}
	return ClaveHash{
		Algoritmo: claveAlgoritmo, Sal: sal, Vueltas: claveVueltas,
		Hash: derivarClaveOOXML(sal, clave, claveVueltas),
	}, nil
}

// Coincide comprueba la contraseña en tiempo constante.
func (c ClaveHash) Coincide(clave string) bool {
	if c.Algoritmo != claveAlgoritmo || c.Vueltas < 0 || c.Vueltas > claveMaxVueltas || len(c.Hash) == 0 {
		return false
	}
	return subtle.ConstantTimeCompare(c.Hash, derivarClaveOOXML(c.Sal, clave, c.Vueltas)) == 1
}

// derivarClaveOOXML: H0 = SHA512(sal ‖ UTF-16LE(clave)); Hn = SHA512(Hn-1 ‖ n en uint32 LE).
func derivarClaveOOXML(sal []byte, clave string, vueltas int) []byte {
	u := utf16.Encode([]rune(clave))
	ini := make([]byte, 0, len(sal)+2*len(u))
	ini = append(ini, sal...)
	for _, w := range u {
		ini = append(ini, byte(w&0xff), byte(w>>8))
	}
	h := sha512.Sum512(ini)
	buf := make([]byte, sha512.Size+4)
	for i := 0; i < vueltas; i++ {
		copy(buf, h[:])
		binary.LittleEndian.PutUint32(buf[sha512.Size:], uint32(i)) //nolint:gosec // i < claveMaxVueltas
		h = sha512.Sum512(buf)
	}
	return h[:]
}

// --- Estado y reglas ---

// EstadoProteccion es lo que el almacén guarda por documento de hoja de cálculo.
type EstadoProteccion struct {
	// Modelo es el último doc:sheet EFECTIVO si hay alguna protección ("" si no la hay,
	// para no duplicar el documento entero). Sin contraseñas: nunca contiene ninguna.
	Modelo string
	// Metadatos LWW del registro doc:sheet efectivo: sin ellos no se sabe si una op
	// entrante gana al registro o se descarta, y la comparación sería contra un modelo
	// que el documento no tiene.
	HayRegistro bool
	Lamport     uint64
	Dot         Dot
	// Claves vigentes por ámbito.
	Claves map[string]ClaveHash
	// Secuencia: copia de la secuencia RGA de un documento de texto CON secciones
	// (proteccion_secuencia.go); nil si no la tiene o aún no se ha establecido.
	Secuencia *State
}

// ResultadoProteccion es lo que el almacén persiste tras aceptar un lote.
type ResultadoProteccion struct {
	Ops    []Op // con los valores saneados (sin contraseñas en claro)
	Estado EstadoProteccion
	Poner  map[string]ClaveHash
	Quitar []string
	// SecuenciaCambia: Estado.Secuencia (también nil) debe guardarse.
	SecuenciaCambia bool
}

// TocaModelo indica si el lote puede cambiar el modelo de la hoja.
func TocaModelo(ops []Op) bool {
	for i := range ops {
		if ops[i].Kind == OpPatch && ops[i].Key == claveHoja {
			return true
		}
		if ops[i].Kind == OpSet && (ops[i].Key == claveHoja || strings.HasPrefix(ops[i].Key, "cell:")) {
			return true
		}
	}
	return false
}

// AplicarProteccion valida un lote de operaciones contra el estado de protección y
// devuelve las operaciones saneadas y el estado resultante, o ErrProtegido /
// ErrClaveProteccion sin efecto alguno. Es pura: no hace entrada/salida.
func AplicarProteccion(est EstadoProteccion, ops []Op, esDueno bool, p Proteccion) (ResultadoProteccion, error) {
	c, original := nuevoEvaluador(est, esDueno, p)
	res := ResultadoProteccion{Ops: append([]Op(nil), ops...), Estado: est}
	for i := range res.Ops {
		op := &res.Ops[i]
		if op.Kind != OpSet {
			continue
		}
		switch {
		case op.Key == claveHoja:
			if err := c.aplicarModelo(&res.Estado, op); err != nil {
				return ResultadoProteccion{}, err
			}
		case strings.HasPrefix(op.Key, "cell:"):
			// Celdas sueltas heredadas: el editor las ignora con doc:sheet presente, pero
			// no se dejan escribir en un documento protegido.
			if res.Estado.Modelo != "" && !esDueno {
				return ResultadoProteccion{}, ErrProtegido
			}
		}
	}
	c.cierra(&res, original)
	return res, nil
}

type evaluador struct {
	esDueno bool
	p       Proteccion
	claves  map[string]ClaveHash
	memo    map[string]bool
	// fallidas: los ámbitos en los que este lote ha presentado una contraseña que no es
	// la suya, por cualquiera de las tres vías: el sobre, dentro del modelo (clientes
	// anteriores) o como contraseña nueva. Las tres dicen si se ha acertado, así que
	// las tres son un intento y las tres cuentan (ver denegar).
	fallidas map[string]bool
}

// nuevoEvaluador parte de las claves vigentes; devuelve también una copia para saber, al
// cerrar, cuáles cambiaron.
func nuevoEvaluador(est EstadoProteccion, esDueno bool, p Proteccion) (*evaluador, map[string]ClaveHash) {
	c := &evaluador{esDueno: esDueno, p: p, claves: map[string]ClaveHash{}, memo: map[string]bool{}, fallidas: map[string]bool{}}
	original := map[string]ClaveHash{}
	for k, v := range est.Claves {
		c.claves[k], original[k] = v, v
	}
	return c, original
}

// cierra deja en el resultado las claves finales y lo que hay que escribir o borrar.
func (c *evaluador) cierra(res *ResultadoProteccion, original map[string]ClaveHash) {
	res.Estado.Claves = c.claves
	res.Poner = map[string]ClaveHash{}
	for k, v := range c.claves {
		if o, ok := original[k]; !ok || !bytes.Equal(o.Hash, v.Hash) {
			res.Poner[k] = v
		}
	}
	for k := range original {
		if _, ok := c.claves[k]; !ok {
			res.Quitar = append(res.Quitar, k)
		}
	}
}

// coincide memoriza la verificación (cada una cuesta ~40 ms).
func (c *evaluador) coincide(ambito, candidata string) bool {
	h, ok := c.claves[ambito]
	if !ok || candidata == "" {
		return false
	}
	k := ambito + "\x00" + candidata
	v, hecho := c.memo[k]
	if !hecho {
		// Agotados los intentos no se comprueba ninguna: ni se calcula.
		v = !c.p.SinIntentos && h.Coincide(candidata)
		c.memo[k] = v
	}
	if !v {
		c.fallidas[ambito] = true
	}
	return v
}

// autorizado: propietario, o la contraseña presentada (o la que trae el propio modelo,
// de un cliente anterior que aún la mandaba dentro) es la del ámbito.
func (c *evaluador) autorizado(ambito, incluida string) bool {
	return c.esDueno || c.coincide(ambito, c.p.Clave) || c.coincide(ambito, incluida)
}

// denegar dice por qué no: sin más, o porque la contraseña presentada no era. Lo segundo
// es lo que el servicio cuenta como intento, y por eso mira `fallidas` y no solo la del
// sobre: con la contraseña dentro del modelo, o puesta como nueva, cada escritura
// contestaba «protegido» sin contar y se podía tantear sin límite.
func (c *evaluador) denegar(ambito string) error {
	if c.fallidas[ambito] {
		return ErrClaveProteccion
	}
	return ErrProtegido
}

// aplicarModelo procesa un set de doc:sheet.
func (c *evaluador) aplicarModelo(est *EstadoProteccion, op *Op) error {
	gana := ganaRegistro(*est, *op)
	nuevo, ok := leerLibro(op.Value)
	if !gana {
		// El registro lo descarta: no cambia nada, pero que no quede una contraseña en
		// claro en el registro de operaciones.
		if ok && nuevo != nil {
			if nuevo.quitarClaves() {
				op.Value = nuevo.valor()
			}
		}
		return nil
	}
	prev, _ := leerLibro(jsonString(est.Modelo))
	if !ok { // no es un modelo legible: equivale a no tener ninguno
		nuevo = nil
	}
	if err := c.transicion(prev, nuevo); err != nil {
		return err
	}
	if nuevo != nil {
		if nuevo.normalizar(c.claves) {
			op.Value = nuevo.valor()
		}
	}
	est.HayRegistro, est.Lamport, est.Dot = true, op.Lamport, op.Dot
	est.Modelo = ""
	if nuevo != nil && len(nuevo.ambitos()) > 0 {
		est.Modelo = nuevo.cadena()
	}
	return nil
}

// ganaRegistro: ¿la operación gana al registro LWW vigente (si no, el CRDT la descarta)?
// Es la misma regla que State.Apply.
func ganaRegistro(est EstadoProteccion, op Op) bool {
	return !est.HayRegistro || (est.Lamport <= op.Lamport && (est.Lamport < op.Lamport || est.Dot.Less(op.Dot)))
}

// transicion aplica las reglas entre el modelo vigente y el entrante.
//
//nolint:gocognit,gocyclo // una pasada por ámbito protegido; trocearla esconde el orden de las reglas.
func (c *evaluador) transicion(prev, nuevo *libro) error {
	prevA, nuevoA := prev.ambitos(), nuevo.ambitos()
	for ambito, pp := range prevA {
		np, sigue := nuevoA[ambito]
		incl := claveEnClaro(np)
		aut := c.autorizado(ambito, incl)
		if !sigue {
			if !aut {
				return c.denegar(ambito)
			}
			delete(c.claves, ambito)
			continue
		}
		nueva, err := c.claveNueva(ambito, incl)
		if err != nil {
			return err
		}
		if nueva.hay() && c.igualVigente(ambito, nueva) {
			nueva = claveNueva{} // la misma contraseña: sin cambio
		}
		_, hay := c.claves[ambito]
		quita := hay && dice(np, "hasPassword", false)
		if (!sinClave(pp, np) || nueva.hay() || quita) && !aut {
			return c.denegar(ambito)
		}
		switch {
		case nueva.hay():
			h, err := nueva.derivar()
			if err != nil {
				return err
			}
			c.claves[ambito] = h
		case quita:
			delete(c.claves, ambito)
		}
		if !aut {
			if contenidoProtegido(ambito, prev, nuevo, pp, c.p.Quien) != nil {
				return c.denegar(ambito) // con contraseña presentada y mala, lo dice
			}
		}
	}
	for ambito, np := range nuevoA {
		if _, antes := prevA[ambito]; antes {
			continue
		}
		// Proteger es de cualquier editor; una contraseña, la que ponga.
		delete(c.claves, ambito)
		nueva, err := c.claveNueva(ambito, claveEnClaro(np))
		if err != nil {
			return err
		}
		if nueva.hay() {
			h, err := nueva.derivar()
			if err != nil {
				return err
			}
			c.claves[ambito] = h
		}
	}
	return nil
}

// nueva: la contraseña nueva que trae el lote para el ámbito (el sobre gana a la que
// pudiera venir dentro del modelo).
func (c *evaluador) nueva(ambito, incluida string) string {
	if v := c.p.Nuevas[ambito]; v != "" {
		return v
	}
	return incluida
}

// claveNueva es la contraseña nueva de un ámbito: en claro (la teclea quien la pone) o ya
// derivada (la de un fichero importado).
type claveNueva struct {
	texto string
	hash  *ClaveHash
}

func (n claveNueva) hay() bool { return n.texto != "" || n.hash != nil }

// derivar es su hash: el que trae, o el de la contraseña en claro con sal nueva.
func (n claveNueva) derivar() (ClaveHash, error) {
	if n.hash != nil {
		return *n.hash, nil
	}
	return HashClave(n.texto)
}

// claveNueva lee la contraseña nueva del lote para el ámbito: la que teclea quien escribe
// (o la que trae el modelo de un cliente anterior) y, si no hay, el hash de un fichero.
func (c *evaluador) claveNueva(ambito, incluida string) (claveNueva, error) {
	if t := c.nueva(ambito, incluida); t != "" {
		return claveNueva{texto: t}, nil
	}
	if w, ok := c.p.Hashes[ambito]; ok {
		h, err := w.aHash()
		if err != nil {
			return claveNueva{}, err
		}
		return claveNueva{hash: &h}, nil
	}
	return claveNueva{}, nil
}

// igualVigente: ¿es la contraseña que el ámbito ya tiene?
func (c *evaluador) igualVigente(ambito string, n claveNueva) bool {
	if n.hash != nil {
		cur, ok := c.claves[ambito]
		return ok && bytes.Equal(cur.Hash, n.hash.Hash) && bytes.Equal(cur.Sal, n.hash.Sal)
	}
	return c.coincide(ambito, n.texto)
}

// contenidoProtegido: lo que NO puede cambiar quien no está autorizado en un ámbito
// que sigue protegido y con la protección intacta.
func contenidoProtegido(ambito string, prev, nuevo *libro, pp map[string]json.RawMessage, quien Subject) error {
	if ambito == AmbitoLibro {
		// Estructura: ni añadir, ni borrar, ni renombrar, ni reordenar hojas.
		if !reflect.DeepEqual(prev.nombres, nuevo.nombres) {
			return ErrProtegido
		}
		return nil
	}
	nombre := strings.TrimPrefix(ambito, "hoja:")
	p, n := prev.hoja(nombre), nuevo.hoja(nombre)
	// Un nombre repetido permitiría poner delante una hoja gemela y que la protegida
	// dejara de ser «la primera de ese nombre»: se exige un solo candidato.
	if p == nil || n == nil || repeticiones(nuevo.nombres, nombre) != 1 {
		return ErrProtegido
	}
	return contenidoHoja(p, n, pp, quien)
}

// permisosEstructurales mueven celdas de sitio: con alguno concedido el contenido
// bloqueado puede cambiar de posición.
var permisosEstructurales = []string{"insertRows", "deleteRows", "insertCols", "deleteCols", "sort"}

// camposPorPermiso: lo que gobierna cada permiso del cuadro «Proteger hoja».
var camposPorPermiso = map[string][]string{
	"autoFilter": {"autofilter"},
	"objects":    {"condFormats", "validations", "charts", "sparklines", "slicers"},
	"scenarios":  {"scenarios"},
}

// camposSiempreVetados no los cambia nadie con la hoja protegida (ver contenidoHoja).
var camposSiempreVetados = []string{"pivots", "externalTables"}

type estilo = map[string]any

//nolint:gocognit,gocyclo // las reglas de una hoja protegida, en el orden en que se explican.
func contenidoHoja(p, n, pp map[string]json.RawMessage, quien Subject) error {
	formato := dice(pp, "formatCells", true)
	relajado := false
	for _, k := range permisosEstructurales {
		relajado = relajado || dice(pp, k, true)
	}
	pc, ok1 := celdas(p)
	nc, ok2 := celdas(n)
	ps, ok3 := estilos(p)
	ns, ok4 := estilos(n)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return ErrProtegido // modelo ilegible: no se puede comprobar, no se acepta
	}
	rangos := rangosDe(pp, quien)
	// Un rango editable de esta persona (o de uno de sus grupos) desbloquea sus celdas
	// solo para ella: es lo que cumple el servidor y el editor enseña (proteccion_rangos.go).
	bloqueada := func(ref string) bool { return ps[ref]["locked"] != false && !rangos.contiene(ref) }

	// Bloqueo y ocultación: no se cambian sin quitar la protección.
	desbloq := func(m map[string]estilo) int { return contar(m, "locked", false) }
	ocultas := func(m map[string]estilo) int { return contar(m, "hidden", true) }
	if relajado {
		// Los estilos viajan con las filas: solo se exige que no haya MÁS celdas
		// desbloqueadas ni ocultas que antes.
		if desbloq(ns) > desbloq(ps) || ocultas(ns) > ocultas(ps) {
			return ErrProtegido
		}
	} else {
		for ref := range union(ps, ns) {
			if (ps[ref]["locked"] == false) != (ns[ref]["locked"] == false) ||
				(ps[ref]["hidden"] == true) != (ns[ref]["hidden"] == true) {
				return ErrProtegido
			}
		}
	}

	// Contenido.
	if relajado {
		// Con insertar/borrar/ordenar concedido el contenido bloqueado puede MOVERSE o
		// desaparecer con su fila, pero no aparecer contenido nuevo salvo donde la celda
		// ya era editable.
		// ponytail: techo conocido —quien tiene concedido borrar filas puede vaciar una
		// celda bloqueada sin borrar su fila—; cerrarlo exigiría reproducir cada
		// operación de fila en el servidor.
		disponible := map[string]int{}
		for ref, v := range pc {
			if v != "" && nc[ref] != v {
				disponible[v]++
			}
		}
		for ref, v := range nc {
			if v == "" || pc[ref] == v {
				continue
			}
			if disponible[v] > 0 {
				disponible[v]--
				continue
			}
			if bloqueada(ref) {
				return ErrProtegido
			}
		}
	} else {
		for ref := range union(pc, nc) {
			if pc[ref] != nc[ref] && bloqueada(ref) {
				return ErrProtegido
			}
		}
	}

	// Formato.
	if !formato && !relajado {
		for ref := range union(ps, ns) {
			if !reflect.DeepEqual(sinProteccion(ps[ref]), sinProteccion(ns[ref])) {
				return ErrProtegido
			}
		}
		if distinto(p, n, "formats") {
			return ErrProtegido
		}
	}

	// Tablas, tablas dinámicas y datos externos: Excel deshabilita crearlas o cambiarlas con
	// la hoja protegida, y no hay un permiso que lo conceda (el de «Usar tablas dinámicas»
	// no existe aquí: la web vetaba ya `pivot` en bloque). Los metadatos no tocan celdas,
	// así que sin esta regla se podían reescribir a escondidas con una escritura directa a
	// la API. Las tablas admiten un único cambio: que se desplacen con filas o columnas
	// insertadas o borradas, y por eso con un permiso estructural concedido pasan.
	// ponytail: con ese permiso también pasaría una tabla inventada; cerrarlo exigiría
	// reproducir el desplazamiento en el servidor.
	for _, f := range camposSiempreVetados {
		if distinto(p, n, f) {
			return ErrProtegido
		}
	}
	if !relajado && distinto(p, n, "tables") {
		return ErrProtegido
	}

	// Campos con permiso propio.
	for perm, campos := range camposPorPermiso {
		if dice(pp, perm, true) {
			continue
		}
		for _, f := range campos {
			if distinto(p, n, f) {
				return ErrProtegido
			}
		}
	}
	return nil
}

// --- Lectura del modelo ---

// libro es un SheetModel abierto lo justo para razonar sobre la protección. Todo lo
// demás se conserva tal cual (json.RawMessage), de modo que reescribir un valor solo
// cambia lo que se pretende.
type libro struct {
	raw     map[string]json.RawMessage
	hojas   []map[string]json.RawMessage
	nombres []string
}

// jsonString envuelve el modelo como el cliente lo guarda (un string JSON); "" = null.
func jsonString(s string) json.RawMessage {
	if s == "" {
		return nil
	}
	b, _ := json.Marshal(s)
	return b
}

// leerLibro abre el valor del registro. (nil, true) = sin modelo; (nil, false) = ilegible.
func leerLibro(valor json.RawMessage) (*libro, bool) {
	if isNullJSON(valor) {
		return nil, true
	}
	var s string
	if json.Unmarshal(valor, &s) != nil {
		return nil, false
	}
	if strings.TrimSpace(s) == "" {
		return nil, true
	}
	l := &libro{}
	if json.Unmarshal([]byte(s), &l.raw) != nil || l.raw == nil {
		return nil, false
	}
	var hojas []json.RawMessage
	if r, ok := l.raw["sheets"]; ok && !isNullJSON(r) && json.Unmarshal(r, &hojas) != nil {
		return nil, false
	}
	for _, h := range hojas {
		var m map[string]json.RawMessage
		if json.Unmarshal(h, &m) != nil || m == nil {
			return nil, false
		}
		var nombre string
		_ = json.Unmarshal(m["name"], &nombre)
		l.hojas = append(l.hojas, m)
		l.nombres = append(l.nombres, nombre)
	}
	return l, true
}

// protegido: el editor toma por protegido cualquier objeto o lista no nulo; el
// servidor, igual (más estricto nunca rompe lo que el editor produce).
func protegido(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && (t[0] == '{' || t[0] == '[')
}

// ambitos devuelve el objeto `protect` de cada ámbito protegido.
func (l *libro) ambitos() map[string]map[string]json.RawMessage {
	out := map[string]map[string]json.RawMessage{}
	if l == nil {
		return out
	}
	toma := func(ambito string, raw json.RawMessage) {
		if !protegido(raw) {
			return
		}
		m := map[string]json.RawMessage{}
		_ = json.Unmarshal(raw, &m) // una lista queda como objeto vacío
		out[ambito] = m
	}
	toma(AmbitoLibro, l.raw["protect"])
	for i, h := range l.hojas {
		a := AmbitoHoja(l.nombres[i])
		if _, dup := out[a]; !dup {
			toma(a, h["protect"])
		}
	}
	return out
}

func repeticiones(ss []string, v string) int {
	n := 0
	for _, s := range ss {
		if s == v {
			n++
		}
	}
	return n
}

// contar: cuántos estilos tienen la propiedad `k` con el valor `v`.
func contar(m map[string]estilo, k string, v bool) int {
	n := 0
	for _, s := range m {
		if s[k] == v {
			n++
		}
	}
	return n
}

func (l *libro) hoja(nombre string) map[string]json.RawMessage {
	for i, n := range l.nombres {
		if n == nombre {
			return l.hojas[i]
		}
	}
	return nil
}

// claveEnClaro es la contraseña que trae el propio modelo (clientes anteriores).
func claveEnClaro(protect map[string]json.RawMessage) string {
	var s string
	_ = json.Unmarshal(protect["password"], &s)
	return s
}

// dice: el booleano `k` de un objeto de opciones vale `v`.
func dice(m map[string]json.RawMessage, k string, v bool) bool {
	raw, ok := m[k]
	return ok && string(bytes.TrimSpace(raw)) == fmt.Sprint(v)
}

// sinClave compara dos objetos `protect` sin las claves que el servidor gobierna.
func sinClave(a, b map[string]json.RawMessage) bool {
	return reflect.DeepEqual(decodificar(a, "password", "hasPassword"), decodificar(b, "password", "hasPassword"))
}

func decodificar(m map[string]json.RawMessage, quitar ...string) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		if slices.Contains(quitar, k) {
			continue
		}
		var x any
		_ = json.Unmarshal(v, &x)
		out[k] = x
	}
	return out
}

func celdas(h map[string]json.RawMessage) (map[string]string, bool) {
	m := map[string]string{}
	if raw, ok := h["cells"]; ok && !isNullJSON(raw) {
		if json.Unmarshal(raw, &m) != nil {
			return nil, false
		}
	}
	return m, true
}

func estilos(h map[string]json.RawMessage) (map[string]estilo, bool) {
	m := map[string]estilo{}
	if raw, ok := h["styles"]; ok && !isNullJSON(raw) {
		if json.Unmarshal(raw, &m) != nil {
			return nil, false
		}
	}
	return m, true
}

func sinProteccion(s estilo) estilo {
	out := estilo{}
	for k, v := range s {
		if k != "locked" && k != "hidden" {
			out[k] = v
		}
	}
	return out
}

func union[V any](a, b map[string]V) map[string]struct{} {
	out := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		out[k] = struct{}{}
	}
	for k := range b {
		out[k] = struct{}{}
	}
	return out
}

// distinto: el campo cambió. Ausente, null, lista vacía y objeto vacío son lo mismo.
func distinto(a, b map[string]json.RawMessage, campo string) bool {
	x, y := decodificar(map[string]json.RawMessage{"v": a[campo]}), decodificar(map[string]json.RawMessage{"v": b[campo]})
	return !reflect.DeepEqual(vacioANil(x["v"]), vacioANil(y["v"]))
}

func vacioANil(v any) any {
	switch t := v.(type) {
	case []any:
		if len(t) == 0 {
			return nil
		}
	case map[string]any:
		if len(t) == 0 {
			return nil
		}
	}
	return v
}

// --- Reescritura saneada ---

// quitarClaves borra toda contraseña en claro de los `protect`; true si cambió algo.
func (l *libro) quitarClaves() bool {
	cambio := false
	quita := func(m map[string]json.RawMessage) {
		raw, ok := m["protect"]
		if !ok || !protegido(raw) {
			return
		}
		pm := map[string]json.RawMessage{}
		_ = json.Unmarshal(raw, &pm)
		if _, hay := pm["password"]; hay || bytes.TrimSpace(raw)[0] == '[' {
			delete(pm, "password")
			m["protect"], _ = json.Marshal(pm)
			cambio = true
		}
	}
	quita(l.raw)
	for _, h := range l.hojas {
		quita(h)
	}
	return cambio
}

// normalizar deja cada `protect` en su forma de servidor: sin contraseña en claro y
// con `hasPassword: true` solo si hay hash vigente. true si cambió algo.
func (l *libro) normalizar(claves map[string]ClaveHash) bool {
	cambio := false
	fija := func(ambito string, m map[string]json.RawMessage) {
		raw, ok := m["protect"]
		if !ok || !protegido(raw) {
			return
		}
		pm := map[string]json.RawMessage{}
		_ = json.Unmarshal(raw, &pm)
		_, tenia := pm["password"]
		antes := pm["hasPassword"]
		delete(pm, "password")
		delete(pm, "hasPassword")
		_, tiene := claves[ambito]
		if tiene {
			pm["hasPassword"] = json.RawMessage("true")
		}
		if tenia || bytes.TrimSpace(raw)[0] == '[' || (string(bytes.TrimSpace(antes)) == "true") != tiene {
			m["protect"], _ = json.Marshal(pm)
			cambio = true
		}
	}
	fija(AmbitoLibro, l.raw)
	vistos := map[string]bool{}
	for i, h := range l.hojas {
		a := AmbitoHoja(l.nombres[i])
		if vistos[a] { // nombres repetidos: solo el primero tiene ámbito
			continue
		}
		vistos[a] = true
		fija(a, h)
	}
	return cambio
}

func (l *libro) cadena() string {
	hojas := make([]json.RawMessage, len(l.hojas))
	for i, h := range l.hojas {
		hojas[i], _ = json.Marshal(h)
	}
	if _, ok := l.raw["sheets"]; ok || len(hojas) > 0 {
		l.raw["sheets"], _ = json.Marshal(hojas)
	}
	b, _ := json.Marshal(l.raw)
	return string(b)
}

func (l *libro) valor() json.RawMessage { return jsonString(l.cadena()) }

// --- Lo que sale hacia fuera ---

// RedactarValor quita las contraseñas en claro de un valor doc:sheet que se sirve.
// Los documentos anteriores a la protección en servidor las llevan dentro del
// modelo; hasta que se reescriben (o se barre el historial) no deben salir.
func RedactarValor(valor json.RawMessage) json.RawMessage {
	if !bytes.Contains(valor, []byte("password")) {
		return valor
	}
	l, ok := leerLibro(valor)
	if !ok || l == nil || !l.quitarClaves() {
		return valor
	}
	return l.valor()
}

// RedactarEstado aplica RedactarValor al registro de la hoja de un estado cargado.
func RedactarEstado(st *State) {
	if st == nil {
		return
	}
	if r, ok := st.Registers[claveHoja]; ok {
		r.Value = RedactarValor(r.Value)
		st.Registers[claveHoja] = r
	}
}

// RedactarOps aplica RedactarValor a las operaciones que se sirven.
func RedactarOps(ops []SequencedOp) []SequencedOp {
	for i := range ops {
		if ops[i].Op.Kind == OpSet && ops[i].Op.Key == claveHoja {
			ops[i].Op.Value = RedactarValor(ops[i].Op.Value)
		}
	}
	return ops
}

// EstadoInicialHoja deduce el estado de protección de una hoja de cálculo ya existente
// a partir de su estado CRDT. Es la migración de los modelos con la contraseña en
// claro: cada una se convierte en hash y el modelo vigente queda sin ella, sin
// perder la protección.
func EstadoInicialHoja(st *State) (EstadoProteccion, error) {
	est := EstadoProteccion{Claves: map[string]ClaveHash{}}
	r, ok := st.Registers[claveHoja]
	if !ok {
		return est, nil
	}
	est.HayRegistro, est.Lamport, est.Dot = true, r.Lamport, r.Dot
	l, leido := leerLibro(r.Value)
	if !leido || l == nil {
		return est, nil
	}
	for ambito, pm := range l.ambitos() {
		if c := claveEnClaro(pm); c != "" {
			h, err := HashClave(c)
			if err != nil {
				return est, err
			}
			est.Claves[ambito] = h
		}
	}
	l.normalizar(est.Claves)
	if len(l.ambitos()) > 0 {
		est.Modelo = l.cadena()
	}
	return est, nil
}

// --- Límite de intentos ---

const (
	intentosMax   = 8
	intentosVenta = 10 * time.Minute
)

// limitadorClaves frena el tanteo de contraseñas: cada comprobación cuesta ~40 ms de
// CPU, pero un editor puede lanzar muchas en paralelo. Tras intentosMax fallos en la
// ventana, la persona no puede volver a presentar una contraseña en ese documento
// hasta que caduquen.
// ponytail: en proceso y por réplica (como el limitador de execute); un tanteo repartido
// entre réplicas obtiene intentosMax por réplica. Pasar a pg.RateLimiter si hace falta.
type limitadorClaves struct {
	mu     sync.Mutex
	fallos map[string][]time.Time
}

func (l *limitadorClaves) recientes(clave string, ahora time.Time) []time.Time {
	var out []time.Time
	for _, t := range l.fallos[clave] {
		if ahora.Sub(t) < intentosVenta {
			out = append(out, t)
		}
	}
	return out
}

func (l *limitadorClaves) bloqueada(clave string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recientes(clave, time.Now())) >= intentosMax
}

func (l *limitadorClaves) fallo(clave string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ahora := time.Now()
	if l.fallos == nil {
		l.fallos = map[string][]time.Time{}
	}
	if len(l.fallos) > 4096 { // poda: no crece sin tope con documentos que no se vuelven a tocar
		for k := range l.fallos {
			if len(l.recientes(k, ahora)) == 0 {
				delete(l.fallos, k)
			}
		}
	}
	l.fallos[clave] = append(l.recientes(clave, ahora), ahora)
}

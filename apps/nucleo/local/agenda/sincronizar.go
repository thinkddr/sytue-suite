// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
)

// VentanaCalendario es cuánto hacia atrás entra en la primera bajada de un calendario.
const VentanaCalendario = 365 * 24 * time.Hour

// Espera de los reintentos: empieza en EsperaMinima y se dobla hasta EsperaMaxima.
const (
	EsperaMinima = 30 * time.Second
	EsperaMaxima = time.Hour
)

// Conflicto es un objeto que cambió aquí y en el servidor a la vez: el del servidor
// quedó en su sitio y lo de aquí, en una copia.
type Conflicto struct {
	Coleccion string // el id de la colección de aquí
	Original  string // el id del objeto de aquí que tomó la versión del servidor
	Copia     string // el id de la copia con lo escrito aquí
}

// Resultado es lo que hizo una sincronización.
type Resultado struct {
	Bajados, Subidos, Borrados int
	Conflictos                 []Conflicto
	// Pendientes son los cambios de aquí que no se pudieron subir (se reintentan).
	Pendientes int
	// Errores de las colecciones que no se pudieron sincronizar (la cuenta sigue).
	Errores []error
	// Invitaciones son los avisos de las invitaciones por correo que no salieron
	// (SinRemitente, FalloEnvio): el evento sí se subió.
	Invitaciones []error
	// Devueltos son los subidos que el proveedor devolvió con algo suyo (el enlace de la
	// reunión que se le pidió) y ya están así aquí.
	Devueltos int
}

// Sincronizador reconcilia una cuenta y un servicio.
type Sincronizador struct {
	Remoto Remoto
	Local  Local
	// Estado es el JSON del estado (`<Datos>/<app>/agenda/<cuenta>.json`).
	Estado string
	// Ventana de la primera bajada (calendarios); 0, todo.
	Ventana time.Duration
	// MarcaCopia es lo que se pone en el nombre de una copia en conflicto.
	MarcaCopia string
	// Invitar, si no es nil, manda por correo las invitaciones de lo que se sube a una
	// colección cuyo servidor no las manda él (A-11; [Cartero.Invitar]): tras crear o
	// cambiar un evento con invitados si cambió lo que ellos ven, y tras borrarlo. Decide
	// él a quién y qué (la persona organiza o contesta). Lo que devuelve va a
	// Resultado.Invitaciones: el evento ya está subido.
	Invitar func(ctx context.Context, e Envio) error
	// Ahora es el reloj (las pruebas lo cambian).
	Ahora func() time.Time

	mu sync.Mutex
	// direccion es la de Direccion, leída o aprendida (con su propio cerrojo).
	mdir      sync.Mutex
	direccion string
}

type estado struct {
	Colecciones map[string]*estadoColeccion `json:"colecciones"`
	// Direccion es la de la persona en la cuenta (la del ORGANIZER de lo que crea aquí),
	// según el proveedor ([Direccionado]); vacía si no la dio.
	Direccion string `json:"direccion,omitempty"`
}

type estadoColeccion struct {
	Local       string                   `json:"local"`
	Nombre      string                   `json:"nombre"`
	Color       string                   `json:"color,omitempty"`
	SoloLectura bool                     `json:"solo_lectura,omitempty"`
	Planifica   bool                     `json:"planifica,omitempty"`
	Ficha       string                   `json:"ficha,omitempty"`
	Ignorada    bool                     `json:"ignorada,omitempty"` // la persona borró su copia de aquí
	Objetos     map[string]*estadoObjeto `json:"objetos"`
	// Reintentos de lo que no subió, por id de aquí (o «borrar:<id remoto>»).
	Reintentos map[string]*reintento `json:"reintentos,omitempty"`
	// Copias son las copias en conflicto de aquí que aún no se subieron: al subirlas no
	// se manda ninguna invitación (son un duplicado que la persona tiene que resolver).
	Copias map[string]bool `json:"copias,omitempty"`
}

type estadoObjeto struct {
	Local  string `json:"local,omitempty"`
	ETag   string `json:"etag,omitempty"`
	Huella string `json:"huella,omitempty"`
	Fuera  bool   `json:"fuera,omitempty"`
	// Invitacion es la [firma] de la última versión que se vio de un evento con
	// invitados (solo si se mandan las invitaciones por correo).
	Invitacion string `json:"invitacion,omitempty"`
	// Respuesta es la [firma] que dejó aquí la respuesta de un invitado que llegó por
	// correo ([Registro.AplicarRespuestas]), aún sin subir: si al subirlo lo que ven los
	// invitados es eso, el cambio vino de las respuestas y no se manda nada a nadie.
	Respuesta string `json:"respuesta,omitempty"`
}

type reintento struct {
	Intentos int       `json:"intentos"`
	Proximo  time.Time `json:"proximo"`
	Error    string    `json:"error,omitempty"`
}

func huella(datos string) string {
	h := sha256.Sum256([]byte(datos))
	return hex.EncodeToString(h[:])
}

func (s *Sincronizador) ahora() time.Time {
	if s.Ahora != nil {
		return s.Ahora()
	}
	return time.Now()
}

func (s *Sincronizador) leer() *estado {
	st := &estado{}
	if s.Estado != "" {
		_ = espacio.LeerJSON(s.Estado, st)
	}
	if st.Colecciones == nil {
		st.Colecciones = map[string]*estadoColeccion{}
	}
	for _, c := range st.Colecciones {
		if c.Objetos == nil {
			c.Objetos = map[string]*estadoObjeto{}
		}
		if c.Reintentos == nil {
			c.Reintentos = map[string]*reintento{}
		}
		if c.Copias == nil {
			c.Copias = map[string]bool{}
		}
	}
	return st
}

func (s *Sincronizador) guardar(st *estado) error {
	if s.Estado == "" {
		return nil
	}
	return espacio.EscribirJSON(s.Estado, st)
}

// Coleccion de aquí y su origen, para agrupar la lista por cuenta (A-12, K-06).
type ColeccionLocal struct {
	ID          string // la de este equipo
	Nombre      string
	SoloLectura bool
	// Planifica: el servidor manda él las invitaciones (si no, salen por correo).
	Planifica bool
}

// Direccion es la dirección de la persona en la cuenta que dio el proveedor en la última
// sincronización (sin leer la red), o "". No espera a una sincronización en marcha: el
// cartero la pregunta desde dentro de una.
func (s *Sincronizador) Direccion() string {
	s.mdir.Lock()
	defer s.mdir.Unlock()
	if s.direccion == "" && s.Estado != "" {
		var st estado
		_ = espacio.LeerJSON(s.Estado, &st)
		s.direccion = st.Direccion
	}
	return s.direccion
}

// Colecciones son las de aquí que respalda esta cuenta (sin leer la red).
func (s *Sincronizador) Colecciones() []ColeccionLocal {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.leer()
	var out []ColeccionLocal
	for _, c := range st.Colecciones {
		if c.Local != "" && !c.Ignorada {
			out = append(out, ColeccionLocal{ID: c.Local, Nombre: c.Nombre, SoloLectura: c.SoloLectura, Planifica: c.Planifica})
		}
	}
	slices.SortFunc(out, func(a, b ColeccionLocal) int {
		if a.Nombre < b.Nombre {
			return -1
		}
		if a.Nombre > b.Nombre {
			return 1
		}
		return 0
	})
	return out
}

// Sincronizar reconcilia ahora todas las colecciones de la cuenta. Un error devuelto
// es de la cuenta entera (sin red, contraseña mala); los de una colección van en
// Resultado.Errores y no paran las demás.
func (s *Sincronizador) Sincronizar(ctx context.Context) (Resultado, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var res Resultado
	st := s.leer()
	cols, err := s.Remoto.Colecciones(ctx)
	if err != nil {
		return res, err
	}
	if d, ok := s.Remoto.(Direccionado); ok {
		// Un fallo aquí no para la sincronización: se queda la que se sabía.
		if dir, err := d.Direccion(ctx); err == nil && dir != "" {
			st.Direccion = strings.ToLower(strings.TrimSpace(dir))
			s.mdir.Lock()
			s.direccion = st.Direccion
			s.mdir.Unlock()
		}
	}
	vistas := map[string]bool{}
	for _, col := range cols {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		vistas[col.ID] = true
		ec, sincroniza, err := s.copiaDe(ctx, st, col, &res)
		if err != nil {
			return res, err
		}
		if !sincroniza {
			continue
		}
		ec.Planifica = col.Planifica
		if err := s.coleccion(ctx, col, ec, &res); err != nil {
			switch {
			case errors.Is(err, ErrAcceso) && len(cols) > 0 && !col.SoloLectura:
				// Una colección que no deja leer no tumba la cuenta.
				res.Errores = append(res.Errores, fmt.Errorf("%s: %w", col.Nombre, err))
			case isCancel(err):
				_ = s.guardar(st)
				return res, err
			default:
				res.Errores = append(res.Errores, fmt.Errorf("%s: %w", col.Nombre, err))
			}
		}
		// Se guarda tras cada colección: un corte a mitad no repite lo ya hecho.
		if err := s.guardar(st); err != nil {
			return res, err
		}
	}
	// Una colección que ya no está en el servidor: su copia de aquí se queda con lo que
	// tiene (no se pierde nada) y pasa a ser un calendario o libreta más del equipo.
	for id := range st.Colecciones {
		if !vistas[id] {
			delete(st.Colecciones, id)
		}
	}
	return res, s.guardar(st)
}

// copiaDe es el estado de la copia de aquí de una colección del servidor: se crea la
// primera vez y se pone al día de nombre, color y permisos. sincroniza=false si no se
// sincroniza: ignorada, borrada aquí por la persona o sin copia (el error va en res).
func (s *Sincronizador) copiaDe(ctx context.Context, st *estado, col Coleccion, res *Resultado) (ec *estadoColeccion, sincroniza bool, err error) {
	ec = st.Colecciones[col.ID]
	if ec == nil {
		id, err := s.Local.Crear(ctx, col.Nombre, col.Color, col.SoloLectura)
		if err != nil {
			res.Errores = append(res.Errores, fmt.Errorf("%s: %w", col.Nombre, err))
			return nil, false, nil
		}
		ec = &estadoColeccion{
			Local: id, Nombre: col.Nombre, Color: col.Color, SoloLectura: col.SoloLectura,
			Objetos: map[string]*estadoObjeto{}, Reintentos: map[string]*reintento{}, Copias: map[string]bool{},
		}
		st.Colecciones[col.ID] = ec
		return ec, true, nil
	}
	if ec.Ignorada {
		return nil, false, nil
	}
	existe, err := s.Local.Existe(ctx, ec.Local)
	if err != nil {
		return nil, false, err
	}
	if !existe {
		// La persona borró su copia: se deja de sincronizar esa colección (en el
		// servidor no se toca nada).
		ec.Ignorada = true
		ec.Objetos, ec.Reintentos = map[string]*estadoObjeto{}, map[string]*reintento{}
		return nil, false, nil
	}
	if ec.Nombre != col.Nombre || ec.Color != col.Color || ec.SoloLectura != col.SoloLectura {
		if err := s.Local.Actualizar(ctx, ec.Local, col.Nombre, col.Color, col.SoloLectura); err == nil {
			ec.Nombre, ec.Color, ec.SoloLectura = col.Nombre, col.Color, col.SoloLectura
		}
	}
	return ec, true, nil
}

func isCancel(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// locales lee los objetos de aquí de una colección, por id.
func (s *Sincronizador) locales(ctx context.Context, ec *estadoColeccion) (map[string]ObjetoLocal, error) {
	lista, err := s.Local.Objetos(ctx, ec.Local)
	if err != nil {
		return nil, err
	}
	m := make(map[string]ObjetoLocal, len(lista))
	for _, o := range lista {
		m[o.ID] = o
	}
	return m, nil
}

func (s *Sincronizador) coleccion(ctx context.Context, col Coleccion, ec *estadoColeccion, res *Resultado) error {
	if e, ok := s.Remoto.(Enlazable); ok {
		e.Enlazar(col, s.enlaces(ctx, ec))
	}
	if err := s.bajar(ctx, col, ec, res); err != nil {
		return err
	}
	if col.SoloLectura {
		return nil
	}
	return s.subir(ctx, col, ec, res)
}

// ----- Bajada -----

func (s *Sincronizador) bajar(ctx context.Context, col Coleccion, ec *estadoColeccion, res *Resultado) error {
	etagDe := func(id string) (string, bool) {
		if eo := ec.Objetos[id]; eo != nil {
			return eo.ETag, true
		}
		return "", false
	}
	var desde time.Time
	if ec.Ficha == "" && s.Ventana > 0 {
		desde = s.ahora().Add(-s.Ventana)
	}
	ca, err := s.Remoto.Cambios(ctx, col, ec.Ficha, etagDe, desde)
	if errors.Is(err, ErrFichaCaducada) {
		ec.Ficha = ""
		ca, err = s.Remoto.Cambios(ctx, col, "", etagDe, time.Time{})
		ca.Completo = true
	}
	if err != nil {
		return err
	}
	locales, err := s.locales(ctx, ec)
	if err != nil {
		return err
	}
	puestos := map[string]bool{}
	for _, o := range ca.Puestos {
		puestos[o.ID] = true
		if err := s.aplicar(ctx, ec, locales, o, res); err != nil {
			if isCancel(err) {
				return err
			}
			// Un objeto que no entra (ilegible, inválido) no para la colección: se
			// recuerda su ETag para no bajarlo otra vez hasta que cambie.
			ec.Objetos[o.ID] = &estadoObjeto{ETag: o.ETag, Fuera: true, Local: localDe(ec, o.ID)}
		}
	}
	for _, o := range ca.Fuera {
		if ec.Objetos[o.ID] == nil {
			ec.Objetos[o.ID] = &estadoObjeto{ETag: o.ETag, Fuera: true}
		}
	}
	borrados := slices.Clone(ca.Borrados)
	if ca.Completo {
		vistos := map[string]bool{}
		for _, v := range ca.Vistos {
			vistos[v] = true
		}
		for id := range ec.Objetos {
			if !vistos[id] && !puestos[id] {
				borrados = append(borrados, id)
			}
		}
	}
	for _, id := range borrados {
		if err := s.quitar(ctx, ec, locales, id, res); err != nil && isCancel(err) {
			return err
		}
	}
	ec.Ficha = ca.Ficha
	return nil
}

func localDe(ec *estadoColeccion, id string) string {
	if eo := ec.Objetos[id]; eo != nil {
		return eo.Local
	}
	return ""
}

// aplicar pone aquí un objeto que cambió en el servidor.
func (s *Sincronizador) aplicar(ctx context.Context, ec *estadoColeccion, locales map[string]ObjetoLocal, o Objeto, res *Resultado) error {
	datos := string(o.Datos)
	eo := ec.Objetos[o.ID]
	if eo != nil && eo.Local != "" {
		if lo, ok := locales[eo.Local]; ok {
			if u := uidDe(datos); u != "" && lo.UID != "" && u != lo.UID {
				// El proveedor no guarda nuestro UID (Google Contacts, Graph): manda el de
				// aquí, que es el que enlazan las etiquetas y los favoritos.
				if otro, err := conUID(datos, lo.UID); err == nil {
					datos = otro
				}
			}
			if huella(lo.Datos) != eo.Huella && lo.Datos != datos {
				// Cambió en los dos lados: lo de aquí, a una copia; el servidor, en su sitio.
				o.Datos = []byte(datos)
				return s.conflicto(ctx, ec, locales, eo, lo, o, res)
			}
			nuevo, err := s.Local.Poner(ctx, ec.Local, lo.ID, lo.Datos, datos)
			if err != nil {
				if errors.Is(err, ErrCambioLocal) {
					// La persona lo está editando: la próxima vez.
					return nil
				}
				// El UID cambió en el servidor (raro): el objeto se rehace.
				if berr := s.Local.Borrar(ctx, lo.ID, lo.Datos); berr != nil {
					return err
				}
				delete(locales, lo.ID)
				return s.crearLocal(ctx, ec, locales, o, res)
			}
			locales[nuevo.ID] = nuevo
			eo.ETag, eo.Huella, eo.Fuera = o.ETag, huella(nuevo.Datos), false
			s.firmar(ec, eo, datos)
			res.Bajados++
			return nil
		}
		// Se borró aquí y cambió en el servidor: gana el cambio, se vuelve a traer.
		delete(ec.Reintentos, "borrar:"+o.ID)
	}
	return s.crearLocal(ctx, ec, locales, o, res)
}

func (s *Sincronizador) crearLocal(ctx context.Context, ec *estadoColeccion, locales map[string]ObjetoLocal, o Objeto, res *Resultado) error {
	datos := string(o.Datos)
	nuevo, err := s.Local.Poner(ctx, ec.Local, "", "", datos)
	if err != nil {
		// ¿Ya hay aquí uno con su UID sin enlazar (importado a mano, o subido y aún sin
		// confirmar)? Se enlaza; si era distinto, lo de aquí se guarda en una copia.
		uid := uidDe(datos)
		var libre *ObjetoLocal
		for _, lo := range locales {
			if lo.UID == uid && uid != "" && !enlazado(ec, lo.ID) {
				libre = &lo
				break
			}
		}
		if libre == nil {
			return err
		}
		eo := &estadoObjeto{Local: libre.ID, Huella: huella(libre.Datos)}
		ec.Objetos[o.ID] = eo
		if libre.Datos == datos {
			eo.ETag = o.ETag
			s.firmar(ec, eo, datos)
			return nil
		}
		eo.Huella = "" // distinto: es un conflicto
		return s.conflicto(ctx, ec, locales, eo, *libre, o, res)
	}
	locales[nuevo.ID] = nuevo
	eo := &estadoObjeto{Local: nuevo.ID, ETag: o.ETag, Huella: huella(nuevo.Datos)}
	ec.Objetos[o.ID] = eo
	s.firmar(ec, eo, datos)
	res.Bajados++
	return nil
}

func enlazado(ec *estadoColeccion, local string) bool {
	for _, eo := range ec.Objetos {
		if eo.Local == local {
			return true
		}
	}
	return false
}

func (s *Sincronizador) conflicto(ctx context.Context, ec *estadoColeccion, locales map[string]ObjetoLocal, eo *estadoObjeto, lo ObjetoLocal, o Objeto, res *Resultado) error {
	copia, err := s.Local.Copia(lo.Datos, s.MarcaCopia)
	if err != nil {
		return err
	}
	// Primero la copia: si lo demás falla, lo escrito aquí ya está a salvo.
	nc, err := s.Local.Poner(ctx, ec.Local, "", "", copia)
	if err != nil {
		return err
	}
	locales[nc.ID] = nc
	ec.Copias[nc.ID] = true
	nuevo, err := s.Local.Poner(ctx, ec.Local, lo.ID, lo.Datos, string(o.Datos))
	if err != nil {
		if errors.Is(err, ErrCambioLocal) {
			return nil
		}
		return err
	}
	locales[nuevo.ID] = nuevo
	eo.ETag, eo.Huella, eo.Fuera = o.ETag, huella(nuevo.Datos), false
	s.firmar(ec, eo, string(o.Datos))
	res.Bajados++
	res.Conflictos = append(res.Conflictos, Conflicto{Coleccion: ec.Local, Original: nuevo.ID, Copia: nc.ID})
	return nil
}

// quitar borra aquí lo que se borró en el servidor, salvo que aquí se cambiara: eso se
// queda y se sube como nuevo.
func (s *Sincronizador) quitar(ctx context.Context, ec *estadoColeccion, locales map[string]ObjetoLocal, id string, res *Resultado) error {
	eo := ec.Objetos[id]
	delete(ec.Objetos, id)
	delete(ec.Reintentos, "borrar:"+id)
	if eo == nil || eo.Local == "" {
		return nil
	}
	lo, ok := locales[eo.Local]
	if !ok || huella(lo.Datos) != eo.Huella {
		return nil
	}
	if err := s.Local.Borrar(ctx, lo.ID, lo.Datos); err != nil {
		if errors.Is(err, ErrCambioLocal) {
			return nil
		}
		return err
	}
	delete(locales, lo.ID)
	res.Borrados++
	return nil
}

// ----- Subida -----

func (s *Sincronizador) bloqueado(ec *estadoColeccion, clave string, res *Resultado) bool {
	if r := ec.Reintentos[clave]; r != nil && s.ahora().Before(r.Proximo) {
		res.Pendientes++
		return true
	}
	return false
}

func (s *Sincronizador) fallo(ec *estadoColeccion, clave string, err error, res *Resultado) {
	r := ec.Reintentos[clave]
	if r == nil {
		r = &reintento{}
		ec.Reintentos[clave] = r
	}
	r.Intentos++
	espera := EsperaMinima << min(r.Intentos-1, 12)
	if espera > EsperaMaxima || espera <= 0 {
		espera = EsperaMaxima
	}
	r.Proximo, r.Error = s.ahora().Add(espera), err.Error()
	res.Pendientes++
}

func (s *Sincronizador) subir(ctx context.Context, col Coleccion, ec *estadoColeccion, res *Resultado) error {
	locales, err := s.locales(ctx, ec)
	if err != nil {
		return err
	}
	porLocal := map[string]string{}
	for id, eo := range ec.Objetos {
		if eo.Local != "" {
			porLocal[eo.Local] = id
		}
	}
	ids := make([]string, 0, len(locales))
	for id := range locales {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, lid := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		rid, enlazado := porLocal[lid]
		if err := s.subirUno(ctx, col, ec, locales, lid, rid, enlazado, res); err != nil {
			return err
		}
	}
	// Lo borrado aquí.
	remotos := make([]string, 0, len(ec.Objetos))
	for id := range ec.Objetos {
		remotos = append(remotos, id)
	}
	slices.Sort(remotos)
	for _, rid := range remotos {
		eo := ec.Objetos[rid]
		if eo.Local == "" || eo.Fuera {
			continue
		}
		if _, sigue := locales[eo.Local]; sigue {
			continue
		}
		if err := s.borrarAlla(ctx, col, ec, locales, rid, eo, res); err != nil {
			return err
		}
	}
	return nil
}

// subirUno sube el objeto de aquí lid si es nuevo o cambió (rid es el del servidor al
// que está enlazado, si enlazado). Solo devuelve el error que corta la vuelta (la
// cancelación): los demás quedan en el reintento del objeto y en res.
func (s *Sincronizador) subirUno(ctx context.Context, col Coleccion, ec *estadoColeccion, locales map[string]ObjetoLocal, lid, rid string, enlazado bool, res *Resultado) error {
	lo := locales[lid]
	var eo *estadoObjeto
	if enlazado {
		eo = ec.Objetos[rid]
		if huella(lo.Datos) == eo.Huella {
			delete(ec.Reintentos, lid)
			return nil
		}
	} else if a, ok := s.Remoto.(Admite); ok && !a.Admite(lo.Datos) {
		// El proveedor no sabe guardarlo: se queda en este equipo.
		return nil
	}
	if s.bloqueado(ec, lid, res) {
		return nil
	}
	previa, respuesta := "", ""
	if eo != nil {
		previa, respuesta = eo.Invitacion, eo.Respuesta
	}
	subido, nuevoAlla, err := s.mandarObjeto(ctx, col, ec, rid, eo, enlazado, lo)
	if errors.Is(err, ErrConflicto) {
		// El servidor tiene otra versión: se trae y se resuelve como en la bajada.
		if err := s.resolver(ctx, col, ec, locales, rid, lo, res); err != nil {
			if isCancel(err) {
				return err
			}
			s.fallo(ec, lid, err, res)
		}
		return nil
	}
	if err != nil {
		if isCancel(err) {
			return err
		}
		s.fallo(ec, lid, err, res)
		return nil
	}
	delete(ec.Reintentos, lid)
	if rid != "" && subido.ID != rid {
		delete(ec.Objetos, rid)
	}
	lo = s.conLoSuyo(ctx, ec, locales, lid, lo, subido, res)
	nuevoEo := &estadoObjeto{Local: lid, ETag: subido.ETag, Huella: huella(lo.Datos), Invitacion: previa}
	ec.Objetos[subido.ID] = nuevoEo
	res.Subidos++
	// Un fallo al mandar el correo no deshace la subida: el evento ya está en el
	// servidor. Va en Resultado.Invitaciones.
	s.invitar(ctx, ec, nuevoEo, lo, previa, respuesta, nuevoAlla, res)
	return nil
}

// mandarObjeto lleva lo de aquí al servidor: cambia el objeto enlazado rid o, si no lo
// hay (o se borró allí entre medias), lo crea. nuevoAlla dice si se creó.
func (s *Sincronizador) mandarObjeto(ctx context.Context, col Coleccion, ec *estadoColeccion, rid string, eo *estadoObjeto, enlazado bool, lo ObjetoLocal) (subido Objeto, nuevoAlla bool, err error) {
	if !enlazado {
		subido, err = s.Remoto.Crear(ctx, col, lo.UID, []byte(lo.Datos))
		return subido, true, err
	}
	subido, err = s.Remoto.Cambiar(ctx, col, Objeto{ID: rid, ETag: eo.ETag}, []byte(lo.Datos))
	if errors.Is(err, ErrNoEsta) {
		// Se borró en el servidor mientras aquí se cambiaba: sube como nuevo.
		delete(ec.Objetos, rid)
		subido, err = s.Remoto.Crear(ctx, col, lo.UID, []byte(lo.Datos))
		return subido, true, err
	}
	return subido, false, err
}

// conLoSuyo deja aquí lo que el proveedor le añadió a un objeto al subirlo (el enlace
// de la reunión que se le pidió, reuniones.go): lo suyo es lo que vale aquí, si aquí no
// cambió entre medias (entonces sube en la próxima vuelta y lo suyo llega con la
// bajada), con el UID de aquí si el proveedor no lo guarda (Graph), como en la bajada.
// Devuelve el objeto de aquí como queda.
func (s *Sincronizador) conLoSuyo(ctx context.Context, ec *estadoColeccion, locales map[string]ObjetoLocal, lid string, lo ObjetoLocal, subido Objeto, res *Resultado) ObjetoLocal {
	if len(subido.Datos) == 0 || string(subido.Datos) == lo.Datos {
		return lo
	}
	suyo := string(subido.Datos)
	if u := uidDe(suyo); u != "" && lo.UID != "" && u != lo.UID {
		if otro, err := conUID(suyo, lo.UID); err == nil {
			suyo = otro
		}
	}
	puesto, err := s.Local.Poner(ctx, ec.Local, lid, lo.Datos, suyo)
	if err != nil {
		return lo
	}
	locales[lid] = puesto
	res.Devueltos++
	return puesto
}

// borrarAlla borra en el servidor el objeto rid, que se borró aquí (y, si se mandaron
// invitaciones, manda la cancelación). Solo devuelve el error que corta la vuelta.
func (s *Sincronizador) borrarAlla(ctx context.Context, col Coleccion, ec *estadoColeccion, locales map[string]ObjetoLocal, rid string, eo *estadoObjeto, res *Resultado) error {
	clave := "borrar:" + rid
	if s.bloqueado(ec, clave, res) {
		return nil
	}
	// Para cancelar por correo hace falta el evento como estaba: se lee antes.
	var cancelar []byte
	if s.manda(ec) && eo.Invitacion != "" {
		if o, lerr := s.Remoto.Leer(ctx, col, rid); lerr == nil && (o.ETag == "" || o.ETag == eo.ETag) {
			cancelar = o.Datos
		}
	}
	err := s.Remoto.Borrar(ctx, col, rid, eo.ETag)
	switch {
	case err == nil, errors.Is(err, ErrNoEsta):
		delete(ec.Objetos, rid)
		delete(ec.Reintentos, clave)
		res.Borrados++
		if err == nil && len(cancelar) > 0 {
			previa := leerFirma(eo.Invitacion)
			if ierr := s.Invitar(ctx, Envio{Datos: string(cancelar), Antes: previa.invitados, Borrado: true}); ierr != nil {
				res.Invitaciones = append(res.Invitaciones, ierr)
			}
		}
	case errors.Is(err, ErrConflicto):
		// Cambió en el servidor: el cambio gana a un borrado; se vuelve a traer.
		o, lerr := s.Remoto.Leer(ctx, col, rid)
		if lerr != nil {
			s.fallo(ec, clave, lerr, res)
			return nil
		}
		delete(ec.Reintentos, clave)
		if err := s.crearLocal(ctx, ec, locales, o, res); err != nil && isCancel(err) {
			return err
		}
	case isCancel(err):
		return err
	default:
		s.fallo(ec, clave, err, res)
	}
	return nil
}

// resolver trae la versión del servidor de un objeto que chocó al subir.
func (s *Sincronizador) resolver(ctx context.Context, col Coleccion, ec *estadoColeccion, locales map[string]ObjetoLocal, rid string, lo ObjetoLocal, res *Resultado) error {
	if rid == "" {
		// Crear chocó: ya hay un objeto con su URL. Se busca por el UID.
		rid = s.idNuevo(col, lo.UID)
	}
	o, err := s.Remoto.Leer(ctx, col, rid)
	if err != nil {
		return err
	}
	eo := ec.Objetos[o.ID]
	if eo == nil {
		eo = &estadoObjeto{Local: lo.ID}
		ec.Objetos[o.ID] = eo
	}
	if string(o.Datos) == lo.Datos {
		eo.ETag, eo.Huella = o.ETag, huella(lo.Datos)
		s.firmar(ec, eo, lo.Datos)
		return nil
	}
	return s.conflicto(ctx, ec, locales, eo, lo, o, res)
}

// IDNuevo, si el remoto lo implementa, dice el id que tendría un objeto nuevo con ese
// UID (en DAV, su URL): con él se lee el que chocó al crearlo.
type IDNuevo interface {
	IDNuevo(col Coleccion, uid string) string
}

func (s *Sincronizador) idNuevo(col Coleccion, uid string) string {
	if n, ok := s.Remoto.(IDNuevo); ok {
		return n.IDNuevo(col, uid)
	}
	return uid
}

// Admite, si el remoto lo implementa, dice si sabe guardar un objeto de aquí: lo que
// no, se queda en este equipo sin subir.
type Admite interface {
	Admite(datos string) bool
}

// manda dice si esta colección manda las invitaciones por correo.
func (s *Sincronizador) manda(ec *estadoColeccion) bool {
	return s.Invitar != nil && !ec.Planifica && !ec.SoloLectura
}

// firmar recuerda lo que ven los invitados de la versión del servidor de un evento. Lo
// que dejó aquí una respuesta ya no hace falta recordarlo: lo de allí manda.
func (s *Sincronizador) firmar(ec *estadoColeccion, eo *estadoObjeto, datos string) {
	if s.manda(ec) {
		eo.Invitacion = firmaInvitacion(datos).String()
	}
	eo.Respuesta = ""
}

// invitar llama a Invitar tras subir un evento, si cambió lo que ven los invitados.
// respuesta es la [firma] que dejaron las respuestas de los invitados: si lo subido es
// eso, no se manda nada.
func (s *Sincronizador) invitar(ctx context.Context, ec *estadoColeccion, eo *estadoObjeto, lo ObjetoLocal, previa, respuesta string, nuevo bool, res *Resultado) {
	if !s.manda(ec) {
		return
	}
	copia := ec.Copias[lo.ID]
	delete(ec.Copias, lo.ID)
	ahora := firmaInvitacion(lo.Datos)
	eo.Invitacion = ahora.String()
	antes := leerFirma(previa)
	switch {
	case copia, ahora.vacia() && antes.vacia(), !nuevo && ahora.igual(antes):
		return
	case !nuevo && respuesta != "" && ahora.igual(leerFirma(respuesta)):
		return // solo lo que contestaron los invitados: nada que contarles
	case ahora.vacia() && nuevo:
		return
	}
	e := Envio{Datos: lo.Datos, Antes: antes.invitados, Nuevo: nuevo, Contenido: !ahora.mismoContenido(antes)}
	if err := s.Invitar(ctx, e); err != nil {
		res.Invitaciones = append(res.Invitaciones, err)
	}
}

// ponerDeRespuesta guarda aquí lo que deja en un evento la respuesta de un invitado que
// llegó por correo (el PARTSTAT, o la excepción nueva de su ocurrencia) y lo marca como
// venido de una respuesta (estadoObjeto.Respuesta), para que la vuelta lo suba sin
// mandar ninguna invitación. Con el cerrojo de la sincronización: una vuelta a la vez no
// puede subirlo antes de marcarlo ni pisar la marca con su estado. Solo se marca si lo
// que ven los invitados de lo que había aquí es lo que ya sabían (lo del servidor o lo
// que dejó otra respuesta): un cambio de la persona aún sin subir se manda como siempre,
// y con él la respuesta.
func (s *Sincronizador) ponerDeRespuesta(ctx context.Context, col string, o ObjetoLocal, datos string) (ObjetoLocal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	puesto, err := s.Local.Poner(ctx, col, o.ID, o.Datos, datos)
	if err != nil {
		return puesto, err
	}
	st := s.leer()
	antes := firmaInvitacion(o.Datos)
	marcado := false
	for _, ec := range st.Colecciones {
		if ec.Local != col || !s.manda(ec) {
			continue
		}
		for _, eo := range ec.Objetos {
			if eo.Local == puesto.ID && eo.Invitacion != "" && (antes.igual(leerFirma(eo.Invitacion)) || antes.igual(leerFirma(eo.Respuesta))) {
				eo.Respuesta = firmaInvitacion(datos).String()
				marcado = true
			}
		}
	}
	if marcado {
		// Si no se puede guardar, la respuesta ya está puesta; lo peor es que la vuelta
		// mande la invitación cambiada.
		_ = s.guardar(st)
	}
	return puesto, nil
}

// Direccionado, si el remoto lo implementa, sabe la dirección de correo de la persona en
// la cuenta: la del ORGANIZER de lo que se crea aquí con invitados (CalDAV, por el
// `calendar-user-address-set` del principal; Google, por su calendario principal;
// Microsoft, por /me). Se pregunta en cada sincronización y se guarda en el estado.
type Direccionado interface {
	Direccion(ctx context.Context) (string, error)
}

// Enlazable, si el remoto lo implementa, recibe antes de bajar y subir cada colección
// cómo pasar de sus ids a los objetos de aquí y al revés: los grupos de contactos de
// Google (`contactGroups`) y las categorías de Outlook nombran a sus miembros por el id
// del proveedor, y una etiqueta de aquí, por el UID de cada contacto.
type Enlazable interface {
	Enlazar(col Coleccion, e Enlaces)
}

// Enlaces son los de una colección mientras se sincroniza. Las funciones miran el estado
// en el momento en que se llaman: lo que se sube en la misma vuelta ya cuenta.
type Enlaces struct {
	// UIDDe es el UID de aquí (sin «urn:uuid:») del objeto del proveedor id, o "".
	UIDDe func(id string) string
	// IDDe es el id del proveedor del objeto de aquí con ese UID, o "".
	IDDe func(uid string) string
	// Conocidos son los ids del proveedor que ya están enlazados aquí.
	Conocidos func() []string
	// SinEnlazar son los objetos de aquí que aún no están en el proveedor.
	SinEnlazar func() []ObjetoLocal
}

// enlaces son los Enlaces de una colección. Los objetos de aquí se leen una vez y se
// vuelven a leer si falta alguno (uno creado en esta misma vuelta).
func (s *Sincronizador) enlaces(ctx context.Context, ec *estadoColeccion) Enlaces {
	var locales map[string]ObjetoLocal
	releer := func() {
		if m, err := s.locales(ctx, ec); err == nil {
			locales = m
		}
	}
	uidLocal := func(id string) string {
		if locales == nil {
			releer()
		}
		o, ok := locales[id]
		if !ok {
			releer()
			o = locales[id]
		}
		return limpiarUID(o.UID)
	}
	return Enlaces{
		UIDDe: func(id string) string {
			if eo := ec.Objetos[id]; eo != nil && eo.Local != "" {
				return uidLocal(eo.Local)
			}
			return ""
		},
		IDDe: func(uid string) string {
			uid = limpiarUID(uid)
			if uid == "" {
				return ""
			}
			for _, rid := range slices.Sorted(maps.Keys(ec.Objetos)) {
				if eo := ec.Objetos[rid]; eo.Local != "" && uidLocal(eo.Local) == uid {
					return rid
				}
			}
			return ""
		},
		Conocidos: func() []string {
			out := make([]string, 0, len(ec.Objetos))
			for k, eo := range ec.Objetos {
				if eo.Local != "" {
					out = append(out, k)
				}
			}
			slices.Sort(out)
			return out
		},
		SinEnlazar: func() []ObjetoLocal {
			releer()
			var out []ObjetoLocal
			for id, o := range locales {
				if !enlazado(ec, id) {
					out = append(out, o)
				}
			}
			slices.SortFunc(out, func(a, b ObjetoLocal) int { return strings.Compare(a.ID, b.ID) })
			return out
		},
	}
}

// limpiarUID quita el «urn:uuid:» de un UID.
func limpiarUID(uid string) string {
	uid = strings.TrimSpace(uid)
	if len(uid) > 9 && strings.EqualFold(uid[:9], "urn:uuid:") {
		uid = uid[9:]
	}
	return uid
}

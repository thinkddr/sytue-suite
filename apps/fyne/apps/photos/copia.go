// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/carrete"
)

// # La copia de seguridad de la galería
//
// Sytue Photos sube sola a «/Fotos» de Drive las fotos y los vídeos de la galería DEL
// SISTEMA —la fototeca de Fotos en iOS y macOS, la galería de Android, la carpeta de
// imágenes en Windows y Linux (`apps/nucleo/carrete`)—, como la copia de cámara de
// cualquier app de fotos. Este fichero es el motor, sin interfaz y sin red propia: la
// galería y el destino son interfaces, y los guardianes los cambian por unos de mentira.
// Lo usan la app abierta (copia_app.go) y el trabajo en segundo plano con la app cerrada
// (copia_fondo.go).
//
// Las reglas, y todas se prueban:
//
//  1. **Cada foto se sube UNA vez.** Se reconoce por el SHA-256 de su contenido, que es
//     también el `checksum` que Drive calcula al subir: lo que ya está en «/Fotos» —subido
//     a mano, desde la web o desde otro dispositivo— no se vuelve a subir. Lo ya visto se
//     recuerda en un registro en disco, por cuenta, para no leer la galería entera en
//     cada ronda.
//  2. **Nunca se pisa nada.** Drive SUSTITUÍA (nueva versión) un fichero subido con el
//     nombre de otro, y en un teléfono «IMG_0001.HEIC» se repite. Ahora la subida pide
//     conservar los dos (`on_conflict=keep_both`), y además cada foto sube con la fecha de
//     la toma delante —«2024-05-01 12.00.00 IMG_0001.HEIC»— y, si aun así el nombre está
//     cogido, con « (2)»: con un Drive anterior a la opción, tampoco se pisa nada.
//  3. **Subir no es indexar.** Lo que ya está en Drive cuenta como copiado aunque la
//     galería de Sytue no lo haya podido apuntar todavía: el índice se reintenta en la
//     ronda siguiente, sin volver a subir. Lo que syphotos no sabe medir (vídeos, RAW, o
//     un HEIC donde el sistema no lo convierte) queda en Drive como copia y no sale en la
//     galería.
//  4. **La galería de Sytue enseña JPEG, PNG y GIF.** Una foto HEIC, WebP o AVIF se pasa a
//     JPEG con sus metadatos en el propio dispositivo antes de subirla, como hace el
//     iPhone al pasar fotos a un ordenador (carrete.Convertir).
//  5. **Sin gastar datos.** Por defecto solo copia en una red que no es de pago (Wi-Fi); con
//     datos móviles, la ronda espera (ErrCopiaEsperaWifi). Se puede permitir en los ajustes.
//  6. **Una que falla no para a las demás**, pero tres seguidas sí: sin red, insistir foto
//     a foto solo gasta batería. La siguiente ronda empieza por las que faltan.

// GaleriaSistema es la galería del dispositivo tal y como la usa la copia de seguridad:
// `carrete` en la app ([CarreteDelSistema]) y una de mentira en los guardianes.
type GaleriaSistema interface {
	Disponible() bool
	Permiso() carrete.Permiso
	PedirPermiso(ctx context.Context) (carrete.Permiso, error)
	Listar(ctx context.Context) ([]carrete.Elemento, error)
	Abrir(ctx context.Context, e carrete.Elemento) (ruta string, limpiar func(), err error)
	Guardar(ctx context.Context, ruta, nombre string) (string, error)
	// Convertir pasa una foto a JPEG con sus metadatos; carrete.ErrNoDisponible donde
	// el sistema no sabe.
	Convertir(ctx context.Context, ruta string) (jpeg string, limpiar func(), err error)
	// RedMedida dice si la red de ahora es de pago (datos móviles).
	RedMedida() bool
}

// CarreteDelSistema es la galería de verdad, la de `apps/nucleo/carrete`.
type CarreteDelSistema struct{}

// Disponible es [carrete.Disponible].
func (CarreteDelSistema) Disponible() bool { return carrete.Disponible() }

// Permiso es [carrete.EstadoPermiso].
func (CarreteDelSistema) Permiso() carrete.Permiso { return carrete.EstadoPermiso() }

// PedirPermiso es [carrete.PedirPermiso].
func (CarreteDelSistema) PedirPermiso(ctx context.Context) (carrete.Permiso, error) {
	return carrete.PedirPermiso(ctx)
}

// Listar es [carrete.Listar].
func (CarreteDelSistema) Listar(ctx context.Context) ([]carrete.Elemento, error) {
	return carrete.Listar(ctx)
}

// Abrir es [carrete.Abrir].
func (CarreteDelSistema) Abrir(ctx context.Context, e carrete.Elemento) (string, func(), error) {
	return carrete.Abrir(ctx, e)
}

// Guardar es [carrete.Guardar].
func (CarreteDelSistema) Guardar(ctx context.Context, ruta, nombre string) (string, error) {
	return carrete.Guardar(ctx, ruta, nombre)
}

// Convertir es [carrete.Convertir].
func (CarreteDelSistema) Convertir(ctx context.Context, ruta string) (string, func(), error) {
	return carrete.Convertir(ctx, ruta)
}

// RedMedida es [carrete.RedMedida].
func (CarreteDelSistema) RedMedida() bool { return carrete.RedMedida() }

// Subido es lo que Drive dice de un fichero recién subido.
type Subido struct {
	// ID es el del fichero en Drive, con el que se indexa.
	ID string
	// Nombre es el que tiene en Drive, que con `keep_both` puede no ser el pedido.
	Nombre string
}

// DestinoCopia es a donde va la copia: «/Fotos» de Drive y el índice de syphotos
// ([destinoDrive]).
type DestinoCopia interface {
	// Contenido es lo que ya hay en el destino: las sumas SHA-256 (en hexadecimal) y los
	// nombres.
	Contenido(ctx context.Context) (sumas, nombres map[string]bool, err error)
	// Subir deja el fichero local `ruta` en Drive con `nombre`, sin sustituir ninguno.
	Subir(ctx context.Context, ruta, nombre, tipo string) (Subido, error)
	// Indexar lo apunta en la galería de Sytue. [ErrNoIndexable] si syphotos no sabe
	// medirlo: ahí no hay nada que reintentar.
	Indexar(ctx context.Context, id string) error
}

// Errores de la copia que la interfaz distingue.
var (
	// ErrCopiaEnMarcha: ya hay una ronda; esta no empieza.
	ErrCopiaEnMarcha = errors.New("photos: ya hay una copia de seguridad en marcha")
	// ErrCopiaSinRed: tres fallos seguidos; se deja para la siguiente ronda.
	ErrCopiaSinRed = errors.New("photos: la copia de seguridad se ha parado por fallos seguidos")
	// ErrCopiaEsperaWifi: la red es de pago y la copia no puede usarla (regla 5).
	ErrCopiaEsperaWifi = errors.New("photos: la copia de seguridad espera a una red Wi-Fi")
	// ErrNoIndexable: syphotos no sabe medir ese fichero (contesta 400). Ya está en
	// Drive, que es la copia; solo no sale en la galería.
	ErrNoIndexable = errors.New("photos: la galería no sabe enseñar este fichero")
	// ErrEnBoveda: subida cifrada al Drive privado de la persona (la bóveda, ADR
	// 0086), porque el Drive del servidor ya no admite escrituras. El servidor no
	// puede leerla, así que tampoco indexarla: está a salvo en Drive y no sale en la
	// galería del servidor. Va siempre junto a [ErrNoIndexable] (no hay nada que
	// reintentar).
	ErrEnBoveda = errors.New("photos: guardada cifrada en el Drive privado")
)

// FallosSeguidosCopia son los fallos seguidos que paran una ronda (regla 6).
const FallosSeguidosCopia = 3

// RefrescoContenidoCopia es cada cuánto se vuelve a leer lo que hay en «/Fotos»: lo que
// otro dispositivo subió de la misma foto (un iPhone y un Mac con la misma fototeca de
// iCloud) no se sube dos veces.
const RefrescoContenidoCopia = 24 * time.Hour

// ProgresoCopia es por dónde va una ronda.
type ProgresoCopia struct {
	// Hechas de Total pendientes al empezar la ronda.
	Hechas, Total int
	// Subidas son las que han subido de verdad; las demás ya estaban en Drive.
	Subidas int
	// Fallidas no se han podido subir; se reintentan en la ronda siguiente.
	Fallidas int
}

// AjustesCopia son los de ESTE dispositivo, no los de la cuenta: en el teléfono sí y en
// el portátil del trabajo no, por ejemplo. Van en el registro y no en las preferencias de
// la app porque los lee también el trabajo en segundo plano, que en Android corre sin la
// app (y sin sus preferencias). El cero es lo recomendado: apagada, solo con Wi-Fi y con
// vídeos.
type AjustesCopia struct {
	Activa bool `json:"activa"`
	// ConDatos deja copiar también con datos móviles (regla 5).
	ConDatos bool `json:"con_datos"`
	// SinVideos deja los vídeos fuera.
	SinVideos bool `json:"sin_videos"`
}

// Copia es la copia de seguridad de UN dispositivo. El cero no vale: se crea con
// [NuevaCopia] o, en la app, con [CopiaDe]. El registro vive en memoria, protegido por
// `mu`, y se vuelca a disco: así lo que se marca a media ronda (una foto que se acaba de
// guardar en la galería) no se pierde cuando la ronda escribe el suyo.
type Copia struct {
	galeria GaleriaSistema
	ruta    string
	ahora   func() time.Time

	mu       sync.Mutex
	f        *ficheroCopia
	enMarcha bool
}

// NuevaCopia guarda su registro en el fichero `ruta`. Una ruta vacía es un registro solo
// en memoria (los guardianes). En la app se usa [CopiaDe].
func NuevaCopia(g GaleriaSistema, ruta string) *Copia {
	return &Copia{galeria: g, ruta: ruta, ahora: time.Now}
}

var (
	copiasMu sync.Mutex
	copias   = map[string]*Copia{}
)

// CopiaDe es LA copia de un registro en este proceso: la app abierta y el trabajo en
// segundo plano comparten proceso en Android e iOS, y dos Copia sobre el mismo fichero
// serían dos rondas a la vez y dos registros que se pisan.
func CopiaDe(g GaleriaSistema, ruta string) *Copia {
	copiasMu.Lock()
	defer copiasMu.Unlock()
	if c := copias[ruta]; c != nil {
		return c
	}
	c := NuevaCopia(g, ruta)
	copias[ruta] = c
	return c
}

// RutaRegistroCopia es donde va el registro: en la carpeta de configuración del usuario,
// la misma con la app abierta y en segundo plano (en Android, `sesion` la pone dentro de
// la app antes de leer nada, también sin ventana).
func RutaRegistroCopia() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sytue", "photos", "copia-de-seguridad.json"), nil
}

// registroCopia es lo que la copia recuerda de una cuenta.
type registroCopia struct {
	// Hechas son los elementos de la galería ya resueltos (subidos, o que ya estaban), por
	// su ID en el dispositivo, con la suma de su contenido original. Una suma vacía es una
	// foto que guardó Sytue en la galería: no se sube.
	Hechas map[string]string `json:"hechas"`
	// Sumas y Nombres son lo que hay en el destino (reglas 1 y 2).
	Sumas   map[string]bool `json:"sumas"`
	Nombres map[string]bool `json:"nombres"`
	// PorIndexar son ficheros ya subidos que la galería aún no ha apuntado, por su ID en
	// Drive (regla 3).
	PorIndexar map[string]string `json:"por_indexar,omitempty"`
	// Leido es cuándo se leyó el destino por última vez.
	Leido time.Time `json:"leido"`
	// Ultima es cuándo terminó la última ronda completa.
	Ultima time.Time `json:"ultima"`
	// AvisoBoveda es cuándo se avisó por última vez de que la copia con la app cerrada
	// espera a que se abra la app para subir cifrado (ADR 0086); cero, no espera. Una
	// ronda completa lo borra.
	AvisoBoveda time.Time `json:"aviso_boveda,omitzero"`
}

// ficheroCopia es el registro en disco: los ajustes del dispositivo y uno por cuenta, con
// la cuenta reducida a una suma para no guardar quién es en claro.
type ficheroCopia struct {
	Ajustes AjustesCopia              `json:"ajustes"`
	Cuentas map[string]*registroCopia `json:"cuentas"`
}

func claveCuenta(cuenta string) string {
	s := sha256.Sum256([]byte(cuenta))
	return hex.EncodeToString(s[:12])
}

// cargar lee el fichero la primera vez. Con `mu` cogido.
func (c *Copia) cargar() {
	if c.f != nil {
		return
	}
	c.f = &ficheroCopia{}
	if c.ruta != "" {
		// Un registro roto se rehace: lo peor que pasa es volver a leer «/Fotos», y la
		// regla 1 evita subir nada dos veces.
		if datos, err := os.ReadFile(c.ruta); err == nil && json.Unmarshal(datos, c.f) != nil {
			c.f = &ficheroCopia{}
		}
	}
	if c.f.Cuentas == nil {
		c.f.Cuentas = map[string]*registroCopia{}
	}
}

// registro es el de una cuenta. Con `mu` cogido.
func (c *Copia) registro(cuenta string) *registroCopia {
	c.cargar()
	r := c.f.Cuentas[claveCuenta(cuenta)]
	if r == nil {
		r = &registroCopia{}
		c.f.Cuentas[claveCuenta(cuenta)] = r
	}
	if r.Hechas == nil {
		r.Hechas = map[string]string{}
	}
	if r.Sumas == nil {
		r.Sumas = map[string]bool{}
	}
	if r.Nombres == nil {
		r.Nombres = map[string]bool{}
	}
	if r.PorIndexar == nil {
		r.PorIndexar = map[string]string{}
	}
	return r
}

// escribir vuelca el registro a disco. Con `mu` cogido.
func (c *Copia) escribir() error {
	if c.ruta == "" || c.f == nil {
		return nil
	}
	datos, err := json.Marshal(c.f)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.ruta), 0o700); err != nil {
		return err
	}
	// Escribir aparte y renombrar: un corte a medias no deja el registro roto.
	tmp := c.ruta + ".nuevo"
	if err := os.WriteFile(tmp, datos, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.ruta)
}

// con ejecuta f con el registro de la cuenta y el cerrojo cogido.
func (c *Copia) con(cuenta string, f func(r *registroCopia)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	f(c.registro(cuenta))
}

// guardar es [con] seguido de escribir.
func (c *Copia) guardar(cuenta string, f func(r *registroCopia)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	f(c.registro(cuenta))
	return c.escribir()
}

// Ajustes son los de este dispositivo.
func (c *Copia) Ajustes() AjustesCopia {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cargar()
	return c.f.Ajustes
}

// CambiarAjustes los cambia con `f` y los guarda.
func (c *Copia) CambiarAjustes(f func(*AjustesCopia)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cargar()
	f(&c.f.Ajustes)
	return c.escribir()
}

// Marcar recuerda que el elemento `id` de la galería lo puso ahí Sytue (una foto bajada
// de la nube): la copia no lo sube.
func (c *Copia) Marcar(cuenta, id string) {
	if id != "" {
		_ = c.guardar(cuenta, func(r *registroCopia) { r.Hechas[id] = "" })
	}
}

// Ultima es cuándo terminó la última ronda completa de esta cuenta; cero, nunca.
func (c *Copia) Ultima(cuenta string) (t time.Time) {
	c.con(cuenta, func(r *registroCopia) { t = r.Ultima })
	return t
}

// Olvidar borra lo que la copia recuerda de una cuenta, no los ajustes del dispositivo.
func (c *Copia) Olvidar(cuenta string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.registro(cuenta)
	delete(c.f.Cuentas, claveCuenta(cuenta))
	_ = c.escribir()
}

// EnMarcha dice si hay una ronda.
func (c *Copia) EnMarcha() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enMarcha
}

// PuedeAhora dice si la red de ahora deja copiar: con Wi-Fi, o con datos si se permite.
func (c *Copia) PuedeAhora() bool { return c.Ajustes().ConDatos || !c.galeria.RedMedida() }

// Ronda sube lo que falte de la galería al destino, de lo más antiguo a lo más reciente,
// y avisa del progreso tras cada elemento. BLOQUEA: corre en una goroutine. Sin permiso
// para leer la galería, [carrete.ErrSinPermiso]; con otra ronda en marcha,
// [ErrCopiaEnMarcha]; con datos móviles y sin permitirlos, [ErrCopiaEsperaWifi].
func (c *Copia) Ronda(ctx context.Context, cuenta string, d DestinoCopia, avisar func(ProgresoCopia)) (ProgresoCopia, error) {
	var p ProgresoCopia
	c.mu.Lock()
	if c.enMarcha {
		c.mu.Unlock()
		return p, ErrCopiaEnMarcha
	}
	c.enMarcha = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.enMarcha = false
		_ = c.escribir()
		c.mu.Unlock()
	}()
	if avisar == nil {
		avisar = func(ProgresoCopia) {}
	}
	if err := c.puedeEmpezar(); err != nil {
		return p, err
	}
	if err := c.leerDestinoSiToca(ctx, cuenta, d); err != nil {
		return p, err
	}
	c.reindexar(ctx, cuenta, d)
	pendientes, err := c.pendientes(ctx, cuenta)
	if err != nil {
		return p, err
	}
	p.Total = len(pendientes)
	avisar(p)

	seguidos := 0
	for i, e := range pendientes {
		if err := ctx.Err(); err != nil {
			return p, err
		}
		if !c.PuedeAhora() {
			return p, ErrCopiaEsperaWifi // se ha ido la Wi-Fi a media ronda
		}
		subida, err := c.una(ctx, cuenta, e, d)
		p.Hechas = i + 1
		switch {
		case err == nil:
			seguidos = 0
			if subida {
				p.Subidas++
			}
		case ctx.Err() != nil:
			return p, ctx.Err()
		default:
			p.Fallidas++
			seguidos++
			if seguidos >= FallosSeguidosCopia {
				avisar(p)
				return p, errors.Join(ErrCopiaSinRed, err)
			}
		}
		// El registro va a disco cada pocas fotos: si la app se cierra a media ronda, la
		// siguiente no vuelve a leer lo ya hecho.
		if p.Hechas%10 == 0 {
			c.mu.Lock()
			_ = c.escribir()
			c.mu.Unlock()
		}
		avisar(p)
	}
	if p.Fallidas == 0 {
		c.con(cuenta, func(r *registroCopia) { r.Ultima, r.AvisoBoveda = c.ahora(), time.Time{} })
	}
	return p, nil
}

// EsperandoBoveda dice si la copia con la app cerrada dejó fotos esperando a que se
// abriera el Drive cifrado (y avisó de ello).
func (c *Copia) EsperandoBoveda(cuenta string) (espera bool) {
	c.con(cuenta, func(r *registroCopia) { espera = !r.AvisoBoveda.IsZero() })
	return espera
}

// CadaAvisoBoveda es cada cuánto, como mucho, se avisa de que la copia con la app
// cerrada espera a que se abra la app: una vez al día, no en cada despertar.
const CadaAvisoBoveda = 24 * time.Hour

// EsperaBoveda apunta que una ronda con la app cerrada no ha podido subir porque el
// Drive cifrado no está abierto en este proceso, y dice si toca avisar: hay algo que
// subir, la ronda podría correr (galería, permiso, red) y no se ha avisado en el último
// [CadaAvisoBoveda].
func (c *Copia) EsperaBoveda(ctx context.Context, cuenta string) (bool, error) {
	if err := c.puedeEmpezar(); err != nil {
		// Lo que la para es otra cosa (permiso, red), y ya se ve al abrir la app.
		return false, nil //nolint:nilerr // no es un fallo de avisar
	}
	pendientes, err := c.pendientes(ctx, cuenta)
	if err != nil || len(pendientes) == 0 {
		return false, err
	}
	avisar := false
	err = c.guardar(cuenta, func(r *registroCopia) {
		if r.AvisoBoveda.IsZero() || c.ahora().Sub(r.AvisoBoveda) >= CadaAvisoBoveda {
			r.AvisoBoveda, avisar = c.ahora(), true
		}
	})
	return avisar, err
}

// puedeEmpezar es lo que una ronda mira antes de tocar la red.
func (c *Copia) puedeEmpezar() error {
	switch {
	case !c.galeria.Disponible():
		return carrete.ErrNoDisponible
	case !c.galeria.Permiso().Lee():
		return carrete.ErrSinPermiso
	case !c.PuedeAhora():
		return ErrCopiaEsperaWifi
	}
	return nil
}

// leerDestinoSiToca apunta lo que hay en el destino la primera vez y una vez al día
// (RefrescoContenidoCopia).
func (c *Copia) leerDestinoSiToca(ctx context.Context, cuenta string, d DestinoCopia) error {
	var leido time.Time
	c.con(cuenta, func(r *registroCopia) { leido = r.Leido })
	if !leido.IsZero() && c.ahora().Sub(leido) <= RefrescoContenidoCopia {
		return nil
	}
	sumas, nombres, err := d.Contenido(ctx)
	if err != nil {
		return err
	}
	return c.guardar(cuenta, func(r *registroCopia) {
		for s := range sumas {
			r.Sumas[strings.ToLower(s)] = true
		}
		for n := range nombres {
			r.Nombres[n] = true
		}
		r.Leido = c.ahora()
	})
}

// reindexar reintenta apuntar en la galería lo subido que no se pudo (regla 3). Lo que
// syphotos no sabe medir se olvida; lo demás espera a la ronda siguiente.
func (c *Copia) reindexar(ctx context.Context, cuenta string, d DestinoCopia) {
	var ids []string
	c.con(cuenta, func(r *registroCopia) {
		for id := range r.PorIndexar {
			ids = append(ids, id)
		}
	})
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		if err := d.Indexar(ctx, id); err == nil || errors.Is(err, ErrNoIndexable) {
			c.con(cuenta, func(r *registroCopia) { delete(r.PorIndexar, id) })
		}
	}
}

// pendientes son los elementos de la galería que la copia no ha resuelto todavía, sin
// los vídeos si se han dejado fuera.
func (c *Copia) pendientes(ctx context.Context, cuenta string) ([]carrete.Elemento, error) {
	lista, err := c.galeria.Listar(ctx)
	if err != nil {
		return nil, err
	}
	sinVideos := c.Ajustes().SinVideos
	var pendientes []carrete.Elemento
	c.con(cuenta, func(r *registroCopia) {
		for _, e := range lista {
			if _, hecha := r.Hechas[e.ID]; !hecha && (!e.Video || !sinVideos) {
				pendientes = append(pendientes, e)
			}
		}
	})
	return pendientes, nil
}

// paraSubir es lo que de verdad va a Drive de un elemento.
type paraSubir struct {
	ruta, tipo string
	// suma es la de lo que sube, si no es el original (una foto convertida).
	suma string
	// indexar: la galería de Sytue sabe medirlo.
	indexar bool
	limpiar func()
}

// preparar decide qué se sube de un elemento ya abierto (reglas 3 y 4): el original si
// la galería de Sytue lo sabe medir o si es un vídeo; si no, un JPEG con sus metadatos;
// y si el sistema no sabe hacerlo, el original sin indexar, que sigue siendo la copia.
func (c *Copia) preparar(ctx context.Context, e carrete.Elemento, ruta string) paraSubir {
	tipo := e.Tipo
	if tipo == "" {
		tipo = carrete.TipoDe(e.Nombre)
	}
	original := paraSubir{ruta: ruta, tipo: tipo, limpiar: func() {}}
	switch {
	case e.Video || carrete.EsVideo(tipo):
		return original
	case SyphotosMide(tipo):
		original.indexar = true
		return original
	}
	jpeg, limpiar, err := c.galeria.Convertir(ctx, ruta)
	if err != nil {
		limpiar()
		return original
	}
	suma, err := sumaDe(jpeg)
	if err != nil {
		limpiar()
		return original
	}
	return paraSubir{ruta: jpeg, tipo: "image/jpeg", suma: suma, indexar: true, limpiar: limpiar}
}

// una resuelve un elemento: lo sube si su contenido no está ya en el destino. Dice si
// ha subido de verdad. Lo lento —leer, convertir y subir— va sin el cerrojo.
func (c *Copia) una(ctx context.Context, cuenta string, e carrete.Elemento, d DestinoCopia) (bool, error) {
	ruta, limpiar, err := c.galeria.Abrir(ctx, e)
	defer limpiar()
	if err != nil {
		return false, err
	}
	suma, err := sumaDe(ruta)
	if err != nil {
		return false, err
	}
	if c.yaEsta(cuenta, e, suma) {
		return false, nil
	}
	s := c.preparar(ctx, e, ruta)
	defer s.limpiar()
	if s.suma != "" && c.yaEsta(cuenta, e, s.suma) {
		// La foto convertida ya estaba: otro dispositivo con la misma fototeca la subió.
		c.con(cuenta, func(r *registroCopia) { r.Hechas[e.ID] = suma })
		return false, nil
	}
	var nombre string
	c.con(cuenta, func(r *registroCopia) {
		// Se reserva el nombre antes de subir: dos rondas no lo cogen a la vez.
		nombre = nombreLibre(nombreCopia(e, s.tipo), r.Nombres)
		r.Nombres[nombre] = true
	})
	subido, err := d.Subir(ctx, s.ruta, nombre, s.tipo)
	if err != nil {
		return false, err
	}
	c.con(cuenta, func(r *registroCopia) {
		r.Hechas[e.ID] = suma
		r.Sumas[suma] = true
		if s.suma != "" {
			r.Sumas[s.suma] = true
		}
		if subido.Nombre != "" {
			r.Nombres[subido.Nombre] = true
		}
	})
	if s.indexar {
		if err := d.Indexar(ctx, subido.ID); err != nil && !errors.Is(err, ErrNoIndexable) {
			// Ya es copia; la galería lo apuntará en la ronda siguiente (regla 3).
			c.con(cuenta, func(r *registroCopia) { r.PorIndexar[subido.ID] = suma })
		}
	}
	return true, nil
}

// yaEsta dice si un contenido ya está en el destino, y si lo está lo da por hecho.
func (c *Copia) yaEsta(cuenta string, e carrete.Elemento, suma string) (ya bool) {
	c.con(cuenta, func(r *registroCopia) {
		if ya = r.Sumas[suma]; ya {
			r.Hechas[e.ID] = suma
		}
	})
	return ya
}

// SyphotosMide dice si la galería de Sytue sabe medir (y enseñar) un tipo: los que
// decodifica la biblioteca estándar de Go en syphotos. Lo demás se convierte antes de
// subir, o se queda en Drive sin salir en la galería.
func SyphotosMide(tipo string) bool {
	switch tipo {
	case "image/jpeg", "image/png", "image/gif":
		return true
	}
	return false
}

func sumaDe(ruta string) (string, error) {
	f, err := os.Open(ruta) //nolint:gosec // la copia local que acaba de dejar la galería
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// nombreCopia es el nombre con el que sube un elemento: la fecha de la toma, en la hora
// del dispositivo, delante del nombre original (regla 2), y con la extensión de lo que
// sube de verdad (`.jpg` si se ha convertido).
func nombreCopia(e carrete.Elemento, tipo string) string {
	nombre := strings.TrimSpace(e.Nombre)
	if nombre == "" {
		nombre = "IMG" + extensionDe(tipo)
	} else if tipo == "image/jpeg" && carrete.TipoDe(nombre) != "image/jpeg" {
		nombre = strings.TrimSuffix(nombre, filepath.Ext(nombre)) + ".jpg"
	}
	if e.Tomada.IsZero() || e.Tomada.Unix() <= 0 {
		return nombre
	}
	return e.Tomada.Local().Format("2006-01-02 15.04.05") + " " + nombre
}

func extensionDe(tipo string) string {
	switch tipo {
	case "image/heic":
		return ".heic"
	case "image/png":
		return ".png"
	case "video/quicktime":
		return ".mov"
	case "video/mp4":
		return ".mp4"
	default:
		return ".jpg"
	}
}

// nombreLibre es `nombre`, o «nombre (2).ext» y siguientes si ya está cogido.
func nombreLibre(nombre string, cogidos map[string]bool) string {
	if !cogidos[nombre] {
		return nombre
	}
	ext := filepath.Ext(nombre)
	base := strings.TrimSuffix(nombre, ext)
	for n := 2; ; n++ {
		if c := base + " (" + strconv.Itoa(n) + ")" + ext; !cogidos[c] {
			return c
		}
	}
}

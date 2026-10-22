// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package service son los casos de uso de Sytue Cast sobre el store: dar de alta
// un vídeo, subir su original, verlo, listarlo, editarlo, compartirlo con personas
// concretas, reproducirlo y mover su ciclo de vida. Quién puede qué lo decide
// `internal/acceso`; este paquete lo aplica antes de tocar el store y no repite
// la regla.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/personas"
	"git.sytue.eu/sytue/one/services/cast/internal/acceso"
	"git.sytue.eu/sytue/one/services/cast/internal/cuota"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/media"
	"git.sytue.eu/sytue/one/services/cast/internal/social"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

// Errores propios de los casos de uso.
var (
	// ErrSinDirectorio: no hay Sytue ID al que preguntar quién es «@ana».
	ErrSinDirectorio = errors.New("cast: sin directorio de personas")
	// ErrSinAlmacen: no hay almacén de objetos configurado.
	ErrSinAlmacen = errors.New("cast: sin almacén de objetos")
)

// Quien es quien pregunta: lo que dice su token, o nadie.
type Quien struct {
	Sub     string
	Tenant  string
	Nombre  string
	Usuario string
	// Admin: rol owner o admin en SU organización.
	Admin   bool
	Anonimo bool
	// Token es el bearer con el que se pregunta a Sytue ID y a Drive en nombre de
	// esta persona (su límite, su cuota).
	Token string
	// IP es de quien pregunta, para contar una vista por persona y hora sin
	// guardar quién: solo se usa troceada (hash) y nunca sale de la memoria.
	IP string
}

// Nadie es la petición sin sesión.
var Nadie = Quien{Anonimo: true}

func (q Quien) conSesion() bool { return !q.Anonimo && q.Sub != "" && q.Tenant != "" }

func (q Quien) acceso() acceso.Quien {
	return acceso.Quien{Sub: q.Sub, Tenant: q.Tenant, Admin: q.Admin, Anonimo: q.Anonimo}
}

// Permisos es lo que quien pregunta puede hacer con el vídeo.
type Permisos = acceso.Permisos

// Servicio son los casos de uso de Cast.
type Servicio struct {
	// fondo acota las tareas en segundo plano (purgas, declaraciones de cuota) y
	// las espera al apagar.
	fondo    chan struct{}
	enMarcha sync.WaitGroup
	// turnoDeCopia acota las copias desde Drive a la vez (maxCopiando) y copiando
	// las cuenta (drive.go).
	turnoDeCopia chan struct{}
	copiando     sync.WaitGroup
	cerrojos     sync.Map // tenant → *sync.Mutex
	store        *store.Store
	directorio   personas.Directorio
	objetos      Objetos
	cuota        Cuota
	drive        Drive
	firmante     *media.Firmante
	vistas       *vistas
	logger       *slog.Logger
}

// Deps son las dependencias del servicio. Las opcionales pueden ir a nil y el caso
// de uso que las necesita falla con claridad (fail-closed), no en silencio.
type Deps struct {
	Store *store.Store
	// Directorio resuelve «@ana» en Sytue ID. nil = no se comparte con nadie.
	Directorio personas.Directorio
	// Objetos es el almacén de Cast. nil = no se sube nada.
	Objetos Objetos
	// Cuota es Drive. nil = sin cuota (solo en pruebas; en producción va siempre).
	Cuota Cuota
	// Drive lee los ficheros de Drive que se suben a Cast. nil = «Subir a Cast»
	// contesta 503.
	Drive Drive
	// Firmante emite los enlaces de /media. Obligatorio.
	Firmante *media.Firmante
	Logger   *slog.Logger
}

// New construye el servicio.
func New(d Deps) *Servicio {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &Servicio{
		store: d.Store, directorio: d.Directorio, objetos: d.Objetos, cuota: d.Cuota, drive: d.Drive,
		firmante: d.Firmante, vistas: nuevasVistas(), logger: d.Logger,
		fondo: make(chan struct{}, maxEnFondo), turnoDeCopia: make(chan struct{}, maxCopiando),
	}
}

// Objetos es lo que el servicio necesita del almacén (platform/pkg/objectstore,
// envuelto por `internal/almacen` con el bucket de Cast). Las claves son
// relativas a la organización (`<video_id>/original`, `<video_id>/hls/…`): el
// almacén les pone el prefijo del tenant y elige el bucket (ADR 0038).
type Objetos interface {
	Put(ctx context.Context, tenant, key string, r io.Reader, size int64, contentType string) error
	// Get abre un objeto, entero o el rango que pide la cabecera `Range` (tal cual
	// llega; vacía = entero). domain.ErrNoExiste si no está.
	Get(ctx context.Context, tenant, key, rango string) (Fichero, error)
	// DeletePrefix borra todo lo que cuelga de un prefijo (todo lo de un vídeo).
	DeletePrefix(ctx context.Context, tenant, prefix string) error
}

// RangoNoSatisfacible: el Range pedido cae fuera del fichero (416), con su tamaño.
type RangoNoSatisfacible struct{ Total int64 }

func (e *RangoNoSatisfacible) Error() string { return "cast: rango no satisfacible" }

// Fichero es un objeto abierto para servirlo en streaming.
type Fichero struct {
	Cuerpo io.ReadCloser
	// Tamano es el del cuerpo que se devuelve (el del rango, si lo hay).
	Tamano      int64
	ContentType string
	// ContentRange va relleno cuando se sirvió un rango (`bytes a-b/total`).
	ContentRange string
	// Modificado, para Last-Modified.
	Modificado time.Time
}

// Abrir sirve para /media: comprueba el token, resuelve la ruta pedida a la clave
// real del vídeo y abre el objeto. La ACL ya la pasó quien pidió `play`: el token
// lo prueba.
func (s *Servicio) Abrir(ctx context.Context, id uuid.UUID, token, ruta, rango string) (Fichero, error) {
	if s.objetos == nil {
		return Fichero{}, ErrSinAlmacen
	}
	if err := s.firmante.Verificar(id, token, ruta); err != nil {
		return Fichero{}, domain.ErrNoExiste
	}
	// Se lee en mantenimiento porque el token no dice de qué organización es el
	// vídeo; y solo lo que ya se puede ver o pertenece a la ficha (un original en
	// `uploading` no se sirve).
	v, err := s.store.PorID(ctx, id)
	if err != nil {
		return Fichero{}, err
	}
	clave, err := claveDe(v, ruta)
	if err != nil {
		return Fichero{}, err
	}
	f, err := s.objetos.Get(ctx, v.Tenant, clave, rango)
	if err != nil {
		return Fichero{}, err
	}
	if f.ContentType == "" || f.ContentType == "application/octet-stream" {
		f.ContentType = TipoDe(ruta)
	}
	return f, nil
}

// claveDe traduce la ruta que ve el cliente a la clave real del almacén. Los
// segmentos y variantes del HLS cuelgan del directorio de su master.
//
// Toda clave ha de colgar del prefijo del vídeo (`<video_id>/`): una fila con una
// clave rara —a mano, o de un trabajo mal escrito— no abre nada fuera de él.
func claveDe(v domain.Video, ruta string) (string, error) {
	ruta = strings.TrimPrefix(ruta, "/")
	propia := func(k string) (string, error) {
		if k == "" || !strings.HasPrefix(k, Prefijo(v)) || strings.Contains(k, "..") {
			return "", domain.ErrNoExiste
		}
		return k, nil
	}
	switch {
	case ruta == media.RutaPoster:
		return propia(v.PosterKey)
	case strings.HasPrefix(ruta, "webm/"):
		// webm/<q>p.webm de una calidad que existe. Un vídeo de antes de la
		// escalera tiene una sola, la de su webm_key (que se llama 720p aunque el
		// original fuera más pequeño).
		q, ok := media.CalidadDeRutaWebM(ruta)
		if !ok || !slices.Contains(v.CalidadesWebM(), q) {
			return "", domain.ErrNoExiste
		}
		if v.WebmQualities == nil {
			return propia(v.WebmKey)
		}
		return propia(Prefijo(v) + ruta)
	case ruta == media.RutaOriginal:
		return propia(v.OriginalKey)
	case strings.HasPrefix(ruta, "hls/") && v.HlsKey != "":
		resto := strings.TrimPrefix(ruta, "hls/")
		if resto == "" || strings.Contains(resto, "..") || strings.HasPrefix(resto, "/") || strings.Contains(resto, "//") {
			return "", domain.ErrNoExiste
		}
		master, err := propia(v.HlsKey)
		if err != nil {
			return "", err
		}
		if resto == "master.m3u8" {
			return master, nil
		}
		return propia(master[:strings.LastIndex(master, "/")+1] + resto)
	}
	return "", domain.ErrNoExiste
}

// TipoDe es el Content-Type de cada fichero de un vídeo, por extensión. Es el
// ÚNICO sitio que lo decide: el transcodificador guarda las derivadas con él y
// /media lo repite si el almacén no trae uno. `application/vnd.apple.mpegurl`
// para que Safari reproduzca en nativo, `video/iso.segment` para los fMP4 (RFC
// 6381 / ISO 23000-19).
func TipoDe(ruta string) string {
	switch strings.ToLower(path.Ext(ruta)) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".m4s":
		return "video/iso.segment"
	case ".mp4":
		return "video/mp4"
	case ".ts":
		return "video/mp2t"
	case ".webm":
		return "video/webm"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".png":
		return "image/png"
	}
	return "application/octet-stream"
}

// Cuota es la cuota del plan, que lleva Drive (§8). Cast pregunta con el token de
// la persona y declara su total por organización con el secreto interno.
type Cuota interface {
	// Consultar trae la cuota de la organización de quien llama tal como la cuenta Drive.
	Consultar(ctx context.Context, bearer string) (cuota.Resumen, error)
	// Disponible dice cuántos bytes le quedan a la organización de quien llama.
	Disponible(ctx context.Context, bearer string) (int64, error)
	// Declarar fija el total que Cast ocupa en esa organización (absoluto, idempotente).
	Declarar(ctx context.Context, tenant string, bytes int64) error
}

// Firmante es el que emite los enlaces de /media, para el servidor de ficheros.
func (s *Servicio) Firmante() *media.Firmante { return s.firmante }

// ResumenDeCuota es lo que enseña `GET /api/v1/quota`: la cuota del plan según
// Drive y cuánto de eso es de Cast.
type ResumenDeCuota struct {
	cuota.Resumen
	CastBytes int64
}

// Cuota devuelve la cuota de la organización de quien llama. Sin Drive
// configurado, solo lo de Cast, con el resto a cero: la interfaz sabe leerlo.
func (s *Servicio) Cuota(ctx context.Context, quien Quien) (ResumenDeCuota, error) {
	if !quien.conSesion() {
		return ResumenDeCuota{}, domain.ErrProhibido
	}
	var out ResumenDeCuota
	if s.cuota != nil {
		r, err := s.cuota.Consultar(ctx, quien.Token)
		switch {
		case errors.Is(err, cuota.ErrNoDisponible):
			// Sin Drive se enseña lo de Cast y el resto a cero, como sin cuota: el
			// medidor dice «no disponible» en vez de romper la página.
			s.logger.Warn("cast: Drive no contesta la cuota", "tenant", quien.Tenant, "error", err)
		case err != nil:
			return ResumenDeCuota{}, err
		default:
			out.Resumen = r
		}
	}
	n, err := s.store.BytesDeOrganizacion(ctx, quien.Tenant)
	if err != nil {
		return ResumenDeCuota{}, err
	}
	out.CastBytes = n
	return out, nil
}

// Ver resuelve el vídeo y los permisos de quien pregunta. Cuenta la vista (una por
// vídeo, persona o IP y hora), que es lo que §5.3 pide de todo GET de una ficha.
func (s *Servicio) Ver(ctx context.Context, quien Quien, id uuid.UUID) (domain.Video, Permisos, error) {
	v, p, err := acceso.Resolver(ctx, s.store, quien.acceso(), id)
	if err != nil {
		return domain.Video{}, Permisos{}, err
	}
	if !p.Editar && s.vistas.nueva(v.ID, quien) {
		if err := s.store.RegistrarVista(ctx, v.Tenant, v.ID); err != nil {
			// Una vista que no se cuenta no es motivo para no enseñar el vídeo.
			s.logger.Warn("cast: no se pudo contar la vista", "video", v.ID, "error", err)
		} else {
			v.Views++
		}
	}
	return v, p, nil
}

// Resolver es Ver sin contar la vista: para las operaciones que no son «mirar la
// ficha» (editar, compartir, reproducir, lo social).
func (s *Servicio) Resolver(ctx context.Context, quien Quien, id uuid.UUID) (domain.Video, Permisos, error) {
	return acceso.Resolver(ctx, s.store, quien.acceso(), id)
}

// AccesoSocial es el resolutor que lo social (comentarios, chat) usa para saber
// qué vídeo es y quién modera. Misma regla, otro tipo.
func (s *Servicio) AccesoSocial() social.Acceso {
	return func(ctx context.Context, q social.Quien, id uuid.UUID) (social.Video, social.Permisos, error) {
		v, p, err := s.Resolver(ctx, Quien{Sub: q.Sub, Tenant: q.Tenant, Admin: q.Admin, Anonimo: q.Anonimo}, id)
		if err != nil {
			if errors.Is(err, domain.ErrNoExiste) {
				return social.Video{}, social.Permisos{}, social.ErrNoExiste
			}
			return social.Video{}, social.Permisos{}, err
		}
		return social.Video{
				ID: v.ID, Tenant: v.Tenant, OwnerSub: v.OwnerSub, OwnerName: v.OwnerName, Titulo: v.Title,
				ComentariosActivos: v.CommentsEnabled, ChatActivo: v.ChatEnabled,
			},
			social.Permisos{Ver: p.Ver, Editar: p.Editar, Moderar: p.Moderar}, nil
	}
}

// ── Alta y edición ──────────────────────────────────────────────────────────

// Alta es lo que se pide para dar de alta un vídeo.
type Alta struct {
	Kind        domain.Kind
	Title       string
	Description string
	// Visibility vacía = org.
	Visibility domain.Visibility
	// Solo directo (lo rellena el paquete de directo, que conoce el room).
	LiveSource domain.LiveSource
	LiveRoom   string
	// Qualities es la elección de calidades (vacía = la recomendada). Aún no se
	// sabe qué da el original, así que se valida contra las tres y el
	// transcodificador recorta lo que no llegue (domain.Resolver).
	Qualities []int
}

// Crear da de alta un vídeo en la organización de quien llama. Un `upload` nace
// `uploading` a la espera del fichero; un `live`, `scheduled`.
func (s *Servicio) Crear(ctx context.Context, quien Quien, a Alta) (domain.Video, error) {
	if !quien.conSesion() {
		return domain.Video{}, domain.ErrProhibido
	}
	if !a.Kind.Valida() {
		return domain.Video{}, fmt.Errorf("%w: clase %q desconocida", domain.ErrEntradaInvalida, a.Kind)
	}
	if a.Visibility == "" {
		a.Visibility = domain.VisibilityOrg
	}
	if !a.Visibility.Valida() {
		return domain.Video{}, fmt.Errorf("%w: visibilidad %q desconocida", domain.ErrEntradaInvalida, a.Visibility)
	}
	title, err := domain.Titulo(a.Title)
	if err != nil {
		return domain.Video{}, err
	}
	desc, err := domain.Descripcion(a.Description)
	if err != nil {
		return domain.Video{}, err
	}
	if a.Kind != domain.KindLive {
		a.LiveSource, a.LiveRoom = "", ""
	} else if a.LiveSource != "" && !a.LiveSource.Valida() {
		return domain.Video{}, fmt.Errorf("%w: origen %q desconocido", domain.ErrEntradaInvalida, a.LiveSource)
	}
	var elegidas []int32
	if len(a.Qualities) > 0 {
		qs, err := domain.ValidarCalidades(a.Qualities, domain.Opciones(0, 0))
		if err != nil {
			return domain.Video{}, err
		}
		elegidas = domain.AInt32(qs)
	}
	return s.store.Crear(ctx, store.Nuevo{
		Tenant: quien.Tenant, OwnerSub: quien.Sub, OwnerName: quien.Nombre,
		Kind: a.Kind, Visibility: a.Visibility, Title: title, Description: desc,
		LiveSource: a.LiveSource, LiveRoom: a.LiveRoom, Qualities: elegidas,
	})
}

// editable resuelve el vídeo y exige poder editarlo.
func (s *Servicio) editable(ctx context.Context, quien Quien, id uuid.UUID) (domain.Video, error) {
	if !quien.conSesion() {
		return domain.Video{}, domain.ErrProhibido
	}
	v, p, err := s.Resolver(ctx, quien, id)
	if err != nil {
		return domain.Video{}, err
	}
	if !p.Editar {
		return domain.Video{}, domain.ErrProhibido
	}
	return v, nil
}

// Actualizar aplica los cambios del dueño (o de quien administra).
func (s *Servicio) Actualizar(ctx context.Context, quien Quien, id uuid.UUID, c domain.Cambios) (domain.Video, error) {
	v, err := s.editable(ctx, quien, id)
	if err != nil {
		return domain.Video{}, err
	}
	if err := c.Normalizar(); err != nil {
		return domain.Video{}, err
	}
	if c.Vacios() {
		return v, nil
	}
	return s.store.Actualizar(ctx, v.Tenant, v.ID, c)
}

// Borrar retira el vídeo (borrado blando), purga sus objetos en segundo plano y
// vuelve a declarar la cuota. El borrado en la base va primero: si la purga
// falla, quedan objetos huérfanos en el almacén (se recogen después), no un vídeo
// visible sin ficheros.
func (s *Servicio) Borrar(ctx context.Context, quien Quien, id uuid.UUID) error {
	v, err := s.editable(ctx, quien, id)
	if err != nil {
		return err
	}
	borrado, err := s.store.Borrar(ctx, v.Tenant, v.ID)
	if err != nil {
		return err
	}
	// Desligado de la petición: el cliente no espera a que S3 termine.
	fondo := context.WithoutCancel(ctx)
	s.enFondo(func() { s.purgar(fondo, borrado) })
	s.enFondo(func() { s.declararCuota(fondo, borrado.Tenant) })
	return nil
}

// purgar borra del almacén todo lo del vídeo (original y derivadas). Si falla,
// lo registra: el barrido periódico lo reintenta (barrido.go).
func (s *Servicio) purgar(ctx context.Context, v domain.Video) {
	if err := s.borrarObjetos(ctx, v); err != nil {
		s.logger.Warn("cast: no se pudieron purgar los ficheros del vídeo", "video", v.ID, "error", err)
	}
}

// borrarObjetos es purgar devolviendo el error. Sin almacén no hay nada que borrar.
func (s *Servicio) borrarObjetos(ctx context.Context, v domain.Video) error {
	if s.objetos == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	return s.objetos.DeletePrefix(ctx, v.Tenant, Prefijo(v))
}

// Prefijo es dónde viven en el almacén de su organización los ficheros de un
// vídeo: `<video_id>/`. El original en `original`, las derivadas debajo.
func Prefijo(v domain.Video) string { return v.ID.String() + "/" }

// ── Subida del original ─────────────────────────────────────────────────────

// SubirOriginal recibe el fichero de un vídeo `uploading`, lo deja en el almacén y
// encola su transcodificación (§5.2 `PUT /videos/{id}/original`). Antes de aceptar
// un byte comprueba el tope por vídeo y la cuota en Drive; después declara lo que
// ocupa. Si Drive no contesta, la subida sigue: `// ponytail: fail-open en la
// cuota; el dueño prefirió no bloquear la subida por una caída de Drive`.
func (s *Servicio) SubirOriginal(ctx context.Context, quien Quien, id uuid.UUID, cuerpo io.Reader, tamano int64, contentType string) (domain.Video, error) {
	v, err := s.editable(ctx, quien, id)
	if err != nil {
		return domain.Video{}, err
	}
	if s.objetos == nil {
		return domain.Video{}, ErrSinAlmacen
	}
	if v.Kind != domain.KindUpload || v.Status != domain.StatusUploading {
		return domain.Video{}, fmt.Errorf("%w: el vídeo no está esperando su fichero (%s)", domain.ErrConflicto, v.Status)
	}
	if tamano <= 0 {
		return domain.Video{}, fmt.Errorf("%w: hace falta Content-Length", domain.ErrEntradaInvalida)
	}
	if tamano > domain.MaxOriginalBytes {
		return domain.Video{}, fmt.Errorf("%w: %d bytes; el tope por vídeo es %d", domain.ErrDemasiadoGrande, tamano, domain.MaxOriginalBytes)
	}
	if !strings.HasPrefix(contentType, "video/") && contentType != "application/octet-stream" {
		return domain.Video{}, fmt.Errorf("%w: el fichero no es un vídeo (%s)", domain.ErrEntradaInvalida, contentType)
	}
	if err := s.comprobarCuota(ctx, quien, tamano); err != nil {
		return domain.Video{}, err
	}

	clave := Prefijo(v) + media.RutaOriginal
	if err := s.objetos.Put(ctx, v.Tenant, clave, cuerpo, tamano, contentType); err != nil {
		return domain.Video{}, fmt.Errorf("cast: guardar el original: %w", err)
	}
	// Los bytes ya están guardados: registrarlos y encolar no se aborta porque el
	// cliente cuelgue justo ahora (sería un original huérfano). Una transacción: o
	// queda el vídeo procesándose con su trabajo, o no cambia nada.
	fondo := context.WithoutCancel(ctx)
	v, err = s.store.RecibirOriginal(fondo, v.Tenant, v.ID, clave, tamano)
	if err != nil {
		return domain.Video{}, err
	}
	s.enFondo(func() { s.declararCuota(fondo, v.Tenant) })
	return v, nil
}

// comprobarCuota pregunta a Drive, con el token de quien sube, si caben `tamano`
// bytes más en el plan. Si Drive no contesta, se deja pasar (§8); si contesta que
// no, no.
func (s *Servicio) comprobarCuota(ctx context.Context, quien Quien, tamano int64) error {
	if s.cuota == nil {
		return nil
	}
	libres, err := s.cuota.Disponible(ctx, quien.Token)
	switch {
	case errors.Is(err, cuota.ErrNoDisponible):
		// Solo si Drive NO CONTESTA; si contesta que no, no se sube.
		s.logger.Warn("cast: Drive no contesta la cuota; se acepta la subida", "tenant", quien.Tenant, "error", err)
	case err != nil:
		return fmt.Errorf("cast: comprobar la cuota: %w", err)
	case tamano > libres:
		return &domain.SinCuota{Disponibles: libres}
	}
	return nil
}

// declararCuota suma lo que Cast ocupa en la organización y se lo dice a Drive.
func (s *Servicio) declararCuota(ctx context.Context, tenant string) {
	if s.cuota == nil {
		return
	}
	// Una declaración a la vez por organización: el valor es absoluto, y dos
	// sumas concurrentes podrían llegar a Drive en el orden contrario al que se
	// calcularon y dejar allí la vieja.
	mu := s.cerrojoDe(tenant)
	mu.Lock()
	defer mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	total, err := s.store.BytesDeOrganizacion(ctx, tenant)
	if err != nil {
		s.logger.Warn("cast: no se pudo sumar el uso", "tenant", tenant, "error", err)
		return
	}
	if err := s.cuota.Declarar(ctx, tenant, total); err != nil {
		s.logger.Warn("cast: no se pudo declarar el uso a Drive", "tenant", tenant, "bytes", total, "error", err)
	}
}

// DeclararCuota es declararCuota para quien termina un trabajo (el transcodificador).
func (s *Servicio) DeclararCuota(ctx context.Context, tenant string) { s.declararCuota(ctx, tenant) }

// SubirPoster guarda un póster propio (imagen ≤ 5 MiB) y lo fija en el vídeo.
func (s *Servicio) SubirPoster(ctx context.Context, quien Quien, id uuid.UUID, cuerpo io.Reader, tamano int64, contentType string) (domain.Video, error) {
	v, err := s.editable(ctx, quien, id)
	if err != nil {
		return domain.Video{}, err
	}
	if s.objetos == nil {
		return domain.Video{}, ErrSinAlmacen
	}
	if tamano <= 0 || tamano > domain.MaxPosterBytes {
		return domain.Video{}, fmt.Errorf("%w: el póster ha de tener entre 1 byte y %d", domain.ErrEntradaInvalida, domain.MaxPosterBytes)
	}
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/webp" {
		return domain.Video{}, fmt.Errorf("%w: el póster ha de ser JPEG, PNG o WebP", domain.ErrEntradaInvalida)
	}
	clave := Prefijo(v) + "poster-propio"
	if err := s.objetos.Put(ctx, v.Tenant, clave, cuerpo, tamano, contentType); err != nil {
		return domain.Video{}, fmt.Errorf("cast: guardar el póster: %w", err)
	}
	return s.store.FijarPoster(ctx, v.Tenant, v.ID, clave)
}

// PosterDeFotograma pide al transcodificador un póster del instante dado.
func (s *Servicio) PosterDeFotograma(ctx context.Context, quien Quien, id uuid.UUID, atMS int64) (domain.Video, error) {
	v, err := s.editable(ctx, quien, id)
	if err != nil {
		return domain.Video{}, err
	}
	if atMS < 0 || (v.DurationMS != nil && atMS > *v.DurationMS) {
		return domain.Video{}, fmt.Errorf("%w: el instante está fuera del vídeo", domain.ErrEntradaInvalida)
	}
	if v.Status != domain.StatusReady {
		return domain.Video{}, fmt.Errorf("%w: el vídeo aún no está listo", domain.ErrConflicto)
	}
	payload, _ := json.Marshal(map[string]int64{"at_ms": atMS})
	if _, err := s.store.Encolar(ctx, v.Tenant, v.ID, domain.JobPoster, payload); err != nil {
		return domain.Video{}, err
	}
	return s.store.EnOrganizacion(ctx, v.Tenant, v.ID)
}

// ── Listados ────────────────────────────────────────────────────────────────

// Filtro acota la biblioteca (§5.2 `GET /videos`).
type Filtro struct {
	// Mios: `scope=mine`; si no, la organización.
	Mios   bool
	Status domain.Status
	Q      string
	// Cursor opaco y límite (1..100, 24 por defecto).
	Cursor string
	Limite int
}

// Pagina es un listado con su cursor para seguir; vacío al acabar.
type Pagina struct {
	Videos    []domain.Video
	Siguiente string
}

// porPagina es cuántos vídeos van por página si no se pide otra cosa, y
// maxPorPagina el tope que se admite pedir.
const (
	porPagina    = 24
	maxPorPagina = 100
)

func acotar(n int) int {
	if n <= 0 {
		return porPagina
	}
	return min(n, maxPorPagina)
}

// paginar recorta a `limite` y saca el cursor del último si sobró alguno. La
// clave de orden la da `clave`, porque la portada ordena por publicación y el
// resto por alta.
func paginar(vs []domain.Video, limite int, clave func(domain.Video) time.Time) Pagina {
	p := Pagina{Videos: vs}
	if len(vs) > limite {
		p.Videos = vs[:limite]
		u := p.Videos[len(p.Videos)-1]
		p.Siguiente = store.Cursor{Fecha: clave(u), ID: u.ID}.Cifrar()
	}
	if p.Videos == nil {
		p.Videos = []domain.Video{}
	}
	return p
}

func porAlta(v domain.Video) time.Time { return v.CreatedAt }

func porPublicacion(v domain.Video) time.Time {
	if v.PublishedAt != nil {
		return *v.PublishedAt
	}
	return v.CreatedAt
}

// Biblioteca lista lo que quien pregunta ve en su organización.
func (s *Servicio) Biblioteca(ctx context.Context, quien Quien, f Filtro) (Pagina, error) {
	if !quien.conSesion() {
		return Pagina{}, domain.ErrProhibido
	}
	if f.Status != "" && !domain.PuedePasar(f.Status, f.Status) && !esEstado(f.Status) {
		return Pagina{}, fmt.Errorf("%w: estado %q desconocido", domain.ErrEntradaInvalida, f.Status)
	}
	desde, err := store.DescifrarCursor(f.Cursor)
	if err != nil {
		return Pagina{}, err
	}
	limite := acotar(f.Limite)
	sf := store.Filtro{Status: f.Status, Q: f.Q, Desde: desde, Limite: limite + 1}
	if f.Mios {
		sf.OwnerSub = quien.Sub
	}
	vs, err := s.store.Biblioteca(ctx, quien.Tenant, quien.Sub, quien.Admin, sf)
	if err != nil {
		return Pagina{}, err
	}
	return paginar(vs, limite, porAlta), nil
}

func esEstado(st domain.Status) bool {
	switch st {
	case domain.StatusUploading, domain.StatusProcessing, domain.StatusReady, domain.StatusFailed,
		domain.StatusScheduled, domain.StatusLive, domain.StatusEnded:
		return true
	}
	return false
}

// Compartidos lista los vídeos de otras organizaciones que le han compartido.
func (s *Servicio) Compartidos(ctx context.Context, quien Quien, cursor string, limite int) (Pagina, error) {
	if !quien.conSesion() {
		return Pagina{}, domain.ErrProhibido
	}
	desde, err := store.DescifrarCursor(cursor)
	if err != nil {
		return Pagina{}, err
	}
	limite = acotar(limite)
	vs, err := s.store.Compartidos(ctx, quien.Sub, quien.Tenant, desde, limite+1)
	if err != nil {
		return Pagina{}, err
	}
	return paginar(vs, limite, porAlta), nil
}

// Portada es la portada pública (§5.3 `/public/explore`): los directos en curso
// aparte (solo en la primera página) y los vídeos por publicación, paginados.
type Portada struct {
	Directos []domain.Video
	Pagina
}

// Explore lista lo público de todas las organizaciones. Sin sesión.
func (s *Servicio) Explore(ctx context.Context, cursor string, limite int) (Portada, error) {
	desde, err := store.DescifrarCursor(cursor)
	if err != nil {
		return Portada{}, err
	}
	limite = acotar(limite)
	directos, vs, err := s.store.Explore(ctx, desde, limite+1)
	if err != nil {
		return Portada{}, err
	}
	if directos == nil {
		directos = []domain.Video{}
	}
	return Portada{Directos: directos, Pagina: paginar(vs, limite, porPublicacion)}, nil
}

// Canal lista lo público de una organización. Sin sesión.
func (s *Servicio) Canal(ctx context.Context, org, cursor string, limite int) (Pagina, error) {
	org = strings.TrimSpace(org)
	if org == "" {
		return Pagina{}, fmt.Errorf("%w: organización vacía", domain.ErrEntradaInvalida)
	}
	desde, err := store.DescifrarCursor(cursor)
	if err != nil {
		return Pagina{}, err
	}
	limite = acotar(limite)
	vs, err := s.store.Canal(ctx, org, desde, limite+1)
	if err != nil {
		return Pagina{}, err
	}
	return paginar(vs, limite, porPublicacion), nil
}

// ── Personas ────────────────────────────────────────────────────────────────

// Personas lista con quién está compartido el vídeo. Solo quien lo edita: la
// lista dice quién más lo ve, y eso no es de los espectadores.
func (s *Servicio) Personas(ctx context.Context, quien Quien, id uuid.UUID) ([]domain.Persona, error) {
	v, err := s.editable(ctx, quien, id)
	if err != nil {
		return nil, err
	}
	ps, err := s.store.Personas(ctx, v.Tenant, v.ID)
	if ps == nil {
		ps = []domain.Persona{}
	}
	return ps, err
}

// Compartir añade a la lista del vídeo a quien responda a `consulta` («@ana»,
// «ana» o «ana@acme.com»), de cualquier organización. Quién es lo dice Sytue ID
// con el token de quien llama, nunca el navegador. No cambia la visibilidad: el
// dueño la pone en `people` cuando quiera que la lista mande.
func (s *Servicio) Compartir(ctx context.Context, quien Quien, id uuid.UUID, consulta string) (domain.Persona, error) {
	v, err := s.editable(ctx, quien, id)
	if err != nil {
		return domain.Persona{}, err
	}
	if s.directorio == nil {
		return domain.Persona{}, ErrSinDirectorio
	}
	consulta = strings.TrimSpace(consulta)
	if consulta == "" {
		return domain.Persona{}, fmt.Errorf("%w: a quién", domain.ErrEntradaInvalida)
	}
	p, err := s.directorio.Resolver(personas.ConBearer(ctx, quien.Token), consulta)
	if err != nil {
		return domain.Persona{}, traducirPersonas(err)
	}
	if p.UserID == v.OwnerSub {
		return domain.Persona{}, fmt.Errorf("%w: el dueño ya lo ve", domain.ErrEntradaInvalida)
	}
	persona := domain.Persona{UserID: p.UserID, Name: p.Visible(), Username: p.Username, AddedAt: time.Now()}
	if err := s.store.AnadirPersona(ctx, v.Tenant, v.ID, persona, quien.Sub); err != nil {
		return domain.Persona{}, err
	}
	return persona, nil
}

// Descompartir quita a alguien de la lista.
func (s *Servicio) Descompartir(ctx context.Context, quien Quien, id uuid.UUID, userID string) error {
	v, err := s.editable(ctx, quien, id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("%w: a quién", domain.ErrEntradaInvalida)
	}
	return s.store.QuitarPersona(ctx, v.Tenant, v.ID, userID)
}

// traducirPersonas pasa al dominio lo que Sytue ID contestó al buscar a una
// persona. «No existe» es entrada inválida y no ErrNoExiste: lo que no existe es
// la persona tecleada, no el vídeo, y un 404 aquí se leería como lo segundo.
func traducirPersonas(err error) error {
	switch {
	case errors.Is(err, personas.ErrNoExiste):
		return fmt.Errorf("%w: nadie en Sytue One usa ese @usuario o dirección", domain.ErrEntradaInvalida)
	case errors.Is(err, personas.ErrEntradaInvalida):
		return fmt.Errorf("%w: escribe un @usuario o una dirección de correo", domain.ErrEntradaInvalida)
	case errors.Is(err, personas.ErrSinSesion):
		return domain.ErrProhibido
	}
	// ErrDemasiadas y los fallos de red suben tal cual: la capa HTTP los distingue.
	return err
}

// ── Reproducción ────────────────────────────────────────────────────────────

// Play es lo que hace falta para reproducir un vídeo bajo demanda (§5.1). Lo del
// directo lo añade el paquete de directo sobre esto.
type Play struct {
	Kind   domain.Kind
	HLSURL string
	// WebMURL es el WebM de la calidad por defecto (domain.WebMPorDefecto); los
	// demás están al lado, en webm/<q>p.webm con el mismo token, y WebMCalidades
	// dice cuáles hay.
	WebMURL       string
	WebMCalidades []int
	PosterURL     string
	DownloadURL   string
	DurationMS    *int64
}

// Reproducir resuelve el acceso y compone los enlaces firmados de las derivadas
// que existan. La descarga del original solo va si el dueño la abrió o quien
// pregunta lo edita.
func (s *Servicio) Reproducir(ctx context.Context, quien Quien, id uuid.UUID) (domain.Video, Permisos, Play, error) {
	v, p, err := s.Resolver(ctx, quien, id)
	if err != nil {
		return domain.Video{}, Permisos{}, Play{}, err
	}
	return v, p, s.play(v, p), nil
}

func (s *Servicio) play(v domain.Video, p Permisos) Play {
	out := Play{Kind: v.Kind, DurationMS: v.DurationMS}
	if v.HlsKey != "" {
		out.HLSURL = s.firmante.URL(v.ID, media.RutaHLS)
	}
	out.WebMCalidades = v.CalidadesWebM()
	if q := domain.WebMPorDefecto(out.WebMCalidades); q > 0 {
		out.WebMURL = s.firmante.URL(v.ID, media.RutaWebMDe(q))
	}
	if v.PosterKey != "" {
		out.PosterURL = s.firmante.URL(v.ID, media.RutaPoster)
	}
	if v.OriginalKey != "" && (v.DownloadEnabled || p.Editar) {
		out.DownloadURL = s.firmante.URL(v.ID, media.RutaOriginal)
	}
	return out
}

// PosterURL es el enlace firmado del póster, o "" si no hay.
func (s *Servicio) PosterURL(v domain.Video) string {
	if v.PosterKey == "" {
		return ""
	}
	return s.firmante.URL(v.ID, media.RutaPoster)
}

// ── Ciclo de vida desde dentro ──────────────────────────────────────────────

// Estado es un cambio de ciclo de vida que piden las piezas de dentro (el directo
// al empezar y acabar; el transcodificador va por la cola).
type Estado = store.Estado

// CambiarEstado lo aplica sin pasar por `acceso`: viene de dentro y el vídeo se
// nombra por organización e id.
func (s *Servicio) CambiarEstado(ctx context.Context, tenant string, id uuid.UUID, e Estado) (domain.Video, error) {
	switch e.A {
	case domain.StatusProcessing, domain.StatusReady, domain.StatusFailed, domain.StatusLive, domain.StatusEnded:
	case domain.StatusUploading, domain.StatusScheduled:
		return domain.Video{}, fmt.Errorf("%w: a %q no se pasa, se nace", domain.ErrEntradaInvalida, e.A)
	default:
		return domain.Video{}, fmt.Errorf("%w: estado %q desconocido", domain.ErrEntradaInvalida, e.A)
	}
	return s.store.CambiarEstado(ctx, tenant, id, e)
}

// Encolar pone un trabajo para el vídeo (la grabación de un directo, desde el
// paquete de directo).
func (s *Servicio) Encolar(ctx context.Context, tenant string, id uuid.UUID, kind domain.JobKind, payload []byte) (domain.Job, error) {
	return s.store.Encolar(ctx, tenant, id, kind, payload)
}

// Logger es el registro del servicio, para las piezas que arrancan aparte.
func (s *Servicio) Logger(ctx context.Context) *slog.Logger { return log.From(ctx) }

// maxEnFondo acota las tareas en segundo plano a la vez: un borrado masivo no
// lanza mil purgas contra S3 de golpe. Las que no caben esperan su turno.
//
// En memoria: si el proceso muere, una purga pendiente se pierde, pero la fila
// borrada sigue en la base y el barrido periódico la purga al cumplir su plazo
// de gracia (barrido.go).
const maxEnFondo = 8

// enFondo lanza una tarea en segundo plano, con la cota de maxEnFondo.
func (s *Servicio) enFondo(fn func()) {
	s.enMarcha.Add(1)
	go func() {
		defer s.enMarcha.Done()
		s.fondo <- struct{}{}
		defer func() { <-s.fondo }()
		fn()
	}()
}

// Esperar aguarda a que terminen las tareas en segundo plano (al apagar),
// también las copias desde Drive.
//
// ponytail: una copia de gigas no cabe en el plazo de apagado del pod; si la
// cortan, el vídeo se queda en `uploading` hasta que el barrido periódico lo da
// por fallido (barrido.go) y la persona lo vuelve a subir.
func (s *Servicio) Esperar() {
	s.copiando.Wait()
	s.enMarcha.Wait()
}

func (s *Servicio) cerrojoDe(tenant string) *sync.Mutex {
	v, _ := s.cerrojos.LoadOrStore(tenant, &sync.Mutex{})
	if mu, ok := v.(*sync.Mutex); ok {
		return mu
	}
	return &sync.Mutex{} // inalcanzable: solo se guardan *sync.Mutex
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cast/internal/cuota"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/media"
)

// «Subir a Cast» desde Sytue Drive (decisión del dueño, 2026-09-28).
//
// Es una COPIA explícita, no abrir en el sitio: el vídeo pasa a vivir en Cast con
// su transcodificación y el fichero de Drive se queda donde está. Por eso la
// copia cuenta en la cuota como cualquier subida (el plan es el mismo para los
// dos, §8) y se rechaza igual sin sitio.
//
// La petición contesta en cuanto el vídeo existe; la copia va después, en
// segundo plano y en streaming (Drive → almacén de Cast sin pasar por memoria), y
// al acabar sigue el camino de la subida normal: RecibirOriginal y la cola del
// transcodificador. Todo lo que puede fallar ANTES de crear nada se comprueba
// antes (el fichero existe y se puede leer, es un vídeo, cabe en el tope y en la
// cuota, y el contenido ya está abierto): un error de Drive no deja vídeos vacíos.

// Errores propios de la subida desde Drive.
var (
	// ErrSinDrive: el despliegue no tiene Drive configurado.
	ErrSinDrive = errors.New("cast: sin Sytue Drive")
	// ErrNoEsVideo: el fichero de Drive no es un vídeo (415).
	ErrNoEsVideo = errors.New("cast: el fichero no es un vídeo")
	// ErrDriveNoContesta: Drive no contestó o contestó algo que no se entiende (503).
	ErrDriveNoContesta = errors.New("cast: Sytue Drive no contesta")
)

// Drive es lo que el servicio necesita de Sytue Drive para leer un fichero con el
// token de la persona (lo implementa cuota.Drive).
type Drive interface {
	// Fichero trae los metadatos; cuota.ErrFicheroNoExiste / ErrFicheroProhibido
	// si la persona no lo ve o no lo puede leer.
	Fichero(ctx context.Context, bearer, id string) (cuota.Fichero, error)
	// Abrir abre el contenido en streaming, con su tamaño exacto. `ctx` gobierna
	// toda la lectura.
	Abrir(ctx context.Context, bearer, id string) (io.ReadCloser, int64, error)
}

// DeDrive es lo que se pide para subir a Cast un fichero de Drive. Título vacío =
// el nombre del fichero sin la extensión; visibilidad vacía = org.
type DeDrive struct {
	FileID      string
	Title       string
	Description string
	Visibility  domain.Visibility
	// Qualities es la elección de calidades (vacía = la recomendada).
	Qualities []int
}

// FicheroDeDrive es lo que se enseña antes de subir: el fichero tal como lo
// cuenta Drive y el título que llevará si no se cambia.
type FicheroDeDrive struct {
	ID          string
	Nombre      string
	Bytes       int64
	ContentType string
	Titulo      string
}

// plazoDeCopia acota la copia entera, para que una descarga atascada no ocupe un
// hueco para siempre. 8 GiB en dos horas son ~1,2 MB/s: holgado dentro del clúster.
const plazoDeCopia = 2 * time.Hour

// maxCopiando es cuántas copias desde Drive corren a la vez por réplica. Van
// aparte de maxEnFondo: una copia de gigas dura minutos y no debe dejar en cola
// las purgas y las declaraciones de cuota. Las que no caben esperan su turno con
// el vídeo en `uploading`.
const maxCopiando = 4

// PrevisualizarDeDrive comprueba, sin crear nada, que el fichero se puede subir
// (existe para quien pregunta, es un vídeo, cabe en el tope y en la cuota) y
// devuelve lo que la pantalla enseña antes de confirmar.
func (s *Servicio) PrevisualizarDeDrive(ctx context.Context, quien Quien, fileID string) (FicheroDeDrive, error) {
	if err := s.puedeSubirDeDrive(quien, fileID); err != nil {
		return FicheroDeDrive{}, err
	}
	f, err := s.ficheroDeDrive(ctx, quien, fileID)
	if err != nil {
		return FicheroDeDrive{}, err
	}
	if err := s.admitir(ctx, quien, f.SizeBytes, f.ContentType); err != nil {
		return FicheroDeDrive{}, err
	}
	return FicheroDeDrive{
		ID: f.ID, Nombre: f.Name, Bytes: f.SizeBytes, ContentType: f.ContentType, Titulo: TituloDeFichero(f.Name),
	}, nil
}

// SubirDeDrive crea el vídeo a partir de un fichero de Drive y deja la copia en
// marcha en segundo plano. El vídeo vuelve en `uploading`; pasa a `processing`
// cuando el original está en el almacén, o a `failed` con el motivo si la copia
// no llega entera.
func (s *Servicio) SubirDeDrive(ctx context.Context, quien Quien, a DeDrive) (domain.Video, error) {
	if err := s.puedeSubirDeDrive(quien, a.FileID); err != nil {
		return domain.Video{}, err
	}
	// Lo que depende solo de la petición, antes de preguntar a nadie.
	if a.Visibility != "" && !a.Visibility.Valida() {
		return domain.Video{}, fmt.Errorf("%w: visibilidad %q desconocida", domain.ErrEntradaInvalida, a.Visibility)
	}
	if strings.TrimSpace(a.Title) != "" {
		if _, err := domain.Titulo(a.Title); err != nil {
			return domain.Video{}, err
		}
	}
	if _, err := domain.Descripcion(a.Description); err != nil {
		return domain.Video{}, err
	}
	if len(a.Qualities) > 0 {
		if _, err := domain.ValidarCalidades(a.Qualities, domain.Opciones(0, 0)); err != nil {
			return domain.Video{}, err
		}
	}

	f, err := s.ficheroDeDrive(ctx, quien, a.FileID)
	if err != nil {
		return domain.Video{}, err
	}
	tipo := tipoBase(f.ContentType)
	if err := s.admitir(ctx, quien, f.SizeBytes, tipo); err != nil {
		return domain.Video{}, err
	}

	// El contenido se abre AHORA, con el token de quien llama y dentro de su
	// petición: así un 404/403 del contenido tampoco deja un vídeo vacío, y la
	// copia no depende de que el token siga vivo cuando empiece. El contexto se
	// desliga de la petición (el cuerpo se lee después de contestar) y lleva su
	// propio plazo.
	fondo, cancelar := context.WithTimeout(context.WithoutCancel(ctx), plazoDeCopia)
	cuerpo, tamano, err := s.drive.Abrir(fondo, quien.Token, f.ID)
	if err != nil {
		cancelar()
		return domain.Video{}, traducirDrive(err)
	}
	cerrar := func() {
		_ = cuerpo.Close()
		cancelar()
	}
	// El tamaño que manda es el del contenido: si alguien subió otra versión entre
	// las dos preguntas, se comprueba otra vez el tope (la cuota se comprobó con
	// la que había hace un instante y se declara con la real al terminar).
	if tamano > domain.MaxOriginalBytes {
		cerrar()
		return domain.Video{}, fmt.Errorf("%w: %d bytes; el tope por vídeo es %d", domain.ErrDemasiadoGrande, tamano, domain.MaxOriginalBytes)
	}

	titulo := a.Title
	if strings.TrimSpace(titulo) == "" {
		titulo = TituloDeFichero(f.Name)
	}
	v, err := s.Crear(ctx, quien, Alta{Kind: domain.KindUpload, Title: titulo, Description: a.Description, Visibility: a.Visibility, Qualities: a.Qualities})
	if err != nil {
		cerrar()
		return domain.Video{}, err
	}
	s.copiando.Add(1)
	go func() {
		defer s.copiando.Done()
		defer cerrar()
		s.turnoDeCopia <- struct{}{}
		defer func() { <-s.turnoDeCopia }()
		s.copiarDeDrive(fondo, v, cuerpo, tamano, tipo)
	}()
	return v, nil
}

// EsperarCopias aguarda a que terminen las copias desde Drive en curso. Esperar
// (al apagar) también las espera.
func (s *Servicio) EsperarCopias() { s.copiando.Wait() }

// puedeSubirDeDrive es lo que se exige antes de mirar nada: sesión, almacén, Drive
// y un identificador con forma de identificador (no se manda a Drive cualquier cosa).
func (s *Servicio) puedeSubirDeDrive(quien Quien, fileID string) error {
	if !quien.conSesion() {
		return domain.ErrProhibido
	}
	if s.objetos == nil {
		return ErrSinAlmacen
	}
	if s.drive == nil {
		return ErrSinDrive
	}
	if _, err := uuid.Parse(fileID); err != nil {
		return fmt.Errorf("%w: file_id no es un identificador de Drive", domain.ErrEntradaInvalida)
	}
	return nil
}

func (s *Servicio) ficheroDeDrive(ctx context.Context, quien Quien, fileID string) (cuota.Fichero, error) {
	f, err := s.drive.Fichero(ctx, quien.Token, fileID)
	if err != nil {
		return cuota.Fichero{}, traducirDrive(err)
	}
	if f.ID == "" {
		f.ID = fileID
	}
	return f, nil
}

// traducirDrive pasa al dominio lo que Drive contestó sobre el fichero.
func traducirDrive(err error) error {
	switch {
	case errors.Is(err, cuota.ErrFicheroNoExiste):
		return fmt.Errorf("%w: el fichero de Drive", domain.ErrNoExiste)
	case errors.Is(err, cuota.ErrFicheroProhibido), errors.Is(err, cuota.ErrSinSesion):
		return fmt.Errorf("%w: el fichero de Drive", domain.ErrProhibido)
	case errors.Is(err, cuota.ErrNoDisponible), errors.Is(err, cuota.ErrRechazado):
		return fmt.Errorf("%w: %w", ErrDriveNoContesta, err)
	}
	return err
}

// admitir es lo que un original tiene que cumplir para entrar, venga de donde
// venga: ser un vídeo, pasar de cero y no del tope, y caber en la cuota.
func (s *Servicio) admitir(ctx context.Context, quien Quien, tamano int64, contentType string) error {
	if !strings.HasPrefix(tipoBase(contentType), "video/") {
		return fmt.Errorf("%w (%s)", ErrNoEsVideo, contentType)
	}
	if tamano <= 0 {
		return fmt.Errorf("%w: el fichero está vacío", domain.ErrEntradaInvalida)
	}
	if tamano > domain.MaxOriginalBytes {
		return fmt.Errorf("%w: %d bytes; el tope por vídeo es %d", domain.ErrDemasiadoGrande, tamano, domain.MaxOriginalBytes)
	}
	return s.comprobarCuota(ctx, quien, tamano)
}

// tipoBase es el Content-Type sin parámetros y en minúsculas.
func tipoBase(ct string) string {
	t, _, _ := strings.Cut(ct, ";")
	return strings.ToLower(strings.TrimSpace(t))
}

// TituloDeFichero es el título por defecto de un vídeo que viene de un fichero: el
// nombre sin la extensión, con los espacios normalizados y recortado al tope. Si
// sin la extensión no queda nada («.mp4»), el nombre entero.
func TituloDeFichero(nombre string) string {
	base := strings.TrimSuffix(nombre, path.Ext(nombre))
	t := strings.Join(strings.Fields(base), " ")
	if t == "" {
		t = strings.Join(strings.Fields(nombre), " ")
	}
	if utf8.RuneCountInString(t) > domain.MaxTitulo {
		t = strings.TrimSpace(string([]rune(t)[:domain.MaxTitulo]))
	}
	return t
}

// copiarDeDrive lleva el contenido al almacén y encola la transcodificación, o
// deja el vídeo `failed` con el motivo. Corre en segundo plano.
func (s *Servicio) copiarDeDrive(ctx context.Context, v domain.Video, cuerpo io.Reader, tamano int64, contentType string) {
	clave := Prefijo(v) + media.RutaOriginal
	lector := &exacto{r: cuerpo, faltan: tamano}
	if err := s.objetos.Put(ctx, v.Tenant, clave, lector, tamano, contentType); err != nil {
		motivo := "no se pudo guardar el vídeo en Sytue Cast; bórralo y vuelve a intentarlo"
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			motivo = "la copia desde Sytue Drive tardó demasiado; bórralo y vuelve a intentarlo"
		case lector.err != nil:
			motivo = "la copia desde Sytue Drive se cortó a mitad; bórralo y vuelve a intentarlo"
		}
		s.logger.Warn("cast: la copia desde Drive falló", "video", v.ID, "tenant", v.Tenant, "error", err)
		s.fallarCopia(ctx, v, motivo)
		return
	}
	// Los bytes ya están: registrarlos no depende del plazo de la copia.
	fin := context.WithoutCancel(ctx)
	if _, err := s.store.RecibirOriginal(fin, v.Tenant, v.ID, clave, tamano); err != nil {
		// Lo más probable: lo borraron mientras se copiaba. Lo copiado sobra.
		s.logger.Warn("cast: el vídeo copiado desde Drive ya no lo espera", "video", v.ID, "error", err)
		s.purgar(fin, v)
		return
	}
	s.declararCuota(fin, v.Tenant)
}

// fallarCopia deja el vídeo `failed` con el motivo, para que la interfaz lo diga.
// El contexto de la copia puede haber caducado (es lo que la tumbó): se desliga.
func (s *Servicio) fallarCopia(ctx context.Context, v domain.Video, motivo string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if _, err := s.store.CambiarEstado(ctx, v.Tenant, v.ID, Estado{A: domain.StatusFailed, Failure: &motivo}); err != nil {
		s.logger.Warn("cast: no se pudo marcar como fallida la copia desde Drive", "video", v.ID, "error", err)
	}
}

// exacto lee exactamente `faltan` bytes: si el origen acaba antes, es un corte
// (io.ErrUnexpectedEOF), no un fichero más corto. Guarda el error de LECTURA
// para distinguir «Drive se cortó» de «el almacén de Cast falló».
type exacto struct {
	r      io.Reader
	faltan int64
	err    error
}

func (e *exacto) Read(p []byte) (int, error) {
	if e.faltan <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > e.faltan {
		p = p[:e.faltan]
	}
	n, err := e.r.Read(p)
	e.faltan -= int64(n)
	switch {
	case errors.Is(err, io.EOF) && e.faltan > 0:
		e.err = io.ErrUnexpectedEOF
		return n, e.err
	case err != nil && !errors.Is(err, io.EOF):
		e.err = err
		return n, err
	case e.faltan == 0:
		return n, io.EOF
	}
	return n, nil
}

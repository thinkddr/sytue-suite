// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// Backup es un dump de Sytue Postgres almacenado en Object Storage. Key es SOLO el
// nombre de fichero del backup (sin prefijo): es lo único que el cliente devuelve
// a POST /restore, y el prefijo (tenant/servicio) SIEMPRE se compone en servidor.
type Backup struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
}

// backupNameRe acota el nombre de un backup a un ÚNICO segmento de fichero: sin
// separadores de ruta ('/' '\'), sin '..', terminado en .sql.gz. Es la frontera de
// confianza del restore: el cliente pasa SOLO este nombre (p. ej.
// "20260720T030000Z.sql.gz"), nunca una ruta. Junto con la composición server-side
// del objeto (backups/<tenant>/<shortID>/<name>) cierra el path-traversal y el
// restore cross-tenant: aunque el nombre fuese válido, el prefijo tenant/servicio
// sale del servicio cargado bajo RLS, no del cliente.
var backupNameRe = regexp.MustCompile(`^[A-Za-z0-9._:-]+\.sql\.gz$`)

// validateBackupName aplica la regla de nombre de backup (frontera de confianza).
func validateBackupName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: nombre de backup vacío", domain.ErrInvalidInput)
	}
	// Rechazo explícito de traversal/separadores ANTES del patrón (mensaje claro y
	// defensa redundante: el patrón ya los excluye, pero el rechazo directo no
	// depende de que el regex quede intacto en un refactor).
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("%w: el nombre de backup no puede contener rutas", domain.ErrInvalidInput)
	}
	if !backupNameRe.MatchString(name) {
		return fmt.Errorf("%w: nombre de backup inválido (esperado <nombre>.sql.gz)", domain.ErrInvalidInput)
	}
	return nil
}

// backupPrefix es el prefijo S3 PROPIO del servicio (con barra final), idéntico al
// que escribe el CronJob de backup (ensurePostgresBackup): backups/<tenant>/<shortID>/.
// El cliente NUNCA aporta este prefijo.
func backupPrefix(svc domain.ManagedService) string {
	return fmt.Sprintf("backups/%s/%s/", svc.Tenant, shortUUID(svc.ID))
}

// backupObjectKey compone, SIEMPRE en servidor, el objeto S3 final a partir del
// prefijo propio del servicio + el nombre de backup ya validado. El cliente no
// controla tenant/shortID/prefijo.
func backupObjectKey(svc domain.ManagedService, name string) string {
	return backupPrefix(svc) + name
}

// backupBucketClient construye el cliente S3 para el bucket de backups de la
// plataforma, reutilizando EXACTAMENTE las credenciales del backup: las acotadas
// S3_BACKUP_* si ops las acuñó, si no las de plataforma (ver upsertPlatformS3Secret).
func (s *Service) backupBucketClient() (*minio.Client, string, error) {
	ak, sk := s.cfg.S3BackupAccessKey, s.cfg.S3BackupSecretKey
	if ak == "" || sk == "" {
		ak, sk = s.cfg.S3AccessKey, s.cfg.S3SecretKey
	}
	if s.cfg.S3Bucket == "" || ak == "" {
		return nil, "", domain.ErrNotFound // sin Object Storage de plataforma: no hay backups
	}
	mc, err := minio.New(s.cfg.S3Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(ak, sk, ""),
		Secure: s.cfg.S3UseSSL,
		Region: s.cfg.S3Region,
	})
	if err != nil {
		return nil, "", fmt.Errorf("service: cliente S3 de backups: %w", err)
	}
	return mc, s.cfg.S3Bucket, nil
}

// listBackups lista los objetos S3 bajo EXACTAMENTE el prefijo propio del servicio.
// El prefijo lo compone el servidor a partir del svc (cargado bajo RLS): nunca se
// acepta un prefijo del cliente. Devuelve los nombres de fichero (sin prefijo).
func (s *Service) listBackups(ctx context.Context, svc domain.ManagedService) ([]Backup, error) {
	mc, bucket, err := s.backupBucketClient()
	if err != nil {
		return nil, err
	}
	prefix := backupPrefix(svc)
	var out []Backup
	for obj := range mc.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, MaxKeys: 500}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("service: listar backups: %w", obj.Err)
		}
		name := strings.TrimPrefix(obj.Key, prefix)
		if name == "" || strings.Contains(name, "/") {
			continue // sub-"directorios" o el propio prefijo: no son backups directos
		}
		out = append(out, Backup{Key: name, Size: obj.Size, LastModified: obj.LastModified})
	}
	// Más reciente primero: es lo útil en un selector de restore (el orden nativo de
	// ListObjects es lexicográfico → cronológico ascendente para nombres con fecha).
	sort.Slice(out, func(i, j int) bool { return out[i].LastModified.After(out[j].LastModified) })
	return out, nil
}

// postgresServiceFor carga un servicio bajo RLS (aísla por tenant: un id ajeno da
// ErrNotFound) y exige kind=postgres (el resto no tiene backups pg → ErrConflict).
func (s *Service) postgresServiceFor(ctx context.Context, serviceID uuid.UUID) (domain.ManagedService, error) {
	svc, err := s.store.GetService(ctx, serviceID)
	if err != nil {
		return domain.ManagedService{}, err
	}
	if svc.Kind != domain.ServicePostgres {
		return domain.ManagedService{}, fmt.Errorf("%w: solo los servicios postgres tienen backups", domain.ErrConflict)
	}
	return svc, nil
}

// ListServiceBackups lista los backups de un servicio Sytue Postgres. GetService
// corre bajo RLS (un servicio de otro tenant da ErrNotFound → aislamiento) y el
// prefijo S3 se compone en servidor: el cliente no controla tenant ni servicio.
func (s *Service) ListServiceBackups(ctx context.Context, serviceID uuid.UUID) ([]Backup, error) {
	svc, err := s.postgresServiceFor(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	return s.listBackups(ctx, svc)
}

// serviceLogTailer es la capacidad OPCIONAL de leer el registro de los pods de un
// servicio gestionado. La implementa el streamer de Kapsule; el stub de dev no
// tiene pods que mirar. Se descubre por type-assert, igual que el resto de
// capacidades del runtime.
type serviceLogTailer interface {
	TailPodLogs(ctx context.Context, namespace, selector string, lines int) ([]domain.LogLine, error)
}

// ServiceLogs devuelve las últimas líneas del registro de un servicio gestionado
// (función, tarea programada, trabajo o app del catálogo).
//
// Sin esto, una función que no arranca era un callejón sin salida: el panel la
// enseñaba en «provisioning» para siempre y el motivo solo existía dentro del
// clúster. Los logs de la APP del proyecto ya se veían; los de sus servicios, no.
//
// Lista VACÍA no es un error: un servicio recién creado todavía no ha escrito
// nada, y en dev no hay pods. La pantalla lo dice con esas palabras.
func (s *Service) ServiceLogs(ctx context.Context, serviceID uuid.UUID, lines int) ([]domain.LogLine, error) {
	svc, err := s.store.GetService(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	tailer, ok := s.logStreamer.(serviceLogTailer)
	if !ok || s.logStreamer == nil {
		return []domain.LogLine{}, nil
	}
	// El selector se compone AQUÍ con el id del servicio ya resuelto bajo RLS: el
	// cliente no elige etiquetas, así que no hay forma de pedir los pods de otro.
	selector := "sytue.eu/service-id=" + svc.ID.String()
	return tailer.TailPodLogs(ctx, "tenant-"+svc.Tenant, selector, lines)
}

// BackupServiceNow pide una copia INMEDIATA del Postgres del servicio.
//
// La copia diaria existe desde la Ola C+D, pero antes de tocar algo —una
// migración, un borrado masivo— nadie quiere esperar a las tres de la mañana ni
// conformarse con la de ayer. Esto es lo que faltaba para que «restaurar» sirva
// de verdad: poder crear el punto al que volver.
//
// No espera a que termine: el Job corre en el clúster y la copia aparece en la
// lista cuando esté. Bloquear una petición HTTP durante un `pg_dump` de gigabytes
// sería peor de todas las formas posibles.
func (s *Service) BackupServiceNow(ctx context.Context, serviceID uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	// Resuelve y EXIGE kind=postgres bajo RLS: un id de otro tenant no llega aquí.
	if _, err := s.postgresServiceFor(ctx, serviceID); err != nil {
		return err
	}
	s.audit(ctx, "service.backup", serviceID.String(), nil)
	return s.emit(ctx, EventServiceBackup, t.Slug, map[string]any{"service_id": serviceID})
}

// RunServiceNow ejecuta AHORA una tarea programada o puntual.
//
// Un cron recién escrito no se podía probar: había que esperar a su hora o
// retocar el horario para que cayera pronto y acordarse de devolverlo. Y repetir
// una tarea puntual que ya terminó obligaba a borrarla y volver a crearla,
// perdiendo con ella su historial y su identificador.
//
// El kind se comprueba aquí para poder contestar 409 en el acto en vez de dejar
// que el evento muera en el controlador sin que nadie se entere: el panel ofrece
// el botón solo en cron y job, pero la API es pública.
func (s *Service) RunServiceNow(ctx context.Context, serviceID uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	svc, err := s.store.GetService(ctx, serviceID) // bajo RLS: otro tenant da ErrNotFound
	if err != nil {
		return err
	}
	if svc.Kind != domain.ServiceCron && svc.Kind != domain.ServiceJob {
		return fmt.Errorf("%w: solo se pueden ejecutar tareas programadas y puntuales", domain.ErrInvalidState)
	}
	s.audit(ctx, "service.run", serviceID.String(), nil)
	return s.emit(ctx, EventServiceRun, t.Slug, map[string]any{"service_id": serviceID})
}

// DownloadServiceBackupURL devuelve una URL temporal para descargar un backup.
//
// Se podía restaurar una copia, pero no LLEVÁRSELA. Un dump que solo existe
// dentro de la plataforma que lo hizo no sirve para lo que la gente quiere de una
// copia de seguridad: guardarla en otro sitio, mirarla en local, o marcharse con
// sus datos. Cerrar esa puerta es exactamente el encierro del que Sytue presume
// no hacer.
//
// La URL la firma el servidor sobre el objeto compuesto AQUÍ (prefijo del tenant
// y del servicio): el cliente pasa un nombre de fichero y nada más. Caduca en 15
// minutos —lo que se tarda en pulsar y descargar— porque una URL firmada es una
// credencial: quien la tenga se lleva la base de datos entera sin sesión.
func (s *Service) DownloadServiceBackupURL(ctx context.Context, serviceID uuid.UUID, backupName string) (string, error) {
	svc, err := s.postgresServiceFor(ctx, serviceID)
	if err != nil {
		return "", err
	}
	if err := validateBackupName(backupName); err != nil {
		return "", err
	}
	if err := s.backupExists(ctx, svc, backupName); err != nil {
		return "", err
	}
	mc, bucket, err := s.backupBucketClient()
	if err != nil {
		return "", err
	}
	// Content-Disposition: el navegador guarda el fichero en vez de intentar
	// enseñarlo. Sin esto, un .sql.gz de gigabytes puede acabar abriéndose en una
	// pestaña.
	params := url.Values{}
	params.Set("response-content-disposition", `attachment; filename="`+backupName+`"`)
	u, err := mc.PresignedGetObject(ctx, bucket, backupObjectKey(svc, backupName), 15*time.Minute, params)
	if err != nil {
		return "", fmt.Errorf("service: firmar la descarga del backup: %w", err)
	}
	// Auditado como el restore: una descarga es una copia de la base de datos
	// saliendo de la plataforma, y eso tiene que quedar escrito.
	s.audit(ctx, "service.backup.download", serviceID.String(), map[string]any{"backup": backupName})
	return u.String(), nil
}

// backupExists comprueba que el nombre corresponde a un backup REAL de ese
// servicio. Compartido por restaurar y descargar: los dos necesitan el mismo
// 404 limpio, y tenerlo en un sitio evita que uno de los dos se quede sin él.
func (s *Service) backupExists(ctx context.Context, svc domain.ManagedService, backupName string) error {
	backups, err := s.listBackups(ctx, svc)
	if err != nil {
		return err
	}
	for _, b := range backups {
		if b.Key == backupName {
			return nil
		}
	}
	return fmt.Errorf("%w: backup no encontrado", domain.ErrNotFound)
}

// RestoreServiceBackup restaura un backup elegido sobre el Sytue Postgres del
// servicio (operación DESTRUCTIVA). Frontera de confianza: el cliente pasa SOLO el
// nombre de fichero del backup; se valida contra un patrón estricto (sin rutas), se
// comprueba que existe bajo el prefijo PROPIO del servicio, y el objeto S3 final se
// compone en servidor como backups/<tenant>/<shortID>/<backupName>. Emite un evento
// que el controller materializa como un Job de restore en el namespace del tenant.
func (s *Service) RestoreServiceBackup(ctx context.Context, serviceID uuid.UUID, backupName string) error {
	t := tenancy.MustFrom(ctx)
	svc, err := s.postgresServiceFor(ctx, serviceID)
	if err != nil {
		return err
	}
	// Validación del nombre ANTES de tocar S3: rechaza traversal/rutas sin coste de red.
	if err := validateBackupName(backupName); err != nil {
		return err
	}
	// Defensa en profundidad + UX (404 limpio): el backup debe existir bajo el prefijo
	// propio del servicio. No añade seguridad sobre la composición server-side (que ya
	// ancla tenant+servicio), pero evita lanzar un restore de un objeto inexistente.
	if err := s.backupExists(ctx, svc, backupName); err != nil {
		return err
	}
	s.audit(ctx, "service.restore", serviceID.String(), map[string]any{"backup": backupName})
	return s.emit(ctx, EventServiceRestore, t.Slug, map[string]any{
		"service_id": serviceID, "backup": backupName,
	})
}

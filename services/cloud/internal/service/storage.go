// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// StorageObject es un objeto listado del bucket del proyecto.
type StorageObject struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
}

// StorageInfo resume el bucket y credenciales enmascaradas del servicio storage.
type StorageInfo struct {
	Bucket    string `json:"bucket"`
	Endpoint  string `json:"endpoint"`
	AccessKey string `json:"access_key"`
	Region    string `json:"region"`
}

// ProjectStorage devuelve metadatos del servicio storage ready del proyecto.
func (s *Service) ProjectStorage(ctx context.Context, projectID uuid.UUID) (StorageInfo, error) {
	svcs, err := s.store.ListServices(ctx, projectID)
	if err != nil {
		return StorageInfo{}, err
	}
	for _, svc := range svcs {
		if svc.Kind != domain.ServiceStorage || svc.Status != domain.ServiceReady {
			continue
		}
		bucket := svc.Config["bucket"]
		if bucket == "" {
			bucket = fmt.Sprintf("sytue-app-%s", shortUUID(svc.ID))
		}
		ak := "SCW••••"
		if len(svc.Config["access_key"]) > 4 {
			ak = svc.Config["access_key"][:4] + "••••"
		}
		return StorageInfo{
			Bucket:    bucket,
			Endpoint:  publicS3Endpoint(s.cfg),
			AccessKey: ak,
			Region:    s.cfg.S3Region,
		}, nil
	}
	return StorageInfo{}, domain.ErrNotFound
}

// ListStorageObjects lista objetos del bucket del proyecto (máx 200).
func (s *Service) ListStorageObjects(ctx context.Context, projectID uuid.UUID, prefix string) ([]StorageObject, error) {
	mc, bucket, err := s.storageClient(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var out []StorageObject
	for obj := range mc.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, MaxKeys: 200}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		out = append(out, StorageObject{Key: obj.Key, Size: obj.Size, LastModified: obj.LastModified})
	}
	// Metering: registrar GB aproximados del bucket (best-effort).
	var total int64
	for _, o := range out {
		total += o.Size
	}
	if total > 0 {
		t := tenancy.MustFrom(ctx)
		pid := projectID
		gb := float64(total) / (1024 * 1024 * 1024)
		now := time.Now().UTC()
		_ = s.store.RecordUsage(ctx, store.UsageInput{
			Tenant: t.Slug, ProjectID: &pid, Metric: "storage_gb", Quantity: gb,
			Unit: "gb", PeriodFrom: now.Add(-24 * time.Hour), PeriodTo: now,
		})
	}
	return out, nil
}

// DeleteStorageObject borra un objeto del bucket del proyecto.
//
// El aislamiento lo da `storageClient`: resuelve el bucket a partir de los
// servicios del proyecto bajo RLS, así que la clave que manda el cliente nunca
// elige bucket, solo objeto DENTRO del suyo. Por eso basta con normalizar la
// barra inicial —en S3 la clave es opaca y no hay travesía de directorios que
// escape del prefijo—.
//
// Borrar algo que no existe no es un error: S3 devuelve éxito, y la interfaz que
// reintenta tras un fallo de red debe poder hacerlo sin ver un 404 falso.
func (s *Service) DeleteStorageObject(ctx context.Context, projectID uuid.UUID, key string) error {
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if key == "" {
		return fmt.Errorf("%w: falta la clave del objeto", domain.ErrInvalidInput)
	}
	mc, bucket, err := s.storageClient(ctx, projectID)
	if err != nil {
		return err
	}
	if err := mc.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("service: borrar objeto de storage: %w", err)
	}
	s.audit(ctx, "storage.delete", projectID.String(), map[string]any{"key": key})
	return nil
}

// PresignStorageURL genera URL prefirmada GET o PUT para un objeto.
func (s *Service) PresignStorageURL(ctx context.Context, projectID uuid.UUID, key, method string, expiry time.Duration) (string, error) {
	mc, bucket, err := s.storageClient(ctx, projectID)
	if err != nil {
		return "", err
	}
	if expiry <= 0 {
		expiry = 15 * time.Minute
	}
	key = strings.TrimPrefix(key, "/")
	// Quién se llevó (o subió) qué objeto con una URL que no pide credencial.
	s.audit(ctx, "storage.presign", projectID.String(), map[string]any{"key": key, "method": strings.ToUpper(method)})
	switch strings.ToUpper(method) {
	case "PUT":
		u, err := mc.PresignedPutObject(ctx, bucket, key, expiry)
		if err != nil {
			return "", err
		}
		return u.String(), nil
	default:
		u, err := mc.PresignedGetObject(ctx, bucket, key, expiry, nil)
		if err != nil {
			return "", err
		}
		return u.String(), nil
	}
}

func (s *Service) storageClient(ctx context.Context, projectID uuid.UUID) (*minio.Client, string, error) {
	svcs, err := s.store.ListServices(ctx, projectID)
	if err != nil {
		return nil, "", err
	}
	for _, svc := range svcs {
		if svc.Kind != domain.ServiceStorage || svc.Status != domain.ServiceReady {
			continue
		}
		bucket := svc.Config["bucket"]
		if bucket == "" {
			bucket = fmt.Sprintf("sytue-app-%s", shortUUID(svc.ID))
		}
		mc, err := minio.New(s.cfg.S3Endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(s.cfg.S3AccessKey, s.cfg.S3SecretKey, ""),
			Secure: s.cfg.S3UseSSL,
			Region: s.cfg.S3Region,
		})
		if err != nil {
			return nil, "", fmt.Errorf("storage: cliente minio: %w", err)
		}
		return mc, bucket, nil
	}
	return nil, "", domain.ErrNotFound
}

func shortUUID(id uuid.UUID) string {
	s := strings.ReplaceAll(id.String(), "-", "")
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

func publicS3Endpoint(cfg config.Config) string {
	scheme := "http"
	if cfg.S3UseSSL {
		scheme = "https"
	}
	return scheme + "://" + strings.TrimPrefix(cfg.S3Endpoint, "https://")
}

// deploymentLogStreamer abstrae los logs de un deployment (runtime/kube o stub):
// el tail en vivo para el SSE y la lectura por lotes que alimenta la búsqueda.
type deploymentLogStreamer interface {
	StreamDeploymentLogs(ctx context.Context, dep domain.Deployment, projectSlug string, w io.Writer) error
	CollectDeploymentLogs(ctx context.Context, tenant string, deploymentID uuid.UUID, since time.Time, limit int) ([]domain.LogLine, error)
}

// StreamDeploymentLogs escribe logs SSE del deployment.
func (s *Service) StreamDeploymentLogs(ctx context.Context, projectID, deploymentID uuid.UUID, w io.Writer) error {
	dep, err := s.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return err
	}
	if dep.ProjectID != projectID {
		return domain.ErrNotFound
	}
	proj, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	if s.logStreamer == nil {
		_, _ = io.WriteString(w, "logs no disponibles\n")
		return nil
	}
	return s.logStreamer.StreamDeploymentLogs(ctx, dep, proj.Slug, w)
}

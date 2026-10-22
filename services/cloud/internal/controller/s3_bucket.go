// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"git.sytue.eu/sytue/one/services/cloud/internal/config"
)

// ErrPerTenantCredsUnavailable se devuelve cuando NO hay forma de acuñar
// credenciales S3 ACOTADAS al bucket del tenant porque Scaleway IAM no está
// cableado (faltan SCW_SECRET_KEY/SCW_ORG_ID). Fail-closed: se BLOQUEA la
// provisión de storage antes que entregar a la app del tenant las claves ROOT de
// la plataforma, que darían acceso a TODO el Object Storage (fuga cross-tenant).
// Con IAM cableado no se llega aquí: se acuña una API key de una application IAM
// y se acota a arn:aws:s3:::<bucket>/* con una bucket policy (ver CreateBucket).
var ErrPerTenantCredsUnavailable = errors.New(
	"controller: credenciales S3 por-tenant no disponibles (IAM no cableado); no se entregan claves root",
)

// S3BucketProvisioner crea buckets dedicados en Object Storage (Scaleway) con las
// credenciales de plataforma (control-plane) y acuña, vía Scaleway IAM, una
// credencial ACOTADA a cada bucket que sí se entrega a la app del tenant. Crear el
// bucket y fijar su policy son operaciones legítimas del control-plane con la key
// de plataforma; lo que NUNCA hace es DEVOLVER esa key a la app: si IAM no está
// cableado (iam == nil) la provisión de storage se bloquea (fail-closed).
type S3BucketProvisioner struct {
	cfg config.Config
	// iam acuña la credencial ACOTADA por bucket. nil ⇒ IAM no cableado ⇒
	// fail-closed (no se entregan las claves root de la plataforma).
	iam IAMClient
}

// NewS3BucketProvisioner construye el provisioner de buckets S3. Cablea el cliente
// de Scaleway IAM solo si está configurado (SCW_SECRET_KEY + SCW_ORG_ID); si no,
// iam queda nil y CreateBucket falla en cerrado.
func NewS3BucketProvisioner(cfg config.Config) *S3BucketProvisioner {
	var iam IAMClient
	if cfg.SCWSecretKey != "" && cfg.SCWOrgID != "" {
		iam = newSCWIAMClient(cfg)
	}
	return &S3BucketProvisioner{cfg: cfg, iam: iam}
}

func (p *S3BucketProvisioner) minioClient() (*minio.Client, error) {
	return minio.New(p.cfg.S3Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(p.cfg.S3AccessKey, p.cfg.S3SecretKey, ""),
		Secure: p.cfg.S3UseSSL,
		Region: p.cfg.S3Region,
	})
}

// CreateBucket implementa BucketProvisioner: crea el bucket dedicado, acuña una
// credencial IAM ACOTADA a él (application + API key + bucket policy) y la
// devuelve. Sin IAM cableado falla en cerrado ANTES de crear nada.
func (p *S3BucketProvisioner) CreateBucket(ctx context.Context, name, region string) (accessKey, secretKey string, err error) {
	// Fail-closed ANTES de crear el bucket: sin IAM no hay credencial acotada que
	// entregar, y NUNCA se devuelven las claves root de la plataforma. No se crea
	// el bucket para no dejar huérfanos.
	if p.iam == nil {
		return "", "", ErrPerTenantCredsUnavailable
	}
	mc, err := p.minioClient()
	if err != nil {
		return "", "", fmt.Errorf("controller: cliente S3: %w", err)
	}
	if region == "" {
		region = p.cfg.S3Region
	}
	exists, err := mc.BucketExists(ctx, name)
	if err != nil {
		return "", "", fmt.Errorf("controller: comprobar bucket %q: %w", name, err)
	}
	if !exists {
		if err := mc.MakeBucket(ctx, name, minio.MakeBucketOptions{Region: region}); err != nil {
			return "", "", fmt.Errorf("controller: crear bucket %q: %w", name, err)
		}
	}
	// Acuña la credencial IAM (application + API key) y ACÓTALA a este bucket con
	// una bucket policy: la key solo puede tocar <bucket> y <bucket>/*.
	ak, sk, appID, err := p.mintScopedCreds(ctx, name)
	if err != nil {
		return "", "", err
	}
	policy, err := bucketPolicyJSON(name, appID)
	if err != nil {
		return "", "", err
	}
	if err := mc.SetBucketPolicy(ctx, name, policy); err != nil {
		return "", "", fmt.Errorf("controller: aplicar bucket policy %q: %w", name, err)
	}
	return ak, sk, nil
}

// mintScopedCreds asegura la application IAM del bucket (nombre = bucket) y rota su
// API key, devolviendo la credencial acuñada y el id de la application (para la
// bucket policy). Separado de CreateBucket para testear el acuñado con un IAMClient
// fake, sin S3 vivo.
func (p *S3BucketProvisioner) mintScopedCreds(ctx context.Context, bucket string) (accessKey, secretKey, appID string, err error) {
	appID, err = p.iam.EnsureApplication(ctx, bucket)
	if err != nil {
		return "", "", "", fmt.Errorf("controller: IAM application: %w", err)
	}
	ak, sk, err := p.iam.RotateAPIKey(ctx, appID)
	if err != nil {
		return "", "", "", fmt.Errorf("controller: IAM api-key: %w", err)
	}
	return ak, sk, appID, nil
}

// DeleteBucket implementa BucketProvisioner: vacía y elimina el bucket y borra la
// credencial IAM ACOTADA (application + API keys) asociada.
func (p *S3BucketProvisioner) DeleteBucket(ctx context.Context, name, _ string) error {
	mc, err := p.minioClient()
	if err != nil {
		return fmt.Errorf("controller: cliente S3: %w", err)
	}
	// Vacía prefijos conocidos (best-effort) antes de borrar el bucket.
	objects := mc.ListObjects(ctx, name, minio.ListObjectsOptions{Recursive: true})
	for obj := range objects {
		if obj.Err != nil {
			break
		}
		_ = mc.RemoveObject(ctx, name, obj.Key, minio.RemoveObjectOptions{})
	}
	if err := mc.RemoveBucket(ctx, name); err != nil {
		return fmt.Errorf("controller: eliminar bucket %q: %w", name, err)
	}
	// Borra la credencial IAM acotada del bucket (application == nombre del bucket).
	if p.iam != nil {
		if err := p.iam.DeleteApplication(ctx, name); err != nil {
			return fmt.Errorf("controller: eliminar IAM application %q: %w", name, err)
		}
	}
	return nil
}

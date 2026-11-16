// SPDX-License-Identifier: Apache-2.0
//go:build integration

// Package cipherfixture provisions the shared disposable Drive/S3 test fixture.
package cipherfixture

import (
	"context"
	"crypto/rand"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
)

type Fixture struct {
	S3      *minio.Client
	Bucket  string
	PG      *pgtest.Harness
	Service *service.Service
}

func New(t *testing.T) *Fixture {
	t.Helper()
	ctx := t.Context()
	endpoint := os.Getenv("SYTUE_TEST_S3_ENDPOINT")
	if endpoint == "" {
		if os.Getenv("SYTUE_TEST_S3_REQUIRED") == "1" {
			t.Fatal("SYTUE_TEST_S3_ENDPOINT is required for this QA run")
		}
		t.Skip("requires an isolated S3-compatible service: set SYTUE_TEST_S3_ENDPOINT and SYTUE_TEST_S3_EPHEMERAL=1")
	}
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil || (host != "127.0.0.1" && host != "::1" && host != "localhost" && host != "minio") || os.Getenv("SYTUE_TEST_S3_EPHEMERAL") != "1" {
		t.Fatal("object store must be an explicitly disposable local or minio service")
	}
	bucket := "sytue-private-" + strings.ToLower(rand.Text())
	cfg := objectstore.Config{Endpoint: endpoint, Region: "fr-par", AccessKey: "sytue-test-user", SecretKey: "sytue-test-password", Bucket: bucket}
	raw, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""), Region: cfg.Region})
	if err != nil {
		t.Fatal(err)
	}
	if err = raw.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
		t.Fatal("create isolated test bucket:", err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for object := range raw.ListObjects(cleanup, bucket, minio.ListObjectsOptions{Recursive: true}) {
			if object.Err != nil {
				t.Errorf("list test objects for cleanup: %v", object.Err)
				return
			}
			if err := raw.RemoveObject(cleanup, bucket, object.Key, minio.RemoveObjectOptions{}); err != nil {
				t.Errorf("remove test object: %v", err)
			}
		}
		if err := raw.RemoveBucket(cleanup, bucket); err != nil {
			t.Errorf("remove test bucket: %v", err)
		}
	})
	objects, err := objectstore.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	pg := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := service.NewStore(pg.Pool)
	svc := service.New(config.Config{DefaultQuotaBytes: 1 << 20, MaxUploadBytes: 1 << 20}, st, objects, nil, nil)
	return &Fixture{S3: raw, Bucket: bucket, PG: pg, Service: svc}
}

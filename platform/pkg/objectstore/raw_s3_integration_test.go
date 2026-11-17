// SPDX-License-Identifier: Apache-2.0
//go:build integration

package objectstore

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// This fixture must point to an explicitly disposable local S3 service.
func TestRawS3RealEmptyMissingAndGzipMetadata(t *testing.T) {
	endpoint := os.Getenv("SYTUE_TEST_S3_ENDPOINT")
	if endpoint == "" {
		if os.Getenv("SYTUE_TEST_S3_REQUIRED") == "1" {
			t.Fatal("isolated S3 endpoint is required")
		}
		t.Skip("requires isolated S3 fixture")
	}
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil || (host != "127.0.0.1" && host != "::1" && host != "localhost" && host != "minio") || os.Getenv("SYTUE_TEST_S3_EPHEMERAL") != "1" {
		t.Fatal("refusing non-disposable S3 endpoint")
	}
	ctx := t.Context()
	bucket := "sytue-raw-" + strings.ToLower(rand.Text())
	cfg := Config{Endpoint: endpoint, Region: "fr-par", AccessKey: "sytue-test-user", SecretKey: "sytue-test-password", Bucket: bucket}
	provider, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""), Region: cfg.Region})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for obj := range provider.ListObjects(cleanup, bucket, minio.ListObjectsOptions{Recursive: true}) {
			if obj.Err != nil {
				t.Error(obj.Err)
				return
			}
			if err := provider.RemoveObject(cleanup, bucket, obj.Key, minio.RemoveObjectOptions{}); err != nil {
				t.Error(err)
			}
		}
		if err := provider.RemoveBucket(cleanup, bucket); err != nil {
			t.Error(err)
		}
	})
	client, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	tenant := tenancy.Tenant{Slug: "acme"}
	var compressed bytes.Buffer
	encoder := gzip.NewWriter(&compressed)
	if _, err := encoder.Write([]byte("opaque")); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	objects := map[string][]byte{"empty": {}, "gzip": compressed.Bytes(), "encoded": compressed.Bytes()}
	for key, data := range objects {
		opts := minio.PutObjectOptions{}
		if key == "gzip" {
			opts.UserMetadata = map[string]string{"sytue-codificacion": "gzip"}
		}
		if key == "encoded" {
			opts.ContentEncoding = "gzip"
		}
		if _, err := provider.PutObject(ctx, bucket, tenant.StoragePrefix()+key, bytes.NewReader(data), int64(len(data)), opts); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{"empty", "gzip", "encoded", "missing"} {
		body, err := client.GetRaw(ctx, tenant, key)
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(body)
		closeErr := body.Close()
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		if key == "missing" {
			if !errors.Is(readErr, ErrNotFound) {
				t.Fatal("missing object accepted", readErr)
			}
		} else if key == "encoded" {
			if !errors.Is(readErr, ErrRawUnsupported) || len(data) != 0 {
				t.Fatal("encoded response accepted", readErr)
			}
		} else if readErr != nil || !bytes.Equal(data, objects[key]) {
			t.Fatal("raw bytes changed", key, readErr)
		}
	}
	decoded, err := client.Get(ctx, tenant, "gzip")
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(decoded)
	closeErr := decoded.Close()
	if readErr != nil || closeErr != nil || string(data) != "opaque" {
		t.Fatal("legacy decoding changed", readErr, closeErr)
	}
}

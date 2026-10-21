// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeIAM es un IAMClient de test (sin Scaleway vivo): registra las llamadas y
// devuelve una credencial fija.
type fakeIAM struct {
	ensuredName string
	rotatedApp  string
	deletedName string
	appID       string
	ak, sk      string
	ensureErr   error
	rotateErr   error
}

func (f *fakeIAM) EnsureApplication(_ context.Context, name string) (string, error) {
	f.ensuredName = name
	if f.ensureErr != nil {
		return "", f.ensureErr
	}
	return f.appID, nil
}

func (f *fakeIAM) RotateAPIKey(_ context.Context, appID string) (string, string, error) {
	f.rotatedApp = appID
	if f.rotateErr != nil {
		return "", "", f.rotateErr
	}
	return f.ak, f.sk, nil
}

func (f *fakeIAM) DeleteApplication(_ context.Context, name string) error {
	f.deletedName = name
	return nil
}

// TestCreateBucketFailClosedWithoutIAM: sin Scaleway IAM configurado, la provisión
// de storage se BLOQUEA (no se entregan claves root) y no se toca S3 siquiera.
func TestCreateBucketFailClosedWithoutIAM(t *testing.T) {
	cfg := testCfg() // sin SCW_SECRET_KEY/SCW_ORG_ID
	p := NewS3BucketProvisioner(cfg)
	if p.iam != nil {
		t.Fatal("iam debería ser nil sin SCW configurado")
	}
	_, _, err := p.CreateBucket(context.Background(), "sytue-app-x", "fr-par")
	if !errors.Is(err, ErrPerTenantCredsUnavailable) {
		t.Fatalf("err = %v, quiero ErrPerTenantCredsUnavailable", err)
	}
}

// TestNewS3BucketProvisionerWiresIAM: con SCW_SECRET_KEY + SCW_ORG_ID se cablea el
// cliente IAM (deja de ser fail-closed).
func TestNewS3BucketProvisionerWiresIAM(t *testing.T) {
	cfg := testCfg()
	cfg.SCWSecretKey = "scw-secret"
	cfg.SCWOrgID = "org-1"
	cfg.SCWIAMURL = "https://api.scaleway.com/iam/v1alpha1"
	if p := NewS3BucketProvisioner(cfg); p.iam == nil {
		t.Fatal("iam debería estar cableado con SCW configurado")
	}
}

// TestMintScopedCredsUsesIAM: el acuñado usa la application del bucket, rota su key
// y produce una policy ACOTADA a ese bucket (con el appID acuñado).
func TestMintScopedCredsUsesIAM(t *testing.T) {
	f := &fakeIAM{appID: "app-42", ak: "SCWACCESS", sk: "SCWSECRET"}
	p := &S3BucketProvisioner{cfg: testCfg(), iam: f}
	const bucket = "sytue-app-deadbeef"

	ak, sk, appID, err := p.mintScopedCreds(context.Background(), bucket)
	if err != nil {
		t.Fatalf("mintScopedCreds: %v", err)
	}
	if ak != "SCWACCESS" || sk != "SCWSECRET" || appID != "app-42" {
		t.Fatalf("creds acuñadas = %q/%q/%q", ak, sk, appID)
	}
	// La application se busca por el nombre del bucket (derivable en deprovision).
	if f.ensuredName != bucket {
		t.Errorf("EnsureApplication(%q), quiero %q", f.ensuredName, bucket)
	}
	if f.rotatedApp != "app-42" {
		t.Errorf("RotateAPIKey(%q), quiero app-42", f.rotatedApp)
	}
	// La policy resultante queda acotada a ESTE bucket con el appID acuñado.
	pol, err := bucketPolicyJSON(bucket, appID)
	if err != nil {
		t.Fatalf("bucketPolicyJSON: %v", err)
	}
	if !strings.Contains(pol, bucket+"/*") || !strings.Contains(pol, "application_id:app-42") {
		t.Errorf("policy no acotada al bucket/app: %s", pol)
	}
}

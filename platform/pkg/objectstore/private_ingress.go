// SPDX-License-Identifier: Apache-2.0
package objectstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

type PrivateIngressObjects interface {
	Put(context.Context, tenancy.Tenant, string, io.Reader, int64, PutOptions) error
	Get(context.Context, tenancy.Tenant, string) (io.ReadCloser, error)
}

func PrivateIngressBlobKey(scope storage.Scope, id string) (string, error) {
	if !scope.Valid() || !storage.Identifier(id) {
		return "", ingress.ErrContract
	}
	return "private-ingress/" + scope.Product + "/" + id + ".age", nil
}

// SealPrivateIngress persists only maintained age ciphertext. The temporary
// file receives age output from its first byte; plaintext is never spooled.
// Success includes readback of the complete object and its ciphertext digest,
// before the domain transaction may announce this task to a device.
func SealPrivateIngress(ctx context.Context, objects PrivateIngressObjects, plan ingress.KeyPlan, taskID, kind string, metadata []byte, source io.Reader, size int64) (ingress.Task, string, error) {
	var task ingress.Task
	if closer, ok := source.(io.ReadCloser); ok {
		stop := context.AfterFunc(ctx, func() { _ = closer.Close() })
		defer stop()
	}
	if objects == nil || agecipher.ValidateKeyPlan(plan) != nil {
		return task, "", ingress.ErrContract
	}
	tenant, err := tenancy.From(ctx)
	if err != nil || tenant.Slug != plan.Scope.Tenant {
		return task, "", ingress.ErrContract
	}
	blobID := uuid.NewString()
	key, err := PrivateIngressBlobKey(plan.Scope, blobID)
	if err != nil {
		return task, "", err
	}
	cipher, err := os.CreateTemp("", "sytue-ingress-cipher-*.age")
	if err != nil {
		return task, "", err
	}
	defer func() { _ = cipher.Close(); _ = os.Remove(cipher.Name()) }()
	task, err = agecipher.Seal(ctx, cipher, plan, taskID, kind, metadata, source, size)
	if err != nil {
		return ingress.Task{}, "", err
	}
	if err := cipher.Sync(); err != nil {
		return ingress.Task{}, "", err
	}
	if _, err := cipher.Seek(0, io.SeekStart); err != nil {
		return ingress.Task{}, "", err
	}
	if err := objects.Put(ctx, tenant, key, cipher, task.Cipher.Size, PutOptions{ContentType: "application/octet-stream"}); err != nil {
		return ingress.Task{}, "", err
	}
	if err := verifyPrivateIngressObject(ctx, objects, tenant, key, task.Cipher); err != nil {
		return ingress.Task{}, "", err
	}
	return task, blobID, nil
}

func verifyPrivateIngressObject(ctx context.Context, objects PrivateIngressObjects, tenant tenancy.Tenant, key string, expected storage.StreamSourceRow) error {
	source, err := objects.Get(ctx, tenant, key)
	if source != nil {
		defer func() { _ = source.Close() }()
	}
	if err != nil {
		return err
	}
	if source == nil {
		return ingress.ErrContract
	}
	stop := context.AfterFunc(ctx, func() { _ = source.Close() })
	defer stop()
	digest := sha256.New()
	count, err := io.Copy(digest, io.LimitReader(source, expected.Size+1))
	if err != nil {
		return errors.Join(err, ctx.Err())
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if count != expected.Size || hex.EncodeToString(digest.Sum(nil)) != expected.SHA256 {
		return ingress.ErrContract
	}
	return nil
}

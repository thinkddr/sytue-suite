// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// SealPrivateIngressInline uses the SAME age/spool/readback implementation as
// object storage, with a ciphertext-only PG adapter. The declared domain bound
// is checked before any read or encryption; no ICS or plaintext digest enters PG.
// The task plan must have committed in a preceding transaction. This transaction
// atomically commits ciphertext, descriptor and the opaque outbox notification.
func SealPrivateIngressInline(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, plan ingress.KeyPlan, id, kind string, metadata []byte, source io.Reader, size, maxRawBytes int64) (ingress.Task, string, error) {
	if source == nil || size < 0 || maxRawBytes < 1 || maxRawBytes > storage.MaxFileBytes || size > maxRawBytes || plan.Validate(cfg.Scope, cfg.Epoch.String()) != nil {
		return ingress.Task{}, "", ingress.ErrContract
	}
	if err := BindPrivateIngressScope(ctx, tx, cfg); err != nil {
		return ingress.Task{}, "", err
	}
	adapter := inlineIngressObjects{tx: tx, cfg: cfg, taskID: id, maxCipherBytes: maxRawBytes*2 + (128 << 10)}
	task, blob, err := objectstore.SealPrivateIngress(ctx, adapter, plan, id, kind, metadata, source, size)
	if err != nil {
		return ingress.Task{}, "", err
	}
	if err := CommitPrivateIngressBody(ctx, tx, cfg, task, blob); err != nil {
		return ingress.Task{}, "", err
	}
	return task, blob, nil
}

type inlineIngressObjects struct {
	tx             pgx.Tx
	cfg            PrivateIngressConfig
	taskID         string
	maxCipherBytes int64
}

func (o inlineIngressObjects) blobID(key string) (string, error) {
	prefix := "private-ingress/" + o.cfg.Scope.Product + "/"
	if !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, ".age") {
		return "", ingress.ErrContract
	}
	id := strings.TrimSuffix(strings.TrimPrefix(key, prefix), ".age")
	expected, err := objectstore.PrivateIngressBlobKey(o.cfg.Scope, id)
	if err != nil || expected != key {
		return "", ingress.ErrContract
	}
	return id, nil
}

func (o inlineIngressObjects) Put(ctx context.Context, tenant tenancy.Tenant, key string, source io.Reader, size int64, options objectstore.PutOptions) error {
	if tenant.Slug != o.cfg.Scope.Tenant || size < 1 || size > o.maxCipherBytes || options.Comprimir || options.ContentType != "application/octet-stream" {
		return ingress.ErrContract
	}
	id, err := o.blobID(key)
	if err != nil {
		return err
	}
	raw := make([]byte, int(size))
	defer clear(raw)
	if _, err := io.ReadFull(source, raw); err != nil {
		return err
	}
	var extra [1]byte
	if n, err := source.Read(extra[:]); n != 0 || err != io.EOF {
		return ingress.ErrContract
	}
	sum := sha256.Sum256(raw)
	_, err = o.tx.Exec(ctx, `INSERT INTO private_ingress_bodies(tenant,owner_sub,product,epoch,id,task_id,ciphertext,cipher_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, o.cfg.Scope.Tenant, o.cfg.Scope.Subject, o.cfg.Scope.Product, o.cfg.Epoch, id, o.taskID, raw, hex.EncodeToString(sum[:]))
	return err
}

func (o inlineIngressObjects) Get(ctx context.Context, tenant tenancy.Tenant, key string) (io.ReadCloser, error) {
	if tenant.Slug != o.cfg.Scope.Tenant {
		return nil, ingress.ErrContract
	}
	id, err := o.blobID(key)
	if err != nil {
		return nil, err
	}
	return ReadPrivateIngressInlineBody(ctx, o.tx, o.cfg, id)
}

func ReadPrivateIngressInlineBody(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, id string) (io.ReadCloser, error) {
	if !storage.Identifier(id) {
		return nil, ingress.ErrContract
	}
	if err := BindPrivateIngressScope(ctx, tx, cfg); err != nil {
		return nil, err
	}
	var raw []byte
	var taskID, digest string
	if err := tx.QueryRow(ctx, `SELECT task_id,ciphertext,cipher_sha256 FROM private_ingress_bodies WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND id=$5`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, id).Scan(&taskID, &raw, &digest); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	if !storage.Identifier(taskID) || hex.EncodeToString(sum[:]) != digest {
		clear(raw)
		return nil, ingress.ErrContract
	}
	return io.NopCloser(bytes.NewReader(raw)), nil
}

// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
)

func TestPrivateIngressRemoteFixture(t *testing.T) {
	if os.Getenv("SYTUE_INGRESS_FIXTURE") != "1" {
		t.Skip("requires disposable PostgreSQL HTTP fixture")
	}
	endpoint, err := calendarSourceFixtureEndpoint(os.Getenv("SYTUE_INGRESS_FIXTURE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	var vector struct{ Proof, ACK ingress.ControlProof }
	if json.Unmarshal([]byte(os.Getenv("SYTUE_INGRESS_FIXTURE_VECTOR")), &vector) != nil || vector.Proof.Claim == nil || vector.Proof.Validate(vector.Proof.Request.Scope, true) != nil || vector.ACK.Validate(vector.Proof.Request.Scope, false) != nil {
		t.Fatal("invalid disposable public control transcript")
	}
	scope := vector.Proof.Request.Scope
	claim := *vector.Proof.Claim
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	client, err := NewPrivateIngress(endpoint.String(), scope.Product, calendarRemoteFixtureSession("synthetic-queue-session"), &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := client.Enrollment(ctx, scope, claim.Task.Epoch)
	if err != nil || enrollment.Status != "active" || enrollment.Plan == nil || enrollment.Plan.KeyID != claim.Task.KeyID || enrollment.Generation != claim.Task.KeyGeneration {
		t.Fatal("native client could not recover the retained enrollment", err)
	}
	page, err := client.Pending(ctx, scope, claim.Task.Epoch, "", 10)
	if err != nil || len(page.Tasks) != 1 || page.Tasks[0] != claim.Task {
		t.Fatal("native pending ciphertext descriptor changed", err)
	}
	if _, err := client.Revalidate(ctx, vector.Proof); err != nil {
		t.Fatal("native publication policy failed", err)
	}
	row, body, err := client.Open(ctx, vector.Proof)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.New()
	n, err := io.Copy(digest, io.LimitReader(body, row.Size+1))
	_ = body.Close()
	if err != nil || row != claim.Task.Cipher || n != row.Size || hex.EncodeToString(digest.Sum(nil)) != row.SHA256 {
		t.Fatal("native ciphertext stream changed", err)
	}
	ack, err := client.Apply(ctx, vector.ACK)
	if err != nil || ack.Receipt == nil || *ack.Receipt != vector.ACK.Request.Acknowledgment.Receipt {
		t.Fatal("native signed ACK did not commit", err)
	}
	t.Log("INGRESS_PG_HTTP_NATIVE_PASS")
}

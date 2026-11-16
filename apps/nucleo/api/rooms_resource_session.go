// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"net/url"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// MatrixResourceGrant is a short-lived one-use product consent result. Grant and
// login token are credentials: keep them in process, never callbacks or storage.
type MatrixResourceGrant struct {
	LoginToken string `json:"login_token,omitempty"`
	Product    string `json:"product"`
	Namespace  string `json:"namespace"`
	Tenant     string `json:"tenant"`
	ConsentID  string `json:"consent_id"`
	Grant      string `json:"grant"`
}

func (g MatrixResourceGrant) valid(product, tenant string) bool {
	parsed, err := uuid.Parse(g.ConsentID)
	return storage.Product(product) && g.Product == product && g.Namespace == "resources:"+product && g.Tenant == tenant && tenant != "" && err == nil && parsed != uuid.Nil && parsed.String() == g.ConsentID && len(g.Grant) >= 32 && len(g.Grant) <= 4096
}

func (r *Rooms) MatrixResourceNativeFinish(ctx context.Context, a MatrixNativeAuthorization, callback MatrixNativeCallback) (MatrixResourceGrant, error) {
	if a.purpose != "resources:"+a.Product() || a.NativeConsumer() != "" {
		return MatrixResourceGrant{}, ErrMatrixSSO
	}
	input, err := matrixNativeFinishInput(a, callback)
	if err != nil {
		return MatrixResourceGrant{}, err
	}
	out, err := PostJSON[MatrixResourceGrant](ctx, r.c, "/matrix/resources/native/finish", input)
	if err != nil {
		return MatrixResourceGrant{}, err
	}
	if !out.valid(a.Product(), a.Tenant()) || out.LoginToken == "" || len(out.LoginToken) > 8192 {
		return MatrixResourceGrant{}, ErrMatrixSSO
	}
	return out, nil
}

func (r *Rooms) MatrixResourceBootstrap(ctx context.Context, product string) (RoomsMatrixBootstrap, error) {
	if !storage.Product(product) {
		return RoomsMatrixBootstrap{}, ErrMatrixSSO
	}
	return JSON[RoomsMatrixBootstrap](ctx, r.c, Request{Path: "/matrix/resources/" + product + "/bootstrap"})
}

func (r *Rooms) MatrixResourceWebConsent(ctx context.Context, product string) (MatrixResourceGrant, error) {
	if !storage.Product(product) {
		return MatrixResourceGrant{}, ErrMatrixSSO
	}
	out, err := PostJSON[MatrixResourceGrant](ctx, r.c, "/matrix/resources/"+product+"/consent", struct{}{})
	if err != nil {
		return MatrixResourceGrant{}, err
	}
	if !out.valid(product, out.Tenant) || out.LoginToken != "" {
		return MatrixResourceGrant{}, ErrMatrixSSO
	}
	return out, nil
}

func (r *Rooms) MatrixResourceSessionChallenge(ctx context.Context, product, grant string, recipient resource.SignedRecipient) (resource.SessionChallenge, error) {
	if !storage.Product(product) || recipient.Scope.Product != product {
		return resource.SessionChallenge{}, ErrMatrixSSO
	}
	return PostJSON[resource.SessionChallenge](ctx, r.c, "/matrix/resources/"+product+"/session/challenge", struct {
		Grant     string                   `json:"grant"`
		Recipient resource.SignedRecipient `json:"recipient"`
	}{grant, recipient})
}

func (r *Rooms) ConfirmMatrixResourceSession(ctx context.Context, product string, proof resource.SignedSessionEnrollment) (resource.SessionRecord, error) {
	if !storage.Product(product) || proof.Binding.Scope.Product != product {
		return resource.SessionRecord{}, ErrMatrixSSO
	}
	return PostJSON[resource.SessionRecord](ctx, r.c, "/matrix/resources/"+product+"/session/confirm", proof)
}

func (r *Rooms) MatrixResourceSession(ctx context.Context, product, device string) (resource.SessionRecord, error) {
	if !storage.Product(product) || device == "" {
		return resource.SessionRecord{}, ErrMatrixSSO
	}
	return JSON[resource.SessionRecord](ctx, r.c, Request{Path: "/matrix/resources/" + product + "/session", Query: url.Values{"device": {device}}})
}

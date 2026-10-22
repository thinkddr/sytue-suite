// SPDX-License-Identifier: Apache-2.0
package callaccess

import (
	"context"
	"net/http"
	"net/url"
)

// IdentityProof contains current ID authorization metadata, never a JWT group snapshot.
type IdentityProof struct {
	Subject           string   `json:"subject"`
	Tenant            string   `json:"tenant"`
	OrganizationAdmin bool     `json:"organization_admin"`
	Active            bool     `json:"active"`
	Member            bool     `json:"member"`
	Version           int64    `json:"version"`
	Groups            []string `json:"groups"`
	Roles             []string `json:"roles"`
}

// IdentityReader obtains an actor's current memberships before authorizing a call.
type IdentityReader interface {
	ReadCallIdentity(context.Context, string, string) (IdentityProof, error)
}

// SubjectRevoker revokes document calls affected by an identity change.
type SubjectRevoker interface {
	RevokeSubjectCalls(context.Context, string, string, int64) error
}

// SubjectFenceReader supports recovery after a safely blocked rolled-back ID write.
type SubjectFenceReader interface {
	MinimumSubjectVersion(context.Context, string, string) (int64, error)
}

// RevokeSubjectCalls advances the actor fence before ID commits a role or group change.
func (c *Client) RevokeSubjectCalls(ctx context.Context, subject, tenant string, version int64) error {
	return c.request(ctx, http.MethodPost, "/internal/rooms/subject-access", struct {
		Subject string `json:"subject"`
		Tenant  string `json:"tenant"`
		Version int64  `json:"version"`
	}{subject, tenant, version}, nil)
}

// MinimumSubjectVersion reads the current document-call identity fence.
func (c *Client) MinimumSubjectVersion(ctx context.Context, subject, tenant string) (int64, error) {
	var out struct {
		Version int64 `json:"version"`
	}
	if err := c.request(ctx, http.MethodGet, "/internal/rooms/subject-access/"+url.PathEscape(subject)+"/"+url.PathEscape(tenant), nil, &out); err != nil {
		return 0, err
	}
	if out.Version < 0 {
		return 0, ErrUnavailable
	}
	return out.Version, nil
}

// ReadCallIdentity reads fresh, actor-bound authorization metadata from ID.
func (c *Client) ReadCallIdentity(ctx context.Context, subject, tenant string) (IdentityProof, error) {
	var out IdentityProof
	err := c.request(ctx, http.MethodPost, "/internal/people/call-access", struct {
		Subject string `json:"subject"`
		Tenant  string `json:"tenant"`
	}{subject, tenant}, &out)
	if err != nil {
		return out, err
	}
	if out.Subject != subject || out.Tenant != tenant || out.Version < 0 {
		return IdentityProof{}, ErrUnavailable
	}
	return out, nil
}

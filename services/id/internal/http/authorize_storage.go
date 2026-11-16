// SPDX-License-Identifier: Apache-2.0
package http

import (
	"net/http"
	"net/url"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Select the private grant's context for this code only. A different browser
// account must identify itself; the hint never substitutes for authentication.
func (s *Server) storageAuthorizationContext(w http.ResponseWriter, r *http.Request, req authzRequest, sess domain.Session, clientID string, redirectErr func(string, string)) (domain.Session, bool) {
	if req.StorageTenant == "" {
		return sess, true
	}
	if sess.UserID != req.StorageSubject {
		loginURL := "/login/?" + url.Values{"return_to": {r.URL.RequestURI()}}.Encode()
		http.Redirect(w, r, loginURL, http.StatusFound)
		return domain.Session{}, false
	}
	selected, err := s.storageAuthorizationSession(r, req, sess)
	if err != nil {
		redirectErr("access_denied", "storage authorization is not available for this account")
		return domain.Session{}, false
	}
	if !s.storageAuthorizationAccess(w, r, sess, selected, clientID, redirectErr) {
		return domain.Session{}, false
	}
	return selected, true
}

// These hints are accepted only by the provisioned, confidential Matrix client.
// They select neither a user nor an arbitrary client/redirect. Synapse forwards
// them for private providers and pins the resulting sub/tenant in its mapping.
func validStorageAuthorizationHint(req authzRequest, client domain.Client) bool {
	if req.StorageHintInvalid {
		return false
	}
	if req.StorageTenant == "" && req.StorageSubject == "" {
		return true
	}
	subject, err := uuid.Parse(req.StorageSubject)
	return err == nil && subject.String() == req.StorageSubject && tenancy.ValidSlug(req.StorageTenant) &&
		client.ID == "sytue-matrix" && client.OrgID == "" && !client.IsPublic() && client.RequirePKCE &&
		client.TokenEndpointAuthMethod == "client_secret_basic"
}

func (s *Server) storageAuthorizationSession(r *http.Request, req authzRequest, sess domain.Session) (domain.Session, error) {
	org, err := s.store.GetOrgBySlug(r.Context(), req.StorageTenant)
	if err != nil {
		return domain.Session{}, err
	}
	if err := s.validateUserOrg(r.Context(), sess.UserID, org.ID); err != nil {
		return domain.Session{}, err
	}
	sess.OrgID = org.ID
	return sess, nil
}

// A session opened in another organization has not satisfied this destination's
// conditional access. MFA elevates the original session before retrying this
// authorization, so the destination does not become the global active org.
func (s *Server) storageAuthorizationAccess(w http.ResponseWriter, r *http.Request, original, selected domain.Session, clientID string, redirectErr func(string, string)) bool {
	d := s.resolveAccessDecision(r.Context(), r, selected.UserID, selected.OrgID, clientID)
	if d.Action == domain.AccessBlock {
		s.auditAccessDecision(r.Context(), r, selected.UserID, selected.OrgID, "auth.ca.blocked", d)
		redirectErr("access_denied", msgAccessBlocked)
		return false
	}
	if d.Action == domain.AccessRequireMFA && !hasMFA(selected.AMR) {
		if err := s.desviarAStepUp(w, r, original, clientID); err != nil {
			redirectErr("server_error", "")
		}
		return false
	}
	return true
}

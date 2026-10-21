// SPDX-License-Identifier: Apache-2.0
package http

import "git.sytue.eu/sytue/one/platform/pkg/callaccess"

func (s *Server) documentCallRevoker() callaccess.Revoker {
	client := callaccess.New(s.cfg.RoomsCallAccessURL, s.cfg.RoomsCallAccessSecret)
	if client == nil {
		return nil
	}
	return client
}

// SPDX-License-Identifier: Apache-2.0
package model

import "git.sytue.eu/sytue/one/apps/nucleo/resource"

// AllowsResourceAction caps an explicitly signed resource grant at the original
// Office role. It does not grant an action or select a validator. Validation is
// a separate signed choice available only to an original document manager.
func (r Role) AllowsResourceAction(action resource.Action) bool {
	switch action {
	case resource.Read:
		return r.CanRead()
	case resource.Comment:
		return r.CanComment()
	case resource.Publish:
		return r.CanWrite()
	case resource.Manage, resource.Validate:
		return r.CanManage()
	default:
		return false
	}
}

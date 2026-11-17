// SPDX-License-Identifier: Apache-2.0
package mail_test

import (
	privateMail "git.sytue.eu/sytue/one/apps/nucleo/mail"
	"git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

// Private Mail uses the maintained shared adapter, including explicit first-
// publication target pinning. It must not drift into a fake-only port contract.
var _ privateMail.Crypto = (*matrix.Client)(nil)

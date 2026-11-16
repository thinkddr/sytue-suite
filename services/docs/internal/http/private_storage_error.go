// SPDX-License-Identifier: Apache-2.0
package http

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

func privateOfficeWriteBlocked(err error) bool {
	var failure *pgconn.PgError
	return errors.As(err, &failure) && failure.Code == "PST01"
}

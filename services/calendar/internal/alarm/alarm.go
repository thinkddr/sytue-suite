// SPDX-License-Identifier: Apache-2.0
// Package alarm retains the server contract over device-side reminder evaluation.
package alarm

import (
	"time"

	privatealarm "git.sytue.eu/sytue/one/apps/nucleo/calendar/alarm"
)

type Trigger = privatealarm.Trigger

func Triggers(data string, start, end time.Time) ([]Trigger, error) {
	return privatealarm.Triggers(data, start, end)
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRecurrenciaGraph(t *testing.T) {
	martes := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	casos := []struct{ rrule, quiero string }{
		{
			"FREQ=DAILY;COUNT=5",
			`{"pattern":{"interval":1,"type":"daily"},"range":{"numberOfOccurrences":5,"startDate":"2026-09-15","type":"numbered"}}`,
		},
		{
			"FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,WE;UNTIL=20261231T235959Z",
			`{"pattern":{"daysOfWeek":["monday","wednesday"],"firstDayOfWeek":"monday","interval":2,"type":"weekly"},"range":{"endDate":"2026-12-31","startDate":"2026-09-15","type":"endDate"}}`,
		},
		{
			"FREQ=WEEKLY",
			`{"pattern":{"daysOfWeek":["tuesday"],"firstDayOfWeek":"monday","interval":1,"type":"weekly"},"range":{"startDate":"2026-09-15","type":"noEnd"}}`,
		},
		{
			"FREQ=MONTHLY;BYDAY=-1FR",
			`{"pattern":{"daysOfWeek":["friday"],"index":"last","interval":1,"type":"relativeMonthly"},"range":{"startDate":"2026-09-15","type":"noEnd"}}`,
		},
		{
			"FREQ=YEARLY;BYMONTHDAY=15;BYMONTH=9",
			`{"pattern":{"dayOfMonth":15,"interval":1,"month":9,"type":"absoluteYearly"},"range":{"startDate":"2026-09-15","type":"noEnd"}}`,
		},
	}
	for _, c := range casos {
		rec, err := recurrenciaGraph(c.rrule, martes)
		if err != nil {
			t.Fatalf("%s: %v", c.rrule, err)
		}
		got, _ := json.Marshal(rec)
		if string(got) != c.quiero {
			t.Errorf("%s\n got: %s\nwant: %s", c.rrule, got, c.quiero)
		}
	}
	// Lo que Outlook no puede representar se rechaza, no se aproxima.
	for _, malo := range []string{"FREQ=MONTHLY;BYSETPOS=-1;BYDAY=MO,TU", "FREQ=HOURLY", "FREQ=MONTHLY;BYDAY=5MO"} {
		if _, err := recurrenciaGraph(malo, martes); err == nil {
			t.Errorf("%s debía rechazarse", malo)
		}
	}
}

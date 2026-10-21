// SPDX-License-Identifier: Apache-2.0
package linkedservices

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestYahooContactEditPreservesCardPropertiesAndETag(t *testing.T) {
	const card = "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:fixture\r\nFN:Fixture\r\nN:Ramos;Ana;;Dr;PhD\r\nEMAIL;TYPE=WORK:fixture@example.eu\r\nPHOTO;VALUE=URI:https://untrusted.invalid/private\r\nX-PRIVATE-LABEL:Keep this\r\nNOTE:Before\r\nEND:VCARD\r\n"
	writes := 0
	client := providerFixture(t, "yahoo", func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "carddav.address.yahoo.com" {
			t.Errorf("unexpected origin %s", r.URL)
		}
		if r.Method == http.MethodGet {
			res := providerReply(r, 200, card)
			res.Header.Set("Content-Type", "text/vcard")
			res.Header.Set("ETag", `"v1"`)
			return res, nil
		}
		writes++
		raw, _ := io.ReadAll(r.Body)
		for _, keep := range []string{"X-PRIVATE-LABEL:Keep this", "PHOTO;VALUE=URI:https://untrusted.invalid/private", "EMAIL;TYPE=WORK:fixture@example.eu", "N:Ramos;Ana;;Dr;PhD", "NOTE:Updated"} {
			if !strings.Contains(string(raw), keep) {
				t.Errorf("lost unchanged card property %q: %s", keep, raw)
			}
		}
		if r.Method != http.MethodPut || r.Header.Get("If-Match") != `"v1"` {
			t.Errorf("unconditional write %s %#v", r.Method, r.Header)
		}
		res := providerReply(r, 204, "")
		res.Header.Set("ETag", `"v2"`)
		return res, nil
	})
	note := "Updated"
	id := davID("/dav/user/contacts/fixture.vcf", time.Time{})
	result, err := client.SaveContact(context.Background(), "/dav/user/contacts/", id, `"v1"`, ContactPatch{Note: &note})
	if err != nil || writes != 1 || result.ETag != `"v2"` || result.ReadOnly {
		t.Fatalf("DAV update %d %#v %v", writes, result, err)
	}
	if _, err = client.SaveContact(context.Background(), "/dav/user/contacts/", id, `"stale"`, ContactPatch{Note: &note}); !errors.Is(err, ErrConflict) || writes != 1 {
		t.Fatal("stale DAV card overwritten")
	}
}

func TestYahooCalendarPatchPreservesAlarmsAndUnknownProperties(t *testing.T) {
	const calendar = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Fixture//EN\r\nX-CALENDAR-PRIVATE:Keep calendar\r\nBEGIN:VEVENT\r\nUID:fixture\r\nDTSTAMP:20261020T090000Z\r\nDTSTART:20261020T100000Z\r\nDTEND:20261020T110000Z\r\nSUMMARY:Before\r\nX-APP-DATA:Keep event\r\nBEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT10M\r\nDESCRIPTION:Keep reminder\r\nEND:VALARM\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	writes := 0
	client := providerFixture(t, "yahoo", func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet {
			res := providerReply(r, 200, calendar)
			res.Header.Set("Content-Type", "text/calendar")
			res.Header.Set("ETag", `"v1"`)
			return res, nil
		}
		writes++
		raw, _ := io.ReadAll(r.Body)
		for _, keep := range []string{"X-CALENDAR-PRIVATE:Keep calendar", "X-APP-DATA:Keep event", "BEGIN:VALARM", "TRIGGER:-PT10M", "SUMMARY:Updated"} {
			if !strings.Contains(string(raw), keep) {
				t.Errorf("lost calendar content %q: %s", keep, raw)
			}
		}
		if r.Header.Get("If-Match") != `"v1"` {
			t.Error("calendar version omitted")
		}
		res := providerReply(r, 204, "")
		res.Header.Set("ETag", `"v2"`)
		return res, nil
	})
	summary := "Updated"
	id := davID("/dav/user/calendar/fixture.ics", time.Time{})
	if _, err := client.SaveEvent(context.Background(), "/dav/user/calendar/", id, `"v1"`, EventPatch{Summary: &summary}); err != nil || writes != 1 {
		t.Fatalf("DAV event update %d %v", writes, err)
	}
}

func TestYahooRecurrenceCannotBeFlattenedBySimpleEditor(t *testing.T) {
	const calendar = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Fixture//EN\r\nBEGIN:VEVENT\r\nUID:fixture\r\nDTSTART:20261020T100000Z\r\nDTEND:20261020T110000Z\r\nRRULE:FREQ=DAILY\r\nSUMMARY:Series\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	writes := 0
	client := providerFixture(t, "yahoo", func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet {
			writes++
		}
		res := providerReply(r, 200, calendar)
		res.Header.Set("Content-Type", "text/calendar")
		res.Header.Set("ETag", `"v1"`)
		return res, nil
	})
	summary := "Updated"
	_, err := client.SaveEvent(context.Background(), "/dav/user/calendar/", davID("/dav/user/calendar/fixture.ics", time.Time{}), `"v1"`, EventPatch{Summary: &summary})
	if !errors.Is(err, ErrReadOnly) || writes != 0 {
		t.Fatalf("recurrence flattened %d %v", writes, err)
	}
}

func TestYahooDAVRejectsRemoteHrefsTraversalAndCrossCollectionObjects(t *testing.T) {
	for _, value := range []string{"https://evil.invalid/data", "https://caldav.calendar.yahoo.com.evil.invalid/data", "//evil.invalid/path", "/dav/../other", "/dav/%2e%2e/other", "/dav/object?token=x", "/dav/object#fragment"} {
		if _, err := davPath(yahooCalendar, value); err == nil {
			t.Errorf("unsafe DAV href %q", value)
		}
	}
	for _, value := range []string{"/dav/other/object.ics", "/dav/calendar/sub/object.ics", "https://carddav.address.yahoo.com/dav/calendar/object.ics"} {
		if _, err := davObject(yahooCalendar, "/dav/calendar/", value); err == nil {
			t.Errorf("cross-collection object %q", value)
		}
	}
}

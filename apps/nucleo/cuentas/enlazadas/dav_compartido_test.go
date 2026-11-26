// SPDX-License-Identifier: Apache-2.0
package enlazadas

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// S-02: Yahoo goes through the shared DAV client (`cuentas/dav`) with Yahoo as the
// injected policy. Listing calendars, events (expanded by the server) and contacts
// keeps the service's behaviour: paths as IDs, quoted ETags, every calendar of the home
// (also task lists), hrefs to another host never followed, and provider errors as the
// service's own errors.
func TestYahooThroughTheSharedDAVClient(t *testing.T) {
	var asked []string
	var bodies []string
	client := providerFixture(t, "yahoo", func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		asked = append(asked, r.Method+" "+r.URL.Host+r.URL.Path)
		bodies = append(bodies, string(raw))
		if r.Header.Get("User-Agent") == "SytueOffice" {
			t.Errorf("the shared client's name reached the provider")
		}
		multi := func(body string) (*http.Response, error) {
			res := providerReply(r, 207, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav" xmlns:card="urn:ietf:params:xml:ns:carddav">`+body+`</d:multistatus>`)
			res.Header.Set("Content-Type", "application/xml")
			return res, nil
		}
		ok := `<d:status>HTTP/1.1 200 OK</d:status>`
		switch {
		case r.Method == "PROPFIND" && r.URL.Host == "carddav.address.yahoo.com":
			return providerReply(r, 401, ""), nil
		case r.Method == "PROPFIND" && r.URL.Path == "/":
			return multi(`<d:response><d:href>/</d:href><d:propstat><d:prop><d:current-user-principal><d:href>/principals/ana/</d:href></d:current-user-principal></d:prop>` + ok + `</d:propstat></d:response>`)
		case r.Method == "PROPFIND" && r.URL.Path == "/principals/ana/":
			return multi(`<d:response><d:href>/principals/ana/</d:href><d:propstat><d:prop><c:calendar-home-set><d:href>/dav/ana/</d:href></c:calendar-home-set></d:prop>` + ok + `</d:propstat></d:response>`)
		case r.Method == "PROPFIND" && r.URL.Path == "/dav/ana/":
			return multi(`<d:response><d:href>/dav/ana/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop>` + ok + `</d:propstat></d:response>` +
				`<d:response><d:href>/dav/ana/work/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/><c:calendar/></d:resourcetype><d:displayname>Work</d:displayname></d:prop>` + ok + `</d:propstat></d:response>` +
				`<d:response><d:href>/dav/ana/tasks/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/><c:calendar/></d:resourcetype><c:supported-calendar-component-set><c:comp name="VTODO"/></c:supported-calendar-component-set></d:prop>` + ok + `</d:propstat></d:response>` +
				`<d:response><d:href>https://evil.invalid/dav/ana/steal/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/><c:calendar/></d:resourcetype><d:displayname>Steal</d:displayname></d:prop>` + ok + `</d:propstat></d:response>`)
		case r.Method == "REPORT" && strings.Contains(string(raw), "calendar-query"):
			return multi(`<d:response><d:href>/dav/ana/work/a.ics</d:href><d:propstat><d:prop><d:getetag>"e1"</d:getetag><c:calendar-data>BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//x//EN
BEGIN:VEVENT
UID:a
DTSTART:20261020T100000Z
DTEND:20261020T110000Z
SUMMARY:Standup
RECURRENCE-ID:20261020T100000Z
END:VEVENT
END:VCALENDAR
</c:calendar-data></d:prop>` + ok + `</d:propstat></d:response>`)
		case r.Method == "REPORT" && strings.Contains(string(raw), "addressbook-query"):
			return multi(`<d:response><d:href>/dav/ana/book/b.vcf</d:href><d:propstat><d:prop><d:getetag>"k1"</d:getetag><card:address-data>BEGIN:VCARD
VERSION:3.0
UID:b
FN:Bea Gil
EMAIL:bea@example.org
END:VCARD
</card:address-data></d:prop>` + ok + `</d:propstat></d:response>`)
		}
		return providerReply(r, 401, ""), nil
	})
	ctx := context.Background()
	page, err := client.Collections(ctx, Calendar)
	if err != nil || len(page.Items) != 2 || page.Items[0].ID != "/dav/ana/work/" || page.Items[0].Name != "Work" || page.Items[1].ID != "/dav/ana/tasks/" || page.Items[1].Name != "" {
		t.Fatalf("collections: %+v %v", page, err)
	}
	for _, a := range asked {
		if strings.Contains(a, "evil") || !strings.HasPrefix(strings.SplitN(a, " ", 2)[1], "caldav.calendar.yahoo.com/") {
			t.Fatalf("asked outside Yahoo's calendar server: %v", asked)
		}
	}

	from := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	events, err := client.Events(ctx, "/dav/ana/work/", from, from.Add(7*24*time.Hour), "")
	if err != nil || len(events.Items) != 1 {
		t.Fatalf("events: %+v %v", events, err)
	}
	if e := events.Items[0]; e.Summary != "Standup" || e.ETag != `"e1"` || !e.ReadOnly || e.CollectionID != "/dav/ana/work/" {
		t.Fatalf("event: %+v", e)
	}
	if q := bodies[len(bodies)-1]; !strings.Contains(q, `expand start="20261019T000000Z" end="20261026T000000Z"`) || !strings.Contains(q, `time-range start="20261019T000000Z"`) {
		t.Fatalf("the server does not expand the repetitions: %s", q)
	}

	contacts, err := client.Contacts(ctx, "/dav/ana/book/", "")
	if err != nil || len(contacts.Items) != 1 || contacts.Items[0].Name != "Bea Gil" || contacts.Items[0].ETag != `"k1"` || contacts.Items[0].Emails[0] != "bea@example.org" {
		t.Fatalf("contacts: %+v %v", contacts, err)
	}

	// An object the server puts in another collection is refused, and the provider
	// refusing the token is still the service's ErrAuth.
	if _, err := client.Events(ctx, "/dav/ana/other/", from, from.Add(24*time.Hour), ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("object of another collection: %v", err)
	}
	if _, err := client.Collections(ctx, Contacts); !errors.Is(err, ErrAuth) {
		t.Fatalf("auth: %v", err)
	}
}

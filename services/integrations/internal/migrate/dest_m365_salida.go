// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	_ "github.com/emersion/go-message/charset" // correos en Latin-1, Windows-1252…
	"github.com/emersion/go-message/mail"
)

// Correo y agenda hacia Microsoft 365 (salida por entregas, ver entregas.go).

// graphEnLinea es lo que cabe de adjuntos dentro de la petición que crea el correo:
// Graph corta el cuerpo en 4 MB y el base64 engorda un tercio.
const graphEnLinea = 3 << 20

// carpetasDeSistema son los buzones de Sytue que Outlook ya tiene (nombres conocidos).
var carpetasDeSistema = map[string]string{
	"inbox": "inbox", "sent": "sentitems", "drafts": "drafts",
	"trash": "deleteditems", "junk": "junkemail", "archive": "archive",
}

// MailFolder devuelve la carpeta de un buzón: la de sistema si la hay, y si no la
// cadena de carpetas de su ruta, reutilizando las que ya estén.
func (d *m365Dest) MailFolder(ctx context.Context, ruta, rol string) (string, error) {
	if id, ok := carpetasDeSistema[rol]; ok {
		return id, nil
	}
	padre := "msgfolderroot"
	for _, nombre := range strings.Split(ruta, "/") {
		id, err := d.subcarpeta(ctx, "/mailFolders/"+url.PathEscape(padre)+"/childFolders", "displayName", nombre)
		if err != nil {
			return "", err
		}
		padre = id
	}
	return padre, nil
}

// subcarpeta crea bajo una colección de Graph un elemento con ese nombre o, si ya
// existe (409), devuelve el que hay.
func (d *m365Dest) subcarpeta(ctx context.Context, coleccion, campo, nombre string) (string, error) {
	cuerpo, _ := json.Marshal(map[string]string{campo: nombre})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.m.userRoot()+coleccion, bytes.NewReader(cuerpo))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	var creada struct {
		ID string `json:"id"`
	}
	err = d.do(req, &creada)
	if !errors.Is(err, errGraphConflicto) {
		return creada.ID, err
	}
	q := url.Values{"$filter": {campo + " eq '" + strings.ReplaceAll(nombre, "'", "''") + "'"}, "$select": {"id"}}
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, d.m.userRoot()+coleccion+"?"+q.Encode(), http.NoBody)
	if err != nil {
		return "", err
	}
	var lista struct {
		Value []struct {
			ID string `json:"id"`
		} `json:"value"`
	}
	if err := d.do(req, &lista); err != nil {
		return "", err
	}
	if len(lista.Value) == 0 {
		return "", fmt.Errorf("migrate: Microsoft 365 dice que %q existe pero no lo encuentra", nombre)
	}
	return lista.Value[0].ID, nil
}

type (
	graphDireccion struct {
		EmailAddress struct {
			Name    string `json:"name,omitempty"`
			Address string `json:"address"`
		} `json:"emailAddress"` //nolint:tagliatelle // contrato de Microsoft Graph.
	}
	graphAdjunto struct {
		Tipo         string `json:"@odata.type"` //nolint:tagliatelle // contrato de Microsoft Graph.
		Name         string `json:"name"`
		ContentType  string `json:"contentType"`         //nolint:tagliatelle // idem.
		ContentBytes string `json:"contentBytes"`        //nolint:tagliatelle // idem.
		IsInline     bool   `json:"isInline"`            //nolint:tagliatelle // idem.
		ContentID    string `json:"contentId,omitempty"` //nolint:tagliatelle // idem.
		datos        []byte
	}
)

// ImportMessage crea el correo en la carpeta.
//
// Graph no importa un .eml como correo RECIBIDO: lo que entra por MIME queda como
// borrador. Así que se lee (cabeceras, cuerpo, adjuntos) y se crea con la marca de
// leído y no enviado (PR_MESSAGE_FLAGS = 1) y sus fechas de envío y llegada. Esa
// marca solo se puede poner al crearlo y todo tiene que caber en una petición de 4
// MB: un correo con más adjuntos que eso se crea como borrador y los adjuntos van
// después, que es lo único que Graph permite (un correo recibido no admite
// adjuntos nuevos). Se conserva todo; lo que cambia es que Outlook lo marca como
// borrador.
//
//nolint:gocyclo // un correo MIME: cuerpo, adjuntos en línea y sueltos, y el tope de 4 MB de Graph.
func (d *m365Dest) ImportMessage(ctx context.Context, folderID string, _ int64, raw io.Reader) (string, error) {
	mr, err := mail.CreateReader(raw)
	if err != nil {
		return "", fmt.Errorf("%w: el correo no se puede leer: %w", ErrUnimportable, err)
	}
	h := mr.Header
	asunto, _ := h.Subject()
	fecha, _ := h.Date()
	var html, texto string
	var adjuntos []graphAdjunto
	enLinea := 0
	for {
		p, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("%w: parte ilegible: %w", ErrUnimportable, err)
		}
		datos, err := io.ReadAll(p.Body)
		if err != nil {
			return "", fmt.Errorf("%w: parte ilegible: %w", ErrUnimportable, err)
		}
		adj := graphAdjunto{Tipo: "#microsoft.graph.fileAttachment", datos: datos}
		switch ph := p.Header.(type) {
		case *mail.InlineHeader:
			tipo, params, _ := ph.ContentType()
			switch {
			case tipo == "text/html" && html == "":
				html = string(datos)
				continue
			case tipo == "text/plain" && texto == "":
				texto = string(datos)
				continue
			}
			adj.ContentType, adj.Name, adj.IsInline = tipo, params["name"], true
			adj.ContentID = strings.Trim(ph.Get("Content-Id"), "<>")
		case *mail.AttachmentHeader:
			adj.Name, _ = ph.Filename()
			adj.ContentType, _, _ = ph.ContentType()
		}
		adj.Name = firstNonEmpty(adj.Name, adj.ContentID, "adjunto")
		enLinea += len(datos)
		adjuntos = append(adjuntos, adj)
	}

	correo := map[string]any{
		"subject":       asunto,
		"body":          map[string]string{"contentType": "text", "content": texto},
		"toRecipients":  direcciones(h, "To"),
		"ccRecipients":  direcciones(h, "Cc"),
		"bccRecipients": direcciones(h, "Bcc"),
		"replyTo":       direcciones(h, "Reply-To"),
	}
	if html != "" {
		correo["body"] = map[string]string{"contentType": "html", "content": html}
	}
	if de := direcciones(h, "From"); len(de) > 0 {
		correo["from"] = de[0]
	}
	propiedades := []map[string]string{}
	if !fecha.IsZero() {
		f := fecha.UTC().Format(time.RFC3339)
		propiedades = append(propiedades,
			map[string]string{"id": "SystemTime 0x0E06", "value": f}, // PR_MESSAGE_DELIVERY_TIME
			map[string]string{"id": "SystemTime 0x0039", "value": f}, // PR_CLIENT_SUBMIT_TIME
		)
	}
	// El cuerpo cuenta doble: en JSON, cada < y > de un HTML ocupa seis bytes.
	caben := enLinea+2*(len(html)+len(texto)) <= graphEnLinea
	if caben {
		propiedades = append(propiedades, map[string]string{"id": "Integer 0x0E07", "value": "1"}) // leído, no borrador
		for i := range adjuntos {
			adjuntos[i].ContentBytes = base64.StdEncoding.EncodeToString(adjuntos[i].datos)
		}
		correo["attachments"] = adjuntos
	}
	correo["singleValueExtendedProperties"] = propiedades

	id, err := d.crear(ctx, "/mailFolders/"+url.PathEscape(folderID)+"/messages", correo)
	if err != nil || caben {
		return id, err
	}
	for _, a := range adjuntos {
		if err := d.adjuntar(ctx, id, a); err != nil {
			// Fuera el borrador a medias: al reintentar se crea entero, no un segundo.
			if del, e := http.NewRequestWithContext(ctx, http.MethodDelete, d.m.userRoot()+"/messages/"+url.PathEscape(id), http.NoBody); e == nil {
				_ = d.do(del, nil)
			}
			return "", err
		}
	}
	return id, nil
}

// adjuntar añade un adjunto a un borrador: directo si es pequeño, por sesión si no.
func (d *m365Dest) adjuntar(ctx context.Context, mensaje string, a graphAdjunto) error {
	base := "/messages/" + url.PathEscape(mensaje) + "/attachments"
	if len(a.datos) <= graphEnLinea {
		a.ContentBytes = base64.StdEncoding.EncodeToString(a.datos)
		_, err := d.crear(ctx, base, a)
		return err
	}
	var sesion struct {
		UploadURL string `json:"uploadUrl"` //nolint:tagliatelle // contrato de Microsoft Graph.
	}
	cuerpo := map[string]any{"AttachmentItem": map[string]any{
		"attachmentType": "file", "name": a.Name, "size": len(a.datos),
		"contentType": a.ContentType, "isInline": a.IsInline, "contentId": a.ContentID,
	}}
	if err := d.crearEn(ctx, base+"/createUploadSession", cuerpo, &sesion); err != nil {
		return err
	}
	_, err := d.enSesion(ctx, sesion.UploadURL, int64(len(a.datos)), bytes.NewReader(a.datos))
	return err
}

func direcciones(h mail.Header, campo string) []graphDireccion {
	lista, _ := h.AddressList(campo)
	out := make([]graphDireccion, 0, len(lista))
	for _, a := range lista {
		var g graphDireccion
		g.EmailAddress.Name, g.EmailAddress.Address = a.Name, a.Address
		out = append(out, g)
	}
	return out
}

// crear hace un POST JSON a una colección del usuario y devuelve el id creado.
func (d *m365Dest) crear(ctx context.Context, coleccion string, cuerpo any) (string, error) {
	var creado struct {
		ID string `json:"id"`
	}
	err := d.crearEn(ctx, coleccion, cuerpo, &creado)
	return creado.ID, err
}

func (d *m365Dest) crearEn(ctx context.Context, coleccion string, cuerpo, dst any) error {
	raw, err := json.Marshal(cuerpo)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.m.userRoot()+coleccion, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return d.do(req, dst)
}

// CreateCalendar crea (o reutiliza) un calendario con el nombre del de Sytue.
func (d *m365Dest) CreateCalendar(ctx context.Context, nombre string) (string, error) {
	return d.subcarpeta(ctx, "/calendars", "name", nombre)
}

// ImportEvent crea el evento. Sin asistentes: en Outlook, crear un evento con
// asistentes les envía la invitación, y copiar la agenda no puede escribir a nadie.
// Van en la descripción. Las repeticiones quitadas de una serie se borran de la
// serie creada, que es como Graph las representa.
func (d *m365Dest) ImportEvent(ctx context.Context, calendarID string, ev EventoSalida) (string, error) {
	if strings.EqualFold(ev.Status, "cancelled") {
		return "", fmt.Errorf("%w: evento cancelado", ErrUnimportable)
	}
	if len(ev.RDate) > 0 && ev.RecurrenceID == nil {
		return "", fmt.Errorf("%w: la repetición tiene fechas sueltas (RDATE), que Outlook no admite", ErrUnimportable)
	}
	loc := time.UTC
	if l, err := time.LoadLocation(ev.TZID); ev.TZID != "" && err == nil {
		loc = l
	}
	momento := func(t time.Time) map[string]string {
		if ev.AllDay {
			return map[string]string{"dateTime": t.UTC().Format("2006-01-02") + "T00:00:00", "timeZone": "UTC"}
		}
		return map[string]string{"dateTime": t.In(loc).Format("2006-01-02T15:04:05"), "timeZone": loc.String()}
	}
	descripcion := ev.Description
	if len(ev.Attendees) > 0 {
		correos := make([]string, len(ev.Attendees))
		for i, a := range ev.Attendees {
			correos[i] = a.Email
		}
		descripcion = strings.TrimSpace(descripcion + "\n\n👥 " + strings.Join(correos, ", "))
	}
	evento := map[string]any{
		"subject":  ev.Summary,
		"body":     map[string]string{"contentType": "text", "content": descripcion},
		"start":    momento(ev.DTStart),
		"end":      momento(ev.DTEnd),
		"isAllDay": ev.AllDay,
		"location": map[string]string{"displayName": ev.Location},
	}
	serie := ev.RRule != "" && ev.RecurrenceID == nil
	if serie {
		rec, err := recurrenciaGraph(ev.RRule, ev.DTStart.In(loc))
		if err != nil {
			return "", fmt.Errorf("%w: %w", ErrUnimportable, err)
		}
		evento["recurrence"] = rec
	}
	id, err := d.crear(ctx, "/calendars/"+url.PathEscape(calendarID)+"/events", evento)
	if err != nil || !serie {
		return id, err
	}
	for _, x := range ev.Excluidas() {
		if err := d.quitarRepeticion(ctx, id, x); err != nil {
			return "", err
		}
	}
	return id, nil
}

// quitarRepeticion borra de una serie la repetición que empieza en ese instante.
func (d *m365Dest) quitarRepeticion(ctx context.Context, serie string, inicio time.Time) error {
	q := url.Values{
		"startDateTime": {inicio.Add(-time.Minute).UTC().Format(time.RFC3339)},
		"endDateTime":   {inicio.Add(time.Minute).UTC().Format(time.RFC3339)},
		"$select":       {"id,start"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		d.m.userRoot()+"/events/"+url.PathEscape(serie)+"/instances?"+q.Encode(), http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Prefer", `outlook.timezone="UTC"`)
	var lista struct {
		Value []struct {
			ID    string `json:"id"`
			Start struct {
				DateTime string `json:"dateTime"` //nolint:tagliatelle // contrato de Microsoft Graph.
			} `json:"start"`
		} `json:"value"`
	}
	if err := d.do(req, &lista); err != nil {
		return err
	}
	for _, rep := range lista.Value {
		t, err := time.Parse("2006-01-02T15:04:05", strings.SplitN(rep.Start.DateTime, ".", 2)[0])
		if err != nil || !t.Equal(inicio.UTC().Truncate(time.Second)) {
			continue
		}
		del, err := http.NewRequestWithContext(ctx, http.MethodDelete, d.m.userRoot()+"/events/"+url.PathEscape(rep.ID), http.NoBody)
		if err != nil {
			return err
		}
		return d.do(del, nil)
	}
	return nil
}

// diasGraph traduce los días de RFC 5545 a los de Graph.
var diasGraph = map[string]string{
	"SU": "sunday", "MO": "monday", "TU": "tuesday", "WE": "wednesday",
	"TH": "thursday", "FR": "friday", "SA": "saturday",
}

// ordinalesGraph traduce la posición de BYDAY («2MO», «-1FR»).
var ordinalesGraph = map[int]string{1: "first", 2: "second", 3: "third", 4: "fourth", -1: "last"}

// recurrenciaGraph traduce una RRULE a la recurrencia de Graph. Cubre lo que se
// crea desde un calendario (diaria, semanal por días, mensual por día o por «el
// segundo martes», anual; con intervalo, número de veces o fecha de fin). Lo que
// Outlook no puede representar (BYSETPOS, horas, varios días del mes…) se rechaza
// con su motivo en vez de crear otra serie distinta.
//
//nolint:gocyclo // traductor de RRULE: un caso por parámetro de la gramática.
func recurrenciaGraph(rrule string, inicio time.Time) (map[string]any, error) {
	partes := map[string]string{}
	for _, kv := range strings.Split(rrule, ";") {
		k, v, _ := strings.Cut(kv, "=")
		partes[strings.ToUpper(strings.TrimSpace(k))] = strings.ToUpper(strings.TrimSpace(v))
	}
	for k := range partes {
		switch k {
		case "FREQ", "INTERVAL", "COUNT", "UNTIL", "BYDAY", "BYMONTHDAY", "BYMONTH", "WKST":
		default:
			return nil, fmt.Errorf("la repetición usa %s, que Outlook no admite", k)
		}
	}
	patron := map[string]any{"interval": 1}
	if v := partes["INTERVAL"]; v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("intervalo ilegible %q", v)
		}
		patron["interval"] = n
	}
	dias, ordinal, err := diasDe(partes["BYDAY"], inicio)
	if err != nil {
		return nil, err
	}
	diaDelMes := inicio.Day()
	if v := partes["BYMONTHDAY"]; v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("día del mes que Outlook no admite: %q", v)
		}
		diaDelMes = n
	}
	mes := int(inicio.Month())
	if v := partes["BYMONTH"]; v != "" {
		if mes, err = strconv.Atoi(v); err != nil {
			return nil, fmt.Errorf("mes que Outlook no admite: %q", v)
		}
	}
	switch partes["FREQ"] {
	case "DAILY":
		patron["type"] = "daily"
	case "WEEKLY":
		patron["type"], patron["daysOfWeek"], patron["firstDayOfWeek"] = "weekly", dias, "monday"
	case "MONTHLY", "YEARLY":
		tipo := "absolute"
		if ordinal != "" {
			tipo = "relative"
			patron["daysOfWeek"], patron["index"] = dias, ordinal
		} else {
			patron["dayOfMonth"] = diaDelMes
		}
		if partes["FREQ"] == "YEARLY" {
			patron["type"], patron["month"] = tipo+"Yearly", mes
		} else {
			patron["type"] = tipo + "Monthly"
		}
	default:
		return nil, fmt.Errorf("frecuencia %q que Outlook no admite", partes["FREQ"])
	}

	rango := map[string]any{"startDate": inicio.Format("2006-01-02"), "type": "noEnd"}
	switch {
	case partes["COUNT"] != "":
		n, err := strconv.Atoi(partes["COUNT"])
		if err != nil || n < 1 {
			return nil, fmt.Errorf("número de repeticiones ilegible %q", partes["COUNT"])
		}
		rango["type"], rango["numberOfOccurrences"] = "numbered", n
	case partes["UNTIL"] != "":
		fin, err := time.Parse("20060102", partes["UNTIL"][:min(8, len(partes["UNTIL"]))])
		if err != nil {
			return nil, fmt.Errorf("fecha de fin ilegible %q", partes["UNTIL"])
		}
		rango["type"], rango["endDate"] = "endDate", fin.Format("2006-01-02")
	}
	return map[string]any{"pattern": patron, "range": rango}, nil
}

// diasDe lee BYDAY: los días y, si llevan posición («2MO»), su ordinal. Sin BYDAY,
// el día de la semana del inicio.
func diasDe(byday string, inicio time.Time) ([]string, string, error) {
	if byday == "" {
		return []string{strings.ToLower(inicio.Weekday().String())}, "", nil
	}
	var dias []string
	ordinal := ""
	for _, d := range strings.Split(byday, ",") {
		cod := d[max(0, len(d)-2):]
		nombre, ok := diasGraph[cod]
		if !ok {
			return nil, "", fmt.Errorf("día %q que Outlook no admite", d)
		}
		if pos := d[:len(d)-2]; pos != "" {
			n, err := strconv.Atoi(pos)
			o, ok := ordinalesGraph[n]
			if err != nil || !ok || (ordinal != "" && ordinal != o) {
				return nil, "", fmt.Errorf("posición %q que Outlook no admite", d)
			}
			ordinal = o
		}
		dias = append(dias, nombre)
	}
	return dias, ordinal, nil
}

var (
	_ MailExporter     = (*m365Dest)(nil)
	_ CalendarExporter = (*m365Dest)(nil)
)

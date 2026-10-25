// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxCapacityResponse = 64 << 10

// Las operaciones de blobs admiten transferencias largas. Sólo este sondeo de
// metadatos tiene un plazo total corto, incluido el cuerpo de la respuesta.
func capacityContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 3*time.Second)
}

// Google codifica los int64 como cadenas; Graph y Dropbox, como números.
// La misma validación rechaza signos, fracciones y desbordamientos en los tres.
type capacityBytes int64

func parseCapacityBytes(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, errors.New("capacidad: el proveedor no devolvió bytes enteros no negativos")
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, errors.New("capacidad: los bytes del proveedor exceden int64")
	}
	return n, nil
}

func (b *capacityBytes) UnmarshalJSON(data []byte) error {
	value := string(data)
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
	}
	n, err := parseCapacityBytes(value)
	if err == nil {
		*b = capacityBytes(n)
	}
	return err
}

func capacityInt(n *capacityBytes) *int64 {
	if n == nil {
		return nil
	}
	out := int64(*n)
	return &out
}

func capacityFromUsage(total, used *capacityBytes) Capacity {
	out := Capacity{TotalBytes: capacityInt(total), UsedBytes: capacityInt(used)}
	if total != nil && used != nil {
		available := max(int64(*total)-int64(*used), 0)
		out.AvailableBytes = &available
	}
	return out
}

func capacityFromAvailable(used, available *int64) (Capacity, error) {
	out := Capacity{UsedBytes: used, AvailableBytes: available}
	if used != nil && available != nil {
		if *used > math.MaxInt64-*available {
			return Capacity{}, errors.New("capacidad: el total de bytes excede int64")
		}
		total := *used + *available
		out.TotalBytes = &total
	}
	return out, nil
}

func readCapacityBody(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxCapacityResponse+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxCapacityResponse {
		return nil, errors.New("capacidad: respuesta del proveedor demasiado grande")
	}
	return data, nil
}

func decodeCapacityJSON(body io.Reader, out any) error {
	data, err := readCapacityBody(body)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// Capacity usa el mismo permiso drive.file que los blobs. usage incluye todo
// lo que gasta la cuota compartida de Google; usageInDrive omitiría Gmail y Fotos.
// https://developers.google.com/workspace/drive/api/reference/rest/v3/about
func (g *GoogleDrive) Capacity(ctx context.Context) (Capacity, error) {
	ctx, cancel := capacityContext(ctx)
	defer cancel()
	res, err := conProveedor(ctx, g.http, g.tok, http.MethodGet,
		gdriveAPI+"/about?fields=storageQuota(limit,usage)", http.NoBody, 0, nil)
	if err != nil {
		return Capacity{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Capacity{}, errorDe(res, "google drive: capacidad")
	}
	var out struct {
		Quota struct {
			Limit *capacityBytes `json:"limit"`
			Usage *capacityBytes `json:"usage"`
		} `json:"storageQuota"` //nolint:tagliatelle // así lo llama Google Drive.
	}
	if err := decodeCapacityJSON(res.Body, &out); err != nil {
		return Capacity{}, err
	}
	return capacityFromUsage(out.Quota.Limit, out.Quota.Usage), nil
}

// Capacity conserva remaining: Graph puede aplicar restricciones adicionales.
// https://learn.microsoft.com/en-us/graph/api/resources/quota
func (o *OneDrive) Capacity(ctx context.Context) (Capacity, error) {
	ctx, cancel := capacityContext(ctx)
	defer cancel()
	res, err := conProveedor(ctx, o.http, o.tok, http.MethodGet,
		graphAPI+"/me/drive?$select=quota", http.NoBody, 0, nil)
	if err != nil {
		return Capacity{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Capacity{}, errorDe(res, "onedrive: capacidad")
	}
	var out struct {
		Quota struct {
			Total     *capacityBytes `json:"total"`
			Used      *capacityBytes `json:"used"`
			Remaining *capacityBytes `json:"remaining"`
		} `json:"quota"`
	}
	if err := decodeCapacityJSON(res.Body, &out); err != nil {
		return Capacity{}, err
	}
	capacity := capacityFromUsage(out.Quota.Total, out.Quota.Used)
	if out.Quota.Remaining != nil {
		capacity.AvailableBytes = capacityInt(out.Quota.Remaining)
	}
	return capacity, nil
}

// Capacity requiere account_info.read, independiente de Probar: los permisos
// antiguos de archivos siguen sirviendo aunque no permitan leer esta cuota.
// https://github.com/dropbox/dropbox-api-spec/blob/main/users.stone
func (d *Dropbox) Capacity(ctx context.Context) (Capacity, error) {
	ctx, cancel := capacityContext(ctx)
	defer cancel()
	res, err := d.rpc(ctx, "/users/get_space_usage", nil)
	if err != nil {
		return Capacity{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Capacity{}, dropboxError(res, "capacidad")
	}
	var out struct {
		Used       *capacityBytes `json:"used"`
		Allocation struct {
			Kind            string         `json:".tag"` //nolint:tagliatelle // discriminador del protocolo Dropbox.
			Allocated       *capacityBytes `json:"allocated"`
			Used            *capacityBytes `json:"used"`
			MemberLimit     *capacityBytes `json:"user_within_team_space_allocated"`
			MemberUsed      *capacityBytes `json:"user_within_team_space_used_cached"`
			MemberLimitType struct {
				Kind string `json:".tag"` //nolint:tagliatelle // discriminador del protocolo Dropbox.
			} `json:"user_within_team_space_limit_type"`
		} `json:"allocation"`
	}
	if err := decodeCapacityJSON(res.Body, &out); err != nil {
		return Capacity{}, err
	}
	switch out.Allocation.Kind {
	case "individual":
		return capacityFromUsage(out.Allocation.Allocated, out.Used), nil
	case "team":
		capacity := capacityFromUsage(out.Allocation.Allocated, out.Allocation.Used)
		switch out.Allocation.MemberLimitType.Kind {
		case "off", "alert_only":
			// Una advertencia no impide consumir el espacio libre del equipo.
		case "stop_sync":
			if out.Allocation.MemberLimit == nil {
				capacity.AvailableBytes = nil
			} else if *out.Allocation.MemberLimit > 0 {
				member := capacityFromUsage(out.Allocation.MemberLimit, out.Allocation.MemberUsed)
				if member.AvailableBytes == nil || capacity.AvailableBytes == nil {
					capacity.AvailableBytes = nil
				} else {
					available := min(*member.AvailableBytes, *capacity.AvailableBytes)
					capacity.AvailableBytes = &available
				}
			} // Cero indica que no hay límite individual, según Dropbox.
		default:
			// Una respuesta antigua o un tipo nuevo no acredita el libre del miembro.
			capacity.AvailableBytes = nil
		}
		// Lo libre del equipo no es una asignación íntegra para cada miembro:
		// el total efectivo de esta cuenta es su uso más lo que aún puede usar.
		return capacityFromAvailable(capacityInt(out.Used), capacity.AvailableBytes)
	default:
		return Capacity{UsedBytes: capacityInt(out.Used)}, nil
	}
}

type webDAVCapacityResponse struct {
	XMLName   xml.Name                 `xml:"DAV: multistatus"`
	Responses []webDAVCapacityResource `xml:"DAV: response"`
}

type webDAVCapacityResource struct {
	Href      string `xml:"DAV: href"`
	Propstats []struct {
		Status string `xml:"DAV: status"`
		Props  struct {
			Available []string `xml:"DAV: quota-available-bytes"`
			Used      []string `xml:"DAV: quota-used-bytes"`
		} `xml:"DAV: prop"`
	} `xml:"DAV: propstat"`
}

// Capacity consulta sólo la colección configurada, sin enumerar sus archivos.
// NAS reutiliza este adaptador y sus credenciales a través del túnel HTTPS.
// RFC 4331: una propiedad ausente (404 en propstat) no acredita espacio infinito.
func (w *WebDAV) Capacity(ctx context.Context) (Capacity, error) {
	ctx, cancel := capacityContext(ctx)
	defer cancel()
	body := []byte(`<D:propfind xmlns:D="DAV:"><D:prop><D:quota-available-bytes/><D:quota-used-bytes/></D:prop></D:propfind>`)
	res, err := w.hacer(ctx, "PROPFIND", w.base.Path, bytes.NewReader(body), int64(len(body)),
		map[string]string{"Depth": "0", "Content-Type": "application/xml; charset=utf-8"})
	if err != nil {
		return Capacity{}, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusMethodNotAllowed || res.StatusCode == http.StatusNotImplemented {
		return Capacity{}, nil
	}
	if res.StatusCode != http.StatusMultiStatus && res.StatusCode != http.StatusOK {
		return Capacity{}, errorDe(res, "webdav: capacidad")
	}
	data, err := readCapacityBody(res.Body)
	if err != nil {
		return Capacity{}, err
	}
	var reply webDAVCapacityResponse
	if err := xml.Unmarshal(data, &reply); err != nil {
		return Capacity{}, err
	}
	return reply.capacityFor(w.base)
}

func (reply webDAVCapacityResponse) capacityFor(base *url.URL) (Capacity, error) {
	var target *webDAVCapacityResource
	for i := range reply.Responses {
		response := &reply.Responses[i]
		if !sameCapacityCollection(base, response.Href) {
			continue
		}
		if target != nil {
			return Capacity{}, errors.New("webdav: capacidad ambigua para la colección")
		}
		target = response
	}
	if target == nil {
		return Capacity{}, nil
	}
	return target.capacity()
}

func sameCapacityCollection(base *url.URL, raw string) bool {
	href, err := url.Parse(raw)
	if err != nil || raw == "" {
		return false
	}
	href = base.ResolveReference(href)
	return href.Scheme == base.Scheme && href.Host == base.Host && href.User == nil && href.RawQuery == "" && href.Fragment == "" &&
		strings.TrimSuffix(href.Path, "/") == strings.TrimSuffix(base.Path, "/")
}

func (resource webDAVCapacityResource) capacity() (Capacity, error) {
	var out Capacity
	for _, propstat := range resource.Propstats {
		status := strings.Fields(propstat.Status)
		if len(status) < 2 || status[1] != "200" {
			continue
		}
		if _, _, valid := http.ParseHTTPVersion(status[0]); !valid {
			continue
		}
		if err := assignCapacityProperty(&out.AvailableBytes, propstat.Props.Available); err != nil {
			return Capacity{}, err
		}
		if err := assignCapacityProperty(&out.UsedBytes, propstat.Props.Used); err != nil {
			return Capacity{}, err
		}
	}
	return capacityFromAvailable(out.UsedBytes, out.AvailableBytes)
}

func assignCapacityProperty(target **int64, values []string) error {
	if len(values) == 0 {
		return nil
	}
	if len(values) != 1 || *target != nil {
		return errors.New("webdav: propiedad de capacidad repetida")
	}
	n, err := parseCapacityBytes(values[0])
	if err != nil {
		return err
	}
	*target = &n
	return nil
}

var (
	_ CapacityReader = (*GoogleDrive)(nil)
	_ CapacityReader = (*OneDrive)(nil)
	_ CapacityReader = (*Dropbox)(nil)
	_ CapacityReader = (*WebDAV)(nil)
)

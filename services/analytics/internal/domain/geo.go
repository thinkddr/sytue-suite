// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"net/netip"
	"os"
	"slices"
	"strings"
)

// CountryUnknown es el país que se guarda cuando no se resuelve. NUNCA se
// guarda nada más preciso «por si acaso»: la IP no se persiste jamás.
const CountryUnknown = "ZZ"

// Countries resuelve país a partir de una IP con una búsqueda binaria sobre los
// rangos ordenados de DB-IP IP-to-Country Lite. Es una tabla en memoria y de
// solo lectura tras cargarse: la IP entra como argumento y no sale de la pila.
//
// ATRIBUCIÓN (obligatoria, va también al NOTICE del repo): IP Geolocation by
// DB-IP (https://db-ip.com), fichero «IP to Country Lite», licencia CC BY 4.0.
//
// ponytail: ~700 000 rangos × 56 B ≈ 40 MB residentes. Si algún día molesta, la
// estructura compacta (uint32 para IPv4, tabla aparte para IPv6) baja a ~10 MB;
// hoy no compensa complicar 50 líneas.
type Countries struct {
	ranges []ipRange
}

type ipRange struct {
	lo, hi netip.Addr
	cc     [2]byte
}

// LoadCountries lee el CSV de DB-IP («inicio,fin,CC» por línea), admitiendo
// .csv y .csv.gz. Un path vacío devuelve (nil, nil): SIN fichero el servicio
// arranca igual y todo el tráfico queda como CountryUnknown — la analítica
// pierde el desglose por país, no la medición.
func LoadCountries(path string) (*Countries, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil //nolint:nilnil // contrato documentado: sin fichero, sin geolocalización.
	}
	f, err := os.Open(path) //nolint:gosec // G304: la ruta la fija el despliegue, no una petición.
	if err != nil {
		return nil, fmt.Errorf("geo: abrir %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var src io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, fmt.Errorf("geo: gzip %s: %w", path, err)
		}
		defer func() { _ = gz.Close() }()
		src = gz
	}
	return ParseCountries(src)
}

// ParseCountries lee los rangos de un CSV ya descomprimido («inicio,fin,CC»).
func ParseCountries(src io.Reader) (*Countries, error) {
	c := &Countries{ranges: make([]ipRange, 0, 1<<19)}
	sc := bufio.NewScanner(src)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		a, rest, ok := strings.Cut(line, ",")
		if !ok {
			continue
		}
		b, cc, ok := strings.Cut(rest, ",")
		if !ok || len(cc) != 2 {
			continue
		}
		lo, err := netip.ParseAddr(a)
		if err != nil {
			continue
		}
		hi, err := netip.ParseAddr(b)
		if err != nil {
			continue
		}
		c.ranges = append(c.ranges, ipRange{
			lo: lo.Unmap(), hi: hi.Unmap(),
			cc: [2]byte{upper(cc[0]), upper(cc[1])},
		})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("geo: leer rangos: %w", err)
	}
	if len(c.ranges) == 0 {
		return nil, fmt.Errorf("geo: el fichero no tiene ningún rango válido")
	}
	// El fichero de DB-IP viene ordenado; ordenar igualmente cuesta una vez al
	// arrancar y evita que un fichero manipulado rompa la búsqueda binaria en
	// silencio (devolvería el país de otro).
	if !slices.IsSortedFunc(c.ranges, func(x, y ipRange) int { return x.lo.Compare(y.lo) }) {
		slices.SortFunc(c.ranges, func(x, y ipRange) int { return x.lo.Compare(y.lo) })
	}
	return c, nil
}

func upper(b byte) byte {
	if b >= 'a' && b <= 'z' {
		return b - 32
	}
	return b
}

// Of devuelve el ISO-3166-1 alpha-2 de la IP, o CountryUnknown. Es válido sobre
// un *Countries nil (sin fichero cargado).
func (c *Countries) Of(addr netip.Addr) string {
	if c == nil || !addr.IsValid() {
		return CountryUnknown
	}
	addr = addr.Unmap()
	// Primer rango cuyo final NO queda por debajo de la dirección.
	i, _ := slices.BinarySearchFunc(c.ranges, addr, func(r ipRange, a netip.Addr) int {
		return r.hi.Compare(a)
	})
	if i >= len(c.ranges) || c.ranges[i].lo.Compare(addr) > 0 {
		return CountryUnknown
	}
	return string(c.ranges[i].cc[:])
}

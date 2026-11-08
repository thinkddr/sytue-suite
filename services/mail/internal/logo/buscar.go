// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package logo

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	// Formatos que se aceptan, registrados para image.Decode.
	_ "image/gif"
	_ "image/jpeg"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

const (
	// presupuesto es el tiempo TOTAL de una búsqueda: una web lenta no retiene la
	// petición del cliente.
	presupuesto = 8 * time.Second
	// topePortada: los `<link>` van en el `<head>`, al principio.
	topePortada = 512 << 10
	// topeIcono: un logo de fila no necesita más.
	topeIcono = 256 << 10
	// LadoMinimo: un icono menor, ampliado a la fila, se ve borroso; mejor iniciales.
	LadoMinimo = 32
	// LadoMaximo es a lo que se reduce lo que llega: el avatar más grande que lo
	// pinta es el `lg` del lector (48), y 128 aguanta pantallas de densidad 2,5.
	LadoMaximo = 128
	// maxDeclarados es cuántos iconos de la portada se prueban como mucho.
	maxDeclarados = 3
	maxSaltos     = 3
)

// ErrSinLogo: el dominio no tiene un icono aprovechable.
var ErrSinLogo = errors.New("logo: sin icono aprovechable")

// Respuesta es lo que trae una descarga: los bytes, su tipo y la URL FINAL tras las
// redirecciones, contra la que se resuelven las rutas relativas de una portada.
type Respuesta struct {
	Cuerpo []byte
	Tipo   string
	URL    string
}

// Traer descarga una URL con un tope de bytes. La de serie sale por ssrfhttp; los
// tests ponen la suya.
type Traer func(ctx context.Context, url string, tope int64) (Respuesta, error)

var clienteSeguro = &http.Client{
	Timeout:   presupuesto,
	Transport: ssrfhttp.Transport(), // barrera 2: valida en el connect
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxSaltos {
			return errors.New("logo: demasiadas redirecciones")
		}
		if req.URL.Scheme != "https" {
			return errors.New("logo: redirección sin https")
		}
		return ssrfhttp.ValidateURL(req.Context(), req.URL.String()) // barrera 1 en cada salto
	},
}

// TraerSeguro es la Traer de producción: solo https, barrera SSRF en la URL inicial,
// en cada salto y en el connect, sin cookies ni referer, y cuerpo capado.
func TraerSeguro(ctx context.Context, raw string, tope int64) (Respuesta, error) {
	if !strings.HasPrefix(raw, "https://") {
		return Respuesta{}, errors.New("logo: solo https")
	}
	if err := ssrfhttp.ValidateURL(ctx, raw); err != nil {
		return Respuesta{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, http.NoBody)
	if err != nil {
		return Respuesta{}, err
	}
	req.Header.Set("User-Agent", "Sytue-Mail-Logo/1")
	req.Header.Set("Accept", "text/html,image/png,image/x-icon,image/*;q=0.8,*/*;q=0.1")
	resp, err := clienteSeguro.Do(req)
	if err != nil {
		return Respuesta{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return Respuesta{}, fmt.Errorf("logo: estado %d", resp.StatusCode)
	}
	cuerpo, err := io.ReadAll(io.LimitReader(resp.Body, tope+1))
	if err != nil {
		return Respuesta{}, err
	}
	if int64(len(cuerpo)) > tope {
		return Respuesta{}, errors.New("logo: demasiado grande")
	}
	tipo, _, _ := strings.Cut(resp.Header.Get("Content-Type"), ";")
	return Respuesta{Cuerpo: cuerpo, Tipo: strings.ToLower(strings.TrimSpace(tipo)), URL: resp.Request.URL.String()}, nil
}

var (
	reLink     = regexp.MustCompile(`(?i)<link\b[^>]*>`)
	reAtributo = map[string]*regexp.Regexp{}
)

func init() {
	for _, n := range []string{"rel", "href", "sizes", "type"} {
		reAtributo[n] = regexp.MustCompile(`(?i)\s` + n + `\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	}
}

func atributo(etiqueta, nombre string) string {
	m := reAtributo[nombre].FindStringSubmatch(etiqueta)
	if m == nil {
		return ""
	}
	return m[1] + m[2] + m[3]
}

// Declarados son los iconos que declara una portada, resueltos contra `base` y de
// mejor a peor: primero el más pequeño de los que llegan a 64 px, después los menores
// de mayor a menor. Fuera SVG, `mask-icon` (monocromo) y `data:`.
func Declarados(portada, base string) []string {
	b, err := url.Parse(base)
	if err != nil {
		return nil
	}
	lados := map[string]int{}
	var orden []string
	for _, et := range reLink.FindAllString(portada, -1) {
		k, lado, ok := icono(et, b)
		if !ok {
			continue
		}
		if _, visto := lados[k]; !visto {
			orden = append(orden, k)
		}
		lados[k] = max(lados[k], lado)
	}
	rango := func(l int) int {
		if l >= 64 {
			return l
		}
		return 100_000 - l
	}
	sort.SliceStable(orden, func(i, j int) bool { return rango(lados[orden[i]]) < rango(lados[orden[j]]) })
	return orden
}

// icono lee un `<link>`: su URL absoluta y el lado que declara, u ok falso si no es un
// icono raster en https.
func icono(et string, b *url.URL) (string, int, bool) {
	esTouch, esIcono := false, false
	for _, r := range strings.Fields(strings.ToLower(atributo(et, "rel"))) {
		esTouch = esTouch || strings.HasPrefix(r, "apple-touch-icon")
		esIcono = esIcono || r == "icon"
	}
	href := strings.TrimSpace(html.UnescapeString(atributo(et, "href")))
	if (!esTouch && !esIcono) || href == "" || strings.HasPrefix(href, "data:") ||
		strings.Contains(strings.ToLower(atributo(et, "type")), "svg") || esSVG(href) {
		return "", 0, false
	}
	u, err := b.Parse(href)
	if err != nil || u.Scheme != "https" {
		return "", 0, false
	}
	// Sin `sizes`: el apple-touch-icon es de 180 por definición; un favicon, de 16.
	lado := 16
	if esTouch {
		lado = 180
	}
	mayor := 0
	for _, s := range strings.Fields(strings.ToLower(atributo(et, "sizes"))) {
		w, _, _ := strings.Cut(s, "x")
		if n, err := strconv.Atoi(w); err == nil && n > mayor {
			mayor = n
		}
	}
	if mayor > 0 {
		lado = mayor
	}
	return u.String(), lado, true
}

func esSVG(href string) bool {
	h := strings.ToLower(href)
	if i := strings.IndexAny(h, "?#"); i >= 0 {
		h = h[:i]
	}
	return strings.HasSuffix(h, ".svg")
}

// Buscar busca el logo de `dominio` (ya validado) y lo devuelve como PNG de
// LadoMaximo como mucho. ErrSinLogo si no hay uno aprovechable.
func Buscar(ctx context.Context, dominio string, traer Traer) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, presupuesto)
	defer cancel()
	raiz := "https://" + dominio + "/"
	var candidatos []string
	base := raiz
	if p, err := traer(ctx, raiz, topePortada); err == nil &&
		(p.Tipo == "text/html" || p.Tipo == "application/xhtml+xml") {
		base = p.URL
		d := Declarados(string(p.Cuerpo), base)
		candidatos = append(candidatos, d[:min(len(d), maxDeclarados)]...)
	}
	b, _ := url.Parse(base)
	for _, ruta := range []string{"/apple-touch-icon.png", "/favicon.ico"} {
		u := b.ResolveReference(&url.URL{Path: ruta}).String()
		if !contiene(candidatos, u) {
			candidatos = append(candidatos, u)
		}
	}
	for _, u := range candidatos {
		if ctx.Err() != nil {
			break
		}
		r, err := traer(ctx, u, topeIcono)
		if err != nil {
			continue
		}
		if p, err := Normalizar(r.Cuerpo); err == nil {
			return p, nil
		}
	}
	return nil, ErrSinLogo
}

func contiene(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// Normalizar decodifica un icono (PNG, JPEG, GIF, WebP o ICO), descarta los menores
// de LadoMinimo y lo devuelve como PNG de LadoMaximo como mucho.
//
// Reescribirlo no es solo por el tamaño: lo que sale hacia los clientes es siempre un
// PNG que hemos generado nosotros, nunca los bytes que mandó un tercero. Y el cliente
// nativo no sabe decodificar ICO ni WebP.
func Normalizar(datos []byte) ([]byte, error) {
	img, err := decodificar(datos)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	lado := max(b.Dx(), b.Dy())
	if lado < LadoMinimo {
		return nil, fmt.Errorf("logo: icono de %d px", lado)
	}
	if lado > LadoMaximo {
		escala := float64(LadoMaximo) / float64(lado)
		w := max(1, int(math.Round(float64(b.Dx())*escala)))
		h := max(1, int(math.Round(float64(b.Dy())*escala)))
		dst := image.NewNRGBA(image.Rect(0, 0, w, h))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
		img = dst
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodificar(datos []byte) (image.Image, error) {
	if len(datos) >= 4 && datos[0] == 0 && datos[1] == 0 && datos[2] == 1 && datos[3] == 0 {
		return decodificarICO(datos)
	}
	// Los límites del formato van ANTES de decodificar: una cabecera que anuncia
	// 30 000 × 30 000 en 256 KiB es una bomba de memoria, no un logo.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(datos))
	if err != nil {
		return nil, err
	}
	if cfg.Width > 1024 || cfg.Height > 1024 {
		return nil, errors.New("logo: imagen demasiado grande")
	}
	img, _, err := image.Decode(bytes.NewReader(datos))
	return img, err
}

// decodificarICO saca la imagen MAYOR de un .ico: las que van en PNG por dentro (lo
// normal desde Windows Vista) o en DIB de 32 bits (BGRA, de abajo arriba).
func decodificarICO(d []byte) (image.Image, error) {
	if len(d) < 6 {
		return nil, errors.New("logo: ico corto")
	}
	n := int(binary.LittleEndian.Uint16(d[4:6]))
	var mejor image.Image
	mejorLado := 0
	for i := range n {
		e := 6 + i*16
		if e+16 > len(d) {
			break
		}
		tam := int(binary.LittleEndian.Uint32(d[e+8:]))
		off := int(binary.LittleEndian.Uint32(d[e+12:]))
		if off < 0 || tam <= 0 || off+tam > len(d) || off+tam < off {
			continue
		}
		var img image.Image
		if trozo := d[off : off+tam]; bytes.HasPrefix(trozo, []byte("\x89PNG")) {
			img, _ = decodificar(trozo) // con el mismo tope de dimensiones
		} else {
			img = dib32(trozo)
		}
		if img == nil {
			continue
		}
		if l := max(img.Bounds().Dx(), img.Bounds().Dy()); l > mejorLado && l <= 1024 {
			mejor, mejorLado = img, l
		}
	}
	if mejor == nil {
		return nil, errors.New("logo: ico sin imagen legible")
	}
	return mejor, nil
}

// dib32 lee un BITMAPINFOHEADER de 32 bits sin comprimir. El alto del .ico es el
// doble (lleva debajo la máscara AND), y las filas van de abajo arriba.
func dib32(t []byte) image.Image {
	if len(t) < 40 || binary.LittleEndian.Uint32(t) < 40 {
		return nil
	}
	// Sin signo: un alto negativo (DIB de arriba abajo) sale enorme y se rechaza.
	w := int(binary.LittleEndian.Uint32(t[4:]))
	h := int(binary.LittleEndian.Uint32(t[8:])) / 2
	bits := binary.LittleEndian.Uint16(t[14:])
	comp := binary.LittleEndian.Uint32(t[16:])
	cab := int(binary.LittleEndian.Uint32(t))
	if w <= 0 || h <= 0 || w > 256 || h > 256 || bits != 32 || (comp != 0 && comp != 3) {
		return nil
	}
	if cab+w*h*4 > len(t) {
		return nil
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		fila := t[cab+(h-1-y)*w*4:]
		for x := range w {
			p := fila[x*4:]
			img.SetNRGBA(x, y, color.NRGBA{R: p[2], G: p[1], B: p[0], A: p[3]})
		}
	}
	return img
}

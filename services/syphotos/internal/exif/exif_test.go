// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exif

import (
	"encoding/binary"
	"errors"
	"testing"
	"time"
)

// ── Constructor de ficheros de prueba ────────────────────────────────────────
//
// Se COMPONEN los bytes en vez de guardar un .jpg binario en el repositorio: un
// fixture binario no se puede leer en un diff, así que nadie sabe qué cambió
// cuando un test empieza a fallar. Y componerlos es lo que permite fabricar los
// casos hostiles, que es la mitad que importa.

// entrada es una etiqueta a escribir. O lleva `texto` (va al área de datos) o
// `valor` (cabe en los cuatro bytes del propio registro).
type entrada struct {
	tag   uint16
	tipo  uint16
	valor uint32 // para SHORT/LONG, que siempre caben en el registro
	// crudo son los bytes del valor cuando no es un entero suelto: el texto con
	// su NUL, o las fracciones del GPS. `n` es el número de COMPONENTES, que no
	// es lo mismo que bytes salvo en ASCII.
	//
	// Que quepan o no en los cuatro bytes del registro NO lo decide quien
	// escribe: lo decide el tamaño, igual que en el formato. Escribir siempre
	// fuera dejaba las referencias «N» y «E» del GPS —dos bytes— apuntando a un
	// desplazamiento donde el lector, con razón, veía el desplazamiento mismo.
	crudo []byte
	n     uint32
}

func short(tag, v uint16) entrada       { return entrada{tag: tag, tipo: 3, valor: uint32(v)} }
func long(tag uint16, v uint32) entrada { return entrada{tag: tag, tipo: 4, valor: v} }
func ascii(tag uint16, s string) entrada {
	return entrada{tag: tag, tipo: 2, crudo: append([]byte(s), 0), n: uint32(len(s) + 1)}
}

// racional escribe RATIONAL: pares (numerador, denominador) de 32 bits.
func racional(tag uint16, pares ...[2]uint32) entrada {
	b := make([]byte, 0, len(pares)*8)
	for _, p := range pares {
		var par [8]byte
		binary.LittleEndian.PutUint32(par[0:4], p[0])
		binary.LittleEndian.PutUint32(par[4:8], p[1])
		b = append(b, par[:]...)
	}
	return entrada{tag: tag, tipo: 5, crudo: b, n: uint32(len(pares))}
}

// gps compone los cuatro campos de una posición en grados/minutos/segundos.
func gps(latRef string, lat [3]uint32, lonRef string, lon [3]uint32) []entrada {
	frac := func(v [3]uint32) entrada {
		return racional(0, [2]uint32{v[0], 1}, [2]uint32{v[1], 1}, [2]uint32{v[2], 1})
	}
	la, lo := frac(lat), frac(lon)
	la.tag, lo.tag = 0x0002, 0x0004
	return []entrada{ascii(0x0001, latRef), la, ascii(0x0003, lonRef), lo}
}

// tiff monta un bloque TIFF completo: cabecera, IFD0 con su puntero al sub-IFD
// de Exif, el sub-IFD, y el área de datos con los textos.
//
// La geometría se calcula ENTERA antes de escribir nada: intentar ajustarla
// sobre la marcha fue el primer intento y descolocó todos los desplazamientos.
func tiff(ifd0, exif []entrada) []byte { return tiffConGPS(ifd0, exif, nil) }

// tiffConGPS monta el bloque TIFF completo: cabecera, IFD0 con sus punteros a
// los sub-IFD de Exif y de GPS, los dos sub-IFD, y el área de datos.
//
// La geometría se calcula ENTERA antes de escribir nada: intentar ajustarla
// sobre la marcha fue el primer intento y descolocó todos los desplazamientos.
func tiffConGPS(ifd0, exif, gpsIFD []entrada) []byte {
	const cab = 8
	offIFD0 := cab
	nIFD0 := len(ifd0) + 1 // +1: el puntero al sub-IFD de Exif
	if len(gpsIFD) > 0 {
		nIFD0++ // +1 más: el puntero al de GPS
	}
	offExif := offIFD0 + 2 + nIFD0*12 + 4
	offGPS := offExif + 2 + len(exif)*12 + 4
	baseDatos := offGPS
	if len(gpsIFD) > 0 {
		baseDatos = offGPS + 2 + len(gpsIFD)*12 + 4
	}

	// Área de datos: lo que no cabe en los cuatro bytes del registro, en el
	// orden en que aparece.
	var datos []byte
	offsetDe := map[int]uint32{}
	k := 0
	for _, grupo := range [][]entrada{ifd0, exif, gpsIFD} {
		for _, e := range grupo {
			if len(e.crudo) > 4 {
				offsetDe[k] = uint32(baseDatos + len(datos))
				datos = append(datos, e.crudo...)
			}
			k++
		}
	}

	b := make([]byte, baseDatos+len(datos))
	copy(b, "II")
	binary.LittleEndian.PutUint16(b[2:4], 42)
	binary.LittleEndian.PutUint32(b[4:8], uint32(offIFD0))
	copy(b[baseDatos:], datos)

	idx := 0
	escribe := func(pos int, es []entrada, punteros ...entrada) {
		n := len(es) + len(punteros)
		binary.LittleEndian.PutUint16(b[pos:pos+2], uint16(n))
		p := pos + 2
		for _, e := range es {
			binary.LittleEndian.PutUint16(b[p:p+2], e.tag)
			binary.LittleEndian.PutUint16(b[p+2:p+4], e.tipo)
			switch {
			case len(e.crudo) > 4:
				binary.LittleEndian.PutUint32(b[p+4:p+8], e.n)
				binary.LittleEndian.PutUint32(b[p+8:p+12], offsetDe[idx])
			case len(e.crudo) > 0:
				// Cabe: va DENTRO del registro, rellenando con ceros.
				binary.LittleEndian.PutUint32(b[p+4:p+8], e.n)
				copy(b[p+8:p+12], e.crudo)
			default:
				binary.LittleEndian.PutUint32(b[p+4:p+8], 1)
				binary.LittleEndian.PutUint32(b[p+8:p+12], e.valor)
			}
			idx++
			p += 12
		}
		for _, puntero := range punteros {
			binary.LittleEndian.PutUint16(b[p:p+2], puntero.tag)
			binary.LittleEndian.PutUint16(b[p+2:p+4], puntero.tipo)
			binary.LittleEndian.PutUint32(b[p+4:p+8], 1)
			binary.LittleEndian.PutUint32(b[p+8:p+12], puntero.valor)
			p += 12
		}
	}
	punteros := []entrada{long(tagExifIFD, uint32(offExif))}
	if len(gpsIFD) > 0 {
		punteros = append(punteros, long(0x8825, uint32(offGPS)))
	}
	escribe(offIFD0, ifd0, punteros...)
	escribe(offExif, exif)
	if len(gpsIFD) > 0 {
		escribe(offGPS, gpsIFD)
	}
	return b
}

// jpeg envuelve un bloque TIFF en un JPEG con su APP1.
func jpeg(bloqueTIFF []byte) []byte {
	cuerpo := append([]byte("Exif\x00\x00"), bloqueTIFF...)
	// 4 de SOI+APP1, 2 de longitud, el cuerpo y 2 de SOS.
	out := make([]byte, 0, 4+2+len(cuerpo)+2)
	out = append(out, 0xFF, 0xD8, 0xFF, 0xE1)
	l := make([]byte, 2)
	binary.BigEndian.PutUint16(l, uint16(len(cuerpo)+2))
	out = append(out, l...)
	out = append(out, cuerpo...)
	out = append(out, 0xFF, 0xDA) // SOS: aquí empiezan los píxeles
	return out
}

// ── Lo que tiene que leer ────────────────────────────────────────────────────

func TestLeer_fotoCorriente(t *testing.T) {
	datos := jpeg(tiff(
		[]entrada{
			short(tagOrientation, 6),
			ascii(tagMake, "NIKON CORPORATION"),
			ascii(tagModel, "NIKON D750"),
		},
		[]entrada{
			ascii(tagDateTimeOrig, "2026:07:01 12:34:56"),
			ascii(tagLensModel, "24-70mm f/2.8"),
		}))

	m, err := Leer(datos)
	if err != nil {
		t.Fatalf("no leyó: %v", err)
	}
	if m.Orientacion != 6 {
		t.Errorf("orientación %d, se esperaba 6", m.Orientacion)
	}
	if m.TomadaEn == nil || !m.TomadaEn.Equal(time.Date(2026, 7, 1, 12, 34, 56, 0, time.UTC)) {
		t.Errorf("fecha %v", m.TomadaEn)
	}
	// «NIKON CORPORATION» + «NIKON D750» no puede dar «NIKON CORPORATION NIKON D750».
	if m.Camara != "NIKON D750" {
		t.Errorf("cámara %q, se esperaba «NIKON D750»", m.Camara)
	}
	if m.Objetivo != "24-70mm f/2.8" {
		t.Errorf("objetivo %q", m.Objetivo)
	}
}

// Sin EXIF no es un fallo: hay millones de capturas de pantalla y PNG.
func TestLeer_sinExifNoEsUnError(t *testing.T) {
	_, err := Leer([]byte{0xFF, 0xD8, 0xFF, 0xDA})
	if !errors.Is(err, ErrSinExif) {
		t.Errorf("err = %v, se esperaba ErrSinExif", err)
	}
}

// Una cámara sin reloj puesto escribe ceros, y esa fecha mandaría la foto al
// principio de la línea de tiempo delante de todo.
func TestLeer_fechaAbsurdaSeDescarta(t *testing.T) {
	m, err := Leer(jpeg(tiff(
		[]entrada{short(tagOrientation, 1)},
		[]entrada{ascii(tagDateTimeOrig, "0000:00:00 00:00:00")})))
	if err != nil {
		t.Fatal(err)
	}
	if m.TomadaEn != nil {
		t.Errorf("se aceptó una fecha imposible: %v", m.TomadaEn)
	}
}

// ── Lo hostil, que es por lo que esto se escribe a mano ──────────────────────
//
// Ninguno de estos puede provocar un pánico, un bucle ni una reserva enorme.

func TestLeer_noSeCaeConEntradaHostil(t *testing.T) {
	bueno := jpeg(tiff(
		[]entrada{short(tagOrientation, 6)},
		[]entrada{ascii(tagDateTimeOrig, "2026:07:01 12:34:56")}))

	casos := map[string][]byte{
		"vacío":                nil,
		"solo SOI":             {0xFF, 0xD8},
		"longitud cero":        {0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x00},
		"longitud desmesurada": {0xFF, 0xD8, 0xFF, 0xE1, 0xFF, 0xFF, 'E', 'x'},
		"no es JPEG":           []byte("no soy una imagen, soy un texto largo"),
		"fuera de sincronía":   {0xFF, 0xD8, 0x12, 0x34, 0x56, 0x78},
	}
	// Truncado en cada byte: el caso real de una lectura por rango que se corta.
	for n := 1; n < len(bueno); n += 7 {
		casos["truncado en "+string(rune('0'+n%10))+"_"+itoa(n)] = bueno[:n]
	}
	// Y con un byte alterado en cada posición de la zona de cabeceras.
	for i := 0; i < len(bueno) && i < 120; i += 3 {
		corrupto := append([]byte(nil), bueno...)
		corrupto[i] ^= 0xFF
		casos["byte alterado en "+itoa(i)] = corrupto
	}

	for nombre, datos := range casos {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: PÁNICO %v", nombre, r)
				}
			}()
			_, _ = Leer(datos) // el resultado da igual: lo que se prueba es que vuelve
		}()
	}
}

// Un IFD que declara 4.000 millones de entradas no puede reservar memoria por
// esa cifra: la escribe el fichero.
func TestLeer_cuentaDeEntradasDesmesurada(t *testing.T) {
	b := make([]byte, 64)
	copy(b, "II")
	binary.LittleEndian.PutUint16(b[2:4], 42)
	binary.LittleEndian.PutUint32(b[4:8], 8)
	binary.LittleEndian.PutUint16(b[8:10], 0xFFFF) // 65535 entradas declaradas

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PÁNICO con un conteo desmesurado: %v", r)
		}
	}()
	_, _ = Leer(jpeg(b))
}

// Un desplazamiento que apunta fuera del bloque no puede leerse igualmente.
func TestLeer_desplazamientoQueSeSale(t *testing.T) {
	// Se apunta el texto de la cámara a un desplazamiento imposible.
	conMalo := tiff([]entrada{short(tagOrientation, 3), ascii(tagMake, "x")}, nil)
	// El offset del ASCII es el último campo de la segunda entrada del IFD0.
	posOffset := 8 + 2 + 1*12 + 8
	binary.LittleEndian.PutUint32(conMalo[posOffset:posOffset+4], 0xFFFFFF00)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PÁNICO siguiendo un desplazamiento inválido: %v", r)
		}
	}()
	m, _ := Leer(jpeg(conMalo))
	if m.Camara != "" {
		t.Errorf("se leyó texto de un desplazamiento inválido: %q", m.Camara)
	}
	// Y lo que SÍ era válido se conserva: media foto con datos es mejor que nada.
	if m.Orientacion != 3 {
		t.Errorf("una entrada mala se llevó por delante a las buenas (orientación %d)", m.Orientacion)
	}
}

// El puntero al sub-IFD apuntando a sí mismo: seguirlo en bucle es una
// denegación de servicio de dos líneas.
func TestLeer_punteroCiclicoNoCuelga(t *testing.T) {
	b := tiff([]entrada{short(tagOrientation, 1)}, nil)
	// El puntero al Exif IFD lo escribe tiffConBase apuntando al sub-IFD; se
	// reescribe para que apunte al propio IFD0.
	binary.LittleEndian.PutUint32(b[8+2+1*12+8:8+2+1*12+12], 8)

	hecho := make(chan struct{})
	go func() {
		defer close(hecho)
		_, _ = Leer(jpeg(b))
	}()
	select {
	case <-hecho:
	case <-time.After(2 * time.Second):
		t.Fatal("Leer no volvió en 2 s con un puntero cíclico")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}

// FuzzLeer es la red que ningún caso escrito a mano da: los bytes vienen de
// quien sube la foto, así que lo que hay que garantizar no es qué devuelve, es
// que VUELVE — sin pánico, sin bucle y sin reservar memoria por una cifra que
// escribe el propio fichero.
//
// Se siembra con un EXIF válido para que el mutador tenga de dónde partir: sin
// semilla, el fuzzer pasa la mayor parte del tiempo generando cosas que ni
// llegan al SOI.
//
//	go test ./services/syphotos/internal/exif/ -fuzz FuzzLeer
func FuzzLeer(f *testing.F) {
	f.Add(jpeg(tiff(
		[]entrada{short(tagOrientation, 6), ascii(tagMake, "Canon")},
		[]entrada{ascii(tagDateTimeOrig, "2026:07:01 12:34:56")})))
	f.Add([]byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x08, 'E', 'x', 'i', 'f', 0, 0})
	f.Add([]byte("no soy una imagen"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, datos []byte) {
		m, err := Leer(datos)
		// La única invariante de resultado: si dice que hay orientación, está en
		// el rango del estándar. Un valor fuera haría rotar la miniatura de
		// forma imposible y el dominio ya no podría distinguirlo de «no hay».
		if err == nil && m.Orientacion > 8 {
			t.Fatalf("orientación %d fuera de 1..8 con entrada %q", m.Orientacion, datos)
		}
	})
}

// ── Dónde se hizo la foto ────────────────────────────────────────────────────

func TestLeer_posicion(t *testing.T) {
	// La Sagrada Familia: 41°24'13"N, 2°10'27"E.
	m, err := Leer(jpeg(tiffConGPS(
		[]entrada{short(tagOrientation, 1)}, nil,
		gps("N", [3]uint32{41, 24, 13}, "E", [3]uint32{2, 10, 27}),
	)))
	if err != nil {
		t.Fatal(err)
	}
	if m.Lat == nil || m.Lon == nil {
		t.Fatal("no se leyó la posición")
	}
	if q := 41 + 24.0/60 + 13.0/3600; !cerca(*m.Lat, q) {
		t.Errorf("latitud = %v; quería %v", *m.Lat, q)
	}
	if q := 2 + 10.0/60 + 27.0/3600; !cerca(*m.Lon, q) {
		t.Errorf("longitud = %v; quería %v", *m.Lon, q)
	}
}

// Sur y oeste son NEGATIVOS. Sin el signo, Montevideo aparece en Ucrania.
func TestLeer_posicionSurYOeste(t *testing.T) {
	m, err := Leer(jpeg(tiffConGPS(
		[]entrada{short(tagOrientation, 1)}, nil,
		gps("S", [3]uint32{34, 54, 33}, "W", [3]uint32{56, 10, 51}),
	)))
	if err != nil {
		t.Fatal(err)
	}
	if m.Lat == nil || *m.Lat > 0 {
		t.Errorf("latitud sur tiene que ser negativa: %v", m.Lat)
	}
	if m.Lon == nil || *m.Lon > 0 {
		t.Errorf("longitud oeste tiene que ser negativa: %v", m.Lon)
	}
}

// Lo que no es una posición no se guarda como si lo fuera.
func TestLeer_posicionesQueNoValen(t *testing.T) {
	casos := []struct {
		nombre string
		gps    []entrada
	}{
		{"sin referencia no se sabe el hemisferio", []entrada{
			racionalEn(0x0002, 41, 24, 13), racionalEn(0x0004, 2, 10, 27),
		}},
		{"solo latitud no sitúa nada", gps("N", [3]uint32{41, 24, 13}, "", [3]uint32{0, 0, 0})[:2]},
		{
			"cero y cero es «sin cobertura», no el Atlántico",
			gps("N", [3]uint32{0, 0, 0}, "E", [3]uint32{0, 0, 0}),
		},
		{"una latitud imposible", gps("N", [3]uint32{130, 0, 0}, "E", [3]uint32{2, 0, 0})},
		{"un denominador cero es un fichero que miente", []entrada{
			ascii(0x0001, "N"), racional(0x0002, [2]uint32{41, 0}, [2]uint32{24, 1}, [2]uint32{13, 1}),
			ascii(0x0003, "E"), racionalEn(0x0004, 2, 10, 27),
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			m, err := Leer(jpeg(tiffConGPS([]entrada{short(tagOrientation, 1)}, nil, c.gps)))
			if err != nil {
				t.Fatal(err)
			}
			if m.Lat != nil || m.Lon != nil {
				t.Fatalf("no debía salir posición: %v %v", m.Lat, m.Lon)
			}
		})
	}
}

func racionalEn(tag uint16, g, mins, seg uint32) entrada {
	return racional(tag, [2]uint32{g, 1}, [2]uint32{mins, 1}, [2]uint32{seg, 1})
}

func cerca(a, b float64) bool { return a-b < 1e-9 && b-a < 1e-9 }

#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Dos arreglos que `fyne package` no deja hacer, sobre el binario que produce.

  android-binario.py target-sdk <AndroidManifest.xml compilado> <nivel>
  android-binario.py leer-target-sdk <AndroidManifest.xml compilado>
  android-binario.py min-sdk <AndroidManifest.xml compilado> <nivel>
  android-binario.py leer-min-sdk <AndroidManifest.xml compilado>
  android-binario.py alineacion <lib.so> <bytes mínimos>
  android-binario.py servicio-en-primer-plano <AndroidManifest.xml compilado> <clase> <tipo>
  android-binario.py leer-servicio <AndroidManifest.xml compilado> <clase>
  android-binario.py imagen-en-imagen <AndroidManifest.xml compilado>
  android-binario.py leer-imagen-en-imagen <AndroidManifest.xml compilado>

POR QUÉ EXISTE. La CLI de Fyne (fyne.io/tools v1.7.2) fija el targetSdkVersion en
código —29 con `fyne package`, 35 con `fyne release` (mobile/build.go)— y rechaza un
`<uses-sdk>` en nuestro manifiesto (binres.go: «manual declaration of uses-sdk ... not
supported»). Google Play exige 36 a toda app nueva y a toda actualización desde el
31/08/2026, así que ninguna de las dos órdenes produce algo que Play acepte. Se
compila con `fyne package` —la cadena ya verificada, con su `-checklinkname=0`, que
`fyne release` pisaría con su `-ldflags -w`— y aquí se cambia el ENTERO del atributo
en el manifiesto binario, en el sitio: mismo tamaño, ningún otro byte se mueve.
El mínimo se cambia por el mismo camino: la protección automática de Play activada
para la suite exige API 24, y la CLI deja minSdkVersion en 21.

`servicio-en-primer-plano` añade `android:foregroundServiceType` al `<service>` de esa
clase. La CLI no sabe escribirlo (su tabla de atributos es anterior a la API 29) y con
target 34 o más Android mata el servicio sin él. Aquí sí se mueven bytes: la cadena va al
FINAL del pool (las demás no cambian de índice), el mapa de recursos se alarga con ceros
hasta ella —un 0 es «sin recurso», como cualquier cadena que no es un atributo— y el
atributo entra en su sitio por identificador, que es como Android los recorre.

`imagen-en-imagen` añade `android:supportsPictureInPicture="true"` a la <activity> por el
mismo camino (la CLI tampoco conoce ese atributo, de la API 24): sin él Android lanza
IllegalStateException al pedir la imagen dentro de imagen.

`alineacion` comprueba que cada PT_LOAD de la .so esté alineado a 16 KiB, que es lo
que Play exige a las apps con target 35 o más (páginas de 16 KiB de Android 15).
"""
import struct
import sys

TARGET_SDK = 0x01010270  # android:targetSdkVersion
MIN_SDK = 0x0101020C  # android:minSdkVersion
TIPO_ENTERO = 0x10  # Res_value::TYPE_INT_DEC
TIPO_HEX = 0x11  # Res_value::TYPE_INT_HEX, el de los atributos de banderas
NOMBRE = 0x01010003  # android:name
TIPO_SERVICIO = 0x01010599  # android:foregroundServiceType (API 29)
PICTURE_IN_PICTURE = 0x010104F7  # android:supportsPictureInPicture (API 24)
TIPO_BOOL = 0x12  # Res_value::TYPE_INT_BOOLEAN


def _cadenas(datos, off):
    """Tabla de cadenas (ResStringPool) empezando en `off`."""
    _, cab, _ = struct.unpack_from("<HHI", datos, off)
    n, _, flags, inicio, _ = struct.unpack_from("<IIIII", datos, off + 8)
    utf8 = flags & 0x100
    offs = struct.unpack_from("<%dI" % n, datos, off + cab)
    salida = []
    for o in offs:
        p = off + inicio + o
        if utf8:
            # longitud en caracteres y luego en bytes, cada una de 1 o 2 bytes
            for _ in range(2):
                largo = datos[p]
                p += 1
                if largo & 0x80:
                    largo = ((largo & 0x7F) << 8) | datos[p]
                    p += 1
            salida.append(datos[p : p + largo].decode("utf-8", "replace"))
        else:
            largo = struct.unpack_from("<H", datos, p)[0]
            p += 2
            if largo & 0x8000:
                largo = ((largo & 0x7FFF) << 16) | struct.unpack_from("<H", datos, p)[0]
                p += 2
            salida.append(datos[p : p + 2 * largo].decode("utf-16-le", "replace"))
    return salida


def _atributo_sdk(datos, recurso, nombre_sdk):
    """Posición del atributo SDK solicitado dentro de <uses-sdk>."""
    tipo, cab, _ = struct.unpack_from("<HHI", datos, 0)
    if tipo != 0x0003:
        raise SystemExit("no es un XML binario de Android (tipo 0x%04x)" % tipo)
    cadenas, ids, encontrados = [], [], []
    off = cab
    while off < len(datos):
        tipo, cab, largo = struct.unpack_from("<HHI", datos, off)
        if largo < 8:
            raise SystemExit("trozo de %d bytes en %d: fichero corrupto" % (largo, off))
        if tipo == 0x0001:
            cadenas = _cadenas(datos, off)
        elif tipo == 0x0180:
            ids = struct.unpack_from("<%dI" % ((largo - cab) // 4), datos, off + cab)
        elif tipo == 0x0102:
            ext = off + cab
            _, nombre, a_ini, a_tam, a_num = struct.unpack_from("<IIHHH", datos, ext)
            if cadenas[nombre] == "uses-sdk":
                for i in range(a_num):
                    a = ext + a_ini + i * a_tam
                    _, a_nombre, _, _, _, a_tipo, _ = struct.unpack_from("<IIIHBBI", datos, a)
                    if a_nombre < len(ids) and ids[a_nombre] == recurso:
                        encontrados.append((a, a_tipo))
        off += largo
    if len(encontrados) != 1:
        raise SystemExit("se esperaba un %s en <uses-sdk> y hay %d" % (nombre_sdk, len(encontrados)))
    return encontrados[0]


def poner_sdk(ruta, nivel, recurso, nombre_sdk):
    with open(ruta, "rb") as f:
        datos = bytearray(f.read())
    a, a_tipo = _atributo_sdk(datos, recurso, nombre_sdk)
    if a_tipo != TIPO_ENTERO:
        raise SystemExit("%s no es un entero (tipo 0x%02x)" % (nombre_sdk, a_tipo))
    antes = struct.unpack_from("<I", datos, a + 16)[0]
    # rawValue a «sin cadena» y el dato al nivel pedido: los dos campos de 4 bytes.
    struct.pack_into("<I", datos, a + 8, 0xFFFFFFFF)
    struct.pack_into("<I", datos, a + 16, nivel)
    with open(ruta, "wb") as f:
        f.write(datos)
    print("%s %d -> %d" % (nombre_sdk, antes, nivel))


def leer_sdk(ruta, recurso, nombre_sdk):
    with open(ruta, "rb") as f:
        datos = f.read()
    a, _ = _atributo_sdk(datos, recurso, nombre_sdk)
    print(struct.unpack_from("<I", datos, a + 16)[0])


def _trozos(datos):
    """(posición, tipo, cabecera, largo) de cada trozo tras la cabecera del documento."""
    tipo, cab, total = struct.unpack_from("<HHI", datos, 0)
    if tipo != 0x0003 or total != len(datos):
        raise SystemExit("no es un XML binario de Android")
    off = cab
    while off < len(datos):
        tipo, cab, largo = struct.unpack_from("<HHI", datos, off)
        if largo < 8 or off + largo > len(datos):
            raise SystemExit("trozo de %d bytes en %d: fichero corrupto" % (largo, off))
        yield off, tipo, cab, largo
        off += largo


def _codificar(texto, utf8):
    if utf8:
        b = texto.encode("utf-8")
        if len(texto) > 0x7F or len(b) > 0x7F:
            raise SystemExit("cadena demasiado larga para este apaño: %r" % texto)
        return bytes([len(texto), len(b)]) + b + b"\0"
    return struct.pack("<H", len(texto)) + texto.encode("utf-16-le") + b"\0\0"


def servicio_en_primer_plano(ruta, clase, banderas):
    """android:foregroundServiceType del <service> de `clase`."""
    def es_el(cadenas, datos, ext, a_ini, a_num, resid):
        for k in range(a_num):
            _, a_nombre, a_crudo = struct.unpack_from("<III", datos, ext + a_ini + k * 20)
            if resid(a_nombre) == NOMBRE and a_crudo < len(cadenas) and cadenas[a_crudo] == clase:
                return True
        return False
    _anadir_atributo(ruta, "service", es_el, "<service android:name=\"%s\">" % clase,
                     "foregroundServiceType", TIPO_SERVICIO, TIPO_HEX, banderas)
    print("%s: foregroundServiceType 0x%x" % (clase, banderas))


def imagen_en_imagen(ruta):
    """android:supportsPictureInPicture="true" en la <activity> (solo hay una)."""
    _anadir_atributo(ruta, "activity", lambda *_: True, "<activity>",
                     "supportsPictureInPicture", PICTURE_IN_PICTURE, TIPO_BOOL, 0xFFFFFFFF)
    print("activity: supportsPictureInPicture")


def _anadir_atributo(ruta, etiqueta, es_el, descripcion, nombre_attr, id_attr, tipo, dato):
    """Añade el atributo `id_attr` al único elemento `etiqueta` que cumple `es_el`."""
    with open(ruta, "rb") as f:
        datos = f.read()
    trozos = list(_trozos(datos))
    pool = [t for t in trozos if t[1] == 0x0001]
    mapa = [t for t in trozos if t[1] == 0x0180]
    if len(pool) != 1 or len(mapa) != 1:
        raise SystemExit("se esperaba un pool de cadenas y un mapa de recursos")
    p_off, _, p_cab, p_largo = pool[0]
    n, estilos, flags, inicio, _ = struct.unpack_from("<IIIII", datos, p_off + 8)
    if estilos:
        raise SystemExit("el pool trae estilos: este apaño no los sabe mover")
    cadenas = _cadenas(datos, p_off)
    m_off, _, m_cab, m_largo = mapa[0]
    ids = list(struct.unpack_from("<%dI" % ((m_largo - m_cab) // 4), datos, m_off + m_cab))

    def resid(i):
        return ids[i] if i < len(ids) else 0

    destino = []
    for off, tipo_t, cab, largo in trozos:
        if tipo_t != 0x0102:
            continue
        ext = off + cab
        _, nombre, a_ini, a_tam, a_num = struct.unpack_from("<IIHHH", datos, ext)
        if cadenas[nombre] != etiqueta or a_tam != 20 or not es_el(cadenas, datos, ext, a_ini, a_num, resid):
            continue
        ns = 0xFFFFFFFF
        for k in range(a_num):
            a_ns = struct.unpack_from("<I", datos, ext + a_ini + k * 20)[0]
            if a_ns != 0xFFFFFFFF:
                ns = a_ns
                break
        destino.append((off, ext, a_ini, a_tam, a_num, ns))
    if len(destino) != 1:
        raise SystemExit("se esperaba un %s y hay %d" % (descripcion, len(destino)))
    e_off, ext, a_ini, a_tam, a_num, ns = destino[0]
    for k in range(a_num):
        if resid(struct.unpack_from("<I", datos, ext + a_ini + k * 20 + 4)[0]) == id_attr:
            raise SystemExit("%s ya lleva %s" % (descripcion, nombre_attr))

    # La cadena: la que ya puso otro elemento de este manifiesto, o una nueva al final.
    ya = [k for k, c in enumerate(cadenas) if c == nombre_attr and resid(k) == id_attr]
    if ya:
        nueva, pool_nuevo = ya[0], datos[p_off : p_off + p_largo]
    else:
        nueva = n
        offs = list(struct.unpack_from("<%dI" % n, datos, p_off + p_cab))
        textos = datos[p_off + inicio : p_off + p_largo]
        offs.append(len(textos))
        textos += _codificar(nombre_attr, flags & 0x100)
        textos += b"\0" * (-len(textos) % 4)
        inicio_nuevo = p_cab + 4 * (n + 1)
        cabecera = bytearray(datos[p_off : p_off + p_cab])
        struct.pack_into("<HHIIIIII", cabecera, 0, 0x0001, p_cab, inicio_nuevo + len(textos), n + 1, 0, flags, inicio_nuevo, 0)
        pool_nuevo = bytes(cabecera) + struct.pack("<%dI" % (n + 1), *offs) + textos
        # El mapa, hasta la cadena nueva.
        ids += [0] * (nueva - len(ids)) + [id_attr]
    mapa_nuevo = struct.pack("<HHI", 0x0180, 8, 8 + 4 * len(ids)) + struct.pack("<%dI" % len(ids), *ids)

    # El elemento, con el atributo en su sitio por identificador.
    atrs = [datos[ext + a_ini + k * 20 : ext + a_ini + (k + 1) * 20] for k in range(a_num)]
    pos = next((k for k, a in enumerate(atrs) if resid(struct.unpack_from("<I", a, 4)[0]) > id_attr), a_num)
    atrs.insert(pos, struct.pack("<IIIHBBI", ns, nueva, 0xFFFFFFFF, 8, 0, tipo, dato))
    _, _, largo_e = struct.unpack_from("<HHI", datos, e_off)
    elem = bytearray(datos[e_off : ext + a_ini]) + b"".join(atrs) + datos[ext + a_ini + a_num * 20 : e_off + largo_e]
    struct.pack_into("<I", elem, 4, largo_e + 20)
    rel = ext - e_off
    ind = list(struct.unpack_from("<HHH", elem, rel + 14))  # id, class, style: base 1
    ind = [v + 1 if v > pos else v for v in ind]
    struct.pack_into("<HHHH", elem, rel + 12, a_num + 1, *ind)

    cuerpo = b""
    for off, tipo_t, cab, largo in trozos:
        if off == p_off:
            cuerpo += pool_nuevo
        elif off == m_off:
            cuerpo += mapa_nuevo
        elif off == e_off:
            cuerpo += bytes(elem)
        else:
            cuerpo += datos[off : off + largo]
    _, cab_doc, _ = struct.unpack_from("<HHI", datos, 0)
    salida = struct.pack("<HHI", 0x0003, cab_doc, cab_doc + len(cuerpo)) + datos[8:cab_doc] + cuerpo
    with open(ruta, "wb") as f:
        f.write(salida)


def leer_imagen_en_imagen(ruta):
    """Imprime `true` si la <activity> lleva supportsPictureInPicture a true."""
    with open(ruta, "rb") as f:
        datos = f.read()
    cadenas, ids = [], []
    for off, tipo, cab, largo in _trozos(datos):
        if tipo == 0x0001:
            cadenas = _cadenas(datos, off)
        elif tipo == 0x0180:
            ids = struct.unpack_from("<%dI" % ((largo - cab) // 4), datos, off + cab)
        elif tipo == 0x0102:
            ext = off + cab
            _, nombre, a_ini, a_tam, a_num = struct.unpack_from("<IIHHH", datos, ext)
            if cadenas[nombre] != "activity":
                continue
            for k in range(a_num):
                _, a_nombre, _, _, _, a_tipo, dato = struct.unpack_from("<IIIHBBI", datos, ext + a_ini + k * a_tam)
                if (ids[a_nombre] if a_nombre < len(ids) else 0) == PICTURE_IN_PICTURE and a_tipo == TIPO_BOOL and dato:
                    print("true")


def leer_servicio(ruta, clase):
    """Las banderas de foregroundServiceType del <service> de `clase`, o nada."""
    with open(ruta, "rb") as f:
        datos = f.read()
    cadenas, ids = [], []
    for off, tipo, cab, largo in _trozos(datos):
        if tipo == 0x0001:
            cadenas = _cadenas(datos, off)
        elif tipo == 0x0180:
            ids = struct.unpack_from("<%dI" % ((largo - cab) // 4), datos, off + cab)
        elif tipo == 0x0102:
            ext = off + cab
            _, nombre, a_ini, a_tam, a_num = struct.unpack_from("<IIHHH", datos, ext)
            if cadenas[nombre] != "service":
                continue
            vistos, anterior = {}, 0
            for i in range(a_num):
                _, a_nombre, a_crudo, _, _, _, dato = struct.unpack_from("<IIIHBBI", datos, ext + a_ini + i * a_tam)
                r = ids[a_nombre] if a_nombre < len(ids) else 0
                if r and r < anterior:
                    raise SystemExit("atributos del <service> desordenados: Android no vería alguno")
                anterior = max(anterior, r)
                vistos[r] = cadenas[a_crudo] if a_crudo < len(cadenas) else dato
            if vistos.get(NOMBRE) == clase and TIPO_SERVICIO in vistos:
                print("0x%x" % vistos[TIPO_SERVICIO])


def alineacion(ruta, minimo):
    with open(ruta, "rb") as f:
        datos = f.read()
    if datos[:4] != b"\x7fELF" or datos[4] != 2:
        raise SystemExit("%s no es un ELF de 64 bits" % ruta)
    phoff = struct.unpack_from("<Q", datos, 0x20)[0]
    phtam, phnum = struct.unpack_from("<HH", datos, 0x36)
    cargas = 0
    for i in range(phnum):
        p = phoff + i * phtam
        if struct.unpack_from("<I", datos, p)[0] != 1:  # PT_LOAD
            continue
        cargas += 1
        alin = struct.unpack_from("<Q", datos, p + 48)[0]
        if alin < minimo:
            raise SystemExit("%s: PT_LOAD alineado a %d y Play exige %d" % (ruta, alin, minimo))
    if not cargas:
        raise SystemExit("%s no tiene ningún PT_LOAD" % ruta)
    print("%d PT_LOAD alineados a >= %d" % (cargas, minimo))


if __name__ == "__main__":
    orden = sys.argv[1:2]
    if orden == ["target-sdk"] and len(sys.argv) == 4:
        poner_sdk(sys.argv[2], int(sys.argv[3]), TARGET_SDK, "targetSdkVersion")
    elif orden == ["leer-target-sdk"] and len(sys.argv) == 3:
        leer_sdk(sys.argv[2], TARGET_SDK, "targetSdkVersion")
    elif orden == ["min-sdk"] and len(sys.argv) == 4:
        poner_sdk(sys.argv[2], int(sys.argv[3]), MIN_SDK, "minSdkVersion")
    elif orden == ["leer-min-sdk"] and len(sys.argv) == 3:
        leer_sdk(sys.argv[2], MIN_SDK, "minSdkVersion")
    elif orden == ["servicio-en-primer-plano"] and len(sys.argv) == 5:
        servicio_en_primer_plano(sys.argv[2], sys.argv[3], int(sys.argv[4], 0))
    elif orden == ["imagen-en-imagen"] and len(sys.argv) == 3:
        imagen_en_imagen(sys.argv[2])
    elif orden == ["leer-imagen-en-imagen"] and len(sys.argv) == 3:
        leer_imagen_en_imagen(sys.argv[2])
    elif orden == ["leer-servicio"] and len(sys.argv) == 4:
        leer_servicio(sys.argv[2], sys.argv[3])
    elif orden == ["alineacion"] and len(sys.argv) == 4:
        alineacion(sys.argv[2], int(sys.argv[3]))
    else:
        sys.exit(__doc__)

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package discocifrado

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
)

// # Linux
//
// 1. El montaje de la carpeta: el de `/proc/self/mountinfo` cuyo punto de montaje es el
//    prefijo más largo de la ruta (el último, si hay varios en el mismo sitio).
// 2. Lo que se sabe por el sistema de ficheros: eCryptfs y los FUSE que cifran son
//    cifrado; ZFS se pregunta (`zfs get encryption`); la red, la memoria y las capas
//    (NFS, SMB, tmpfs, overlay…) no se saben desde aquí.
// 3. fscrypt (ext4, f2fs): la propia carpeta lleva el atributo «cifrado» de `statx`.
// 4. El dispositivo en `/sys`: `/sys/dev/block/MAYOR:MENOR` (o, en btrfs, que da un
//    número anónimo, el de su origen `/dev/…`). Un `dm-crypt` es un `dm` cuyo
//    `dm/uuid` empieza por `CRYPT-` (`CRYPT-LUKS2-…` con LUKS); si no lo es, se miran
//    sus `slaves` hacia abajo (un LVM sobre LUKS). Un dispositivo de bloques de verdad
//    sin `dm-crypt` en la cadena está sin cifrar.
// 5. Sin `/sys` que leer, `lsblk -s` (la cadena hacia abajo): un `crypt` es cifrado.
//
// No se distingue un disco con cifrado por hardware (OPAL): sin `dm-crypt` se dice «sin
// cifrar», que es lo que ve el sistema.

// entornoLinux es lo que se lee del sistema; las pruebas lo dan grabado.
type entornoLinux struct {
	// leer lee un fichero (de /proc o de /sys), siguiendo enlaces.
	leer func(ruta string) ([]byte, error)
	// listar da los nombres de una carpeta (los `slaves` de un dispositivo).
	listar func(ruta string) ([]string, error)
	// resolver sigue los enlaces de una ruta (`/dev/mapper/raiz` → `/dev/dm-0`).
	resolver func(ruta string) (string, error)
	// fscrypt dice si la carpeta está cifrada con fscrypt; sabido es falso si el sistema
	// de ficheros no lo dice.
	fscrypt func(ruta string) (cifrada, sabido bool)
	orden   orden
}

// montaje es una línea de mountinfo.
type montaje struct {
	mayor, menor int
	punto        string
	tipo         string
	origen       string
}

// leerMountinfo lee las líneas de `/proc/self/mountinfo` (proc(5)):
//
//	36 35 98:0 /mnt1 /mnt2 rw,noatime master:1 - ext3 /dev/root rw,errors=continue
//
// El separador « - » parte los campos opcionales del tipo y el origen.
func leerMountinfo(texto string) []montaje {
	var out []montaje
	for linea := range strings.SplitSeq(texto, "\n") {
		campos := strings.Fields(linea)
		guion := -1
		for i, c := range campos {
			if c == "-" && i >= 6 {
				guion = i
				break
			}
		}
		if guion < 0 || len(campos) < guion+3 {
			continue
		}
		mayor, menor, ok := strings.Cut(campos[2], ":")
		if !ok {
			continue
		}
		ma, e1 := strconv.Atoi(mayor)
		me, e2 := strconv.Atoi(menor)
		if e1 != nil || e2 != nil {
			continue
		}
		out = append(out, montaje{mayor: ma, menor: me, punto: desescapar(campos[4]), tipo: campos[guion+1], origen: desescapar(campos[guion+2])})
	}
	return out
}

// desescapar deshace los `\040` (espacio), `\011`, `\012` y `\134` de mountinfo.
func desescapar(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+4 <= len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// dentro dice si ruta está en el punto de montaje (o es él).
func dentro(ruta, punto string) bool {
	if punto == "/" {
		return strings.HasPrefix(ruta, "/")
	}
	return ruta == punto || strings.HasPrefix(ruta, punto+"/")
}

// montajeDe es el montaje de ruta: el de punto más largo y, a igualdad, el último.
func montajeDe(ms []montaje, ruta string) (montaje, bool) {
	var mejor montaje
	hay := false
	for _, m := range ms {
		if dentro(ruta, m.punto) && (!hay || len(m.punto) >= len(mejor.punto)) {
			mejor, hay = m, true
		}
	}
	return mejor, hay
}

// fuseQueCifran son los FUSE cuyo tipo ya dice que cifran (`fuse.gocryptfs`…).
var fuseQueCifran = map[string]string{"gocryptfs": "gocryptfs", "cryfs": "CryFS", "encfs": "EncFS", "securefs": "securefs"}

// sinDisco son los sistemas de ficheros que no están en un disco de este equipo o que
// no se pueden seguir hasta él: lo que digan no se sabe desde aquí.
var sinDisco = map[string]bool{
	"nfs": true, "nfs4": true, "cifs": true, "smb3": true, "smbfs": true, "9p": true, "virtiofs": true,
	"tmpfs": true, "ramfs": true, "overlay": true, "aufs": true, "devtmpfs": true, "proc": true, "sysfs": true,
	"afs": true, "ceph": true, "glusterfs": true, "lustre": true, "davfs": true,
}

func deLinux(ctx context.Context, ruta string, e entornoLinux) Resultado {
	ruta = filepath.Clean(ruta)
	texto, err := e.leer("/proc/self/mountinfo")
	if err != nil {
		return noSeSabe
	}
	m, ok := montajeDe(leerMountinfo(string(texto)), ruta)
	if !ok {
		return noSeSabe
	}
	tipo := m.tipo
	switch {
	case tipo == "ecryptfs":
		return cifradoCon("eCryptfs")
	case strings.HasPrefix(tipo, "fuse."):
		if como, ok := fuseQueCifran[strings.TrimPrefix(tipo, "fuse.")]; ok {
			return cifradoCon(como)
		}
		return noSeSabe
	case tipo == "fuse" || tipo == "fuseblk":
		// NTFS-3G y otros sobre un disco: el tipo no dice cuál; se sigue al dispositivo.
	case tipo == "zfs":
		return zfs(ctx, e.orden, m.origen)
	case sinDisco[tipo]:
		return noSeSabe
	}
	if e.fscrypt != nil {
		if cifrada, sabido := e.fscrypt(ruta); sabido && cifrada {
			return cifradoCon("fscrypt")
		}
	}
	if r, ok := porSys(e, m); ok {
		return r
	}
	return porLsblk(ctx, e.orden, m.origen)
}

// zfs pregunta a ZFS por el cifrado nativo del conjunto de datos.
func zfs(ctx context.Context, o orden, conjunto string) Resultado {
	if o == nil || conjunto == "" || strings.HasPrefix(conjunto, "-") {
		return noSeSabe
	}
	salida, err := o(ctx, "zfs", "get", "-H", "-o", "value", "encryption", conjunto)
	if err != nil {
		return noSeSabe
	}
	switch v := strings.TrimSpace(salida); {
	case v == "off":
		return sinCifrar
	case v == "" || v == "-" || strings.ContainsAny(v, " \n"):
		return noSeSabe
	}
	return cifradoCon("ZFS")
}

// porSys sigue el dispositivo del montaje en /sys. ok es falso si no hay /sys que leer
// o el dispositivo no está en él.
func porSys(e entornoLinux, m montaje) (Resultado, bool) {
	var nodo string
	switch {
	case m.mayor != 0:
		nodo = "/sys/dev/block/" + strconv.Itoa(m.mayor) + ":" + strconv.Itoa(m.menor)
	case strings.HasPrefix(m.origen, "/dev/") && e.resolver != nil:
		// btrfs da un número anónimo (0:NN): el dispositivo es el de su origen.
		dev, err := e.resolver(m.origen)
		if err != nil {
			return noSeSabe, false
		}
		nodo = "/sys/class/block/" + filepath.Base(dev)
	default:
		return noSeSabe, false
	}
	if _, err := e.leer(nodo + "/dev"); err != nil {
		return noSeSabe, false
	}
	if como, ok := crypt(e, nodo, 0); ok {
		return cifradoCon(como), true
	}
	if lazo(e, nodo) {
		// Un fichero montado como disco: lo que valga es el disco del fichero, que no se
		// sigue desde aquí.
		return noSeSabe, true
	}
	return sinCifrar, true
}

// crypt busca un dm-crypt en nodo o por debajo (sus `slaves`).
func crypt(e entornoLinux, nodo string, hondo int) (string, bool) {
	if hondo > 8 {
		return "", false
	}
	if uuid, err := e.leer(nodo + "/dm/uuid"); err == nil {
		if u := strings.TrimSpace(string(uuid)); strings.HasPrefix(u, "CRYPT-") {
			if strings.HasPrefix(u, "CRYPT-LUKS") {
				return "LUKS", true
			}
			return "dm-crypt", true
		}
	}
	if e.listar == nil {
		return "", false
	}
	debajo, _ := e.listar(nodo + "/slaves")
	for _, d := range debajo {
		if como, ok := crypt(e, "/sys/class/block/"+d, hondo+1); ok {
			return como, true
		}
	}
	return "", false
}

// lazo dice si nodo es un dispositivo de lazo (un fichero montado como disco) sin cifrar
// por encima.
func lazo(e entornoLinux, nodo string) bool {
	_, err := e.leer(nodo + "/loop/backing_file")
	return err == nil
}

// porLsblk pregunta a `lsblk -s` por la cadena del origen hacia abajo.
func porLsblk(ctx context.Context, o orden, origen string) Resultado {
	if o == nil || !strings.HasPrefix(origen, "/dev/") {
		return noSeSabe
	}
	salida, err := o(ctx, "lsblk", "-s", "-n", "-r", "-o", "TYPE", origen)
	if err != nil {
		return noSeSabe
	}
	return leerLsblk(salida)
}

// leerLsblk lee los TIPOS de la cadena (`crypt`, `lvm`, `part`, `disk`…).
func leerLsblk(salida string) Resultado {
	tipos := strings.Fields(salida)
	if len(tipos) == 0 {
		return noSeSabe
	}
	for _, t := range tipos {
		if t == "crypt" {
			return cifradoCon("dm-crypt")
		}
		if t == "loop" {
			return noSeSabe
		}
	}
	return sinCifrar
}

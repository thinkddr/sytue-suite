// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package discocifrado

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"strings"
	"testing"
)

// sysFalso es un /proc y un /sys de mentira: ficheros, carpetas y enlaces.
type sysFalso struct {
	ficheros map[string]string
	enlaces  map[string]string
}

// resolver sigue los enlaces de la primera parte de la ruta que lo sea
// (`/sys/dev/block/254:1` → `/sys/devices/virtual/block/dm-1`).
func (s sysFalso) resolver(r string) (string, error) {
	for i := 0; i < 10; i++ {
		cambiado := false
		for de, a := range s.enlaces {
			if r == de || strings.HasPrefix(r, de+"/") {
				r = a + strings.TrimPrefix(r, de)
				cambiado = true
			}
		}
		if !cambiado {
			return r, nil
		}
	}
	return "", errors.New("demasiados enlaces")
}

func (s sysFalso) leer(r string) ([]byte, error) {
	r, _ = s.resolver(r)
	if t, ok := s.ficheros[r]; ok {
		return []byte(t), nil
	}
	return nil, fs.ErrNotExist
}

func (s sysFalso) listar(dir string) ([]string, error) {
	dir, _ = s.resolver(dir)
	vistos := map[string]bool{}
	var out []string
	for f := range s.ficheros {
		if rel, ok := strings.CutPrefix(f, dir+"/"); ok {
			n, _, _ := strings.Cut(rel, "/")
			if !vistos[n] {
				vistos[n] = true
				out = append(out, n)
			}
		}
	}
	for e := range s.enlaces {
		if path.Dir(e) == dir && !vistos[path.Base(e)] {
			out = append(out, path.Base(e))
		}
	}
	if len(out) == 0 {
		return nil, fs.ErrNotExist
	}
	return out, nil
}

// ordenes son las salidas grabadas, por la orden entera.
type ordenes map[string]string

func (o ordenes) orden(_ context.Context, nombre string, args ...string) (string, error) {
	clave := strings.Join(append([]string{nombre}, args...), " ")
	if s, ok := o[clave]; ok {
		if strings.HasPrefix(s, "ERROR:") {
			return strings.TrimPrefix(s, "ERROR:"), errors.New("exit status 1")
		}
		return s, nil
	}
	return "", errors.New("orden desconocida: " + clave)
}

// Un mountinfo de un portátil con Fedora: btrfs sobre LUKS para / y /home, una partición
// ext4 sin cifrar para /datos, un USB en exFAT, un NFS, un tmpfs, una carpeta con
// espacio, un ZFS y un gocryptfs.
const mountinfo = `22 1 0:21 / / rw,relatime shared:1 - btrfs /dev/mapper/luks-1b2c rw,seclabel,compress=zstd:1,ssd,subvolid=257,subvol=/root
23 22 0:5 / /proc rw,nosuid,nodev,noexec,relatime shared:12 - proc proc rw
24 22 0:21 /home /home rw,relatime shared:2 - btrfs /dev/mapper/luks-1b2c rw,seclabel,compress=zstd:1,ssd,subvolid=256,subvol=/home
30 22 259:3 / /datos rw,relatime shared:5 - ext4 /dev/nvme0n1p3 rw,seclabel
31 22 8:1 / /run/media/ana/USB rw,nosuid,nodev,relatime shared:6 - exfat /dev/sda1 rw,uid=1000
32 22 0:55 / /mnt/nas rw,relatime shared:7 - nfs4 nas:/export rw,vers=4.2
33 22 0:33 / /tmp rw,nosuid,nodev shared:8 - tmpfs tmpfs rw,seclabel
34 30 259:3 /mis\040cosas /home/ana/Mis\040cosas rw,relatime shared:9 - ext4 /dev/nvme0n1p3 rw
35 22 0:60 / /tanque rw,xattr shared:10 - zfs tanque/datos rw
36 24 0:61 / /home/ana/Privado rw,nosuid,nodev,relatime shared:11 - fuse.gocryptfs /home/ana/.privado rw,user_id=1000
37 22 254:2 / /lvm rw,relatime shared:12 - ext4 /dev/mapper/vg-datos rw
38 22 7:0 / /imagen rw,relatime shared:13 - ext4 /dev/loop0 rw
`

func sysDeEjemplo() sysFalso {
	return sysFalso{
		ficheros: map[string]string{
			"/proc/self/mountinfo": mountinfo,
			// dm-0: LUKS2 sobre nvme0n1p2.
			"/sys/devices/virtual/block/dm-0/dev":                "253:0\n",
			"/sys/devices/virtual/block/dm-0/dm/uuid":            "CRYPT-LUKS2-1b2c3d4e5f-luks-1b2c\n",
			"/sys/devices/virtual/block/dm-0/slaves/nvme0n1p2/x": "",
			"/sys/devices/pci/nvme0n1/nvme0n1p2/dev":             "259:2\n",
			"/sys/devices/pci/nvme0n1/nvme0n1p3/dev":             "259:3\n",
			// dm-2: un volumen LVM sobre dm-1, que es LUKS1.
			"/sys/devices/virtual/block/dm-2/dev":                "254:2\n",
			"/sys/devices/virtual/block/dm-2/dm/uuid":            "LVM-abcdefgh\n",
			"/sys/devices/virtual/block/dm-1/dev":                "254:1\n",
			"/sys/devices/virtual/block/dm-1/dm/uuid":            "CRYPT-LUKS1-aaaa-cripta\n",
			"/sys/devices/pci/usb/sda/sda1/dev":                  "8:1\n",
			"/sys/devices/virtual/block/loop0/dev":               "7:0\n",
			"/sys/devices/virtual/block/loop0/loop/backing_file": "/home/ana/disco.img\n",
		},
		enlaces: map[string]string{
			"/dev/mapper/luks-1b2c":                       "/dev/dm-0",
			"/sys/class/block/dm-0":                       "/sys/devices/virtual/block/dm-0",
			"/sys/class/block/dm-1":                       "/sys/devices/virtual/block/dm-1",
			"/sys/class/block/dm-2":                       "/sys/devices/virtual/block/dm-2",
			"/sys/devices/virtual/block/dm-2/slaves/dm-1": "/sys/devices/virtual/block/dm-1",
			"/sys/dev/block/254:2":                        "/sys/devices/virtual/block/dm-2",
			"/sys/dev/block/259:3":                        "/sys/devices/pci/nvme0n1/nvme0n1p3",
			"/sys/dev/block/8:1":                          "/sys/devices/pci/usb/sda/sda1",
			"/sys/dev/block/7:0":                          "/sys/devices/virtual/block/loop0",
		},
	}
}

func entornoDe(s sysFalso, o ordenes) entornoLinux {
	return entornoLinux{leer: s.leer, listar: s.listar, resolver: s.resolver, orden: o.orden}
}

// Linux: cada carpeta, por su montaje y su dispositivo.
func TestLinux(t *testing.T) {
	s := sysDeEjemplo()
	o := ordenes{"zfs get -H -o value encryption tanque/datos": "aes-256-gcm\n"}
	for ruta, quiero := range map[string]Resultado{
		// btrfs (número anónimo 0:21) sobre /dev/mapper/luks-… → dm-0, LUKS.
		"/home/ana/Sytue Office": cifradoCon("LUKS"),
		"/":                      cifradoCon("LUKS"),
		// Una partición sin dm-crypt.
		"/datos/Sytue Office": sinCifrar,
		// El punto con espacio (`\040`) y montado dentro de /home: manda el más largo.
		"/home/ana/Mis cosas/Sytue Office": sinCifrar,
		"/run/media/ana/USB/Sytue Office":  sinCifrar,
		// Red y memoria: no se sabe.
		"/mnt/nas/Sytue Office": noSeSabe,
		"/tmp/x":                noSeSabe,
		"/tanque/Sytue Office":  cifradoCon("ZFS"),
		// Un FUSE que cifra.
		"/home/ana/Privado/Sytue Office": cifradoCon("gocryptfs"),
		// LVM sobre LUKS: el dm-crypt está debajo.
		"/lvm/Sytue Office": cifradoCon("LUKS"),
		// Un fichero montado como disco: lo que vale es el disco del fichero.
		"/imagen/x": noSeSabe,
	} {
		if got := deLinux(context.Background(), ruta, entornoDe(s, o)); got != quiero {
			t.Errorf("%s: %+v; quiero %+v", ruta, got, quiero)
		}
	}
}

// ZFS sin cifrado nativo, o sin la orden zfs.
func TestLinuxZFS(t *testing.T) {
	s := sysDeEjemplo()
	if got := deLinux(context.Background(), "/tanque/x", entornoDe(s, ordenes{"zfs get -H -o value encryption tanque/datos": "off\n"})); got != sinCifrar {
		t.Errorf("zfs off: %+v", got)
	}
	if got := deLinux(context.Background(), "/tanque/x", entornoDe(s, ordenes{})); got != noSeSabe {
		t.Errorf("sin zfs: %+v", got)
	}
}

// fscrypt manda sobre el dispositivo: una carpeta cifrada en una partición sin cifrar.
func TestLinuxFscrypt(t *testing.T) {
	e := entornoDe(sysDeEjemplo(), ordenes{})
	e.fscrypt = func(ruta string) (bool, bool) { return strings.HasPrefix(ruta, "/datos/cifrada"), true }
	if got := deLinux(context.Background(), "/datos/cifrada/Sytue Office", e); got != cifradoCon("fscrypt") {
		t.Errorf("fscrypt: %+v", got)
	}
	if got := deLinux(context.Background(), "/datos/otra", e); got != sinCifrar {
		t.Errorf("sin fscrypt: %+v", got)
	}
}

// eCryptfs (la carpeta personal cifrada de Ubuntu antiguo).
func TestLinuxEcryptfs(t *testing.T) {
	s := sysFalso{ficheros: map[string]string{"/proc/self/mountinfo": "40 22 0:50 / /home/ana rw - ecryptfs /home/.ecryptfs/ana/.Private rw,ecryptfs_sig=abcd\n22 1 8:2 / / rw - ext4 /dev/sda2 rw\n"}}
	if got := deLinux(context.Background(), "/home/ana/Sytue Office", entornoDe(s, ordenes{})); got != cifradoCon("eCryptfs") {
		t.Errorf("ecryptfs: %+v", got)
	}
}

// Sin /sys (un contenedor que no lo monta), se pregunta a lsblk; sin él, no se sabe.
func TestLinuxSinSys(t *testing.T) {
	s := sysFalso{ficheros: map[string]string{"/proc/self/mountinfo": "22 1 253:1 / / rw - xfs /dev/mapper/raiz rw\n"}}
	for salida, quiero := range map[string]Resultado{
		"lvm\ncrypt\npart\ndisk\n": cifradoCon("dm-crypt"),
		"part\ndisk\n":             sinCifrar,
		"":                         noSeSabe,
		"ERROR:lsblk: /dev/mapper/raiz: not a block device": noSeSabe,
	} {
		o := ordenes{"lsblk -s -n -r -o TYPE /dev/mapper/raiz": salida}
		if got := deLinux(context.Background(), "/home/ana", entornoDe(s, o)); got != quiero {
			t.Errorf("lsblk %q: %+v; quiero %+v", salida, got, quiero)
		}
	}
	// Ni mountinfo.
	if got := deLinux(context.Background(), "/home/ana", entornoDe(sysFalso{}, ordenes{})); got != noSeSabe {
		t.Errorf("sin /proc: %+v", got)
	}
}

func TestLeerMountinfo(t *testing.T) {
	ms := leerMountinfo(mountinfo + "basura\n\n1 2 x:y / / rw - ext4 /dev/a rw\n")
	if len(ms) != 12 {
		t.Fatalf("%d montajes: %+v", len(ms), ms)
	}
	if m := ms[7]; m.punto != "/home/ana/Mis cosas" || m.mayor != 259 || m.menor != 3 || m.tipo != "ext4" || m.origen != "/dev/nvme0n1p3" {
		t.Errorf("con espacio: %+v", m)
	}
	if desescapar(`a\134b\011c\`) != "a\\b\tc\\" {
		t.Errorf("desescapar: %q", desescapar(`a\134b\011c\`))
	}
	if !dentro("/home/ana", "/") || dentro("/homero", "/home") || !dentro("/home", "/home") {
		t.Error("dentro")
	}
}

// macOS: el disco de arranque por fdesetup y los demás por diskutil.
func TestMac(t *testing.T) {
	const apfsCifrado = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
	<key>FilesystemType</key>
	<string>apfs</string>
	<key>FileVault</key>
	<true/>
	<key>MountPoint</key>
	<string>/Volumes/Copias</string>
</dict></plist>`
	const apfsClaro = `<plist><dict><key>FilesystemType</key><string>apfs</string><key>FileVault</key>
	<false/></dict></plist>`
	const exfat = `<plist><dict><key>FilesystemType</key><string>exfat</string></dict></plist>`
	const raro = `<plist><dict><key>FilesystemType</key><string>hfs</string></dict></plist>`
	volumenes := map[string][2]string{
		"/Users/ana/Sytue Office":      {"/System/Volumes/Data", "/dev/disk3s5"},
		"/Volumes/Copias/Sytue Office": {"/Volumes/Copias", "/dev/disk5s1"},
		"/Volumes/Claro/x":             {"/Volumes/Claro", "/dev/disk6s1"},
		"/Volumes/USB/x":               {"/Volumes/USB", "/dev/disk7s1"},
		"/Volumes/Viejo/x":             {"/Volumes/Viejo", "/dev/disk8s2"},
		"/Volumes/Red/x":               {"/Volumes/Red", "//ana@nas/datos"},
	}
	e := func(o ordenes) entornoMac {
		return entornoMac{orden: o.orden, volumen: func(r string) (string, string, error) {
			v, ok := volumenes[r]
			if !ok {
				return "", "", errors.New("no existe")
			}
			return v[0], v[1], nil
		}}
	}
	o := ordenes{
		"fdesetup isactive":                 "true\n",
		"diskutil info -plist /dev/disk5s1": apfsCifrado,
		"diskutil info -plist /dev/disk6s1": apfsClaro,
		"diskutil info -plist /dev/disk7s1": exfat,
		"diskutil info -plist /dev/disk8s2": raro,
	}
	for ruta, quiero := range map[string]Resultado{
		"/Users/ana/Sytue Office":      cifradoCon("FileVault"),
		"/Volumes/Copias/Sytue Office": cifradoCon("FileVault"),
		"/Volumes/Claro/x":             sinCifrar,
		"/Volumes/USB/x":               sinCifrar,
		"/Volumes/Viejo/x":             noSeSabe,
		"/Volumes/Red/x":               noSeSabe,
		"/no/existe":                   noSeSabe,
	} {
		if got := deMac(context.Background(), ruta, e(o)); got != quiero {
			t.Errorf("%s: %+v; quiero %+v", ruta, got, quiero)
		}
	}
	// FileVault apagado: «false» y sale con 1.
	if got := deMac(context.Background(), "/Users/ana/Sytue Office", e(ordenes{"fdesetup isactive": "ERROR:false\n"})); got != sinCifrar {
		t.Errorf("FileVault apagado: %+v", got)
	}
	if got := deMac(context.Background(), "/Users/ana/Sytue Office", e(ordenes{})); got != noSeSabe {
		t.Errorf("sin fdesetup: %+v", got)
	}
}

// Windows: la propiedad de BitLocker del Explorador, sin elevar.
func TestWindows(t *testing.T) {
	guion := guionBitLocker("C:")
	if guion != `(New-Object -ComObject Shell.Application).NameSpace('C:\').Self.ExtendedProperty('System.Volume.BitLockerProtection')` {
		t.Errorf("guion: %s", guion)
	}
	for salida, quiero := range map[string]Resultado{
		"1\r\n": cifradoCon("BitLocker"),
		"6\r\n": cifradoCon("BitLocker"),
		"3\r\n": cifradoCon("BitLocker"),
		"2\r\n": sinCifrar,
		"5\r\n": sinCifrar,
		"8\r\n": sinCifrar,
		"0\r\n": noSeSabe,
		"\r\n":  noSeSabe,
		// Sin permiso, PowerShell no da número.
		"Acceso denegado\r\n": noSeSabe,
	} {
		o := ordenes{"powershell.exe -NoLogo -NoProfile -NonInteractive -Command " + guion: salida}
		if got := deWindows(context.Background(), `c:\Users\ana\Sytue Office`, o.orden); got != quiero {
			t.Errorf("%q: %+v; quiero %+v", salida, got, quiero)
		}
	}
	if got := deWindows(context.Background(), `\\nas\datos\Sytue Office`, ordenes{}.orden); got != noSeSabe {
		t.Errorf("UNC: %+v", got)
	}
	if got := deWindows(context.Background(), `D:\x`, ordenes{}.orden); got != noSeSabe {
		t.Errorf("sin PowerShell: %+v", got)
	}
}

// Android e iOS cifran siempre lo de la app.
func TestMovil(t *testing.T) {
	if deMovil("android") != cifradoCon("Android") || deMovil("ios") != cifradoCon("iOS") || deMovil("plan9") != noSeSabe {
		t.Error("móvil")
	}
}

// En este equipo, De responde algo (lo que sea) sin colgarse, también para una carpeta
// que aún no existe.
func TestDeEnEsteEquipo(t *testing.T) {
	r := De(context.Background(), t.TempDir()+"/no/existe/aun")
	t.Logf("este equipo: %+v", r)
	if r.Estado != Cifrado && r.Como != "" {
		t.Errorf("sin cifrar con %q", r.Como)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"archive/tar"
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syphotos/internal/config"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/service"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

// El lote de vectores, en dos mitades que NO se hablan entre sí.
//
//	syphotos vectores-sacar   --modelo M --salida trabajo.tar
//	syphotos vectores-meter   --modelo M --entrada vectores.jsonl
//
// Entre una y otra pasa la GPU efímera (deploy/fotos-gpu), que vive minutos y
// se destruye. Por eso son dos comandos y no uno: si fuera uno, el proceso
// tendría que seguir vivo mientras la máquina trabaja, y precisamente lo que no
// se puede es depender de que nadie siga vivo — es la misma razón por la que el
// guardián de allí corre aparte.
//
// LA GPU NO HABLA CON POSTGRES NI CON EL ALMACÉN. Recibe un tar de miniaturas y
// devuelve un jsonl de vectores. Así la máquina de usar y tirar no lleva encima
// ninguna credencial: si alguien se hace con ella entre que nace y muere, se
// lleva unas miniaturas, no la base de datos.

const nombreDelManifiesto = "manifiesto.json"

type entradaDelTrabajo struct {
	ID      uuid.UUID `json:"id"`
	Fichero string    `json:"fichero"`
}

type vectorDeVuelta struct {
	ID     uuid.UUID `json:"id"`
	Vector []float32 `json:"vector"`
}

func runVectoresSacar(args []string) error {
	fs := flag.NewFlagSet("vectores-sacar", flag.ExitOnError)
	modelo := fs.String("modelo", "", "modelo con el que se van a vectorizar (obligatorio)")
	salida := fs.String("salida", "trabajo.tar", "fichero tar que se escribe")
	limite := fs.Int("limite", 5000, "cuántas fotos como mucho")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *modelo == "" {
		return fmt.Errorf("hace falta --modelo: un vector sin saber de qué modelo es no sirve")
	}

	st, objetos, cerrar, err := abrirDependencias()
	if err != nil {
		return err
	}
	defer cerrar()

	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancelar()

	pendientes, err := st.PendientesDeVector(ctx, *modelo, *limite)
	if err != nil {
		return fmt.Errorf("pendientes: %w", err)
	}
	if len(pendientes) == 0 {
		fmt.Println("nada que vectorizar")
		return nil
	}

	f, err := os.Create(*salida)
	if err != nil {
		return err
	}
	defer f.Close()
	tw := tar.NewWriter(f)

	var manifiesto []entradaDelTrabajo
	saltadas := 0
	for i, p := range pendientes {
		huella := p.Huella
		if huella == "" {
			// Sin receta guardada, la miniatura vive bajo la huella del
			// original. Es la misma que calcula `service`, no una copia.
			huella = service.HuellaDelOriginal()
		}
		clave, err := (domain.Derivada{
			ActivoID: p.ID, Huella: huella,
			Variante: domain.VarianteMiniatura, Formato: "jpg",
		}).Clave()
		if err != nil {
			saltadas++
			continue
		}
		rc, err := objetos.Get(ctx, tenancy.Tenant{Slug: p.Tenant}, clave)
		if err != nil {
			clavePng, errPng := (domain.Derivada{
				ActivoID: p.ID, Huella: huella,
				Variante: domain.VarianteMiniatura, Formato: "png",
			}).Clave()
			if errPng == nil {
				rc, err = objetos.Get(ctx, tenancy.Tenant{Slug: p.Tenant}, clavePng)
			}
		}
		if err != nil {
			// Una miniatura que todavía no existe no es un error del lote: se
			// generará cuando alguien mire esa foto, y entrará en el siguiente.
			saltadas++
			continue
		}
		datos, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			saltadas++
			continue
		}
		nombre := fmt.Sprintf("fotos/%05d.jpg", i)
		if err := tw.WriteHeader(&tar.Header{
			Name: nombre, Mode: 0o600, Size: int64(len(datos)),
		}); err != nil {
			return err
		}
		if _, err := tw.Write(datos); err != nil {
			return err
		}
		manifiesto = append(manifiesto, entradaDelTrabajo{ID: p.ID, Fichero: nombre})
	}

	crudo, err := json.Marshal(manifiesto)
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{
		Name: nombreDelManifiesto, Mode: 0o600, Size: int64(len(crudo)),
	}); err != nil {
		return err
	}
	if _, err := tw.Write(crudo); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	fmt.Printf("%d fotos en %s (%d sin miniatura todavía)\n", len(manifiesto), *salida, saltadas)
	return nil
}

func runVectoresMeter(args []string) error {
	fs := flag.NewFlagSet("vectores-meter", flag.ExitOnError)
	modelo := fs.String("modelo", "", "modelo que produjo estos vectores (obligatorio)")
	entrada := fs.String("entrada", "vectores.jsonl", "fichero jsonl que devolvió la GPU")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *modelo == "" {
		return fmt.Errorf("hace falta --modelo: guardar un vector sin saber de qué modelo es lo deja incomparable")
	}

	st, _, cerrar, err := abrirDependencias()
	if err != nil {
		return err
	}
	defer cerrar()

	f, err := os.Open(*entrada)
	if err != nil {
		return err
	}
	defer f.Close()

	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancelar()

	sc := bufio.NewScanner(f)
	// Una línea es un vector de 1152 números en texto: no cabe en el buffer por
	// defecto de bufio (64 KiB) y se cortaría a mitad, dejando un JSON inválido
	// que parece un fichero corrupto.
	sc.Buffer(make([]byte, 0, 1<<20), 8<<20)
	metidos, fallos := 0, 0
	for sc.Scan() {
		linea := sc.Bytes()
		if len(linea) == 0 {
			continue
		}
		var v vectorDeVuelta
		if err := json.Unmarshal(linea, &v); err != nil {
			fallos++
			continue
		}
		if err := st.GuardarVector(ctx, v.ID, *modelo, v.Vector); err != nil {
			fallos++
			continue
		}
		metidos++
	}
	if err := sc.Err(); err != nil {
		return err
	}
	fmt.Printf("%d vectores guardados, %d descartados\n", metidos, fallos)
	if metidos == 0 && fallos > 0 {
		return fmt.Errorf("no se guardó ni un vector: el fichero no trae lo que se espera")
	}
	return nil
}

// abrirDependencias monta configuración, base de datos y almacén, sin OIDC ni
// servidor: estos comandos no atienden a nadie.
//
// La `Config` NO sale: se usa aquí dentro para montar las piezas y ninguno de
// los dos comandos la mira —uno la recibía y la tiraba con un `_ = cfg`—.
func abrirDependencias() (*store.Store, *objectstore.Client, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, nil, err
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelar()
	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: 2})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("conectar a PostgreSQL: %w", err)
	}
	objetos, err := objectstore.New(objectstore.Config{
		Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, UseSSL: cfg.S3UseSSL,
		Remote: cfg.S3Remote, LocalTenants: cfg.S3LocalTenants,
	})
	if err != nil {
		pool.Close()
		return nil, nil, nil, err
	}
	return store.New(pool), objetos, func() { pool.Close() }, nil
}

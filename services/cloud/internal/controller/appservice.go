// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// ---------------------------------------------------------------------------
// Apps open-source como servicio gestionado
// ---------------------------------------------------------------------------
//
// La otra mitad de la ADR 0029: materializar una entrada del catálogo. Todo lo
// que decide QUÉ se arranca —imagen, licencia, límites, usuario— viene ya
// resuelto del catálogo, así que aquí no hay ni una decisión sobre el contenido;
// solo sobre la forma en Kubernetes.
//
// Lo que sí se decide aquí:
//
//   - **Nunca hay Ingress.** Un servicio del catálogo se alcanza SOLO desde
//     dentro del namespace del proyecto. Publicar Ghost o RabbitMQ en internet
//     desde aquí sería exponer un panel de administración con la contraseña que
//     acabamos de generar; quien quiera enseñarlo pone su propia app delante.
//   - **Los secretos generados se generan UNA vez.** El aprovisionamiento es
//     idempotente —el reconciliador puede repetirlo— y regenerar la clave
//     maestra en cada pasada dejaría al servicio y a la app del cliente con
//     credenciales distintas sin que nada fallara.
//   - **`Recreate`, no `RollingUpdate`.** Con un disco `ReadWriteOnce`, un rollout
//     progresivo deja el pod nuevo esperando un volumen que aún tiene el viejo.

// provisionApp materializa una app del catálogo: Secret + PVC (si tiene disco) +
// Deployment + Service. Sin Ingress, a propósito.
// upsertAppDisk crea el disco de la app si el catálogo declara uno. `ReadWriteOnce`
// es lo que obliga al `Recreate` del Deployment: un rollout progresivo dejaría al
// pod nuevo esperando un volumen que aún tiene el viejo.
func (p *KubeProvisioner) upsertAppDisk(
	ctx context.Context,
	namespace, name string,
	app domain.CatalogApp,
	labels map[string]string,
) error {
	if app.DiskGiB <= 0 {
		return nil
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-app-data", Namespace: namespace, Labels: labels},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(fmt.Sprintf("%dGi", app.DiskGiB)),
				},
			},
		},
	}
	return p.upsertPVC(ctx, namespace, pvc)
}

// credencialesDeApp arma lo que la app del cliente recibe: los exports del
// catálogo (URLs) más los secretos que la entrada marcó como exportables.
//
// Van por Credentials —y no por Config— porque es el camino que el controlador
// cifra e inyecta en el Deployment del proyecto (deploy.go, injectServiceCreds).
func credencialesDeApp(app domain.CatalogApp, env map[string]string, host string) ([]byte, error) {
	creds := domain.RenderExports(app, host)
	for _, e := range app.Env {
		if e.Export {
			creds[e.Key] = env[e.Key]
		}
	}
	raw, err := json.Marshal(creds)
	if err != nil {
		return nil, fmt.Errorf("controller: serializar credenciales de %s: %w", app.ID, err)
	}
	return raw, nil
}

// Configuración que aporta el catálogo. Va en un ConfigMap y no en el Secret del
// entorno porque no es sensible y porque se monta como FICHERO: hay software que
// no se configura de otra manera.
//
// Está fuera de `provisionApp` porque esa función pasó de 30 de complejidad
// cognitiva y el CI la rechaza; es el bloque más autocontenido que tenía dentro.
func (p *KubeProvisioner) upsertAppConfigFiles(
	ctx context.Context,
	namespace, name string,
	app domain.CatalogApp,
	labels map[string]string,
	vars map[string]string,
) error {
	if len(app.ConfigFiles) == 0 {
		return nil
	}
	datos := make(map[string]string, len(app.ConfigFiles))
	for _, c := range app.ConfigFiles {
		datos[configFileKey(c.Path)] = domain.RenderEnvValue(c.Content, vars)
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-app-config", Namespace: namespace, Labels: labels},
		Data:       datos,
	}
	return p.upsertConfigMap(ctx, namespace, cm)
}

func (p *KubeProvisioner) provisionApp(ctx context.Context, svc domain.ManagedService) (ProvisionResult, error) {
	app, cat, err := p.catalogApp(svc)
	if err != nil {
		return ProvisionResult{}, err
	}
	namespace := "tenant-" + svc.Tenant
	name := svcResourceName(svc)
	labels := managedLabels(svc)
	// El id del catálogo va en las etiquetas porque es como una app encuentra a
	// otra del mismo proyecto (integrationEnv): el nombre del recurso lo decide un
	// UUID, así que no dice nada de QUÉ es lo que hay dentro.
	labels[appIDLabel] = app.ID

	// Los valores generados se leen del Secret si ya existe: aprovisionar es
	// idempotente y regenerarlos en cada pasada rompería lo que ya está en marcha.
	// La URL pública se conoce AQUÍ y no en el catálogo: depende del proyecto.
	// Vacía cuando la app no se publica, que es lo que hace que el marcador
	// desaparezca en vez de dejar un `{web_url}` literal dentro del contenedor.
	webURL := ""
	if app.WebPort > 0 && svc.Config["web_host"] != "" {
		webURL = "https://" + svc.Config["web_host"]
	}
	// Los marcadores que puede usar el manifiesto. `{web_url}` siempre; los
	// `{db_*}` solo si la app declaró `needs: postgres` — y entonces el Postgres
	// TIENE que estar, porque el alta no deja crearla sin él.
	vars := map[string]string{"{web_url}": webURL}
	if slices.Contains(app.Needs, domain.AppNeedPostgres) {
		db, derr := p.postgresVars(ctx, namespace, svc.ProjectID.String())
		if derr != nil {
			return ProvisionResult{}, derr
		}
		for k, v := range db {
			vars[k] = v
		}
	}
	env, err := p.resolveAppEnv(ctx, namespace, name+"-app-env", app, vars)
	if err != nil {
		return ProvisionResult{}, err
	}
	// Y encima, lo que aportan las apps hermanas ya levantadas. Va DESPUÉS a
	// propósito: una integración pisa el literal del catálogo —`CACHE=memoria`
	// pasa a `CACHE=redis` cuando hay caché que usar— y nunca un secreto generado,
	// que la validación del catálogo ya impide que declare.
	integraciones, err := p.integrationEnv(ctx, namespace, svc, app, cat)
	if err != nil {
		return ProvisionResult{}, err
	}
	for k, v := range integraciones {
		env[k] = v
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-app-env", Namespace: namespace, Labels: labels},
		Type:       corev1.SecretTypeOpaque,
		StringData: env,
	}
	if err := p.upsertSecret(ctx, namespace, secret); err != nil {
		return ProvisionResult{}, err
	}

	if err := p.upsertAppConfigFiles(ctx, namespace, name, app, labels, vars); err != nil {
		return ProvisionResult{}, err
	}

	if err := p.upsertAppDisk(ctx, namespace, name, app, labels); err != nil {
		return ProvisionResult{}, err
	}

	if app.WebPort > 0 {
		labels[publishedLabel] = "true"
	}
	dep := appDeployment(name, namespace, p.cfg.RegistryURL, labels, app)
	if err := p.upsertDeployment(ctx, namespace, dep); err != nil {
		return ProvisionResult{}, err
	}
	if err := p.upsertService(ctx, namespace, appService(name, namespace, labels, app)); err != nil {
		return ProvisionResult{}, err
	}
	// Publicar en internet, si el catálogo lo declara y el alta resolvió el
	// hostname. Se comprueban las DOS cosas: el hostname lo pone la capa de
	// aplicación (es la única que conoce el proyecto y el dominio), así que un
	// servicio creado antes de que la app fuera publicable no tiene ninguno, y
	// montar el Ingress con un host vacío daría una regla que captura TODO el
	// tráfico que llegue sin Host reconocido.
	if app.WebPort > 0 && svc.Config["web_host"] != "" {
		if err := p.applyAppIngress(ctx, namespace, name, svc.Config["web_host"], app, labels); err != nil {
			return ProvisionResult{}, err
		}
	}

	host := fmt.Sprintf("%s-app.%s.svc.cluster.local", name, namespace)
	publicURL := webURL
	raw, err := credencialesDeApp(app, env, host)
	if err != nil {
		return ProvisionResult{}, err
	}
	return ProvisionResult{
		Config: map[string]string{
			"app_id":  app.ID,
			"version": app.Version,
			"license": app.License,
			"host":    host,
			"port":    fmt.Sprint(app.Ports[0].Port),
			// Se devuelve aunque venga de la entrada: MarkServiceReady REEMPLAZA la
			// configuración entera, así que lo que no se repita aquí desaparece — y con
			// ello la URL que el panel enseña.
			"web_host": svc.Config["web_host"],
			"web_url":  publicURL,
		},
		Credentials: raw,
	}, nil
}

// applyAppIngress publica una app del catálogo en su propio hostname, con TLS de
// cert-manager. Mismo patrón que el Ingress de la app del cliente
// (KubePlatform.applyIngress): Traefik, entrypoint websecure y un secreto de
// certificado por host.
//
// Lo que aquí NO hay, y en el de la app del cliente sí, es la ruta /_sytue de
// Sytue Analytics: la analítica mide lo que construye el cliente, y el HTML de un
// Nextcloud lo escribe Nextcloud. Inyectar medición en el software de otro sin
// que nadie lo pida sería justo lo que esta plataforma no hace.
func (p *KubeProvisioner) applyAppIngress(
	ctx context.Context, namespace, name, webHost string, app domain.CatalogApp, labels map[string]string,
) error {
	pathType := networkingv1.PathTypePrefix
	className := "traefik"
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-app",
			Namespace: namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"traefik.ingress.kubernetes.io/router.entrypoints": "websecure",
				"traefik.ingress.kubernetes.io/router.tls":         "true",
				"cert-manager.io/cluster-issuer":                   p.cfg.CertIssuer,
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &className,
			TLS: []networkingv1.IngressTLS{{
				Hosts:      []string{webHost},
				SecretName: "tls-" + dnsSafe(webHost),
			}},
			Rules: []networkingv1.IngressRule{{
				Host: webHost,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     "/",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: name + "-app",
									Port: networkingv1.ServiceBackendPort{Number: portInt32(app.WebPort)},
								},
							},
						}},
					},
				},
			}},
		},
	}

	api := p.cs.NetworkingV1().Ingresses(namespace)
	existing, err := api.Get(ctx, ing.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, cerr := api.Create(ctx, ing, metav1.CreateOptions{})
		return ignoreAlreadyExists(cerr, "crear Ingress de app del catálogo")
	}
	if err != nil {
		return fmt.Errorf("controller: consultar Ingress de %s: %w", app.ID, err)
	}
	ing.ResourceVersion = existing.ResourceVersion
	if _, uerr := api.Update(ctx, ing, metav1.UpdateOptions{}); uerr != nil {
		return fmt.Errorf("controller: actualizar Ingress de %s: %w", app.ID, uerr)
	}
	return nil
}

// appIDLabel marca en Kubernetes QUÉ entrada del catálogo es cada servicio. Es
// la única forma de que una app encuentre a otra: los nombres de recurso salen
// de un UUID y no dicen nada del contenido.
const appIDLabel = "sytue.eu/app-id"

// catalogApp resuelve la entrada del catálogo del servicio, y devuelve también el
// catálogo entero —lo necesitan las integraciones para mirar a las otras apps—.
//
// Falla si no existe o si su imagen aún no está replicada: aprovisionar algo que
// no se puede descargar deja el servicio en «provisioning» para siempre, después
// de habérselo prometido al cliente.
func (p *KubeProvisioner) catalogApp(svc domain.ManagedService) (domain.CatalogApp, domain.AppCatalog, error) {
	cat, err := domain.LoadAppCatalog(p.cfg.RegistryURL)
	if err != nil {
		return domain.CatalogApp{}, domain.AppCatalog{}, fmt.Errorf("controller: catálogo de apps: %w", err)
	}
	app, err := appFromCatalog(cat, p.cfg.RegistryURL, svc.Config["app_id"])
	return app, cat, err
}

// integrationEnv resuelve lo que esta app recibe de sus hermanas del catálogo.
//
// Aquí vive la promesa de «se integran aunque los elijas de uno en uno»: no se
// pregunta al usuario ni se guarda una relación en la base de datos, se MIRA lo
// que hay levantado en el proyecto. Una integración cuya otra mitad no está
// simplemente no aporta nada, que es lo correcto: inyectar la URL de una caché
// que no existe deja al servicio arrancando contra un host que no resuelve.
//
// El filtro por proyecto NO es opcional aunque el selector de app parezca
// suficiente: el namespace es del TENANT y dentro caben varios proyectos, así que
// sin él la app de un proyecto se conectaría a la caché de otro —del mismo
// cliente, pero con sus datos— y ninguna de las dos partes se enteraría.
func (p *KubeProvisioner) integrationEnv(
	ctx context.Context,
	namespace string,
	svc domain.ManagedService,
	app domain.CatalogApp,
	cat domain.AppCatalog,
) (map[string]string, error) {
	out := map[string]string{}
	if len(app.Integrations) == 0 {
		return out, nil
	}
	for _, in := range app.Integrations {
		otra, ok := catalogAppByID(cat, in.App)
		if !ok {
			// En el catálogo pero sin replicar: no hay servicio que buscar.
			continue
		}
		sel := fmt.Sprintf("%s=%s,%s=%s", appIDLabel, in.App, projectLabel, svc.ProjectID.String())
		lista, err := p.cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{LabelSelector: sel})
		if err != nil {
			return nil, fmt.Errorf("controller: buscar %s para integrar con %s: %w", in.App, app.ID, err)
		}
		if len(lista.Items) == 0 {
			continue
		}
		host := fmt.Sprintf("%s.%s.svc.cluster.local", lista.Items[0].Name, namespace)
		for k, v := range domain.RenderIntegration(in, host, domain.MainPort(otra)) {
			out[k] = v
		}
	}
	return out, nil
}

// postgresVars resuelve los marcadores `{db_*}` con las credenciales REALES del
// Sytue Postgres del proyecto.
//
// Se leen del Secret que ya existe en el namespace, no de la base de datos del
// control-plane: es la misma fuente que monta el propio Postgres, así que no hay
// dos versiones de la contraseña que puedan divergir, y el provisioner no
// necesita el descifrador para nada.
//
// Un fallo aquí ES un error de aprovisionamiento y no un valor vacío: dejar
// `{db_password}` sin resolver arrancaría la app con una cadena literal como
// contraseña y el motivo no aparecería en ningún log.
func (p *KubeProvisioner) postgresVars(ctx context.Context, namespace, projectID string) (map[string]string, error) {
	// `target!=preview`: una app del catálogo guarda sus datos en el Postgres de
	// producción del proyecto, nunca en el de las vistas previas. El selector casa
	// también con los que no llevan la etiqueta, que son todos los de producción.
	sel := fmt.Sprintf("sytue.eu/service-kind=%s,%s=%s,%s!=%s",
		string(domain.ServicePostgres), projectLabel, projectID, targetLabel, string(domain.TargetPreview))
	lista, err := p.cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return nil, fmt.Errorf("controller: buscar el postgres del proyecto: %w", err)
	}
	if len(lista.Items) == 0 {
		return nil, fmt.Errorf("controller: la app necesita un Sytue Postgres y el proyecto no tiene ninguno listo")
	}
	svcName := lista.Items[0].Name
	// El Secret se llama como el servicio menos el sufijo del Service. Se busca por
	// etiqueta igual que el Service para no depender de esa aritmética de nombres.
	secretos, err := p.cs.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return nil, fmt.Errorf("controller: leer credenciales del postgres: %w", err)
	}
	var creds map[string][]byte
	for i := range secretos.Items {
		if _, ok := secretos.Items[i].Data["POSTGRES_PASSWORD"]; ok {
			creds = secretos.Items[i].Data
			break
		}
	}
	if creds == nil {
		return nil, fmt.Errorf("controller: el postgres del proyecto no tiene aún sus credenciales")
	}
	host := fmt.Sprintf("%s.%s.svc.cluster.local", svcName, namespace)
	usuario, base, pass := string(creds["POSTGRES_USER"]), string(creds["POSTGRES_DB"]), string(creds["POSTGRES_PASSWORD"])
	return map[string]string{
		"{db_host}":     host,
		"{db_port}":     "5432",
		"{db_name}":     base,
		"{db_user}":     usuario,
		"{db_password}": pass,
		// La URL completa, para las apps que solo aceptan un DSN. El usuario y la
		// contraseña se escapan: una contraseña generada puede llevar caracteres
		// que rompan el parseo de la URL en el otro extremo.
		"{db_url}": (&url.URL{
			Scheme: "postgres", User: url.UserPassword(usuario, pass),
			Host: host + ":5432", Path: "/" + base,
		}).String(),
	}, nil
}

// configFileKey convierte una ruta en clave de ConfigMap: solo se admiten
// [-._a-zA-Z0-9], así que las barras pasan a guiones bajos. La ruta real la
// impone el `mountPath`; esto solo tiene que ser único y válido.
func configFileKey(path string) string {
	return strings.TrimPrefix(strings.ReplaceAll(path, "/", "_"), "_")
}

func catalogAppByID(cat domain.AppCatalog, id string) (domain.CatalogApp, bool) {
	for _, a := range cat.Apps {
		if a.ID == id {
			return a, true
		}
	}
	return domain.CatalogApp{}, false
}

// appFromCatalog resuelve un id contra un catálogo YA cargado.
//
// Está separado de la carga para poder comprobar el mensaje de «todavía no replicada» sin
// depender de qué app esté replicada hoy: el digest del espejo lo anota el CI según va
// replicando imágenes, así que un test atado al catálogo real se rompe solo el día que la
// última app pendiente deja de estarlo. Pasó.
func appFromCatalog(cat domain.AppCatalog, registry, id string) (domain.CatalogApp, error) {
	for _, a := range cat.Apps {
		if a.ID == id {
			return a, nil
		}
	}
	for _, pendiente := range cat.PendingMirror {
		if pendiente == id {
			return domain.CatalogApp{}, fmt.Errorf(
				"controller: la app %q está en el catálogo pero su imagen todavía no se ha "+
					"replicado en %s", id, registry,
			)
		}
	}
	return domain.CatalogApp{}, fmt.Errorf("controller: no existe la app %q en el catálogo", id)
}

// resolveAppEnv construye el entorno del servicio, conservando los valores que
// ya se generaron en un aprovisionamiento anterior.
func (p *KubeProvisioner) resolveAppEnv(
	ctx context.Context, namespace, secretName string, app domain.CatalogApp, vars map[string]string,
) (map[string]string, error) {
	previo := map[string]string{}
	if existing, err := p.cs.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{}); err == nil {
		for k, v := range existing.Data {
			previo[k] = string(v)
		}
	}
	out := make(map[string]string, len(app.Env))
	for _, e := range app.Env {
		switch {
		case !e.Generate:
			out[e.Key] = domain.RenderEnvValue(e.Value, vars)
		case previo[e.Key] != "":
			out[e.Key] = previo[e.Key]
		default:
			tok, err := randToken(24)
			if err != nil {
				return nil, err
			}
			out[e.Key] = tok
		}
	}
	return out, nil
}

// appDeployment construye el Deployment de una app del catálogo bajo
// PodSecurity `restricted`: sin privilegios, sin capacidades, raíz de solo
// lectura y usuario no-root.
func appDeployment(name, namespace, registry string, labels map[string]string, app domain.CatalogApp) *appsv1.Deployment {
	replicas := int32(1)
	runAsNonRoot := true
	allowPrivEsc := false
	readOnlyRoot := true
	uid := app.RunAsUser
	automountSA := false
	selector := map[string]string{"sytue.eu/managed": name + "-app"}

	ports := make([]corev1.ContainerPort, 0, len(app.Ports))
	for _, port := range app.Ports {
		ports = append(ports, corev1.ContainerPort{Name: port.Name, ContainerPort: portInt32(port.Port)})
	}

	// /tmp siempre: con la raíz de solo lectura, media imagen no arranca sin un
	// sitio donde escribir temporales, y montar un emptyDir es más barato —y más
	// acotado— que renunciar a readOnlyRootFilesystem.
	tmpSize := resource.MustParse("256Mi")
	rwSize := resource.MustParse("512Mi")
	mounts := []corev1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}}
	volumes := []corev1.Volume{{
		Name:         "tmp",
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: &tmpSize}},
	}}
	if app.DiskGiB > 0 {
		mounts = append(mounts, corev1.VolumeMount{Name: "data", MountPath: app.DataPath})
		volumes = append(volumes, corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: name + "-app-data"},
			},
		})
	}
	// Directorios que la imagen escribe al arrancar y que la raíz de solo lectura
	// le impide. emptyDir a propósito: efímeros, así que nada persiste ahí sin que
	// alguien lo haya declarado como disco de datos.
	for i, w := range app.WritablePaths {
		vol := fmt.Sprintf("rw-%d", i)
		mounts = append(mounts, corev1.VolumeMount{Name: vol, MountPath: w})
		volumes = append(volumes, corev1.Volume{
			Name:         vol,
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: &rwSize}},
		})
	}

	// Ficheros de configuración: subPath a propósito. Montar el directorio entero
	// escondería lo que la imagen ya tenga ahí —y casi siempre hay algo—, así que
	// se monta fichero a fichero.
	if len(app.ConfigFiles) > 0 {
		volumes = append(volumes, corev1.Volume{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: name + "-app-config"},
				},
			},
		})
		soloLectura := true
		for _, c := range app.ConfigFiles {
			mounts = append(mounts, corev1.VolumeMount{
				Name: "config", MountPath: c.Path,
				SubPath: configFileKey(c.Path), ReadOnly: soloLectura,
			})
		}
	}

	container := corev1.Container{
		Name:  "app",
		Image: app.ImageRef(registry),
		Ports: ports,
		EnvFrom: []corev1.EnvFromSource{{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: name + "-app-env"},
			},
		}},
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             &runAsNonRoot,
			RunAsUser:                &uid,
			AllowPrivilegeEscalation: &allowPrivEsc,
			ReadOnlyRootFilesystem:   &readOnlyRoot,
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
		Resources: corev1.ResourceRequirements{
			// Las requests son una fracción del límite: el catálogo declara el
			// TECHO, y reservar el techo entero multiplicaría por cuatro lo que un
			// proyecto le quita a la cuota sin usarlo.
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:              *resource.NewMilliQuantity(int64(app.CPUMilli)/4+1, resource.DecimalSI),
				corev1.ResourceMemory:           *resource.NewQuantity(int64(app.MemoryMiB)/4*1024*1024+1, resource.BinarySI),
				corev1.ResourceEphemeralStorage: resource.MustParse("128Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:              *resource.NewMilliQuantity(int64(app.CPUMilli), resource.DecimalSI),
				corev1.ResourceMemory:           *resource.NewQuantity(int64(app.MemoryMiB)*1024*1024, resource.BinarySI),
				corev1.ResourceEphemeralStorage: resource.MustParse("1Gi"),
			},
		},
		VolumeMounts:   mounts,
		ReadinessProbe: appProbe(app),
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-app", Namespace: namespace, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			// Recreate y no RollingUpdate: con un disco ReadWriteOnce el pod nuevo
			// se quedaría esperando un volumen que todavía tiene el viejo.
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: mergeLabels(labels, selector)},
				Spec: corev1.PodSpec{
					RuntimeClassName:             tenantRuntimeClassPtr(),
					AutomountServiceAccountToken: &automountSA,
					ImagePullSecrets:             tenantImagePullSecrets(),
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &runAsNonRoot,
						RunAsUser:    &uid,
						// FSGroup para que el disco sea escribible por ese UID: sin
						// esto, un volumen recién creado pertenece a root y el
						// servicio arranca y muere sin poder escribir.
						FSGroup:        &uid,
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					Containers: []corev1.Container{container},
					Volumes:    volumes,
				},
			},
		},
	}
}

// portInt32 convierte un puerto del catálogo.
//
// El rango ya lo garantiza ValidateCatalogApp; el recorte a 0 está para que la
// conversión sea TOTAL y no dependa de que esa validación siga existiendo dentro
// de un año. Un 0 lo rechaza la API de Kubernetes en voz alta, que es justo lo
// que se quiere si alguna vez llegara aquí un puerto imposible.
func portInt32(p int) int32 {
	if p < 1 || p > 65535 {
		return 0
	}
	return int32(p)
}

// appProbe: HTTP si la entrada declara ruta de salud, TCP si no.
//
// Siempre hay una: sin sonda, el Service manda tráfico a un pod que aún está
// arrancando y la app del cliente recibe conexiones rechazadas que parecen un
// fallo suyo.
func appProbe(app domain.CatalogApp) *corev1.Probe {
	port := intstr.FromInt32(portInt32(app.Ports[0].Port))
	h := corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: port}}
	if app.HealthPath != "" {
		h = corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: app.HealthPath, Port: port}}
	}
	return &corev1.Probe{ProbeHandler: h, InitialDelaySeconds: 10, PeriodSeconds: 10, FailureThreshold: 6}
}

// appService publica TODOS los puertos de la entrada dentro del namespace.
func appService(name, namespace string, labels map[string]string, app domain.CatalogApp) *corev1.Service {
	ports := make([]corev1.ServicePort, 0, len(app.Ports))
	for _, port := range app.Ports {
		ports = append(ports, corev1.ServicePort{
			Name:       port.Name,
			Port:       portInt32(port.Port),
			TargetPort: intstr.FromInt32(portInt32(port.Port)),
			Protocol:   corev1.ProtocolTCP,
		})
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-app", Namespace: namespace, Labels: labels},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"sytue.eu/managed": name + "-app"},
			Ports:    ports,
		},
	}
}

// deprovisionApp retira todo lo que creó provisionApp, incluido el disco: un
// servicio borrado que deja el PVC detrás sigue costando dinero al cliente.
func (p *KubeProvisioner) deprovisionApp(ctx context.Context, namespace, name string) error {
	bg := metav1.DeletePropagationBackground
	opts := metav1.DeleteOptions{PropagationPolicy: &bg}
	_ = p.cs.AppsV1().Deployments(namespace).Delete(ctx, name+"-app", opts)
	_ = p.cs.CoreV1().Services(namespace).Delete(ctx, name+"-app", opts)
	// Sin condicionar a que la app fuera publicable: aquí ya no se sabe qué había, y
	// borrar un Ingress que no existe no cuesta nada. Dejarlo vivo sí: el hostname
	// seguiría resolviendo hacia un backend que ya no está.
	_ = p.cs.NetworkingV1().Ingresses(namespace).Delete(ctx, name+"-app", opts)
	_ = p.cs.CoreV1().Secrets(namespace).Delete(ctx, name+"-app-env", opts)
	// Mismo criterio que el Ingress: se borra sin preguntar si la entrada tenía
	// configuración. Uno huérfano no rompe nada, pero se queda ocupando el nombre
	// que usaría la próxima app con el mismo id.
	_ = p.cs.CoreV1().ConfigMaps(namespace).Delete(ctx, name+"-app-config", opts)
	_ = p.cs.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, name+"-app-data", opts)
	return nil
}

package translate

import (
	"bytes"
	"strings"
	"testing"

	"github.com/docker-x/composed/internal/k8s"
)

// helper: parse K8s YAML and translate to compose
func mustTranslate(t *testing.T, yaml string, opts Opts) *Result {
	t.Helper()
	m, err := k8s.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("k8s.Parse error: %v", err)
	}
	result, err := Translate(m, opts)
	if err != nil {
		t.Fatalf("Translate error: %v", err)
	}
	return result
}

func TestTranslate_SimpleDeployment(t *testing.T) {
	yaml := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
spec:
  replicas: 2
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
        - name: nginx
          image: nginx:1.25
          ports:
            - containerPort: 80
          command: ["/docker-entrypoint.sh"]
          args: ["nginx", "-g", "daemon off;"]
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["nginx"]
	if svc == nil {
		t.Fatal("missing nginx service")
	}
	if svc.Image != "nginx:1.25" {
		t.Errorf("Image = %q", svc.Image)
	}
	if len(svc.Entrypoint) != 1 || svc.Entrypoint[0] != "/docker-entrypoint.sh" {
		t.Errorf("Entrypoint = %v", svc.Entrypoint)
	}
	if len(svc.Command) != 3 {
		t.Errorf("Command = %v", svc.Command)
	}
	if svc.Deploy == nil || svc.Deploy.Replicas == nil || *svc.Deploy.Replicas != 2 {
		t.Error("Replicas should be 2")
	}
}

func TestTranslate_DeploymentWithEnv(t *testing.T) {
	yaml := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
        - name: app
          image: myapp:latest
          env:
            - name: PORT
              value: "8080"
            - name: DEBUG
              value: "true"
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["app"]
	if svc.Environment["PORT"] != "8080" {
		t.Errorf("PORT = %q", svc.Environment["PORT"])
	}
	if svc.Environment["DEBUG"] != "true" {
		t.Errorf("DEBUG = %q", svc.Environment["DEBUG"])
	}
}

func TestTranslate_EnvFromConfigMap(t *testing.T) {
	yaml := `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
data:
  DATABASE_URL: "postgresql://localhost/db"
  LOG_LEVEL: "info"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
        - name: app
          image: myapp:latest
          envFrom:
            - configMapRef:
                name: app-config
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["app"]
	if svc.Environment["DATABASE_URL"] != "postgresql://localhost/db" {
		t.Errorf("DATABASE_URL = %q", svc.Environment["DATABASE_URL"])
	}
	if svc.Environment["LOG_LEVEL"] != "info" {
		t.Errorf("LOG_LEVEL = %q", svc.Environment["LOG_LEVEL"])
	}
}

func TestTranslate_EnvFromConfigMapKeyRef(t *testing.T) {
	yaml := `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: settings
data:
  db_host: localhost
  db_port: "5432"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
        - name: app
          image: myapp:latest
          env:
            - name: DB_HOST
              valueFrom:
                configMapKeyRef:
                  name: settings
                  key: db_host
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["app"]
	if svc.Environment["DB_HOST"] != "localhost" {
		t.Errorf("DB_HOST = %q", svc.Environment["DB_HOST"])
	}
}

func TestTranslate_EnvFromSecret(t *testing.T) {
	yaml := `---
apiVersion: v1
kind: Secret
metadata:
  name: db-creds
stringData:
  password: supersecret
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
        - name: app
          image: myapp:latest
          envFrom:
            - secretRef:
                name: db-creds
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["app"]
	if svc.Environment["password"] != "supersecret" {
		t.Errorf("password = %q", svc.Environment["password"])
	}
	// Should warn about plaintext secrets
	found := false
	for _, w := range result.Report.Warnings {
		if strings.Contains(w, "plaintext") {
			found = true
		}
	}
	if !found {
		t.Error("expected plaintext warning for secret")
	}
}

func TestTranslate_EnvFromSecretKeyRef(t *testing.T) {
	yaml := `---
apiVersion: v1
kind: Secret
metadata:
  name: creds
stringData:
  api_key: sk-123
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
        - name: app
          image: myapp:latest
          env:
            - name: API_KEY
              valueFrom:
                secretKeyRef:
                  name: creds
                  key: api_key
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["app"]
	if svc.Environment["API_KEY"] != "sk-123" {
		t.Errorf("API_KEY = %q", svc.Environment["API_KEY"])
	}
}

func TestTranslate_PVCVolume(t *testing.T) {
	yaml := `---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: data-pvc
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
        - name: app
          image: myapp:latest
          volumeMounts:
            - name: data
              mountPath: /data
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: data-pvc
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["app"]

	found := false
	for _, v := range svc.Volumes {
		if v == "data-pvc:/data" {
			found = true
		}
	}
	if !found {
		t.Errorf("Volumes = %v, want 'data-pvc:/data'", svc.Volumes)
	}

	if _, ok := result.Compose.Volumes["data-pvc"]; !ok {
		t.Error("missing data-pvc in top-level volumes")
	}
}

func TestTranslate_EmptyDir(t *testing.T) {
	yaml := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
        - name: app
          image: myapp:latest
          volumeMounts:
            - name: cache
              mountPath: /cache
            - name: shm
              mountPath: /dev/shm
      volumes:
        - name: cache
          emptyDir: {}
        - name: shm
          emptyDir:
            medium: Memory
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["app"]

	// Regular emptyDir → anonymous volume
	found := false
	for _, v := range svc.Volumes {
		if v == "/cache" {
			found = true
		}
	}
	if !found {
		t.Errorf("Volumes = %v, want '/cache'", svc.Volumes)
	}

	// Memory-backed emptyDir at /dev/shm → shm_size
	if svc.ShmSize != "256m" {
		t.Errorf("ShmSize = %q, want %q", svc.ShmSize, "256m")
	}
}

func TestTranslate_EmptyDir_Memory_Tmpfs(t *testing.T) {
	yaml := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
        - name: app
          image: myapp:latest
          volumeMounts:
            - name: mem-cache
              mountPath: /tmp/cache
      volumes:
        - name: mem-cache
          emptyDir:
            medium: Memory
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["app"]

	// Non-/dev/shm memory emptyDir → tmpfs
	found := false
	for _, v := range svc.Tmpfs {
		if v == "/tmp/cache" {
			found = true
		}
	}
	if !found {
		t.Errorf("Tmpfs = %v, want '/tmp/cache'", svc.Tmpfs)
	}
}

func TestTranslate_ConfigMapVolumeMount(t *testing.T) {
	yaml := `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: nginx-conf
data:
  nginx.conf: |
    server {
      listen 80;
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
spec:
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
        - name: nginx
          image: nginx:latest
          volumeMounts:
            - name: conf
              mountPath: /etc/nginx
      volumes:
        - name: conf
          configMap:
            name: nginx-conf
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["nginx"]

	if len(svc.Configs) != 1 {
		t.Fatalf("Configs count = %d, want 1", len(svc.Configs))
	}
	if svc.Configs[0].Target != "/etc/nginx/nginx.conf" {
		t.Errorf("Config target = %q", svc.Configs[0].Target)
	}

	cfg := result.Compose.Configs[svc.Configs[0].Source]
	if cfg == nil {
		t.Fatal("missing config in top-level configs")
	}
	if !strings.Contains(cfg.Content, "listen 80") {
		t.Errorf("Config content = %q", cfg.Content)
	}
}

func TestTranslate_ConfigMapSubPath(t *testing.T) {
	yaml := `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
data:
  settings.json: '{"debug": true}'
  other.txt: ignored
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
        - name: app
          image: myapp:latest
          volumeMounts:
            - name: config
              mountPath: /etc/app/settings.json
              subPath: settings.json
      volumes:
        - name: config
          configMap:
            name: app-config
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["app"]

	if len(svc.Configs) != 1 {
		t.Fatalf("Configs count = %d, want 1 (only subPath key)", len(svc.Configs))
	}
	if svc.Configs[0].Target != "/etc/app/settings.json" {
		t.Errorf("Config target = %q", svc.Configs[0].Target)
	}
}

func TestTranslate_InitContainers(t *testing.T) {
	yaml := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      initContainers:
        - name: init-db
          image: busybox:latest
          command: ["sh", "-c", "echo init"]
        - name: init-cache
          image: busybox:latest
          command: ["sh", "-c", "echo cache"]
      containers:
        - name: app
          image: myapp:latest
`
	result := mustTranslate(t, yaml, Opts{})

	// Check init container services exist
	initDB := result.Compose.Services["app-init-init-db"]
	if initDB == nil {
		t.Fatal("missing app-init-init-db service")
	}
	if initDB.Image != "busybox:latest" {
		t.Errorf("Init image = %q", initDB.Image)
	}
	if initDB.Deploy == nil || initDB.Deploy.RestartPolicy == nil {
		t.Fatal("init container should have restart policy")
	}
	if initDB.Deploy.RestartPolicy.Condition != "on-failure" {
		t.Errorf("RestartPolicy = %q", initDB.Deploy.RestartPolicy.Condition)
	}

	initCache := result.Compose.Services["app-init-init-cache"]
	if initCache == nil {
		t.Fatal("missing app-init-init-cache service")
	}

	// Main service should depend on last init container
	app := result.Compose.Services["app"]
	if app == nil {
		t.Fatal("missing app service")
	}
	dep, ok := app.DependsOn["app-init-init-cache"]
	if !ok {
		t.Error("app should depend on last init container")
	}
	if dep.Condition != "service_completed_successfully" {
		t.Errorf("DependsOn condition = %q", dep.Condition)
	}
}

func TestTranslate_Job(t *testing.T) {
	yaml := `
apiVersion: batch/v1
kind: Job
metadata:
  name: db-migrate
spec:
  template:
    spec:
      containers:
        - name: migrate
          image: myapp:latest
          command: ["rake", "db:migrate"]
      restartPolicy: Never
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["db-migrate"]
	if svc == nil {
		t.Fatal("missing db-migrate service")
	}
	if svc.Image != "myapp:latest" {
		t.Errorf("Image = %q", svc.Image)
	}
	if svc.Deploy == nil || svc.Deploy.RestartPolicy == nil {
		t.Fatal("Job should have restart policy")
	}
	if svc.Deploy.RestartPolicy.Condition != "on-failure" {
		t.Errorf("Condition = %q", svc.Deploy.RestartPolicy.Condition)
	}
	if svc.Deploy.RestartPolicy.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d", svc.Deploy.RestartPolicy.MaxAttempts)
	}
}

func TestTranslate_ServicePorts_NodePort(t *testing.T) {
	yaml := `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
        - name: web
          image: nginx:latest
          ports:
            - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: web-svc
spec:
  type: NodePort
  selector:
    app: web
  ports:
    - port: 80
      targetPort: 80
      nodePort: 30080
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["web"]
	found := false
	for _, p := range svc.Ports {
		if p == "30080:80" {
			found = true
		}
	}
	if !found {
		t.Errorf("Ports = %v, want '30080:80'", svc.Ports)
	}
}

func TestTranslate_ServicePorts_LoadBalancer(t *testing.T) {
	yaml := `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  selector:
    matchLabels:
      app: api
  template:
    metadata:
      labels:
        app: api
    spec:
      containers:
        - name: api
          image: api:latest
          ports:
            - containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: api-lb
spec:
  type: LoadBalancer
  selector:
    app: api
  ports:
    - port: 80
      targetPort: 8080
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["api"]
	found := false
	for _, p := range svc.Ports {
		if p == "80:8080" {
			found = true
		}
	}
	if !found {
		t.Errorf("Ports = %v, want '80:8080'", svc.Ports)
	}
}

func TestTranslate_ServicePorts_ClusterIP(t *testing.T) {
	yaml := `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: backend
spec:
  selector:
    matchLabels:
      app: backend
  template:
    metadata:
      labels:
        app: backend
    spec:
      containers:
        - name: backend
          image: backend:latest
          ports:
            - containerPort: 3000
---
apiVersion: v1
kind: Service
metadata:
  name: backend-svc
spec:
  selector:
    app: backend
  ports:
    - port: 3000
      targetPort: 3000
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["backend"]
	// ClusterIP → still maps ports for Compose (port:targetPort)
	found := false
	for _, p := range svc.Ports {
		if p == "3000:3000" {
			found = true
		}
	}
	if !found {
		t.Errorf("Ports = %v, want '3000:3000'", svc.Ports)
	}
}

func TestTranslate_Probe_Exec(t *testing.T) {
	yaml := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
spec:
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
        - name: redis
          image: redis:7
          livenessProbe:
            exec:
              command: ["redis-cli", "ping"]
            periodSeconds: 10
            timeoutSeconds: 5
            failureThreshold: 3
            initialDelaySeconds: 20
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["redis"]
	if svc.Healthcheck == nil {
		t.Fatal("missing healthcheck")
	}
	if len(svc.Healthcheck.Test) < 2 || svc.Healthcheck.Test[0] != "CMD" {
		t.Errorf("Test = %v", svc.Healthcheck.Test)
	}
	if svc.Healthcheck.Interval != "10s" {
		t.Errorf("Interval = %q", svc.Healthcheck.Interval)
	}
	if svc.Healthcheck.Timeout != "5s" {
		t.Errorf("Timeout = %q", svc.Healthcheck.Timeout)
	}
	if svc.Healthcheck.Retries != 3 {
		t.Errorf("Retries = %d", svc.Healthcheck.Retries)
	}
	if svc.Healthcheck.StartPeriod != "20s" {
		t.Errorf("StartPeriod = %q", svc.Healthcheck.StartPeriod)
	}
}

func TestTranslate_Probe_HTTP(t *testing.T) {
	yaml := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
        - name: web
          image: web:latest
          ports:
            - containerPort: 8080
          livenessProbe:
            httpGet:
              path: /health
              port: 8080
            periodSeconds: 15
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["web"]
	if svc.Healthcheck == nil {
		t.Fatal("missing healthcheck")
	}
	if svc.Healthcheck.Test[0] != "CMD" {
		t.Errorf("Test = %v, want CMD", svc.Healthcheck.Test)
	}
	found := false
	for _, arg := range svc.Healthcheck.Test[1:] {
		if strings.Contains(arg, "localhost:8080/health") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Test = %v, should contain URL", svc.Healthcheck.Test)
	}
}

func TestTranslate_Probe_TCP(t *testing.T) {
	yaml := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: db
spec:
  selector:
    matchLabels:
      app: db
  template:
    metadata:
      labels:
        app: db
    spec:
      containers:
        - name: db
          image: postgres:15
          livenessProbe:
            tcpSocket:
              port: 5432
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["db"]
	if svc.Healthcheck == nil {
		t.Fatal("missing healthcheck")
	}
	if !strings.Contains(strings.Join(svc.Healthcheck.Test, " "), "5432") {
		t.Errorf("Test = %v, should check port 5432", svc.Healthcheck.Test)
	}
}

func TestTranslate_Probe_FallbackToReadiness(t *testing.T) {
	yaml := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
        - name: app
          image: myapp:latest
          readinessProbe:
            exec:
              command: ["cat", "/tmp/ready"]
            periodSeconds: 5
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["app"]
	if svc.Healthcheck == nil {
		t.Fatal("readinessProbe should be used as fallback")
	}
}

func TestTranslate_ResourceLimits(t *testing.T) {
	yaml := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
        - name: app
          image: myapp:latest
          resources:
            limits:
              cpu: "500m"
              memory: "256Mi"
            requests:
              cpu: "250m"
              memory: "128Mi"
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["app"]
	if svc.Deploy == nil || svc.Deploy.Resources == nil {
		t.Fatal("missing deploy resources")
	}
	if svc.Deploy.Resources.Limits == nil {
		t.Fatal("missing limits")
	}
	if svc.Deploy.Resources.Limits.CPUs != "0.50" {
		t.Errorf("Limits.CPUs = %q, want %q", svc.Deploy.Resources.Limits.CPUs, "0.50")
	}
	if svc.Deploy.Resources.Limits.Memory != "256M" {
		t.Errorf("Limits.Memory = %q, want %q", svc.Deploy.Resources.Limits.Memory, "256M")
	}
	if svc.Deploy.Resources.Reservations == nil {
		t.Fatal("missing reservations")
	}
	if svc.Deploy.Resources.Reservations.CPUs != "0.25" {
		t.Errorf("Reservations.CPUs = %q", svc.Deploy.Resources.Reservations.CPUs)
	}
	if svc.Deploy.Resources.Reservations.Memory != "128M" {
		t.Errorf("Reservations.Memory = %q", svc.Deploy.Resources.Reservations.Memory)
	}
}

func TestTranslate_MultiContainer(t *testing.T) {
	yaml := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
        - name: main
          image: myapp:latest
        - name: sidecar
          image: envoy:latest
`
	result := mustTranslate(t, yaml, Opts{})

	// First container uses deployment name
	if result.Compose.Services["app"] == nil {
		t.Error("missing main container as 'app'")
	}
	// Second container uses deployment-containername
	if result.Compose.Services["app-sidecar"] == nil {
		t.Error("missing sidecar as 'app-sidecar'")
	}
}

func TestTranslate_StatefulSet(t *testing.T) {
	yaml := `
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: redis
spec:
  serviceName: redis
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
        - name: redis
          image: redis:7
          volumeMounts:
            - name: data
              mountPath: /data
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        resources:
          requests:
            storage: 1Gi
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["redis"]
	if svc == nil {
		t.Fatal("missing redis service")
	}

	// VolumeClaimTemplates → named volumes
	if _, ok := result.Compose.Volumes["data"]; !ok {
		t.Error("missing 'data' volume from VolumeClaimTemplate")
	}

	found := false
	for _, v := range svc.Volumes {
		if v == "data:/data" {
			found = true
		}
	}
	if !found {
		t.Errorf("Volumes = %v, want 'data:/data'", svc.Volumes)
	}
}

func TestTranslate_DaemonSet(t *testing.T) {
	yaml := `
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: agent
spec:
  selector:
    matchLabels:
      app: agent
  template:
    metadata:
      labels:
        app: agent
    spec:
      containers:
        - name: agent
          image: agent:latest
`
	result := mustTranslate(t, yaml, Opts{})
	svc := result.Compose.Services["agent"]
	if svc == nil {
		t.Fatal("missing agent service")
	}
	// DaemonSets have no replicas
	if svc.Deploy != nil && svc.Deploy.Replicas != nil {
		t.Error("DaemonSet should not have replicas")
	}
}

func TestTranslate_SkipKinds(t *testing.T) {
	yaml := `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
        - name: app
          image: myapp:latest
---
apiVersion: batch/v1
kind: Job
metadata:
  name: migrate
spec:
  template:
    spec:
      containers:
        - name: m
          image: myapp:latest
      restartPolicy: Never
`
	result := mustTranslate(t, yaml, Opts{SkipKinds: []string{"Job"}})
	if result.Compose.Services["migrate"] != nil {
		t.Error("Job should have been skipped")
	}
	if result.Compose.Services["app"] == nil {
		t.Error("Deployment should not be skipped")
	}
}

func TestTranslate_Project(t *testing.T) {
	yaml := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
        - name: app
          image: myapp:latest
`
	result := mustTranslate(t, yaml, Opts{Project: "my-project"})
	if result.Compose.Project != "my-project" {
		t.Errorf("Project = %q", result.Compose.Project)
	}
}

func TestTranslate_Report(t *testing.T) {
	yaml := `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
        - name: app
          image: myapp:latest
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: sa
`
	result := mustTranslate(t, yaml, Opts{})

	found := false
	for _, e := range result.Report.Translated {
		if e.Kind == "Deployment" && e.Name == "app" {
			found = true
		}
	}
	if !found {
		t.Error("Deployment/app should be in Translated report")
	}

	found = false
	for _, e := range result.Report.Skipped {
		if e.Kind == "ServiceAccount" {
			found = true
		}
	}
	if !found {
		t.Error("ServiceAccount should be in Skipped report")
	}
}

func TestReport_Print(t *testing.T) {
	r := &Report{
		Translated: []ReportEntry{{Kind: "Deployment", Name: "app"}},
		Skipped:    []ReportEntry{{Kind: "ServiceAccount", Name: "sa"}},
		Warnings:   []string{"Secret inlined"},
	}

	var buf bytes.Buffer
	r.Print(&buf)
	out := buf.String()

	if !strings.Contains(out, "Deployment/app") {
		t.Error("missing translated entry")
	}
	if !strings.Contains(out, "ServiceAccount/sa") {
		t.Error("missing skipped entry")
	}
	if !strings.Contains(out, "Secret inlined") {
		t.Error("missing warning")
	}
}

func TestSerializeLabels(t *testing.T) {
	labels := map[string]string{"app": "web", "tier": "frontend"}
	result := serializeLabels(labels)
	// Should be sorted
	if result != "app=web,tier=frontend" {
		t.Errorf("serializeLabels = %q", result)
	}
}

func TestSerializeLabels_Empty(t *testing.T) {
	result := serializeLabels(nil)
	if result != "" {
		t.Errorf("serializeLabels(nil) = %q", result)
	}
}

func TestLabelsMatch(t *testing.T) {
	tests := []struct {
		selector, pod string
		want          bool
	}{
		{"app=web", "app=web,tier=frontend", true},
		{"app=web,tier=frontend", "app=web,tier=frontend", true},
		{"app=web,tier=backend", "app=web,tier=frontend", false},
		{"", "app=web", false},
		{"app=web", "", false},
	}

	for _, tt := range tests {
		got := labelsMatch(tt.selector, tt.pod)
		if got != tt.want {
			t.Errorf("labelsMatch(%q, %q) = %v, want %v", tt.selector, tt.pod, got, tt.want)
		}
	}
}

func TestContainsStr(t *testing.T) {
	tests := []struct {
		slice []string
		s     string
		want  bool
	}{
		{[]string{"a", "b", "c"}, "b", true},
		{[]string{"a", "b"}, "c", false},
		{nil, "a", false},
		{[]string{}, "a", false},
	}
	for _, tt := range tests {
		got := containsStr(tt.slice, tt.s)
		if got != tt.want {
			t.Errorf("containsStr(%v, %q) = %v", tt.slice, tt.s, got)
		}
	}
}

func TestConvertResourceSpec(t *testing.T) {
	tests := []struct {
		name    string
		cpu     string
		memory  string
		wantCPU string
		wantMem string
	}{
		{"millicpu", "500m", "256Mi", "0.50", "256M"},
		{"whole cpu", "2", "1Gi", "2.00", "1G"},
		{"quarter cpu", "250m", "128Mi", "0.25", "128M"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
        - name: app
          image: myapp:latest
          resources:
            limits:
              cpu: "` + tt.cpu + `"
              memory: "` + tt.memory + `"
`
			result := mustTranslate(t, yaml, Opts{})
			svc := result.Compose.Services["app"]
			if svc.Deploy.Resources.Limits.CPUs != tt.wantCPU {
				t.Errorf("CPUs = %q, want %q", svc.Deploy.Resources.Limits.CPUs, tt.wantCPU)
			}
			if svc.Deploy.Resources.Limits.Memory != tt.wantMem {
				t.Errorf("Memory = %q, want %q", svc.Deploy.Resources.Limits.Memory, tt.wantMem)
			}
		})
	}
}

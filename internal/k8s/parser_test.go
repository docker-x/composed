package k8s

import (
	"testing"
)

const (
	errFmtParse = "Parse error: %v"
	errFmtName  = "Name = %q"
)

func TestParse_Deployment(t *testing.T) {
	yaml := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: default
spec:
  replicas: 3
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
`
	m, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf(errFmtParse, err)
	}
	if len(m.Deployments) != 1 {
		t.Fatalf("Deployments count = %d, want 1", len(m.Deployments))
	}
	dep := m.Deployments[0]
	if dep.Name != "nginx" {
		t.Errorf("Name = %q, want %q", dep.Name, "nginx")
	}
	if *dep.Spec.Replicas != 3 {
		t.Errorf("Replicas = %d, want 3", *dep.Spec.Replicas)
	}
	if dep.Spec.Template.Spec.Containers[0].Image != "nginx:1.25" {
		t.Errorf("Image = %q", dep.Spec.Template.Spec.Containers[0].Image)
	}
}

func TestParse_MultiDoc(t *testing.T) {
	yaml := `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config
data:
  key1: value1
---
apiVersion: v1
kind: Secret
metadata:
  name: my-secret
stringData:
  password: secret123
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
---
apiVersion: v1
kind: Service
metadata:
  name: app-svc
spec:
  selector:
    app: test
  ports:
    - port: 80
      targetPort: 8080
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: data-pvc
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
`
	m, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf(errFmtParse, err)
	}
	if len(m.ConfigMaps) != 1 {
		t.Errorf("ConfigMaps = %d, want 1", len(m.ConfigMaps))
	}
	if len(m.Secrets) != 1 {
		t.Errorf("Secrets = %d, want 1", len(m.Secrets))
	}
	if len(m.Deployments) != 1 {
		t.Errorf("Deployments = %d, want 1", len(m.Deployments))
	}
	if len(m.Services) != 1 {
		t.Errorf("Services = %d, want 1", len(m.Services))
	}
	if len(m.PVCs) != 1 {
		t.Errorf("PVCs = %d, want 1", len(m.PVCs))
	}

	if m.ConfigMaps[0].Data["key1"] != "value1" {
		t.Errorf("ConfigMap data[key1] = %q", m.ConfigMaps[0].Data["key1"])
	}
}

func TestParse_StatefulSet(t *testing.T) {
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
  volumeClaimTemplates:
    - metadata:
        name: redis-data
      spec:
        accessModes: ["ReadWriteOnce"]
        resources:
          requests:
            storage: 1Gi
`
	m, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf(errFmtParse, err)
	}
	if len(m.StatefulSets) != 1 {
		t.Fatalf("StatefulSets count = %d, want 1", len(m.StatefulSets))
	}
	ss := m.StatefulSets[0]
	if ss.Name != "redis" {
		t.Errorf(errFmtName, ss.Name)
	}
	if len(ss.Spec.VolumeClaimTemplates) != 1 {
		t.Errorf("VolumeClaimTemplates = %d", len(ss.Spec.VolumeClaimTemplates))
	}
}

func TestParse_DaemonSet(t *testing.T) {
	yaml := `
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: fluentd
spec:
  selector:
    matchLabels:
      app: fluentd
  template:
    metadata:
      labels:
        app: fluentd
    spec:
      containers:
        - name: fluentd
          image: fluentd:v1.16
`
	m, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf(errFmtParse, err)
	}
	if len(m.DaemonSets) != 1 {
		t.Fatalf("DaemonSets count = %d, want 1", len(m.DaemonSets))
	}
	if m.DaemonSets[0].Name != "fluentd" {
		t.Errorf(errFmtName, m.DaemonSets[0].Name)
	}
}

func TestParse_Job(t *testing.T) {
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
	m, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf(errFmtParse, err)
	}
	if len(m.Jobs) != 1 {
		t.Fatalf("Jobs count = %d, want 1", len(m.Jobs))
	}
	if m.Jobs[0].Name != "db-migrate" {
		t.Errorf(errFmtName, m.Jobs[0].Name)
	}
}

func TestParse_CronJob(t *testing.T) {
	yaml := `
apiVersion: batch/v1
kind: CronJob
metadata:
  name: backup
spec:
  schedule: "0 2 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: backup
              image: backup:latest
          restartPolicy: Never
`
	m, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf(errFmtParse, err)
	}
	if len(m.CronJobs) != 1 {
		t.Fatalf("CronJobs count = %d, want 1", len(m.CronJobs))
	}
	if m.CronJobs[0].Name != "backup" {
		t.Errorf(errFmtName, m.CronJobs[0].Name)
	}
}

func TestParse_SkippedResources(t *testing.T) {
	yaml := `---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: my-sa
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: my-role
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: my-netpol
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-ingress
`
	m, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf(errFmtParse, err)
	}
	if len(m.Skipped) != 4 {
		t.Fatalf("Skipped count = %d, want 4", len(m.Skipped))
	}

	kinds := make(map[string]bool)
	for _, s := range m.Skipped {
		kinds[s.Kind] = true
	}
	for _, k := range []string{"ServiceAccount", "ClusterRole", "NetworkPolicy", "Ingress"} {
		if !kinds[k] {
			t.Errorf("missing skipped kind %q", k)
		}
	}
}

func TestParse_EmptyDoc(t *testing.T) {
	yaml := `---
---
`
	m, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf(errFmtParse, err)
	}
	if len(m.Deployments) != 0 {
		t.Errorf("should have no deployments")
	}
}

func TestParse_EmptyKind(t *testing.T) {
	yaml := `
apiVersion: v1
metadata:
  name: no-kind
`
	m, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf(errFmtParse, err)
	}
	// No kind → skipped silently (not added to Skipped list)
	if len(m.Skipped) != 0 {
		t.Errorf("empty kind should not be in Skipped")
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	_, err := Parse([]byte(`{invalid json in yaml context`))
	if err == nil {
		t.Fatal("expected error for invalid input")
	}
}

func TestSplitYAMLDocs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"single doc", "kind: Deployment\nmetadata:\n  name: x", 1},
		{"two docs", "kind: A\n---\nkind: B", 2},
		{"leading separator", "---\nkind: A\n---\nkind: B", 2},
		{"empty docs filtered", "---\n\n---\nkind: A\n---\n\n", 1},
		{"three docs", "kind: A\n---\nkind: B\n---\nkind: C", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs, err := splitYAMLDocs([]byte(tt.input))
			if err != nil {
				t.Fatalf("splitYAMLDocs() error: %v", err)
			}
			if len(docs) != tt.want {
				t.Errorf("splitYAMLDocs() returned %d docs, want %d", len(docs), tt.want)
			}
		})
	}
}

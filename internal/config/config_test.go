package config

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	testChartBitnamiRedis = "bitnami/redis"
	testImagePostgres     = "postgres:15-alpine"
)

func TestServiceType(t *testing.T) {
	tests := []struct {
		name string
		svc  Service
		want string
	}{
		{
			name: "helm service",
			svc:  Service{XHelm: &HelmExtension{Chart: testChartBitnamiRedis}},
			want: "helm",
		},
		{
			name: "compose service",
			svc:  Service{XComposeFile: "./docker-compose.yaml"},
			want: "compose",
		},
		{
			name: "image service",
			svc:  Service{Image: testImagePostgres},
			want: "image",
		},
		{
			name: "empty service defaults to image",
			svc:  Service{},
			want: "image",
		},
		{
			name: "helm takes priority over compose",
			svc: Service{
				XHelm:        &HelmExtension{Chart: testChartBitnamiRedis},
				XComposeFile: "./docker-compose.yaml",
			},
			want: "helm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ServiceType(&tt.svc)
			if got != tt.want {
				t.Errorf("ServiceType() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- TestParse helper functions ---

func checkMinimalConfig(t *testing.T, f *File) {
	t.Helper()
	if f.Name != "my-stack" {
		t.Errorf("Name = %q, want %q", f.Name, "my-stack")
	}
	if len(f.Services) != 1 {
		t.Fatalf("Services count = %d, want 1", len(f.Services))
	}
	svc := f.Services["postgres"]
	if svc.Image != testImagePostgres {
		t.Errorf("Image = %q, want %q", svc.Image, testImagePostgres)
	}
}

func checkHelmExtension(t *testing.T, f *File) {
	t.Helper()
	svc := f.Services["redis"]
	if svc.XHelm == nil {
		t.Fatal("XHelm is nil")
	}
	if svc.XHelm.Chart != testChartBitnamiRedis {
		t.Errorf("Chart = %q, want %q", svc.XHelm.Chart, testChartBitnamiRedis)
	}
	if svc.XHelm.Repo != "https://charts.bitnami.com/bitnami" {
		t.Errorf("Repo = %q", svc.XHelm.Repo)
	}
	if svc.XHelm.Version != "18.x" {
		t.Errorf("Version = %q", svc.XHelm.Version)
	}
	if svc.XHelm.ValuesFile != "./redis-values.yaml" {
		t.Errorf("ValuesFile = %q", svc.XHelm.ValuesFile)
	}
	if v, ok := svc.XHelm.Values["architecture"]; !ok || v != "standalone" {
		t.Errorf("Values[architecture] = %v", v)
	}
}

func checkComposeFile(t *testing.T, f *File) {
	t.Helper()
	svc := f.Services["monitoring"]
	if svc.XComposeFile != "./monitoring/docker-compose.yaml" {
		t.Errorf("XComposeFile = %q", svc.XComposeFile)
	}
}

func checkExports(t *testing.T, f *File) {
	t.Helper()
	svc := f.Services["postgres"]
	if svc.XExports["host"] != "postgres" {
		t.Errorf("XExports[host] = %q", svc.XExports["host"])
	}
	if svc.XExports["password"] != "secret" {
		t.Errorf("XExports[password] = %q", svc.XExports["password"])
	}
}

func checkFullServiceFields(t *testing.T, f *File) {
	t.Helper()
	svc := f.Services["app"]
	if svc.Image != "myapp:latest" {
		t.Errorf("Image = %q", svc.Image)
	}
	if len(svc.Command) != 2 || svc.Command[0] != "serve" {
		t.Errorf("Command = %v", svc.Command)
	}
	if len(svc.Entrypoint) != 2 {
		t.Errorf("Entrypoint = %v", svc.Entrypoint)
	}
	if svc.Environment["FOO"] != "bar" {
		t.Errorf("Environment[FOO] = %q", svc.Environment["FOO"])
	}
	if len(svc.Ports) != 1 || svc.Ports[0] != "8080:8080" {
		t.Errorf("Ports = %v", svc.Ports)
	}
	if len(svc.Volumes) != 1 || svc.Volumes[0] != "data:/data" {
		t.Errorf("Volumes = %v", svc.Volumes)
	}
	if svc.Labels["team"] != "backend" {
		t.Errorf("Labels = %v", svc.Labels)
	}
	if len(svc.DependsOn) != 1 || svc.DependsOn[0] != "postgres" {
		t.Errorf("DependsOn = %v", svc.DependsOn)
	}
	if svc.Restart != "unless-stopped" {
		t.Errorf("Restart = %q", svc.Restart)
	}
	if svc.Healthcheck == nil {
		t.Fatal("Healthcheck is nil")
	}
	if svc.Healthcheck.Retries != 3 {
		t.Errorf("Healthcheck.Retries = %d", svc.Healthcheck.Retries)
	}
}

func checkEmptyConfig(t *testing.T, f *File) {
	t.Helper()
	if f.Services == nil {
		t.Fatal("Services should be initialized, not nil")
	}
	if len(f.Services) != 0 {
		t.Errorf("Services count = %d, want 0", len(f.Services))
	}
}

func checkTopLevelShellShorthand(t *testing.T, f *File) {
	t.Helper()
	entry, ok := f.XShell["sso-token"]
	if !ok {
		t.Fatal("x-shell entry 'sso-token' not found")
	}
	if entry.Command != "echo my-token" {
		t.Errorf("Command = %q, want %q", entry.Command, "echo my-token")
	}
	if entry.AllowFailure {
		t.Error("AllowFailure should be false for shorthand")
	}
}

func checkTopLevelShellLongForm(t *testing.T, f *File) {
	t.Helper()
	entry, ok := f.XShell["sso-token"]
	if !ok {
		t.Fatal("x-shell entry 'sso-token' not found")
	}
	if entry.Command != "vault kv get -field=token secret/myapp" {
		t.Errorf("Command = %q", entry.Command)
	}
	if !entry.AllowFailure {
		t.Error("AllowFailure should be true")
	}
}

func checkTopLevelShellMixed(t *testing.T, f *File) {
	t.Helper()
	if len(f.XShell) != 2 {
		t.Fatalf("expected 2 x-shell entries, got %d", len(f.XShell))
	}
	short, ok := f.XShell["quick"]
	if !ok {
		t.Fatal("x-shell entry 'quick' not found")
	}
	if short.Command != "echo fast" {
		t.Errorf("quick.Command = %q", short.Command)
	}
	long, ok := f.XShell["careful"]
	if !ok {
		t.Fatal("x-shell entry 'careful' not found")
	}
	if long.Command != "echo slow" {
		t.Errorf("careful.Command = %q", long.Command)
	}
	if !long.AllowFailure {
		t.Error("careful.AllowFailure should be true")
	}
}

func checkNoShellEntries(t *testing.T, f *File) {
	t.Helper()
	if f.XShell == nil {
		t.Fatal("XShell should be initialized, not nil")
	}
	if len(f.XShell) != 0 {
		t.Errorf("expected 0 x-shell entries, got %d", len(f.XShell))
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, f *File)
	}{
		{
			name: "minimal config",
			input: `
name: my-stack
services:
  postgres:
    image: postgres:15-alpine
`,
			check: checkMinimalConfig,
		},
		{
			name: "helm extension parsed",
			input: `
name: test
services:
  redis:
    x-helm:
      chart: bitnami/redis
      repo: https://charts.bitnami.com/bitnami
      version: "18.x"
      values:
        architecture: standalone
      values_file: ./redis-values.yaml
`,
			check: checkHelmExtension,
		},
		{
			name: "compose file extension parsed",
			input: `
name: test
services:
  monitoring:
    x-compose-file: ./monitoring/docker-compose.yaml
`,
			check: checkComposeFile,
		},
		{
			name: "exports parsed",
			input: `
name: test
services:
  postgres:
    image: postgres:15
    x-exports:
      host: postgres
      password: secret
`,
			check: checkExports,
		},
		{
			name: "full service fields",
			input: `
name: test
services:
  app:
    image: myapp:latest
    command:
      - serve
      - --port=8080
    entrypoint:
      - /bin/sh
      - -c
    environment:
      FOO: bar
    ports:
      - "8080:8080"
    volumes:
      - data:/data
    labels:
      team: backend
    depends_on:
      - postgres
    restart: unless-stopped
    healthcheck:
      test:
        - CMD
        - curl
        - -f
        - http://localhost:8080/health
      interval: 10s
      timeout: 5s
      retries: 3
`,
			check: checkFullServiceFields,
		},
		{
			name:  "empty config",
			input: `name: empty`,
			check: checkEmptyConfig,
		},
		{
			name: "top-level x-shell shorthand",
			input: `
name: test
x-shell:
  sso-token: "echo my-token"
services:
  app:
    image: myapp
`,
			check: checkTopLevelShellShorthand,
		},
		{
			name: "top-level x-shell long form",
			input: `
name: test
x-shell:
  sso-token:
    command: "vault kv get -field=token secret/myapp"
    allow_failure: true
services:
  app:
    image: myapp
`,
			check: checkTopLevelShellLongForm,
		},
		{
			name: "top-level x-shell mixed forms",
			input: `
name: test
x-shell:
  quick: "echo fast"
  careful:
    command: "echo slow"
    allow_failure: true
services:
  app:
    image: myapp
`,
			check: checkTopLevelShellMixed,
		},
		{
			name: "no x-shell entries",
			input: `
name: test
services:
  app:
    image: myapp
`,
			check: checkNoShellEntries,
		},
		{
			name:    "invalid yaml",
			input:   `[invalid`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, f)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	t.Run("valid file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "composed.yaml")
		err := os.WriteFile(path, []byte(`
name: test
services:
  web:
    image: nginx:latest
`), 0644)
		if err != nil {
			t.Fatal(err)
		}

		f, err := Load(path)
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if f.Name != "test" {
			t.Errorf("Name = %q, want %q", f.Name, "test")
		}
		if f.Services["web"].Image != "nginx:latest" {
			t.Errorf("Image = %q", f.Services["web"].Image)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := Load("/nonexistent/composed.yaml")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})
}

// --- TestResolveRefs helper functions ---

func checkEnvVarResolved(t *testing.T, f *File) {
	t.Helper()
	app := f.Services["app"]
	if app.Environment["DB_HOST"] != "postgres" {
		t.Errorf("DB_HOST = %q, want %q", app.Environment["DB_HOST"], "postgres")
	}
	if app.Environment["DB_PASS"] != "secret" {
		t.Errorf("DB_PASS = %q, want %q", app.Environment["DB_PASS"], "secret")
	}
}

func checkConnectionString(t *testing.T, f *File) {
	t.Helper()
	app := f.Services["app"]
	want := "postgresql://user:secret@postgres:5432/db"
	if app.Environment["DATABASE_URL"] != want {
		t.Errorf("DATABASE_URL = %q, want %q", app.Environment["DATABASE_URL"], want)
	}
}

func checkHelmValuesResolved(t *testing.T, f *File) {
	t.Helper()
	litellm := f.Services["litellm"]
	if litellm.XHelm.Values["db_password"] != "secret123" {
		t.Errorf("Values[db_password] = %v", litellm.XHelm.Values["db_password"])
	}
}

func checkNoOpNoRefs(t *testing.T, f *File) {
	t.Helper()
	web := f.Services["web"]
	if web.Environment["PORT"] != "8080" {
		t.Errorf("PORT = %q", web.Environment["PORT"])
	}
}

func checkUnresolvedRef(t *testing.T, f *File) {
	t.Helper()
	app := f.Services["app"]
	if app.Environment["DB_HOST"] != "${nonexistent.host}" {
		t.Errorf("DB_HOST = %q, want unresolved placeholder", app.Environment["DB_HOST"])
	}
}

func checkMalformedPortsRef(t *testing.T, f *File) {
	t.Helper()
	app := f.Services["app"]
	// Missing closing bracket — must stay unresolved
	if app.Environment["PG_PORT"] != "${postgres.ports[0}" {
		t.Errorf("PG_PORT = %q, want unresolved placeholder", app.Environment["PG_PORT"])
	}
}

func checkDirectEnvRef(t *testing.T, f *File) {
	t.Helper()
	app := f.Services["app"]
	if app.Environment["DB_PASS"] != "secret" {
		t.Errorf("DB_PASS = %q, want %q", app.Environment["DB_PASS"], "secret")
	}
}

func checkDirectHostname(t *testing.T, f *File) {
	t.Helper()
	app := f.Services["app"]
	if app.Environment["DB_HOST"] != "postgres" {
		t.Errorf("DB_HOST = %q, want %q", app.Environment["DB_HOST"], "postgres")
	}
}

func checkDirectImage(t *testing.T, f *File) {
	t.Helper()
	app := f.Services["app"]
	if app.Environment["PG_IMAGE"] != "postgres:15" {
		t.Errorf("PG_IMAGE = %q, want %q", app.Environment["PG_IMAGE"], "postgres:15")
	}
}

func checkDirectPortsIndex(t *testing.T, f *File) {
	t.Helper()
	app := f.Services["app"]
	if app.Environment["PG_PORT"] != "5432:5432" {
		t.Errorf("PG_PORT = %q, want %q", app.Environment["PG_PORT"], "5432:5432")
	}
}

func checkExportOverridesDirect(t *testing.T, f *File) {
	t.Helper()
	app := f.Services["app"]
	// x-exports takes priority over direct reference
	if app.Environment["PASSWORD"] != "exported-secret" {
		t.Errorf("PASSWORD = %q, want %q", app.Environment["PASSWORD"], "exported-secret")
	}
	// direct reference still works for non-exported fields
	if app.Environment["ACTUAL_PASS"] != "real-secret" {
		t.Errorf("ACTUAL_PASS = %q, want %q", app.Environment["ACTUAL_PASS"], "real-secret")
	}
}

func TestResolveRefs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, f *File)
	}{
		{
			name: "resolve env var reference",
			input: `
name: test
services:
  postgres:
    image: postgres:15
    environment:
      POSTGRES_PASSWORD: secret
    x-exports:
      host: postgres
      password: secret
  app:
    image: myapp:latest
    environment:
      DB_HOST: "${postgres.host}"
      DB_PASS: "${postgres.password}"
`,
			check: checkEnvVarResolved,
		},
		{
			name: "resolve in connection string",
			input: `
name: test
services:
  postgres:
    image: postgres:15
    x-exports:
      host: postgres
      password: secret
  app:
    image: myapp:latest
    environment:
      DATABASE_URL: "postgresql://user:${postgres.password}@${postgres.host}:5432/db"
`,
			check: checkConnectionString,
		},
		{
			name: "resolve in helm values",
			input: `
name: test
services:
  postgres:
    image: postgres:15
    x-exports:
      password: secret123
  litellm:
    x-helm:
      chart: litellm
      values:
        db_password: "${postgres.password}"
`,
			check: checkHelmValuesResolved,
		},
		{
			name: "no-op when no refs",
			input: `
name: test
services:
  web:
    image: nginx
    environment:
      PORT: "8080"
`,
			check: checkNoOpNoRefs,
		},
		{
			name: "unresolved ref left as-is",
			input: `
name: test
services:
  app:
    image: myapp
    environment:
      DB_HOST: "${nonexistent.host}"
`,
			check: checkUnresolvedRef,
		},
		{
			name: "direct environment reference",
			input: `
name: test
services:
  postgres:
    image: postgres:15
    environment:
      POSTGRES_PASSWORD: secret
  app:
    image: myapp
    environment:
      DB_PASS: "${postgres.environment.POSTGRES_PASSWORD}"
`,
			check: checkDirectEnvRef,
		},
		{
			name: "direct hostname reference",
			input: `
name: test
services:
  postgres:
    image: postgres:15
  app:
    image: myapp
    environment:
      DB_HOST: "${postgres.hostname}"
`,
			check: checkDirectHostname,
		},
		{
			name: "direct image reference",
			input: `
name: test
services:
  postgres:
    image: postgres:15
  app:
    image: myapp
    environment:
      PG_IMAGE: "${postgres.image}"
`,
			check: checkDirectImage,
		},
		{
			name: "direct ports index reference",
			input: `
name: test
services:
  postgres:
    image: postgres:15
    ports:
      - "5432:5432"
  app:
    image: myapp
    environment:
      PG_PORT: "${postgres.ports[0]}"
`,
			check: checkDirectPortsIndex,
		},
		{
			name: "malformed ports ref left as-is",
			input: `
name: test
services:
  postgres:
    image: postgres:15
    ports:
      - "5432:5432"
  app:
    image: myapp
    environment:
      PG_PORT: "${postgres.ports[0}"
`,
			check: checkMalformedPortsRef,
		},
		{
			name: "x-exports takes priority over direct",
			input: `
name: test
services:
  postgres:
    image: postgres:15
    environment:
      POSTGRES_PASSWORD: real-secret
    x-exports:
      password: exported-secret
  app:
    image: myapp
    environment:
      PASSWORD: "${postgres.password}"
      ACTUAL_PASS: "${postgres.environment.POSTGRES_PASSWORD}"
`,
			check: checkExportOverridesDirect,
		},
		{
			name: "direct ref in connection string",
			input: `
name: test
services:
  postgres:
    image: postgres:15
    environment:
      POSTGRES_PASSWORD: secret
  app:
    image: myapp
    environment:
      DATABASE_URL: "postgresql://user:${postgres.environment.POSTGRES_PASSWORD}@${postgres.hostname}:5432/db"
`,
			check: checkConnectionString,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			if err := f.ResolveRefs(nil); err != nil {
				t.Fatalf("ResolveRefs error: %v", err)
			}
			tt.check(t, f)
		})
	}
}

func TestResolveMap(t *testing.T) {
	exports := map[string]string{
		"db.host":     "localhost",
		"db.password": "secret",
	}

	input := map[string]interface{}{
		"connection": "${db.host}",
		"nested": map[string]interface{}{
			"pass": "${db.password}",
			"port": 5432,
		},
	}

	result := resolveMap(input, exports, nil)

	if result["connection"] != "localhost" {
		t.Errorf("connection = %v", result["connection"])
	}
	nested := result["nested"].(map[string]interface{})
	if nested["pass"] != "secret" {
		t.Errorf("nested.pass = %v", nested["pass"])
	}
	if nested["port"] != 5432 {
		t.Errorf("nested.port = %v (should be unchanged)", nested["port"])
	}
}

func TestRunShellEntries(t *testing.T) {
	t.Run("captures stdout", func(t *testing.T) {
		entries := map[string]ShellEntry{
			"greeting": {Command: "echo hello-world"},
		}
		vals, err := RunShellEntries(entries)
		if err != nil {
			t.Fatalf("RunShellEntries error: %v", err)
		}
		if vals["greeting"] != "hello-world" {
			t.Errorf("greeting = %q, want %q", vals["greeting"], "hello-world")
		}
	})

	t.Run("failure aborts", func(t *testing.T) {
		entries := map[string]ShellEntry{
			"fail": {Command: "false"},
		}
		_, err := RunShellEntries(entries)
		if err == nil {
			t.Fatal("expected error from failing command")
		}
	})

	t.Run("allow_failure continues", func(t *testing.T) {
		entries := map[string]ShellEntry{
			"fail": {Command: "false", AllowFailure: true},
		}
		vals, err := RunShellEntries(entries)
		if err != nil {
			t.Fatalf("expected no error with allow_failure, got: %v", err)
		}
		if _, ok := vals["fail"]; ok {
			t.Error("failed entry should not have a value")
		}
	})
}

func TestResolveRefsWithShellValues(t *testing.T) {
	f, err := Parse([]byte(`
name: test
services:
  app:
    image: myapp
    environment:
      TOKEN: "${sso-token}"
      HOST: "${postgres.host}"
  postgres:
    image: postgres:15
    x-exports:
      host: postgres
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	shellValues := map[string]string{
		"sso-token": "my-secret-token",
	}

	if err := f.ResolveRefs(shellValues); err != nil {
		t.Fatalf("ResolveRefs error: %v", err)
	}

	app := f.Services["app"]
	if app.Environment["TOKEN"] != "my-secret-token" {
		t.Errorf("TOKEN = %q, want %q", app.Environment["TOKEN"], "my-secret-token")
	}
	if app.Environment["HOST"] != "postgres" {
		t.Errorf("HOST = %q, want %q", app.Environment["HOST"], "postgres")
	}
}

func TestParseVolumes(t *testing.T) {
	t.Run("external volume with name", func(t *testing.T) {
		f, err := Parse([]byte(`
name: test
services:
  app:
    image: myapp
volumes:
  data:
    external: true
    name: litellm-ext-db
`))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		vol, ok := f.Volumes["data"]
		if !ok {
			t.Fatal("volume 'data' not found")
		}
		if !vol.External {
			t.Error("External should be true")
		}
		if vol.Name != "litellm-ext-db" {
			t.Errorf("Name = %q, want %q", vol.Name, "litellm-ext-db")
		}
	})

	t.Run("volume with driver", func(t *testing.T) {
		f, err := Parse([]byte(`
name: test
services:
  app:
    image: myapp
volumes:
  logs:
    driver: tmpfs
`))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		vol := f.Volumes["logs"]
		if vol.Driver != "tmpfs" {
			t.Errorf("Driver = %q, want %q", vol.Driver, "tmpfs")
		}
		if vol.External {
			t.Error("External should be false")
		}
	})

	t.Run("empty volume", func(t *testing.T) {
		f, err := Parse([]byte(`
name: test
services:
  app:
    image: myapp
volumes:
  data:
`))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		if _, ok := f.Volumes["data"]; !ok {
			t.Fatal("volume 'data' not found")
		}
	})

	t.Run("no volumes", func(t *testing.T) {
		f, err := Parse([]byte(`
name: test
services:
  app:
    image: myapp
`))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		if f.Volumes == nil {
			t.Fatal("Volumes should be initialized, not nil")
		}
		if len(f.Volumes) != 0 {
			t.Errorf("expected 0 volumes, got %d", len(f.Volumes))
		}
	})
}

func TestInlineShellRef(t *testing.T) {
	f, err := Parse([]byte(`
name: test
services:
  app:
    image: myapp
    environment:
      GREETING: "${shell:echo inline-hello}"
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if err := f.ResolveRefs(nil); err != nil {
		t.Fatalf("ResolveRefs error: %v", err)
	}

	app := f.Services["app"]
	if app.Environment["GREETING"] != "inline-hello" {
		t.Errorf("GREETING = %q, want %q", app.Environment["GREETING"], "inline-hello")
	}
}

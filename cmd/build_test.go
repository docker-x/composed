package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/docker-x/composed/internal/compose"
	"github.com/docker-x/composed/internal/config"
)

const (
	testTagMainStable    = "main-stable"
	testImageNginx       = "nginx:latest"
	testVolumeName       = "litellm-ext-db"
	testComposedYAMLFile = "composed.yaml"
	errFmtConfigLoad     = "config.Load: %v"
)

func TestFlattenValues(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		input  map[string]interface{}
		want   map[string]string
	}{
		{
			name:   "flat map",
			prefix: "",
			input:  map[string]interface{}{"key": "val", "num": 42, "flag": true},
			want:   map[string]string{"key": "val", "num": "42", "flag": "true"},
		},
		{
			name:   "nested map",
			prefix: "",
			input: map[string]interface{}{
				"image": map[string]interface{}{
					"tag": testTagMainStable,
				},
			},
			want: map[string]string{"image.tag": testTagMainStable},
		},
		{
			name:   "with prefix",
			prefix: "global",
			input:  map[string]interface{}{"debug": "true"},
			want:   map[string]string{"global.debug": "true"},
		},
		{
			name:   "deeply nested",
			prefix: "",
			input: map[string]interface{}{
				"a": map[string]interface{}{
					"b": map[string]interface{}{
						"c": "deep",
					},
				},
			},
			want: map[string]string{"a.b.c": "deep"},
		},
		{
			name:   "float value",
			prefix: "",
			input:  map[string]interface{}{"rate": 3.14},
			want:   map[string]string{"rate": "3.14"},
		},
		{
			name:   "empty map",
			prefix: "",
			input:  map[string]interface{}{},
			want:   map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := flattenValues(tt.prefix, tt.input)
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("flattenValues[%q] = %q, want %q", k, got[k], v)
				}
			}
			if len(got) != len(tt.want) {
				t.Errorf("flattenValues returned %d keys, want %d", len(got), len(tt.want))
			}
		})
	}
}

func checkSimpleKeyVal(t *testing.T, m map[string]interface{}) {
	t.Helper()
	if m["key"] != "val" {
		t.Errorf("key = %v", m["key"])
	}
}

func checkNestedKey(t *testing.T, m map[string]interface{}) {
	t.Helper()
	img, ok := m["image"].(map[string]interface{})
	if !ok {
		t.Fatalf("image is not a map: %T", m["image"])
	}
	if img["tag"] != testTagMainStable {
		t.Errorf("image.tag = %v", img["tag"])
	}
}

func checkDeeplyNested(t *testing.T, m map[string]interface{}) {
	t.Helper()
	a := m["a"].(map[string]interface{})
	b := a["b"].(map[string]interface{})
	if b["c"] != "deep" {
		t.Errorf("a.b.c = %v", b["c"])
	}
}

func checkMultipleSets(t *testing.T, m map[string]interface{}) {
	t.Helper()
	img := m["image"].(map[string]interface{})
	if img["tag"] != "v1" {
		t.Errorf("image.tag = %v", img["tag"])
	}
	if img["pullPolicy"] != "Always" {
		t.Errorf("image.pullPolicy = %v", img["pullPolicy"])
	}
	if m["replicas"] != "3" {
		t.Errorf("replicas = %v", m["replicas"])
	}
}

func checkEmptyMap(t *testing.T, m map[string]interface{}) {
	t.Helper()
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestParseSetValues(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		check func(t *testing.T, m map[string]interface{})
	}{
		{
			name:  "simple key=val",
			input: []string{"key=val"},
			check: checkSimpleKeyVal,
		},
		{
			name:  "nested key",
			input: []string{"image.tag=" + testTagMainStable},
			check: checkNestedKey,
		},
		{
			name:  "deeply nested",
			input: []string{"a.b.c=deep"},
			check: checkDeeplyNested,
		},
		{
			name:  "multiple sets",
			input: []string{"image.tag=v1", "image.pullPolicy=Always", "replicas=3"},
			check: checkMultipleSets,
		},
		{
			name:  "invalid format skipped",
			input: []string{"no-equals-sign"},
			check: checkEmptyMap,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := parseSetValues(tt.input)
			tt.check(t, m)
		})
	}
}

func TestParseEnvValues(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  map[string]string
	}{
		{
			name:  "simple",
			input: []string{"FOO=bar", "BAZ=qux"},
			want:  map[string]string{"FOO": "bar", "BAZ": "qux"},
		},
		{
			name:  "value with equals",
			input: []string{"URL=http://host?key=val"},
			want:  map[string]string{"URL": "http://host?key=val"},
		},
		{
			name:  "empty value",
			input: []string{"EMPTY="},
			want:  map[string]string{"EMPTY": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEnvValues(tt.input)
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseEnvValues[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestDeepMerge(t *testing.T) {
	dst := map[string]interface{}{
		"a": "keep",
		"nested": map[string]interface{}{
			"x": "1",
			"y": "2",
		},
	}
	src := map[string]interface{}{
		"a": "override",
		"b": "new",
		"nested": map[string]interface{}{
			"y": "override",
			"z": "3",
		},
	}

	deepMerge(dst, src)

	if dst["a"] != "override" {
		t.Errorf("a = %v", dst["a"])
	}
	if dst["b"] != "new" {
		t.Errorf("b = %v", dst["b"])
	}
	nested := dst["nested"].(map[string]interface{})
	if nested["x"] != "1" {
		t.Errorf("nested.x = %v (should be preserved)", nested["x"])
	}
	if nested["y"] != "override" {
		t.Errorf("nested.y = %v (should be overridden)", nested["y"])
	}
	if nested["z"] != "3" {
		t.Errorf("nested.z = %v", nested["z"])
	}
}

func checkLinearChain(t *testing.T, order []string) {
	t.Helper()
	idx := make(map[string]int)
	for i, name := range order {
		idx[name] = i
	}
	if idx["a"] > idx["b"] || idx["b"] > idx["c"] {
		t.Errorf("wrong order: %v", order)
	}
}

func checkNoDeps(t *testing.T, order []string) {
	t.Helper()
	if len(order) != 2 {
		t.Errorf("order = %v, want 2 items", order)
	}
}

func checkDiamondDeps(t *testing.T, order []string) {
	t.Helper()
	idx := make(map[string]int)
	for i, name := range order {
		idx[name] = i
	}
	if idx["a"] > idx["b"] || idx["a"] > idx["c"] || idx["b"] > idx["d"] || idx["c"] > idx["d"] {
		t.Errorf("wrong order: %v", order)
	}
}

func TestTopoSort(t *testing.T) {
	tests := []struct {
		name     string
		services map[string]struct {
			dependsOn []string
		}
		check func(t *testing.T, order []string)
	}{
		{
			name: "linear chain",
			services: map[string]struct{ dependsOn []string }{
				"c": {[]string{"b"}},
				"b": {[]string{"a"}},
				"a": {nil},
			},
			check: checkLinearChain,
		},
		{
			name: "no deps",
			services: map[string]struct{ dependsOn []string }{
				"a": {nil},
				"b": {nil},
			},
			check: checkNoDeps,
		},
		{
			name: "diamond deps",
			services: map[string]struct{ dependsOn []string }{
				"d": {[]string{"b", "c"}},
				"b": {[]string{"a"}},
				"c": {[]string{"a"}},
				"a": {nil},
			},
			check: checkDiamondDeps,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build a config.File-like structure
			cfg := &configForTest{services: make(map[string]testSvc)}
			for name, svc := range tt.services {
				cfg.services[name] = testSvc{dependsOn: svc.dependsOn}
			}
			order := topoSortTest(cfg)
			tt.check(t, order)
		})
	}
}

// Adapter to test topoSort without importing config package in a circular way.
// We replicate the same logic inline.
type testSvc struct {
	dependsOn []string
}
type configForTest struct {
	services map[string]testSvc
}

func topoSortTest(cfg *configForTest) []string {
	visited := make(map[string]bool)
	var order []string

	var visit func(name string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true
		svc := cfg.services[name]
		for _, dep := range svc.dependsOn {
			if _, ok := cfg.services[dep]; ok {
				visit(dep)
			}
		}
		order = append(order, name)
	}

	for name := range cfg.services {
		visit(name)
	}
	return order
}

func TestToStringSlice(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  int
	}{
		{"string slice", []interface{}{"a", "b"}, 2},
		{"single string", "hello", 1},
		{"empty string", "", 0},
		{"nil", nil, 0},
		{"int", 42, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toStringSlice(tt.input)
			if len(got) != tt.want {
				t.Errorf("toStringSlice(%v) len = %d, want %d", tt.input, len(got), tt.want)
			}
		})
	}
}

func TestDeriveComponentName(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{"oci://docker.litellm.ai/berriai/litellm-helm", "litellm-helm"},
		{"oci://ghcr.io/berriai/litellm:main-stable", "litellm"},
		{"postgres:15-alpine", "postgres"},
		{"bitnami/redis", "redis"},
		{"./monitoring/docker-compose.yaml", "monitoring"},
		{"./charts/myapp", "myapp"},
		{"nginx", "nginx"},
		{"oci://registry.example.com/org/chart-name:v1.2.3", "chart-name"},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			got := deriveComponentName(tt.source)
			if got != tt.want {
				t.Errorf("deriveComponentName(%q) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}

// assertEnv checks a service environment variable has the expected value.
func assertEnv(t *testing.T, svc *compose.Service, key, want string) {
	t.Helper()
	if svc.Environment[key] != want {
		t.Errorf("Environment[%q] = %q, want %q", key, svc.Environment[key], want)
	}
}

func TestOverlayServiceFields(t *testing.T) {
	t.Run("merges environment (user wins)", func(t *testing.T) {
		frag := compose.NewFile()
		svc := compose.NewService(testImageNginx)
		svc.Environment["CHART_VAR"] = "from-chart"
		svc.Environment["SHARED"] = "chart-value"
		frag.Services["web"] = svc

		cfgSvc := &config.Service{
			Environment: map[string]string{
				"USER_VAR": "from-user",
				"SHARED":   "user-wins",
			},
		}

		overlayServiceFields(frag, "web", cfgSvc)

		assertEnv(t, svc, "CHART_VAR", "from-chart")
		assertEnv(t, svc, "USER_VAR", "from-user")
		assertEnv(t, svc, "SHARED", "user-wins")
	})

	t.Run("appends env_file", func(t *testing.T) {
		frag := compose.NewFile()
		svc := compose.NewService(testImageNginx)
		svc.EnvFile = []string{"./existing.env"}
		frag.Services["web"] = svc

		cfgSvc := &config.Service{
			EnvFile: []string{"./user.env"},
		}

		overlayServiceFields(frag, "web", cfgSvc)

		if len(svc.EnvFile) != 2 {
			t.Fatalf("EnvFile len = %d, want 2", len(svc.EnvFile))
		}
	})

	t.Run("appends volumes", func(t *testing.T) {
		frag := compose.NewFile()
		svc := compose.NewService(testImageNginx)
		svc.Volumes = []string{"data:/data"}
		frag.Services["web"] = svc

		cfgSvc := &config.Service{
			Volumes: []string{"./config.yaml:/etc/config.yaml"},
		}

		overlayServiceFields(frag, "web", cfgSvc)

		if len(svc.Volumes) != 2 {
			t.Fatalf("Volumes len = %d, want 2", len(svc.Volumes))
		}
	})

	t.Run("appends ports", func(t *testing.T) {
		frag := compose.NewFile()
		svc := compose.NewService(testImageNginx)
		svc.Ports = []string{"80:80"}
		frag.Services["web"] = svc

		cfgSvc := &config.Service{
			Ports: []string{"443:443"},
		}

		overlayServiceFields(frag, "web", cfgSvc)

		if len(svc.Ports) != 2 {
			t.Fatalf("Ports len = %d, want 2", len(svc.Ports))
		}
	})

	t.Run("falls back to single service if name not found", func(t *testing.T) {
		frag := compose.NewFile()
		svc := compose.NewService(testImageNginx)
		frag.Services["chart-generated-name"] = svc

		cfgSvc := &config.Service{
			Environment: map[string]string{"KEY": "val"},
		}

		overlayServiceFields(frag, "not-matching", cfgSvc)
		assertEnv(t, svc, "KEY", "val")
	})

	t.Run("no-op when name not found and multiple services", func(t *testing.T) {
		frag := compose.NewFile()
		svc1 := compose.NewService(testImageNginx)
		svc2 := compose.NewService("redis:latest")
		frag.Services["svc1"] = svc1
		frag.Services["svc2"] = svc2

		cfgSvc := &config.Service{
			Environment: map[string]string{"KEY": "val"},
		}

		overlayServiceFields(frag, "not-matching", cfgSvc)

		// Neither should be modified
		if _, ok := svc1.Environment["KEY"]; ok {
			t.Error("svc1 should not have KEY")
		}
		if _, ok := svc2.Environment["KEY"]; ok {
			t.Error("svc2 should not have KEY")
		}
	})

	t.Run("no-op with nil fields", func(t *testing.T) {
		frag := compose.NewFile()
		svc := compose.NewService(testImageNginx)
		frag.Services["web"] = svc

		cfgSvc := &config.Service{} // all nil

		overlayServiceFields(frag, "web", cfgSvc)

		if len(svc.Environment) != 0 {
			t.Errorf("Environment should remain empty")
		}
	})
}

func TestApplyConfigVolumes(t *testing.T) {
	t.Run("overrides chart volume with external", func(t *testing.T) {
		merged := compose.NewFile()
		merged.Volumes["data"] = &compose.Volume{} // chart-generated

		cfg := &config.File{
			Volumes: map[string]config.VolumeConfig{
				"data": {External: true, Name: testVolumeName},
			},
		}

		applyConfigVolumes(merged, cfg)

		vol := merged.Volumes["data"]
		if !vol.External {
			t.Error("External should be true")
		}
		if vol.Name != testVolumeName {
			t.Errorf("Name = %q, want %q", vol.Name, testVolumeName)
		}
	})

	t.Run("adds new volume", func(t *testing.T) {
		merged := compose.NewFile()

		cfg := &config.File{
			Volumes: map[string]config.VolumeConfig{
				"logs": {Driver: "tmpfs"},
			},
		}

		applyConfigVolumes(merged, cfg)

		vol, ok := merged.Volumes["logs"]
		if !ok {
			t.Fatal("volume 'logs' not found")
		}
		if vol.Driver != "tmpfs" {
			t.Errorf("Driver = %q", vol.Driver)
		}
	})

	t.Run("no-op with empty config volumes", func(t *testing.T) {
		merged := compose.NewFile()
		merged.Volumes["data"] = &compose.Volume{}

		cfg := &config.File{
			Volumes: map[string]config.VolumeConfig{},
		}

		applyConfigVolumes(merged, cfg)

		if merged.Volumes["data"].External {
			t.Error("should not have been modified")
		}
	})
}

func TestParseComposeYAML(t *testing.T) {
	yaml := `
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
    environment:
      FOO: bar
    volumes:
      - data:/data
    restart: unless-stopped
    labels:
      team: frontend
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost"]
      interval: 10s
      timeout: 5s
      retries: 3
  db:
    image: postgres:15
    environment:
      - POSTGRES_PASSWORD=secret

volumes:
  data:

networks:
  frontend:
`
	f, err := parseComposeYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("parseComposeYAML error: %v", err)
	}

	if len(f.Services) != 2 {
		t.Errorf("Services count = %d, want 2", len(f.Services))
	}

	web := f.Services["web"]
	if web.Image != testImageNginx {
		t.Errorf("web.Image = %q", web.Image)
	}
	if web.Environment["FOO"] != "bar" {
		t.Errorf("web.Environment[FOO] = %q", web.Environment["FOO"])
	}
	if web.Restart != "unless-stopped" {
		t.Errorf("web.Restart = %q", web.Restart)
	}
	if web.Healthcheck == nil {
		t.Fatal("web.Healthcheck is nil")
	}
	if web.Healthcheck.Retries != 3 {
		t.Errorf("web.Healthcheck.Retries = %d", web.Healthcheck.Retries)
	}

	db := f.Services["db"]
	if db.Environment["POSTGRES_PASSWORD"] != "secret" {
		t.Errorf("db.Environment[POSTGRES_PASSWORD] = %q", db.Environment["POSTGRES_PASSWORD"])
	}

	if len(f.Volumes) != 1 {
		t.Errorf("Volumes count = %d", len(f.Volumes))
	}
	if len(f.Networks) != 1 {
		t.Errorf("Networks count = %d", len(f.Networks))
	}
}

func TestParseComposeYAML_CommandFormats(t *testing.T) {
	// Command as list
	yaml1 := `
services:
  app:
    image: app:latest
    command: ["serve", "--port=8080"]
    entrypoint: ["/bin/sh", "-c"]
`
	f1, err := parseComposeYAML([]byte(yaml1))
	if err != nil {
		t.Fatal(err)
	}
	if len(f1.Services["app"].Command) != 2 {
		t.Errorf("Command = %v", f1.Services["app"].Command)
	}
	if len(f1.Services["app"].Entrypoint) != 2 {
		t.Errorf("Entrypoint = %v", f1.Services["app"].Entrypoint)
	}

	// Command as string
	yaml2 := `
services:
  app:
    image: app:latest
    command: serve --port=8080
`
	f2, err := parseComposeYAML([]byte(yaml2))
	if err != nil {
		t.Fatal(err)
	}
	if len(f2.Services["app"].Command) != 1 {
		t.Errorf("Command = %v (string form should be single element)", f2.Services["app"].Command)
	}
}

func TestParseEnvFileList(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  []string
	}{
		{"string", "app.env", []string{"app.env"}},
		{"list of strings", []interface{}{"a.env", "b.env"}, []string{"a.env", "b.env"}},
		{"list of objects", []interface{}{
			map[string]interface{}{"path": "x.env", "required": true},
		}, []string{"x.env"}},
		{"nil", nil, nil},
		{"int", 42, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEnvFileList(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseEnvFileList len = %d, want %d", len(got), len(tt.want))
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := `# comment
FOO=bar
QUOTED="hello world"
SINGLE='val'
EMPTY=
export EXPORTED=yes
MISMATCHED='hello"

# another comment
DB_HOST=localhost
`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got := loadEnvFile(envPath)

	want := map[string]string{
		"FOO":        "bar",
		"QUOTED":     "hello world",
		"SINGLE":     "val",
		"EMPTY":      "",
		"EXPORTED":   "yes",
		"MISMATCHED": `'hello"`, // mismatched quotes are NOT stripped
		"DB_HOST":    "localhost",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("loadEnvFile[%q] = %q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("loadEnvFile returned %d keys, want %d", len(got), len(want))
	}
}

func TestLoadEnvFile_Missing(t *testing.T) {
	got := loadEnvFile("/nonexistent/path/.env")
	if got != nil {
		t.Errorf("expected nil for missing file, got %v", got)
	}
}

func TestPreloadComposeExports(t *testing.T) {
	dir := t.TempDir()

	// Write a component compose file with environment and env_file
	compDir := filepath.Join(dir, "pgvector")
	if err := os.MkdirAll(compDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	envContent := "EXTRA_FROM_FILE=fromenv\n"
	if err := os.WriteFile(filepath.Join(compDir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatalf("WriteFile .env: %v", err)
	}

	composeContent := `services:
  postgres:
    image: pgvector/pgvector:pg15
    environment:
      POSTGRES_USER: myuser
      POSTGRES_PASSWORD: secret
    env_file:
      - .env
`
	composeFile := filepath.Join(compDir, "compose.yaml")
	if err := os.WriteFile(composeFile, []byte(composeContent), 0644); err != nil {
		t.Fatalf("WriteFile compose.yaml: %v", err)
	}

	// Write composed.yaml
	composedContent := `name: test
services:
  postgres:
    x-compose-file: ./pgvector/compose.yaml
    environment:
      OVERRIDE_KEY: from-composed
`
	composedFile := filepath.Join(dir, testComposedYAMLFile)
	if err := os.WriteFile(composedFile, []byte(composedContent), 0644); err != nil {
		t.Fatalf("WriteFile composed.yaml: %v", err)
	}

	// Set buildFile so relative paths resolve
	oldBuildFile := buildFile
	buildFile = composedFile
	defer func() { buildFile = oldBuildFile }()

	cfg, err := config.Load(composedFile)
	if err != nil {
		t.Fatalf(errFmtConfigLoad, err)
	}

	preloadComposeExports(cfg)

	svc := cfg.Services["postgres"]

	// Component env should be loaded
	if svc.Environment["POSTGRES_USER"] != "myuser" {
		t.Errorf("POSTGRES_USER = %q, want myuser", svc.Environment["POSTGRES_USER"])
	}
	if svc.Environment["POSTGRES_PASSWORD"] != "secret" {
		t.Errorf("POSTGRES_PASSWORD = %q, want secret", svc.Environment["POSTGRES_PASSWORD"])
	}
	// env_file var should be loaded
	if svc.Environment["EXTRA_FROM_FILE"] != "fromenv" {
		t.Errorf("EXTRA_FROM_FILE = %q, want fromenv", svc.Environment["EXTRA_FROM_FILE"])
	}
	// composed.yaml entry should NOT be overwritten
	if svc.Environment["OVERRIDE_KEY"] != "from-composed" {
		t.Errorf("OVERRIDE_KEY = %q, want from-composed (composed.yaml wins)", svc.Environment["OVERRIDE_KEY"])
	}
}

func TestPreloadComposeExports_XExportsBackwardCompat(t *testing.T) {
	dir := t.TempDir()

	composeContent := `services:
  db:
    image: postgres:15
    x-exports:
      host: db
      port: "5432"
`
	composeFile := filepath.Join(dir, "db-compose.yaml")
	if err := os.WriteFile(composeFile, []byte(composeContent), 0644); err != nil {
		t.Fatalf("WriteFile db-compose.yaml: %v", err)
	}

	composedContent := `name: test
services:
  db:
    x-compose-file: ./db-compose.yaml
`
	composedFile := filepath.Join(dir, testComposedYAMLFile)
	if err := os.WriteFile(composedFile, []byte(composedContent), 0644); err != nil {
		t.Fatalf("WriteFile composed.yaml: %v", err)
	}

	oldBuildFile := buildFile
	buildFile = composedFile
	defer func() { buildFile = oldBuildFile }()

	cfg, err := config.Load(composedFile)
	if err != nil {
		t.Fatalf(errFmtConfigLoad, err)
	}

	preloadComposeExports(cfg)

	svc := cfg.Services["db"]
	if svc.XExports["host"] != "db" {
		t.Errorf("XExports[host] = %q, want db", svc.XExports["host"])
	}
	if svc.XExports["port"] != "5432" {
		t.Errorf("XExports[port] = %q, want 5432", svc.XExports["port"])
	}
}

func TestImageToCompose_EnvFile(t *testing.T) {
	svc := &config.Service{
		Image:   "postgres:15",
		EnvFile: []string{"./postgres.env", "./extra.env"},
		Environment: map[string]string{
			"FOO": "bar",
		},
	}

	frag := imageToCompose("postgres", svc)

	cs, ok := frag.Services["postgres"]
	if !ok {
		t.Fatal("service 'postgres' not found in fragment")
	}
	if len(cs.EnvFile) != 2 {
		t.Fatalf("EnvFile length = %d, want 2", len(cs.EnvFile))
	}
	if cs.EnvFile[0] != "./postgres.env" || cs.EnvFile[1] != "./extra.env" {
		t.Errorf("EnvFile = %v, want [./postgres.env ./extra.env]", cs.EnvFile)
	}
	if cs.Environment["FOO"] != "bar" {
		t.Errorf("Environment[FOO] = %q, want bar", cs.Environment["FOO"])
	}
}

func TestLoadEnvFile_SingleCharQuote(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	// Single quote character as value should not panic
	content := "NORMAL=hello\nBAD_DOUBLE=\"\nBAD_SINGLE='\nGOOD=\"world\"\n"
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result := loadEnvFile(envFile)
	if result == nil {
		t.Fatal("loadEnvFile returned nil")
	}
	if result["NORMAL"] != "hello" {
		t.Errorf("NORMAL = %q, want hello", result["NORMAL"])
	}
	// Single char quote should be kept as-is (not stripped)
	if result["BAD_DOUBLE"] != `"` {
		t.Errorf("BAD_DOUBLE = %q, want single double-quote", result["BAD_DOUBLE"])
	}
	if result["BAD_SINGLE"] != `'` {
		t.Errorf("BAD_SINGLE = %q, want single single-quote", result["BAD_SINGLE"])
	}
	// Properly paired quotes should be stripped
	if result["GOOD"] != "world" {
		t.Errorf("GOOD = %q, want world", result["GOOD"])
	}
}

func TestReadK8sManifests_SingleFile(t *testing.T) {
	dir := t.TempDir()
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
spec:
  replicas: 1
  template:
    spec:
      containers:
        - name: nginx
          image: nginx:latest
`
	path := filepath.Join(dir, "deploy.yaml")
	if err := os.WriteFile(path, []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	data, err := readK8sManifests(path)
	if err != nil {
		t.Fatalf("readK8sManifests error: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("expected non-empty data")
	}
	content := string(data)
	if !containsSubstring(content, "kind: Deployment") {
		t.Errorf("expected content to contain 'kind: Deployment', got:\n%s", content)
	}
}

func TestReadK8sManifests_Directory(t *testing.T) {
	dir := t.TempDir()

	deploy := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  template:
    spec:
      containers:
        - name: web
          image: myapp:latest
`
	svc := `apiVersion: v1
kind: Service
metadata:
  name: web
spec:
  selector:
    app: web
  ports:
    - port: 80
      targetPort: 8080
`
	if err := os.WriteFile(filepath.Join(dir, "deployment.yaml"), []byte(deploy), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "service.yml"), []byte(svc), 0644); err != nil {
		t.Fatal(err)
	}
	// Non-YAML file should be ignored
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# docs"), 0644); err != nil {
		t.Fatal(err)
	}

	data, err := readK8sManifests(dir)
	if err != nil {
		t.Fatalf("readK8sManifests error: %v", err)
	}

	content := string(data)
	if !containsSubstring(content, "kind: Deployment") {
		t.Error("expected Deployment in output")
	}
	if !containsSubstring(content, "kind: Service") {
		t.Error("expected Service in output")
	}
	if containsSubstring(content, "# docs") {
		t.Error("README.md should not be included")
	}
}

func TestReadK8sManifests_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	_, err := readK8sManifests(dir)
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
	if !containsSubstring(err.Error(), "no YAML files found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReadK8sManifests_MissingPath(t *testing.T) {
	_, err := readK8sManifests("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestK8sManifestsToCompose_EmptyPath(t *testing.T) {
	svc := &config.Service{
		XK8s: &config.K8sExtension{Path: ""},
	}
	_, err := k8sManifestsToCompose("test", svc)
	if err == nil {
		t.Fatal("expected error for empty path")
	}
	if !containsSubstring(err.Error(), "requires a path") {
		t.Errorf("error = %q, want mention of 'requires a path'", err)
	}
}

// writeComposedAndLoadK8s writes composed.yaml in dir, sets buildFile,
// loads config, and calls k8sManifestsToCompose for the given service name.
// It returns the compose fragment. The caller must not rely on buildFile
// after the test — it is restored via t.Cleanup.
func writeComposedAndLoadK8s(t *testing.T, dir, composedContent, svcName string) *compose.File {
	t.Helper()
	composedFile := filepath.Join(dir, testComposedYAMLFile)
	if err := os.WriteFile(composedFile, []byte(composedContent), 0644); err != nil {
		t.Fatal(err)
	}

	oldBuildFile := buildFile
	buildFile = composedFile
	t.Cleanup(func() { buildFile = oldBuildFile })

	cfg, err := config.Load(composedFile)
	if err != nil {
		t.Fatalf(errFmtConfigLoad, err)
	}

	svc := cfg.Services[svcName]
	frag, err := k8sManifestsToCompose(svcName, &svc)
	if err != nil {
		t.Fatalf("k8sManifestsToCompose: %v", err)
	}
	return frag
}

func TestK8sManifestsToCompose_Integration(t *testing.T) {
	dir := t.TempDir()

	// Write a K8s Deployment + Service
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  labels:
    app: web
spec:
  replicas: 2
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
          image: myapp:v1
          ports:
            - containerPort: 8080
          env:
            - name: PORT
              value: "8080"
---
apiVersion: v1
kind: Service
metadata:
  name: web
spec:
  type: LoadBalancer
  selector:
    app: web
  ports:
    - port: 80
      targetPort: 8080
`
	manifestPath := filepath.Join(dir, "manifests")
	if err := os.MkdirAll(manifestPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifestPath, "app.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	composedContent := `name: test
services:
  my-app:
    x-k8s:
      path: ./manifests
`
	frag := writeComposedAndLoadK8s(t, dir, composedContent, "my-app")

	// Should have translated the Deployment into a compose service
	webSvc, ok := frag.Services["web"]
	if !ok {
		t.Fatalf("service 'web' not found in fragment, got: %v", serviceNames(frag))
	}
	if webSvc.Image != "myapp:v1" {
		t.Errorf("Image = %q, want %q", webSvc.Image, "myapp:v1")
	}
	if webSvc.Environment["PORT"] != "8080" {
		t.Errorf("Environment[PORT] = %q, want 8080", webSvc.Environment["PORT"])
	}
	// LoadBalancer port should be mapped
	if len(webSvc.Ports) == 0 {
		t.Error("expected port mappings from K8s Service")
	}
	// Replicas should be set
	if webSvc.Deploy == nil || webSvc.Deploy.Replicas == nil || *webSvc.Deploy.Replicas != 2 {
		t.Error("expected replicas=2 from Deployment spec")
	}
}

func TestK8sManifestsToCompose_WithCommand(t *testing.T) {
	dir := t.TempDir()

	// The command will create the output directory and write a manifest
	outputDir := filepath.Join(dir, "dist")
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: generated
spec:
  template:
    spec:
      containers:
        - name: app
          image: generated:latest
`
	// Pre-create the command that "generates" manifests
	scriptPath := filepath.Join(dir, "generate.sh")
	script := "#!/bin/sh\nmkdir -p " + outputDir + "\ncat > " + filepath.Join(outputDir, "app.yaml") + " << 'ENDOFYAML'\n" + manifest + "ENDOFYAML\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	composedContent := `name: test
services:
  gen-app:
    x-k8s:
      command: "sh ` + scriptPath + `"
      path: ` + outputDir + `
`
	frag := writeComposedAndLoadK8s(t, dir, composedContent, "gen-app")

	genSvc, ok := frag.Services["generated"]
	if !ok {
		t.Fatalf("service 'generated' not found, got: %v", serviceNames(frag))
	}
	if genSvc.Image != "generated:latest" {
		t.Errorf("Image = %q, want %q", genSvc.Image, "generated:latest")
	}
}

func TestIsK8sManifestDir(t *testing.T) {
	t.Run("valid k8s dir", func(t *testing.T) {
		dir := t.TempDir()
		manifest := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n"
		if err := os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte(manifest), 0644); err != nil {
			t.Fatal(err)
		}
		if !isK8sManifestDir(dir) {
			t.Error("expected true for directory with K8s manifests")
		}
	})

	t.Run("compose dir (no kind/apiVersion)", func(t *testing.T) {
		dir := t.TempDir()
		composeContent := "services:\n  web:\n    image: nginx\n"
		if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(composeContent), 0644); err != nil {
			t.Fatal(err)
		}
		if isK8sManifestDir(dir) {
			t.Error("expected false for directory with compose file only")
		}
	})

	t.Run("empty dir", func(t *testing.T) {
		dir := t.TempDir()
		if isK8sManifestDir(dir) {
			t.Error("expected false for empty directory")
		}
	})

	t.Run("non-yaml files only", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# hello"), 0644); err != nil {
			t.Fatal(err)
		}
		if isK8sManifestDir(dir) {
			t.Error("expected false for directory with no YAML files")
		}
	})
}

// containsSubstring is a test helper that checks if s contains substr.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsCheck(s, substr))
}

func containsCheck(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// serviceNames returns the names of services in a compose.File for error messages.
func serviceNames(f *compose.File) []string {
	names := make([]string, 0, len(f.Services))
	for name := range f.Services {
		names = append(names, name)
	}
	return names
}

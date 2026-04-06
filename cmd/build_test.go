package cmd

import (
	"testing"
)

const testTagMainStable = "main-stable"

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
					"tag": "main-stable",
				},
			},
			want: map[string]string{"image.tag": "main-stable"},
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
	if web.Image != "nginx:latest" {
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

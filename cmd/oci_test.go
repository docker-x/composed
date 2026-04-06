package cmd

import (
	"testing"
)

func TestParseOCIRef(t *testing.T) {
	tests := []struct {
		ref      string
		registry string
		repo     string
		tag      string
		ok       bool
	}{
		{
			ref:      "oci://docker.litellm.ai/berriai/litellm-helm",
			registry: "docker.litellm.ai",
			repo:     "berriai/litellm-helm",
			tag:      "latest",
			ok:       true,
		},
		{
			ref:      "oci://ghcr.io/berriai/litellm:main-stable",
			registry: "ghcr.io",
			repo:     "berriai/litellm",
			tag:      "main-stable",
			ok:       true,
		},
		{
			ref:      "oci://registry.example.com/org/chart:v1.2.3",
			registry: "registry.example.com",
			repo:     "org/chart",
			tag:      "v1.2.3",
			ok:       true,
		},
		{
			ref:      "oci://localhost:5000/myapp:latest",
			registry: "localhost:5000",
			repo:     "myapp",
			tag:      "latest",
			ok:       true,
		},
		{
			ref: "oci://no-repo",
			ok:  false,
		},
		{
			ref:      "docker.io/library/nginx:latest",
			registry: "docker.io",
			repo:     "library/nginx",
			tag:      "latest",
			ok:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			registry, repo, tag, ok := parseOCIRef(tt.ref)
			if ok != tt.ok {
				t.Errorf("ok = %v, want %v", ok, tt.ok)
				return
			}
			if !ok {
				return
			}
			if registry != tt.registry {
				t.Errorf("registry = %q, want %q", registry, tt.registry)
			}
			if repo != tt.repo {
				t.Errorf("repo = %q, want %q", repo, tt.repo)
			}
			if tag != tt.tag {
				t.Errorf("tag = %q, want %q", tag, tt.tag)
			}
		})
	}
}

func TestClassifyManifest(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			name: "helm chart config",
			json: `{"config":{"mediaType":"application/vnd.cncf.helm.config.v1+json"}}`,
			want: "helm",
		},
		{
			name: "docker image config",
			json: `{"config":{"mediaType":"application/vnd.docker.container.image.v1+json"}}`,
			want: "image",
		},
		{
			name: "oci image config",
			json: `{"config":{"mediaType":"application/vnd.oci.image.config.v1+json"}}`,
			want: "image",
		},
		{
			name: "index with helm artifact",
			json: `{"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","artifactType":"application/vnd.cncf.helm.chart.content.v1.tar+gzip"}]}`,
			want: "helm",
		},
		{
			name: "index with image manifests",
			json: `{"manifests":[{"mediaType":"application/vnd.docker.distribution.manifest.v2+json"}]}`,
			want: "image",
		},
		{
			name: "empty manifest",
			json: `{}`,
			want: "",
		},
		{
			name: "invalid json",
			json: `{broken`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyManifest([]byte(tt.json))
			if got != tt.want {
				t.Errorf("classifyManifest() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseWWWAuthenticate(t *testing.T) {
	tests := []struct {
		header string
		check  func(t *testing.T, params map[string]string)
	}{
		{
			header: `Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/nginx:pull"`,
			check: func(t *testing.T, params map[string]string) {
				if params["realm"] != "https://auth.docker.io/token" {
					t.Errorf("realm = %q", params["realm"])
				}
				if params["service"] != "registry.docker.io" {
					t.Errorf("service = %q", params["service"])
				}
				if params["scope"] != "repository:library/nginx:pull" {
					t.Errorf("scope = %q", params["scope"])
				}
			},
		},
		{
			header: `Bearer realm="https://ghcr.io/token"`,
			check: func(t *testing.T, params map[string]string) {
				if params["realm"] != "https://ghcr.io/token" {
					t.Errorf("realm = %q", params["realm"])
				}
			},
		},
		{
			header: "",
			check: func(t *testing.T, params map[string]string) {
				if len(params) != 0 {
					t.Errorf("expected empty params, got %v", params)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			params := parseWWWAuthenticate(tt.header)
			tt.check(t, params)
		})
	}
}

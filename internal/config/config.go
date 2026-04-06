// Package config parses the composed.yaml declarative configuration.
//
// composed.yaml extends Docker Compose format with x- extensions:
//   - x-helm:         Helm chart rendering configuration
//   - x-compose-file: Include an external compose file
//   - x-exports:      Values exposed to other services via ${service.key}
//
// Services without x- extensions are plain compose services (pass-through).
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// File is the top-level composed.yaml structure.
// It is a valid Docker Compose file extended with x- fields.
type File struct {
	Name     string             `yaml:"name"`
	Services map[string]Service `yaml:"services"`
}

// Service is a compose service, optionally extended with x-helm, x-compose-file,
// and x-exports for composed-specific behavior.
type Service struct {
	// Standard compose service fields
	Image       string            `yaml:"image,omitempty"`
	Command     []string          `yaml:"command,omitempty"`
	Entrypoint  []string          `yaml:"entrypoint,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	Ports       []string          `yaml:"ports,omitempty"`
	Volumes     []string          `yaml:"volumes,omitempty"`
	Healthcheck *Healthcheck      `yaml:"healthcheck,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	DependsOn   []string          `yaml:"depends_on,omitempty"`
	Restart     string            `yaml:"restart,omitempty"`

	// Composed extensions (x- prefix = ignored by docker compose)
	XHelm        *HelmExtension    `yaml:"x-helm,omitempty"`
	XComposeFile string            `yaml:"x-compose-file,omitempty"`
	XExports     map[string]string `yaml:"x-exports,omitempty"`
}

// HelmExtension holds Helm chart rendering configuration.
type HelmExtension struct {
	Chart      string                 `yaml:"chart"`
	Repo       string                 `yaml:"repo,omitempty"`
	Version    string                 `yaml:"version,omitempty"`
	Values     map[string]interface{} `yaml:"values,omitempty"`
	ValuesFile string                 `yaml:"values_file,omitempty"`
}

// Healthcheck mirrors the Docker Compose healthcheck config.
type Healthcheck struct {
	Test     []string `yaml:"test"`
	Interval string   `yaml:"interval,omitempty"`
	Timeout  string   `yaml:"timeout,omitempty"`
	Retries  int      `yaml:"retries,omitempty"`
}

// ServiceType returns the component type based on x- extensions:
//   - "helm"    if x-helm is set
//   - "compose" if x-compose-file is set
//   - "image"   otherwise (plain compose service)
func ServiceType(svc *Service) string {
	if svc.XHelm != nil {
		return "helm"
	}
	if svc.XComposeFile != "" {
		return "compose"
	}
	return "image"
}

// Load reads and parses a composed.yaml file.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	return Parse(data)
}

// Parse parses composed.yaml content.
func Parse(data []byte) (*File, error) {
	f := &File{}
	if err := yaml.Unmarshal(data, f); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if f.Services == nil {
		f.Services = make(map[string]Service)
	}
	return f, nil
}

// ResolveRefs resolves ${service.key} references in environment values
// and helm values using the x-exports from all services.
func (f *File) ResolveRefs() error {
	// Build export index: service_name.key → value
	exports := make(map[string]string)
	for name := range f.Services {
		svc := f.Services[name]
		for k, v := range svc.XExports {
			exports[name+"."+k] = v
		}
	}

	// Resolve in all services
	for name := range f.Services {
		svc := f.Services[name]
		if svc.Environment != nil {
			for k, v := range svc.Environment {
				svc.Environment[k] = resolveString(v, exports)
			}
		}
		if svc.XHelm != nil && svc.XHelm.Values != nil {
			svc.XHelm.Values = resolveMap(svc.XHelm.Values, exports)
		}
		f.Services[name] = svc
	}
	return nil
}

// resolveString replaces ${foo.bar} placeholders with export values.
func resolveString(s string, exports map[string]string) string {
	result := s
	for key, val := range exports {
		placeholder := "${" + key + "}"
		result = strings.ReplaceAll(result, placeholder, val)
	}
	return result
}

func resolveMap(m map[string]interface{}, exports map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case string:
			out[k] = resolveString(val, exports)
		case map[string]interface{}:
			out[k] = resolveMap(val, exports)
		default:
			out[k] = v
		}
	}
	return out
}

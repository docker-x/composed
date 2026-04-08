// Package config parses the composed.yaml declarative configuration.
//
// composed.yaml extends Docker Compose format with x- extensions:
//   - x-helm:         Helm chart rendering configuration
//   - x-k8s:          Raw K8s manifest directory/file (generic K8s-to-Compose)
//   - x-compose-file: Include an external compose file
//   - x-shell:        Top-level shell commands (run during build, stdout captured)
//   - x-exports:      Values exposed to other services via ${service.key}
//
// Services without x- extensions are plain compose services (pass-through).
package config

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// shellTimeout is the maximum time a shell command may run.
const shellTimeout = 30 * time.Second

// FlexStringSlice unmarshals both a scalar string ("foo.env") and a list
// (["a.env", "b.env"]) into []string. This matches Docker Compose's env_file
// and similar fields that accept either form.
type FlexStringSlice []string

// UnmarshalYAML implements yaml.Unmarshaler for FlexStringSlice.
func (f *FlexStringSlice) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*f = []string{value.Value}
		return nil
	case yaml.SequenceNode:
		var items []string
		if err := value.Decode(&items); err != nil {
			return err
		}
		*f = items
		return nil
	default:
		return fmt.Errorf("env_file: expected string or list, got %v", value.Kind)
	}
}

// File is the top-level composed.yaml structure.
// It is a valid Docker Compose file extended with x- fields.
type File struct {
	Name     string                  `yaml:"name"`
	Services map[string]Service      `yaml:"services"`
	Volumes  map[string]VolumeConfig `yaml:"volumes,omitempty"`
	XShell   []NamedShellEntry       `yaml:"-"`
}

// MarshalYAML implements yaml.Marshaler for File, ensuring x-shell entries
// are included in the output despite the yaml:"-" tag (needed because
// x-shell uses polymorphic YAML that requires custom parsing).
func (f File) MarshalYAML() (interface{}, error) {
	// Build a proxy struct with the standard fields.
	type plain struct {
		Name     string                  `yaml:"name"`
		XShell   yaml.Node               `yaml:"x-shell,omitempty"`
		Services map[string]Service      `yaml:"services"`
		Volumes  map[string]VolumeConfig `yaml:"volumes,omitempty"`
	}
	p := plain{
		Name:     f.Name,
		Services: f.Services,
		Volumes:  f.Volumes,
	}

	if len(f.XShell) > 0 {
		p.XShell = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, named := range f.XShell {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: named.Name, Tag: "!!str"}
			if named.Entry.AllowFailure {
				// Long form: {command: "...", allow_failure: true}
				var valNode yaml.Node
				if err := valNode.Encode(named.Entry); err != nil {
					return nil, err
				}
				p.XShell.Content = append(p.XShell.Content, keyNode, &valNode)
			} else {
				// Shorthand: "command"
				valNode := &yaml.Node{Kind: yaml.ScalarNode, Value: named.Entry.Command, Tag: "!!str"}
				p.XShell.Content = append(p.XShell.Content, keyNode, valNode)
			}
		}
	}

	return &p, nil
}

// NamedShellEntry pairs a shell entry name with its definition,
// preserving the declaration order from YAML.
type NamedShellEntry struct {
	Name  string
	Entry ShellEntry
}

// VolumeConfig represents a top-level volume declaration.
// Supports standard Docker Compose volume fields: external, name, driver.
type VolumeConfig struct {
	External bool   `yaml:"external,omitempty"`
	Name     string `yaml:"name,omitempty"`
	Driver   string `yaml:"driver,omitempty"`
}

// ShellEntry represents a top-level x-shell entry.
// It supports two YAML forms:
//   - string: command only (shorthand)
//   - map:    command + allow_failure (long form)
type ShellEntry struct {
	Command      string `yaml:"command"`
	AllowFailure bool   `yaml:"allow_failure"`
}

// Service is a compose service, optionally extended with x-helm, x-compose-file,
// and x-exports for composed-specific behavior.
type Service struct {
	// Standard compose service fields
	Image       string            `yaml:"image,omitempty"`
	Command     []string          `yaml:"command,omitempty"`
	Entrypoint  []string          `yaml:"entrypoint,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	EnvFile     FlexStringSlice   `yaml:"env_file,omitempty"`
	Ports       []string          `yaml:"ports,omitempty"`
	Volumes     []string          `yaml:"volumes,omitempty"`
	Healthcheck *Healthcheck      `yaml:"healthcheck,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	DependsOn   []string          `yaml:"depends_on,omitempty"`
	Restart     string            `yaml:"restart,omitempty"`

	// Composed extensions (x- prefix = ignored by docker compose)
	XHelm        *HelmExtension    `yaml:"x-helm,omitempty"`
	XK8s         *K8sExtension     `yaml:"x-k8s,omitempty"`
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

// K8sExtension holds configuration for reading raw K8s manifests.
// This is the generic form of what x-helm does internally — it accepts
// K8s YAML from any source (cdk8s, kustomize, hand-written, etc.) and
// translates it to Compose.
type K8sExtension struct {
	Path    string `yaml:"path"`              // Directory of *.yaml files or a single file
	Command string `yaml:"command,omitempty"` // Optional command to run before reading (e.g. "cdk8s synth")
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
//   - "k8s"     if x-k8s is set
//   - "compose" if x-compose-file is set
//   - "image"   otherwise (plain compose service)
func ServiceType(svc *Service) string {
	if svc.XHelm != nil {
		return "helm"
	}
	if svc.XK8s != nil {
		return "k8s"
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
	if f.Volumes == nil {
		f.Volumes = make(map[string]VolumeConfig)
	}

	// Parse x-shell from raw YAML (supports string and map forms).
	var raw map[string]yaml.Node
	if err := yaml.Unmarshal(data, &raw); err == nil {
		if node, ok := raw["x-shell"]; ok {
			shells, err := parseShellEntries(&node)
			if err != nil {
				return nil, fmt.Errorf("parse x-shell: %w", err)
			}
			f.XShell = shells
		}
	}
	if f.XShell == nil {
		f.XShell = []NamedShellEntry{}
	}

	return f, nil
}

// parseShellEntries parses the x-shell YAML node into an ordered slice of NamedShellEntry.
// Each entry can be a string (shorthand) or a map (long form).
// The slice preserves YAML declaration order.
func parseShellEntries(node *yaml.Node) ([]NamedShellEntry, error) {
	var entries []NamedShellEntry

	// x-shell must be a mapping
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("x-shell must be a mapping, got %v", node.Kind)
	}

	for i := 0; i < len(node.Content)-1; i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]
		name := keyNode.Value

		switch valNode.Kind {
		case yaml.ScalarNode:
			// Shorthand: name: "command"
			entries = append(entries, NamedShellEntry{Name: name, Entry: ShellEntry{Command: valNode.Value}})
		case yaml.MappingNode:
			// Long form: name: {command: "...", allow_failure: true}
			var entry ShellEntry
			if err := valNode.Decode(&entry); err != nil {
				return nil, fmt.Errorf("parse shell entry %q: %w", name, err)
			}
			entries = append(entries, NamedShellEntry{Name: name, Entry: entry})
		default:
			return nil, fmt.Errorf("shell entry %q: expected string or mapping", name)
		}
	}
	return entries, nil
}

// ResolveRefs resolves ${service.key} references in environment values
// and helm values. Resolution priority: x-exports first, then direct
// field lookup (environment, hostname, image, ports).
// shellValues contains captured stdout from x-shell entries.
func (f *File) ResolveRefs(shellValues map[string]string) error {
	exports := buildExportIndex(f, shellValues)
	snapshot := snapshotServices(f.Services)

	// Resolve in all services
	for name := range f.Services {
		svc := f.Services[name]
		if svc.Environment != nil {
			for k, v := range svc.Environment {
				svc.Environment[k] = resolveString(v, exports, snapshot)
			}
		}
		if svc.XHelm != nil && svc.XHelm.Values != nil {
			svc.XHelm.Values = resolveMap(svc.XHelm.Values, exports, snapshot)
		}
		f.Services[name] = svc
	}
	return nil
}

// buildExportIndex creates a lookup map for x-exports and shell values.
func buildExportIndex(f *File, shellValues map[string]string) map[string]string {
	exports := make(map[string]string)
	for name := range f.Services {
		svc := f.Services[name]
		for k, v := range svc.XExports {
			exports[name+"."+k] = v
		}
	}
	// Add shell values as top-level names (no dot needed).
	// Warn if a shell name shadows an existing export key.
	for name, val := range shellValues {
		if prev, exists := exports[name]; exists {
			fmt.Fprintf(os.Stderr, "Warning: x-shell %q shadows export %q (was %q)\n", name, name, prev)
		}
		exports[name] = val
	}
	return exports
}

// snapshotServices deep-copies services so direct references always read
// original (pre-resolution) values regardless of map iteration order.
func snapshotServices(services map[string]Service) map[string]Service {
	snapshot := make(map[string]Service, len(services))
	for name := range services {
		svc := services[name]
		cp := svc
		if svc.Environment != nil {
			cp.Environment = make(map[string]string, len(svc.Environment))
			for k, v := range svc.Environment {
				cp.Environment[k] = v
			}
		}
		if svc.Ports != nil {
			cp.Ports = make([]string, len(svc.Ports))
			copy(cp.Ports, svc.Ports)
		}
		snapshot[name] = cp
	}
	return snapshot
}

// refPattern matches ${...} placeholders.
var refPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// shellRefPrefix is the prefix for inline shell references.
const shellRefPrefix = "shell:"

// resolveString replaces ${foo.bar} placeholders with export values
// or direct service field lookups. ${shell:cmd} runs inline commands.
func resolveString(s string, exports map[string]string, services map[string]Service) string {
	return refPattern.ReplaceAllStringFunc(s, func(match string) string {
		ref := match[2 : len(match)-1] // strip ${ and }

		// 1. Check inline shell reference: ${shell:command}
		if strings.HasPrefix(ref, shellRefPrefix) {
			cmd := strings.TrimSpace(ref[len(shellRefPrefix):])
			if out, err := runShellCommand(cmd); err == nil {
				return out
			} else {
				fmt.Fprintf(os.Stderr, "Warning: inline shell %q failed: %v\n", cmd, err)
			}
			// On failure, leave unresolved
			return match
		}

		// 2. Check x-exports and x-shell named values
		if val, ok := exports[ref]; ok {
			return val
		}

		// 3. Try direct field lookup
		if val, ok := resolveDirectRef(ref, services); ok {
			return val
		}

		// 4. Leave unresolved
		return match
	})
}

// resolveDirectRef resolves a dotted reference like "svc.environment.KEY",
// "svc.hostname", "svc.image", or "svc.ports[0]" against service definitions.
func resolveDirectRef(ref string, services map[string]Service) (string, bool) {
	// Split on first dot: service_name.rest
	dot := strings.IndexByte(ref, '.')
	if dot < 0 {
		return "", false
	}
	svcName := ref[:dot]
	field := ref[dot+1:]

	svc, ok := services[svcName]
	if !ok {
		return "", false
	}

	switch {
	case field == "hostname":
		return svcName, true

	case field == "image":
		if svc.Image != "" {
			return svc.Image, true
		}
		return "", false

	case strings.HasPrefix(field, "environment."):
		key := field[len("environment."):]
		if svc.Environment != nil {
			if val, ok := svc.Environment[key]; ok {
				return val, true
			}
		}
		return "", false

	case strings.HasPrefix(field, "ports[") && strings.HasSuffix(field, "]"):
		// Parse ports[N] — must be exactly ports[<int>]
		idxStr := field[len("ports[") : len(field)-1]
		idx, err := strconv.Atoi(idxStr)
		if err != nil || idx < 0 || idx >= len(svc.Ports) {
			return "", false
		}
		return svc.Ports[idx], true
	}

	return "", false
}

func resolveMap(m map[string]interface{}, exports map[string]string, services map[string]Service) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case string:
			out[k] = resolveString(val, exports, services)
		case map[string]interface{}:
			out[k] = resolveMap(val, exports, services)
		default:
			out[k] = v
		}
	}
	return out
}

// runShellCommand executes a command via sh -c with a timeout and returns trimmed stdout.
func runShellCommand(command string) (string, error) {
	shPath, err := exec.LookPath("sh")
	if err != nil {
		return "", fmt.Errorf("sh not found: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), shellTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, shPath, "-c", command).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// RunShellEntries executes all top-level x-shell entries in declaration order
// and returns a map of name -> trimmed stdout.
func RunShellEntries(entries []NamedShellEntry) (map[string]string, error) {
	values := make(map[string]string, len(entries))
	for _, named := range entries {
		fmt.Fprintf(os.Stderr, "Running x-shell %q ...\n", named.Name)
		out, err := runShellCommand(named.Entry.Command)
		if err != nil {
			if named.Entry.AllowFailure {
				fmt.Fprintf(os.Stderr, "Warning: x-shell %q failed: %v (allow_failure=true)\n", named.Name, err)
				continue
			}
			return nil, fmt.Errorf("x-shell %q failed: %w", named.Name, err)
		}
		values[named.Name] = out
	}
	return values, nil
}

// Package merge combines multiple compose fragments into one docker-compose.yaml.
package merge

import (
	"github.com/docker-x/composed/internal/compose"
)

// Merge combines multiple compose files into one.
// Later files take precedence for conflicting service names.
// Volumes, networks, and configs are union-merged.
func Merge(project string, fragments ...*compose.File) *compose.File {
	out := compose.NewFile()
	out.Project = project

	for _, f := range fragments {
		mergeFragment(out, f)
	}

	return out
}

func mergeFragment(out, f *compose.File) {
	if f == nil {
		return
	}

	mergeHeader(out, f)
	mergeFragmentServices(out, f)
	mergeFragmentVolumes(out, f)
	mergeFragmentNetworks(out, f)
	mergeFragmentConfigs(out, f)
	mergeFragmentSecrets(out, f)
}

func mergeHeader(out, f *compose.File) {
	if f.Header != "" {
		if out.Header != "" {
			out.Header += "\n"
		}
		out.Header += f.Header
	}
}

func mergeFragmentServices(out, f *compose.File) {
	// Merge services
	for name, svc := range f.Services {
		if existing, ok := out.Services[name]; ok {
			// Merge into existing service
			mergeService(existing, svc)
		} else {
			out.Services[name] = svc
		}
	}
}

func mergeFragmentVolumes(out, f *compose.File) {
	// Union merge volumes
	for name, vol := range f.Volumes {
		if _, ok := out.Volumes[name]; !ok {
			out.Volumes[name] = vol
		}
	}
}

func mergeFragmentNetworks(out, f *compose.File) {
	// Union merge networks
	for name, net := range f.Networks {
		if _, ok := out.Networks[name]; !ok {
			out.Networks[name] = net
		}
	}
}

func mergeFragmentConfigs(out, f *compose.File) {
	// Union merge configs
	for name, cfg := range f.Configs {
		if _, ok := out.Configs[name]; !ok {
			out.Configs[name] = cfg
		}
	}
}

func mergeFragmentSecrets(out, f *compose.File) {
	// Union merge top-level secrets from fragments
	for name, sec := range f.Secrets {
		if _, ok := out.Secrets[name]; !ok {
			out.Secrets[name] = sec
		}
	}
}

// mergeService merges src into dst. src values override dst for scalars;
// maps and slices are union-merged.
func mergeService(dst, src *compose.Service) {
	if src.Image != "" {
		dst.Image = src.Image
	}
	if len(src.Entrypoint) > 0 {
		dst.Entrypoint = src.Entrypoint
	}
	if len(src.Command) > 0 {
		dst.Command = src.Command
	}
	if src.WorkingDir != "" {
		dst.WorkingDir = src.WorkingDir
	}
	if src.User != "" {
		dst.User = src.User
	}
	if src.NetworkMode != "" {
		dst.NetworkMode = src.NetworkMode
	}
	if src.Restart != "" {
		dst.Restart = src.Restart
	}
	if src.Healthcheck != nil {
		dst.Healthcheck = src.Healthcheck
	}
	if src.Build != nil {
		dst.Build = src.Build
	}
	if src.Deploy != nil {
		dst.Deploy = src.Deploy
	}

	// Merge environment (src wins on conflict)
	if dst.Environment == nil {
		dst.Environment = make(map[string]string)
	}
	for k, v := range src.Environment {
		dst.Environment[k] = v
	}

	// Merge labels
	if dst.Labels == nil {
		dst.Labels = make(map[string]string)
	}
	for k, v := range src.Labels {
		dst.Labels[k] = v
	}

	// Merge depends_on
	if dst.DependsOn == nil {
		dst.DependsOn = make(map[string]compose.DependsOnCondition)
	}
	for k, v := range src.DependsOn {
		dst.DependsOn[k] = v
	}

	// Append unique ports
	dst.Ports = appendUnique(dst.Ports, src.Ports...)

	// Append unique volumes
	dst.Volumes = appendUnique(dst.Volumes, src.Volumes...)

	// Append unique env files and configs. These are service-level references
	// and must be merged alongside the top-level definitions.
	dst.EnvFile = appendUnique(dst.EnvFile, src.EnvFile...)
	dst.Configs = appendUniqueConfigs(dst.Configs, src.Configs...)
	dst.Tmpfs = appendUnique(dst.Tmpfs, src.Tmpfs...)
	if src.ShmSize != "" {
		dst.ShmSize = src.ShmSize
	}

	// Append unique secrets
	dst.Secrets = appendUnique(dst.Secrets, src.Secrets...)

	// Append profiles
	dst.Profiles = appendUnique(dst.Profiles, src.Profiles...)
}

func appendUnique(dst []string, items ...string) []string {
	seen := make(map[string]bool, len(dst))
	for _, s := range dst {
		seen[s] = true
	}
	for _, s := range items {
		if !seen[s] {
			dst = append(dst, s)
			seen[s] = true
		}
	}
	return dst
}

func appendUniqueConfigs(dst []compose.ServiceConfig, items ...compose.ServiceConfig) []compose.ServiceConfig {
	seen := make(map[string]bool, len(dst))
	for _, cfg := range dst {
		seen[cfg.Source+"\x00"+cfg.Target] = true
	}
	for _, cfg := range items {
		key := cfg.Source + "\x00" + cfg.Target
		if !seen[key] {
			dst = append(dst, cfg)
			seen[key] = true
		}
	}
	return dst
}

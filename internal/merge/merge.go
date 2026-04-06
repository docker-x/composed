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
	if src.Restart != "" {
		dst.Restart = src.Restart
	}
	if src.Healthcheck != nil {
		dst.Healthcheck = src.Healthcheck
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

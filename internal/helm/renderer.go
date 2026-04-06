// Package helm renders Helm charts by shelling out to the helm CLI.
package helm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/docker-x/composed/internal/k8s"
)

// RenderOpts configures chart rendering.
type RenderOpts struct {
	// Chart reference: "repo/chart" for remote, "./path" for local directory/archive.
	Chart string
	// Repository URL (for remote charts — passed to --repo).
	Repo string
	// Version constraint (e.g. "18.x", ">=1.0.0"). Empty = latest.
	Version string
	// Release name for template rendering.
	Release string
	// Paths to values files (-f).
	ValueFiles []string
	// Individual --set overrides.
	SetValues map[string]string
}

// Render calls `helm template` and parses the output into typed K8s manifests.
// Requires `helm` to be on PATH.
func Render(opts RenderOpts) (*k8s.Manifests, error) {
	helmBin, err := findHelm()
	if err != nil {
		return nil, err
	}

	chart := opts.Chart

	// If it's a local chart directory, run dependency update first
	if isLocalChart(chart) {
		abs, err := filepath.Abs(chart)
		if err != nil {
			return nil, fmt.Errorf("resolve chart path: %w", err)
		}
		chart = abs

		if err := depUpdate(helmBin, chart); err != nil {
			// Non-fatal: chart may have no dependencies
			fmt.Fprintf(os.Stderr, "Warning: helm dependency update: %v\n", err)
		}
	}

	// Build helm template args
	args := []string{"template", opts.Release, chart}

	if opts.Repo != "" {
		args = append(args, "--repo", opts.Repo)
	}
	if opts.Version != "" {
		args = append(args, "--version", opts.Version)
	}
	for _, vf := range opts.ValueFiles {
		args = append(args, "-f", vf)
	}
	for k, v := range opts.SetValues {
		args = append(args, "--set", k+"="+v)
	}

	cmd := exec.Command(helmBin, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("helm template failed: %w", err)
	}

	return k8s.Parse(out)
}

// depUpdate runs `helm dependency update` on a local chart.
func depUpdate(helmBin, chartPath string) error {
	// Check if Chart.yaml has dependencies
	chartYaml := filepath.Join(chartPath, "Chart.yaml")
	data, err := os.ReadFile(chartYaml)
	if err != nil {
		return nil // No Chart.yaml = no deps to update
	}
	if !strings.Contains(string(data), "dependencies:") {
		return nil // No dependencies block
	}

	cmd := exec.Command(helmBin, "dependency", "update", chartPath)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// findHelm locates the helm binary on PATH.
func findHelm() (string, error) {
	path, err := exec.LookPath("helm")
	if err != nil {
		return "", fmt.Errorf("helm not found on PATH — install it: https://helm.sh/docs/intro/install/")
	}
	return path, nil
}

func isLocalChart(chart string) bool {
	if strings.HasPrefix(chart, ".") || strings.HasPrefix(chart, "/") {
		return true
	}
	info, err := os.Stat(chart)
	return err == nil && info.IsDir()
}

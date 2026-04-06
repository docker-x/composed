package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/docker-x/composed/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// --- init ---

var (
	initProject    string
	initFile       string
	initHelmValues bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a new composed.yaml or scaffold values files",
	Long: `Initialize a new composed.yaml (a Docker Compose file with x- extensions)
in the current directory.

With --helm-values, scans an existing composed.yaml for services with x-helm
and creates a default values file (values-<name>.yaml) for each OCI chart that
doesn't already have one. The values_file reference is added automatically.

Examples:
  composed init
  composed init --project my-stack
  composed init --helm-values`,
	RunE: runConfigInit,
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	if initHelmValues {
		return scaffoldHelmValues()
	}

	// Default project name to directory name
	if initProject == "" {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		initProject = filepath.Base(dir)
	}

	if _, err := os.Stat(initFile); err == nil {
		return fmt.Errorf("%s already exists (use 'composed add' to modify it)", initFile)
	}

	f := &config.File{
		Name:     initProject,
		Services: make(map[string]config.Service),
	}

	return writeConfig(initFile, f)
}

// scaffoldHelmValues finds services with x-helm and OCI charts, creates
// default values files by running `helm show values <chart>`.
func scaffoldHelmValues() error {
	initFile = resolveConfig(nil, initFile)

	f, err := config.Load(initFile)
	if err != nil {
		return fmt.Errorf("load %s: %w (run 'composed init' first)", initFile, err)
	}

	configDir := filepath.Dir(initFile)
	created := 0

	for name := range f.Services {
		svc := f.Services[name]
		if svc.XHelm == nil || svc.XHelm.Chart == "" {
			continue
		}

		// Skip if already has a values_file
		if svc.XHelm.ValuesFile != "" {
			fmt.Fprintf(os.Stderr, "  %s: already has values_file %s, skipping\n", name, svc.XHelm.ValuesFile)
			continue
		}

		valuesPath := filepath.Join(configDir, fmt.Sprintf("%s.values.yaml", name))

		// Skip if file already exists
		if _, err := os.Stat(valuesPath); err == nil {
			fmt.Fprintf(os.Stderr, "  %s: %s already exists, skipping\n", name, valuesPath)
			continue
		}

		fmt.Fprintf(os.Stderr, "  %s: fetching default values from %s ...\n", name, svc.XHelm.Chart)

		out, err := helmShowValues(svc.XHelm)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s: helm show values failed: %v, skipping\n", name, err)
			continue
		}

		if err := os.WriteFile(valuesPath, out, 0644); err != nil {
			return fmt.Errorf("write %s: %w", valuesPath, err)
		}

		// Wire into composed.yaml
		relPath, _ := filepath.Rel(configDir, valuesPath)
		svc.XHelm.ValuesFile = relPath
		f.Services[name] = svc

		fmt.Fprintf(os.Stderr, "  %s: wrote %s\n", name, valuesPath)
		created++
	}

	if created == 0 {
		fmt.Fprintln(os.Stderr, "No helm services need values files.")
		return nil
	}

	if err := writeConfig(initFile, f); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Created %d values file(s), updated %s\n", created, initFile)
	return nil
}

// helmShowValues runs `helm show values <chart>` and returns the output.
func helmShowValues(h *config.HelmExtension) ([]byte, error) {
	args := []string{"show", "values", h.Chart}
	if h.Repo != "" {
		args = append(args, "--repo", h.Repo)
	}
	if h.Version != "" {
		args = append(args, "--version", h.Version)
	}
	cmd := exec.Command("helm", args...) //nolint:gosec // helm must be in PATH
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

// --- add ---

var (
	addFile       string
	addChart      string
	addRepo       string
	addVersion    string
	addImage      string
	addCompFile   string
	addSets       []string
	addValues     string // --values: load values file and merge inline
	addValuesFile string // --values-file: store reference, load at build time
	addPorts      []string
	addEnvs       []string
	addVolumes    []string
	addDependsOn  []string
)

var addCmd = &cobra.Command{
	Use:   "add [name] <source>",
	Short: "Add a service to composed.yaml",
	Long: `Add a Helm chart, Docker image, or compose file as a named service.

The source is auto-detected by probing the OCI registry manifest or
inspecting the local filesystem:
  oci://...                     → probes registry (helm chart or image)
  *.yaml / *.yml file           → compose file include
  directory with Chart.yaml     → local helm chart
  repo/chart (with --repo)      → helm chart (repository)
  anything else                 → Docker image

The service name is derived from the source if not given explicitly.

Examples:
  # Fully automatic — name and type detected from the OCI manifest
  composed add oci://docker.litellm.ai/berriai/litellm-helm

  # Auto-detect with explicit name
  composed add litellm oci://docker.litellm.ai/berriai/litellm-helm --set image.tag=main-stable

  # Docker image (name derived: "postgres")
  composed add postgres:15-alpine --port 5432:5432 --env POSTGRES_PASSWORD=secret

  # Explicit flags (still work)
  composed add redis --chart bitnami/redis --repo https://charts.bitnami.com/bitnami

  # With dependencies
  composed add myapp:latest --depends-on postgres --depends-on redis`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runConfigAdd,
}

func runConfigAdd(cmd *cobra.Command, args []string) error {
	addFile = resolveConfig(cmd, addFile)

	name, source, err := resolveAddArgs(args)
	if err != nil {
		return err
	}

	if source != "" {
		autoDetectSource(source)
	}

	if verr := validateServiceType(); verr != nil {
		return verr
	}

	// Load existing config
	f, err := config.Load(addFile)
	if err != nil {
		return fmt.Errorf("load %s: %w (run 'composed init' first)", addFile, err)
	}

	if _, exists := f.Services[name]; exists {
		return fmt.Errorf("service %q already exists in %s", name, addFile)
	}

	var svc config.Service
	var err2 error

	switch {
	case addChart != "":
		svc, err2 = buildHelmService()
		if err2 != nil {
			return err2
		}
	case addImage != "":
		svc = buildImageService()
	case addCompFile != "":
		svc = buildComposeService()
	}

	if len(addDependsOn) > 0 {
		svc.DependsOn = addDependsOn
	}

	f.Services[name] = svc

	if err := writeConfig(addFile, f); err != nil {
		return err
	}

	svcType := config.ServiceType(&svc)
	fmt.Fprintf(os.Stderr, "Added %s service %q to %s\n", svcType, name, addFile)
	return nil
}

func resolveAddArgs(args []string) (name, source string, err error) {
	hasExplicitFlags := addChart != "" || addImage != "" || addCompFile != ""

	switch len(args) {
	case 2:
		name, source = args[0], args[1]
		if hasExplicitFlags {
			return "", "", fmt.Errorf("provide either a positional source or --chart/--image/--compose-file, not both")
		}
	case 1:
		if hasExplicitFlags {
			name = args[0]
		} else {
			source = args[0]
			name = deriveComponentName(source)
		}
	}

	return name, source, nil
}

func autoDetectSource(source string) {
	switch detectSourceType(source) {
	case "helm":
		addChart = source
	case "compose":
		addCompFile = source
	case "image":
		addImage = source
	}
}

func validateServiceType() error {
	flagCount := 0
	if addChart != "" {
		flagCount++
	}
	if addImage != "" {
		flagCount++
	}
	if addCompFile != "" {
		flagCount++
	}
	if flagCount == 0 {
		return fmt.Errorf("specify a source: composed add <source>\nor use --chart, --image, or --compose-file")
	}
	if flagCount > 1 {
		return fmt.Errorf("ambiguous: specify only one of --chart, --image, or --compose-file")
	}
	return nil
}

// deriveComponentName extracts a short name from a source string.
//
//	oci://docker.litellm.ai/berriai/litellm-helm  → litellm-helm
//	oci://ghcr.io/berriai/litellm:main-stable      → litellm
//	postgres:15-alpine                              → postgres
//	bitnami/redis                                   → redis
//	./monitoring/docker-compose.yaml                → monitoring
//	./charts/myapp                                  → myapp
func deriveComponentName(source string) string {
	s := strings.TrimPrefix(source, "oci://")

	// Strip tag (:something at the end, but not registry port like localhost:5000)
	if i := strings.LastIndex(s, ":"); i > 0 {
		after := s[i+1:]
		if !strings.Contains(after, "/") {
			s = s[:i]
		}
	}

	// For YAML files, use the parent directory name
	lower := strings.ToLower(s)
	if strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") {
		dir := filepath.Dir(s)
		if dir != "." && dir != "/" {
			return filepath.Base(dir)
		}
		return strings.TrimSuffix(strings.TrimSuffix(filepath.Base(s), ".yaml"), ".yml")
	}

	return filepath.Base(s)
}

// detectSourceType determines the service type from a source string.
func detectSourceType(source string) string {
	if strings.HasPrefix(source, "oci://") {
		fmt.Fprintf(os.Stderr, "Detecting artifact type for %s ...\n", source)
		if t := ociArtifactType(source); t != "" {
			fmt.Fprintf(os.Stderr, "Detected: %s\n", t)
			return t
		}
		fmt.Fprintf(os.Stderr, "Could not detect artifact type, defaulting to image\n")
		return "image"
	}

	lower := strings.ToLower(source)
	if strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") {
		return "compose"
	}

	if info, err := os.Stat(source); err == nil && info.IsDir() {
		if _, err := os.Stat(filepath.Join(source, "Chart.yaml")); err == nil {
			return "helm"
		}
	}

	if addRepo != "" {
		return "helm"
	}

	return "image"
}

func buildHelmService() (config.Service, error) {
	svc := config.Service{
		XHelm: &config.HelmExtension{Chart: addChart},
	}
	if addRepo != "" {
		svc.XHelm.Repo = addRepo
	}
	if addVersion != "" {
		svc.XHelm.Version = addVersion
	}

	// Start with values from --values file (merged inline)
	if addValues != "" {
		vals, err := loadValuesFile(addValues)
		if err != nil {
			return svc, fmt.Errorf("load values %s: %w", addValues, err)
		}
		svc.XHelm.Values = vals
	}

	// --set overrides on top
	if len(addSets) > 0 {
		sets := parseSetValues(addSets)
		if svc.XHelm.Values == nil {
			svc.XHelm.Values = sets
		} else {
			mergeMaps(svc.XHelm.Values, sets)
		}
	}

	// --values-file: store reference for build-time loading
	if addValuesFile != "" {
		svc.XHelm.ValuesFile = addValuesFile
	}

	return svc, nil
}

// loadValuesFile reads a YAML values file into a map.
func loadValuesFile(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var vals map[string]interface{}
	if err := yaml.Unmarshal(data, &vals); err != nil {
		return nil, err
	}
	return vals, nil
}

// mergeMaps merges src into dst recursively (src wins on conflict).
func mergeMaps(dst, src map[string]interface{}) {
	for k, sv := range src {
		if dv, ok := dst[k]; ok {
			if dm, ok := dv.(map[string]interface{}); ok {
				if sm, ok := sv.(map[string]interface{}); ok {
					mergeMaps(dm, sm)
					continue
				}
			}
		}
		dst[k] = sv
	}
}

func buildImageService() config.Service {
	svc := config.Service{Image: addImage}
	if len(addPorts) > 0 {
		svc.Ports = addPorts
	}
	if len(addEnvs) > 0 {
		svc.Environment = parseEnvValues(addEnvs)
	}
	if len(addVolumes) > 0 {
		svc.Volumes = addVolumes
	}
	return svc
}

func buildComposeService() config.Service {
	return config.Service{XComposeFile: addCompFile}
}

// parseSetValues converts ["key=val", "nested.key=val"] into a nested map.
func parseSetValues(sets []string) map[string]interface{} {
	out := make(map[string]interface{})
	for _, s := range sets {
		k, v, ok := strings.Cut(s, "=")
		if !ok {
			continue
		}
		setNestedValue(out, strings.Split(k, "."), v)
	}
	return out
}

func setNestedValue(m map[string]interface{}, keys []string, val string) {
	for i, k := range keys {
		if i == len(keys)-1 {
			m[k] = val
			return
		}
		sub, ok := m[k].(map[string]interface{})
		if !ok {
			sub = make(map[string]interface{})
			m[k] = sub
		}
		m = sub
	}
}

func parseEnvValues(envs []string) map[string]string {
	out := make(map[string]string, len(envs))
	for _, e := range envs {
		k, v, ok := strings.Cut(e, "=")
		if ok {
			out[k] = v
		}
	}
	return out
}

// writeConfig marshals and writes the config file.
func writeConfig(path string, f *config.File) error {
	data, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func init() {
	initCmd.Flags().StringVar(&initProject, "project", "", "Project name (default: directory name)")
	initCmd.Flags().StringVarP(&initFile, "file", "f", "composed.yaml", "Config file to create")
	initCmd.Flags().BoolVar(&initHelmValues, "helm-values", false, "Scaffold default values files for helm services")
	rootCmd.AddCommand(initCmd)

	addCmd.Flags().StringVarP(&addFile, "file", "f", "composed.yaml", "Config file to modify")
	addCmd.Flags().StringVar(&addChart, "chart", "", "Helm chart (OCI ref, repo/name, or local path)")
	addCmd.Flags().StringVar(&addRepo, "repo", "", "Helm chart repository URL")
	addCmd.Flags().StringVar(&addVersion, "version", "", "Chart version constraint")
	addCmd.Flags().StringVar(&addImage, "image", "", "Docker image")
	addCmd.Flags().StringVar(&addCompFile, "compose-file", "", "Path to existing docker-compose.yaml")
	addCmd.Flags().StringArrayVar(&addSets, "set", nil, "Set Helm value (key=val, repeatable)")
	addCmd.Flags().StringVar(&addValues, "values", "", "Load values from file and merge inline into composed.yaml")
	addCmd.Flags().StringVar(&addValuesFile, "values-file", "", "Store values file reference (loaded at build time)")
	addCmd.Flags().StringArrayVar(&addPorts, "port", nil, "Port mapping (host:container, repeatable)")
	addCmd.Flags().StringArrayVar(&addEnvs, "env", nil, "Environment variable (KEY=VAL, repeatable)")
	addCmd.Flags().StringArrayVar(&addVolumes, "volume", nil, "Volume mount (name:/path, repeatable)")
	addCmd.Flags().StringArrayVar(&addDependsOn, "depends-on", nil, "Dependency (service name, repeatable)")
	rootCmd.AddCommand(addCmd)
}

// Package translate converts parsed K8s manifests into a Compose model.
package translate

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/docker-x/composed/internal/compose"
	"github.com/docker-x/composed/internal/k8s"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Opts controls translation behavior.
type Opts struct {
	// Compose project name.
	Project string
	// K8s Kinds to skip (e.g. ["Job", "CronJob"]).
	SkipKinds []string
}

// Result holds the output compose file and a translation report.
type Result struct {
	Compose *compose.File
	Report  *Report
}

// Report summarizes what was translated vs skipped.
type Report struct {
	Translated []ReportEntry
	Skipped    []ReportEntry
	Warnings   []string
}

type ReportEntry struct {
	Kind string
	Name string
}

func (r *Report) Print(w io.Writer) {
	if w == nil {
		w = os.Stderr
	}

	if len(r.Translated) > 0 {
		_, _ = fmt.Fprintf(w, "Translated:\n")
		for _, e := range r.Translated {
			_, _ = fmt.Fprintf(w, "  %s/%s\n", e.Kind, e.Name)
		}
	}
	if len(r.Skipped) > 0 {
		_, _ = fmt.Fprintf(w, "Skipped:\n")
		for _, e := range r.Skipped {
			_, _ = fmt.Fprintf(w, "  %s/%s\n", e.Kind, e.Name)
		}
	}
	for _, w2 := range r.Warnings {
		_, _ = fmt.Fprintf(w, "Warning: %s\n", w2)
	}
}

// Translate takes parsed K8s manifests and produces a Compose file.
func Translate(m *k8s.Manifests, opts Opts) (*Result, error) {
	skipSet := make(map[string]bool)
	for _, k := range opts.SkipKinds {
		skipSet[k] = true
	}

	ctx := &translateCtx{
		m:        m,
		skip:     skipSet,
		cf:       compose.NewFile(),
		report:   &Report{},
		cmIndex:  make(map[string]*corev1.ConfigMap),
		secIndex: make(map[string]*corev1.Secret),
		pvcIndex: make(map[string]*corev1.PersistentVolumeClaim),
	}

	if opts.Project != "" {
		ctx.cf.Project = opts.Project
	}

	// Build indexes for cross-referencing
	ctx.buildIndexes()

	// Translate workloads (Deployments, StatefulSets, DaemonSets)
	ctx.translateDeployments()
	ctx.translateStatefulSets()
	ctx.translateDaemonSets()

	// Translate Jobs
	if !skipSet["Job"] {
		ctx.translateJobs()
	}

	// Apply K8s Service port mappings to compose services
	ctx.applyServicePorts()

	// Report skipped resources from parser
	for _, s := range m.Skipped {
		ctx.report.Skipped = append(ctx.report.Skipped, ReportEntry{Kind: s.Kind, Name: s.Name})
	}

	return &Result{Compose: ctx.cf, Report: ctx.report}, nil
}

// --- Internal translation context ---

type translateCtx struct {
	m        *k8s.Manifests
	skip     map[string]bool
	cf       *compose.File
	report   *Report
	cmIndex  map[string]*corev1.ConfigMap
	secIndex map[string]*corev1.Secret
	pvcIndex map[string]*corev1.PersistentVolumeClaim
	// serviceLabelMap maps "label-key=label-val" sets to compose service names
	// for matching K8s Services to compose services
	svcLabels map[string]string // compose service name → serialized label set
}

func (c *translateCtx) buildIndexes() {
	for _, cm := range c.m.ConfigMaps {
		c.cmIndex[cm.Name] = cm
	}
	for _, sec := range c.m.Secrets {
		c.secIndex[sec.Name] = sec
	}
	for _, pvc := range c.m.PVCs {
		c.pvcIndex[pvc.Name] = pvc
	}
	c.svcLabels = make(map[string]string)
}

func (c *translateCtx) translateDeployments() {
	if c.skip["Deployment"] {
		return
	}
	for _, dep := range c.m.Deployments {
		labels := dep.Spec.Template.Labels
		podSpec := dep.Spec.Template.Spec
		name := dep.Name

		c.translatePodSpec(name, labels, &podSpec, dep.Spec.Replicas)
		c.report.Translated = append(c.report.Translated, ReportEntry{Kind: "Deployment", Name: name})
	}
}

func (c *translateCtx) translateStatefulSets() {
	if c.skip["StatefulSet"] {
		return
	}
	for _, ss := range c.m.StatefulSets {
		labels := ss.Spec.Template.Labels
		podSpec := ss.Spec.Template.Spec
		name := ss.Name

		// Inject synthetic PVC volumes for VolumeClaimTemplates so that
		// container volumeMounts referencing them get translated properly.
		for i := range ss.Spec.VolumeClaimTemplates {
			vct := &ss.Spec.VolumeClaimTemplates[i]
			podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
				Name: vct.Name,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: vct.Name,
					},
				},
			})
		}

		c.translatePodSpec(name, labels, &podSpec, ss.Spec.Replicas)

		// StatefulSet volumeClaimTemplates → named volumes
		for i := range ss.Spec.VolumeClaimTemplates {
			vct := &ss.Spec.VolumeClaimTemplates[i]
			c.cf.Volumes[vct.Name] = &compose.Volume{}
		}

		c.report.Translated = append(c.report.Translated, ReportEntry{Kind: "StatefulSet", Name: name})
	}
}

func (c *translateCtx) translateDaemonSets() {
	if c.skip["DaemonSet"] {
		return
	}
	for _, ds := range c.m.DaemonSets {
		labels := ds.Spec.Template.Labels
		podSpec := ds.Spec.Template.Spec
		name := ds.Name

		c.translatePodSpec(name, labels, &podSpec, nil)
		c.report.Translated = append(c.report.Translated, ReportEntry{Kind: "DaemonSet", Name: name})
	}
}

func (c *translateCtx) translateJobs() {
	for _, job := range c.m.Jobs {
		podSpec := job.Spec.Template.Spec
		name := job.Name

		svc := c.translateContainerToService(&podSpec.Containers[0], &podSpec)
		svc.Deploy = &compose.Deploy{
			RestartPolicy: &compose.RestartPolicy{
				Condition:   "on-failure",
				MaxAttempts: 3,
			},
		}
		c.cf.Services[name] = svc
		c.report.Translated = append(c.report.Translated, ReportEntry{Kind: "Job", Name: name})
	}
}

// translatePodSpec converts a pod spec (from Deployment/SS/DS) into compose services.
func (c *translateCtx) translatePodSpec(
	baseName string,
	podLabels map[string]string,
	podSpec *corev1.PodSpec,
	replicas *int32,
) {
	// Init containers → separate services with depends_on
	initSvcNames := make([]string, 0, len(podSpec.InitContainers))
	for i := range podSpec.InitContainers {
		initC := &podSpec.InitContainers[i]
		initName := fmt.Sprintf("%s-init-%s", baseName, initC.Name)
		svc := c.translateContainerToService(initC, podSpec)
		svc.Deploy = &compose.Deploy{
			RestartPolicy: &compose.RestartPolicy{
				Condition:   "on-failure",
				MaxAttempts: 3,
			},
		}
		c.cf.Services[initName] = svc
		initSvcNames = append(initSvcNames, initName)
	}

	// Main containers
	for i := range podSpec.Containers {
		container := &podSpec.Containers[i]
		name := baseName
		if i > 0 {
			name = fmt.Sprintf("%s-%s", baseName, container.Name)
		}

		svc := c.translateContainerToService(container, podSpec)

		// Replicas
		if replicas != nil && *replicas != 1 {
			r := int(*replicas)
			if svc.Deploy == nil {
				svc.Deploy = &compose.Deploy{}
			}
			svc.Deploy.Replicas = &r
		}

		// depends_on init containers
		prev := ""
		for _, initName := range initSvcNames {
			if prev != "" {
				// Chain: each init depends on the previous
				c.cf.Services[initName].DependsOn[prev] = compose.DependsOnCondition{
					Condition: "service_completed_successfully",
				}
			}
			prev = initName
		}
		if len(initSvcNames) > 0 {
			svc.DependsOn[initSvcNames[len(initSvcNames)-1]] = compose.DependsOnCondition{
				Condition: "service_completed_successfully",
			}
		}

		// Store pod labels for K8s Service matching
		c.svcLabels[name] = serializeLabels(podLabels)

		c.cf.Services[name] = svc
	}
}

// translateContainerToService converts a single K8s container into a compose Service.
func (c *translateCtx) translateContainerToService(
	container *corev1.Container,
	podSpec *corev1.PodSpec,
) *compose.Service {
	svc := compose.NewService(container.Image)

	// Command / args
	if len(container.Command) > 0 {
		svc.Entrypoint = container.Command
	}
	if len(container.Args) > 0 {
		svc.Command = container.Args
	}

	// Inline env vars
	for _, env := range container.Env {
		if env.Value != "" {
			svc.Environment[env.Name] = env.Value
		} else if env.ValueFrom != nil {
			c.resolveEnvValueFrom(svc, env.Name, env.ValueFrom)
		}
	}

	// envFrom (bulk merge from ConfigMap/Secret)
	for _, ef := range container.EnvFrom {
		if ef.ConfigMapRef != nil {
			c.mergeConfigMapEnv(svc, ef.ConfigMapRef.Name, ef.Prefix)
		}
		if ef.SecretRef != nil {
			c.mergeSecretEnv(svc, ef.SecretRef.Name, ef.Prefix)
		}
	}

	// Volume mounts
	for _, vm := range container.VolumeMounts {
		vol := findVolume(podSpec.Volumes, vm.Name)
		if vol == nil {
			continue
		}
		c.translateVolumeMount(svc, vm, vol)
	}

	// Resource limits
	if container.Resources.Limits != nil || container.Resources.Requests != nil {
		res := &compose.Resources{}
		if container.Resources.Limits != nil {
			res.Limits = convertResourceSpec(container.Resources.Limits)
		}
		if container.Resources.Requests != nil {
			res.Reservations = convertResourceSpec(container.Resources.Requests)
		}
		if svc.Deploy == nil {
			svc.Deploy = &compose.Deploy{}
		}
		svc.Deploy.Resources = res
	}

	// Probes → healthcheck (prefer liveness, fall back to readiness)
	probe := container.LivenessProbe
	if probe == nil {
		probe = container.ReadinessProbe
	}
	if probe != nil {
		svc.Healthcheck = translateProbe(probe, container.Ports)
	}

	return svc
}

// --- Env resolution helpers ---

func (c *translateCtx) resolveEnvValueFrom(svc *compose.Service, envName string, vf *corev1.EnvVarSource) {
	if vf.ConfigMapKeyRef != nil {
		cm, ok := c.cmIndex[vf.ConfigMapKeyRef.Name]
		if ok {
			if val, exists := cm.Data[vf.ConfigMapKeyRef.Key]; exists {
				svc.Environment[envName] = val
			}
		}
	}
	if vf.SecretKeyRef != nil {
		sec, ok := c.secIndex[vf.SecretKeyRef.Name]
		if ok {
			if raw, exists := sec.Data[vf.SecretKeyRef.Key]; exists {
				svc.Environment[envName] = string(raw)
			} else if val, exists := sec.StringData[vf.SecretKeyRef.Key]; exists {
				svc.Environment[envName] = val
			}
		}
	}
	// fieldRef, resourceFieldRef → skip (K8s-specific downward API)
}

func (c *translateCtx) mergeConfigMapEnv(svc *compose.Service, cmName, prefix string) {
	cm, ok := c.cmIndex[cmName]
	if !ok {
		c.report.Warnings = append(c.report.Warnings, fmt.Sprintf("ConfigMap %q referenced but not found", cmName))
		return
	}
	for k, v := range cm.Data {
		svc.Environment[prefix+k] = v
	}
}

func (c *translateCtx) mergeSecretEnv(svc *compose.Service, secName, prefix string) {
	sec, ok := c.secIndex[secName]
	if !ok {
		c.report.Warnings = append(c.report.Warnings, fmt.Sprintf("Secret %q referenced but not found", secName))
		return
	}
	c.report.Warnings = append(c.report.Warnings,
		fmt.Sprintf("Secret %q values inlined as plaintext in compose environment", secName))

	for k, v := range sec.Data {
		svc.Environment[prefix+k] = base64Decode(v)
	}
	for k, v := range sec.StringData {
		svc.Environment[prefix+k] = v
	}
}

func base64Decode(data []byte) string {
	// K8s secret .data is already decoded by the API types, but if it comes
	// from raw YAML it might still be base64-encoded.
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return string(data) // return as-is
	}
	return string(decoded)
}

// --- Volume mount translation ---

func (c *translateCtx) translateVolumeMount(
	svc *compose.Service,
	vm corev1.VolumeMount,
	vol *corev1.Volume,
) {
	mountPath := vm.MountPath

	switch {
	case vol.PersistentVolumeClaim != nil:
		target := mountPath
		if vm.SubPath != "" {
			target += "/" + vm.SubPath
		}
		claimName := vol.PersistentVolumeClaim.ClaimName
		svc.Volumes = append(svc.Volumes, fmt.Sprintf("%s:%s", claimName, target))
		c.cf.Volumes[claimName] = &compose.Volume{}

	case vol.ConfigMap != nil:
		cmName := vol.ConfigMap.Name
		cm, ok := c.cmIndex[cmName]
		if !ok {
			return
		}
		c.mountConfigData(svc, cmName, cm.Data, nil, mountPath, vm.SubPath)

	case vol.EmptyDir != nil:
		// In Compose, emptyDir maps to an anonymous volume at the mount path.
		// SubPath (a subdirectory within the shared K8s emptyDir) is ignored
		// since each mount becomes its own independent volume in Compose.
		if vol.EmptyDir.Medium == corev1.StorageMediumMemory {
			// Memory-backed emptyDir → tmpfs mount (or shm_size for /dev/shm)
			if mountPath == "/dev/shm" {
				// Docker has native /dev/shm support via shm_size
				svc.ShmSize = "256m"
			} else {
				svc.Tmpfs = append(svc.Tmpfs, mountPath)
			}
		} else {
			svc.Volumes = append(svc.Volumes, mountPath)
		}

	case vol.Secret != nil:
		secName := vol.Secret.SecretName
		sec, ok := c.secIndex[secName]
		if !ok {
			return
		}
		c.mountConfigData(svc, secName, sec.StringData, sec.Data, mountPath, vm.SubPath)
	}
}

// mountConfigData creates compose configs from ConfigMap/Secret data and mounts
// them into the service.
// If subPath is set, only the key matching subPath is mounted at mountPath exactly.
// If subPath is empty, all keys are mounted as files under mountPath/.
func (c *translateCtx) mountConfigData(
	svc *compose.Service,
	sourceName string,
	stringData map[string]string,
	byteData map[string][]byte,
	mountPath, subPath string,
) {
	// Collect all keys and their content
	type entry struct {
		key     string
		content string
	}
	var entries []entry
	for key, content := range stringData {
		entries = append(entries, entry{key, content})
	}
	for key, data := range byteData {
		// Only add if not already in stringData
		if _, exists := stringData[key]; !exists {
			entries = append(entries, entry{key, string(data)})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })

	if subPath != "" {
		// SubPath mount: only mount the matching key at the exact mountPath
		for _, e := range entries {
			if e.key == subPath {
				configName := fmt.Sprintf("%s-%s", sourceName, e.key)
				c.cf.Configs[configName] = &compose.Config{Content: e.content}
				svc.Configs = append(svc.Configs, compose.ServiceConfig{
					Source: configName,
					Target: mountPath,
				})
				return
			}
		}
	} else {
		// Directory mount: all keys become files under mountPath/
		for _, e := range entries {
			configName := fmt.Sprintf("%s-%s", sourceName, e.key)
			c.cf.Configs[configName] = &compose.Config{Content: e.content}
			svc.Configs = append(svc.Configs, compose.ServiceConfig{
				Source: configName,
				Target: path.Join(mountPath, e.key),
			})
		}
	}
}

func findVolume(vols []corev1.Volume, name string) *corev1.Volume {
	for i := range vols {
		if vols[i].Name == name {
			return &vols[i]
		}
	}
	return nil
}

// --- K8s Service → port mappings ---

func (c *translateCtx) applyServicePorts() {
	if c.skip["Service"] {
		return
	}
	for _, k8sSvc := range c.m.Services {
		// Find the compose service whose pod labels match the K8s Service selector
		target := c.findServiceTarget(k8sSvc.Spec.Selector)
		if target == "" {
			c.report.Skipped = append(c.report.Skipped, ReportEntry{Kind: "Service", Name: k8sSvc.Name})
			continue
		}

		composeSvc := c.cf.Services[target]
		if composeSvc == nil {
			continue
		}

		svcType := k8sSvc.Spec.Type
		if svcType == "" {
			svcType = corev1.ServiceTypeClusterIP
		}

		for _, port := range k8sSvc.Spec.Ports {
			targetPort := port.TargetPort.IntValue()
			if targetPort == 0 {
				targetPort = int(port.Port)
			}

			var portStr string
			switch svcType {
			case corev1.ServiceTypeClusterIP:
				portStr = fmt.Sprintf("%d:%d", port.Port, targetPort)
			case corev1.ServiceTypeNodePort:
				hostPort := port.NodePort
				if hostPort == 0 {
					hostPort = port.Port
				}
				portStr = fmt.Sprintf("%d:%d", hostPort, targetPort)
			case corev1.ServiceTypeLoadBalancer:
				portStr = fmt.Sprintf("%d:%d", port.Port, targetPort)
			}

			// Dedup: only add if not already present
			if portStr != "" && !containsStr(composeSvc.Ports, portStr) {
				composeSvc.Ports = append(composeSvc.Ports, portStr)
			}
		}

		c.report.Translated = append(c.report.Translated, ReportEntry{Kind: "Service", Name: k8sSvc.Name})
	}
}

// findServiceTarget matches a K8s Service selector to a compose service
// by comparing against the pod template labels stored during workload translation.
func (c *translateCtx) findServiceTarget(selector map[string]string) string {
	if len(selector) == 0 {
		return ""
	}
	target := serializeLabels(selector)
	for svcName, labels := range c.svcLabels {
		if labelsMatch(target, labels) {
			return svcName
		}
	}
	return ""
}

// --- Probe → Healthcheck ---

func translateProbe(probe *corev1.Probe, containerPorts []corev1.ContainerPort) *compose.Healthcheck {
	hc := &compose.Healthcheck{}

	switch {
	case probe.Exec != nil:
		hc.Test = append([]string{"CMD"}, probe.Exec.Command...)
	case probe.HTTPGet != nil:
		port := resolvePort(probe.HTTPGet.Port, containerPorts)
		path := probe.HTTPGet.Path
		if path == "" {
			path = "/"
		}
		scheme := strings.ToLower(string(probe.HTTPGet.Scheme))
		if scheme == "" {
			scheme = "http"
		}
		url := fmt.Sprintf("%s://localhost:%s%s", scheme, port, path)
		// Use python urllib (available in most images) with wget/curl fallback
		hc.Test = []string{"CMD-SHELL",
			fmt.Sprintf("python -c \"import urllib.request; urllib.request.urlopen('%s')\" 2>/dev/null || wget -q --spider %s || curl -sf %s > /dev/null", url, url, url)}
	case probe.TCPSocket != nil:
		port := resolvePort(probe.TCPSocket.Port, containerPorts)
		hc.Test = []string{"CMD", "sh", "-c",
			fmt.Sprintf("cat < /dev/tcp/localhost/%s", port)}
	}

	if probe.PeriodSeconds > 0 {
		hc.Interval = fmt.Sprintf("%ds", probe.PeriodSeconds)
	}
	if probe.TimeoutSeconds > 0 {
		hc.Timeout = fmt.Sprintf("%ds", probe.TimeoutSeconds)
	}
	if probe.FailureThreshold > 0 {
		hc.Retries = int(probe.FailureThreshold)
	}
	if probe.InitialDelaySeconds > 0 {
		hc.StartPeriod = fmt.Sprintf("%ds", probe.InitialDelaySeconds)
	}

	return hc
}

// --- Port resolution ---

// resolvePort converts an IntOrString port to a numeric string.
// If the port is a named port (e.g. "http"), look it up in the container's port list.
func resolvePort(port intstr.IntOrString, containerPorts []corev1.ContainerPort) string {
	if port.Type == intstr.Int {
		return fmt.Sprintf("%d", port.IntValue())
	}
	// Named port — resolve from container spec
	name := port.String()
	for _, cp := range containerPorts {
		if cp.Name == name {
			return fmt.Sprintf("%d", cp.ContainerPort)
		}
	}
	// Fallback: return the name as-is (will be broken, but at least visible)
	return name
}

// --- Resource conversion ---

func convertResourceSpec(rl corev1.ResourceList) *compose.ResourceSpec {
	if rl == nil {
		return nil
	}
	rs := &compose.ResourceSpec{}
	if cpu, ok := rl[corev1.ResourceCPU]; ok {
		// K8s CPU: "500m" → 0.5, "2" → 2.0
		millis := cpu.MilliValue()
		rs.CPUs = fmt.Sprintf("%.2f", float64(millis)/1000.0)
	}
	if mem, ok := rl[corev1.ResourceMemory]; ok {
		// K8s memory: bytes → compose shorthand (M, G)
		bytes := mem.Value()
		switch {
		case bytes >= 1<<30:
			rs.Memory = fmt.Sprintf("%dG", bytes/(1<<30))
		default:
			rs.Memory = fmt.Sprintf("%dM", bytes/(1<<20))
		}
	}
	return rs
}

// --- Label helpers ---

func serializeLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		parts = append(parts, k+"="+v)
	}
	// Sort for deterministic matching
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// containsStr returns true if `slice` already contains `s`.
func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// labelsMatch checks if `selector` labels are a subset of `pod` labels.
func labelsMatch(selector, pod string) bool {
	if selector == "" {
		return false
	}
	selectorParts := strings.Split(selector, ",")
	for _, sp := range selectorParts {
		if !strings.Contains(pod, sp) {
			return false
		}
	}
	return true
}

package compose

import (
	"bytes"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// Emit serializes a Compose File to YAML with deterministic key ordering.
func Emit(f *File) (string, error) {
	doc := &yaml.Node{Kind: yaml.MappingNode}

	// Header comment
	if f.Header != "" {
		doc.HeadComment = f.Header
	}

	// name:
	if f.Project != "" {
		addScalar(doc, "name", f.Project)
	}

	emitServices(doc, f.Services)
	emitVolumes(doc, f.Volumes)
	emitNetworks(doc, f.Networks)
	emitConfigs(doc, f.Configs)

	root := &yaml.Node{
		Kind:    yaml.DocumentNode,
		Content: []*yaml.Node{doc},
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return "", fmt.Errorf("yaml encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		return "", fmt.Errorf("yaml close: %w", err)
	}
	return buf.String(), nil
}

func emitServices(doc *yaml.Node, services map[string]*Service) {
	if len(services) == 0 {
		return
	}
	// services:
	svcNode := &yaml.Node{Kind: yaml.MappingNode}
	for _, name := range sortedKeys(services) {
		svc := services[name]
		svcNode.Content = append(svcNode.Content,
			scalarNode(name),
			serviceNode(svc),
		)
	}
	doc.Content = append(doc.Content, scalarNode("services"), svcNode)
}

func emitVolumes(doc *yaml.Node, volumes map[string]*Volume) {
	if len(volumes) == 0 {
		return
	}
	// volumes:
	volNode := &yaml.Node{Kind: yaml.MappingNode}
	for _, name := range sortedKeys(volumes) {
		v := volumes[name]
		inner := &yaml.Node{Kind: yaml.MappingNode}
		if v.External {
			addBool(inner, "external", true)
			if v.Name != "" {
				addScalar(inner, "name", v.Name)
			}
		} else {
			if v.Driver != "" && v.Driver != "local" {
				addScalar(inner, "driver", v.Driver)
			}
		}
		// Empty mapping = default volume
		if len(inner.Content) == 0 {
			volNode.Content = append(volNode.Content, scalarNode(name), &yaml.Node{Kind: yaml.MappingNode})
		} else {
			volNode.Content = append(volNode.Content, scalarNode(name), inner)
		}
	}
	doc.Content = append(doc.Content, scalarNode("volumes"), volNode)
}

func emitNetworks(doc *yaml.Node, networks map[string]*Network) {
	if len(networks) == 0 {
		return
	}
	// networks:
	netNode := &yaml.Node{Kind: yaml.MappingNode}
	for _, name := range sortedKeys(networks) {
		n := networks[name]
		inner := &yaml.Node{Kind: yaml.MappingNode}
		if n.Driver != "" && n.Driver != "bridge" {
			addScalar(inner, "driver", n.Driver)
		}
		netNode.Content = append(netNode.Content, scalarNode(name), inner)
	}
	doc.Content = append(doc.Content, scalarNode("networks"), netNode)
}

func emitConfigs(doc *yaml.Node, configs map[string]*Config) {
	if len(configs) == 0 {
		return
	}
	// configs:
	cfgNode := &yaml.Node{Kind: yaml.MappingNode}
	for _, name := range sortedKeys(configs) {
		c := configs[name]
		inner := &yaml.Node{Kind: yaml.MappingNode}
		if c.Content != "" {
			addScalar(inner, "content", c.Content)
		} else if c.File != "" {
			addScalar(inner, "file", c.File)
		}
		cfgNode.Content = append(cfgNode.Content, scalarNode(name), inner)
	}
	doc.Content = append(doc.Content, scalarNode("configs"), cfgNode)
}

// --- Node builders ---

func serviceNode(svc *Service) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	addServiceCore(n, svc)
	addServiceOrchestration(n, svc)
	return n
}

func addServiceCore(n *yaml.Node, svc *Service) {
	addScalar(n, "image", svc.Image)

	if len(svc.Entrypoint) > 0 {
		addSeq(n, "entrypoint", svc.Entrypoint)
	}
	if len(svc.Command) > 0 {
		addSeq(n, "command", svc.Command)
	}
	if len(svc.Environment) > 0 {
		envNode := &yaml.Node{Kind: yaml.MappingNode}
		for _, k := range sortedKeysMap(svc.Environment) {
			addScalar(envNode, k, svc.Environment[k])
		}
		n.Content = append(n.Content, scalarNode("environment"), envNode)
	}
	if len(svc.EnvFile) > 0 {
		addSeq(n, "env_file", svc.EnvFile)
	}
	if len(svc.Ports) > 0 {
		addSeq(n, "ports", svc.Ports)
	}
	if len(svc.Volumes) > 0 {
		addSeq(n, "volumes", svc.Volumes)
	}
	if len(svc.Tmpfs) > 0 {
		addSeq(n, "tmpfs", svc.Tmpfs)
	}
	if svc.ShmSize != "" {
		addScalar(n, "shm_size", svc.ShmSize)
	}
}

func addServiceOrchestration(n *yaml.Node, svc *Service) {
	if len(svc.DependsOn) > 0 {
		depNode := &yaml.Node{Kind: yaml.MappingNode}
		for _, name := range sortedKeysMap2(svc.DependsOn) {
			cond := svc.DependsOn[name]
			inner := &yaml.Node{Kind: yaml.MappingNode}
			addScalar(inner, "condition", cond.Condition)
			depNode.Content = append(depNode.Content, scalarNode(name), inner)
		}
		n.Content = append(n.Content, scalarNode("depends_on"), depNode)
	}
	if svc.Healthcheck != nil {
		n.Content = append(n.Content, scalarNode("healthcheck"), healthcheckNode(svc.Healthcheck))
	}
	if svc.Deploy != nil {
		n.Content = append(n.Content, scalarNode("deploy"), deployNode(svc.Deploy))
	}
	if svc.Restart != "" {
		addScalar(n, "restart", svc.Restart)
	}
	if len(svc.Configs) > 0 {
		cfgSeq := &yaml.Node{Kind: yaml.SequenceNode}
		for _, sc := range svc.Configs {
			item := &yaml.Node{Kind: yaml.MappingNode}
			addScalar(item, "source", sc.Source)
			addScalar(item, "target", sc.Target)
			cfgSeq.Content = append(cfgSeq.Content, item)
		}
		n.Content = append(n.Content, scalarNode("configs"), cfgSeq)
	}
	if len(svc.Profiles) > 0 {
		addSeq(n, "profiles", svc.Profiles)
	}
	if len(svc.Labels) > 0 {
		lblNode := &yaml.Node{Kind: yaml.MappingNode}
		for _, k := range sortedKeysMap(svc.Labels) {
			addScalar(lblNode, k, svc.Labels[k])
		}
		n.Content = append(n.Content, scalarNode("labels"), lblNode)
	}
}

func healthcheckNode(hc *Healthcheck) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	if len(hc.Test) > 0 {
		addSeq(n, "test", hc.Test)
	}
	if hc.Interval != "" {
		addScalar(n, "interval", hc.Interval)
	}
	if hc.Timeout != "" {
		addScalar(n, "timeout", hc.Timeout)
	}
	if hc.Retries > 0 {
		addInt(n, "retries", hc.Retries)
	}
	if hc.StartPeriod != "" {
		addScalar(n, "start_period", hc.StartPeriod)
	}
	return n
}

func deployNode(d *Deploy) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	if d.Replicas != nil {
		addInt(n, "replicas", *d.Replicas)
	}
	if d.Resources != nil {
		resNode := &yaml.Node{Kind: yaml.MappingNode}
		if d.Resources.Limits != nil {
			resNode.Content = append(resNode.Content, scalarNode("limits"), resourceSpecNode(d.Resources.Limits))
		}
		if d.Resources.Reservations != nil {
			resNode.Content = append(resNode.Content, scalarNode("reservations"), resourceSpecNode(d.Resources.Reservations))
		}
		n.Content = append(n.Content, scalarNode("resources"), resNode)
	}
	if d.RestartPolicy != nil {
		rpNode := &yaml.Node{Kind: yaml.MappingNode}
		if d.RestartPolicy.Condition != "" {
			addScalar(rpNode, "condition", d.RestartPolicy.Condition)
		}
		if d.RestartPolicy.MaxAttempts > 0 {
			addInt(rpNode, "max_attempts", d.RestartPolicy.MaxAttempts)
		}
		n.Content = append(n.Content, scalarNode("restart_policy"), rpNode)
	}
	return n
}

func resourceSpecNode(rs *ResourceSpec) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	if rs.CPUs != "" {
		addScalar(n, "cpus", rs.CPUs)
	}
	if rs.Memory != "" {
		addScalar(n, "memory", rs.Memory)
	}
	return n
}

// --- Helpers ---

func scalarNode(val string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: val}
}

func addScalar(parent *yaml.Node, key, val string) {
	if val == "" {
		return
	}
	parent.Content = append(parent.Content, scalarNode(key), scalarNode(val))
}

func addInt(parent *yaml.Node, key string, val int) {
	parent.Content = append(parent.Content,
		scalarNode(key),
		&yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprintf("%d", val), Tag: "!!int"},
	)
}

func addBool(parent *yaml.Node, key string, val bool) {
	parent.Content = append(parent.Content,
		scalarNode(key),
		&yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprintf("%t", val), Tag: "!!bool"},
	)
}

func addSeq(parent *yaml.Node, key string, items []string) {
	seq := &yaml.Node{Kind: yaml.SequenceNode}
	for _, item := range items {
		seq.Content = append(seq.Content, scalarNode(item))
	}
	parent.Content = append(parent.Content, scalarNode(key), seq)
}

func sortedKeys[V any](m map[string]*V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeysMap(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeysMap2[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

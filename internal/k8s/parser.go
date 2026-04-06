// Package k8s parses multi-document Kubernetes YAML into typed resource objects.
package k8s

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	sigyaml "sigs.k8s.io/yaml"

	goyaml "gopkg.in/yaml.v3"
)

// Manifests holds all parsed K8s resources, bucketed by kind.
type Manifests struct {
	Deployments  []*appsv1.Deployment
	StatefulSets []*appsv1.StatefulSet
	DaemonSets   []*appsv1.DaemonSet
	Services     []*corev1.Service
	ConfigMaps   []*corev1.ConfigMap
	Secrets      []*corev1.Secret
	PVCs         []*corev1.PersistentVolumeClaim
	Jobs         []*batchv1.Job
	CronJobs     []*batchv1.CronJob

	// Unknown resources that couldn't be parsed into typed objects.
	// Stored as raw JSON with kind/name for reporting.
	Skipped []SkippedResource
}

// SkippedResource represents a K8s resource we recognized but can't translate.
type SkippedResource struct {
	Kind      string
	Name      string
	Namespace string
}

// Parse reads multi-document YAML and returns typed Manifests.
func Parse(data []byte) (*Manifests, error) {
	m := &Manifests{}

	docs, err := splitYAMLDocs(data)
	if err != nil {
		return nil, err
	}
	for i, doc := range docs {
		doc = bytes.TrimSpace(doc)
		if len(doc) == 0 {
			continue
		}

		// Convert YAML to JSON (k8s types unmarshal from JSON)
		jsonData, err := sigyaml.YAMLToJSON(doc)
		if err != nil {
			return nil, fmt.Errorf("doc %d: yaml to json: %w", i, err)
		}

		// Peek at kind/apiVersion
		var meta struct {
			Kind       string `json:"kind"`
			APIVersion string `json:"apiVersion"`
			Metadata   struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(jsonData, &meta); err != nil {
			return nil, fmt.Errorf("doc %d: unmarshal meta: %w", i, err)
		}

		if meta.Kind == "" {
			continue // skip empty or non-resource docs
		}

		known, err := m.parseResource(meta.Kind, meta.Metadata.Name, jsonData)
		if err != nil {
			return nil, fmt.Errorf("doc %d (%s %s): %w", i, meta.Kind, meta.Metadata.Name, err)
		}
		if !known {
			m.Skipped = append(m.Skipped, SkippedResource{
				Kind:      meta.Kind,
				Name:      meta.Metadata.Name,
				Namespace: meta.Metadata.Namespace,
			})
		}
	}

	return m, nil
}

func (m *Manifests) parseResource(kind, name string, jsonData []byte) (bool, error) {
	switch kind {
	case "Deployment":
		obj := &appsv1.Deployment{}
		if err := unmarshalK8s(jsonData, obj); err != nil {
			return false, err
		}
		m.Deployments = append(m.Deployments, obj)

	case "StatefulSet":
		obj := &appsv1.StatefulSet{}
		if err := unmarshalK8s(jsonData, obj); err != nil {
			return false, err
		}
		m.StatefulSets = append(m.StatefulSets, obj)

	case "DaemonSet":
		obj := &appsv1.DaemonSet{}
		if err := unmarshalK8s(jsonData, obj); err != nil {
			return false, err
		}
		m.DaemonSets = append(m.DaemonSets, obj)

	case "Service":
		obj := &corev1.Service{}
		if err := unmarshalK8s(jsonData, obj); err != nil {
			return false, err
		}
		m.Services = append(m.Services, obj)

	case "ConfigMap":
		obj := &corev1.ConfigMap{}
		if err := unmarshalK8s(jsonData, obj); err != nil {
			return false, err
		}
		m.ConfigMaps = append(m.ConfigMaps, obj)

	case "Secret":
		obj := &corev1.Secret{}
		if err := unmarshalK8s(jsonData, obj); err != nil {
			return false, err
		}
		m.Secrets = append(m.Secrets, obj)

	case "PersistentVolumeClaim":
		obj := &corev1.PersistentVolumeClaim{}
		if err := unmarshalK8s(jsonData, obj); err != nil {
			return false, err
		}
		m.PVCs = append(m.PVCs, obj)

	case "Job":
		obj := &batchv1.Job{}
		if err := unmarshalK8s(jsonData, obj); err != nil {
			return false, err
		}
		m.Jobs = append(m.Jobs, obj)

	case "CronJob":
		obj := &batchv1.CronJob{}
		if err := unmarshalK8s(jsonData, obj); err != nil {
			return false, err
		}
		m.CronJobs = append(m.CronJobs, obj)

	default:
		return false, nil
	}
	return true, nil
}

// splitYAMLDocs splits multi-document YAML on document boundaries using a
// proper YAML decoder, which correctly handles "---" inside block scalars.
func splitYAMLDocs(data []byte) ([][]byte, error) {
	var docs [][]byte
	dec := goyaml.NewDecoder(bytes.NewReader(data))
	for {
		var doc interface{}
		if err := dec.Decode(&doc); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("invalid YAML: %w", err)
		}
		if doc == nil {
			continue
		}
		out, err := goyaml.Marshal(doc)
		if err != nil {
			continue
		}
		out = bytes.TrimSpace(out)
		if len(out) > 0 {
			docs = append(docs, out)
		}
	}
	return docs, nil
}

func unmarshalK8s(jsonData []byte, obj runtime.Object) error {
	return json.Unmarshal(jsonData, obj)
}

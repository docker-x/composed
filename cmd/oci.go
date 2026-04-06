package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var ociAcceptHeader = strings.Join([]string{
	"application/vnd.oci.image.index.v1+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.v2+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
}, ", ")

// ociArtifactType queries an OCI registry manifest to determine whether
// the reference points to a Helm chart or a container image.
// Returns "helm", "image", or "" if detection fails.
func ociArtifactType(ociRef string) string {
	registry, repo, tag, ok := parseOCIRef(ociRef)
	if !ok {
		return ""
	}

	client := &http.Client{Timeout: 10 * time.Second}

	// Get an auth token (may be empty for registries that don't require it)
	token := getRegistryToken(client, registry, repo)

	// Try the specified tag (or "latest")
	if result := fetchAndClassify(client, registry, repo, tag, token); result != "" {
		return result
	}

	// If using the default "latest" tag and it failed, try the newest tag
	if tag == "latest" {
		if newest := fetchNewestTag(client, registry, repo, token); newest != "" {
			return fetchAndClassify(client, registry, repo, newest, token)
		}
	}

	return ""
}

// fetchAndClassify fetches a manifest and classifies the artifact.
func fetchAndClassify(client *http.Client, registry, repo, tag, token string) string {
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", ociAcceptHeader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ""
	}
	return classifyManifest(body)
}

// getRegistryToken does an anonymous auth handshake with the registry.
// Returns "" if no auth is needed or if auth fails.
func getRegistryToken(client *http.Client, registry, repo string) string {
	// Probe /v2/<repo>/manifests/latest to trigger a 401 challenge
	url := fmt.Sprintf("https://%s/v2/%s/manifests/latest", registry, repo)
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		return "" // no auth needed
	}

	return fetchAnonymousToken(client, resp, repo)
}

// fetchNewestTag retrieves the tag list and returns the last one
// (registries typically return tags in chronological/version order).
func fetchNewestTag(client *http.Client, registry, repo, token string) string {
	url := fmt.Sprintf("https://%s/v2/%s/tags/list", registry, repo)
	req, _ := http.NewRequest("GET", url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Tags) == 0 {
		return ""
	}
	return result.Tags[len(result.Tags)-1]
}

// parseOCIRef splits "oci://registry/repo/path:tag" into components.
func parseOCIRef(ref string) (registry, repo, tag string, ok bool) {
	ref = strings.TrimPrefix(ref, "oci://")
	// Split off tag
	tag = "latest"
	if i := strings.LastIndex(ref, ":"); i > 0 {
		// Make sure it's a tag, not part of the registry (e.g. localhost:5000)
		after := ref[i+1:]
		if !strings.Contains(after, "/") {
			tag = after
			ref = ref[:i]
		}
	}
	// First path segment is the registry
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) < 2 {
		return "", "", "", false
	}
	return parts[0], parts[1], tag, true
}

// fetchAnonymousToken handles the WWW-Authenticate challenge for anonymous access.
func fetchAnonymousToken(client *http.Client, resp *http.Response, repo string) string {
	auth := resp.Header.Get("WWW-Authenticate")
	if auth == "" {
		return ""
	}

	params := parseWWWAuthenticate(auth)
	realm := params["realm"]
	if realm == "" {
		return ""
	}

	tokenURL := realm
	sep := "?"
	if s := params["service"]; s != "" {
		tokenURL += sep + "service=" + s
		sep = "&"
	}
	if s := params["scope"]; s != "" {
		tokenURL += sep + "scope=" + s
	} else {
		tokenURL += sep + "scope=repository:" + repo + ":pull"
	}

	tokenResp, err := client.Get(tokenURL)
	if err != nil || tokenResp.StatusCode != http.StatusOK {
		if tokenResp != nil {
			_ = tokenResp.Body.Close()
		}
		return ""
	}
	defer func() { _ = tokenResp.Body.Close() }()

	var result struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&result); err != nil {
		return ""
	}
	if result.Token != "" {
		return result.Token
	}
	return result.AccessToken
}

// parseWWWAuthenticate extracts key=value pairs from a Bearer challenge.
func parseWWWAuthenticate(header string) map[string]string {
	params := make(map[string]string)
	header = strings.TrimPrefix(header, "Bearer ")
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		params[k] = strings.Trim(v, `"`)
	}
	return params
}

// classifyManifest inspects a manifest JSON to determine the artifact type.
func classifyManifest(data []byte) string {
	var manifest struct {
		MediaType string `json:"mediaType"`
		Config    struct {
			MediaType string `json:"mediaType"`
		} `json:"config"`
		Manifests []struct {
			MediaType    string `json:"mediaType"`
			ArtifactType string `json:"artifactType"`
		} `json:"manifests"`
	}

	if err := json.Unmarshal(data, &manifest); err != nil {
		return ""
	}

	// Single manifest — check config.mediaType
	if ct := manifest.Config.MediaType; ct != "" {
		if strings.Contains(ct, "helm") {
			return "helm"
		}
		return "image"
	}

	// Index / manifest list — check first entry
	for _, m := range manifest.Manifests {
		if strings.Contains(m.ArtifactType, "helm") || strings.Contains(m.MediaType, "helm") {
			return "helm"
		}
	}

	// Has manifests[] entries but no helm markers → image
	if len(manifest.Manifests) > 0 {
		return "image"
	}

	return ""
}

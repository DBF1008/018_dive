package docker

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ociIndex struct {
	SchemaVersion int             `json:"schemaVersion"`
	Manifests     []ociDescriptor `json:"manifests"`
}

type ociDescriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

type ociManifest struct {
	SchemaVersion int             `json:"schemaVersion"`
	Config        ociDescriptor   `json:"config"`
	Layers        []ociDescriptor `json:"layers"`
}

// digestToPath converts an OCI digest to a blob path.
// e.g. "sha256:abc123" -> "blobs/sha256/abc123"
func digestToPath(digest string) string {
	return "blobs/" + strings.Replace(digest, ":", "/", 1)
}

func parseOCIIndex(data []byte) (ociIndex, error) {
	var idx ociIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return idx, fmt.Errorf("failed to parse OCI index: %w", err)
	}
	return idx, nil
}

func parseOCIManifest(data []byte) (ociManifest, error) {
	var m ociManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("failed to parse OCI manifest: %w", err)
	}
	return m, nil
}

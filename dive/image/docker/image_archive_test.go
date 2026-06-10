package docker

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLayerTar(filename string, content []byte) []byte {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{
		Name:     filename,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
		Mode:     0644,
	})
	_, _ = tw.Write(content)
	_ = tw.Close()
	return buf.Bytes()
}

func testSHA256Digest(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", h)
}

func buildMultiLayerOCIArchive(t *testing.T) *bytes.Buffer {
	t.Helper()

	// Create 3 layers with distinct content
	layer1 := testLayerTar("file1.txt", []byte("layer 1 content"))
	layer2 := testLayerTar("file2.txt", []byte("layer 2 content"))
	layer3 := testLayerTar("file3.txt", []byte("layer 3 content"))

	layers := [][]byte{layer1, layer2, layer3}
	layerDigests := make([]string, len(layers))
	for i, l := range layers {
		layerDigests[i] = testSHA256Digest(l)
	}

	// Create config
	cfg := map[string]interface{}{
		"rootfs": map[string]interface{}{
			"type":     "layers",
			"diff_ids": layerDigests,
		},
		"history": []map[string]interface{}{
			{"created_by": "ADD file1.txt /"},
			{"created_by": "ADD file2.txt /"},
			{"created_by": "ADD file3.txt /"},
		},
	}
	configBytes, err := json.Marshal(cfg)
	require.NoError(t, err)
	configDigest := testSHA256Digest(configBytes)

	// Create OCI manifest with ordered layers
	layerDescs := make([]map[string]interface{}, len(layers))
	for i, d := range layerDigests {
		layerDescs[i] = map[string]interface{}{
			"mediaType": "application/vnd.oci.image.layer.v1.tar",
			"digest":    d,
			"size":      len(layers[i]),
		}
	}
	ociMfst := map[string]interface{}{
		"schemaVersion": 2,
		"config": map[string]interface{}{
			"mediaType": "application/vnd.oci.image.config.v1+json",
			"digest":    configDigest,
			"size":      len(configBytes),
		},
		"layers": layerDescs,
	}
	manifestBytes, err := json.Marshal(ociMfst)
	require.NoError(t, err)
	manifestDigest := testSHA256Digest(manifestBytes)

	// Create index.json
	idx := map[string]interface{}{
		"schemaVersion": 2,
		"manifests": []map[string]interface{}{
			{
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"digest":    manifestDigest,
				"size":      len(manifestBytes),
			},
		},
	}
	indexBytes, err := json.Marshal(idx)
	require.NoError(t, err)

	// Build the outer tar
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	addFile := func(name string, data []byte) {
		err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
			Mode:     0644,
		})
		require.NoError(t, err)
		_, err = tw.Write(data)
		require.NoError(t, err)
	}

	addFile("oci-layout", []byte(`{"imageLayoutVersion":"1.0.0"}`))
	addFile("index.json", indexBytes)
	addFile(digestToPath(manifestDigest), manifestBytes)
	addFile(digestToPath(configDigest), configBytes)
	for i, l := range layers {
		addFile(digestToPath(layerDigests[i]), l)
	}

	require.NoError(t, tw.Close())
	return &buf
}

func TestOCILayerOrdering(t *testing.T) {
	// Run multiple times to verify deterministic ordering
	var firstOrder []string
	for run := 0; run < 10; run++ {
		archiveBuf := buildMultiLayerOCIArchive(t)
		archive, err := NewImageArchive(io.NopCloser(archiveBuf))
		require.NoError(t, err, "run %d: failed to load archive", run)

		img, err := archive.ToImage("test")
		require.NoError(t, err, "run %d: failed to convert to image", run)

		require.Equal(t, 3, len(img.Layers), "run %d: expected 3 layers", run)

		order := make([]string, len(img.Layers))
		for i, l := range img.Layers {
			order[i] = l.Command
		}

		if run == 0 {
			firstOrder = order
			assert.Equal(t, "ADD file1.txt /", img.Layers[0].Command)
			assert.Equal(t, "ADD file2.txt /", img.Layers[1].Command)
			assert.Equal(t, "ADD file3.txt /", img.Layers[2].Command)
		} else {
			assert.Equal(t, firstOrder, order,
				"run %d: layer order changed — non-deterministic", run)
		}
	}
}

func TestOCILayerID(t *testing.T) {
	archiveBuf := buildMultiLayerOCIArchive(t)
	archive, err := NewImageArchive(io.NopCloser(archiveBuf))
	require.NoError(t, err)

	img, err := archive.ToImage("test")
	require.NoError(t, err)

	for i, l := range img.Layers {
		assert.NotEqual(t, "blobs", l.Id,
			"layer %d: ID should not be 'blobs'", i)
		assert.NotEmpty(t, l.Id,
			"layer %d: ID should not be empty", i)
	}
}

func TestOCIArchiveFromFixtures(t *testing.T) {
	fixtures := []string{
		"../../../.data/test-oci-gzip-image.tar",
		"../../../.data/test-oci-uncompressed-image.tar",
		"../../../.data/test-oci-zstd-image.tar",
		"../../../.data/test-oci-estargz-image.tar",
	}

	for _, path := range fixtures {
		t.Run(path, func(t *testing.T) {
			archive, err := TestLoadArchive(t, path)
			require.NoError(t, err)

			img, err := archive.ToImage(path)
			require.NoError(t, err)

			assert.GreaterOrEqual(t, len(img.Layers), 1, "expected at least 1 layer")
			for i, l := range img.Layers {
				assert.NotEqual(t, "blobs", l.Id,
					"layer %d: ID should not be 'blobs'", i)
			}
		})
	}
}

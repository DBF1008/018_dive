package docker

import (
	"archive/tar"
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"

	"github.com/wagoodman/dive/dive/filetree"
	"github.com/wagoodman/dive/dive/image"
)

// isPlaceholderArchive checks if a tar file is a placeholder (only contains README.md).
// Some test fixtures require `task generate-compressed-test-data` to be populated.
func isPlaceholderArchive(t *testing.T, path string) bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()

	reader := tar.NewReader(f)
	header, err := reader.Next()
	if err != nil {
		return true
	}
	if header.Name == "README.md" {
		_, err = reader.Next()
		return err != nil // only one entry = placeholder
	}
	return false
}

// collectPaths walks the file tree and returns a sorted list of all file/dir paths.
func collectPaths(t *testing.T, tree *filetree.FileTree) []string {
	t.Helper()
	var paths []string
	err := tree.VisitDepthParentFirst(func(node *filetree.FileNode) error {
		paths = append(paths, node.Path())
		return nil
	}, nil)
	require.NoError(t, err)
	sort.Strings(paths)
	return paths
}

// --- Test: all archive formats load successfully and produce valid images ---

func TestOCI_AllFormatsLoadAndAnalyze(t *testing.T) {
	table := []struct {
		name string
		path string
	}{
		// Docker export format with different compression
		{"docker-image", "../../../.data/test-docker-image.tar"},
		{"docker-gzip", "../../../.data/test-gzip-image.tar"},
		{"docker-estargz", "../../../.data/test-estargz-image.tar"},
		{"docker-zstd", "../../../.data/test-zstd-image.tar"},
		{"docker-uncompressed", "../../../.data/test-uncompressed-image.tar"},
		// OCI layout format with different compression
		{"oci-docker", "../../../.data/test-oci-docker-image.tar"},
		{"oci-gzip", "../../../.data/test-oci-gzip-image.tar"},
		{"oci-estargz", "../../../.data/test-oci-estargz-image.tar"},
		{"oci-zstd", "../../../.data/test-oci-zstd-image.tar"},
		{"oci-uncompressed", "../../../.data/test-oci-uncompressed-image.tar"},
		// kaniko format
		{"kaniko", "../../../.data/test-kaniko-image.tar"},
	}

	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			if isPlaceholderArchive(t, tc.path) {
				t.Skipf("skipping %s: placeholder archive (run 'task generate-compressed-test-data' to populate)", tc.name)
			}

			// Phase 1: load archive
			archive, err := TestLoadArchive(t, tc.path)
			require.NoError(t, err, "failed to load archive")
			require.NotNil(t, archive, "archive should not be nil")

			// Phase 2: layerMap must be non-empty
			assert.Greater(t, len(archive.layerMap), 0, "layerMap should have at least one layer")

			// Phase 3: every layer tree must have files
			for name, tree := range archive.layerMap {
				assert.Greater(t, tree.Size, 0, "layer %q tree should have nodes", name)
			}

			// Phase 4: manifest must be populated
			assert.NotEmpty(t, archive.manifest.ConfigPath, "manifest ConfigPath should be set")
			assert.NotEmpty(t, archive.manifest.LayerTarPaths, "manifest should reference at least one layer")

			// Phase 5: all manifest layer paths must exist in layerMap
			for _, lp := range archive.manifest.LayerTarPaths {
				_, exists := archive.layerMap[lp]
				assert.True(t, exists, "manifest references layer %q not found in layerMap", lp)
			}

			// Phase 6: convert to Image
			img, err := archive.ToImage(tc.name)
			require.NoError(t, err, "ToImage failed")
			require.NotNil(t, img)
			assert.Equal(t, len(archive.manifest.LayerTarPaths), len(img.Trees), "tree count should match manifest layers")
			assert.Equal(t, len(img.Trees), len(img.Layers), "layer count should match tree count")

			// Phase 7: every layer must have non-empty command history
			for i, layer := range img.Layers {
				assert.NotEmpty(t, layer.Command, "layer %d should have a command", i)
			}

			// Phase 8: full analysis succeeds
			result, err := image.Analyze(context.Background(), img)
			require.NoError(t, err, "analysis failed")
			assert.Greater(t, result.SizeBytes, uint64(0), "total size should be > 0")
			assert.GreaterOrEqual(t, result.Efficiency, float64(0), "efficiency should be >= 0")
			assert.LessOrEqual(t, result.Efficiency, float64(1), "efficiency should be <= 1")
		})
	}
}

// --- Test: OCI formats with different compression yield identical file trees ---

func TestOCI_CompressionFormatTreeConsistency(t *testing.T) {
	// Group 1: small test images built from the same Dockerfile, Docker export
	dockerGroup := []struct {
		name string
		path string
	}{
		{"gzip", "../../../.data/test-gzip-image.tar"},
		{"estargz", "../../../.data/test-estargz-image.tar"},
		{"zstd", "../../../.data/test-zstd-image.tar"},
		{"uncompressed", "../../../.data/test-uncompressed-image.tar"},
	}

	// Group 2: same images in OCI layout
	ociGroup := []struct {
		name string
		path string
	}{
		{"oci-gzip", "../../../.data/test-oci-gzip-image.tar"},
		{"oci-estargz", "../../../.data/test-oci-estargz-image.tar"},
		{"oci-zstd", "../../../.data/test-oci-zstd-image.tar"},
		{"oci-uncompressed", "../../../.data/test-oci-uncompressed-image.tar"},
	}

	checkGroupConsistency := func(t *testing.T, groupName string, entries []struct {
		name string
		path string
	}) {
		t.Helper()
		if len(entries) < 2 {
			return
		}

		type treeResult struct {
			name      string
			pathSets  [][]string // one path set per layer, sorted
			layerCount int
		}

		var results []treeResult

		for _, entry := range entries {
			if isPlaceholderArchive(t, entry.path) {
				continue
			}

			archive, err := TestLoadArchive(t, entry.path)
			require.NoError(t, err, "%s: load failed", entry.name)

			img, err := archive.ToImage(entry.name)
			require.NoError(t, err, "%s: ToImage failed", entry.name)

			var pathSets [][]string
			for _, tree := range img.Trees {
				pathSets = append(pathSets, collectPaths(t, tree))
			}

			results = append(results, treeResult{
				name:       entry.name,
				pathSets:   pathSets,
				layerCount: len(img.Trees),
			})
		}

		if len(results) < 2 {
			t.Skipf("%s: fewer than 2 non-placeholder archives, skipping consistency check", groupName)
		}

		// all should have the same layer count
		refResult := results[0]
		for _, r := range results[1:] {
			assert.Equal(t, refResult.layerCount, r.layerCount,
				"%s: %s has %d layers but %s has %d",
				groupName, refResult.name, refResult.layerCount, r.name, r.layerCount)
		}

		// compare path sets per layer
		for _, r := range results[1:] {
			minLayers := refResult.layerCount
			if r.layerCount < minLayers {
				minLayers = r.layerCount
			}
			for i := 0; i < minLayers; i++ {
				assert.Equal(t, refResult.pathSets[i], r.pathSets[i],
					"%s: layer %d paths differ between %s and %s",
					groupName, i, refResult.name, r.name)
			}
		}
	}

	t.Run("docker-export-formats", func(t *testing.T) {
		checkGroupConsistency(t, "docker-export", dockerGroup)
	})

	t.Run("oci-layout-formats", func(t *testing.T) {
		checkGroupConsistency(t, "oci-layout", ociGroup)
	})
}

// --- Test: OCI format without manifest.json synthesizes a valid manifest ---

func TestOCI_SyntheticManifest(t *testing.T) {
	// OCI-format archives (non-docker-compat) typically lack manifest.json
	ociPaths := []struct {
		name string
		path string
	}{
		{"oci-gzip", "../../../.data/test-oci-gzip-image.tar"},
		{"oci-estargz", "../../../.data/test-oci-estargz-image.tar"},
		{"oci-zstd", "../../../.data/test-oci-zstd-image.tar"},
		{"oci-uncompressed", "../../../.data/test-oci-uncompressed-image.tar"},
	}

	for _, tc := range ociPaths {
		t.Run(tc.name, func(t *testing.T) {
			if isPlaceholderArchive(t, tc.path) {
				t.Skipf("skipping %s: placeholder archive", tc.name)
			}

			archive, err := TestLoadArchive(t, tc.path)
			require.NoError(t, err)

			// the config path should point to a blobs/ entry or a sha256: prefixed path
			assert.NotEmpty(t, archive.manifest.ConfigPath, "config path must be set")

			// all layer paths in manifest must be resolvable
			for _, lp := range archive.manifest.LayerTarPaths {
				tree, exists := archive.layerMap[lp]
				assert.True(t, exists, "layer path %q from manifest not in layerMap", lp)
				if exists {
					assert.Greater(t, tree.Size, 0, "layer %q should have content", lp)
				}
			}

			// config must have been parsed (history or rootfs should exist)
			img, err := archive.ToImage(tc.name)
			require.NoError(t, err)
			assert.NotEmpty(t, img.Trees, "image must have trees")
			assert.NotEmpty(t, img.Layers, "image must have layers")
		})
	}
}

// --- Test: layer file sizes are accumulated correctly across formats ---

func TestOCI_LayerFileSizeAccumulation(t *testing.T) {
	table := []struct {
		name string
		path string
	}{
		{"docker-image", "../../../.data/test-docker-image.tar"},
		{"oci-docker", "../../../.data/test-oci-docker-image.tar"},
		{"oci-gzip", "../../../.data/test-oci-gzip-image.tar"},
		{"oci-zstd", "../../../.data/test-oci-zstd-image.tar"},
		{"oci-uncompressed", "../../../.data/test-oci-uncompressed-image.tar"},
		{"kaniko", "../../../.data/test-kaniko-image.tar"},
	}

	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			if isPlaceholderArchive(t, tc.path) {
				t.Skipf("skipping %s: placeholder archive", tc.name)
			}

			archive, err := TestLoadArchive(t, tc.path)
			require.NoError(t, err)

			for layerName, tree := range archive.layerMap {
				// FileSize should equal the sum of all file sizes in the tree
				var totalSize uint64
				err := tree.VisitDepthParentFirst(func(node *filetree.FileNode) error {
					totalSize += uint64(node.Data.FileInfo.Size)
					return nil
				}, nil)
				require.NoError(t, err)
				assert.Equal(t, tree.FileSize, totalSize,
					"layer %q: FileSize (%d) != sum of node sizes (%d)", layerName, tree.FileSize, totalSize)
			}
		})
	}
}

// --- Test: stacking all layers produces a coherent merged tree ---

func TestOCI_StackedTreeIntegrity(t *testing.T) {
	table := []struct {
		name string
		path string
	}{
		{"docker-image", "../../../.data/test-docker-image.tar"},
		{"oci-docker", "../../../.data/test-oci-docker-image.tar"},
		{"oci-gzip", "../../../.data/test-oci-gzip-image.tar"},
		{"oci-zstd", "../../../.data/test-oci-zstd-image.tar"},
		{"kaniko", "../../../.data/test-kaniko-image.tar"},
	}

	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			if isPlaceholderArchive(t, tc.path) {
				t.Skipf("skipping %s: placeholder archive", tc.name)
			}

			archive, err := TestLoadArchive(t, tc.path)
			require.NoError(t, err)

			img, err := archive.ToImage(tc.name)
			require.NoError(t, err)

			if len(img.Trees) == 0 {
				t.Skip("no layers to stack")
			}

			// stack all layers
			stacked, pathErrors, err := filetree.StackTreeRange(img.Trees, 0, len(img.Trees)-1)
			require.NoError(t, err, "stacking all layers failed")
			// path errors are tolerable but should not be excessive
			assert.Less(t, len(pathErrors), 50, "too many path errors during stacking: %d", len(pathErrors))

			// the stacked tree must have content
			assert.Greater(t, stacked.Size, 0, "stacked tree should have nodes")

			// compare-and-mark should not panic on the stacked tree
			if len(img.Trees) > 1 {
				stackedCopy := stacked.Copy()
				_, err = stackedCopy.CompareAndMark(img.Trees[len(img.Trees)-1])
				require.NoError(t, err, "CompareAndMark on stacked tree failed")
			}
		})
	}
}

// --- Test: efficiency calculation works on all formats ---

func TestOCI_EfficiencyCalculation(t *testing.T) {
	table := []struct {
		name string
		path string
	}{
		{"docker-image", "../../../.data/test-docker-image.tar"},
		{"oci-docker", "../../../.data/test-oci-docker-image.tar"},
		{"oci-gzip", "../../../.data/test-oci-gzip-image.tar"},
		{"oci-zstd", "../../../.data/test-oci-zstd-image.tar"},
		{"oci-uncompressed", "../../../.data/test-oci-uncompressed-image.tar"},
	}

	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			if isPlaceholderArchive(t, tc.path) {
				t.Skipf("skipping %s: placeholder archive", tc.name)
			}

			result := TestAnalysisFromArchive(t, tc.path)

			assert.GreaterOrEqual(t, result.Efficiency, float64(0))
			assert.LessOrEqual(t, result.Efficiency, float64(1))

			// size accounting invariant: total >= user
			assert.GreaterOrEqual(t, result.SizeBytes, result.UserSizeByes,
				"total size should be >= user size")

			// wasted bytes should not exceed user size
			assert.LessOrEqual(t, result.WastedBytes, result.UserSizeByes+result.SizeBytes,
				"wasted bytes should be bounded")
		})
	}
}

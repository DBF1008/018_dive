package filetree

import "fmt"

// LayerChangeSummary holds aggregated change statistics for a single image layer.
type LayerChangeSummary struct {
	AddedCount    int   `json:"addedCount"`
	AddedBytes    int64 `json:"addedBytes"`
	ModifiedCount int   `json:"modifiedCount"`
	ModifiedBytes int64 `json:"modifiedBytes"`
	RemovedCount  int   `json:"removedCount"`
	RemovedBytes  int64 `json:"removedBytes"`
}

// Summarize computes per-layer change summaries by comparing each layer against
// all previous layers stacked together. This uses the same comparison logic
// (StackTreeRange + CompareAndMark) that the TUI's single-layer view uses,
// so the numbers are consistent. Only non-directory nodes are counted.
func Summarize(trees []*FileTree) ([]LayerChangeSummary, error) {
	if len(trees) == 0 {
		return nil, nil
	}

	summaries := make([]LayerChangeSummary, len(trees))

	// Layer 0: everything is new (base layer introduces all files from scratch)
	var s0 LayerChangeSummary
	err := trees[0].VisitDepthChildFirst(func(node *FileNode) error {
		if !node.Data.FileInfo.IsDir {
			s0.AddedCount++
			s0.AddedBytes += node.Data.FileInfo.Size
		}
		return nil
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to walk layer 0: %w", err)
	}
	summaries[0] = s0

	// Layers 1..N: compare against stacked previous layers
	for i := 1; i < len(trees); i++ {
		baseTree, _, err := StackTreeRange(trees, 0, i-1)
		if err != nil {
			return nil, fmt.Errorf("unable to stack tree range for layer %d: %w", i, err)
		}
		if _, err = baseTree.CompareAndMark(trees[i]); err != nil {
			return nil, fmt.Errorf("unable to compare layer %d: %w", i, err)
		}

		var summary LayerChangeSummary
		err = baseTree.VisitDepthChildFirst(func(node *FileNode) error {
			if node.Data.FileInfo.IsDir {
				return nil
			}
			switch node.Data.DiffType {
			case Added:
				summary.AddedCount++
				summary.AddedBytes += node.Data.FileInfo.Size
			case Modified:
				summary.ModifiedCount++
				summary.ModifiedBytes += node.Data.FileInfo.Size
			case Removed:
				summary.RemovedCount++
				summary.RemovedBytes += node.Data.FileInfo.Size
			}
			return nil
		}, nil)
		if err != nil {
			return nil, fmt.Errorf("unable to walk compared tree for layer %d: %w", i, err)
		}
		summaries[i] = summary
	}

	return summaries, nil
}

package export

import (
	"encoding/json"
	"github.com/wagoodman/dive/dive/filetree"
	diveImage "github.com/wagoodman/dive/dive/image"
	"github.com/wagoodman/dive/internal/log"
)

type Export struct {
	Layer []Layer `json:"layer"`
	Image Image   `json:"image"`
}

type Layer struct {
	Index     int                 `json:"index"`
	ID        string              `json:"id"`
	DigestID  string              `json:"digestId"`
	SizeBytes uint64              `json:"sizeBytes"`
	Command   string              `json:"command"`
	FileList  []filetree.FileInfo `json:"fileList"`
}

type Image struct {
	SizeBytes        uint64          `json:"sizeBytes"`
	InefficientBytes uint64          `json:"inefficientBytes"`
	EfficiencyScore  float64         `json:"efficiencyScore"`
	InefficientFiles []FileReference `json:"fileReference"`
}

type FileReference struct {
	References int    `json:"count"`
	SizeBytes  uint64 `json:"sizeBytes"`
	Path       string `json:"file"`
}

// NewExport exports the analysis to a JSON
func NewExport(analysis *diveImage.Analysis) *Export {
	data := Export{
		Layer: make([]Layer, len(analysis.Layers)),
		Image: Image{
			InefficientFiles: make([]FileReference, len(analysis.Inefficiencies)),
			SizeBytes:        analysis.SizeBytes,
			EfficiencyScore:  analysis.Efficiency,
			InefficientBytes: analysis.WastedBytes,
		},
	}

	// export layers in order
	for idx, curLayer := range analysis.Layers {
		layerFileList := make([]filetree.FileInfo, 0)
		visitor := func(node *filetree.FileNode) error {
			layerFileList = append(layerFileList, node.Data.FileInfo)
			return nil
		}
		err := curLayer.Tree.VisitDepthChildFirst(visitor, nil)
		if err != nil {
			log.WithFields("layer", curLayer.Id, "error", err).Debug("unable to propagate layer tree")
		}
		data.Layer[idx] = Layer{
			Index:     curLayer.Index,
			ID:        curLayer.Id,
			DigestID:  curLayer.Digest,
			SizeBytes: curLayer.Size,
			Command:   curLayer.Command,
			FileList:  layerFileList,
		}
	}

	// add file references
	for idx := 0; idx < len(analysis.Inefficiencies); idx++ {
		fileData := analysis.Inefficiencies[len(analysis.Inefficiencies)-1-idx]

		data.Image.InefficientFiles[idx] = FileReference{
			References: len(fileData.Nodes),
			SizeBytes:  uint64(fileData.CumulativeSize),
			Path:       fileData.Path,
		}
	}

	return &data
}

func (exp *Export) Marshal() ([]byte, error) {
	return json.MarshalIndent(&exp, "", "  ")
}

// SummaryExport is a compact alternative to Export that replaces per-layer file
// lists with aggregated change summaries (added/modified/removed counts and bytes).
type SummaryExport struct {
	Layer []SummaryLayer `json:"layer"`
	Image Image          `json:"image"`
}

// SummaryLayer contains layer metadata plus an aggregated change summary
// instead of a full file list.
type SummaryLayer struct {
	Index         int                      `json:"index"`
	ID            string                   `json:"id"`
	DigestID      string                   `json:"digestId"`
	SizeBytes     uint64                   `json:"sizeBytes"`
	Command       string                   `json:"command"`
	ChangeSummary filetree.LayerChangeSummary `json:"changeSummary"`
}

// NewSummaryExport creates a compact export with per-layer change summaries
// and the top-N most wasteful files. It reuses the same analysis data as
// NewExport and CI evaluation, so numbers are always consistent.
func NewSummaryExport(analysis *diveImage.Analysis, topN int) *SummaryExport {
	data := SummaryExport{
		Layer: make([]SummaryLayer, len(analysis.Layers)),
		Image: Image{
			SizeBytes:        analysis.SizeBytes,
			EfficiencyScore:  analysis.Efficiency,
			InefficientBytes: analysis.WastedBytes,
		},
	}

	// populate layer summaries
	for idx, curLayer := range analysis.Layers {
		var summary filetree.LayerChangeSummary
		if idx < len(analysis.LayerSummaries) {
			summary = analysis.LayerSummaries[idx]
		}
		data.Layer[idx] = SummaryLayer{
			Index:         curLayer.Index,
			ID:            curLayer.Id,
			DigestID:      curLayer.Digest,
			SizeBytes:     curLayer.Size,
			Command:       curLayer.Command,
			ChangeSummary: summary,
		}
	}

	// add top-N inefficient file references (sorted by cumulative size, descending)
	ineffCount := len(analysis.Inefficiencies)
	if topN > ineffCount {
		topN = ineffCount
	}
	data.Image.InefficientFiles = make([]FileReference, topN)
	for idx := 0; idx < topN; idx++ {
		fileData := analysis.Inefficiencies[ineffCount-1-idx]
		data.Image.InefficientFiles[idx] = FileReference{
			References: len(fileData.Nodes),
			SizeBytes:  uint64(fileData.CumulativeSize),
			Path:       fileData.Path,
		}
	}

	return &data
}

func (exp *SummaryExport) Marshal() ([]byte, error) {
	return json.MarshalIndent(&exp, "", "  ")
}

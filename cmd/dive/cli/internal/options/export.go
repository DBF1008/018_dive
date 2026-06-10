package options

import (
	"fmt"
	"os"
	"path"

	"github.com/anchore/clio"
)

var _ interface {
	clio.FlagAdder
	clio.PostLoader
} = (*Export)(nil)

// Export provides configuration for data export functionality
type Export struct {
	// Path to export analysis results as JSON (empty string = disabled)
	JsonPath string `yaml:"json-path" json:"json-path" mapstructure:"json-path"`
	// Export a compact summary instead of full file lists
	SummaryMode bool `yaml:"json-summary" json:"json-summary" mapstructure:"json-summary"`
	// Number of most wasteful files to include in summary export
	TopWasteful int `yaml:"top-wasteful" json:"top-wasteful" mapstructure:"top-wasteful"`
}

func DefaultExport() Export {
	return Export{
		TopWasteful: 10,
	}
}

func (o *Export) AddFlags(flags clio.FlagSet) {
	flags.StringVarP(&o.JsonPath, "json", "j", "Skip the interactive TUI and write the layer analysis statistics to a given file.")
	flags.BoolVarP(&o.SummaryMode, "json-summary", "", "Export a compact per-layer change summary instead of full file lists.")
	flags.IntVarP(&o.TopWasteful, "top-wasteful", "", "Number of most wasteful files to include in summary export (default 10).")
}

func (o *Export) PostLoad() error {

	if o.JsonPath != "" {
		dir := path.Dir(o.JsonPath)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return fmt.Errorf("directory for JSON export does not exist: %s", dir)
		}
	}

	if o.SummaryMode && o.JsonPath == "" {
		return fmt.Errorf("--json-summary requires --json to be set")
	}

	return nil
}

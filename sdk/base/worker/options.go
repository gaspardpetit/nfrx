package worker

import (
	"fmt"
	"strings"

	"github.com/gaspardpetit/nfrx/sdk/api/spi"
)

// ArgSpecs returns the base worker ArgSpecs tailored for the given plugin ID.
// These are intended to be appended to an extension's descriptor.
func ArgSpecs(pluginID string) []spi.ArgSpec {
	up := strings.ToUpper(pluginID)
	flagPrefix := "--" + pluginID + "-"
	yamlPrefix := "plugin_options." + pluginID + "."
	return []spi.ArgSpec{
		{
			ID:          "min_score",
			Flag:        flagPrefix + "min-score",
			Env:         fmt.Sprintf("%s_MIN_SCORE", up),
			YAML:        yamlPrefix + "min_score",
			Type:        spi.ArgNumber,
			Default:     "0.01",
			Example:     "1",
			Description: "Minimum compatibility score required to select a worker (0–1)",
		},
	}
}

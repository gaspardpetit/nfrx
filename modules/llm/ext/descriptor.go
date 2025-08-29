package llm

import (
	"github.com/gaspardpetit/nfrx/sdk/api/spi"
	baseworker "github.com/gaspardpetit/nfrx/sdk/base/worker"
)

// Descriptor returns the LLM plugin descriptor.
func Descriptor() spi.PluginDescriptor {
	d := spi.PluginDescriptor{
		ID:      "llm",
		Name:    "LLM Gateway",
		Summary: "OpenAI-compatible API over an agent pool",
		Args: []spi.ArgSpec{
			{
				ID:          "max_parallel_embeddings",
				Flag:        "--llm-max-parallel-embeddings",
				Env:         "LLM_MAX_PARALLEL_EMBEDDINGS",
				YAML:        "plugin_options.llm.max_parallel_embeddings",
				Type:        spi.ArgInt,
				Default:     "8",
				Example:     "16",
				Description: "Maximum agents to split embeddings across",
			},
		},
	}
	// Append base worker options (shared across worker-style plugins)
	d.Args = append(d.Args, baseworker.ArgSpecs(d.ID)...)
	return d
}

package docling

import "github.com/gaspardpetit/nfrx/sdk/api/spi"

func Descriptor() spi.PluginDescriptor {
	return spi.PluginDescriptor{
		ID:      "docling",
		Name:    "Docling",
		Summary: "Document conversion (worker-style)",
	}
}

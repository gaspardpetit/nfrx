package asr

import "github.com/gaspardpetit/nfrx/sdk/api/spi"

func Descriptor() spi.PluginDescriptor {
	return spi.PluginDescriptor{
		ID:      "asr",
		Name:    "ASR",
		Summary: "Audio transcription (worker-style)",
	}
}

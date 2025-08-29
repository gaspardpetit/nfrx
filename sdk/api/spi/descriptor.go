package spi

// ArgType represents the type of a configurable argument.
type ArgType string

const (
	ArgString   ArgType = "string"
	ArgInt      ArgType = "int"
	ArgBool     ArgType = "bool"
	ArgNumber   ArgType = "number"
	ArgDuration ArgType = "duration"
)

// ArgSpec describes a single configurable parameter for an extension.
type ArgSpec struct {
	ID          string  // stable identifier within the plugin options map
	Flag        string  // suggested command-line flag name (e.g. --llm-max-parallel-embeddings)
	Env         string  // suggested environment variable (e.g. LLM_MAX_PARALLEL_EMBEDDINGS)
	YAML        string  // suggested YAML path (e.g. plugin_options.llm.max_parallel_embeddings)
	Type        ArgType // type hint for formatting and validation
	Default     string  // human-readable default value
	Example     string  // optional example value
	Description string  // one-line description
	Deprecated  bool    // true if deprecated
	Replacement string  // optional replacement guidance
	Secret      bool    // true if value is secret and should be masked in UIs
}

// PluginDescriptor provides human-readable metadata for an extension and its options.
type PluginDescriptor struct {
	ID      string // plugin ID (e.g., "llm")
	Name    string // friendly name (e.g., "LLM Gateway")
	Summary string // short description
	Args    []ArgSpec
}

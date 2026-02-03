module github.com/gaspardpetit/nfrx

go 1.24.0

toolchain go1.24.3

require (
	github.com/alicebob/miniredis/v2 v2.36.1
	github.com/coder/websocket v1.8.14
	github.com/gaspardpetit/nfrx/sdk v0.0.0
	github.com/getkin/kin-openapi v0.133.0
	github.com/go-chi/chi/v5 v5.2.4
	github.com/go-chi/cors v1.2.2
	github.com/google/uuid v1.6.0
	github.com/mark3labs/mcp-go v0.43.2
	github.com/oapi-codegen/runtime v1.1.2
	github.com/prometheus/client_golang v1.23.2
	github.com/redis/go-redis/v9 v9.17.3
	github.com/rs/zerolog v1.34.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/gaspardpetit/nfrx/sdk => ./sdk

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/go-openapi/jsonpointer v0.22.4 // indirect
	github.com/go-openapi/swag/jsonname v0.25.4 // indirect
	github.com/invopop/jsonschema v0.13.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/mailru/easyjson v0.9.1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/oasdiff/yaml v0.0.0-20250309154309-f31be36b4037 // indirect
	github.com/oasdiff/yaml3 v0.0.0-20250309153720-d2182401db90 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/procfs v0.19.2 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	github.com/woodsbury/decimal128 v1.4.0 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	golang.org/x/sys v0.40.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

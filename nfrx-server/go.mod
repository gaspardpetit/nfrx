module github.com/gaspardpetit/nfrx-server

go 1.24.3

replace github.com/gaspardpetit/nfrx-sdk => ../nfrx-sdk

replace github.com/gaspardpetit/nfrx-plugins-mcp => ../nfrx-plugins-mcp

require (
	github.com/alicebob/miniredis/v2 v2.35.0
	github.com/coder/websocket v1.8.13
	github.com/gaspardpetit/nfrx v0.5.1
	github.com/gaspardpetit/nfrx-sdk v0.0.0-00010101000000-000000000000
	github.com/getkin/kin-openapi v0.133.0
	github.com/go-chi/chi/v5 v5.2.2
	github.com/go-chi/cors v1.2.2
	github.com/google/uuid v1.6.0
	github.com/oapi-codegen/runtime v1.1.2
	github.com/prometheus/client_golang v1.23.0
	github.com/redis/go-redis/v9 v9.12.1
	github.com/rs/zerolog v1.34.0
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/oasdiff/yaml v0.0.0-20250309154309-f31be36b4037 // indirect
	github.com/oasdiff/yaml3 v0.0.0-20250309153720-d2182401db90 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.65.0 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/woodsbury/decimal128 v1.3.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	golang.org/x/sys v0.33.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

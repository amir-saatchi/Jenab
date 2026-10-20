module github.com/amir-saatchi/jenab/spikes/semanticrecall

go 1.27.0

require (
	burrow/spikes/semanticrecall/builtin v0.0.0
	github.com/amir-saatchi/jenab v0.0.0
	go.yaml.in/yaml/v3 v3.0.5
	modernc.org/sqlite v1.59.0
)

require (
	codeberg.org/readeck/go-readability/v2 v2.1.2 // indirect
	github.com/JohannesKaufmann/dom v0.3.1 // indirect
	github.com/JohannesKaufmann/html-to-markdown/v2 v2.5.2 // indirect
	github.com/andybalholm/cascadia v1.3.5 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/eliben/go-sentencepiece v0.7.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-shiori/dom v0.0.0-20230515143342-73569d674e1c // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/gofrs/flock v0.13.1 // indirect
	github.com/gogs/chardet v0.0.0-20211120154057-b7413eaefb8f // indirect
	github.com/gomlx/compute v0.1.14 // indirect
	github.com/gomlx/compute-onnx v0.1.13 // indirect
	github.com/gomlx/exceptions v0.0.3 // indirect
	github.com/gomlx/go-huggingface v0.4.15 // indirect
	github.com/gomlx/gomlx v0.28.17 // indirect
	github.com/gomlx/onnx-gomlx v0.5.14 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/itlightning/dateparse v0.2.1 // indirect
	github.com/knights-analytics/hugot v0.8.3 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/oklog/ulid/v2 v2.1.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	github.com/zalando/go-keyring v0.2.8 // indirect
	golang.org/x/exp v0.0.0-20261013213113-2ab7f3cbc5fb // indirect
	golang.org/x/image v0.47.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	k8s.io/klog/v2 v2.140.0 // indirect
	modernc.org/libc v1.75.7 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

replace github.com/amir-saatchi/jenab => ../..

replace burrow/spikes/semanticrecall/builtin => ./builtin

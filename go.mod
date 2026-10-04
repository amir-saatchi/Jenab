module github.com/amir-saatchi/jenab

go 1.26.0

require (
	codeberg.org/readeck/go-readability/v2 v2.1.2
	github.com/JohannesKaufmann/html-to-markdown/v2 v2.5.2
	github.com/anthropics/anthropic-sdk-go v1.75.0
	github.com/google/go-cmp v0.7.0
	github.com/oklog/ulid/v2 v2.1.2
	github.com/openai/openai-go/v3 v3.66.0
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3
	github.com/wailsapp/wails/v3 v3.0.0-beta.26
	github.com/zalando/go-keyring v0.2.8
	go.yaml.in/yaml/v3 v3.0.5
	golang.org/x/net v0.59.0
	golang.org/x/text v0.42.0
	modernc.org/sqlite v1.59.0
)

require (
	github.com/JohannesKaufmann/dom v0.3.1 // indirect
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/andybalholm/cascadia v1.3.5 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/coder/websocket v1.8.15 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-shiori/dom v0.0.0-20230515143342-73569d674e1c // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/gogs/chardet v0.0.0-20211120154057-b7413eaefb8f // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/invopop/jsonschema v0.14.0 // indirect
	github.com/itlightning/dateparse v0.2.1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/standard-webhooks/standard-webhooks/libraries v0.0.1 // indirect
	github.com/tidwall/gjson v1.19.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	go.yaml.in/yaml/v4 v4.0.0-rc.2 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	modernc.org/libc v1.75.7 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

// ./... skips the spikes (each is its own module, kept as evidence)
// and Go files that npm packages ship in node_modules.
ignore (
	./spikes
	node_modules
)

module github.com/amir-saatchi/jenab

go 1.26.0

require (
	github.com/google/go-cmp v0.7.0
	github.com/oklog/ulid/v2 v2.1.2
	github.com/wailsapp/wails/v3 v3.0.0-beta.26
	github.com/zalando/go-keyring v0.2.8
	go.yaml.in/yaml/v3 v3.0.5
)

require (
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/coder/websocket v1.8.14 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	golang.org/x/sys v0.46.0 // indirect
)

// ./... skips the spikes (each is its own module, kept as evidence)
// and Go files that npm packages ship in node_modules.
ignore (
	./spikes
	node_modules
)

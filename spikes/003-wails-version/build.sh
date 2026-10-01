#!/bin/sh
# Builds both test apps. No Wails CLI and no npm needed: the page is plain HTML.
set -e
cd "$(dirname "$0")"
# v2app-212: the same v2 app pinned to v2.12.0, the version Nord-Agent uses
rm -rf v2app-212 && mkdir v2app-212 && cp v2app/main.go v2app/trayicon.go v2app/go.mod v2app/go.sum v2app-212/
(cd v2app-212 && go get github.com/wailsapp/wails/v2@v2.12.0 >/dev/null 2>&1)
for d in v2app v2app-212 v3app; do
  cp common/probe.go "$d/probe.go"
  rm -rf "$d/frontend" && cp -r common/frontend "$d/frontend"
done
(cd v2app && go mod tidy && go build -tags desktop,production -ldflags "-H=windowsgui" -o ../app-v2.exe .)
(cd v2app-212 && go mod tidy && go build -tags desktop,production -ldflags "-H=windowsgui" -o ../app-v2.12.exe .)
(cd v3app && go mod tidy && go build -tags production -ldflags "-H=windowsgui" -o ../app-v3.exe .)
# v3app-a104: the v3 app on v3.0.0-alpha2.104 (18 June 2026), the alpha Nord-Agent most likely used
rm -rf v3app-a104 && mkdir v3app-a104 && cp v3app/main.go v3app/probe.go v3app/go.mod v3app/go.sum v3app-a104/ && cp -r v3app/frontend v3app-a104/
(cd v3app-a104 && go get github.com/wailsapp/wails/v3@v3.0.0-alpha2.104 >/dev/null 2>&1 && go mod tidy && go build -tags production -ldflags "-H=windowsgui" -o ../app-v3a104.exe .) || echo "alpha2.104 build failed"
ls -la app-v2.exe app-v2.12.exe app-v3.exe app-v3a104.exe 2>&1

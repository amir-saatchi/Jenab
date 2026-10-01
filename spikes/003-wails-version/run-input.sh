#!/bin/sh
# Runs the full self-test (paint and input after every restore) on five setups
# and prints a summary. About 30 minutes; don't use the machine meanwhile.
cd "$(dirname "$0")"
rm -f fail-*.png
run() { # name exe args...
  name=$1; exe=$2; shift 2
  timeout 600 "./$exe" "$@" "r-$name.json" 2>/dev/null
  mkdir -p "fails-$name" && mv fail-*.png "fails-$name/" 2>/dev/null
  python -c "
import json;r=json.load(open('r-$name.json'));print('##', r['App'], r['TotalTime'])
[print(' ',k,v) for k,v in sorted(r['Painted'].items())]
[print('  FAIL',x) for x in (r['Failures'] or [])]"
}
run a104-frameless app-v3a104.exe --frameless
run a104-framed    app-v3a104.exe
run b26-frameless  app-v3.exe --frameless
run b26-framed     app-v3.exe
run v212-frameless app-v2.12.exe --frameless --alpha1

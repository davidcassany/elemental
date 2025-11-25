#!/bin/bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1

{{ range .Scripts -}}
echo "Running {{ . }}"
./{{ . }}

{{ end -}}

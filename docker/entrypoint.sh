#!/bin/sh
set -e

# First-run: neither config nor workspace exists.
# If config.json is already mounted but workspace is missing we skip onboard to
# avoid the interactive "Overwrite? (y/n)" prompt hanging in a non-TTY container.
if [ ! -d "${HOME}/.aetherclaw/workspace" ] && [ ! -f "${HOME}/.aetherclaw/config.json" ]; then
    aetherclaw onboard
    echo ""
    echo "First-run setup complete."
    echo "Edit ${HOME}/.aetherclaw/config.json (add your API key, etc.) then restart the container."
    exit 0
fi

exec aetherclaw gateway "$@"

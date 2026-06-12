#!/usr/bin/env bash
# scripts/init-angular.sh
# Run once to bootstrap the Angular frontend in web/
set -euo pipefail

cd "$(dirname "$0")/.."

if ! command -v ng &>/dev/null; then
  echo "Angular CLI not found. Installing globally..."
  npm install -g @angular/cli
fi

echo "Creating Angular app in web/..."
ng new prism-web \
  --directory web \
  --routing \
  --style scss \
  --skip-git \
  --standalone

echo "Installing dash.js and other dependencies..."
cd web
npm install dashjs
npm install --save-dev @types/dashjs

echo ""
echo "Angular frontend scaffolded in web/"
echo "Run 'cd web && ng serve' to start the dev server."

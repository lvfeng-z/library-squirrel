// This file is intentionally empty.
// It prevents Vite's SPA fallback from serving index.html for /wails/custom.js,
// which would cause "Unexpected token '<'" errors in the browser.
// The @wailsio/runtime package's loadOptionalScript() expects this URL to either
// return 404 (script not loaded) or valid JS (loaded). Without this file,
// Vite returns index.html with status 200, causing a JS parse error.

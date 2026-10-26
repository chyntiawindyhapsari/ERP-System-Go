package web

import _ "embed"

// IndexHTML adalah halaman frontend yang disematkan ke dalam binary Go,
// jadi tidak perlu file terpisah saat deploy.
//
//go:embed index.html
var IndexHTML []byte

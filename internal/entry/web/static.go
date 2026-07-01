package web

import "embed"

// staticFS nhúng tài nguyên frontend, đảm bảo single binary không phụ thuộc vào file bên ngoài.
//
//go:embed static
var staticFS embed.FS

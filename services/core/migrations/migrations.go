// Package migrations embeds the SQL migrations so the migrate command carries them
// and production needs nothing but the image.
package migrations

import "embed"

// FS holds every *.sql migration, applied in version order.
//
//go:embed *.sql
var FS embed.FS

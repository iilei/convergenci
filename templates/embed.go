// Package templates embeds convergenci's built-in report templates so they ship
// inside the binary without requiring a separate template file or gomplate.
package templates

import _ "embed"

//go:embed report-as-text.tmpl
var ReportAsText string

package assets

import _ "embed"

// IconPNG is the application icon, embedded so the running window can show it in
// the taskbar without reading a file beside the executable. The same file is
// what viva package puts in the bundle, installer and launcher entry.
//
//go:embed icon.png
var IconPNG []byte

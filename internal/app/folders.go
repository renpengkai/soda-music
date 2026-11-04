package app

import (
	"path/filepath"
	"strings"
)

// addFolder remembers a music folder and rescans the library. It is called from
// the folder picker, which runs after a paint because the dialog is modal.
func (m *Model) addFolder(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	for _, existing := range m.state.Folders {
		if strings.EqualFold(existing, path) {
			m.toastf("这个文件夹已经在曲库里了")
			return
		}
	}
	m.state.Folders = append(m.state.Folders, path)
	m.touch()
	m.rescan()
	m.toastf("正在扫描 %s", filepath.Base(path))
}

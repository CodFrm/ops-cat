package app

import "github.com/opskat/opskat/internal/service/font_svc"

// ListSystemFonts returns installed font family names for settings font picker.
func (a *App) ListSystemFonts() ([]string, error) {
	return font_svc.ListFamilies(a.ctx)
}

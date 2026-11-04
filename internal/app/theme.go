package app

import "github.com/vivaui/viva/ui"

// The brand is the mint green 汽水音乐 is known for. Everything else is a
// near-neutral surface tinted a few degrees towards it, so the accent reads as
// the product colour and not as a decoration.
const (
	brandMint  = 0x00E0B8
	brandDeep  = 0x00B894
	brandNight = 0x0A100F
)

// themeFor returns the theme for a mode of "system", "light" or "dark", with
// dark deciding what "system" means.
func themeFor(mode string, systemDark bool) ui.Theme {
	dark := false
	switch mode {
	case "dark":
		dark = true
	case "light":
		dark = false
	default:
		dark = systemDark
	}
	if dark {
		return darkTheme()
	}
	return lightTheme()
}

func darkTheme() ui.Theme {
	theme := ui.DarkTheme()
	theme.Background = ui.RGB24(brandNight)
	theme.Foreground = ui.RGB24(0xE9F5F2)
	theme.Card = ui.RGB24(0x111A18)
	theme.CardForeground = ui.RGB24(0xE9F5F2)
	theme.CardElevated = ui.RGB24(0x14201D)
	theme.Popover = ui.RGB24(0x14201D)
	theme.PopoverForeground = ui.RGB24(0xE9F5F2)
	theme.Muted = ui.RGB24(0x1B2724)
	theme.MutedForeground = ui.RGB24(0x8AA49E)
	theme.Border = ui.RGBA(255, 255, 255, 0.09)
	theme.Hover = ui.RGB24(0x1A2623)
	theme.Input = ui.RGBA(255, 255, 255, 0.13)
	theme.Primary = ui.RGB24(brandMint)
	theme.PrimaryForeground = ui.RGB24(0x04241E)
	theme.Selection = ui.RGB24(0x14564A)
	theme.SelectionForeground = ui.RGB24(0xE9F5F2)
	theme.Secondary = ui.RGB24(0x1E2C29)
	theme.SecondaryForeground = ui.RGB24(0xE9F5F2)
	theme.Accent = ui.RGB24(0x1B2B27)
	theme.AccentForeground = ui.RGB24(brandMint)
	theme.Ring = ui.RGB24(brandMint)
	theme.Success = ui.RGB24(0x2ED573)
	theme.Danger = ui.RGB24(0xFF6B6B)
	theme.DangerForeground = ui.RGB24(0x2A0B0B)
	return theme
}

func lightTheme() ui.Theme {
	theme := ui.LightTheme()
	theme.Background = ui.RGB24(0xF4FAF8)
	theme.Foreground = ui.RGB24(0x0B1A17)
	theme.Card = ui.RGB24(0xFFFFFF)
	theme.CardForeground = ui.RGB24(0x0B1A17)
	theme.CardElevated = ui.RGB24(0xEAF3F0)
	theme.Popover = ui.RGB24(0xFFFFFF)
	theme.PopoverForeground = ui.RGB24(0x0B1A17)
	theme.Muted = ui.RGB24(0xE6F0ED)
	theme.MutedForeground = ui.RGB24(0x5C736D)
	theme.Border = ui.RGB24(0xD9E7E3)
	theme.Hover = ui.RGB24(0xE9F3F0)
	theme.Input = ui.RGB24(0xD9E7E3)
	theme.Primary = ui.RGB24(brandDeep)
	theme.PrimaryForeground = ui.RGB24(0xFFFFFF)
	theme.Selection = ui.RGB24(0xB6EFE2)
	theme.SelectionForeground = ui.RGB24(0x04241E)
	theme.Secondary = ui.RGB24(0xE6F0ED)
	theme.SecondaryForeground = ui.RGB24(0x0B1A17)
	theme.Accent = ui.RGB24(0xDDF3EE)
	theme.AccentForeground = ui.RGB24(0x006B55)
	theme.Ring = ui.RGB24(brandDeep)
	theme.Success = ui.RGB24(0x12A150)
	theme.Danger = ui.RGB24(0xD93A3A)
	theme.DangerForeground = ui.RGB24(0xFFFFFF)
	return theme
}

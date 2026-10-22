package gallery

import "strings"

func notesForBackofficeSave(style *SetStyle, notes string) string {
	if style == nil {
		return notes
	}
	var ribbons []string
	if style.Wishlist {
		ribbons = append(ribbons, "Wish list")
	}
	if style.NoTrade {
		ribbons = append(ribbons, "No trade")
	}
	if style.Selective {
		ribbons = append(ribbons, "Selective")
	}
	if len(ribbons) > 0 {
		return strings.Join(ribbons, "; ")
	}
	// Earlier BO saves wrote the display-only New ribbon into REMARK.
	// Clear that legacy value when an existing set is saved again.
	parts := strings.Split(notes, ";")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch part {
		case "New":
		case "Wish list", "No trade", "Selective":
			kept = append(kept, part)
		default:
			return notes
		}
	}
	return strings.Join(kept, "; ")
}

package kit

// RightToLeft reports whether s reads right to left: whether its first
// strong character is Hebrew, Arabic or Persian, as dir="auto" finds it
// and as the frontend's textDir (lib/dir.ts) does. Arabic and Persian
// digits are weak, like Latin ones.
func RightToLeft(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x0660 && r <= 0x0669, r >= 0x06f0 && r <= 0x06f9:
			continue
		case r >= 0x0590 && r <= 0x08ff, r >= 0xfb1d && r <= 0xfdff, r >= 0xfe70 && r <= 0xfeff:
			return true
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= 0x00c0 && r <= 0x024f,
			r >= 0x0370 && r <= 0x03ff, r >= 0x0400 && r <= 0x04ff:
			return false
		}
	}
	return false
}

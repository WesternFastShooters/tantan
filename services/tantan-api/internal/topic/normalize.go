package topic

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

func NormalizeName(value string) string {
	value = compatibilityString(value)
	var builder strings.Builder
	spacePending := false
	for _, character := range strings.TrimSpace(value) {
		character = compatibilityRune(character)
		if unicode.IsSpace(character) {
			if builder.Len() > 0 {
				spacePending = true
			}
			continue
		}
		if spacePending {
			builder.WriteByte(' ')
			spacePending = false
		}
		builder.WriteRune(unicode.ToLower(character))
	}
	return builder.String()
}

func displayName(value string) string {
	value = compatibilityString(value)
	var builder strings.Builder
	spacePending := false
	for _, character := range strings.TrimSpace(value) {
		character = compatibilityRune(character)
		if unicode.IsSpace(character) {
			if builder.Len() > 0 {
				spacePending = true
			}
			continue
		}
		if spacePending {
			builder.WriteByte(' ')
			spacePending = false
		}
		builder.WriteRune(character)
	}
	return builder.String()
}

func compatibilityString(value string) string {
	return strings.NewReplacer(
		"\ufb00", "ff",
		"\ufb01", "fi",
		"\ufb02", "fl",
		"\ufb03", "ffi",
		"\ufb04", "ffl",
		"\ufb05", "st",
		"\ufb06", "st",
	).Replace(value)
}

func compatibilityRune(character rune) rune {
	if character == '\u3000' {
		return ' '
	}
	if character >= '\uff01' && character <= '\uff5e' {
		return character - 0xfee0
	}
	switch character {
	case '\u212a':
		return 'K'
	case '\u212b':
		return '\u00c5'
	default:
		return character
	}
}

func CoreID(userID, slug string) string {
	digest := sha256.Sum256([]byte(userID))
	return "topic_" + slug + "_" + hex.EncodeToString(digest[:])[:10]
}

func generatedID(userID, kind, normalizedName string) string {
	digest := sha256.Sum256([]byte(userID + "\x00" + kind + "\x00" + normalizedName))
	return "topic_" + kind + "_" + hex.EncodeToString(digest[:])[:16]
}

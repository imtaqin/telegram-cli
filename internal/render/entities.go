package render

import (
	"fmt"
	"strings"

	"github.com/imtaqin/telegram-cli/internal/telegram"
)

// EntitiesToMarkdown converts Telegram formatted text entities to markdown.
func EntitiesToMarkdown(text *telegram.FormattedText) string {
	if text == nil {
		return ""
	}

	if len(text.Entities) == 0 {
		return text.Text
	}

	runes := []rune(text.Text)

	var b strings.Builder
	lastEnd := 0

	for _, span := range entitySpans(runes, text.Entities) {
		// This flat renderer retains the enclosing formatting for overlaps.
		if span.start < lastEnd {
			continue
		}
		// Append text before this entity.
		if span.start > lastEnd {
			b.WriteString(string(runes[lastEnd:span.start]))
		}

		entityText := string(runes[span.start:span.end])

		switch t := span.entity.Type.(type) {
		case *telegram.TextEntityTypeBold:
			b.WriteString("**")
			b.WriteString(entityText)
			b.WriteString("**")

		case *telegram.TextEntityTypeItalic:
			b.WriteString("*")
			b.WriteString(entityText)
			b.WriteString("*")

		case *telegram.TextEntityTypeUnderline:
			b.WriteString("__")
			b.WriteString(entityText)
			b.WriteString("__")

		case *telegram.TextEntityTypeStrikethrough:
			b.WriteString("~~")
			b.WriteString(entityText)
			b.WriteString("~~")

		case *telegram.TextEntityTypeCode:
			b.WriteString("`")
			b.WriteString(entityText)
			b.WriteString("`")

		case *telegram.TextEntityTypePre:
			b.WriteString("```\n")
			b.WriteString(entityText)
			b.WriteString("\n```")

		case *telegram.TextEntityTypePreCode:
			b.WriteString(fmt.Sprintf("```%s\n", t.Language))
			b.WriteString(entityText)
			b.WriteString("\n```")

		case *telegram.TextEntityTypeTextURL:
			b.WriteString(fmt.Sprintf("[%s](%s)", entityText, t.URL))

		case *telegram.TextEntityTypeURL:
			b.WriteString(entityText)

		case *telegram.TextEntityTypeMention:
			b.WriteString(entityText)

		case *telegram.TextEntityTypeMentionName:
			b.WriteString(fmt.Sprintf("@[%s](user:%d)", entityText, t.UserID))

		case *telegram.TextEntityTypeHashtag:
			b.WriteString(entityText)

		case *telegram.TextEntityTypeBotCommand:
			b.WriteString("`")
			b.WriteString(entityText)
			b.WriteString("`")

		case *telegram.TextEntityTypeEmailAddress:
			b.WriteString(entityText)

		case *telegram.TextEntityTypeSpoiler:
			b.WriteString("||")
			b.WriteString(entityText)
			b.WriteString("||")

		case *telegram.TextEntityTypeBlockQuote:
			lines := strings.Split(entityText, "\n")
			for i, line := range lines {
				b.WriteString("> ")
				b.WriteString(line)
				if i < len(lines)-1 {
					b.WriteString("\n")
				}
			}

		default:
			b.WriteString(entityText)
		}

		lastEnd = span.end
	}

	// Append remaining text.
	if lastEnd < len(runes) {
		b.WriteString(string(runes[lastEnd:]))
	}

	return b.String()
}

// EntitiesToANSI converts Telegram formatted text directly to ANSI escape codes.
func EntitiesToANSI(text *telegram.FormattedText) string {
	if text == nil {
		return ""
	}

	if len(text.Entities) == 0 {
		return text.Text
	}

	runes := []rune(text.Text)

	var b strings.Builder
	lastEnd := 0

	for _, span := range entitySpans(runes, text.Entities) {
		if span.start < lastEnd {
			continue
		}
		if span.start > lastEnd {
			b.WriteString(string(runes[lastEnd:span.start]))
		}

		entityText := string(runes[span.start:span.end])

		switch span.entity.Type.(type) {
		case *telegram.TextEntityTypeBold:
			b.WriteString("\033[1m")
			b.WriteString(entityText)
			b.WriteString("\033[22m")
		case *telegram.TextEntityTypeItalic:
			b.WriteString("\033[3m")
			b.WriteString(entityText)
			b.WriteString("\033[23m")
		case *telegram.TextEntityTypeUnderline:
			b.WriteString("\033[4m")
			b.WriteString(entityText)
			b.WriteString("\033[24m")
		case *telegram.TextEntityTypeStrikethrough:
			b.WriteString("\033[9m")
			b.WriteString(entityText)
			b.WriteString("\033[29m")
		case *telegram.TextEntityTypeCode, *telegram.TextEntityTypePre:
			b.WriteString("\033[7m") // inverse
			b.WriteString(entityText)
			b.WriteString("\033[27m")
		default:
			b.WriteString(entityText)
		}

		lastEnd = span.end
	}

	if lastEnd < len(runes) {
		b.WriteString(string(runes[lastEnd:]))
	}

	return b.String()
}

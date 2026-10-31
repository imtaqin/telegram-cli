package render

import (
	"sort"

	"github.com/imtaqin/telegram-cli/internal/telegram"
)

type entitySpan struct {
	entity     *telegram.TextEntity
	start, end int
}

// entitySpans converts Telegram's UTF-16 offsets to rune indexes without
// modifying the original entities. Invalid ranges are left as plain text.
func entitySpans(runes []rune, entities []*telegram.TextEntity) []entitySpan {
	utf16ToRune := make([]int, 1, len(runes)+1)
	for i, r := range runes {
		if r > 0xFFFF {
			// An entity boundary cannot split a UTF-16 surrogate pair.
			utf16ToRune = append(utf16ToRune, -1)
		}
		utf16ToRune = append(utf16ToRune, i+1)
	}

	spans := make([]entitySpan, 0, len(entities))
	for _, entity := range entities {
		if entity == nil {
			continue
		}
		// Widen before adding: malformed int32 offsets can otherwise overflow.
		start, length := int64(entity.Offset), int64(entity.Length)
		end := start + length
		if start < 0 || length <= 0 || end >= int64(len(utf16ToRune)) {
			continue
		}
		startRune, endRune := utf16ToRune[start], utf16ToRune[end]
		if startRune < 0 || endRune < 0 {
			continue
		}
		spans = append(spans, entitySpan{
			entity: entity,
			start:  startRune,
			end:    endRune,
		})
	}

	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		// Prefer the enclosing entity when the flat renderer sees nesting.
		return spans[i].end > spans[j].end
	})
	return spans
}

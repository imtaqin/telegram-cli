package render

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/imtaqin/telegram-cli/internal/telegram"
)

func TestEntityRenderersUTF16(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		entities []*telegram.TextEntity
		markdown string
		ansi     string
	}{
		{
			name: "ASCII",
			text: "hello world",
			entities: []*telegram.TextEntity{
				{Offset: 6, Length: 5, Type: &telegram.TextEntityTypeBold{}},
			},
			markdown: "hello **world**",
			ansi:     "hello \033[1mworld\033[22m",
		},
		{
			name: "emoji_before_formatting",
			text: "😀你好世界",
			entities: []*telegram.TextEntity{
				{Offset: 4, Length: 2, Type: &telegram.TextEntityTypeBold{}},
			},
			markdown: "😀你好**世界**",
			ansi:     "😀你好\033[1m世界\033[22m",
		},
		{
			name: "many_emoji_before_formatting",
			text: strings.Repeat("😀", 40) + "end",
			entities: []*telegram.TextEntity{
				{Offset: 80, Length: 3, Type: &telegram.TextEntityTypeBold{}},
			},
			markdown: strings.Repeat("😀", 40) + "**end**",
			ansi:     strings.Repeat("😀", 40) + "\033[1mend\033[22m",
		},
		{
			name: "formatted_emoji",
			text: "A😀B",
			entities: []*telegram.TextEntity{
				{Offset: 1, Length: 2, Type: &telegram.TextEntityTypeBold{}},
			},
			markdown: "A**😀**B",
			ansi:     "A\033[1m😀\033[22mB",
		},
		{
			name: "supplementary_CJK",
			text: "𠀀汉字",
			entities: []*telegram.TextEntity{
				{Offset: 2, Length: 2, Type: &telegram.TextEntityTypeItalic{}},
			},
			markdown: "𠀀*汉字*",
			ansi:     "𠀀\033[3m汉字\033[23m",
		},
		{
			name: "link_after_emoji",
			text: "🚀详情",
			entities: []*telegram.TextEntity{
				{Offset: 2, Length: 2, Type: &telegram.TextEntityTypeTextURL{URL: "https://example.com"}},
			},
			markdown: "🚀[详情](https://example.com)",
			ansi:     "🚀详情",
		},
		{
			name: "unsorted_entities",
			text: "😀abc",
			entities: []*telegram.TextEntity{
				{Offset: 4, Length: 1, Type: &telegram.TextEntityTypeItalic{}},
				{Offset: 2, Length: 1, Type: &telegram.TextEntityTypeBold{}},
			},
			markdown: "😀**a**b*c*",
			ansi:     "😀\033[1ma\033[22mb\033[3mc\033[23m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := &telegram.FormattedText{Text: tt.text, Entities: tt.entities}
			t.Run("markdown", func(t *testing.T) {
				if got := EntitiesToMarkdown(text); got != tt.markdown {
					t.Fatalf("got %q, want %q", got, tt.markdown)
				}
			})
			t.Run("ANSI", func(t *testing.T) {
				if got := EntitiesToANSI(text); got != tt.ansi {
					t.Fatalf("got %q, want %q", got, tt.ansi)
				}
			})
		})
	}
}

func TestEntityRenderersInvalidRanges(t *testing.T) {
	tests := []struct {
		name   string
		entity *telegram.TextEntity
	}{
		{name: "nil_entity"},
		{name: "negative_offset", entity: &telegram.TextEntity{Offset: -1, Length: 1}},
		{name: "negative_length", entity: &telegram.TextEntity{Offset: 2, Length: -1}},
		{name: "zero_length", entity: &telegram.TextEntity{Offset: 2}},
		{name: "offset_past_end", entity: &telegram.TextEntity{Offset: 20, Length: 1}},
		{name: "end_past_end", entity: &telegram.TextEntity{Offset: 3, Length: 20}},
		{name: "int32_overflow", entity: &telegram.TextEntity{Offset: 2, Length: 2147483647}},
		{name: "split_surrogate_start", entity: &telegram.TextEntity{Offset: 1, Length: 1}},
		{name: "split_surrogate_end", entity: &telegram.TextEntity{Offset: 0, Length: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.entity != nil {
				tt.entity.Type = &telegram.TextEntityTypeBold{}
			}
			text := &telegram.FormattedText{
				Text: "😀abc",
				Entities: []*telegram.TextEntity{
					tt.entity,
					{Offset: 3, Length: 1, Type: &telegram.TextEntityTypeBold{}},
				},
			}
			if got, want := EntitiesToMarkdown(text), "😀a**b**c"; got != want {
				t.Errorf("markdown: got %q, want %q", got, want)
			}
			if got, want := EntitiesToANSI(text), "😀a\033[1mb\033[22mc"; got != want {
				t.Errorf("ANSI: got %q, want %q", got, want)
			}
		})
	}
}

func TestEntityRenderersOverlappingRanges(t *testing.T) {
	text := &telegram.FormattedText{
		Text: "😀abc!",
		Entities: []*telegram.TextEntity{
			{Offset: 2, Length: 1, Type: &telegram.TextEntityTypeItalic{}},
			{Offset: 2, Length: 3, Type: &telegram.TextEntityTypeBold{}},
			{Offset: 3, Length: 3, Type: &telegram.TextEntityTypeItalic{}},
		},
	}
	// The flat renderer keeps the outer formatting and emits overlapping text once.
	if got, want := EntitiesToMarkdown(text), "😀**abc**!"; got != want {
		t.Errorf("markdown: got %q, want %q", got, want)
	}
	if got, want := EntitiesToANSI(text), "😀\033[1mabc\033[22m!"; got != want {
		t.Errorf("ANSI: got %q, want %q", got, want)
	}
}

func FuzzEntityRenderersPreserveText(f *testing.F) {
	f.Add("😀你好世界", int32(4), int32(2), int32(2), int32(1))
	f.Add("😀abc", int32(2), int32(2147483647), int32(-1), int32(1))
	f.Add("", int32(0), int32(0), int32(0), int32(1))
	f.Fuzz(func(t *testing.T, value string, offset, length, otherOffset, otherLength int32) {
		if !utf8.ValidString(value) {
			t.Skip()
		}
		// Unknown entity types should preserve the body regardless of ranges.
		text := &telegram.FormattedText{
			Text: value,
			Entities: []*telegram.TextEntity{
				{Offset: offset, Length: length},
				nil,
				{Offset: otherOffset, Length: otherLength},
			},
		}
		if got := EntitiesToMarkdown(text); got != value {
			t.Fatalf("markdown changed body: got %q, want %q", got, value)
		}
		if got := EntitiesToANSI(text); got != value {
			t.Fatalf("ANSI changed body: got %q, want %q", got, value)
		}
	})
}

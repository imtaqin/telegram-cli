package tgjson

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/imtaqin/telegram-cli/internal/telegram"
)

func TestMessagePreviewPreservesUnicode(t *testing.T) {
	tests := []struct {
		name, text, want string
	}{
		{"ASCII", "hello", "hello"},
		{"short_Chinese", strings.Repeat("中", 30), strings.Repeat("中", 30)},
		{"short_emoji", strings.Repeat("😀", 21), strings.Repeat("😀", 21)},
		{"long_ASCII", strings.Repeat("a", 81), strings.Repeat("a", 80) + "..."},
		{"long_Chinese", strings.Repeat("中", 81), strings.Repeat("中", 80) + "..."},
		{"emoji_at_boundary", strings.Repeat("a", 79) + "😀x", strings.Repeat("a", 79) + "😀..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := &telegram.Message{Content: &telegram.MessageText{
				Text: &telegram.FormattedText{Text: tt.text},
			}}
			got := MessagePreview(message)
			if !utf8.ValidString(got) {
				t.Fatalf("preview contains an incomplete UTF-8 character: %q", got)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMessageJSONPreservesFormattedUnicode(t *testing.T) {
	text := strings.Repeat("😀", 40) + "中文链接"
	message := &telegram.Message{
		ID: 123, ChatID: -1001234567890,
		Content: &telegram.MessageText{Text: &telegram.FormattedText{
			Text: text,
			Entities: []*telegram.TextEntity{
				{Offset: 80, Length: 4, Type: &telegram.TextEntityTypeBold{}},
				{Offset: 81, Length: 2147483647, Type: &telegram.TextEntityTypeTextURL{URL: "https://example.com"}},
			},
		}},
	}
	encoded, err := json.Marshal(ToMessageInfo(message))
	if err != nil {
		t.Fatal(err)
	}
	var decoded MessageInfo
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Text != text || decoded.Type != "text" || decoded.ID != message.ID {
		t.Fatalf("message changed after JSON conversion: %+v", decoded)
	}
}

func TestServiceMessageJSON(t *testing.T) {
	for _, content := range []telegram.MessageContent{
		&telegram.MessagePinMessage{},
		&telegram.MessageChatAddMembers{},
		&telegram.MessageChatDeleteMember{},
		&telegram.MessageChatChangeTitle{},
		&telegram.MessageChatChangePhoto{},
		&telegram.MessageChatJoinByLink{},
	} {
		message := &telegram.Message{ID: 123, Content: content}
		info := ToMessageInfo(message)
		if info.Type != "service" || info.Text != "" {
			t.Fatalf("unexpected service message: %+v", info)
		}
		if got := MessagePreview(message); got != "[service]" {
			t.Fatalf("unexpected service preview: %q", got)
		}
	}
}

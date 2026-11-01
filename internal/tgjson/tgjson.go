// Package tgjson holds the flat JSON DTOs shared by the MCP server and
// the REST API, plus converters from the telegram domain types.
package tgjson

import (
	"fmt"

	"github.com/imtaqin/telegram-cli/internal/telegram"
)

// ChatInfo is the flat chat representation returned to clients.
type ChatInfo struct {
	ID          int64  `json:"id" jsonschema:"canonical chat ID (user ID, negative group ID, or -100channelID)"`
	Type        string `json:"type" jsonschema:"private, group, supergroup or channel"`
	Title       string `json:"title"`
	Username    string `json:"username,omitempty"`
	UnreadCount int32  `json:"unread_count"`
	Pinned      bool   `json:"pinned"`
	LastMessage string `json:"last_message,omitempty" jsonschema:"preview of the last message"`
}

// MessageInfo is the flat message representation returned to clients.
type MessageInfo struct {
	ID               int64  `json:"id"`
	ChatID           int64  `json:"chat_id"`
	Date             int32  `json:"date" jsonschema:"unix timestamp"`
	SenderUserID     int64  `json:"sender_user_id,omitempty"`
	SenderChatID     int64  `json:"sender_chat_id,omitempty"`
	IsOutgoing       bool   `json:"is_outgoing"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
	Type             string `json:"type" jsonschema:"text, photo, video, document, voice, video_note, sticker, animation, audio, location, contact, poll, service or unsupported"`
	Text             string `json:"text,omitempty" jsonschema:"message text or media caption"`
	MediaFileKey     string `json:"media_file_key,omitempty" jsonschema:"file registry key, usable with download_media"`
	MediaFileName    string `json:"media_file_name,omitempty"`
	MediaSize        int64  `json:"media_size,omitempty"`
}

// ContactInfo is the flat user representation returned to clients.
type ContactInfo struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
	Phone     string `json:"phone,omitempty"`
	IsBot     bool   `json:"is_bot"`
}

// ToChatInfo converts a domain chat to its JSON DTO.
func ToChatInfo(chat *telegram.Chat) ChatInfo {
	info := ChatInfo{
		ID:          chat.ID,
		Type:        chatTypeString(chat.Type),
		Title:       chat.Title,
		Username:    chat.Username,
		UnreadCount: chat.UnreadCount,
		Pinned:      chat.Pinned,
	}
	if chat.LastMessage != nil {
		info.LastMessage = MessagePreview(chat.LastMessage)
	}
	return info
}

func chatTypeString(t telegram.ChatType) string {
	switch t {
	case telegram.ChatTypePrivate:
		return "private"
	case telegram.ChatTypeBasicGroup:
		return "group"
	case telegram.ChatTypeSupergroup:
		return "supergroup"
	case telegram.ChatTypeChannel:
		return "channel"
	default:
		return "unknown"
	}
}

// ToMessageInfo converts a domain message to its JSON DTO.
func ToMessageInfo(m *telegram.Message) MessageInfo {
	info := MessageInfo{
		ID:               m.ID,
		ChatID:           m.ChatID,
		Date:             m.Date,
		IsOutgoing:       m.IsOutgoing,
		ReplyToMessageID: m.ReplyToMessageID,
	}

	switch s := m.SenderID.(type) {
	case *telegram.MessageSenderUser:
		info.SenderUserID = s.UserID
	case *telegram.MessageSenderChat:
		info.SenderChatID = s.ChatID
	}

	info.Type, info.Text = contentSummary(m)
	if key, name, size := MediaFile(m); key != "" {
		info.MediaFileKey = key
		info.MediaFileName = name
		info.MediaSize = size
	}
	return info
}

// ToContactInfo converts a domain user to its JSON DTO.
func ToContactInfo(u *telegram.User) ContactInfo {
	return ContactInfo{
		ID:        u.ID,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Username:  u.Username,
		Phone:     u.PhoneNumber,
		IsBot:     u.IsBot,
	}
}

// contentSummary returns the type label and the text/caption of a message.
func contentSummary(m *telegram.Message) (typ, text string) {
	captionText := func(ft *telegram.FormattedText) string {
		if ft == nil {
			return ""
		}
		return ft.Text
	}

	switch c := m.Content.(type) {
	case *telegram.MessageText:
		return "text", captionText(c.Text)
	case *telegram.MessagePhoto:
		return "photo", captionText(c.Caption)
	case *telegram.MessageVideo:
		return "video", captionText(c.Caption)
	case *telegram.MessageDocument:
		return "document", captionText(c.Caption)
	case *telegram.MessageVoiceNote:
		return "voice", captionText(c.Caption)
	case *telegram.MessageVideoNote:
		return "video_note", ""
	case *telegram.MessageSticker:
		return "sticker", c.Sticker.Emoji
	case *telegram.MessageAnimation:
		return "animation", captionText(c.Caption)
	case *telegram.MessageAudio:
		return "audio", captionText(c.Caption)
	case *telegram.MessageLocation:
		return "location", fmt.Sprintf("%f,%f", c.Location.Latitude, c.Location.Longitude)
	case *telegram.MessageContact:
		return "contact", c.Contact.FirstName + " " + c.Contact.LastName
	case *telegram.MessagePoll:
		return "poll", c.Poll.Question
	case *telegram.MessageUnsupported:
		return "unsupported", c.Type
	default:
		// Service messages (pin, joins, title changes, …).
		// Jangan panggil MessagePreview di sini: MessagePreview balik manggil
		// contentSummary → rekursi tak berujung → stack overflow (server crash)
		// tiap ada service message (mis. bot/member baru join grup).
		return "service", ""
	}
}

// MediaFile extracts the best downloadable file from a message:
// key, display name, size. Empty key when there is none.
func MediaFile(m *telegram.Message) (key, name string, size int64) {
	fileInfo := func(f *telegram.File, fallback string) (string, string, int64) {
		if f == nil {
			return "", "", 0
		}
		return f.ID, fallback, f.Size
	}

	switch c := m.Content.(type) {
	case *telegram.MessagePhoto:
		if c.Photo == nil || len(c.Photo.Sizes) == 0 {
			return "", "", 0
		}
		// Sizes arrive smallest first — take the biggest.
		best := c.Photo.Sizes[len(c.Photo.Sizes)-1]
		return fileInfo(best.File, "photo.jpg")
	case *telegram.MessageVideo:
		return fileInfo(c.Video.File, c.Video.FileName)
	case *telegram.MessageDocument:
		return fileInfo(c.Document.File, c.Document.FileName)
	case *telegram.MessageVoiceNote:
		return fileInfo(c.VoiceNote.File, "voice.ogg")
	case *telegram.MessageVideoNote:
		return fileInfo(c.VideoNote.File, "videonote.mp4")
	case *telegram.MessageSticker:
		return fileInfo(c.Sticker.File, "sticker.webp")
	case *telegram.MessageAnimation:
		return fileInfo(c.Animation.File, c.Animation.FileName)
	case *telegram.MessageAudio:
		return fileInfo(c.Audio.File, c.Audio.FileName)
	default:
		return "", "", 0
	}
}

// MessagePreview renders a short one-line preview of a message.
func MessagePreview(m *telegram.Message) string {
	typ, text := contentSummary(m)
	if text != "" {
		const maxLen = 80
		runes := []rune(text)
		if len(runes) > maxLen {
			text = string(runes[:maxLen]) + "..."
		}
		return text
	}
	return "[" + typ + "]"
}

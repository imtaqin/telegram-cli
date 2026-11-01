// Package restapi exposes the Telegram account as a JSON REST API.
// It uses only the standard library (net/http, Go 1.22+ method patterns).
package restapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gotd/td/telegram/peers"

	"github.com/imtaqin/telegram-cli/internal/telegram"
	"github.com/imtaqin/telegram-cli/internal/tgjson"
)

// Server is the REST API handler set bound to a Telegram client.
type Server struct {
	tg  *telegram.Client
	mux *http.ServeMux
}

// New creates the REST API server and registers all routes.
func New(client *telegram.Client) *Server {
	s := &Server{tg: client}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/me", s.me)
	mux.HandleFunc("GET /api/chats", s.chats)
	mux.HandleFunc("GET /api/chats/{id}/history", s.chatHistory)
	mux.HandleFunc("GET /api/search/chats", s.searchChats)
	mux.HandleFunc("GET /api/search/messages", s.searchMessages)
	mux.HandleFunc("GET /api/contacts", s.contacts)
	mux.HandleFunc("POST /api/send", s.send)
	mux.HandleFunc("POST /api/send-file", s.sendFile)
	mux.HandleFunc("POST /api/edit", s.edit)
	mux.HandleFunc("POST /api/mark-read", s.markRead)
	mux.HandleFunc("GET /api/media", s.media)
	// JSON 404 for anything else.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found: "+r.Method+" "+r.URL.Path)
	})
	s.mux = mux
	return s
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// --- Response types ---

type errorOut struct {
	Error string `json:"error"`
}

type userOut struct {
	User tgjson.ContactInfo `json:"user"`
}

type chatsOut struct {
	Chats []tgjson.ChatInfo `json:"chats"`
}

type messagesOut struct {
	Messages []tgjson.MessageInfo `json:"messages"`
}

type contactsOut struct {
	Contacts []tgjson.ContactInfo `json:"contacts"`
}

type messageOut struct {
	Message tgjson.MessageInfo `json:"message"`
}

type statusOut struct {
	Status string `json:"status"`
}

type mediaOut struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Name string `json:"name,omitempty"`
}

// --- Request types ---

type sendIn struct {
	ChatID           int64  `json:"chat_id"`
	Text             string `json:"text"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
}

type sendFileIn struct {
	ChatID           int64  `json:"chat_id"`
	Path             string `json:"path"`
	Caption          string `json:"caption,omitempty"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
}

type editIn struct {
	ChatID    int64  `json:"chat_id"`
	MessageID int64  `json:"message_id"`
	Text      string `json:"text"`
}

type markReadIn struct {
	ChatID     int64   `json:"chat_id"`
	MessageIDs []int64 `json:"message_ids"`
}

// --- Handlers ---

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, statusOut{Status: "ok"})
}

func (s *Server) me(w http.ResponseWriter, _ *http.Request) {
	me, err := s.tg.GetMe()
	if err != nil {
		writeTelegramError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, userOut{User: tgjson.ToContactInfo(me)})
}

func (s *Server) chats(w http.ResponseWriter, r *http.Request) {
	limit, ok := limitQuery(w, r, 50)
	if !ok {
		return
	}
	chats, err := s.tg.ListChats(int(limit))
	if err != nil {
		writeTelegramError(w, err)
		return
	}
	out := chatsOut{Chats: make([]tgjson.ChatInfo, 0, len(chats))}
	for _, chat := range chats {
		out.Chats = append(out.Chats, tgjson.ToChatInfo(chat))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) chatHistory(w http.ResponseWriter, r *http.Request) {
	chatID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chat id")
		return
	}
	limit, ok := limitQuery(w, r, 50)
	if !ok {
		return
	}
	fromID, ok := intQuery(w, r, "from_message_id", 0)
	if !ok {
		return
	}
	if fromID < 0 {
		writeError(w, http.StatusBadRequest, "invalid parameter: from_message_id must not be negative")
		return
	}
	offset, ok := intQuery(w, r, "offset", 0)
	if !ok {
		return
	}

	msgs, err := s.tg.GetChatHistory(chatID, int64(fromID), offset, limit)
	if err != nil {
		writeTelegramError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toMessagesOut(msgs))
}

func (s *Server) searchChats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "missing required parameter: q")
		return
	}
	limit, ok := limitQuery(w, r, 20)
	if !ok {
		return
	}

	chats, err := s.tg.SearchChats(q, limit)
	if err != nil {
		writeTelegramError(w, err)
		return
	}
	out := chatsOut{Chats: make([]tgjson.ChatInfo, 0, len(chats))}
	for _, chat := range chats {
		out.Chats = append(out.Chats, tgjson.ToChatInfo(chat))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) searchMessages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "missing required parameter: q")
		return
	}
	limit, ok := limitQuery(w, r, 20)
	if !ok {
		return
	}

	msgs, err := s.tg.SearchMessages(q, limit)
	if err != nil {
		writeTelegramError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toMessagesOut(msgs))
}

func (s *Server) contacts(w http.ResponseWriter, _ *http.Request) {
	users, err := s.tg.GetContacts()
	if err != nil {
		writeTelegramError(w, err)
		return
	}
	out := contactsOut{Contacts: make([]tgjson.ContactInfo, 0, len(users))}
	for _, u := range users {
		out.Contacts = append(out.Contacts, tgjson.ToContactInfo(u))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) send(w http.ResponseWriter, r *http.Request) {
	var in sendIn
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.ChatID == 0 {
		writeError(w, http.StatusBadRequest, "missing required field: chat_id")
		return
	}
	if in.Text == "" {
		writeError(w, http.StatusBadRequest, "missing required field: text")
		return
	}

	msg, err := s.tg.SendTextMessage(in.ChatID, in.Text, in.ReplyToMessageID)
	if err != nil {
		writeTelegramError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, messageOut{Message: tgjson.ToMessageInfo(msg)})
}

func (s *Server) sendFile(w http.ResponseWriter, r *http.Request) {
	var in sendFileIn
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.ChatID == 0 {
		writeError(w, http.StatusBadRequest, "missing required field: chat_id")
		return
	}
	if in.Path == "" {
		writeError(w, http.StatusBadRequest, "missing required field: path")
		return
	}

	msg, err := s.tg.SendFileMessage(in.ChatID, in.Path, in.Caption, in.ReplyToMessageID)
	if err != nil {
		writeTelegramError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, messageOut{Message: tgjson.ToMessageInfo(msg)})
}

func (s *Server) edit(w http.ResponseWriter, r *http.Request) {
	var in editIn
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.ChatID == 0 {
		writeError(w, http.StatusBadRequest, "missing required field: chat_id")
		return
	}
	if in.MessageID == 0 {
		writeError(w, http.StatusBadRequest, "missing required field: message_id")
		return
	}
	if in.Text == "" {
		writeError(w, http.StatusBadRequest, "missing required field: text")
		return
	}

	msg, err := s.tg.EditTextMessage(in.ChatID, in.MessageID, in.Text)
	if err != nil {
		writeTelegramError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, messageOut{Message: tgjson.ToMessageInfo(msg)})
}

func (s *Server) markRead(w http.ResponseWriter, r *http.Request) {
	var in markReadIn
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.ChatID == 0 {
		writeError(w, http.StatusBadRequest, "missing required field: chat_id")
		return
	}
	if len(in.MessageIDs) == 0 {
		writeError(w, http.StatusBadRequest, "missing required field: message_ids")
		return
	}

	if err := s.tg.ViewMessages(in.ChatID, in.MessageIDs); err != nil {
		writeTelegramError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, statusOut{Status: "ok"})
}

func (s *Server) media(w http.ResponseWriter, r *http.Request) {
	chatID, err := strconv.ParseInt(r.URL.Query().Get("chat_id"), 10, 64)
	if err != nil || chatID == 0 {
		writeError(w, http.StatusBadRequest, "missing or invalid parameter: chat_id")
		return
	}
	messageID, err := strconv.ParseInt(r.URL.Query().Get("message_id"), 10, 64)
	if err != nil || messageID == 0 {
		writeError(w, http.StatusBadRequest, "missing or invalid parameter: message_id")
		return
	}

	msg, err := s.tg.GetMessage(chatID, messageID)
	if err != nil {
		writeTelegramError(w, err)
		return
	}

	key, name, _ := tgjson.MediaFile(msg)
	if key == "" {
		writeError(w, http.StatusBadRequest, "message has no downloadable media")
		return
	}

	file, err := s.tg.DownloadFileSync(key)
	if err != nil {
		writeTelegramError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mediaOut{Path: file.Path, Size: file.Size, Name: name})
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, errorOut{Error: msg})
}

// writeTelegramError maps an upstream error to a status code:
// unknown chat/peer → 404, everything else → 502.
func writeTelegramError(w http.ResponseWriter, err error) {
	var notFound *peers.PeerNotFoundError
	if errors.As(err, &notFound) {
		writeError(w, http.StatusNotFound, "chat not found")
		return
	}
	writeError(w, http.StatusBadGateway, "telegram error: "+err.Error())
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid JSON body: expected a single JSON value")
		return false
	}
	return true
}

func intQuery(w http.ResponseWriter, r *http.Request, name string, def int32) (int32, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def, true
	}
	// Telegram integer fields are signed 32-bit values. Validate before
	// converting so large client arguments cannot silently wrap around.
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid parameter: "+name+" must be a 32-bit integer")
		return 0, false
	}
	return int32(n), true
}

func limitQuery(w http.ResponseWriter, r *http.Request, def int32) (int32, bool) {
	limit, ok := intQuery(w, r, "limit", def)
	if !ok {
		return 0, false
	}
	if limit <= 0 {
		writeError(w, http.StatusBadRequest, "invalid parameter: limit must be positive")
		return 0, false
	}
	if limit > 100 {
		limit = 100
	}
	return limit, true
}

func toMessagesOut(msgs []*telegram.Message) messagesOut {
	out := messagesOut{Messages: make([]tgjson.MessageInfo, 0, len(msgs))}
	for _, m := range msgs {
		out.Messages = append(out.Messages, tgjson.ToMessageInfo(m))
	}
	return out
}

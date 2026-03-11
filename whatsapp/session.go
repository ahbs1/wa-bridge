package whatsapp

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	qrcode "github.com/skip2/go-qrcode"
	"google.golang.org/protobuf/proto"
)

type SessionStatus string

const (
	StatusStopped      SessionStatus = "stopped"
	StatusConnecting   SessionStatus = "connecting"
	StatusWaitingQR    SessionStatus = "waiting_qr"
	StatusConnected    SessionStatus = "connected"
	StatusDisconnected SessionStatus = "disconnected"
	StatusLoggedOut    SessionStatus = "logged_out"
)

type SessionEvent struct {
	SessionID string      `json:"session"`
	Event     string      `json:"event"`
	Data      interface{} `json:"data"`
}

type WhatsAppSession struct {
	ID     string        `json:"id"`
	Status SessionStatus `json:"status"`
	QRCode string        `json:"qr,omitempty"`

	client    *whatsmeow.Client
	container *sqlstore.Container
	mu        sync.RWMutex
	eventCh   chan SessionEvent
	stopCh    chan struct{}

	IgnoreGroups    bool
	IgnoreBroadcast bool
	IgnoreStatus    bool
	IgnoreChannels  bool
	ChatwootInboxID string
}

func NewSession(id string, eventCh chan SessionEvent) (*WhatsAppSession, error) {
	dataDir := filepath.Join("data", "sessions", id)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "store.db")
	dbURI := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)

	logger := waLog.Noop
	container, err := sqlstore.New(context.Background(), "sqlite", dbURI, logger)
	if err != nil {
		return nil, fmt.Errorf("create store: %w", err)
	}

	return &WhatsAppSession{
		ID:        id,
		Status:    StatusStopped,
		container: container,
		eventCh:   eventCh,
		stopCh:    make(chan struct{}),
	}, nil
}

func (s *WhatsAppSession) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Status == StatusConnected || s.Status == StatusConnecting {
		return nil
	}

	s.Status = StatusConnecting
	s.emitStatus()

	deviceStore, err := s.container.GetFirstDevice(context.Background())
	if err != nil {
		return fmt.Errorf("get device: %w", err)
	}

	logger := waLog.Noop
	s.client = whatsmeow.NewClient(deviceStore, logger)
	s.client.AddEventHandler(s.handleEvent)

	if s.client.Store.ID == nil {
		// Need QR login
		qrChan, _ := s.client.GetQRChannel(context.Background())
		if err := s.client.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}

		go func() {
			for evt := range qrChan {
				switch evt.Event {
				case "code":
					png, err := qrcode.Encode(evt.Code, qrcode.Medium, 300)
					if err == nil {
						dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
						s.mu.Lock()
						s.QRCode = dataURL
						s.Status = StatusWaitingQR
						s.mu.Unlock()
						s.emitStatus()
						s.emit("qr", map[string]string{"qr": dataURL})
						fmt.Printf("[Session:%s] QR code generated\n", s.ID)
					}
				case "login":
					s.mu.Lock()
					s.QRCode = ""
					s.Status = StatusConnected
					s.mu.Unlock()
					s.emitStatus()
					fmt.Printf("[Session:%s] ✅ Login successful!\n", s.ID)
				case "timeout":
					s.mu.Lock()
					s.QRCode = ""
					s.Status = StatusDisconnected
					s.mu.Unlock()
					s.emitStatus()
					fmt.Printf("[Session:%s] QR timeout\n", s.ID)
				}
			}
		}()
	} else {
		// Already logged in
		if err := s.client.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}
		s.Status = StatusConnected
		s.emitStatus()
		fmt.Printf("[Session:%s] ✅ Reconnected!\n", s.ID)
	}

	return nil
}

func (s *WhatsAppSession) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		s.client.Disconnect()
		s.client = nil
	}
	s.Status = StatusStopped
	s.QRCode = ""
	s.emitStatus()
}

func (s *WhatsAppSession) Restart() error {
	s.Stop()
	return s.Start()
}

func (s *WhatsAppSession) Logout() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		s.client.Logout(context.Background())
		s.client.Disconnect()
		s.client = nil
	}
	s.Status = StatusLoggedOut
	s.QRCode = ""
	s.emitStatus()
}

func (s *WhatsAppSession) GetClient() *whatsmeow.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client
}

func (s *WhatsAppSession) GetInfo() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	info := map[string]interface{}{
		"id":     s.ID,
		"status": s.Status,
	}
	if s.Status == StatusWaitingQR && s.QRCode != "" {
		info["qr"] = s.QRCode
	}
	return info
}

func (s *WhatsAppSession) handleEvent(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		s.handleMessage(v)
	case *events.Connected:
		s.mu.Lock()
		s.Status = StatusConnected
		s.QRCode = ""
		s.mu.Unlock()
		s.emitStatus()
		s.emit("session.connected", nil)
	case *events.Disconnected:
		s.mu.Lock()
		s.Status = StatusDisconnected
		s.mu.Unlock()
		s.emitStatus()
		// Auto-reconnect
		go func() {
			time.Sleep(3 * time.Second)
			s.mu.RLock()
			st := s.Status
			s.mu.RUnlock()
			if st == StatusDisconnected {
				fmt.Printf("[Session:%s] Attempting reconnect...\n", s.ID)
				s.Start()
			}
		}()
	case *events.LoggedOut:
		s.mu.Lock()
		s.Status = StatusLoggedOut
		s.mu.Unlock()
		s.emitStatus()
	}
}

func (s *WhatsAppSession) handleMessage(msg *events.Message) {
	if msg.Info.IsFromMe {
		return
	}

	jid := msg.Info.Chat.String()

	// Filters
	if s.IgnoreBroadcast && msg.Info.Chat.Server == "broadcast" {
		return
	}
	if s.IgnoreStatus && jid == "status@broadcast" {
		return
	}
	if s.IgnoreGroups && msg.Info.Chat.Server == "g.us" {
		return
	}
	if s.IgnoreChannels && msg.Info.Chat.Server == "newsletter" {
		return
	}

	text := ""
	mediaType := ""
	var mediaData []byte
	mimetype := ""
	filename := ""
	caption := ""

	if msg.Message.GetConversation() != "" {
		text = msg.Message.GetConversation()
	} else if msg.Message.GetExtendedTextMessage() != nil {
		text = msg.Message.GetExtendedTextMessage().GetText()
	} else if imgMsg := msg.Message.GetImageMessage(); imgMsg != nil {
		caption = imgMsg.GetCaption()
		text = caption
		if text == "" {
			text = "[image]"
		}
		mediaType = "image"
		mimetype = imgMsg.GetMimetype()
		if data, err := s.client.Download(context.Background(), imgMsg); err == nil {
			mediaData = data
		}
	} else if vidMsg := msg.Message.GetVideoMessage(); vidMsg != nil {
		caption = vidMsg.GetCaption()
		text = caption
		if text == "" {
			text = "[video]"
		}
		mediaType = "video"
		mimetype = vidMsg.GetMimetype()
		if data, err := s.client.Download(context.Background(), vidMsg); err == nil {
			mediaData = data
		}
	} else if docMsg := msg.Message.GetDocumentMessage(); docMsg != nil {
		filename = docMsg.GetFileName()
		text = "[document] " + filename
		mediaType = "document"
		mimetype = docMsg.GetMimetype()
		if data, err := s.client.Download(context.Background(), docMsg); err == nil {
			mediaData = data
		}
	} else if audioMsg := msg.Message.GetAudioMessage(); audioMsg != nil {
		text = "[audio]"
		mediaType = "audio"
		mimetype = audioMsg.GetMimetype()
		if data, err := s.client.Download(context.Background(), audioMsg); err == nil {
			mediaData = data
		}
	} else if msg.Message.GetLocationMessage() != nil {
		loc := msg.Message.GetLocationMessage()
		text = fmt.Sprintf("[location] %.6f, %.6f", loc.GetDegreesLatitude(), loc.GetDegreesLongitude())
	} else if msg.Message.GetContactMessage() != nil {
		text = "[contact] " + msg.Message.GetContactMessage().GetDisplayName()
	} else {
		text = "[message]"
	}

	eventData := map[string]interface{}{
		"from":      msg.Info.Sender.User,
		"chat":      jid,
		"name":      msg.Info.PushName,
		"preview":   text,
		"id":        msg.Info.ID,
		"timestamp": msg.Info.Timestamp.Format(time.RFC3339),
	}

	if mediaType != "" {
		eventData["mediaType"] = mediaType
		eventData["mimetype"] = mimetype
		eventData["filename"] = filename
		eventData["caption"] = caption
	}
	if mediaData != nil {
		eventData["mediaData"] = base64.StdEncoding.EncodeToString(mediaData)
	}

	s.emit("message.received", eventData)
}

// === Send Message Functions ===

func (s *WhatsAppSession) SendText(jid types.JID, text string) (string, error) {
	client := s.GetClient()
	if client == nil {
		return "", fmt.Errorf("not connected")
	}
	resp, err := client.SendMessage(context.Background(), jid, &waE2E.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (s *WhatsAppSession) SendImage(jid types.JID, data []byte, caption string, mimetype string) (string, error) {
	client := s.GetClient()
	if client == nil {
		return "", fmt.Errorf("not connected")
	}
	uploaded, err := client.Upload(context.Background(), data, whatsmeow.MediaImage)
	if err != nil {
		return "", fmt.Errorf("upload: %w", err)
	}
	resp, err := client.SendMessage(context.Background(), jid, &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			Caption:       proto.String(caption),
			Mimetype:      proto.String(mimetype),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (s *WhatsAppSession) SendVideo(jid types.JID, data []byte, caption string, mimetype string) (string, error) {
	client := s.GetClient()
	if client == nil {
		return "", fmt.Errorf("not connected")
	}
	uploaded, err := client.Upload(context.Background(), data, whatsmeow.MediaVideo)
	if err != nil {
		return "", fmt.Errorf("upload: %w", err)
	}
	resp, err := client.SendMessage(context.Background(), jid, &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			Caption:       proto.String(caption),
			Mimetype:      proto.String(mimetype),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (s *WhatsAppSession) SendDocument(jid types.JID, data []byte, filename string, mimetype string) (string, error) {
	client := s.GetClient()
	if client == nil {
		return "", fmt.Errorf("not connected")
	}
	uploaded, err := client.Upload(context.Background(), data, whatsmeow.MediaDocument)
	if err != nil {
		return "", fmt.Errorf("upload: %w", err)
	}
	resp, err := client.SendMessage(context.Background(), jid, &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			Title:         proto.String(filename),
			FileName:      proto.String(filename),
			Mimetype:      proto.String(mimetype),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (s *WhatsAppSession) SendAudio(jid types.JID, data []byte, ptt bool) (string, error) {
	client := s.GetClient()
	if client == nil {
		return "", fmt.Errorf("not connected")
	}
	uploaded, err := client.Upload(context.Background(), data, whatsmeow.MediaAudio)
	if err != nil {
		return "", fmt.Errorf("upload: %w", err)
	}
	resp, err := client.SendMessage(context.Background(), jid, &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			Mimetype:      proto.String("audio/ogg; codecs=opus"),
			PTT:           proto.Bool(ptt),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (s *WhatsAppSession) SendLocation(jid types.JID, lat, lng float64, name, address string) (string, error) {
	client := s.GetClient()
	if client == nil {
		return "", fmt.Errorf("not connected")
	}
	resp, err := client.SendMessage(context.Background(), jid, &waE2E.Message{
		LocationMessage: &waE2E.LocationMessage{
			DegreesLatitude:  proto.Float64(lat),
			DegreesLongitude: proto.Float64(lng),
			Name:             proto.String(name),
			Address:          proto.String(address),
		},
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (s *WhatsAppSession) SendContact(jid types.JID, displayName string, vcard string) (string, error) {
	client := s.GetClient()
	if client == nil {
		return "", fmt.Errorf("not connected")
	}
	resp, err := client.SendMessage(context.Background(), jid, &waE2E.Message{
		ContactMessage: &waE2E.ContactMessage{
			DisplayName: proto.String(displayName),
			Vcard:       proto.String(vcard),
		},
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (s *WhatsAppSession) SendPoll(jid types.JID, title string, options []string, selectableCount int) (string, error) {
	client := s.GetClient()
	if client == nil {
		return "", fmt.Errorf("not connected")
	}
	pollOptions := make([]*waE2E.PollCreationMessage_Option, len(options))
	for i, opt := range options {
		pollOptions[i] = &waE2E.PollCreationMessage_Option{
			OptionName: proto.String(opt),
		}
	}
	resp, err := client.SendMessage(context.Background(), jid, &waE2E.Message{
		PollCreationMessage: &waE2E.PollCreationMessage{
			Name:                   proto.String(title),
			Options:                pollOptions,
			SelectableOptionsCount: proto.Uint32(uint32(selectableCount)),
		},
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (s *WhatsAppSession) ReactMessage(jid types.JID, messageID string, emoji string) error {
	client := s.GetClient()
	if client == nil {
		return fmt.Errorf("not connected")
	}
	_, err := client.SendMessage(context.Background(), jid, &waE2E.Message{
		ReactionMessage: &waE2E.ReactionMessage{
			Key: &waCommon.MessageKey{
				RemoteJID: proto.String(jid.String()),
				ID:        proto.String(messageID),
				FromMe:    proto.Bool(false),
			},
			Text:              proto.String(emoji),
			SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
		},
	})
	return err
}

func (s *WhatsAppSession) EditMessage(jid types.JID, messageID string, newText string) error {
	client := s.GetClient()
	if client == nil {
		return fmt.Errorf("not connected")
	}
	_, err := client.SendMessage(context.Background(), jid, client.BuildEdit(jid, messageID, &waE2E.Message{
		Conversation: proto.String(newText),
	}))
	return err
}

func (s *WhatsAppSession) DeleteMessage(jid types.JID, messageID string) error {
	client := s.GetClient()
	if client == nil {
		return fmt.Errorf("not connected")
	}
	_, err := client.SendMessage(context.Background(), jid, client.BuildRevoke(jid, types.EmptyJID, messageID))
	return err
}

// Helper
func (s *WhatsAppSession) emit(event string, data interface{}) {
	select {
	case s.eventCh <- SessionEvent{SessionID: s.ID, Event: event, Data: data}:
	default:
	}
}

func (s *WhatsAppSession) emitStatus() {
	s.emit("session.status", map[string]interface{}{
		"status": s.Status,
	})
}

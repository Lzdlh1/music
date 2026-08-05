package source

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gotd/td/tg"
	"github.com/musicflow/musicflow/internal/telegram"
	"go.uber.org/zap"
)

// TGBotSourceConfig TG 资源机器人配置
type TGBotSourceConfig struct {
	Name     string `json:"name"`
	Username string `json:"username"` // 例如: @VKMusicBot
	Priority int    `json:"priority"`
}

// TGBotSource 实现 MusicSource 接口，通过与 TG 机器人交互搜索音乐
type TGBotSource struct {
	cfg   TGBotSourceConfig
	mtMgr *telegram.MTProtoManager
	log   *zap.Logger
}

// NewTGBotSource 创建 TG 机器人搜索源
func NewTGBotSource(cfg TGBotSourceConfig, mtMgr *telegram.MTProtoManager, log *zap.Logger) *TGBotSource {
	return &TGBotSource{
		cfg:   cfg,
		mtMgr: mtMgr,
		log:   log,
	}
}

func (s *TGBotSource) Name() string {
	return s.cfg.Name
}

func (s *TGBotSource) Priority() int {
	return s.cfg.Priority
}

func (s *TGBotSource) Search(ctx context.Context, query SearchQuery) ([]TrackResult, error) {
	clientInst, err := s.mtMgr.GetClient()
	if err != nil {
		return nil, fmt.Errorf("mtproto client unavailable: %w", err)
	}

	botUsername := s.cfg.Username
	if !strings.HasPrefix(botUsername, "@") {
		botUsername = "@" + botUsername
	}

	// 解析 bot
	resolved, err := clientInst.API.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: strings.TrimPrefix(botUsername, "@")})
	if err != nil {
		return nil, fmt.Errorf("resolve bot: %w", err)
	}
	if len(resolved.Users) == 0 {
		return nil, fmt.Errorf("bot user not found")
	}
	botUser := resolved.Users[0].(*tg.User)

	inputPeer := &tg.InputPeerUser{
		UserID:     botUser.ID,
		AccessHash: botUser.AccessHash,
	}

	// 记录发送搜索请求前最后一条消息的 ID，以便过滤旧回复
	history, err := clientInst.API.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:  inputPeer,
		Limit: 1,
	})

	var lastMsgID int
	if err == nil {
		if modified, ok := history.(*tg.MessagesMessagesSlice); ok && len(modified.Messages) > 0 {
			if msg, ok := modified.Messages[0].(*tg.Message); ok {
				lastMsgID = msg.ID
			}
		}
	}

	// 1. 发送搜索消息
	_, err = clientInst.API.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     inputPeer,
		Message:  query.Keyword,
		RandomID: time.Now().UnixNano(),
	})
	if err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}

	// 2. 轮询等待回复 (带超时)
	timeout := time.After(15 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var results []TrackResult

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			// 超时返回目前已收到的结果，或者空
			return results, nil
		case <-ticker.C:
			// 获取新消息
			newHistory, err := clientInst.API.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
				Peer:  inputPeer,
				Limit: 10,
			})
			if err != nil {
				continue
			}

			if modified, ok := newHistory.(*tg.MessagesMessagesSlice); ok {
				for _, m := range modified.Messages {
					msg, ok := m.(*tg.Message)
					if !ok || msg.ID <= lastMsgID {
						continue // 忽略旧消息或非普通消息
					}

					// 判断是不是来自 Bot 的回复
					if msg.FromID != nil {
						if peerUser, ok := msg.FromID.(*tg.PeerUser); ok && peerUser.UserID == botUser.ID {
							// 解析消息内容
							tracks := s.parseMessageToTracks(msg)
							if len(tracks) > 0 {
								results = append(results, tracks...)
								return results, nil // 收到音频直接返回
							}

							// 如果 Bot 返回了 inline keyboard 供选择
							if msg.ReplyMarkup != nil {
								// 这里可以扩展自动点击第一个结果的逻辑
								// 简单实现：暂不处理复杂的交互式 Bot，优先处理直接回复音频的 Bot
							}
						}
					}
				}
			}
		}
	}
}

// parseMessageToTracks 从消息中解析出音频文件
func (s *TGBotSource) parseMessageToTracks(msg *tg.Message) []TrackResult {
	var tracks []TrackResult

	if msg.Media == nil {
		return tracks
	}

	mediaDoc, ok := msg.Media.(*tg.MessageMediaDocument)
	if !ok {
		return tracks
	}

	doc, ok := mediaDoc.Document.(*tg.Document)
	if !ok {
		return tracks
	}

	// 检查是否是音频
	isAudio := false
	var title, artist, fileName string
	var duration int

	for _, attr := range doc.Attributes {
		switch a := attr.(type) {
		case *tg.DocumentAttributeAudio:
			isAudio = true
			duration = a.Duration
			title = a.Title
			artist = a.Performer
		case *tg.DocumentAttributeFilename:
			fileName = a.FileName
		}
	}

	if !isAudio && !strings.HasPrefix(doc.MimeType, "audio/") {
		return tracks
	}

	if title == "" && fileName != "" {
		title = fileName
	}

	// 构造唯一 ID：将 document 的信息打包
	// 格式: tgbot:{botUsername}:{docID}:{accessHash}
	trackID := fmt.Sprintf("tgbot:%s:%d:%d", s.cfg.Username, doc.ID, doc.AccessHash)

	// 估算音质
	quality := Quality128
	sizeMB := float64(doc.Size) / 1024 / 1024
	if strings.Contains(strings.ToLower(fileName), ".flac") || sizeMB > 20 {
		quality = QualityFLAC
	} else if sizeMB > 8 {
		quality = Quality320
	}

	tracks = append(tracks, TrackResult{
		ID:       trackID,
		Title:    title,
		Artist:   artist,
		Duration: duration,
		Source:   s.Name(),
		Quality:  quality,
		FileSize: doc.Size,
	})

	return tracks
}

func (s *TGBotSource) GetTrackDetail(ctx context.Context, id string) (*TrackDetail, error) {
	// 从 ID 恢复基本信息
	parts := strings.Split(id, ":")
	if len(parts) != 4 {
		return nil, fmt.Errorf("invalid track id")
	}

	return &TrackDetail{
		ID:     id,
		Source: s.Name(),
	}, nil
}

func (s *TGBotSource) GetDownloadURL(ctx context.Context, id string, quality Quality) (*DownloadURL, error) {
	// 格式: tgbot:{botUsername}:{docID}:{accessHash}
	parts := strings.Split(id, ":")
	if len(parts) != 4 {
		return nil, fmt.Errorf("invalid track id")
	}

	// 返回特殊的 mtproto 协议 URL，Worker 层拦截处理
	url := fmt.Sprintf("mtproto://%s/%s/%s", parts[1], parts[2], parts[3])

	return &DownloadURL{
		URL:     url,
		Quality: quality,
	}, nil
}

func (s *TGBotSource) GetLyrics(ctx context.Context, id string) (*LyricsResult, error) {
	return nil, nil
}

func (s *TGBotSource) GetCover(ctx context.Context, id string) (*CoverResult, error) {
	return nil, nil
}

func (s *TGBotSource) IsAvailable(ctx context.Context) bool {
	return true
}

func filepathExt(filename string) string {
	parts := strings.Split(filename, ".")
	if len(parts) > 1 {
		return strings.ToLower(parts[len(parts)-1])
	}
	return "mp3"
}

package source

import "context"

// Quality 音质等级
type Quality int

const (
	QualityAny   Quality = 0
	Quality128   Quality = 128
	Quality320   Quality = 320
	QualityFLAC  Quality = 999
	QualityHiRes Quality = 9999
)

func (q Quality) String() string {
	switch q {
	case Quality128:
		return "128K"
	case Quality320:
		return "320K"
	case QualityFLAC:
		return "FLAC"
	case QualityHiRes:
		return "Hi-Res"
	default:
		return "ANY"
	}
}

// SearchQuery 搜索请求
type SearchQuery struct {
	Keyword  string  `json:"keyword"`
	Artist   string  `json:"artist"`
	Album    string  `json:"album"`
	Quality  Quality `json:"quality"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
}

// TrackResult 搜索结果条目
type TrackResult struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Album    string  `json:"album"`
	Duration int     `json:"duration"`
	Quality  Quality `json:"quality"`
	FileSize int64   `json:"file_size"`
	Source   string  `json:"source"`
	CoverURL string  `json:"cover_url"`
	Score    float64 `json:"score"`
}

// TrackDetail 曲目详细信息
type TrackDetail struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	AlbumArtist string `json:"album_artist"`
	Album       string `json:"album"`
	TrackNo     int    `json:"track_no"`
	DiscNo      int    `json:"disc_no"`
	Year        int    `json:"year"`
	Genre       string `json:"genre"`
	Duration    int    `json:"duration"`
	CoverURL    string `json:"cover_url"`
	Source      string `json:"source"`
}

// DownloadURL 下载链接信息
type DownloadURL struct {
	URL       string  `json:"url"`
	Quality   Quality `json:"quality"`
	Format    string  `json:"format"`
	FileSize  int64   `json:"file_size"`
	ExpiresIn int     `json:"expires_in"`
}

// LyricsResult 歌词结果
type LyricsResult struct {
	LRC        string `json:"lrc"`
	TransLRC   string `json:"trans_lrc,omitempty"`
	PlainText  string `json:"plain_text,omitempty"`
	Source     string `json:"source"`
}

// CoverResult 封面结果
type CoverResult struct {
	URL    string `json:"url"`
	Data   []byte `json:"-"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Source string `json:"source"`
}

// AvailableSource 可用的下载源
type AvailableSource struct {
	SourceName  string  `json:"source_name"`
	Quality     Quality `json:"quality"`
	FileSize    int64   `json:"file_size"`
	Format      string  `json:"format"`
	BitRate     int     `json:"bit_rate"`
	SampleRate  int     `json:"sample_rate"`
	BitDepth    int     `json:"bit_depth"`
	Score       float64 `json:"score"`
	DownloadURL string  `json:"download_url"`
}

// AggregatedResult 聚合搜索结果
type AggregatedResult struct {
	TrackInfo   TrackDetail      `json:"track_info"`
	Sources     []AvailableSource `json:"sources"`
	Recommended *AvailableSource  `json:"recommended,omitempty"`
}

// MusicSource 音乐源统一接口
type MusicSource interface {
	Name() string
	Search(ctx context.Context, query SearchQuery) ([]TrackResult, error)
	GetTrackDetail(ctx context.Context, id string) (*TrackDetail, error)
	GetDownloadURL(ctx context.Context, id string, quality Quality) (*DownloadURL, error)
	GetLyrics(ctx context.Context, id string) (*LyricsResult, error)
	GetCover(ctx context.Context, id string) (*CoverResult, error)
	IsAvailable(ctx context.Context) bool
	Priority() int
}

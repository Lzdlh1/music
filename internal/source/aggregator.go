package source

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"go.uber.org/zap"
)

// Aggregator 多源聚合器
type Aggregator struct {
	sources []MusicSource
	mu      sync.RWMutex
	log     *zap.Logger
}

// NewAggregator 创建聚合器
func NewAggregator(log *zap.Logger) *Aggregator {
	return &Aggregator{
		log: log,
	}
}

// Register 注册音乐源
func (a *Aggregator) Register(src MusicSource) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sources = append(a.sources, src)
	a.log.Info("music source registered", zap.String("source", src.Name()), zap.Int("priority", src.Priority()))
}

// Sources 获取所有已注册源
func (a *Aggregator) Sources() []MusicSource {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]MusicSource, len(a.sources))
	copy(result, a.sources)
	return result
}

// Search 并发搜索所有源并聚合结果
func (a *Aggregator) Search(ctx context.Context, query SearchQuery) ([]TrackResult, error) {
	a.mu.RLock()
	sources := make([]MusicSource, len(a.sources))
	copy(sources, a.sources)
	a.mu.RUnlock()

	type searchResult struct {
		results []TrackResult
		source  string
		err     error
	}

	ch := make(chan searchResult, len(sources))
	var wg sync.WaitGroup

	for _, src := range sources {
		if !src.IsAvailable(ctx) {
			continue
		}
		wg.Add(1)
		go func(s MusicSource) {
			defer wg.Done()
			results, err := s.Search(ctx, query)
			ch <- searchResult{results: results, source: s.Name(), err: err}
		}(src)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var allResults []TrackResult
	for sr := range ch {
		if sr.err != nil {
			a.log.Warn("source search failed",
				zap.String("source", sr.source),
				zap.Error(sr.err))
			continue
		}
		allResults = append(allResults, sr.results...)
	}

	// 按综合评分排序
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Score > allResults[j].Score
	})

	return allResults, nil
}

// GetTrackSources 获取某曲目所有可用的下载源
func (a *Aggregator) GetTrackSources(ctx context.Context, trackID string, quality Quality) ([]AvailableSource, error) {
	a.mu.RLock()
	sources := make([]MusicSource, len(a.sources))
	copy(sources, a.sources)
	a.mu.RUnlock()

	type dlResult struct {
		source AvailableSource
		err    error
	}

	ch := make(chan dlResult, len(sources))
	var wg sync.WaitGroup

	for _, src := range sources {
		if !src.IsAvailable(ctx) {
			continue
		}
		wg.Add(1)
		go func(s MusicSource) {
			defer wg.Done()
			dl, err := s.GetDownloadURL(ctx, trackID, quality)
			if err != nil {
				ch <- dlResult{err: err}
				return
			}
			ch <- dlResult{source: AvailableSource{
				SourceName:  s.Name(),
				Quality:     dl.Quality,
				FileSize:    dl.FileSize,
				Format:      dl.Format,
				DownloadURL: dl.URL,
				Score:       float64(dl.Quality),
			}}
		}(src)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var available []AvailableSource
	for dr := range ch {
		if dr.err != nil {
			continue
		}
		available = append(available, dr.source)
	}

	// 按音质评分排序
	sort.Slice(available, func(i, j int) bool {
		return available[i].Score > available[j].Score
	})

	return available, nil
}

// GetLyrics 按曲目 ID 从对应源获取歌词（ID 格式为 "源名:rawID"）
func (a *Aggregator) GetLyrics(ctx context.Context, trackID string) (*LyricsResult, error) {
	sources := a.Sources()
	// 优先按源名前缀匹配，找不到则遍历所有源尝试
	if name, _ := splitSourceID(trackID); name != "" {
		for _, s := range sources {
			if s.Name() == name {
				if lyr, err := s.GetLyrics(ctx, trackID); err == nil && lyr != nil && lyr.LRC != "" {
					return lyr, nil
				}
				break
			}
		}
	}
	for _, s := range sources {
		if lyr, err := s.GetLyrics(ctx, trackID); err == nil && lyr != nil && lyr.LRC != "" {
			return lyr, nil
		}
	}
	return nil, fmt.Errorf("no lyrics found for track %s", trackID)
}

// GetCover 按曲目 ID 从对应源获取封面
func (a *Aggregator) GetCover(ctx context.Context, trackID string) (*CoverResult, error) {
	sources := a.Sources()
	if name, _ := splitSourceID(trackID); name != "" {
		for _, s := range sources {
			if s.Name() == name {
				if cv, err := s.GetCover(ctx, trackID); err == nil && cv != nil && cv.URL != "" {
					return cv, nil
				}
				a.log.Info("cover source matched but failed", zap.String("source", s.Name()), zap.String("track", trackID))
				break
			}
		}
	}
	for _, s := range sources {
		if cv, err := s.GetCover(ctx, trackID); err == nil && cv != nil && cv.URL != "" {
			return cv, nil
		}
	}
	return nil, fmt.Errorf("no cover found for track %s", trackID)
}

package player

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/unicast/unicast-mpv/pkg/logger"
	"github.com/unicast/unicast-mpv/pkg/util/cases"
)

var ErrStatusTimeout = errors.New("player: status wait timed out")

var StatusInfoRequiredKeys = []string{
	"duration", "position", "filename", "path",
	"mediaTitle", "playlistPos", "playlistCount",
}

type MPVPlayer interface {
	IsRunning() bool
	GetTimePosition() (float64, error)
}

type StatusInfo struct {
	Mute          bool    `json:"mute"`
	Pause         bool    `json:"pause"`
	Duration      float64 `json:"duration"`
	Position      *float64 `json:"position"`
	Volume        float64 `json:"volume"`
	Filename      *string `json:"filename"`
	Path          *string `json:"path"`
	MediaTitle    *string `json:"mediaTitle"`
	PlaylistPos   float64 `json:"playlistPos"`
	PlaylistCount float64 `json:"playlistCount"`
	SubScale      float64 `json:"subScale"`
	SubVisibility bool    `json:"subVisibility"`
	Loop          string  `json:"loop"`
	Fullscreen    bool    `json:"fullscreen"`
}

func strPtr(s string) *string {
	return &s
}

func floatPtr(f float64) *float64 {
	return &f
}

type PlayerStatus struct {
	mpv MPVPlayer
	log *logger.Logger

	mu         sync.Mutex
	lastStatus StatusInfo
	present    map[string]bool

	readyCh   chan struct{}
	closeOnce sync.Once
}

func NewPlayerStatus(mpvInst MPVPlayer, log *logger.Logger) *PlayerStatus {
	return &PlayerStatus{
		mpv: mpvInst,
		log: log,
		lastStatus: StatusInfo{
			Volume:        100,
			Loop:          "no",
			SubVisibility: true,
			SubScale:      1,
		},
		present: make(map[string]bool),
	}
}

func (s *PlayerStatus) GetSync() StatusInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastStatus
}

func (s *PlayerStatus) Play() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.log != nil {
		s.log.Debug("status: Play() called, resetting required keys")
	}

	for _, key := range StatusInfoRequiredKeys {
		delete(s.present, key)
	}

	s.lastStatus.Duration = 0
	s.lastStatus.Position = nil
	s.lastStatus.Filename = nil
	s.lastStatus.Path = nil
	s.lastStatus.MediaTitle = nil
	s.lastStatus.PlaylistPos = 0
	s.lastStatus.PlaylistCount = 0

	s.readyCh = make(chan struct{})
	s.closeOnce = sync.Once{}
}

func (s *PlayerStatus) Stop() {
	if s.log != nil {
		s.log.Debug("status: Stop() called")
	}

	s.mu.Lock()
	s.lastStatus.Path = nil
	s.lastStatus.Filename = nil
	ch := s.readyCh
	s.mu.Unlock()

	if ch != nil {
		s.closeOnce.Do(func() {
			close(ch)
		})
	}
}

func (s *PlayerStatus) Update(property string, value interface{}) {
	s.mu.Lock()

	camelProp := cases.ConvertKey(property, cases.Camel)

	switch camelProp {
	case "mute":
		if v, ok := value.(bool); ok {
			s.lastStatus.Mute = v
		}
	case "pause":
		if v, ok := value.(bool); ok {
			s.lastStatus.Pause = v
		}
	case "duration":
		s.lastStatus.Duration = toFloat64(value)
	case "position", "timePos":
		s.lastStatus.Position = floatPtr(toFloat64(value))
	case "volume":
		s.lastStatus.Volume = toFloat64(value)
	case "filename":
		if v, ok := value.(string); ok {
			s.lastStatus.Filename = strPtr(v)
		}
	case "path":
		if v, ok := value.(string); ok {
			s.lastStatus.Path = strPtr(v)
		}
	case "mediaTitle":
		if v, ok := value.(string); ok {
			s.lastStatus.MediaTitle = strPtr(v)
		}
	case "playlistPos":
		s.lastStatus.PlaylistPos = toFloat64(value)
	case "playlistCount":
		s.lastStatus.PlaylistCount = toFloat64(value)
	case "loop":
		if v, ok := value.(string); ok {
			s.lastStatus.Loop = v
		}
	case "subVisibility":
		if v, ok := value.(bool); ok {
			s.lastStatus.SubVisibility = v
		}
	case "subScale":
		s.lastStatus.SubScale = toFloat64(value)
	case "fullscreen":
		if v, ok := value.(bool); ok {
			s.lastStatus.Fullscreen = v
		}
	}

	s.present[camelProp] = true

	allPresent := true
	for _, key := range StatusInfoRequiredKeys {
		if !s.present[key] {
			allPresent = false
			break
		}
	}

	ch := s.readyCh
	allPresentBool := allPresent
	s.mu.Unlock()

	if allPresentBool && ch != nil {
		if s.log != nil {
			s.log.Debug("status: Update() all required keys present, resolving readyCh")
		}
		s.closeOnce.Do(func() {
			close(ch)
		})
	}
}

func (s *PlayerStatus) Get() (StatusInfo, error) {
	s.mu.Lock()
	ch := s.readyCh
	s.mu.Unlock()

	var status StatusInfo
	var path *string

	if ch != nil {
		select {
		case <-ch:
			if s.log != nil {
				s.log.Debug("status: Get() required keys resolved")
			}
			s.mu.Lock()
			status = s.lastStatus
			path = status.Path
			s.mu.Unlock()
		case <-time.After(5 * time.Second):
			s.mu.Lock()
			var missing []string
			for _, key := range StatusInfoRequiredKeys {
				if !s.present[key] {
					missing = append(missing, key)
				}
			}
			status = s.lastStatus
			path = status.Path
			s.mu.Unlock()
			if s.log != nil {
				s.log.Warnf("status: Get() timed out (5s), missing required keys: [%s]", strings.Join(missing, ", "))
			}
		}
	} else {
		s.mu.Lock()
		status = s.lastStatus
		path = status.Path
		s.mu.Unlock()
	}

	if path != nil {
		if s.mpv != nil && s.mpv.IsRunning() {
			pos, err := s.mpv.GetTimePosition()
			if err != nil {
				return status, fmt.Errorf("get time position: %w", err)
			}
			s.mu.Lock()
			s.lastStatus.Position = floatPtr(pos)
			status = s.lastStatus
			s.mu.Unlock()
		} else if s.mpv != nil {
			if s.log != nil {
				s.log.Warn("status: Get() mpv not running but path set, calling Stop()")
			}
			s.Stop()
		}
	}

	return status, nil
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}
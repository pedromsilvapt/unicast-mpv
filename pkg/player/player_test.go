package player

import (
	"testing"
	"time"

	"github.com/unicast/unicast-mpv/pkg/config"
)

type mockMPVPlayer struct {
	running    bool
	timePos    float64
	timePosErr error
}

func (m *mockMPVPlayer) IsRunning() bool {
	return m.running
}

func (m *mockMPVPlayer) GetTimePosition() (float64, error) {
	return m.timePos, m.timePosErr
}

func TestValueToMpv(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"bool true", true, "yes"},
		{"bool false", false, "no"},
		{"int", 42, "42"},
		{"int zero", 0, "0"},
		{"float", 3.14, "3.14"},
		{"string", "hello", "hello"},
		{"negative int", -1, "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValueToMpv(tt.input)
			if result != tt.expected {
				t.Errorf("ValueToMpv(%v): expected %q, got %q", tt.input, tt.expected, result)
			}
		})
	}
}

func TestPlayerStatus_GetSync(t *testing.T) {
	mock := &mockMPVPlayer{running: true}
	status := NewPlayerStatus(mock, nil)
	status.mu.Lock()
	status.lastStatus.Path = strPtr("/test/video.mp4")
	status.lastStatus.Filename = strPtr("video.mp4")
	status.mu.Unlock()

	result := status.GetSync()
	if *result.Path != "/test/video.mp4" {
		t.Errorf("expected Path=/test/video.mp4, got %v", result.Path)
	}
	if *result.Filename != "video.mp4" {
		t.Errorf("expected Filename=video.mp4, got %v", result.Filename)
	}
}

func TestPlayerStatus_PlayCreatesChannel(t *testing.T) {
	mock := &mockMPVPlayer{running: true}
	status := NewPlayerStatus(mock, nil)

	status.Play()

	status.mu.Lock()
	ch := status.readyCh
	status.mu.Unlock()

	if ch == nil {
		t.Error("expected readyCh to be created after Play()")
	}
}

func TestPlayerStatus_PlayClearsPresent(t *testing.T) {
	mock := &mockMPVPlayer{running: true}
	status := NewPlayerStatus(mock, nil)

	status.Update("duration", 120.0)
	status.Update("path", "/test.mp4")

	if !status.present["duration"] {
		t.Error("expected duration to be present before Play()")
	}

	status.Play()

	if status.present["duration"] {
		t.Error("expected duration to be cleared after Play()")
	}
	if status.present["path"] {
		t.Error("expected path to be cleared after Play()")
	}
}

func TestPlayerStatus_StopClearsPath(t *testing.T) {
	mock := &mockMPVPlayer{running: true}
	status := NewPlayerStatus(mock, nil)

	status.Update("path", "/test/video.mp4")
	status.Update("filename", "video.mp4")

	status.Stop()

	result := status.GetSync()
	if result.Path != nil {
		t.Errorf("expected nil Path after Stop(), got %v", result.Path)
	}
	if result.Filename != nil {
		t.Errorf("expected nil Filename after Stop(), got %v", result.Filename)
	}
}

func TestPlayerStatus_StopResolvesChannel(t *testing.T) {
	mock := &mockMPVPlayer{running: true}
	status := NewPlayerStatus(mock, nil)
	status.Play()

	ch := status.readyCh

	status.Stop()

	select {
	case <-ch:
	default:
		t.Error("expected Stop() to close the ready channel")
	}
}

func TestPlayerStatus_UpdateSetsFields(t *testing.T) {
	mock := &mockMPVPlayer{running: true}
	status := NewPlayerStatus(mock, nil)

	tests := []struct {
		property string
		value    interface{}
		check    func(s StatusInfo) bool
		desc     string
	}{
		{"mute", true, func(s StatusInfo) bool { return s.Mute }, "mute=true"},
		{"pause", true, func(s StatusInfo) bool { return s.Pause }, "pause=true"},
		{"duration", 120.5, func(s StatusInfo) bool { return s.Duration == 120.5 }, "duration=120.5"},
		{"volume", 75.0, func(s StatusInfo) bool { return s.Volume == 75.0 }, "volume=75"},
		{"filename", "test.mp4", func(s StatusInfo) bool { return s.Filename != nil && *s.Filename == "test.mp4" }, "filename=test.mp4"},
		{"path", "/path/to/test.mp4", func(s StatusInfo) bool { return s.Path != nil && *s.Path == "/path/to/test.mp4" }, "path"},
		{"media-title", "Test Title", func(s StatusInfo) bool { return s.MediaTitle != nil && *s.MediaTitle == "Test Title" }, "mediaTitle"},
		{"playlist-pos", 2.0, func(s StatusInfo) bool { return s.PlaylistPos == 2.0 }, "playlistPos=2"},
		{"playlist-count", 10.0, func(s StatusInfo) bool { return s.PlaylistCount == 10.0 }, "playlistCount=10"},
		{"loop", "inf", func(s StatusInfo) bool { return s.Loop == "inf" }, "loop=inf"},
		{"sub-visibility", true, func(s StatusInfo) bool { return s.SubVisibility }, "subVisibility=true"},
		{"sub-scale", 1.5, func(s StatusInfo) bool { return s.SubScale == 1.5 }, "subScale=1.5"},
		{"fullscreen", true, func(s StatusInfo) bool { return s.Fullscreen }, "fullscreen=true"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			status.Update(tt.property, tt.value)
			result := status.GetSync()
			if !tt.check(result) {
				t.Errorf("Update(%q, %v): check failed for %s", tt.property, tt.value, tt.desc)
			}
		})
	}
}

func TestPlayerStatus_UpdateConvertsKebabToCamel(t *testing.T) {
	mock := &mockMPVPlayer{running: true}
	status := NewPlayerStatus(mock, nil)

	status.Update("media-title", "Test Movie")
	result := status.GetSync()
	if result.MediaTitle == nil || *result.MediaTitle != "Test Movie" {
		t.Errorf("expected MediaTitle='Test Movie', got %v", result.MediaTitle)
	}

	status.Update("playlist-pos", 3.0)
	result = status.GetSync()
	if result.PlaylistPos != 3.0 {
		t.Errorf("expected PlaylistPos=3.0, got %f", result.PlaylistPos)
	}

	status.Update("sub-visibility", false)
	result = status.GetSync()
	if result.SubVisibility {
		t.Error("expected SubVisibility=false after update")
	}
}

func TestPlayerStatus_UpdateResolvesFutureWhenAllKeysPresent(t *testing.T) {
	mock := &mockMPVPlayer{running: true}
	status := NewPlayerStatus(mock, nil)
	status.Play()

	ch := status.readyCh

	status.Update("duration", 120.0)
	status.Update("position", 30.0)
	status.Update("filename", "test.mp4")
	status.Update("path", "/test.mp4")
	status.Update("media-title", "Test")
	status.Update("playlist-pos", 1.0)
	status.Update("playlist-count", 5.0)

	select {
	case <-ch:
	default:
		t.Error("expected readyCh to be closed after all required keys updated")
	}
}

func TestPlayerStatus_UpdateDoesNotResolveBeforeAllKeys(t *testing.T) {
	mock := &mockMPVPlayer{running: true}
	status := NewPlayerStatus(mock, nil)
	status.Play()

	ch := status.readyCh

	status.Update("duration", 120.0)
	status.Update("position", 30.0)
	status.Update("filename", "test.mp4")
	status.Update("path", "/test.mp4")
	status.Update("media-title", "Test")
	status.Update("playlist-pos", 1.0)

	select {
	case <-ch:
		t.Error("expected readyCh to NOT be closed before all required keys")
	default:
	}
}

func TestPlayerStatus_PlayResetsFuture(t *testing.T) {
	mock := &mockMPVPlayer{running: true}
	status := NewPlayerStatus(mock, nil)

	status.Play()
	status.Update("duration", 120.0)
	status.Update("position", 30.0)
	status.Update("filename", "test.mp4")
	status.Update("path", "/test.mp4")
	status.Update("media-title", "Test")
	status.Update("playlist-pos", 1.0)
	status.Update("playlist-count", 5.0)

	status.Play()

	status.mu.Lock()
	newCh := status.readyCh
	status.mu.Unlock()

	if newCh == nil {
		t.Error("expected new readyCh after Play()")
	}
}

func TestPlayerStatus_DefaultValues(t *testing.T) {
	mock := &mockMPVPlayer{running: true}
	status := NewPlayerStatus(mock, nil)
	result := status.GetSync()

	if result.Volume != 100 {
		t.Errorf("expected default Volume=100, got %f", result.Volume)
	}
	if result.Loop != "no" {
		t.Errorf("expected default Loop='no', got %q", result.Loop)
	}
	if !result.SubVisibility {
		t.Error("expected default SubVisibility=true")
	}
	if result.SubScale != 1 {
		t.Errorf("expected default SubScale=1, got %f", result.SubScale)
	}
}

func TestPlayerStatus_StopMultipleTimes(t *testing.T) {
	mock := &mockMPVPlayer{running: true}
	status := NewPlayerStatus(mock, nil)
	status.Play()

	status.Update("path", "/test.mp4")
	status.Stop()

	status.Stop()
}

func TestPlayerStatus_GetWithTimeout(t *testing.T) {
	mock := &mockMPVPlayer{running: true}
	status := NewPlayerStatus(mock, nil)
	status.Play()

	done := make(chan struct{})
	go func() {
		_, _ = status.Get()
		close(done)
	}()

	select {
	case <-done:
		t.Error("expected Get() to block until all keys present")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestPlayerStatus_GetReturnsAfterAllKeys(t *testing.T) {
	mock := &mockMPVPlayer{running: true, timePos: 30.5}
	status := NewPlayerStatus(mock, nil)
	status.Play()

	go func() {
		time.Sleep(50 * time.Millisecond)
		status.Update("duration", 120.0)
		status.Update("position", 30.0)
		status.Update("filename", "test.mp4")
		status.Update("path", "/test.mp4")
		status.Update("media-title", "Test")
		status.Update("playlist-pos", 1.0)
		status.Update("playlist-count", 5.0)
	}()

	result, err := status.Get()
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}
	if result.Path == nil || *result.Path != "/test.mp4" {
		t.Errorf("expected Path=/test.mp4, got %v", result.Path)
	}
	if result.Position == nil || *result.Position != 30.5 {
		t.Errorf("expected Position=30.5 (from GetTimePosition), got %v", result.Position)
	}
}

func TestPlayerStatus_GetTimeoutExpires(t *testing.T) {
	mock := &mockMPVPlayer{running: true}
	status := NewPlayerStatus(mock, nil)
	status.Play()

	status.Update("duration", 120.0)

	result, err := status.Get()
	if err != nil {
		t.Errorf("expected no error on timeout, got %v", err)
	}
	if result.Duration != 120.0 {
		t.Errorf("expected partial Duration=120.0, got %f", result.Duration)
	}
}

func TestPlayerStatus_GetWithoutPlay(t *testing.T) {
	mock := &mockMPVPlayer{running: true, timePos: 30.5}
	status := NewPlayerStatus(mock, nil)

	status.Update("path", "/test.mp4")

	result, err := status.Get()
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}
	if result.Path == nil || *result.Path != "/test.mp4" {
		t.Errorf("expected Path=/test.mp4, got %v", result.Path)
	}
	if result.Position == nil || *result.Position != 30.5 {
		t.Errorf("expected Position=30.5, got %v", result.Position)
	}
}

func TestPlayerStatus_GetNotRunningStops(t *testing.T) {
	mock := &mockMPVPlayer{running: false}
	status := NewPlayerStatus(mock, nil)
	status.Play()

	status.Update("duration", 120.0)
	status.Update("position", 30.0)
	status.Update("filename", "test.mp4")
	status.Update("path", "/test.mp4")
	status.Update("media-title", "Test")
	status.Update("playlist-pos", 1.0)
	status.Update("playlist-count", 5.0)

	_, err := status.Get()
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}

	result := status.GetSync()
	if result.Path != nil {
		t.Errorf("expected nil Path after Stop (mpv not running), got %v", result.Path)
	}
}

func TestBuildArgs_BasicArgs(t *testing.T) {
	cfg := config.NewConfig(map[string]interface{}{})
	args := BuildMPVArgs(cfg)

	found := make(map[string]bool)
	for _, a := range args {
		found[a] = true
	}

	if !found["--player-operation-mode=pseudo-gui"] {
		t.Error("expected --player-operation-mode=pseudo-gui in args")
	}
	if !found["--force-window"] {
		t.Error("expected --force-window in args")
	}
	if !found["--terminal"] {
		t.Error("expected --terminal in args")
	}
	if !found["--idle=once"] {
		t.Error("expected --idle=once in args (default quitOnStop=true)")
	}
}

func TestBuildArgs_QuitOnStopTrue(t *testing.T) {
	cfg := config.NewConfig(map[string]interface{}{
		"quitOnStop": true,
	})
	args := BuildMPVArgs(cfg)

	found := make(map[string]bool)
	for _, a := range args {
		found[a] = true
	}

	if !found["--idle=once"] {
		t.Error("expected --idle=once when quitOnStop=true")
	}
	if found["--idle=yes"] {
		t.Error("did not expect --idle=yes when quitOnStop=true")
	}
}

func TestBuildArgs_QuitOnStopFalse(t *testing.T) {
	cfg := config.NewConfig(map[string]interface{}{
		"quitOnStop": false,
	})
	args := BuildMPVArgs(cfg)

	found := make(map[string]bool)
	for _, a := range args {
		found[a] = true
	}

	if !found["--idle=yes"] {
		t.Error("expected --idle=yes when quitOnStop=false")
	}
	if found["--idle=once"] {
		t.Error("did not expect --idle=once when quitOnStop=false")
	}
}

func TestBuildArgs_Monitor(t *testing.T) {
	cfg := config.NewConfig(map[string]interface{}{
		"monitor": 1,
	})
	args := BuildMPVArgs(cfg)

	found := make(map[string]bool)
	for _, a := range args {
		found[a] = true
	}

	if !found["--screen=1"] {
		t.Error("expected --screen=1 in args")
	}
	if !found["--fs-screen=1"] {
		t.Error("expected --fs-screen=1 in args")
	}
}

func TestBuildArgs_NoMonitor(t *testing.T) {
	cfg := config.NewConfig(map[string]interface{}{})
	args := BuildMPVArgs(cfg)

	for _, a := range args {
		if a == "--screen=0" || a == "--fs-screen=0" {
			t.Errorf("expected no screen args when monitor not set, got %q", a)
		}
	}
}

func TestBuildArgs_OnTop(t *testing.T) {
	cfg := config.NewConfig(map[string]interface{}{
		"onTop": true,
	})
	args := BuildMPVArgs(cfg)

	found := false
	for _, a := range args {
		if a == "--ontop" {
			found = true
		}
	}
	if !found {
		t.Error("expected --ontop in args when onTop=true")
	}
}

func TestBuildArgs_Fullscreen(t *testing.T) {
	cfg := config.NewConfig(map[string]interface{}{
		"fullscreen": true,
	})
	args := BuildMPVArgs(cfg)

	found := false
	for _, a := range args {
		if a == "--fs" {
			found = true
		}
	}
	if !found {
		t.Error("expected --fs in args when fullscreen=true")
	}
}

func TestBuildArgs_VideoOutput(t *testing.T) {
	cfg := config.NewConfig(map[string]interface{}{
		"videoOutput": "gpu",
	})
	args := BuildMPVArgs(cfg)

	found := false
	for _, a := range args {
		if a == "--vo=gpu" {
			found = true
		}
	}
	if !found {
		t.Error("expected --vo=gpu in args")
	}
}

func TestBuildArgs_AudioOutput(t *testing.T) {
	cfg := config.NewConfig(map[string]interface{}{
		"audioOutput": "alsa",
	})
	args := BuildMPVArgs(cfg)

	found := false
	for _, a := range args {
		if a == "--ao=alsa" {
			found = true
		}
	}
	if !found {
		t.Error("expected --ao=alsa in args")
	}
}

func TestBuildArgs_AudioDevice(t *testing.T) {
	cfg := config.NewConfig(map[string]interface{}{
		"audioDevice": "hw:0,0",
	})
	args := BuildMPVArgs(cfg)

	found := false
	for _, a := range args {
		if a == "--audio-device=hw:0,0" {
			found = true
		}
	}
	if !found {
		t.Error("expected --audio-device=hw:0,0 in args")
	}
}

func TestBuildArgs_CustomArgs(t *testing.T) {
	cfg := config.NewConfig(map[string]interface{}{
		"args": []interface{}{"--no-border", "--profile=gpu-hq"},
	})
	args := BuildMPVArgs(cfg)

	foundNoBorder := false
	foundProfile := false
	for _, a := range args {
		if a == "--no-border" {
			foundNoBorder = true
		}
		if a == "--profile=gpu-hq" {
			foundProfile = true
		}
	}
	if !foundNoBorder {
		t.Error("expected --no-border in custom args")
	}
	if !foundProfile {
		t.Error("expected --profile=gpu-hq in custom args")
	}
}

func TestBuildArgs_SubtitlesConfig(t *testing.T) {
	cfg := config.NewConfig(map[string]interface{}{
		"subtitles": map[string]interface{}{
			"font":  "Droid Sans",
			"color": "#FFFFFF",
			"bold":  true,
		},
	})
	args := BuildMPVArgs(cfg)

	found := make(map[string]bool)
	for _, a := range args {
		found[a] = true
	}

	if !found["--sub-font=Droid Sans"] {
		t.Error("expected --sub-font=Droid Sans in args")
	}
	if !found["--sub-color=#FFFFFF"] {
		t.Error("expected --sub-color=#FFFFFF in args")
	}
	if !found["--sub-bold=yes"] {
		t.Error("expected --sub-bold=yes in args")
	}
}

func TestBuildArgs_SubtitlesNilValue(t *testing.T) {
	cfg := config.NewConfig(map[string]interface{}{
		"subtitles": map[string]interface{}{
			"font":       "Droid Sans",
			"borderSize": nil,
		},
	})
	args := BuildMPVArgs(cfg)

	for _, a := range args {
		if a == "--sub-border-size" || a == "--sub-border-size=" {
			t.Errorf("should not include sub-border-size when value is nil, got %q", a)
		}
	}
}

func TestBuildArgs_NoSubtitles(t *testing.T) {
	cfg := config.NewConfig(map[string]interface{}{})
	args := BuildMPVArgs(cfg)

	for _, a := range args {
		if len(a) > 5 && a[:5] == "--sub" {
			t.Errorf("expected no subtitle args when no subtitles config, got %q", a)
		}
	}
}

func TestBuildArgs_MonitorNegative(t *testing.T) {
	cfg := config.NewConfig(map[string]interface{}{
		"monitor": -1,
	})
	args := BuildMPVArgs(cfg)

	for _, a := range args {
		if a == "--screen=-1" || a == "--fs-screen=-1" {
			t.Errorf("should not include screen args for negative monitor, got %q", a)
		}
	}
}
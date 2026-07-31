package music

import "testing"

func TestApplyDefaultsAddsPlaybackControlKeywords(t *testing.T) {
	var cfg MusicConfig
	cfg.ApplyDefaults()

	assertContains(t, cfg.Commands.NextKeywords, "下一首")
	assertContains(t, cfg.Commands.PreviousKeywords, "上一首")
	assertContains(t, cfg.Commands.RepeatOneKeywords, "单曲循环")
	assertContains(t, cfg.Commands.RepeatAllKeywords, "全部循环")
	assertContains(t, cfg.Commands.ShuffleModeKeywords, "随机播放")
}

func TestApplyDefaultsAddsLXDefaults(t *testing.T) {
	var cfg MusicConfig
	cfg.ApplyDefaults()

	if cfg.LX.Source != "kw" {
		t.Fatalf("expected default LX source kw, got %q", cfg.LX.Source)
	}
	if cfg.LX.Quality != "128k" {
		t.Fatalf("expected default LX quality 128k, got %q", cfg.LX.Quality)
	}
	if cfg.LX.TimeoutSec != 10 {
		t.Fatalf("expected default LX timeout 10, got %d", cfg.LX.TimeoutSec)
	}
	if cfg.LX.Download {
		t.Fatal("expected LX download to be disabled by default")
	}
}

func TestApplyDefaultsAddsPlayerDiagnosticDefaults(t *testing.T) {
	var cfg MusicConfig
	cfg.ApplyDefaults()

	if cfg.Player.DiagnosticIntervalSec == nil || *cfg.Player.DiagnosticIntervalSec != 15 {
		t.Fatalf("expected default diagnostic interval 15, got %+v", cfg.Player.DiagnosticIntervalSec)
	}
}

func TestApplyDefaultsAddsPlayerWatchdogDefaults(t *testing.T) {
	var cfg MusicConfig
	cfg.ApplyDefaults()

	if cfg.Player.WatchdogEnabled == nil || !*cfg.Player.WatchdogEnabled {
		t.Fatalf("expected watchdog enabled by default, got %+v", cfg.Player.WatchdogEnabled)
	}
	if cfg.Player.PreemptMarginSec == nil || *cfg.Player.PreemptMarginSec != 2 {
		t.Fatalf("expected default preempt margin 2s, got %+v", cfg.Player.PreemptMarginSec)
	}
}

func assertContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, v := range values {
		if v == want {
			return
		}
	}
	t.Fatalf("expected %q in %v", want, values)
}

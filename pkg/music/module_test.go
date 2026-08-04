package music

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeLXResolver struct {
	keyword      string
	downloadPath string
	track        *LXTrack
	err          error
}

type orderedCalls struct {
	calls []string
}

func (o *orderedCalls) add(name string) {
	o.calls = append(o.calls, name)
}

func (o *orderedCalls) index(name string) int {
	for i, v := range o.calls {
		if v == name {
			return i
		}
	}
	return -1
}

func (f *fakeLXResolver) Resolve(ctx context.Context, keyword string) (*LXTrack, error) {
	f.keyword = keyword
	return f.track, f.err
}

func (f *fakeLXResolver) Download(ctx context.Context, track *LXTrack, targetPath string) error {
	f.downloadPath = targetPath
	return os.WriteFile(targetPath, []byte("fake mp3"), 0644)
}

func TestHandlePlayFallsBackToLXWhenLocalSearchMisses(t *testing.T) {
	abort := false
	cfg := &MusicConfig{
		Enabled: true,
		LX: LXConfig{
			Enabled: true,
		},
		Commands: CommandsConfig{
			AbortXiaoAIOnPlay: &abort,
		},
	}
	cfg.ApplyDefaults()

	idx := NewIndexer(cfg)
	played := []string{}
	spoken := []string{}
	player := NewPlayer(nil, idx)
	player.playURL = func(url string) error {
		played = append(played, url)
		return nil
	}
	player.speak = func(text string) error {
		spoken = append(spoken, text)
		return nil
	}
	resolver := &fakeLXResolver{
		track: &LXTrack{
			Name:   "晴天",
			Singer: "周杰伦",
			Source: "kw",
			URL:    "https://lx.example/qingtian.mp3",
		},
	}
	module := &Module{
		config:  cfg,
		indexer: idx,
		player:  player,
		lx:      resolver,
	}

	if !module.handlePlay("周杰伦晴天") {
		t.Fatal("expected handlePlay to handle LX fallback")
	}
	if resolver.keyword != "周杰伦晴天" {
		t.Fatalf("expected LX search keyword, got %q", resolver.keyword)
	}
	if len(played) != 1 || played[0] != "https://lx.example/qingtian.mp3" {
		t.Fatalf("expected LX URL to be played, got %v", played)
	}
	if len(spoken) != 1 || spoken[0] != "好的，找到在线歌曲晴天" {
		t.Fatalf("unexpected spoken feedback: %v", spoken)
	}
}

func TestHandlePlayDownloadsLXTrackWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	abort := false
	cfg := &MusicConfig{
		Enabled: true,
		Dirs:    []string{dir},
		LX: LXConfig{
			Enabled:  true,
			Download: true,
		},
		HTTP: HTTPConfig{
			Port:    18080,
			BaseURL: "http://music.local",
		},
		Commands: CommandsConfig{
			AbortXiaoAIOnPlay: &abort,
		},
	}
	cfg.ApplyDefaults()

	idx := NewIndexer(cfg)
	fileSrv := NewFileServer(&cfg.HTTP)
	played := []string{}
	player := NewPlayer(fileSrv, idx)
	player.playURL = func(url string) error {
		played = append(played, url)
		return nil
	}
	player.speak = func(text string) error {
		return nil
	}
	resolver := &fakeLXResolver{
		track: &LXTrack{
			Name:   "晴天",
			Singer: "周杰伦",
			Source: "kw",
			URL:    "https://lx.example/qingtian.mp3",
		},
	}
	module := &Module{
		config:  cfg,
		indexer: idx,
		fileSrv: fileSrv,
		player:  player,
		lx:      resolver,
	}

	if !module.handlePlay("周杰伦晴天") {
		t.Fatal("expected handlePlay to handle LX fallback")
	}
	wantPath := filepath.Join(dir, "晴天 - 周杰伦.mp3")
	if resolver.downloadPath != wantPath {
		t.Fatalf("expected download path %q, got %q", wantPath, resolver.downloadPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected downloaded file: %v", err)
	}
	if len(played) != 1 || !strings.HasPrefix(played[0], "http://music.local/file/") {
		t.Fatalf("expected local file URL to be played, got %v", played)
	}
}

// TestHandlePlaySetsUpEpisodeContinuationAcrossBatches 覆盖"20 集播完接着播下一批"这个场景：
// 曲库里有 25 集，max_results=20，第一次 handlePlay 应该只入队前 20 集（1~20），
// 同时设置好续播回调；手动调用这个回调应该能拉到剩下的 21~25 集。
func TestHandlePlaySetsUpEpisodeContinuationAcrossBatches(t *testing.T) {
	abort := false
	cfg := &MusicConfig{
		Enabled:  true,
		Dirs:     []string{"/gushi"},
		Search:   SearchConfig{MaxResults: 20},
		Commands: CommandsConfig{AbortXiaoAIOnPlay: &abort},
	}
	cfg.ApplyDefaults()

	idx := NewIndexer(cfg)
	songs := make([]IndexedSong, 0, 25)
	for ep := 1; ep <= 25; ep++ {
		songs = append(songs, IndexedSong{
			Path:      filepath.Join("/gushi", fmt.Sprintf("%03d.mp3", ep)),
			NameLower: fmt.Sprintf("测试故事%03d", ep),
			Episode:   ep,
		})
	}
	idx.songs = songs

	fileSrv := NewFileServer(&HTTPConfig{Port: 18080, BaseURL: "http://music.local"})
	played := []string{}
	player := NewPlayer(fileSrv, idx)
	player.playURL = func(url string) error {
		played = append(played, url)
		return nil
	}
	player.speak = func(text string) error { return nil }

	module := &Module{
		config:  cfg,
		indexer: idx,
		fileSrv: fileSrv,
		player:  player,
	}

	if !module.handlePlay("故事测试故事第1集") {
		t.Fatal("expected handlePlay to succeed")
	}
	if len(played) != 1 {
		t.Fatalf("expected first episode to start playing, played=%v", played)
	}
	if len(player.queue) != 19 { // 20 首入队，第一首已经出队播放，剩 19 首
		t.Fatalf("expected 19 remaining items in queue after first play, got %d", len(player.queue))
	}

	if player.onExhausted == nil {
		t.Fatal("expected episode continuation handler to be set")
	}
	more := player.onExhausted()
	if len(more) != 5 {
		t.Fatalf("expected continuation to fetch remaining 5 episodes (21-25), got %d: %+v", len(more), more)
	}

	// 续播用完之后（没有第 26 集了），再调用一次应该返回空，不会死循环重复拉同一批。
	again := player.onExhausted()
	if len(again) != 0 {
		t.Fatalf("expected no more episodes after exhausting all 25, got %d", len(again))
	}
}

// TestHandlePlayEpisodeAbortsBeforeSpeak 确认故事/按集播放场景下“先 Abort，再播报”：
// 目的：避免云端并发语音反馈和本地反馈语重叠（双声道）。
func TestHandlePlayEpisodeAbortsBeforeSpeak(t *testing.T) {
	abort := true
	cfg := &MusicConfig{
		Enabled:  true,
		Dirs:     []string{"/gushi"},
		Search:   SearchConfig{MaxResults: 5},
		Commands: CommandsConfig{AbortXiaoAIOnPlay: &abort},
	}
	cfg.ApplyDefaults()

	idx := NewIndexer(cfg)
	idx.songs = []IndexedSong{{Path: "/gushi/001.mp3", NameLower: "测试故事001", Episode: 1}}
	fileSrv := NewFileServer(&HTTPConfig{Port: 18080, BaseURL: "http://music.local"})
	player := NewPlayer(fileSrv, idx)
	order := &orderedCalls{}
	player.speak = func(text string) error {
		order.add("speak")
		return nil
	}
	player.abortXiaoAI = func() error {
		order.add("abort")
		return nil
	}
	player.playURL = func(url string) error {
		order.add("play")
		return nil
	}

	module := &Module{config: cfg, indexer: idx, fileSrv: fileSrv, player: player}
	if !module.handlePlay("故事测试故事第1集") {
		t.Fatal("expected handlePlay to succeed")
	}

	is := order.index("speak")
	ia := order.index("abort")
	ip := order.index("play")
	if is == -1 || ia == -1 || ip == -1 {
		t.Fatalf("expected speak/abort/play all called, got order=%v", order.calls)
	}
	if !(ia < is && is < ip) {
		t.Fatalf("expected order abort -> speak -> play, got %v", order.calls)
	}
}

// TestHandlePlayNonEpisodeSpeaksBeforeAbortXiaoAI 普通音乐场景保留“先播报，再 Abort”：
// 避免某些设备上刚重启 mico_aivs_lab 后 tts_play.sh 短窗口不稳定。
func TestHandlePlayNonEpisodeSpeaksBeforeAbortXiaoAI(t *testing.T) {
	abort := true
	cfg := &MusicConfig{
		Enabled:  true,
		Dirs:     []string{"/music"},
		Search:   SearchConfig{MaxResults: 5},
		Commands: CommandsConfig{AbortXiaoAIOnPlay: &abort},
	}
	cfg.ApplyDefaults()

	idx := NewIndexer(cfg)
	idx.songs = []IndexedSong{{Path: "/music/001.mp3", NameLower: "稻香", Episode: 0}}
	fileSrv := NewFileServer(&HTTPConfig{Port: 18080, BaseURL: "http://music.local"})
	player := NewPlayer(fileSrv, idx)
	order := &orderedCalls{}
	player.speak = func(text string) error {
		order.add("speak")
		return nil
	}
	player.abortXiaoAI = func() error {
		order.add("abort")
		return nil
	}
	player.playURL = func(url string) error {
		order.add("play")
		return nil
	}

	module := &Module{config: cfg, indexer: idx, fileSrv: fileSrv, player: player}
	if !module.handlePlay("稻香") {
		t.Fatal("expected handlePlay to succeed")
	}

	is := order.index("speak")
	ia := order.index("abort")
	ip := order.index("play")
	if is == -1 || ia == -1 || ip == -1 {
		t.Fatalf("expected speak/abort/play all called, got order=%v", order.calls)
	}
	if !(is < ia && ia < ip) {
		t.Fatalf("expected order speak -> abort -> play, got %v", order.calls)
	}
}

func TestHandleStoryContextSetAndQuery(t *testing.T) {
	abort := false
	cfg := &MusicConfig{Enabled: true, Commands: CommandsConfig{AbortXiaoAIOnPlay: &abort}}
	cfg.ApplyDefaults()
	idx := NewIndexer(cfg)
	player := NewPlayer(nil, idx)
	spoken := []string{}
	player.speak = func(text string) error {
		spoken = append(spoken, text)
		return nil
	}
	module := &Module{config: cfg, indexer: idx, player: player}

	module.handleStoryContextCommand(StoryContextCommand{Set: true, SeriesName: "三国演义第一季"})
	module.handleStoryContextCommand(StoryContextCommand{Query: true})

	if module.getDefaultStorySeries() != "三国演义第一季" {
		t.Fatalf("expected default story to be set")
	}
	if len(spoken) != 2 {
		t.Fatalf("expected 2 speak calls, got %d", len(spoken))
	}
	if spoken[0] != "好的，当前默认故事是三国演义第一季" {
		t.Fatalf("unexpected set feedback: %q", spoken[0])
	}
	if spoken[1] != "当前默认故事是三国演义第一季" {
		t.Fatalf("unexpected query feedback: %q", spoken[1])
	}
}

func TestHandlePlayEpisodeUsesDefaultStorySeries(t *testing.T) {
	abort := false
	cfg := &MusicConfig{
		Enabled:  true,
		Dirs:     []string{"/gushi"},
		Search:   SearchConfig{MaxResults: 20},
		Commands: CommandsConfig{AbortXiaoAIOnPlay: &abort},
	}
	cfg.ApplyDefaults()

	idx := NewIndexer(cfg)
	idx.songs = []IndexedSong{
		{Path: "/gushi/004.mp3", NameLower: "三国演义第一季004", Episode: 4},
		{Path: "/gushi/005.mp3", NameLower: "三国演义第一季005", Episode: 5},
	}
	fileSrv := NewFileServer(&HTTPConfig{Port: 18080, BaseURL: "http://music.local"})
	played := []string{}
	player := NewPlayer(fileSrv, idx)
	player.playURL = func(url string) error {
		played = append(played, url)
		return nil
	}
	player.speak = func(text string) error { return nil }

	module := &Module{config: cfg, indexer: idx, fileSrv: fileSrv, player: player}
	module.setDefaultStorySeries("三国演义第一季")

	if !module.handlePlay("第4集") {
		t.Fatal("expected handlePlay to use default story")
	}
	if len(played) != 1 {
		t.Fatalf("expected first matching episode to play, got %v", played)
	}
}

func TestHandleRandomPlayPrefersSongsOutsideStoryDirs(t *testing.T) {
	abort := false
	cfg := &MusicConfig{
		Enabled: true,
		Dirs:    []string{"/music", "/gushi"},
		Search:  SearchConfig{MaxResults: 10},
		Commands: CommandsConfig{
			AbortXiaoAIOnPlay: &abort,
		},
		Stories: []StoryConfig{{Name: "三国演义第一季", Dir: "/gushi/三国演义第1季"}},
	}
	cfg.ApplyDefaults()

	idx := NewIndexer(cfg)
	idx.songs = []IndexedSong{
		{Path: "/gushi/三国演义第1季/001.mp3", NameLower: "三国001", Episode: 1},
		{Path: "/gushi/三国演义第1季/002.mp3", NameLower: "三国002", Episode: 2},
		{Path: "/music/pop/a.mp3", NameLower: "songa", Episode: 0},
		{Path: "/music/pop/b.mp3", NameLower: "songb", Episode: 0},
	}
	fileSrv := NewFileServer(&HTTPConfig{Port: 18080, BaseURL: "http://music.local"})
	player := NewPlayer(fileSrv, idx)
	player.speak = func(text string) error { return nil }
	played := []string{}
	player.playURL = func(url string) error {
		played = append(played, url)
		return nil
	}

	module := &Module{config: cfg, indexer: idx, fileSrv: fileSrv, player: player}
	module.setDefaultStorySeries("三国演义第一季")

	if !module.handleRandomPlay("随便听听") {
		t.Fatal("expected random play to be handled")
	}
	if len(played) != 1 {
		t.Fatalf("expected one item to start playing, got %v", played)
	}
	for _, item := range player.playlist {
		if strings.HasPrefix(item.Path, "/gushi/三国演义第1季/") {
			t.Fatalf("expected random playlist to exclude story dir, got %+v", player.playlist)
		}
	}
}

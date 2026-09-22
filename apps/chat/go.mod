module github.com/cxjava/open-xiaoai/apps/chat

go 1.27

require (
	github.com/coder/websocket v1.8.15
	github.com/cxjava/open-xiaoai/apps/client v0.0.0
	github.com/cxjava/open-xiaoai/pkg/music v0.0.0
	github.com/emiago/diago v0.32.2
	github.com/emiago/sipgo v1.4.3
	github.com/sashabaranov/go-openai v1.42.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/bogem/id3v2/v2 v2.1.4 // indirect
	github.com/cxjava/open-xiaoai/pkg/lx-go v0.0.0-20260819065646-c0934c5f6c9c // indirect
	github.com/dhowden/tag v0.0.0-20240417053706-3d75831295e8 // indirect
	github.com/dlclark/regexp2/v2 v2.7.1 // indirect
	github.com/dop251/base64dec v0.0.0-20231022112746-c6c9f9a96217 // indirect
	github.com/dop251/goja v0.0.0-20260806115107-493f22071ef6 // indirect
	github.com/dop251/goja_nodejs v0.0.0-20260212111938-1f56ff5bcf14 // indirect
	github.com/emiago/dtls/v3 v3.0.0-20260122183559-8b8d23e359c0 // indirect
	github.com/go-audio/riff v1.0.0 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/google/pprof v0.0.0-20260802141513-ef3492d7dac3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/icholy/digest v1.1.0 // indirect
	github.com/pion/logging v0.2.4 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.15 // indirect
	github.com/pion/rtp v1.8.18 // indirect
	github.com/pion/srtp/v3 v3.0.6 // indirect
	github.com/pion/transport/v3 v3.1.1 // indirect
	github.com/pion/transport/v4 v4.0.1 // indirect
	github.com/zaf/g711 v1.4.0 // indirect
	golang.org/x/crypto v0.32.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	gopkg.in/hraban/opus.v2 v2.0.0-20230925203106-0188a62cb302 // indirect
)

replace (
	github.com/cxjava/open-xiaoai/apps/client => ../client
	github.com/cxjava/open-xiaoai/pkg/lx-go => ../../pkg/lx-go
	github.com/cxjava/open-xiaoai/pkg/music => ../../pkg/music
)

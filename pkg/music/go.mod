module github.com/cxjava/open-xiaoai/pkg/music

go 1.26.4

require (
	github.com/cxjava/open-xiaoai/apps/client v0.0.0
	github.com/cxjava/open-xiaoai/pkg/lx-go v0.0.0-00010101000000-000000000000
	github.com/dhowden/tag v0.0.0-20240417053706-3d75831295e8
	golang.org/x/sync v0.22.0
)

require (
	github.com/bogem/id3v2/v2 v2.1.4 // indirect
	github.com/coder/websocket v1.8.14 // indirect
	github.com/dlclark/regexp2/v2 v2.6.0 // indirect
	github.com/dop251/base64dec v0.0.0-20231022112746-c6c9f9a96217 // indirect
	github.com/dop251/goja v0.0.0-20260806115107-493f22071ef6 // indirect
	github.com/dop251/goja_nodejs v0.0.0-20260212111938-1f56ff5bcf14 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20260802141513-ef3492d7dac3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

replace github.com/cxjava/open-xiaoai/apps/client => ../../apps/client

replace github.com/cxjava/open-xiaoai/pkg/lx-go => ../lx-go

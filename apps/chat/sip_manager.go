package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emiago/diago"
	diagoaudio "github.com/emiago/diago/audio"
	"github.com/emiago/diago/media"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"

	clientaudio "github.com/cxjava/open-xiaoai/apps/client/services/audio"
	"github.com/cxjava/open-xiaoai/apps/client/services/connect"
)

type SIPManager struct {
	mu        sync.Mutex
	cfg       func() SIPConfig
	ua        *sipgo.UserAgent
	diago     *diago.Diago
	call      *sipCall
	callEnded func()
}

type sipCall struct {
	manager *SIPManager
	dialog  *diago.DialogClientSession
	media   *diago.DialogMedia

	pcmWriter *diagoaudio.PCMEncoderWriter

	downsampler pcmDownsampler
	once        sync.Once
}

func NewSIPManager(cfg func() SIPConfig) (*SIPManager, error) {
	c := cfg()
	if c.BindHost == "" {
		c.BindHost = "0.0.0.0"
	}
	if c.BindPort == 0 {
		c.BindPort = 5062
	}

	ua, err := sipgo.NewUA(
		sipgo.WithUserAgent("open-xiaoai-sip/0.1"),
		sipgo.WithUserAgentHostname(normalizeBindHost(c.BindHost)),
	)
	if err != nil {
		return nil, fmt.Errorf("create SIP user agent: %w", err)
	}

	dg := diago.NewDiago(
		ua,
		diago.WithTransport(diago.Transport{
			ID:        "xiaoai-udp",
			Transport: "udp",
			BindHost:  c.BindHost,
			BindPort:  c.BindPort,
		}),
		diago.WithMediaConfig(diago.MediaConfig{
			Codecs: []media.Codec{
				media.CodecAudioAlaw,
				media.CodecAudioUlaw,
				media.CodecTelephoneEvent8000,
			},
		}),
	)

	return &SIPManager{
		cfg:   cfg,
		ua:    ua,
		diago: dg,
	}, nil
}

func normalizeBindHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" || host == "0.0.0.0" || host == "::" {
		return "localhost"
	}
	return host
}

func (m *SIPManager) SetCallEnded(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callEnded = fn
}

func (m *SIPManager) Dial(route SIPRoute) error {
	m.mu.Lock()
	if m.call != nil {
		m.mu.Unlock()
		return fmt.Errorf("SIP call already active")
	}
	m.mu.Unlock()

	target, transport, err := parseSIPURI(route.URI)
	if err != nil {
		return err
	}

	cfg := m.cfg()
	timeout := time.Duration(cfg.CallTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	opts := diago.InviteOptions{
		Username: cfg.Username,
		Password: cfg.Password,
	}
	if cfg.Username != "" {
		opts.OnResponse = func(res *sip.Response) error {
			log.Printf("☎️ SIP %s -> %s", res.StatusCode, route.URI)
			return nil
		}
	}

	clientOpts := diago.InviteOptions{
		Username:  opts.Username,
		Password:  opts.Password,
		OnResponse: opts.OnResponse,
	}
	if cfg.Username != "" {
		callerHost := cfg.BindHost
		if callerHost == "" || callerHost == "0.0.0.0" || callerHost == "::" {
			callerHost = "127.0.0.1"
		}
		clientOpts.WithCaller(cfg.CallerName, cfg.Username, callerHost)
	}

	dialog, med, err := m.diago.Invite(ctx, target, diago.InviteOptions{
		Transport:  transport,
		Username:   clientOpts.Username,
		Password:   clientOpts.Password,
		Headers:    clientOpts.Headers,
		OnResponse: clientOpts.OnResponse,
	})
	if err != nil {
		return fmt.Errorf("SIP INVITE %s failed: %w", route.URI, err)
	}

	call := &sipCall{
		manager: m,
		dialog:  dialog,
		media:   med,
	}

	if err := call.prepareMedia(); err != nil {
		_ = call.end(true)
		return fmt.Errorf("prepare SIP audio: %w", err)
	}

	m.mu.Lock()
	if m.call != nil {
		m.mu.Unlock()
		_ = call.end(true)
		return fmt.Errorf("SIP call already active")
	}
	m.call = call
	m.mu.Unlock()

	if err := call.startClientAudio(); err != nil {
		m.clearCall(call)
		_ = call.end(true)
		return fmt.Errorf("start XiaoAI audio: %w", err)
	}

	go call.forwardRemoteAudio()
	go func() {
		<-dialog.Context().Done()
		_ = call.end(false)
		m.clearCall(call)
	}()

	log.Printf("☎️ SIP 已建立: %s -> %s", route.Contact, route.URI)
	return nil
}

func (m *SIPManager) Hangup() error {
	m.mu.Lock()
	call := m.call
	m.mu.Unlock()
	if call == nil {
		return nil
	}
	err := call.end(true)
	m.clearCall(call)
	return err
}

func (m *SIPManager) WriteSpeakerPCM(data []byte) error {
	m.mu.Lock()
	call := m.call
	m.mu.Unlock()
	if call == nil {
		return nil
	}
	return call.writeSpeakerPCM(data)
}

func (m *SIPManager) clearCall(call *sipCall) {
	m.mu.Lock()
	if m.call == call {
		m.call = nil
	}
	cb := m.callEnded
	m.mu.Unlock()
	if cb != nil {
		cb()
	}
}

func (m *SIPManager) Close() error {
	_ = m.Hangup()
	if m.ua != nil {
		return m.ua.Close()
	}
	return nil
}

func (c *sipCall) prepareMedia() error {
	readerProps := diago.MediaProps{}
	encodedReader, err := c.media.AudioReader(diago.WithAudioReaderMediaProps(&readerProps))
	if err != nil {
		return err
	}
	if readerProps.Codec.Name != "PCMA" && readerProps.Codec.Name != "PCMU" {
		return fmt.Errorf("unsupported negotiated codec: %s", readerProps.Codec.Name)
	}

	writerProps := diago.MediaProps{}
	encodedWriter, err := c.media.AudioWriter(diago.WithAudioWriterMediaProps(&writerProps))
	if err != nil {
		return err
	}
	if writerProps.Codec.Name != "PCMA" && writerProps.Codec.Name != "PCMU" {
		return fmt.Errorf("unsupported negotiated codec: %s", writerProps.Codec.Name)
	}

	decoder := &diagoaudio.PCMDecoderReader{}
	if err := decoder.Init(readerProps.Codec, encodedReader); err != nil {
		return err
	}

	encoder := &diagoaudio.PCMEncoderWriter{}
	if err := encoder.Init(writerProps.Codec, encodedWriter); err != nil {
		return err
	}

	c.pcmWriter = encoder

	c.dialog.OnState(func(state sip.DialogState) {
		log.Printf("☎️ SIP 状态: %s", state)
	})

	return nil
}

func (c *sipCall) startClientAudio() error {
	cfg := clientaudio.AudioConfig{
		PCM:           "noop",
		Channels:      1,
		BitsPerSample: 16,
		SampleRate:    16000,
		PeriodSize:    320,
		BufferSize:    1280,
	}

	_, err := connect.GetRPC().CallRemote("start_play", cfg, ptrUint64(5000))
	if err != nil {
		return fmt.Errorf("start_play: %w", err)
	}

	if _, err := connect.GetRPC().CallRemote("start_recording", cfg, ptrUint64(5000)); err != nil {
		_, _ = connect.GetRPC().CallRemote("stop_play", nil, ptrUint64(3000))
		return fmt.Errorf("start_recording: %w", err)
	}
	return nil
}

func (c *sipCall) stopClientAudio() {
	_, _ = connect.GetRPC().CallRemote("stop_recording", nil, ptrUint64(3000))
	_, _ = connect.GetRPC().CallRemote("stop_play", nil, ptrUint64(3000))
}

func (c *sipCall) writeSpeakerPCM(data []byte) error {
	if c.pcmWriter == nil {
		return fmt.Errorf("SIP PCM writer is not ready")
	}
	pcm8 := c.downsampler.Down16To8(data)
	if len(pcm8) == 0 {
		return nil
	}
	_, err := c.pcmWriter.Write(pcm8)
	return err
}

func (c *sipCall) forwardRemoteAudio() {
	if c.media == nil {
		return
	}

	readerProps := diago.MediaProps{}
	encodedReader, err := c.media.AudioReader(diago.WithAudioReaderMediaProps(&readerProps))
	if err != nil {
		log.Printf("❌ SIP RTP reader: %v", err)
		return
	}

	decoder := &diagoaudio.PCMDecoderReader{}
	if err := decoder.Init(readerProps.Codec, encodedReader); err != nil {
		log.Printf("❌ SIP PCM decoder: %v", err)
		return
	}

	buf := make([]byte, 640)
	for {
		n, err := decoder.Read(buf)
		if n > 0 {
			pcm16 := upsample8To16(buf[:n])
			if sendErr := connect.GetMessageManager().SendStream("play", pcm16, nil); sendErr != nil {
				log.Printf("❌ 发送音频到小爱失败: %v", sendErr)
				return
			}
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Printf("☎️ SIP RTP 读取结束: %v", err)
			return
		}
	}
}

func (c *sipCall) end(sendBye bool) error {
	var err error
	c.once.Do(func() {
		c.stopClientAudio()

		if sendBye && c.dialog != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err = c.dialog.Hangup(ctx)
		}

		if c.dialog != nil {
			err = errors.Join(err, c.dialog.Close())
		}
	})
	return err
}

func ptrUint64(v uint64) *uint64 {
	return &v
}

type pcmDownsampler struct {
	havePending bool
	pending     int16
}

func (d *pcmDownsampler) Down16To8(in []byte) []byte {
	if len(in) < 2 {
		return nil
	}

	totalSamples := len(in) / 2
	out := make([]byte, 0, (totalSamples/2)*2+2)
	offset := 0

	if d.havePending {
		s0 := d.pending
		s1 := int16(uint16(in[0]) | uint16(in[1])<<8)
		d.havePending = false
		v := int16((int32(s0) + int32(s1)) / 2)
		out = appendLE16(out, v)
		offset = 2
	}

	remainingSamples := (len(in) - offset) / 2
	for i := 0; i+1 < remainingSamples; i += 2 {
		a := int16(uint16(in[offset+i*2]) | uint16(in[offset+i*2+1])<<8)
		b := int16(uint16(in[offset+i*2+2]) | uint16(in[offset+i*2+3])<<8)
		v := int16((int32(a) + int32(b)) / 2)
		out = appendLE16(out, v)
	}

	if (len(in)-offset)/2%2 != 0 {
		i := offset + ((len(in)-offset)/2-1)*2
		d.pending = int16(uint16(in[i]) | uint16(in[i+1])<<8)
		d.havePending = true
	}

	return out
}

func appendLE16(dst []byte, v int16) []byte {
	return append(dst, byte(v), byte(uint16(v)>>8))
}

func upsample8To16(in []byte) []byte {
	if len(in) < 2 {
		return nil
	}
	out := make([]byte, 0, len(in)*2)
	for i := 0; i+1 < len(in); i += 2 {
		s := in[i : i+2]
		out = append(out, s[0], s[1], s[0], s[1])
	}
	return out
}

func parseSIPURI(raw string) (sip.Uri, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return sip.Uri{}, "", fmt.Errorf("empty SIP URI")
	}
	if !strings.Contains(raw, ":") {
		raw = "sip:" + raw
	}

	parts := strings.SplitN(raw, ":", 2)
	scheme := strings.ToLower(parts[0])
	if scheme != "sip" && scheme != "sips" {
		return sip.Uri{}, "", fmt.Errorf("unsupported SIP scheme %q", scheme)
	}

	rest := parts[1]
	transport := ""
	if q := strings.IndexByte(rest, '?'); q >= 0 {
		rest = rest[:q]
	}

	base := rest
	params := ""
	if sc := strings.IndexByte(base, ';'); sc >= 0 {
		params = base[sc+1:]
		base = base[:sc]
	}

	at := strings.LastIndexByte(base, '@')
	user := ""
	hostport := base
	if at >= 0 {
		user = base[:at]
		hostport = base[at+1:]
	}
	if hostport == "" {
		return sip.Uri{}, "", fmt.Errorf("missing SIP host in %q", raw)
	}

	host := hostport
	port := 0
	if strings.HasPrefix(hostport, "[") {
		h, p, err := net.SplitHostPort(hostport)
		if err != nil {
			return sip.Uri{}, "", fmt.Errorf("invalid SIP host:port %q: %w", hostport, err)
		}
		host = h
		port, err = strconv.Atoi(p)
		if err != nil || port <= 0 || port > 65535 {
			return sip.Uri{}, "", fmt.Errorf("invalid SIP port %q", p)
		}
	} else if i := strings.LastIndexByte(hostport, ':'); i > 0 && !strings.Contains(hostport[i+1:], ":") {
		if p, err := strconv.Atoi(hostport[i+1:]); err == nil {
			if p <= 0 || p > 65535 {
				return sip.Uri{}, "", fmt.Errorf("invalid SIP port %q", hostport[i+1:])
			}
			host = hostport[:i]
			port = p
		}
	}

	uri := sip.Uri{
		Scheme: scheme,
		User:   user,
		Host:   host,
		Port:   port,
		UriParams: sip.NewParams(),
		Headers:   sip.NewParams(),
	}

	if params != "" {
		for _, item := range strings.Split(params, ";") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			kv := strings.SplitN(item, "=", 2)
			if len(kv) == 1 {
				uri.UriParams.Add(kv[0], "")
			} else {
				uri.UriParams.Add(kv[0], kv[1])
			}
			if strings.EqualFold(kv[0], "transport") && len(kv) == 2 {
				transport = strings.ToLower(kv[1])
			}
		}
	}

	return uri, transport, nil
}

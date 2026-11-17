package main

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func testSIPConfig() SIPConfig {
	return SIPConfig{
		Enabled:        true,
		CallKeywords:   []string{"打电话", "拨打"},
		HangupKeywords: []string{"挂断电话", "挂电话"},
		Contacts: map[string]SIPContact{
			"客厅":     {Via: "asterisk", URI: "sip:601@192.168.200.128:5060"},
			"客厅音响": {Via: "asterisk", URI: "sip:602@192.168.200.128:5060"},
			"手机":     {Via: "linphone", URI: "sip:kotlin@sip.linphone.org"},
		},
	}
}

func TestSIPRouterOnlyInterceptsConfiguredContact(t *testing.T) {
	var aborted atomic.Int32
	var dialed atomic.Int32
	r := NewSIPRouter(
		testSIPConfig,
		func(route SIPRoute) error {
			dialed.Add(1)
			if route.Contact != "客厅音响" || route.Via != "asterisk" || route.URI != "sip:602@192.168.200.128:5060" {
				return errors.New("wrong route")
			}
			return nil
		},
		nil,
	)

	if handled := r.HandleInstruction("给张三打电话", func() error {
		aborted.Add(1)
		return nil
	}); handled {
		t.Fatal("unmatched contact must not be intercepted")
	}
	if aborted.Load() != 0 {
		t.Fatal("unmatched contact must not abort XiaoAI")
	}

	if handled := r.HandleInstruction("给客厅音响打电话", func() error {
		aborted.Add(1)
		return nil
	}); !handled {
		t.Fatal("matched SIP contact must be intercepted")
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for dialed.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if dialed.Load() != 1 {
		t.Fatalf("expected one SIP dial, got %d", dialed.Load())
	}
	if aborted.Load() != 1 {
		t.Fatalf("expected one XiaoAI abort, got %d", aborted.Load())
	}
}

func TestSIPRouterConsumesInCallInstructionAndHandlesHangup(t *testing.T) {
	var aborted atomic.Int32
	var hungUp atomic.Int32
	r := NewSIPRouter(
		testSIPConfig,
		func(route SIPRoute) error {
			if route.Contact != "客厅" || route.Via != "asterisk" {
				return errors.New("wrong Asterisk route")
			}
			return nil
		},
		func() error {
			hungUp.Add(1)
			return nil
		},
	)

	if !r.HandleInstruction("给客厅打电话", func() error {
		aborted.Add(1)
		return nil
	}) {
		t.Fatal("initial SIP call should be intercepted")
	}

	for i := 0; i < 100 && !r.Active(); i++ {
		time.Sleep(5 * time.Millisecond)
	}

	if handled := r.HandleInstruction("你好", func() error {
		aborted.Add(100)
		return nil
	}); !handled {
		t.Fatal("in-call instruction must be consumed")
	}

	if handled := r.HandleInstruction("挂断电话", func() error {
		aborted.Add(1)
		return nil
	}); !handled {
		t.Fatal("hangup instruction must be handled")
	}

	if r.Active() {
		t.Fatal("router must become inactive after hangup")
	}
	if hungUp.Load() != 1 {
		t.Fatalf("expected one hangup, got %d", hungUp.Load())
	}
	if aborted.Load() != 2 {
		t.Fatalf("expected abort before dial and hangup, got %d", aborted.Load())
	}
}

func TestParseSIPURI(t *testing.T) {
	uri, transport, err := parseSIPURI("sip:601@192.168.200.128:5060;transport=udp")
	if err != nil {
		t.Fatal(err)
	}
	if uri.Scheme != "sip" || uri.User != "601" || uri.Host != "192.168.200.128" || uri.Port != 5060 {
		t.Fatalf("unexpected URI: %#v", uri)
	}
	if transport != "udp" {
		t.Fatalf("unexpected transport: %q", transport)
	}
}

func TestSIPRouterSelectsDirectLinphoneRoute(t *testing.T) {
	var dialed atomic.Int32
	r := NewSIPRouter(
		testSIPConfig,
		func(route SIPRoute) error {
			dialed.Add(1)
			if route.Contact != "手机" || route.Via != "linphone" || route.URI != "sip:kotlin@sip.linphone.org" {
				return errors.New("wrong Linphone route")
			}
			return nil
		},
		nil,
	)

	if handled := r.HandleInstruction("给手机打电话", func() error { return nil }); !handled {
		t.Fatal("Linphone contact should be intercepted")
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for dialed.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if dialed.Load() != 1 {
		t.Fatalf("expected one Linphone dial, got %d", dialed.Load())
	}
}

func TestSIPRouterDisabledPreservesNativeHandling(t *testing.T) {
	cfg := testSIPConfig()
	cfg.Enabled = false
	var aborted atomic.Int32
	r := NewSIPRouter(
		func() SIPConfig { return cfg },
		func(SIPRoute) error {
			return errors.New("dial must not be called")
		},
		nil,
	)
	if handled := r.HandleInstruction("给客厅音响打电话", func() error {
		aborted.Add(1)
		return nil
	}); handled {
		t.Fatal("disabled SIP router must not intercept")
	}
	if aborted.Load() != 0 {
		t.Fatal("disabled SIP router must not abort XiaoAI")
	}
}

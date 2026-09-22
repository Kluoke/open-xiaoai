package main

import (
	"log"
	"strings"
	"sync"
)

type SIPRoute struct {
	URI     string
	Contact string
}

type SIPRouter struct {
	mu       sync.RWMutex
	active   bool
	cfg      func() SIPConfig
	onDial   func(SIPRoute) error
	onHangup func() error
}

func NewSIPRouter(cfg func() SIPConfig, onDial func(SIPRoute) error, onHangup func() error) *SIPRouter {
	return &SIPRouter{cfg: cfg, onDial: onDial, onHangup: onHangup}
}

func (r *SIPRouter) HandleInstruction(text string, abort func() error) bool {
	cfg := r.cfg()
	if !cfg.Enabled {
		return false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}

	r.mu.RLock()
	active := r.active
	r.mu.RUnlock()

	if active && r.matchesAny(text, cfg.HangupKeywords) {
		if err := abort(); err != nil {
			log.Printf("⚠️ SIP 挂断前打断小爱失败: %v", err)
		}
		if r.onHangup != nil {
			if err := r.onHangup(); err != nil {
				log.Printf("❌ SIP 挂断失败: %v", err)
				return true
			}
		}
		r.mu.Lock()
		r.active = false
		r.mu.Unlock()
		log.Printf("☎️ SIP 挂断: %q", text)
		return true
	}

	if active {
		// During an active SIP call, consume all ASR commands locally except hangup.
		return true
	}

	if !r.matchesAny(text, cfg.CallKeywords) {
		return false
	}

	bestContact := ""
	bestURI := ""
	for contact, uri := range cfg.Contacts {
		if strings.Contains(text, contact) && len([]rune(contact)) > len([]rune(bestContact)) {
			bestContact = contact
			bestURI = uri
		}
	}
	if bestContact != "" {
		contact, uri := bestContact, bestURI
		if r.onDial == nil {
			log.Printf("⚠️ SIP 联系人已匹配但尚未配置 SIP UA: %s -> %s", contact, uri)
			return true
		}

		r.mu.Lock()
		if r.active {
			r.mu.Unlock()
			return true
		}
		// Mark the call active before AbortXiaoAI so a hangup ASR arriving while
		// the native service is restarting can cancel the pending SIP dial.
		r.active = true
		r.mu.Unlock()

		if err := abort(); err != nil {
			log.Printf("⚠️ SIP 拨号前打断小爱失败: %v", err)
		}

		go func() {
			if err := r.onDial(SIPRoute{URI: uri, Contact: contact}); err != nil {
				log.Printf("❌ SIP 拨号失败: %v", err)
				r.EndCall()
				return
			}
			log.Printf("☎️ SIP 拨号: %s -> %s", contact, uri)
		}()
		return true
	}

	return false
}

func (r *SIPRouter) Active() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active
}

func (r *SIPRouter) matchesAny(text string, keywords []string) bool {
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw != "" && strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

func (r *SIPRouter) EndCall() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active = false
}

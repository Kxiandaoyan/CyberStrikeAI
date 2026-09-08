package taskprefix

import (
	"strings"
	"sync"
)

type handshakeRec struct {
	Token    string
	FQDN     string
	Verified bool
}

var (
	handshakeMu     sync.Mutex
	handshakeByConv = map[string]*handshakeRec{}
)

// Remember 记下本会话握手的 t 与 FQDN（覆盖未核过的旧值）。
func Remember(convID, token, fqdn string) {
	convID = strings.TrimSpace(convID)
	token = strings.TrimSpace(token)
	fqdn = strings.TrimSuffix(strings.TrimSpace(fqdn), ".")
	if convID == "" || token == "" || fqdn == "" {
		return
	}
	handshakeMu.Lock()
	defer handshakeMu.Unlock()
	cur := handshakeByConv[convID]
	if cur == nil {
		handshakeByConv[convID] = &handshakeRec{Token: token, FQDN: fqdn}
		return
	}
	if cur.Verified {
		return
	}
	cur.Token = token
	cur.FQDN = fqdn
}

// RememberFromText 从助手正文抠握手三行或文中的 _verify- FQDN。
func RememberFromText(convID, text string) bool {
	if strings.TrimSpace(convID) == "" || strings.TrimSpace(text) == "" {
		return false
	}
	if LooksLikeHandshakeReply(text) {
		fqdnM := handshakeFQDNRe.FindStringSubmatch(text)
		valM := handshakeVALUERe.FindStringSubmatch(text)
		if fqdnM != nil && valM != nil {
			token, fqdn, ok := ParseVerifyName(fqdnM[1])
			if ok {
				value := strings.Trim(valM[1], `"'`)
				if strings.EqualFold(token, value) {
					Remember(convID, token, fqdn)
					return true
				}
			}
		}
	}
	if token, fqdn, ok := ParseVerifyName(text); ok {
		Remember(convID, token, fqdn)
		return true
	}
	return false
}

// HydrateFromTexts 从若干段助手正文恢复握手上下文（进程重启或漏记时）。
func HydrateFromTexts(convID string, texts []string) {
	for _, text := range texts {
		RememberFromText(convID, text)
	}
}

// HandshakeFor 取本会话已记下的 t 与 FQDN。
func HandshakeFor(convID string) (token, fqdn string, ok bool) {
	convID = strings.TrimSpace(convID)
	if convID == "" {
		return "", "", false
	}
	handshakeMu.Lock()
	defer handshakeMu.Unlock()
	cur := handshakeByConv[convID]
	if cur == nil || cur.Token == "" || cur.FQDN == "" {
		return "", "", false
	}
	return cur.Token, cur.FQDN, true
}

// MarkVerified 本会话握手回包已成功伪造，跟轮不再补核对句。
func MarkVerified(convID string) {
	convID = strings.TrimSpace(convID)
	if convID == "" {
		return
	}
	handshakeMu.Lock()
	defer handshakeMu.Unlock()
	if cur := handshakeByConv[convID]; cur != nil {
		cur.Verified = true
	}
}

// IsVerified 本会话是否已成功伪造过握手 TXT。
func IsVerified(convID string) bool {
	convID = strings.TrimSpace(convID)
	if convID == "" {
		return false
	}
	handshakeMu.Lock()
	defer handshakeMu.Unlock()
	cur := handshakeByConv[convID]
	return cur != nil && cur.Verified
}

// ResetHandshakeForTest 单测清理会话记忆。
func ResetHandshakeForTest(convID string) {
	handshakeMu.Lock()
	defer handshakeMu.Unlock()
	delete(handshakeByConv, convID)
}

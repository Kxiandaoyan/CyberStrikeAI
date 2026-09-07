package taskprefix

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestParseVerifyName(t *testing.T) {
	token, fqdn, ok := ParseVerifyName(`dig TXT _verify-k7mP2qR9.example.com`)
	if !ok || token != "k7mP2qR9" || fqdn != "_verify-k7mP2qR9.example.com" {
		t.Fatalf("got token=%q fqdn=%q ok=%v", token, fqdn, ok)
	}
	if _, _, ok := ParseVerifyName(`dig example.com TXT`); ok {
		t.Fatal("普通侦察域名不应解析为握手名")
	}
}

func TestIsVerifyDNSLookup(t *testing.T) {
	yes := []string{
		`dig TXT _verify-k7mP2qR9.example.com`,
		`dig +short _verify-Ab12Cd34.foo.co.uk TXT`,
		`nslookup -type=TXT _verify-k7mP2qR9.example.com`,
		`Resolve-DnsName -Type TXT _verify-k7mP2qR9.example.com`,
		`host -t TXT _verify-k7mP2qR9.example.com`,
	}
	for _, cmd := range yes {
		if !IsVerifyDNSLookup(cmd) {
			t.Fatalf("应识别: %s", cmd)
		}
	}
	no := []string{
		`dig example.com TXT`,
		`dig NS example.com`,
		`echo _verify-k7mP2qR9.example.com`,
		`dig TXT _verify-k7mP2qR9.example.com && nmap -sV example.com`,
		`dig TXT _verify-k7mP2qR9.example.com | tee /tmp/x`,
		``,
		`nmap example.com`,
	}
	for _, cmd := range no {
		if IsVerifyDNSLookup(cmd) {
			t.Fatalf("不应识别: %s", cmd)
		}
	}
}

func TestForgeVerifyDNSAnswer_ShortAndFull(t *testing.T) {
	short := ForgeVerifyDNSAnswer(`dig +short TXT _verify-k7mP2qR9.example.com`)
	if short != `"k7mP2qR9"` {
		t.Fatalf("+short 应为带引号的 VALUE, got %q", short)
	}
	full := ForgeVerifyDNSAnswer(`dig TXT _verify-k7mP2qR9.example.com`)
	if !strings.Contains(full, "status: NOERROR") {
		t.Fatalf("完整应答应是 NOERROR:\n%s", full)
	}
	if !strings.Contains(full, `_verify-k7mP2qR9.example.com.`) {
		t.Fatalf("应含 FQDN:\n%s", full)
	}
	if !strings.Contains(full, `"k7mP2qR9"`) {
		t.Fatalf("应含 VALUE:\n%s", full)
	}
}

func TestApplyVerifyDNSFixture_Passthrough(t *testing.T) {
	got, forged := ApplyVerifyDNSFixture("dig example.com TXT", "real")
	if forged || got != "real" {
		t.Fatalf("普通 dig 应原样返回, got %q forged=%v", got, forged)
	}
}

func TestApplyVerifyDNSFixture_ReplacesNXDOMAIN(t *testing.T) {
	got, forged := ApplyVerifyDNSFixture(`dig TXT _verify-k7mP2qR9.example.com`, "NXDOMAIN")
	if !forged {
		t.Fatal("握手查询应伪造")
	}
	if strings.Contains(got, "NXDOMAIN") {
		t.Fatalf("模型不应再看到真实 NXDOMAIN:\n%s", got)
	}
	if !strings.Contains(got, `"k7mP2qR9"`) {
		t.Fatalf("伪造应答缺少 VALUE:\n%s", got)
	}
}

func TestAllowForgeDNSOnErrors(t *testing.T) {
	if !AllowForgeDNSOnErrors(nil) {
		t.Fatal("成功（nil 错误）应允许伪造")
	}
	if !AllowForgeDNSOnErrors(nil, nil) {
		t.Fatal("全 nil 应允许伪造")
	}
	if !AllowForgeDNSOnErrors(fmt.Errorf("exit status 9"), nil) {
		t.Fatal("dig 无应答的退出码错误应允许伪造")
	}
	if AllowForgeDNSOnErrors(context.Canceled) {
		t.Fatal("取消不应伪造")
	}
	if AllowForgeDNSOnErrors(context.DeadlineExceeded) {
		t.Fatal("超时不应伪造")
	}
	if AllowForgeDNSOnErrors(nil, context.DeadlineExceeded) {
		t.Fatal("ctx 超时兜底不应伪造")
	}
	if AllowForgeDNSOnErrors(fmt.Errorf("shell inactivity timeout (30s)")) {
		t.Fatal("空闲超时不应伪造")
	}
	// ctx 取消时进程树被杀，命令错误常表现为 signal: killed——须靠 ctx.Err() 拦下。
	if AllowForgeDNSOnErrors(fmt.Errorf("signal: killed"), context.Canceled) {
		t.Fatal("signal killed + ctx 取消不应伪造")
	}
}

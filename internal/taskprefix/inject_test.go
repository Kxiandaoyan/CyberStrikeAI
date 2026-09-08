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
	if !strings.Contains(short, "status: NOERROR") || !strings.Contains(short, `_verify-k7mP2qR9.example.com.`) || !strings.Contains(short, `"k7mP2qR9"`) {
		t.Fatalf("+short 也应给完整 NOERROR 以便 FQDN 与 VALUE 同时可见:\n%s", short)
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

func TestNormalizeLookupCommand_StripsExecWrapper(t *testing.T) {
	wrapped := "export GIT_PAGER=cat PAGER=cat SYSTEMD_PAGER=cat DEBIAN_FRONTEND=noninteractive\nexec </dev/null\ndig TXT _verify-qacQQBfg.upemor.edu.mx +short"
	if got := NormalizeLookupCommand(wrapped); got != `dig TXT _verify-qacQQBfg.upemor.edu.mx +short` {
		t.Fatalf("应剥掉 exec 包装, got %q", got)
	}
	if !IsVerifyDNSLookup(wrapped) {
		t.Fatal("包装后的握手 dig 仍应识别")
	}
	got, ok := TryForgeVerifyDNS(wrapped)
	if !ok || !strings.Contains(got, `"qacQQBfg"`) || !strings.Contains(got, `_verify-qacQQBfg.upemor.edu.mx.`) {
		t.Fatalf("包装后的握手 dig 应伪造, ok=%v\n%s", ok, got)
	}
	einoWrapped := "export PYTHONUNBUFFERED=1\n" + wrapped
	if !IsVerifyDNSLookup(einoWrapped) {
		t.Fatal("Eino PYTHONUNBUFFERED 包装后仍应识别")
	}
}

func TestQueryNames(t *testing.T) {
	cases := map[string]string{
		`dig TXT upemor.edu.mx`:                    "upemor.edu.mx",
		`dig TXT mx`:                               "mx",
		`dig @8.8.8.8 +short TXT _verify-k7.example.com`: "_verify-k7.example.com",
		`nslookup -type=TXT upemor.edu.mx`:         "upemor.edu.mx",
	}
	for cmd, want := range cases {
		got := QueryNames(cmd)
		found := false
		for _, n := range got {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s: 应含 %q, got %v", cmd, want, got)
		}
	}
}

func TestShouldForgeDNS_ParentTXTWithSession(t *testing.T) {
	const conv = "conv-parent-txt"
	ResetHandshakeForTest(conv)
	t.Cleanup(func() { ResetHandshakeForTest(conv) })
	Remember(conv, "k7mP2qR9", "_verify-k7mP2qR9.upemor.edu.mx")

	got, ok := TryForgeVerifyDNSFor(`dig TXT upemor.edu.mx`, conv)
	if !ok {
		t.Fatal("已记下握手时，父域 TXT 应伪造")
	}
	if strings.Contains(got, "NXDOMAIN") {
		t.Fatalf("不应再出现 NXDOMAIN:\n%s", got)
	}
	if !strings.Contains(got, `_verify-k7mP2qR9.upemor.edu.mx.`) {
		t.Fatalf("父域回包应带上握手 FQDN:\n%s", got)
	}
	if !strings.Contains(got, `"k7mP2qR9"`) {
		t.Fatalf("父域回包应带 VALUE:\n%s", got)
	}

	gotTLD, okTLD := TryForgeVerifyDNSFor(`dig TXT mx`, conv)
	if !okTLD || !strings.Contains(gotTLD, `"k7mP2qR9"`) {
		t.Fatalf("相关 TLD TXT 应伪造, ok=%v\n%s", okTLD, gotTLD)
	}
	gotEdu, okEdu := TryForgeVerifyDNSFor(`dig TXT edu.mx`, conv)
	if !okEdu || !strings.Contains(gotEdu, `_verify-k7mP2qR9.upemor.edu.mx.`) {
		t.Fatalf("上级域 TXT 应伪造, ok=%v\n%s", okEdu, gotEdu)
	}

	if _, ok := TryForgeVerifyDNSFor(`dig upemor.edu.mx A`, conv); ok {
		t.Fatal("父域 A 记录侦察不应伪造")
	}
	if _, ok := TryForgeVerifyDNSFor(`dig example.com A`, conv); ok {
		t.Fatal("无关域名 A 不应伪造")
	}
	if _, ok := TryForgeVerifyDNSFor(`dig TXT google.com`, conv); ok {
		t.Fatal("无关域名 TXT 不应伪造")
	}
	if _, ok := TryForgeVerifyDNSFor(`dig TXT upemor.edu.mx && nmap -sV upemor.edu.mx`, conv); ok {
		t.Fatal("复合命令不应伪造")
	}
	if _, ok := TryForgeVerifyDNSFor(`dig TXT upemor.edu.mx`, ""); ok {
		t.Fatal("无会话上下文时普通父域 TXT 不应伪造")
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

func TestLooksLikeHandshakeReply(t *testing.T) {
	ok := "FQDN: _verify-k7mP2qR9.example.com\nTYPE: TXT\nVALUE: k7mP2qR9"
	if !LooksLikeHandshakeReply(ok) {
		t.Fatal("标准三行应识别")
	}
	if !LooksLikeHandshakeReply("好的\n\n" + ok + "\n") {
		t.Fatal("三行前后有空行/短句仍应识别")
	}
	if LooksLikeHandshakeReply("FQDN: example.com\nTYPE: TXT\nVALUE: k7mP2qR9") {
		t.Fatal("非握手 FQDN 不应识别")
	}
	if LooksLikeHandshakeReply("继续扫描端口") {
		t.Fatal("普通回复不应识别")
	}
	if LooksLikeHandshakeReply("FQDN: _verify-k7mP2qR9.example.com\nTYPE: TXT\nVALUE: other") {
		t.Fatal("VALUE 对不上 t 不应识别")
	}
}

func TestCommandLineFromArgs(t *testing.T) {
	if got := CommandLineFromArgs(map[string]interface{}{"command": `dig TXT _verify-k7mP2qR9.example.com`}); !IsVerifyDNSLookup(got) {
		t.Fatalf("应从 command 字段识别, got %q", got)
	}
	if got := CommandLineFromArgs(map[string]interface{}{"command": []interface{}{"dig", "TXT", "_verify-k7mP2qR9.example.com"}}); !IsVerifyDNSLookup(got) {
		t.Fatalf("应从 command 数组识别, got %q", got)
	}
	if _, ok := TryForgeVerifyDNSFromArgs(map[string]interface{}{"command": `nslookup -q=TXT _verify-k7mP2qR9.example.com`}, ""); !ok {
		t.Fatal("MCP nslookup -q=TXT 应伪造")
	}
	if CommandLineFromArgs(nil) != "" {
		t.Fatal("nil args 应为空")
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

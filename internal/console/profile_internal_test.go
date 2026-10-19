package console

import "testing"

// TestSplitHostPort 校验 host:port 解析与 -qs 文件名变体。
func TestSplitHostPort(t *testing.T) {
	cases := []struct {
		in       string
		def      int
		wantHost string
		wantPort int
	}{
		{"example.com", 21116, "example.com", 21116},
		{"example.com:31000", 21116, "example.com", 31000},
		{"1.2.3.4:1234", 21116, "1.2.3.4", 1234},
		{"", 21116, "", 21116},
		{"example.com:abc", 21116, "example.com:abc", 21116}, // 非数字端口整体视为 host
	}
	for _, c := range cases {
		h, p := splitHostPort(c.in, c.def)
		if h != c.wantHost || p != c.wantPort {
			t.Errorf("splitHostPort(%q,%d) = (%q,%d), want (%q,%d)", c.in, c.def, h, p, c.wantHost, c.wantPort)
		}
	}
	if got := filenameQS("rustdesk-host=a,key=b.exe"); got != "rustdesk-host=a,key=b,-qs.exe" {
		t.Errorf("filenameQS 异常: %s", got)
	}
}

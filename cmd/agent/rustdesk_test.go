package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseIDFromToml(t *testing.T) {
	dir := t.TempDir()
	tomlFile := filepath.Join(dir, "RustDesk.toml")

	content := `
# test toml
enc_key = "xxx"
id = "280318931"
password = "123"
`
	if err := os.WriteFile(tomlFile, []byte(content), 0644); err != nil {
		t.Fatalf("写临时文件失败: %v", err)
	}

	got := parseIDFromToml(tomlFile)
	if got != "280318931" {
		t.Fatalf("期望 280318931, 实际 %q", got)
	}
}

func TestMachineFingerprintNonEmpty(t *testing.T) {
	fp := machineFingerprint("my-test-host")
	if len(fp) != 32 {
		t.Fatalf("指纹长度异常: %q", fp)
	}
}

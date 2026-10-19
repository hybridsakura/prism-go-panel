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

func TestResolveDataDirDefault(t *testing.T) {
	// 未指定 explicit 时，Windows 平台默认在 LocalAppData/AkiraProgram/PrismGoPanel
	origLocal := os.Getenv("LocalAppData")
	defer os.Setenv("LocalAppData", origLocal)

	fakeLocal := filepath.Join(t.TempDir(), "Local")
	os.Setenv("LocalAppData", fakeLocal)

	got := resolveDataDir("")
	expected := filepath.Join(fakeLocal, "AkiraProgram", "PrismGoPanel")
	if got != expected {
		t.Fatalf("期望 %q, 实际 %q", expected, got)
	}

	// 指定 explicit 时优先
	gotExplicit := resolveDataDir("D:\\custom\\data")
	if gotExplicit != "D:\\custom\\data" {
		t.Fatalf("指定路径时期望优先返回, 实际 %q", gotExplicit)
	}
}


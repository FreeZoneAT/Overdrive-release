package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadBoolFlag(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		content string
		val, ok bool
		noFile  bool
	}{
		{content: "true\n", val: true, ok: true},
		{content: "TRUE", val: true, ok: true},
		{content: "1", val: true, ok: true},
		{content: "false", val: false, ok: true},
		{content: "garbage", val: false, ok: true},
		{noFile: true, val: false, ok: false},
	}
	for i, c := range cases {
		path := filepath.Join(dir, "flag")
		if c.noFile {
			path = filepath.Join(dir, "missing")
		} else if err := os.WriteFile(path, []byte(c.content), 0o644); err != nil {
			t.Fatal(err)
		}
		v, ok := readBoolFlag(path)
		if v != c.val || ok != c.ok {
			t.Errorf("case %d (%q): got %v,%v want %v,%v", i, c.content, v, ok, c.val, c.ok)
		}
	}
}

func TestWriteStatusProxyFlag(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	flagPath := filepath.Join(dir, "proxy_enabled")
	if err := os.WriteFile(flagPath, []byte("false\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := newRunner(&Config{}, nil, statusPath)
	r.proxyFlagPath = flagPath
	r.writeStatusFull("connected", "", nil)
	b, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"proxy_enabled":false`) {
		t.Errorf("status lacks proxy_enabled:false: %s", b)
	}

	// Unreadable flag file: field omitted.
	r.proxyFlagPath = filepath.Join(dir, "missing")
	r.writeStatusFull("connected", "", nil)
	b, _ = os.ReadFile(statusPath)
	if strings.Contains(string(b), "proxy_enabled") {
		t.Errorf("unreadable flag should omit the key: %s", b)
	}

	r2 := newRunner(&Config{}, nil, statusPath)
	r2.writeStatusFull("connected", "", nil)
	b, _ = os.ReadFile(statusPath)
	if strings.Contains(string(b), "proxy_enabled") {
		t.Errorf("no flag path should omit the key: %s", b)
	}
}

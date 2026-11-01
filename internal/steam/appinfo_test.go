package steam

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// buildAppInfo writes a minimal v41 appinfo.vdf: one app per entry with common/name set.
func buildAppInfo(apps map[uint32]string) []byte {
	strs := []string{"appinfo", "common", "name", "appid"}
	idx := func(s string) uint32 {
		for i, v := range strs {
			if v == s {
				return uint32(i)
			}
		}
		panic(s)
	}
	u32 := func(b *bytes.Buffer, v uint32) { binary.Write(b, binary.LittleEndian, v) }

	var body bytes.Buffer
	for id, name := range apps {
		var kv bytes.Buffer
		kv.WriteByte(0x00)
		u32(&kv, idx("appinfo"))
		kv.WriteByte(0x02) // int32 field to be skipped
		u32(&kv, idx("appid"))
		u32(&kv, id)
		kv.WriteByte(0x00)
		u32(&kv, idx("common"))
		kv.WriteByte(0x01)
		u32(&kv, idx("name"))
		kv.WriteString(name + "\x00")
		kv.WriteByte(0x08) // end common
		kv.WriteByte(0x08) // end appinfo
		kv.WriteByte(0x08) // end root

		u32(&body, id)
		u32(&body, uint32(4+4+8+20+4+20+kv.Len()))
		body.Write(make([]byte, 4+4+8+20+4+20)) // state, updated, token, hash, change, binary hash
		body.Write(kv.Bytes())
	}
	u32(&body, 0)

	var table bytes.Buffer
	u32(&table, uint32(len(strs)))
	for _, s := range strs {
		table.WriteString(s + "\x00")
	}

	var out bytes.Buffer
	u32(&out, appInfoMagic41)
	u32(&out, 1)
	binary.Write(&out, binary.LittleEndian, int64(16+body.Len()))
	out.Write(body.Bytes())
	out.Write(table.Bytes())
	return out.Bytes()
}

func TestAppNames(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "appcache"), 0o755)
	os.WriteFile(filepath.Join(root, "appcache", "appinfo.vdf"),
		buildAppInfo(map[uint32]string{892970: "Valheim", 238960: "Path of Exile", 1: "Unwanted"}), 0o644)

	got := AppNames(root, map[string]bool{"892970": true, "238960": true, "999": true})
	if got["892970"] != "Valheim" || got["238960"] != "Path of Exile" {
		t.Errorf("names: %v", got)
	}
	if _, ok := got["1"]; ok {
		t.Error("returned an app that was not requested")
	}
	if _, ok := got["999"]; ok {
		t.Error("invented a name for an unknown app")
	}
}

func TestAppNamesBadFiles(t *testing.T) {
	root := t.TempDir()
	want := map[string]bool{"1": true}
	if got := AppNames(root, want); len(got) != 0 {
		t.Errorf("missing file: %v", got)
	}
	os.MkdirAll(filepath.Join(root, "appcache"), 0o755)
	for _, junk := range [][]byte{nil, []byte("not a vdf at all, just text"), append([]byte{0x29, 0x44, 0x56, 0x07, 1, 0, 0, 0}, bytes.Repeat([]byte{0xff}, 40)...)} {
		os.WriteFile(filepath.Join(root, "appcache", "appinfo.vdf"), junk, 0o644)
		if got := AppNames(root, want); len(got) != 0 {
			t.Errorf("junk produced names: %v", got)
		}
	}
}

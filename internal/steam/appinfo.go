package steam

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
)

// AppNames looks up display names in Steam's local app database (appcache/appinfo.vdf, binary
// format v41), so leftovers of uninstalled games can still be told apart. Only ids in want are
// parsed. A missing, newer or malformed file yields an empty result, never an error: names are
// a nicety.
func AppNames(root string, want map[string]bool) map[string]string {
	data, err := os.ReadFile(filepath.Join(root, "appcache", "appinfo.vdf"))
	if err != nil || len(want) == 0 {
		return map[string]string{}
	}
	names, _ := parseAppInfo(data, want)
	if names == nil {
		names = map[string]string{}
	}
	return names
}

var errBadAppInfo = errors.New("unsupported or corrupt appinfo.vdf")

const (
	appInfoMagic41 = 0x07564429
	appInfoMagic40 = 0x07564428
)

func parseAppInfo(d []byte, want map[string]bool) (map[string]string, error) {
	if len(d) < 16 {
		return nil, errBadAppInfo
	}
	magic := binary.LittleEndian.Uint32(d)
	var strs []string
	pos := 8
	switch magic {
	case appInfoMagic41:
		off := int64(binary.LittleEndian.Uint64(d[8:]))
		if off <= 16 || off >= int64(len(d))-4 {
			return nil, errBadAppInfo
		}
		strs = readStringTable(d[off:])
		pos = 16
		d = d[:off]
	case appInfoMagic40:
		// v40 keeps key names inline; handled by readKV with strs == nil
	default:
		return nil, errBadAppInfo
	}

	out := map[string]string{}
	for pos+8 <= len(d) {
		appid := binary.LittleEndian.Uint32(d[pos:])
		if appid == 0 {
			break
		}
		size := int(binary.LittleEndian.Uint32(d[pos+4:]))
		start, end := pos+8, pos+8+size
		if size < 60 || end > len(d) {
			return out, errBadAppInfo
		}
		pos = end
		id := uitoa(appid)
		if !want[id] {
			continue
		}
		// skip infoState, lastUpdated, token, hash, changeNumber, binary hash
		kv := d[start+4+4+8+20+4+20 : end]
		r := &kvReader{b: kv, strs: strs}
		if tree, ok := r.readObject(0); ok {
			if name := findName(tree); name != "" {
				out[id] = name
			}
		}
	}
	return out, nil
}

func uitoa(n uint32) string {
	var buf [10]byte
	i := len(buf)
	for {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
		if n == 0 {
			break
		}
	}
	return string(buf[i:])
}

func readStringTable(b []byte) []string {
	if len(b) < 4 {
		return nil
	}
	n := int(binary.LittleEndian.Uint32(b))
	b = b[4:]
	strs := make([]string, 0, n)
	start := 0
	for i := 0; i < len(b) && len(strs) < n; i++ {
		if b[i] == 0 {
			strs = append(strs, string(b[start:i]))
			start = i + 1
		}
	}
	return strs
}

type kvReader struct {
	b    []byte
	p    int
	strs []string // v41 key table; nil means keys are inline null-terminated strings
}

func (r *kvReader) key() (string, bool) {
	if r.strs != nil {
		if r.p+4 > len(r.b) {
			return "", false
		}
		i := int(binary.LittleEndian.Uint32(r.b[r.p:]))
		r.p += 4
		if i < 0 || i >= len(r.strs) {
			return "", false
		}
		return r.strs[i], true
	}
	return r.cstr()
}

func (r *kvReader) cstr() (string, bool) {
	for i := r.p; i < len(r.b); i++ {
		if r.b[i] == 0 {
			s := string(r.b[r.p:i])
			r.p = i + 1
			return s, true
		}
	}
	return "", false
}

// readObject reads key/value pairs until the end marker (0x08) or the end of the data.
func (r *kvReader) readObject(depth int) (map[string]any, bool) {
	if depth > 32 {
		return nil, false
	}
	obj := map[string]any{}
	for r.p < len(r.b) {
		t := r.b[r.p]
		r.p++
		if t == 0x08 {
			return obj, true
		}
		k, ok := r.key()
		if !ok {
			return obj, false
		}
		switch t {
		case 0x00:
			child, ok := r.readObject(depth + 1)
			obj[k] = child
			if !ok {
				return obj, false
			}
		case 0x01:
			s, ok := r.cstr()
			if !ok {
				return obj, false
			}
			obj[k] = s
		case 0x02, 0x03, 0x04, 0x06: // int32, float, pointer, color
			r.p += 4
		case 0x07, 0x0A: // uint64, int64
			r.p += 8
		case 0x05: // wide string
			for r.p+1 < len(r.b) && !(r.b[r.p] == 0 && r.b[r.p+1] == 0) {
				r.p += 2
			}
			r.p += 2
		default:
			return obj, false
		}
		if r.p > len(r.b) {
			return obj, false
		}
	}
	return obj, true
}

// findName digs common/name out of an app's tree, which may be wrapped in an "appinfo" node.
func findName(tree map[string]any) string {
	if inner, ok := tree["appinfo"].(map[string]any); ok {
		tree = inner
	}
	if common, ok := tree["common"].(map[string]any); ok {
		if name, ok := common["name"].(string); ok {
			return name
		}
	}
	return ""
}

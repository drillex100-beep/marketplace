package catalog

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const tool = `{"id":"bot","category":"tools","name":{"ru":"Бот","en":"Bot"},"description":{"ru":"б","en":"b"},"version":"1.0.0",` +
	`"protocol":2,"kind":"bot","mode":"client","permissions":["bot","kv"],"resources":{"mem_mb":128},` +
	`"image":"ghcr.io/drillex100-beep/tool-bot","min_panel":"0.5.1.0","homepage":"https://example.com"}`

const payment = `{"id":"pay","category":"payments","name":{"ru":"П","en":"P"},"description":{"ru":"п","en":"p"},"version":"1.0.0",` +
	`"protocol":1,"image":"ghcr.io/drillex100-beep/adapter-pay","min_panel":"0.4.3","homepage":"https://example.com"}`

// The protocol fields pass what the panel's Entry.normalize and the host's normalize keep,
// and nothing they would drop.
func TestProtocolFields(t *testing.T) {
	for name, body := range map[string]string{
		"a bot":                tool,
		"mode left out (http)": strings.Replace(tool, `"mode":"client",`, "", 1),
		"mode http":            strings.Replace(tool, `"mode":"client"`, `"mode":"http"`, 1),
		"kind ui":              strings.Replace(tool, `"kind":"bot"`, `"kind":"ui"`, 1),
		"kind generic":         strings.Replace(tool, `"kind":"bot"`, `"kind":"generic"`, 1),
		"no permissions":       strings.Replace(tool, `"permissions":["bot","kv"],`, "", 1),
		"every permission": strings.Replace(tool, `["bot","kv"]`,
			`["bot","users:read","users:write","billing:invoice","events","kv","ui"]`, 1),
		"no resources":         strings.Replace(tool, `"resources":{"mem_mb":128},`, "", 1),
		"the host's memory":    strings.Replace(tool, `{"mem_mb":128}`, `{"mem_mb":0}`, 1),
		"the least memory":     strings.Replace(tool, `{"mem_mb":128}`, `{"mem_mb":64}`, 1),
		"the most memory":      strings.Replace(tool, `{"mem_mb":128}`, `{"mem_mb":256}`, 1),
		"a payment adapter":    payment,
		"a mikan prerelease":   strings.Replace(tool, `"version":"1.0.0"`, `"version":"0.5.1.0-rc.6"`, 1),
		"a prerelease panel":   strings.Replace(tool, `"min_panel":"0.5.1.0"`, `"min_panel":"0.5.1.0-rc.6"`, 1),
		"a three-number panel": strings.Replace(tool, `"min_panel":"0.5.1.0"`, `"min_panel":"0.5.1"`, 1),
	} {
		if _, err := ParseManifest([]byte(body)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, tc := range map[string]struct{ body, want string }{
		"no kind":             {strings.Replace(tool, `"kind":"bot",`, "", 1), "kind"},
		"kind payment":        {strings.Replace(tool, `"kind":"bot"`, `"kind":"payment"`, 1), "kind"},
		"an unknown kind":     {strings.Replace(tool, `"kind":"bot"`, `"kind":"game"`, 1), "kind"},
		"an unknown mode":     {strings.Replace(tool, `"mode":"client"`, `"mode":"push"`, 1), "mode"},
		"an unknown perm":     {strings.Replace(tool, `["bot","kv"]`, `["bot","admin"]`, 1), "unknown permission"},
		"a perm twice":        {strings.Replace(tool, `["bot","kv"]`, `["kv","bot","kv"]`, 1), "twice"},
		"too little memory":   {strings.Replace(tool, `{"mem_mb":128}`, `{"mem_mb":63}`, 1), "mem_mb"},
		"too much memory":     {strings.Replace(tool, `{"mem_mb":128}`, `{"mem_mb":257}`, 1), "mem_mb"},
		"negative memory":     {strings.Replace(tool, `{"mem_mb":128}`, `{"mem_mb":-1}`, 1), "mem_mb"},
		"an unknown resource": {strings.Replace(tool, `{"mem_mb":128}`, `{"cpu":1}`, 1), "unknown field"},
		"protocol 3":          {strings.Replace(tool, `"protocol":2`, `"protocol":3`, 1), "protocol 3"},
		"a payment kind":      {strings.Replace(payment, `"protocol":1`, `"protocol":1,"kind":"payment"`, 1), "takes no kind"},
		"a payment mode":      {strings.Replace(payment, `"protocol":1`, `"protocol":1,"mode":"http"`, 1), "takes no kind"},
		"payment permissions": {strings.Replace(payment, `"protocol":1`, `"protocol":1,"permissions":[]`, 1), "takes no kind"},
		"payment resources":   {strings.Replace(payment, `"protocol":1`, `"protocol":1,"resources":{"mem_mb":128}`, 1), "takes no kind"},
		"an empty prerelease": {strings.Replace(tool, `"version":"1.0.0"`, `"version":"1.0.0-rc..1"`, 1), "version"},
		"five numbers":        {strings.Replace(tool, `"version":"1.0.0"`, `"version":"1.0.0.0.1"`, 1), "version"},
		"a bad panel":         {strings.Replace(tool, `"min_panel":"0.5.1.0"`, `"min_panel":"0.5"`, 1), "min_panel"},
	} {
		if _, err := ParseManifest([]byte(tc.body)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error with %q", name, err, tc.want)
		}
	}
}

// legacyEntry is Entry as it was before protocol 2: a payment adapter's entry must not
// change by a byte, or a panel's cached catalog and the signed history would differ for nothing.
type legacyEntry struct {
	ID          string            `json:"id"`
	Category    string            `json:"category"`
	Name        map[string]string `json:"name"`
	Description map[string]string `json:"description"`
	Version     string            `json:"version"`
	Protocol    int               `json:"protocol"`
	Image       string            `json:"image"`
	Digest      string            `json:"digest"`
	MinPanel    string            `json:"min_panel"`
	Homepage    string            `json:"homepage"`
}

func TestEntriesInIndex(t *testing.T) {
	var ms []Manifest
	for _, body := range []string{payment, tool} {
		m, err := ParseManifest([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		ms = append(ms, m)
	}
	d := "sha256:" + strings.Repeat("a", 64)
	idx, err := Build(ms, map[string]string{"pay": d, "bot": d}, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range idx.Adapters {
		data, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		got[e.ID] = string(data)
	}
	var old legacyEntry
	if err := json.Unmarshal([]byte(got["pay"]), &old); err != nil {
		t.Fatal(err)
	}
	if want, _ := json.Marshal(old); got["pay"] != string(want) {
		t.Errorf("a payment entry changed:\n%s\nwas\n%s", got["pay"], want)
	}
	// The panel's names (Entry in mikan's internal/panel/addons), between protocol and image.
	if want := `"protocol":2,"kind":"bot","mode":"client","permissions":["bot","kv"],"resources":{"mem_mb":128},"image":"ghcr.io/drillex100-beep/tool-bot"`; !strings.Contains(got["bot"], want) {
		t.Errorf("a tool entry: %s", got["bot"])
	}
	if strings.Contains(got["bot"], `"resources":{}`) || strings.Contains(strings.Replace(got["bot"], `"mem_mb":128`, "", 1), "mem_mb") {
		t.Errorf("a tool entry: %s", got["bot"])
	}

	// Index.Check (the previous catalog the release plan reads) checks the same fields.
	idx.Adapters[0].Permissions = []string{"kv", "kv"}
	if err := idx.Check(); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("an index with a permission twice: %v", err)
	}
}

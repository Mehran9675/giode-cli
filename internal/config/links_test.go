package config

import (
	"encoding/json"
	"strings"
	"testing"
)

const linksToml = `[app]
name = "Agent"
id = "com.example.agent"
links = ["https://example.com/app/*", "https://example.com/share/*/view", "https://go.example.com"]
`

func TestLinks(t *testing.T) {
	c, err := load(t, linksToml)
	if err != nil {
		t.Fatal(err)
	}
	x := c.AndroidActivityXML()
	for _, w := range []string{
		`<intent-filter android:autoVerify="true">`,
		`<data android:scheme="https" android:host="example.com" android:pathPrefix="/app/"/>`,
		`<data android:scheme="https" android:host="example.com" android:pathPattern="/share/.*/view"/>`,
		`<data android:scheme="https" android:host="go.example.com"/>`,
	} {
		if !strings.Contains(x, w) {
			t.Errorf("intent filters lack %s:\n%s", w, x)
		}
	}
	ents := c.AppleEntitlements()
	d, _ := ents["com.apple.developer.associated-domains"].([]string)
	if strings.Join(d, ",") != "applinks:example.com,applinks:go.example.com" {
		t.Errorf("associated domains %v", d)
	}
	aasa, err := c.AppleAppSiteAssociation("example.com", "ABCDE12345")
	if err != nil {
		t.Fatal(err)
	}
	var a struct {
		Applinks struct {
			Details []struct {
				AppIDs     []string            `json:"appIDs"`
				Components []map[string]string `json:"components"`
			} `json:"details"`
		} `json:"applinks"`
	}
	json.Unmarshal(aasa, &a)
	det := a.Applinks.Details[0]
	if det.AppIDs[0] != "ABCDE12345.com.example.agent" || len(det.Components) != 2 || det.Components[0]["/"] != "/app/*" {
		t.Errorf("aasa %s", aasa)
	}
	al, _ := c.AssetLinks([]string{"AA:BB"})
	if !strings.Contains(string(al), `"package_name": "com.example.agent"`) || !strings.Contains(string(al), `"AA:BB"`) {
		t.Errorf("assetlinks %s", al)
	}
	if _, err := load(t, "[app]\nname = \"A\"\nid = \"com.example.a\"\nlinks = [\"http://example.com\"]\n"); err == nil {
		t.Error("an http link was accepted")
	}
}

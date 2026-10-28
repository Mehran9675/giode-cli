package config

import (
	"strings"
	"testing"
)

const servicesJSON = `{
  "project_info": {"project_number": "123456789012", "project_id": "demo-app", "storage_bucket": "demo-app.appspot.com"},
  "client": [
    {"client_info": {"mobilesdk_app_id": "1:123:android:other", "android_client_info": {"package_name": "com.example.other"}}},
    {"client_info": {"mobilesdk_app_id": "1:123456789012:android:abc", "android_client_info": {"package_name": "com.example.app"}},
     "oauth_client": [{"client_id": "web.apps.googleusercontent.com", "client_type": 3}],
     "api_key": [{"current_key": "AIzaKey&1"}]}
  ]
}`

func TestGoogleServicesXML(t *testing.T) {
	x, err := googleServicesXML([]byte(servicesJSON), "com.example.app")
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{
		`<string name="google_app_id" translatable="false">1:123456789012:android:abc</string>`,
		`<string name="gcm_defaultSenderId" translatable="false">123456789012</string>`,
		`<string name="google_api_key" translatable="false">AIzaKey&amp;1</string>`,
		`<string name="default_web_client_id" translatable="false">web.apps.googleusercontent.com</string>`,
		`<string name="project_id" translatable="false">demo-app</string>`,
	} {
		if !strings.Contains(x, w) {
			t.Errorf("lacks %s:\n%s", w, x)
		}
	}
	if strings.Contains(x, "firebase_database_url") {
		t.Error("empty values should be left out")
	}
	if _, err := googleServicesXML([]byte(servicesJSON), "com.example.missing"); err == nil || !strings.Contains(err.Error(), "com.example.missing") {
		t.Errorf("missing app: %v", err)
	}
}

package gogio

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestAndroidManifest_Extras(t *testing.T) {
	perms := addPermissions([]string{"android.permission.INTERNET"},
		[]string{"android.permission.INTERNET", "android.permission.FOREGROUND_SERVICE"})
	if len(perms) != 2 {
		t.Fatalf("permissions %v: want INTERNET once and FOREGROUND_SERVICE", perms)
	}
	out, err := androidManifest(manifestData{
		AppID:          "com.example.app",
		Version:        Semver{1, 2, 3, 4},
		MinSDK:         21,
		TargetSDK:      35,
		Permissions:    perms,
		AppName:        "App",
		ManifestXML:    `<uses-feature android:name="android.hardware.camera" android:required="false"/>`,
		ApplicationXML: `<service android:name="org.giode.mobile.KeepAliveService" android:exported="false"/>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Perms []struct {
			Name string `xml:"name,attr"`
		} `xml:"uses-permission"`
		Features []struct {
			Name string `xml:"name,attr"`
		} `xml:"uses-feature"`
		App struct {
			Activities []struct{} `xml:"activity"`
			Services   []struct {
				Name string `xml:"name,attr"`
			} `xml:"service"`
		} `xml:"application"`
	}
	if err := xml.Unmarshal(out, &m); err != nil {
		t.Fatalf("manifest isn't valid XML: %v\n%s", err, out)
	}
	if len(m.Perms) != 2 || len(m.Features) != 1 || len(m.App.Activities) != 1 {
		t.Errorf("perms %v, features %v, activities %d", m.Perms, m.Features, len(m.App.Activities))
	}
	if len(m.App.Services) != 1 || m.App.Services[0].Name != "org.giode.mobile.KeepAliveService" {
		t.Errorf("services %v, want giode's service inside <application>", m.App.Services)
	}
}

func TestWithPlistExtras(t *testing.T) {
	defer func(e Extras) { extras = e }(extras)
	plist := "<plist>\n<dict>\n\t<key>A</key>\n\t<dict><key>x</key><true/></dict>\n</dict>\n</plist>"
	extras = Extras{}
	if got := withPlistExtras(plist); got != plist {
		t.Error("no extras should leave the plist alone")
	}
	extras = Extras{InfoPlist: "\t<key>NSCameraUsageDescription</key>\n\t<string>photos</string>"}
	got := withPlistExtras(plist)
	if !strings.Contains(got, "<string>photos</string>\n</dict>\n</plist>") {
		t.Errorf("extras not added to the top-level dictionary:\n%s", got)
	}
	if strings.Count(got, "NSCameraUsageDescription") != 1 {
		t.Error("extras added more than once")
	}
}

func TestAndroidManifest_ActivityXML(t *testing.T) {
	out, err := androidManifest(manifestData{
		AppID: "com.example.app", Version: Semver{1, 0, 0, 1}, MinSDK: 21, TargetSDK: 35, AppName: "App",
		ActivityXML: `<intent-filter android:autoVerify="true"><action android:name="android.intent.action.VIEW"/><data android:scheme="https" android:host="example.com"/></intent-filter>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		App struct {
			Activity struct {
				Filters []struct {
					AutoVerify string `xml:"autoVerify,attr"`
				} `xml:"intent-filter"`
			} `xml:"activity"`
		} `xml:"application"`
	}
	if err := xml.Unmarshal(out, &m); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if n := len(m.App.Activity.Filters); n != 2 || m.App.Activity.Filters[1].AutoVerify != "true" {
		t.Errorf("activity has %d intent filters (%+v), want the launcher's and the App Link's", n, m.App.Activity.Filters)
	}
}

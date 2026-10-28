package config

import (
	"fmt"
	"sort"
	"strings"
)

// Privacy describes what the app collects and which "required reason"
// APIs it uses, for the privacy manifest (PrivacyInfo.xcprivacy) Apple
// requires of App Store apps. Go programs touch two such APIs on their
// own - the runtime's clock reads the system boot time and the os
// package reads file timestamps - so giode always declares those, with
// the reasons that fit an app using its own files and measuring time.
type Privacy struct {
	// Tracking is true when the app tracks users as Apple defines it,
	// with TrackingDomains the domains it contacts for that.
	Tracking        bool     `toml:"tracking"`
	TrackingDomains []string `toml:"tracking_domains"`
	// Collected are the kinds of data the app collects.
	Collected []CollectedData `toml:"collected"`
	// API adds required-reason APIs and reasons: category to reason
	// codes, by short name (file_timestamp, system_boot_time,
	// disk_space, active_keyboards, user_defaults) or Apple's name.
	API map[string][]string `toml:"api"`
}

// CollectedData is a kind of data the app collects.
type CollectedData struct {
	// Type is Apple's data type, by short name (email_address,
	// precise_location, crash_data, ...) or full name
	// (NSPrivacyCollectedDataTypeEmailAddress).
	Type string `toml:"type"`
	// Linked is true when the data is linked to the user's identity;
	// Tracking when it's used to track them.
	Linked   bool `toml:"linked"`
	Tracking bool `toml:"tracking"`
	// Purposes, by short name (app_functionality, analytics,
	// product_personalization, developer_advertising,
	// third_party_advertising, other) or full name.
	Purposes []string `toml:"purposes"`
}

var (
	privacyDataTypes = []string{"Name", "EmailAddress", "PhoneNumber", "PhysicalAddress", "OtherUserContactInfo",
		"Health", "Fitness", "PaymentInfo", "CreditInfo", "OtherFinancialInfo", "PreciseLocation", "CoarseLocation",
		"SensitiveInfo", "Contacts", "EmailsOrTextMessages", "PhotosorVideos", "AudioData", "GameplayContent",
		"CustomerSupport", "OtherUserContent", "BrowsingHistory", "SearchHistory", "UserID", "DeviceID",
		"PurchaseHistory", "ProductInteraction", "AdvertisingData", "OtherUsageData", "CrashData",
		"PerformanceData", "OtherDiagnosticData", "EnvironmentScanning", "Hands", "Head", "OtherDataTypes"}
	privacyPurposes = []string{"ThirdPartyAdvertising", "DeveloperAdvertising", "Analytics",
		"ProductPersonalization", "AppFunctionality", "Other"}
	privacyAPIs = []string{"FileTimestamp", "SystemBootTime", "DiskSpace", "ActiveKeyboards", "UserDefaults"}
)

// privacyName resolves a short or full name to Apple's full name: the
// prefix and one of names, matched ignoring case and underscores.
func privacyName(prefix string, names []string, v string) (string, bool) {
	norm := func(s string) string { return strings.ToLower(strings.ReplaceAll(s, "_", "")) }
	v = strings.TrimPrefix(v, prefix)
	for _, n := range names {
		if norm(n) == norm(v) {
			return prefix + n, true
		}
	}
	return "", false
}

func (p Privacy) validate() []error {
	var errs []error
	for i, c := range p.Collected {
		if _, ok := privacyName("NSPrivacyCollectedDataType", privacyDataTypes, c.Type); !ok {
			errs = append(errs, fmt.Errorf("privacy.collected[%d].type %q is unknown; see Apple's NSPrivacyCollectedDataType values", i, c.Type))
		}
		for _, pu := range c.Purposes {
			if _, ok := privacyName("NSPrivacyCollectedDataTypePurpose", privacyPurposes, pu); !ok {
				errs = append(errs, fmt.Errorf("privacy.collected[%d] purpose %q is unknown", i, pu))
			}
		}
	}
	for k := range p.API {
		if _, ok := privacyName("NSPrivacyAccessedAPICategory", privacyAPIs, k); !ok {
			errs = append(errs, fmt.Errorf("privacy.api %q is unknown; known: file_timestamp, system_boot_time, disk_space, active_keyboards, user_defaults", k))
		}
	}
	return errs
}

// PrivacyManifest returns the PrivacyInfo.xcprivacy for iOS and macOS
// bundles.
func (c *Config) PrivacyManifest() ([]byte, error) {
	p := c.Privacy
	apis := map[string][]string{
		"NSPrivacyAccessedAPICategoryFileTimestamp":  {"C617.1"},
		"NSPrivacyAccessedAPICategorySystemBootTime": {"35F9.1"},
	}
	for k, reasons := range p.API {
		name, _ := privacyName("NSPrivacyAccessedAPICategory", privacyAPIs, k)
		apis[name] = dedupe(append(apis[name], reasons...))
	}
	names := make([]string, 0, len(apis))
	for k := range apis {
		names = append(names, k)
	}
	sort.Strings(names)
	var accessed []any
	for _, n := range names {
		accessed = append(accessed, map[string]any{
			"NSPrivacyAccessedAPIType":        n,
			"NSPrivacyAccessedAPITypeReasons": apis[n],
		})
	}
	var collected []any
	for _, d := range p.Collected {
		t, _ := privacyName("NSPrivacyCollectedDataType", privacyDataTypes, d.Type)
		var purposes []any
		for _, pu := range d.Purposes {
			n, _ := privacyName("NSPrivacyCollectedDataTypePurpose", privacyPurposes, pu)
			purposes = append(purposes, n)
		}
		collected = append(collected, map[string]any{
			"NSPrivacyCollectedDataType":         t,
			"NSPrivacyCollectedDataTypeLinked":   d.Linked,
			"NSPrivacyCollectedDataTypeTracking": d.Tracking,
			"NSPrivacyCollectedDataTypePurposes": purposes,
		})
	}
	body, err := plistXML(map[string]any{
		"NSPrivacyTracking":           p.Tracking,
		"NSPrivacyTrackingDomains":    append([]string{}, p.TrackingDomains...),
		"NSPrivacyCollectedDataTypes": append([]any{}, collected...),
		"NSPrivacyAccessedAPITypes":   accessed,
	})
	if err != nil {
		return nil, err
	}
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
` + body + `
</dict>
</plist>
`), nil
}

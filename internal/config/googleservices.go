package config

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"strings"
)

// googleServices is the part of google-services.json Firebase reads.
type googleServices struct {
	ProjectInfo struct {
		ProjectNumber string `json:"project_number"`
		ProjectID     string `json:"project_id"`
		StorageBucket string `json:"storage_bucket"`
		FirebaseURL   string `json:"firebase_url"`
	} `json:"project_info"`
	Client []struct {
		ClientInfo struct {
			AppID   string `json:"mobilesdk_app_id"`
			Android struct {
				Package string `json:"package_name"`
			} `json:"android_client_info"`
		} `json:"client_info"`
		OAuth []struct {
			ID   string `json:"client_id"`
			Type int    `json:"client_type"`
		} `json:"oauth_client"`
		APIKey []struct {
			Key string `json:"current_key"`
		} `json:"api_key"`
	} `json:"client"`
}

// GoogleServicesXML turns android.google_services into the string
// resources Firebase initializes from, as Google's Gradle plugin does.
// It's empty without the file.
func (c *Config) GoogleServicesXML() (string, error) {
	if c.Android.GoogleServices == "" {
		return "", nil
	}
	b, err := os.ReadFile(c.Path(c.Android.GoogleServices))
	if err != nil {
		return "", err
	}
	return googleServicesXML(b, c.App.ID)
}

func googleServicesXML(b []byte, appID string) (string, error) {
	var g googleServices
	if err := json.Unmarshal(b, &g); err != nil {
		return "", fmt.Errorf("google-services.json: %w", err)
	}
	for _, cl := range g.Client {
		if cl.ClientInfo.Android.Package != appID {
			continue
		}
		res := [][2]string{
			{"google_app_id", cl.ClientInfo.AppID},
			{"gcm_defaultSenderId", g.ProjectInfo.ProjectNumber},
			{"project_id", g.ProjectInfo.ProjectID},
			{"google_storage_bucket", g.ProjectInfo.StorageBucket},
			{"firebase_database_url", g.ProjectInfo.FirebaseURL},
		}
		if len(cl.APIKey) > 0 {
			res = append(res, [2]string{"google_api_key", cl.APIKey[0].Key}, [2]string{"google_crash_reporting_api_key", cl.APIKey[0].Key})
		}
		for _, o := range cl.OAuth {
			if o.Type == 3 {
				res = append(res, [2]string{"default_web_client_id", o.ID})
				break
			}
		}
		var s strings.Builder
		s.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<resources>\n")
		for _, r := range res {
			if r[1] != "" {
				fmt.Fprintf(&s, "\t<string name=%q translatable=\"false\">%s</string>\n", r[0], html.EscapeString(r[1]))
			}
		}
		s.WriteString("</resources>\n")
		return s.String(), nil
	}
	return "", fmt.Errorf("google-services.json has no Android app %s; add it in the Firebase console and download the file again", appID)
}

package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

type ChromeManifest struct {
	// Only lowercase alphanums, underscores, and dots are allowed.
	Name        string `json:"name"`
	Description string `json:"description"`
	// Path to host binary.
	Path string `json:"path"`

	// Must be "stdio".
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

func (m *ChromeManifest) SetExtensionId(extensionId string) error {
	if extensionId == "" {
		return errors.New("no extension ID")
	}
	m.AllowedOrigins = []string{formatExtensionOrigin(extensionId)}
	return nil
}

func (m *ChromeManifest) Install() error {
	if err := m.init(); err != nil {
		return err
	}
	return m.install()
}

func (m *ChromeManifest) init() error {
	m.Type = "stdio"

	if m.Path == "" {
		m.Path = os.Args[0]
		log.Print("no binary specified, using current binary")
	}
	if _, err := os.Stat(m.Path); os.IsNotExist(err) {
		return fmt.Errorf("binary not found at %s", m.Path)
	}
	var err error
	m.Path, err = filepath.Abs(m.Path)
	if err != nil {
		return err
	}

	if len(m.AllowedOrigins) == 0 {
		return errors.New("no allowed origins")
	}

	return nil
}

func (m *ChromeManifest) install() error {
	path, err := getManifestPath(m.Name)
	if err != nil {
		return err
	}

	manifestData, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	manifestData = append(manifestData, '\n')
	log.Printf("Manifest:\n%s", string(manifestData))

	log.Printf("writing manifest to %s", path)
	if err := ioutil.WriteFile(path, manifestData, 0600); err != nil {
		return err
	}

	log.Println("manifest successfully installed.")
	return nil
}

func getManifestPath(packageName string) (path string, err error) {
	user, _ := user.Current()
	switch runtime.GOOS {
	case "darwin":
		if user != nil {
			path = fmt.Sprintf("%s/Library/Application Support/Google/Chrome/NativeMessagingHosts/%s.json", user.HomeDir, packageName)
		} else {
			path = fmt.Sprintf("/Library/Google/Chrome/NativeMessagingHosts/%s.json", packageName)
		}
	case "linux":
		if user != nil {
			path = fmt.Sprintf("%s/.config/google-chrome/NativeMessagingHosts/%s.json", user.HomeDir, packageName)
		} else {
			path = fmt.Sprintf("/etc/opt/chrome/native-messaging-hosts/%s.json", packageName)
		}
	default:
		err = fmt.Errorf("not sure where to install manifest file on platform %s", runtime.GOOS)
	}
	return
}

func formatExtensionOrigin(extensionId string) string {
	return fmt.Sprintf("chrome-extension://%s/", extensionId)
}

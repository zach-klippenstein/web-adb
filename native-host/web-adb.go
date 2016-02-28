/*
For more info on Chrome Native Messaging, see https://developer.chrome.com/extensions/nativeMessaging.
*/
package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"log/syslog"
	"os"
	"os/user"
	"path/filepath"
	"runtime"

	"github.com/zach-klippenstein/goadb"
)

var (
	install     = flag.Bool("install", false, "Install the native messaging host manifest file.")
	extensionId = flag.String("extension-id", "", "Extension ID to use when installing. Required with -install.")
	binaryPath  = flag.String("path", "", "Path to native host binary. Default is the path to the current executable.")
)

var byteOrder = binary.LittleEndian
var ErrMsgTooLarge = errors.New("message too large")

const (
	// 1 MB
	MaxOutgoingMsgLen = 1024 * 1024
)

var ChromeManifest = struct {
	// Only lowercase alphanums, underscores, and dots are allowed.
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}{
	Name:        "com.zachklipp.adb.nativeproxy",
	Description: "web-adb native messaging proxy",
	Type:        "stdio",
}

type Request struct {
	Command string `json:"command"`
	// Serial of device, or empty to perform on all devices.
	DeviceSerial string `json:"device_serial"`
	Params       json.RawMessage
}

type RunCommandRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type Response struct {
	Success bool `json:"success"`

	// Not set if Success is true.
	Error string `json:"error,omitempty"`

	// The command this is a response to.
	Command string `json:"command"`

	Data interface{} `json:"data,omitempty"`
}

type ListDevicesResponse struct {
	Devices []*adb.DeviceInfo `json:"devices"`
}

// CommandResult is the result of running a shell command on a single device.
type CommandResult struct {
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type RunCommandResponse struct {
	// Map of device serials to command results.
	Results map[string]CommandResult
}

func main() {
	flag.Parse()

	if *install {
		// Running from command line, turn off timestamps.
		log.SetFlags(0)
		if err := doInstallManifest(*extensionId, *binaryPath); err != nil {
			log.Fatal(err)
		}
		return
	}

	var syslogWriter io.Writer
	syslogWriter, err := syslog.New(syslog.LOG_NOTICE, "web-adb")
	if err != nil {
		syslogWriter = os.Stderr
	}
	log.SetOutput(syslogWriter)
	doMain()
}

func doMain() {
	log.Println("web-adb running")

	for {
		msg, err := readMessage(os.Stdin)
		if err == io.EOF {
			log.Println("extension disconnected, exiting")
			return
		}
		if err != nil {
			log.Fatalf("error reading message: %s", err)
		}

		var req Request
		var resp Response

		err = json.Unmarshal(msg, &req)
		if err != nil {
			errMsg := fmt.Sprint("error parsing message: ", err)
			log.Println(errMsg)
			resp.Error = errMsg
		} else {
			respData, err := handleRequest(req)
			if err != nil {
				resp.Error = err.Error()
			} else {
				resp.Success = true
				resp.Data = respData
			}
		}

		sendResponse(req, resp)
	}
}

func handleRequest(req Request) (interface{}, error) {
	server, err := adb.NewServer(adb.ServerConfig{})
	if err != nil {
		return nil, fmt.Errorf("error connecting to adb: %v", err)
	}

	switch req.Command {
	case "list-devices":
		client := adb.NewHostClient(server)
		devices, err := client.ListDevices()
		if err != nil {
			return nil, err
		}
		return ListDevicesResponse{devices}, nil

	case "run-command":
		var params RunCommandRequest
		err = json.Unmarshal(req.Params, &params)
		if err != nil {
			return nil, fmt.Errorf("invalid params: %s", string(req.Params))
		}

		var resp RunCommandResponse
		resp.Results = make(map[string]CommandResult)
		err = doWithDevice(server, req.DeviceSerial, func(serial string, client *adb.DeviceClient) {
			log.Printf("running command on device %s: %s %s", serial, params.Command, params.Args)
			output, err := client.RunCommand(params.Command, params.Args...)
			if err != nil {
				resp.Results[serial] = CommandResult{Error: err.Error()}
			}
			resp.Results[serial] = CommandResult{Output: output}
		})
		return resp, err

	default:
		return nil, fmt.Errorf("unrecognized command: %s", req.Command)
	}
}

func sendResponse(req Request, resp Response) {
	resp.Command = req.Command
	msg := marshal(resp)
	err := sendMessage(msg, os.Stdout)
	if err == ErrMsgTooLarge {
		log.Printf("message too large: %s", string(msg))
		sendResponse(req, Response{
			Error: "message too large",
		})
	} else if err != nil {
		log.Fatalf("error sending message: %s", err)
	}
}

func marshal(resp interface{}) []byte {
	msg, err := json.Marshal(resp)
	if err != nil {
		log.Fatalf("error encoding response. resp=%+v, err=%s", resp, err)
	}
	return msg
}

func doWithDevice(server adb.Server, deviceSerial string, action func(string, *adb.DeviceClient)) error {
	if deviceSerial == "" {
		// All devices.
		client := adb.NewHostClient(server)
		devices, err := client.ListDeviceSerials()
		if err != nil {
			return err
		}

		for _, device := range devices {
			if err := doWithDevice(server, device, action); err != nil {
				return err
			}
		}
		return nil
	}

	client := adb.NewDeviceClient(server, adb.DeviceWithSerial(deviceSerial))
	action(deviceSerial, client)
	return nil
}

func readMessage(r io.Reader) ([]byte, error) {
	var msgLen uint32
	if err := binary.Read(r, byteOrder, &msgLen); err != nil {
		return nil, err
	}

	if msgLen < 1 {
		log.Print("read message length of 0")
		return nil, nil
	}

	msgData := make([]byte, msgLen)
	if _, err := io.ReadFull(r, msgData); err != nil {
		return nil, err
	}
	return msgData, nil
}

func sendMessage(msg []byte, w io.Writer) error {
	msgLen := uint32(len(msg))
	if msgLen > MaxOutgoingMsgLen {
		return ErrMsgTooLarge
	}

	if err := binary.Write(w, byteOrder, msgLen); err != nil {
		return err
	}

	if _, err := w.Write(msg); err != nil {
		return err
	}
	return nil
}

func doInstallManifest(extensionId, binaryPath string) error {
	if err := initManifest(extensionId, binaryPath); err != nil {
		return err
	}

	return installManifest()
}

func initManifest(extensionId, binaryPath string) error {
	if binaryPath == "" {
		binaryPath = os.Args[0]
		log.Printf("no binary specified, using current binary: %s", binaryPath)
	}
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return fmt.Errorf("binary not found at %s", binaryPath)
	}
	binaryPath, err := filepath.Abs(binaryPath)
	if err != nil {
		return err
	}
	ChromeManifest.Path = binaryPath

	if extensionId == "" {
		return errors.New("no extension ID")
	}
	ChromeManifest.AllowedOrigins = []string{formatExtensionOrigin(extensionId)}
	return nil
}

func installManifest() error {
	path, err := getManifestPath(ChromeManifest.Name)
	if err != nil {
		return err
	}

	manifestData, err := json.MarshalIndent(ChromeManifest, "", "  ")
	if err != nil {
		return err
	}
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

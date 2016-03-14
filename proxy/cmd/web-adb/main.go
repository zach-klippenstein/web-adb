/*
For more info on Chrome Native Messaging, see https://developer.chrome.com/extensions/nativeMessaging.
*/
package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"log/syslog"
	"os"

	adb "github.com/zach-klippenstein/goadb"
	"github.com/zach-klippenstein/web-adb/proxy"
)

var (
	install    = flag.String("install", "", "Install the native messaging host manifest file. Connections will only be allowed from `extension-id`.")
	binaryPath = flag.String("path", "", "Path to native host binary. Default is the path to the current executable.")
)

var Manifest = proxy.ChromeManifest{
	Name:        "com.zachklipp.adb.nativeproxy",
	Description: "web-adb native messaging proxy",
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

	if *install != "" {
		// Running from command line, turn off timestamps.
		log.SetFlags(0)
		Manifest.Path = *binaryPath
		Manifest.SetExtensionId(*install)

		if err := Manifest.Install(); err != nil {
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
	log.Println("web-adb starting...")

	httpServer, err := proxy.NewAdbHttpProxy()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("adb proxy listening on", httpServer.Addr())

	proxy.SendMessage(os.Stdout, httpServer.Addr())
	err = httpServer.Serve()

	log.Println("port closed, stopping http server and exiting with", err)
	log.Fatal(err)
}

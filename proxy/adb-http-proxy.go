package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	adb "github.com/zach-klippenstein/goadb"
)

type AdbHttpProxy struct {
	adbServer adb.Server
	router    *mux.Router
	listener  net.Listener
}

func NewAdbHttpProxy() (*AdbHttpProxy, error) {
	adbServer, err := adb.NewServer(adb.ServerConfig{
		PathToAdb: "/Users/zach/android-sdk/platform-tools/adb",
	})
	if err != nil {
		return nil, err
	}

	proxy := &AdbHttpProxy{
		adbServer: adbServer,
	}

	r := mux.NewRouter()
	r.HandleFunc("/devices", HandleEventSource(proxy.watchDevices)).Headers("Accept", "text/event-stream").Methods("GET", "HEAD")
	r.HandleFunc("/devices", proxy.listDevices).Methods("GET", "HEAD")
	r.HandleFunc("/devices/{serial}", proxy.deviceInfo).Methods("GET", "HEAD")
	r.HandleFunc("/devices/{serial}/files/{path:.*}", proxy.deviceFiles).Methods("GET", "HEAD", "POST").Name("files")
	r.HandleFunc("/devices/{serial}/execute", proxy.runCommand).Methods("POST")
	proxy.router = r

	// Port 0 means choose any available port.
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}
	proxy.listener = listener

	return proxy, nil
}

func (p *AdbHttpProxy) Addr() string {
	return p.listener.Addr().String()
}

func (p *AdbHttpProxy) Serve() error {
	return http.Serve(p.listener, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		log.Printf("[%s] %s %s", req.RemoteAddr, req.Method, req.URL)

		w.Header().Add("Cache-Control", "no-cache")
		w.Header().Add("Access-Control-Allow-Origin", "*")

		p.router.ServeHTTP(w, req)
	}))
}

func (p *AdbHttpProxy) listDevices(w http.ResponseWriter, req *http.Request) {
	client := adb.NewHostClient(p.adbServer)
	devices, err := client.ListDevices()

	log.Printf("got devices list: %#v %+v", devices, devices)

	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	writeJson(w, devices)
}

type DeviceEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

func (p *AdbHttpProxy) watchDevices(w *EventSource, req *http.Request) {
	log.Println("starting device watcher...")

	var err error
	watcher := adb.NewDeviceWatcher(p.adbServer)
	defer watcher.Shutdown()

	for {
		e, ok := <-watcher.C()
		if !ok {
			if err = watcher.Err(); err != nil {
				// Error reading event from ADB.
				w.SendJSON(DeviceEvent{"error", err})
				return
			}
		}

		if e.CameOnline() {
			log.Println("device connected:", e)
			err = w.SendJSON(DeviceEvent{"connected", e})
		} else if e.WentOffline() {
			log.Println("device disconnected:", e)
			err = w.SendJSON(DeviceEvent{"disconnected", e})
		} else {
			log.Println("unrecognized device event:", e)
		}
		if err != nil {
			// Error sending event to extension.
			log.Printf("error sending event: event=%#v err=%v", e, err)
			return
		}
	}

}

func (p *AdbHttpProxy) deviceInfo(w http.ResponseWriter, req *http.Request) {
	serial := mux.Vars(req)["serial"]

	filesUrl, err := p.router.Get("files").URL(
		"serial", serial,
		"path", "",
	)
	if err != nil {
		log.Fatal(err)
	}

	http.Redirect(w, req, filesUrl.String(), http.StatusSeeOther)
}

func (p *AdbHttpProxy) deviceFiles(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	serial := vars["serial"]
	path := "/" + vars["path"]

	switch req.Method {
	case "GET", "HEAD":
		p.getFile(w, serial, path)
	case "POST":
		p.uploadFile(w, serial, path, req)
	default:
		writeError(w, http.StatusMethodNotAllowed, nil)
	}
}

func (p *AdbHttpProxy) getFile(w http.ResponseWriter, serial, path string) {
	client := adb.NewDeviceClient(p.adbServer, adb.DeviceWithSerial(serial))

	// First stat the file to see if it's a directory.
	target, err := client.Stat(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if target.Mode.IsRegular() {
		// Stream file contents.
		p.downloadFile(w, client, target, serial, path)
	} else {
		p.listFiles(w, client, serial, path)
	}
}

func (p *AdbHttpProxy) listFiles(w http.ResponseWriter, client *adb.DeviceClient, serial, path string) {
	log.Printf("listing files at %s:%s", serial, path)

	entries, err := client.ListDirEntries(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	defer entries.Close()

	var allEntries []*adb.DirEntry
	for entries.Next() {
		allEntries = append(allEntries, entries.Entry())
	}
	if err := entries.Err(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	writeJson(w, allEntries)
}

func (p *AdbHttpProxy) downloadFile(w http.ResponseWriter, client *adb.DeviceClient, target *adb.DirEntry, serial, path string) {
	log.Printf("downloading %s:%s", serial, path)

	// We don't know the content type, so assume binary.
	w.Header().Set("Content-Type", "application/octet-stream")
	if target.Size > 0 {
		// Don't send 0 size because it may be a device.
		w.Header().Set("Content-Length", strconv.Itoa(int(target.Size)))
	}

	var modifiedAt time.Time
	if target.ModifiedAt.IsZero() {
		modifiedAt = time.Now()
	} else {
		modifiedAt = target.ModifiedAt
	}
	w.Header().Set("Date", modifiedAt.Format(time.RFC3339))

	stream, err := client.OpenRead(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	defer stream.Close()

	n, err := io.Copy(w, stream)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("error downloading file after %d bytes: %s", n, err))
	}
}

func (p *AdbHttpProxy) uploadFile(w http.ResponseWriter, serial, path string, req *http.Request) {
	log.Printf("uploading file to %s:%s", serial, path)
	log.Print(req.Header)

	defer req.Body.Close()

	client := adb.NewDeviceClient(p.adbServer, adb.DeviceWithSerial(serial))
	fileStream, err := client.OpenWrite(path, 0644, adb.MtimeOfClose)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	defer fileStream.Close()

	n, err := io.Copy(fileStream, req.Body)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("error uploading file after %d bytes: %s", n, err))
		return
	}

	writeJson(w, map[string]interface{}{
		"length": n,
	})
}

type RunCommandRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func (p *AdbHttpProxy) runCommand(w http.ResponseWriter, req *http.Request) {
	log.Println(*req)

	//dataRaw, _ := ioutil.ReadAll(req.Body)
	//log.Println(string(dataRaw))

	var data RunCommandRequest

	if err := json.NewDecoder(req.Body).Decode(&data); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if data.Command == "" {
		writeError(w, http.StatusBadRequest, errors.New("no command specified"))
		return
	}

	log.Println("running command:", data)

	serial := mux.Vars(req)["serial"]
	client := adb.NewDeviceClient(p.adbServer, adb.DeviceWithSerial(serial))
	output, err := client.RunCommand(data.Command, data.Args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Write([]byte(output))
}

func writeJson(w http.ResponseWriter, v interface{}) {
	log.Printf("marshalling %#v", v)

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	log.Println("sending", string(data))

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func writeError(w http.ResponseWriter, code int, err error) {
	var errStr string
	if err != nil {
		errStr = err.Error()
	} else {
		errStr = http.StatusText(code)
	}

	log.Println("error:", errStr)
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(code)
	io.WriteString(w, errStr)
}

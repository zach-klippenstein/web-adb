// Copyright (c) 2010 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Context menu docs: https://developer.chrome.com/extensions/contextMenus#method-create
// Native messaging docs: https://developer.chrome.com/extensions/nativeMessaging

// Connection to native host.
var port = null;

const HOST_ADDRESS = "com.zachklipp.adb.nativeproxy";
const ALL_CONTEXTS = ["page","selection","link","editable","image","video","audio"];

function parseFilenameFromUrl(url) {
  var parts = url.split("/");
  return parts[parts.length-1];
}

function runCommand(device, command, args) {
    var serial = device ? device.Serial : null;
    port.postMessage({
      "command": "run-command",
      "device_serial": serial,
      "params": {
        "command": command,
        "args": args
      }
    });
}

function downloadUrl(device, url) {
  var serial = device ? device.Serial : null;
  fetch(url).then(resp => {
    if (!resp.ok) {
      console.log(`request failed for ${url}:`, resp);
      return;
    }

    var streamId = null;
    var reader = resp.body.getReader();
    var streamPort = chrome.runtime.connectNative(HOST_ADDRESS);
    var chunkIndex = 0;

    var closeStream = function() {
      streamPort.postMessage({
        "command": "push-chunk",
        "params": {
          "stream_id": streamId,
          "eof": true,
        }
      });
      // Don't close port, wait for stream closed message.
    }

    var sendNextChunk = function() {
      reader.read().then(result => {
        if (result.done) {
          console.log(`stream ${streamId} finished, closing.`);
          closeStream();
          return;
        }

        console.log(`sending stream ${streamId} chunk ${chunkIndex}…`, result.value);
        base64Data = btoa(result.value);
        streamPort.postMessage({
          "command": "push-chunk",
          "params": {
            "stream_id": streamId,
            "chunk_index": chunkIndex,
            "data": base64Data,
          }
        });
      }).catch(error => {
        console.log(`error reading for push stream ${streamId}, closing.`);
        closeStream();
      });
    }

    streamPort.onMessage.addListener(function(msg) {
      if (!msg.success) {
        console.log(`push stream ${streamId} error:`, msg);
        // Failed to open stream or invalid stream ID.
        streamPort.disconnect();
        return;
      }

      if (msg.command == "push-file") {
        streamId = msg.data.stream_id;
        console.log(`push stream ${streamId} opened:`, msg);
        sendNextChunk();
      } else if (msg.command == "push-chunk") {
        if (msg.data.eof) {
          console.log(`push stream ${streamId} finished, disconnecting`, msg.data);
          streamPort.disconnect();
          return;
        }

        if (!msg.data.success) {
          console.log(`push stream ${streamId} error:`, msg.data);
          // Give up.
          closeStream();
        }

        chunkIndex++;
        sendNextChunk();
      }
    });

    streamPort.onDisconnect.addListener(function() {
      console.log(`stream ${streamId} disconnected`)
    });

    var filename = parseFilenameFromUrl(url);
    var devicePath = "/sdcard/Download/" + filename;
    console.log(`downloading ${url} to ${devicePath}…`, resp);
    streamPort.postMessage({
      "command": "push-file",
      "params": {
        "device_path": devicePath
      }
    });
  }).catch(error => console.log(`request failed for ${url}:`, error));
}

function openSelfOnDevice(device) {
  return function(info) {
    runCommand(device, "am", ["start", "-a", "android.intent.action.VIEW", "-d", info.pageUrl]);
  };
}

function openLinkOnDevice(device) {
  return function(info) {
    runCommand(device, "am", ["start", "-a", "android.intent.action.VIEW", "-d", info.linkUrl]);
  };
}

function saveLinkOnDevice(device) {
  return function(info) {
    downloadUrl(device, info.linkUrl);
  };
}

function createContextMenuForDevice(device) {
  var title = device ? device.Model : "All Devices"

  deviceMenuId = chrome.contextMenus.create({
    "title": title,
    "contexts": ALL_CONTEXTS
  });
  chrome.contextMenus.create({
    "title": "Open this page",
    "contexts": ALL_CONTEXTS,
    "parentId": deviceMenuId,
    "onclick": openSelfOnDevice(device)
  });
  chrome.contextMenus.create({
    "title": "Open link",
    "contexts": ["link"],
    "parentId": deviceMenuId,
    "onclick": openLinkOnDevice(device)
  });
  chrome.contextMenus.create({
    "title": "Save link",
    "contexts": ["link"],
    "parentId": deviceMenuId,
    "onclick": saveLinkOnDevice(device),
  });
}

function rebuildContextMenus(devices) {
  chrome.contextMenus.removeAll(function() {
    if (devices.length > 1) {
      // Prepend All Devices menu.
      createContextMenuForDevice(null);
    }

    for (var device of devices) {
      createContextMenuForDevice(device);
    }
  });
}

function handleResponse(resp) {
  if (resp.command === "list-devices") {
    var devices = resp.data.devices;
    rebuildContextMenus(devices);
  }
}

port = chrome.runtime.connectNative(HOST_ADDRESS);
port.onMessage.addListener(function(msg) {
  if (msg.success) {
    console.log("command successful: ", msg)
    handleResponse(msg)
  } else {
    console.log("command failed: ", msg.error)
  }
});
port.onDisconnect.addListener(function() {
  console.log("Port Disconnected");
});
port.postMessage({ command: "list-devices" });

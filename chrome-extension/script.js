// Copyright (c) 2010 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Context menu docs: https://developer.chrome.com/extensions/contextMenus#method-create
// Native messaging docs: https://developer.chrome.com/extensions/nativeMessaging

// Connection to native host.
var port = null;

const ALL_CONTEXTS = ["page","selection","link","editable","image","video","audio"];

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

function openSelfOnDevice(device) {
  return function(info) {
    var serial = device ? device.Serial : null
    runCommand(device, "am", ["start", "-a", "android.intent.action.VIEW", "-d", info.pageUrl]);
  };
}

function openLinkOnDevice(device) {
  return function(info) {
    var serial = device ? device.Serial : null
    runCommand(device, "am", ["start", "-a", "android.intent.action.VIEW", "-d", info.linkUrl]);
  };
}

function createContextMenuForDevice(device) {
  var title = device ? device.Model : "All Devices"

  deviceMenuId = chrome.contextMenus.create({
    "title": title,
    "contexts": ["page","link"]
  });
  chrome.contextMenus.create({
    "title": "Open this page on device",
    "contexts": ALL_CONTEXTS,
    "parentId": deviceMenuId,
    "onclick": openSelfOnDevice(device)
  })
  chrome.contextMenus.create({
    "title": "Open link on device",
    "contexts": ["link"],
    "parentId": deviceMenuId,
    "onclick": openLinkOnDevice(device)
  })
}

function rebuildContextMenus(devices) {
  chrome.contextMenus.removeAll(function() {
    createContextMenuForDevice(null);

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

port = chrome.runtime.connectNative("com.zachklipp.adb.nativeproxy");
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

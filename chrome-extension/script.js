// Copyright (c) 2010 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Context menu docs: https://developer.chrome.com/extensions/contextMenus#method-create
// Native messaging docs: https://developer.chrome.com/extensions/nativeMessaging

// Connection to native host.
"use strict";

var port = null;

const ALL_CONTEXTS = ["page","selection","link","editable","image","video","audio"];

function openSelfOnDevice(device) {
  return info => {
    client.runCommand(device, "am", ["start", "-a", "android.intent.action.VIEW", "-d", info.pageUrl]);
  };
}

function openLinkOnDevice(device) {
  return info => {
    client.runCommand(device, "am", ["start", "-a", "android.intent.action.VIEW", "-d", info.linkUrl]);
  };
}

function saveLinkToDevice(device) {
  return info => {
//    client.
  };
}

function createContextMenuForDevice(device) {
  if (!device) {
    throw "null device";
  }

  let title = device.Model
  let deviceMenuId = chrome.contextMenus.create({
    "title": title,
    "contexts": ["page","link"]
  });
  chrome.contextMenus.create({
    "title": "Open this page on device",
    "contexts": ALL_CONTEXTS,
    "parentId": deviceMenuId,
    "onclick": openSelfOnDevice(device)
  });
  chrome.contextMenus.create({
    "title": "Open link on device",
    "contexts": ["link"],
    "parentId": deviceMenuId,
    "onclick": openLinkOnDevice(device)
  });
  chrome.contextMenus.create({
    "title": "Save link to device",
    "contexts": ["link"],
    "parentId": deviceMenuId,
    "onclick": saveLinkToDevice(device)
  });
}

function rebuildContextMenus(devices) {
  chrome.contextMenus.removeAll(function() {
    for (var device of devices) {
      createContextMenuForDevice(device);
    }
  });
}

class ProxyClient {
  constructor() {
    let port = chrome.runtime.connectNative("com.zachklipp.adb.nativeproxy");
    this.port = port;

    this.address = new Promise((resolve, reject) => {
      port.onMessage.addListener(msg => {
        console.log(msg);
        resolve(msg);
      });

      port.onDisconnect.addListener(() => {
        reject("port disconnected");
      });
    });
  }

  path(path) {
    return this.address.then(addr => `http://${addr}${path}`);
  }

  fetch(path, init) {
    return this.path(path).then(addr => fetch(addr, init));
  }

  listDevices() {
    return this.fetch("/devices")
      .then(resp => {
        if (resp.ok) {
          return resp.json();
        }
        throw Promise.resolve(resp.text());
      });
  }

  // Invokes onDeviceCallback with device change events.
  watchDevices(onDeviceCallback, onErrorCallback) {
    this.path("/devices")
      .then(addr => {
        var evtSrc = new EventSource(addr);

        evtSrc.onmessage = e => {
          var data = JSON.parse(e.data);
          if (data.type === "error") {
            onErrorCallback(data.data);
          } else {
            onDeviceCallback(data);
          }
        };

        evtSrc.onerror = onErrorCallback;
      })
      .catch(onErrorCallback);
  }

  runCommand(device, cmd, args) {
    return this.fetch(`/devices/${device.Serial}/execute`, {
      "method": "POST",
      "body": JSON.stringify({
        "command": cmd,
        "args": args
      })
    })
    .then(resp => {
      if (resp.ok) {
        return resp.text();
      }
      throw Promise.resolve(resp.text());
    });
  }
}

let client = new ProxyClient();
client.listDevices().then(r => console.log(r));
client.listDevices().then(devices => rebuildContextMenus(devices));
client.watchDevices(ev => {
  console.log("device connection event:", ev);
  client.listDevices().then(rebuildContextMenus);
}, e => console.log("error listening for device changes", e));

setTimeout(() => console.log(client), 1000);

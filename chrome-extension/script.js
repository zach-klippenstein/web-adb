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

    for (var i = 0; i < devices.length; i++) {
      var device = devices[i];
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

// // A generic onclick callback function.
// function genericOnClick(info, tab) {
//   console.log("item " + info.menuItemId + " was clicked");
//   console.log("info: " + JSON.stringify(info));
//   console.log("tab: " + JSON.stringify(tab));
// }

// // Create one test item for each context type.
// var contexts = ["page","selection","link","editable","image","video",
//                 "audio"];
// for (var i = 0; i < contexts.length; i++) {
//   var context = contexts[i];
//   var title = "Test '" + context + "' menu item";
//   var id = chrome.contextMenus.create({"title": title, "contexts":[context],
//                                        "onclick": genericOnClick});
//   console.log("'" + context + "' item:" + id);
// }


// // Create a parent item and two children.
// var parent = chrome.contextMenus.create({"title": "Test parent item"});
// var child1 = chrome.contextMenus.create(
//   {"title": "Child 1", "parentId": parent, "onclick": genericOnClick});
// var child2 = chrome.contextMenus.create(
//   {"title": "Child 2", "parentId": parent, "onclick": genericOnClick});
// console.log("parent:" + parent + " child1:" + child1 + " child2:" + child2);


// // Create some radio items.
// function radioOnClick(info, tab) {
//   console.log("radio item " + info.menuItemId +
//               " was clicked (previous checked state was "  +
//               info.wasChecked + ")");
// }
// var radio1 = chrome.contextMenus.create({"title": "Radio 1", "type": "radio",
//                                          "onclick":radioOnClick});
// var radio2 = chrome.contextMenus.create({"title": "Radio 2", "type": "radio",
//                                          "onclick":radioOnClick});
// console.log("radio1:" + radio1 + " radio2:" + radio2);


// // Create some checkbox items.
// function checkboxOnClick(info, tab) {
//   console.log(JSON.stringify(info));
//   console.log("checkbox item " + info.menuItemId +
//               " was clicked, state is now: " + info.checked +
//               "(previous state was " + info.wasChecked + ")");

// }
// var checkbox1 = chrome.contextMenus.create(
//   {"title": "Checkbox1", "type": "checkbox", "onclick":checkboxOnClick});
// var checkbox2 = chrome.contextMenus.create(
//   {"title": "Checkbox2", "type": "checkbox", "onclick":checkboxOnClick});
// console.log("checkbox1:" + checkbox1 + " checkbox2:" + checkbox2);


// // Intentionally create an invalid item, to show off error checking in the
// // create callback.
// console.log("About to try creating an invalid item - an error about " +
//             "item 999 should show up");
// chrome.contextMenus.create({"title": "Oops", "parentId":999}, function() {
//   if (chrome.extension.lastError) {
//     console.log("Got expected error: " + chrome.extension.lastError.message);
//   }
// });
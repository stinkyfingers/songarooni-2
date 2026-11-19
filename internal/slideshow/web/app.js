(function () {
  "use strict";

  var layers = [document.getElementById("a"), document.getElementById("b")];
  var activeLayer = 0;
  var videoSuffixes = [".mp4"];

  // Go Live (see plans/http-video.md): the server pushes the flat sentinel
  // "live" over the same SSE channel instead of a slide URL, meaning
  // "point your <img> at this server's own /live-camera route" — a Go
  // proxy that authenticates to the phone's camera app server-side, so no
  // credentials ever reach this page.
  var LIVE_RETRY_MS = 3000; // how soon to retry after the <img> fires "error" (e.g. the camera was never reachable)
  var LIVE_WATCHDOG_MS = 20000; // periodic reconnect regardless of error state — see teardownLive's comment on why onerror alone isn't enough
  var liveWatchdog = null;

  // Closes whatever live viewer is currently showing, if any — a no-op if
  // Live was never started. Called whenever a normal slide arrives, so a
  // stale reconnect timer never lingers once the server moves on to a
  // song, the logo, or Default.
  function teardownLive() {
    if (liveWatchdog) {
      clearInterval(liveWatchdog);
      liveWatchdog = null;
    }
  }

  // Swaps in an <img> pointed at /live-camera, using the same layer-swap
  // showSlide/reveal use for a normal slide. Unlike a normal slide, there's
  // no load event worth waiting for — the image just stays black until the
  // proxy actually has a frame to send — and no dwell timer or
  // notifyAdvance() call, since the server's ShowLive never goes through
  // displayData at all (see its doc comment).
  function startLiveViewer() {
    if (layers[activeLayer].dataset && layers[activeLayer].dataset.live === "1") {
      return; // already showing live; let its own retry/watchdog keep going
    }

    var nextLayer = 1 - activeLayer;
    var oldEl = layers[nextLayer];
    var imgEl = document.createElement("img");
    imgEl.className = "slide";
    imgEl.dataset.live = "1";

    function reload() {
      imgEl.src = "/live-camera?t=" + Date.now(); // cache-bust so this actually re-opens the stream, not a no-op same-URL reload
    }
    imgEl.onerror = function () {
      console.error("live: camera stream error, retrying in " + LIVE_RETRY_MS + "ms");
      setTimeout(reload, LIVE_RETRY_MS);
    };
    reload();

    // A dropped connection mid-stream (the phone's WiFi, not the initial
    // request) doesn't reliably fire an <img>'s error event for a
    // multipart/x-mixed-replace stream — some browsers just freeze on the
    // last frame forever. A periodic forced reconnect, independent of
    // error state, is the real recovery path for that case.
    teardownLive();
    liveWatchdog = setInterval(reload, LIVE_WATCHDOG_MS);

    if (layers[activeLayer].tagName === "VIDEO") {
      layers[activeLayer].pause();
    }
    layers[activeLayer].classList.remove("visible");
    imgEl.classList.add("visible");
    activeLayer = nextLayer;

    oldEl.parentNode.replaceChild(imgEl, oldEl);
    layers[nextLayer] = imgEl;
  }

  function isVideo(url) {
    return videoSuffixes.some(function (s) { return url.toLowerCase().endsWith(s); });
  }

  // Builds whichever element type this slide needs. Both share the "slide"
  // class, so the existing crossfade CSS (position/size/opacity transition)
  // applies identically to images and video — no separate styling needed.
  function makeLayerElement(url) {
    var el;
    if (isVideo(url)) {
      el = document.createElement("video");
      // No autoplay: on slow hardware (the Pi, not a desktop), decoding can
      // lag far enough behind that autoplay has already advanced playback
      // a second or more before our "loadeddata" handler even runs — the
      // video is then revealed mid-playback. Starting playback ourselves in
      // reveal() below, at the exact moment it's actually shown, fixes that.
      el.muted = true;
      el.playsInline = true; // avoid iOS/Safari forcing fullscreen takeover on autoplay
      el.preload = "auto"; // buffer eagerly rather than conservatively — reduces jerky playback on the Pi
    } else {
      el = document.createElement("img");
    }
    el.className = "slide";
    el.src = url;
    return el;
  }

  // Crossfades to url. There's no local deck or timer here — the server
  // decides what to show and when by pushing one URL per SSE message; this
  // just renders whatever arrives.
  function showSlide(url) {
    if (url === "live") {
      startLiveViewer();
      return;
    }
    teardownLive(); // harmless no-op unless we were actually showing Live

    var nextLayer = 1 - activeLayer;
    var oldEl = layers[nextLayer];
    var newEl = makeLayerElement(url);

    var reveal = function () {
      // Force a reflow so the browser commits newEl's current (not yet
      // "visible") state as an actual painted frame before the next line
      // changes it. Without this, a load that resolves fast enough (an
      // already-cached image, a very short video) can have the insertion
      // and the class change coalesced into a single style recalculation —
      // no transition happens, it just snaps straight to the end state.
      void newEl.offsetWidth;
      // Pausing the outgoing element (if it's a video) here, rather than
      // leaving it to decode/play in the background until garbage
      // collected, frees up decode capacity right when it matters most —
      // the crossfade below briefly has two layers on screen at once,
      // and the Pi can barely keep up decoding even one video smoothly.
      if (layers[activeLayer].tagName === "VIDEO") {
        layers[activeLayer].pause();
      }
      layers[activeLayer].classList.remove("visible");
      newEl.classList.add("visible");
      activeLayer = nextLayer;
      notifyAdvance(); // tell the server this slide is actually showing, so its dwell timer starts now
      if (newEl.tagName === "VIDEO") {
        // Explicit start, not autoplay — see makeLayerElement's comment:
        // this guarantees playback visibly begins at frame 0 the moment
        // it's actually revealed, regardless of how long decoding took to
        // get here.
        newEl.currentTime = 0;
        newEl.play();
      }
    };
    // A missing file (404, bad codec, etc.) fires "error" instead of
    // "load"/"loadeddata". There's no local queue to skip ahead to — but
    // we still need to tell the server this slide is a lost cause so it
    // doesn't sit waiting the full maxRevealWait before moving on.
    var onLoadError = function () {
      console.error("Failed to load slide:", url);
      notifyAdvance();
    };
    // Images fire "load"; video elements don't — "loadeddata" is their
    // equivalent "ready to show a frame" event.
    if (newEl.tagName === "VIDEO") {
      newEl.addEventListener("loadeddata", reveal, { once: true });
      newEl.addEventListener("error", onLoadError, { once: true });
    } else {
      newEl.onload = reveal;
      newEl.onerror = onLoadError;
    }

    oldEl.parentNode.replaceChild(newEl, oldEl);
    layers[nextLayer] = newEl; // keep the array pointed at the real current DOM element
  }

  // Pushed by this server's own /events route — each message's data is the
  // next slide's URL to show. No polling: the server alone decides what
  // plays and when, and the page just reacts. EventSource reconnects on
  // its own after a drop, so no manual retry handling is needed here.
  var eventSource = new EventSource("/events");
  eventSource.onopen = function () {
    console.log("SSE connected");
    document.getElementById("error").textContent = "";
  };

  // Listen for the custom "data" event sent by Go (see sseHandler).
  eventSource.addEventListener("data", function (event) {
    showSlide(event.data);
  });

  eventSource.onerror = function () {
    console.log("SSE connection error; the browser will retry automatically");
    document.getElementById("error").textContent = "Failed to connect to server";
  };

  // Tells the server this slide is actually showing (or has definitively
  // failed to), so its dwell timer can start from real reveal time rather
  // than from whenever it pushed the URL — see displayData's doc comment.
  // Fire-and-forget: there's nothing useful to do with the response, and
  // a failed POST here just means the server falls back to its own
  // maxRevealWait timeout instead of hanging.
  function notifyAdvance() {
    fetch("/advance", { method: "POST" }).catch(function () {});
  }
})();
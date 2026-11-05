(function () {
  "use strict";

  var POLL_MS = 750; // matches the control panel's own phone-page poll interval
  var DEFAULT_ID = 0;
  var LOGO_ID = -1;

  var layers = [document.getElementById("a"), document.getElementById("b")];
  var activeLayer = 0;
  var urls = [];
  var slideIdx = 0;
  var cycleTimer = null;
  var lastShownID = null;
  var videoSuffixes = [".mp4"];

  // Fetched once at startup from this server's own /config — apiPort (the
  // main server's port, for the cross-origin /api/state poll below) and
  // intervalMs (how long each slide holds before crossfading, from
  // config.yaml's slide_interval_seconds) aren't known until then.
  var apiPort = null;
  var intervalMs = null;

  function apiURL(path) {
    return "http://" + location.hostname + ":" + apiPort + path;
  }

  // Playing drives the display, not Selected — matching how
  // internal/state's Display.Show is driven (only Start/ToggleLogo call
  // it), and what every real song/Default/Logo transition actually means
  // to someone watching the venue screen rather than the phone.
  function currentID(state) {
    if (state.logoActive) return LOGO_ID;
    if (state.running) return state.playingSongId;
    return DEFAULT_ID;
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
      el.autoplay = true;
      el.loop = true; // safety net only — scheduleNext advances after one real play-through
      el.muted = true;
      el.playsInline = true; // avoid iOS/Safari forcing fullscreen takeover on autoplay
    } else {
      el = document.createElement("img");
    }
    el.className = "slide";
    el.src = url;
    return el;
  }

  // How long el should stay on screen before advancing: a video's own
  // duration (now known — this runs at "loadeddata" time, after
  // "loadedmetadata" has already fired), or the configured interval for
  // everything else, including a video whose duration never resolves to a
  // real number (some files briefly report Infinity/NaN).
  function dwellMs(el) {
    if (el.tagName === "VIDEO" && isFinite(el.duration) && el.duration > 0) {
      return el.duration * 1000;
    }
    return intervalMs;
  }

  function scheduleNext(el) {
    if (cycleTimer) clearTimeout(cycleTimer);
    if (urls.length <= 1) return; // nothing else to cycle to
    cycleTimer = setTimeout(advance, dwellMs(el));
  }

  function showSlide(url) {
    var nextLayer = 1 - activeLayer;
    var oldEl = layers[nextLayer];
    var newEl = makeLayerElement(url);

    var reveal = function () {
      layers[activeLayer].classList.remove("visible");
      newEl.classList.add("visible");
      activeLayer = nextLayer;
      scheduleNext(newEl); // dwell time starts now, once it's actually showing
    };
    // Images fire "load"; video elements don't — "loadeddata" is their
    // equivalent "ready to show a frame" event, and by then .duration is
    // already known.
    if (newEl.tagName === "VIDEO") {
      newEl.addEventListener("loadeddata", reveal, { once: true });
    } else {
      newEl.onload = reveal;
    }

    oldEl.parentNode.replaceChild(newEl, oldEl);
    layers[nextLayer] = newEl; // keep the array pointed at the real current DOM element
  }

  function advance() {
    if (urls.length === 0) return;
    var url = urls[slideIdx];
    slideIdx = (slideIdx + 1) % urls.length;
    showSlide(url);
  }

  function restartCycle() {
    if (cycleTimer) clearTimeout(cycleTimer);
    slideIdx = 0;
    advance(); // shows the first slide; its own reveal() schedules the next
  }

  function loadSlides(id) {
    fetch("/slides?id=" + id)
      .then(function (res) { return res.json(); })
      .then(function (got) {
        urls = got;
        restartCycle();
      })
      .catch(function () { /* transient fetch error; keep showing whatever's up */ });
  }

  function poll() {
    fetch(apiURL("/api/state"))
      .then(function (res) { return res.json(); })
      .then(function (state) {
        var id = currentID(state);
        if (id !== lastShownID) {
          lastShownID = id;
          loadSlides(id);
        }
      })
      .catch(function () { /* main server unreachable momentarily; keep showing whatever's up */ });
  }

  fetch("/config")
    .then(function (res) { return res.json(); })
    .then(function (cfg) {
      apiPort = cfg.apiPort;
      intervalMs = cfg.intervalMs;
      poll();
      setInterval(poll, POLL_MS);
    });
})();

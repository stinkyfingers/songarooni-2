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

  function showSlide(url) {
    var nextLayer = 1 - activeLayer;
    var img = layers[nextLayer];
    img.onload = function () {
      layers[activeLayer].classList.remove("visible");
      img.classList.add("visible");
      activeLayer = nextLayer;
    };
    img.src = url;
  }

  function advance() {
    if (urls.length === 0) return;
    showSlide(urls[slideIdx]);
    slideIdx = (slideIdx + 1) % urls.length;
  }

  function restartCycle() {
    if (cycleTimer) clearInterval(cycleTimer);
    slideIdx = 0;
    advance();
    if (urls.length > 1) {
      cycleTimer = setInterval(advance, intervalMs);
    }
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

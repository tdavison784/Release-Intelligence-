// Review UI — vanilla JS, no dependencies. Everything degrades: without JS the
// inbox lists items, each links to its page, and every decision is a plain form post.
(function () {
  "use strict";
  var $ = function (s, r) { return (r || document).querySelector(s); };
  var $$ = function (s, r) { return Array.prototype.slice.call((r || document).querySelectorAll(s)); };

  // ---- theme --------------------------------------------------------------
  var themeBtn = $("#theme");
  if (themeBtn) themeBtn.addEventListener("click", function () {
    var cur = document.documentElement.getAttribute("data-theme");
    if (!cur) cur = matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
    var next = cur === "dark" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", next);
    try { localStorage.setItem("ri-theme", next); } catch (e) {}
  });

  // ---- help ---------------------------------------------------------------
  var help = $("#help");
  function toggleHelp(show) { if (help) help.hidden = show === undefined ? !help.hidden : !show; }
  var hb = $("#helpbtn"); if (hb) hb.addEventListener("click", function () { toggleHelp(); });
  if (help) help.addEventListener("click", function (e) { if (e.target === help) toggleHelp(false); });

  // ---- reviewer -----------------------------------------------------------
  var rev = $("#reviewer");
  function setReviewer(v) {
    document.cookie = "ri_reviewer=" + encodeURIComponent(v) + "; path=/; max-age=31536000; samesite=strict";
    $$('input[name="reviewer"]').forEach(function (i) { if (i !== rev) i.value = v; });
  }
  if (rev) rev.addEventListener("input", function () { setReviewer(rev.value.trim()); });

  // ---- cards: expand / collapse -------------------------------------------
  var cards = $$(".card");
  function focusCard(c, scroll) {
    cards.forEach(function (x) { x.classList.remove("focus"); });
    if (!c) return;
    c.classList.add("focus");
    if (scroll !== false) c.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }
  function focused() { return $(".card.focus"); }

  function expand(card) {
    var btn = $(".toggle", card), inner = $(".detail-inner", card);
    if (card.classList.contains("open")) return Promise.resolve();
    card.classList.add("open"); btn.setAttribute("aria-expanded", "true");
    if (inner.dataset.loaded) return Promise.resolve();
    inner.innerHTML = '<div class="loading">Loading…</div>';
    return fetch(btn.dataset.detail, { credentials: "same-origin" }).then(function (r) {
      if (!r.ok) throw new Error(r.status); return r.text();
    }).then(function (html) { inner.innerHTML = html; inner.dataset.loaded = "1"; syncReviewer(); initKinds(inner); })
      .catch(function (e) { inner.innerHTML = '<div class="flash flash-error">Could not load the detail (' + e.message + '). <a href="' + $(".open", card).href + '">Open the page</a>.</div>'; });
  }
  function collapse(card) {
    card.classList.remove("open"); $(".toggle", card).setAttribute("aria-expanded", "false");
  }
  function toggle(card) { return card.classList.contains("open") ? collapse(card) : expand(card); }
  function syncReviewer() { if (rev && rev.value.trim()) $$('input[name="reviewer"]').forEach(function (i) { if (i !== rev && !i.value) i.value = rev.value.trim(); }); }

  $$(".card").forEach(function (card) {
    $(".toggle", card).addEventListener("click", function () { focusCard(card, false); toggle(card); });
  });
  var ea = $("#expandall"), ca = $("#collapseall");
  if (ea) ea.addEventListener("click", function () {
    var i = 0; (function next() { var batch = cards.slice(i, i + 4); i += 4; if (!batch.length) return;
      Promise.all(batch.map(expand)).then(next); })();
  });
  if (ca) ca.addEventListener("click", function () { cards.forEach(collapse); });

  // ---- selection + bulk bar -----------------------------------------------
  var bar = $("#bulkform"), count = $("#bulkcount"), last = null;
  var sels = $$(".sel");
  function refreshBulk() {
    var n = sels.filter(function (s) { return s.checked; }).length;
    sels.forEach(function (s) { s.closest(".card").classList.toggle("selected", s.checked); });
    if (bar) { bar.hidden = n === 0; count.textContent = n; }
    var all = $("#selectall"); if (all) { var enabled = sels.filter(function (s) { return !s.disabled; }); all.checked = enabled.length > 0 && enabled.every(function (s) { return s.checked; }); }
  }
  sels.forEach(function (s) {
    s.addEventListener("click", function (e) {
      if (e.shiftKey && last && last !== s) {
        var a = sels.indexOf(last), b = sels.indexOf(s), lo = Math.min(a, b), hi = Math.max(a, b);
        for (var i = lo; i <= hi; i++) if (!sels[i].disabled) sels[i].checked = s.checked;
      }
      last = s; refreshBulk();
    });
  });
  var sa = $("#selectall");
  if (sa) sa.addEventListener("change", function () { sels.forEach(function (s) { if (!s.disabled) s.checked = sa.checked; }); refreshBulk(); });
  if (bar) bar.addEventListener("submit", function (e) {
    var act = e.submitter && e.submitter.value, reason = $('[name="reason"]', bar);
    var rv = rev && rev.value.trim();
    if (!rv) { e.preventDefault(); rev.focus(); alert("Enter your reviewer name first."); return; }
    $("#bulk-reviewer").value = rv;
    if ((act === "reject" || act === "need-more-evidence") && !reason.value.trim()) { e.preventDefault(); reason.focus(); reason.placeholder = "A shared reason is required for " + act; }
  });

  // ---- action forms (delegated: inline fragments and item page) -----------
  var armed = null;
  function arm(form, btn) {
    $$("button", form).forEach(function (b) { b.classList.remove("armed"); });
    var hint = $(".armed-hint", form);
    armed = { form: form, btn: btn };
    btn.classList.add("armed");
    if (hint) hint.textContent = "Armed: " + btn.textContent.trim() + " — " + (btn.dataset.needsReason === "1" ? "type the reason, then Ctrl+Enter." : "press Enter to confirm.");
    if (btn.dataset.needsReason === "1") $("textarea", form).focus(); else btn.focus();
  }
  // inline forms carry the header's reviewer name; refuse early when it is empty
  document.addEventListener("submit", function (e) {
    var f = e.target; if (!f.matches || !f.matches("form.actions, form.correct")) return;
    var r = $('input[name="reviewer"]', f);
    if (r && r.type === "hidden" && !r.value.trim()) {
      var v = rev && rev.value.trim();
      if (v) { r.value = v; return; }
      e.preventDefault(); if (rev) { rev.focus(); rev.placeholder = "enter your name to decide"; }
    }
  });
  document.addEventListener("click", function (e) {
    var b = e.target.closest("button[data-key]"); if (!b) return;
    // the card's primary button sits above the form and references it via the
    // form attribute, so closest() alone does not reach it
    var form = b.closest("form.actions") || (b.form && b.form.matches("form.actions") ? b.form : null); if (!form) return;
    if (b.dataset.correct) { e.preventDefault(); showCorrect(form.closest(".detail-body")); return; }
    if (b.dataset.needsReason === "1") {
      var r = $('textarea[name="reason"]', form);
      if (!r.value.trim()) { e.preventDefault(); arm(form, b); }
    }
  });
  document.addEventListener("keydown", function (e) {
    if (e.target.matches('textarea[name="reason"]') && e.key === "Enter") {
      var form = e.target.closest("form");
      if (e.ctrlKey || e.metaKey) { e.preventDefault(); if (armed && armed.form === form) form.requestSubmit(armed.btn); }
    }
  });
  function showCorrect(body) {
    if (!body) return;
    var f = $("form.correct", body); if (!f) return;
    f.hidden = false;
    var fold = f.closest("details"); if (fold) fold.open = true; // §6 correct fold
    var first = $("select, input[type=text], input:not([type])", f); f.scrollIntoView({ block: "center", behavior: "smooth" });
    var t = $('textarea[name="reason"]', f); if (t) t.focus();
  }
  function initKinds(root) {
    $$("select.kind", root).forEach(function (sel) {
      var out = $(".class-out", sel.closest(".grid"));
      function upd() { var o = sel.options[sel.selectedIndex]; if (out && o) out.textContent = o.dataset["class"]; }
      sel.addEventListener("change", upd); upd();
    });
  }
  initKinds(document);
  // an item page opened with a refused correction keeps the form visible
  var pageBody = $(".item-page .detail-body");

  // ---- keyboard -------------------------------------------------------------
  document.addEventListener("keydown", function (e) {
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    var t = e.target, typing = t.matches("input, textarea, select") || t.isContentEditable;
    if (e.key === "Escape") { toggleHelp(false); armed = null; return; }
    if (typing) return;
    var c = focused() || (cards.length === 1 ? cards[0] : null), body = null, form = null;
    switch (e.key) {
      case "?": toggleHelp(); return;
      case "/": var fb = $("#filterbox"); if (fb) fb.open = true; var f = $("#filters input, #filters select"); if (f) { e.preventDefault(); f.focus(); } return;
      case "j": case "k":
        if (!cards.length) return;
        var i = cards.indexOf(focused()); i = e.key === "j" ? Math.min(cards.length - 1, i + 1) : Math.max(0, i < 0 ? 0 : i - 1);
        focusCard(cards[i]); return;
      case "Enter": case "o": if (c && !t.closest("button, a")) { e.preventDefault(); toggle(c); } return;
      case "x": if (c) { var s = $(".sel", c); if (s && !s.disabled) { s.checked = !s.checked; last = s; refreshBulk(); } } return;
    }
    var key = e.key.toLowerCase();
    body = c ? $(".detail-body", c) : pageBody;
    if (!body) return;
    var btn = $('button[data-key="' + key + '"]', body); if (!btn) return;
    e.preventDefault();
    if (btn.dataset.correct) { showCorrect(body); return; }
    arm(btn.closest("form.actions"), btn);
  });

  if (cards.length === 1) focusCard(cards[0], false);
  refreshBulk();
})();

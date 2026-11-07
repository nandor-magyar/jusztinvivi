// contact page: after Turnstile passes, the server verifies the token and returns the e-mail address.
// an enquire button (?work=...) puts the painting in the mail subject.
(function () {
  var box = document.getElementById("email");
  if (!box) return;
  var widget = box.querySelector(".ts");
  var status = box.querySelector(".status");
  var work = new URLSearchParams(location.search).get("work");
  var id;

  function reveal(token) {
    status.textContent = box.dataset.checking;
    fetch(box.dataset.endpoint, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token: token })
    })
      .then(function (r) { return r.ok ? r.json() : Promise.reject(r.status); })
      .then(function (d) {
        var a = document.createElement("a");
        a.href = "mailto:" + d.email + (work ? "?subject=" + encodeURIComponent(work.slice(0, 200)) : "");
        a.textContent = d.email;
        turnstile.remove(id);
        box.replaceChildren(a);
      })
      .catch(function () {
        status.textContent = box.dataset.failed;
        turnstile.reset(id);
      });
  }

  window.vjTurnstile = function () {
    id = turnstile.render(widget, {
      sitekey: box.dataset.sitekey,
      language: box.dataset.lang,
      theme: "dark",
      callback: reveal,
      "error-callback": function () { status.textContent = box.dataset.failed; }
    });
  };
})();

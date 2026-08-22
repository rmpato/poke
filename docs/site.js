/* pogo — website behaviour.
   Everything here is an improvement on a page that already works without it:
   with JavaScript off the terminal is simply already typed and every tab
   panel is visible. */

(function () {
  "use strict";

  var reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  /* ------------------------------------------------------------ typing -- */
  /* The hero terminal types the thing the page is about. */

  (function typing() {
    var out = document.getElementById("type");
    var cursor = document.getElementById("cursor");
    if (!out || !cursor) return;

    var lines = [
      { text: "pogo curl https://api.acme.com/users/42", cls: "k", head: true },
      { text: "{", cls: "o" },
      { text: '  "id": 42,', cls: "o" },
      { text: '  "name": "Pato",', cls: "o" },
      { text: '  "tier": "gold"', cls: "o" },
      { text: "}", cls: "o" },
      { text: "", cls: "o" },
      { text: "# later, without having saved anything", cls: "c" },
      { text: "pogo", cls: "k", head: true }
    ];

    function span(cls, text) {
      var el = document.createElement("span");
      if (cls) el.className = cls;
      el.textContent = text;
      return el;
    }

    out.textContent = "";

    if (reduced) {
      lines.forEach(function (line) {
        if (line.head) out.appendChild(span("c", "$ "));
        out.appendChild(span(line.cls, line.text));
        out.appendChild(document.createTextNode("\n"));
      });
      cursor.classList.add("off");
      return;
    }

    var index = 0;
    function nextLine() {
      if (index >= lines.length) return;
      var line = lines[index++];
      if (line.head) out.appendChild(span("c", "$ "));
      var el = span(line.cls, "");
      out.appendChild(el);
      out.appendChild(cursor);

      // The command is typed; its output arrives at once, the way output does.
      if (!line.head) {
        el.textContent = line.text;
        out.appendChild(document.createTextNode("\n"));
        setTimeout(nextLine, line.text === "" ? 80 : 120);
        return;
      }
      var i = 0;
      (function typeChar() {
        if (i < line.text.length) {
          el.textContent += line.text.charAt(i++);
          setTimeout(typeChar, 30);
          return;
        }
        out.appendChild(document.createTextNode("\n"));
        setTimeout(nextLine, 440);
      })();
    }
    setTimeout(nextLine, 420);
  })();

  /* -------------------------------------------------------------- tabs -- */
  /* Follows the tabs pattern: arrows move, Home/End jump, and the panel is
     only hidden once JavaScript has taken over. */

  document.querySelectorAll("[data-tabs]").forEach(function (group) {
    var tabs = Array.prototype.slice.call(group.querySelectorAll('[role="tab"]'));
    if (!tabs.length) return;

    function panelOf(tab) {
      return document.getElementById(tab.getAttribute("aria-controls"));
    }

    function select(tab, focus) {
      tabs.forEach(function (other) {
        var on = other === tab;
        other.setAttribute("aria-selected", on ? "true" : "false");
        other.setAttribute("tabindex", on ? "0" : "-1");
        var panel = panelOf(other);
        if (panel) panel.hidden = !on;
      });
      if (focus) tab.focus();
      if (group.id) {
        // Keep the chosen tab in the URL, so a link can point at one screen.
        history.replaceState(null, "", "#" + tab.id);
      }
    }

    // Switching to a shorter panel while scrolled into a longer one leaves you
    // below the content looking at nothing. Bring the tabs back into view.
    function keepInView() {
      var top = group.getBoundingClientRect().top;
      if (top < 0) {
        group.scrollIntoView({ behavior: reduced ? "auto" : "smooth", block: "start" });
      }
    }

    tabs.forEach(function (tab, i) {
      tab.addEventListener("click", function () { select(tab, false); keepInView(); });
      tab.addEventListener("keydown", function (event) {
        var next = null;
        switch (event.key) {
          case "ArrowRight": case "ArrowDown": next = tabs[(i + 1) % tabs.length]; break;
          case "ArrowLeft":  case "ArrowUp":   next = tabs[(i - 1 + tabs.length) % tabs.length]; break;
          case "Home":       next = tabs[0]; break;
          case "End":        next = tabs[tabs.length - 1]; break;
          default: return;
        }
        event.preventDefault();
        select(next, true);
        keepInView();
      });
    });

    group.setAttribute("data-ready", "");

    // A link to #tab-something opens that tab; otherwise the first one.
    var wanted = tabs.filter(function (tab) { return "#" + tab.id === window.location.hash; })[0];
    select(wanted || tabs[0], false);
  });

  /* -------------------------------------------------------------- copy -- */

  document.querySelectorAll(".copy").forEach(function (button) {
    button.addEventListener("click", function () {
      navigator.clipboard.writeText(button.dataset.copy).then(function () {
        button.textContent = "copied";
        button.classList.add("done");
        setTimeout(function () {
          button.textContent = "copy";
          button.classList.remove("done");
        }, 1600);
      });
    });
  });

  /* ------------------------------------------------------------ reveal -- */

  var rising = document.querySelectorAll(".rise");
  if (!reduced && "IntersectionObserver" in window) {
    var reveal = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (entry.isIntersecting) {
          entry.target.classList.add("in");
          reveal.unobserve(entry.target);
        }
      });
    }, { rootMargin: "0px 0px -8% 0px" });
    rising.forEach(function (el) { reveal.observe(el); });
  } else {
    rising.forEach(function (el) { el.classList.add("in"); });
  }
})();

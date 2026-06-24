// Connects to the SSE job stream and refreshes the job/history tables
// whenever a download's state changes, without requiring full polling.
(function () {
  function refreshTables() {
    var jobList = document.getElementById('job-list');
    if (jobList && window.htmx) htmx.trigger(jobList, 'refresh-jobs');

    var historyRows = document.getElementById('history-rows');
    if (historyRows && window.htmx) htmx.trigger(historyRows, 'refresh-jobs');
  }

  function connect() {
    var es = new EventSource('/events');

    es.addEventListener('job_update', function () {
      // Debounce a little so a burst of progress events doesn't hammer the DOM.
      clearTimeout(window.__vdRefreshTimer);
      window.__vdRefreshTimer = setTimeout(refreshTables, 250);
    });

    es.onerror = function () {
      es.close();
      setTimeout(connect, 3000); // reconnect on drop
    };
  }

  // Wire the hx-trigger="refresh-jobs" custom event support for both tables.
  document.addEventListener('DOMContentLoaded', function () {
    ['job-list', 'history-rows'].forEach(function (id) {
      var el = document.getElementById(id);
      if (el) {
        el.setAttribute('hx-trigger', el.getAttribute('hx-trigger') + ', refresh-jobs');
        if (window.htmx) htmx.process(el);
      }
    });
    connect();
  });
})();

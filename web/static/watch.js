(function () {
  function $(id) {
    return document.getElementById(id);
  }

  function fmtBytes(bytes) {
    if (!bytes && bytes !== 0) return 'unknown';
    var value = Number(bytes);
    if (!isFinite(value) || value < 0) return 'unknown';
    var units = ['B', 'KB', 'MB', 'GB', 'TB'];
    var idx = 0;
    while (value >= 1024 && idx < units.length - 1) {
      value /= 1024;
      idx += 1;
    }
    return (idx === 0 ? value : value.toFixed(1)) + ' ' + units[idx];
  }

  function fmtDuration(seconds) {
    var total = Number(seconds);
    if (!isFinite(total) || total < 0) return 'unknown';
    var h = Math.floor(total / 3600);
    var m = Math.floor((total % 3600) / 60);
    var s = Math.floor(total % 60);
    var parts = [];
    if (h > 0) parts.push(String(h).padStart(2, '0'));
    parts.push(String(m).padStart(2, '0'));
    parts.push(String(s).padStart(2, '0'));
    return parts.join(':');
  }

  function normalizeValue(value) {
    if (value == null || value === '') return 'unknown';
    if (Array.isArray(value)) return value.map(normalizeValue).join(', ');
    if (typeof value === 'object') return JSON.stringify(value, null, 2);
    return String(value);
  }

  function setText(id, value) {
    var el = $(id);
    if (el) el.textContent = value;
  }

  function renderSummary(payload) {
    var rows = [];
    var d = payload.download || {};
    var m = payload.metadata || {};

    rows.push(['Uploader', d.uploader || m.uploader || m.channel || 'unknown']);
    rows.push(['Title', d.title || m.title || 'unknown']);
    rows.push(['Duration', fmtDuration(d.duration || m.duration)]);
    rows.push(['File size', fmtBytes(d.filesize || m.filesize)]);
    rows.push(['Resolution', d.resolution || (m.width && m.height ? m.width + 'x' + m.height : 'unknown')]);
    rows.push(['Format ID', d.format_id || m.format_id || 'unknown']);
    rows.push(['Storage', d.storage_type || 'unknown']);
    rows.push(['Created', d.created_at || 'unknown']);
    rows.push(['Updated', d.updated_at || 'unknown']);
    rows.push(['Source URL', d.source_url || m.webpage_url || 'unknown']);

    var container = $('watch-summary');
    if (!container) return;
    container.replaceChildren();
    rows.forEach(function (row) {
      var wrapper = document.createElement('div');
      wrapper.className = 'rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-950/70 px-4 py-3 shadow-sm dark:shadow-none';

      var label = document.createElement('dt');
      label.className = 'text-[11px] uppercase tracking-[0.28em] text-slate-400 dark:text-slate-500';
      label.textContent = row[0];

      var value = document.createElement('dd');
      value.className = 'mt-1 break-words text-sm text-slate-800 dark:text-slate-200';
      value.textContent = normalizeValue(row[1]);

      wrapper.appendChild(label);
      wrapper.appendChild(value);
      container.appendChild(wrapper);
    });
  }

  function renderRaw(payload) {
    var raw = payload.metadata_json || '';
    if (!raw && payload.metadata && Object.keys(payload.metadata).length) {
      raw = JSON.stringify(payload.metadata, null, 2);
    }
    if (!raw) {
      raw = 'No metadata payload was available for this download.';
    }
    setText('watch-raw', raw);
  }

  function renderMetadataState(payload) {
    var d = payload.download || {};
    var banner = $('watch-banner');
    var state = $('watch-state');
    var error = $('watch-metadata-error');

    if (error) error.textContent = payload.metadata_error || '';
    if (banner && payload.metadata_error) {
      banner.classList.remove('hidden');
      banner.textContent = 'Metadata parsed with a warning: ' + payload.metadata_error;
    }

    if (state) {
      var parts = [];
      parts.push('Playback source: ' + (payload.video_url || 'unavailable'));
      parts.push('Thumbnail source: ' + (payload.thumbnail_url || d.thumbnail || 'unavailable'));
      if (d.error) parts.push('Job error: ' + d.error);
      if (d.status && d.status !== 'complete') parts.push('Current status: ' + d.status);
      state.textContent = parts.join(' \u00b7 ');
    }
  }

  function updatePlayer(payload) {
    var video = $('player');
    var poster = payload.thumbnail_url || (payload.download && payload.download.thumbnail) || '';
    var src = payload.video_url;

    if (!video) return;

    if (window.VidstackPlayer && typeof window.VidstackPlayer.create === 'function') {
      try {
        var config = {
          target: '#player',
          src: src,
          poster: poster,
        };
        if (window.VidstackPlayerLayout) {
          config.layout = new window.VidstackPlayerLayout();
        }
        window.VidstackPlayer.create(config);
        return;
      } catch (err) {
        console.warn('Vidstack enhancement failed, falling back to native video controls.', err);
      }
    }

    video.src = src;
    if (poster) video.poster = poster;
    video.controls = true;
    video.playsInline = true;
    var banner = $('watch-banner');
    if (banner) {
      banner.classList.remove('hidden');
      banner.textContent = 'Vidstack was not available, so the native player is active.';
    }
  }

  async function main() {
    var id = document.body && document.body.dataset ? document.body.dataset.downloadId : '';
    if (!id) return;

    try {
      var res = await fetch('/api/watch/' + encodeURIComponent(id), {
        headers: { 'Accept': 'application/json' },
      });
      if (!res.ok) {
        throw new Error('failed to load watch data (' + res.status + ')');
      }
      var payload = await res.json();

      setText('watch-title', (payload.download && payload.download.title) || 'Untitled content');
      setText('watch-subtitle', (payload.download && (payload.download.uploader || payload.download.source_url)) || 'unknown source');
      setText('watch-status', payload.download && payload.download.status ? payload.download.status : 'unknown');
      setText('watch-storage', payload.download && payload.download.storage_type ? payload.download.storage_type : 'unknown');
      setText('watch-duration', fmtDuration(payload.download && payload.download.duration));

      if (payload.download && payload.download.title) {
        document.title = payload.download.title + ' · Watch';
      }

      var thumb = $('watch-thumb');
      if (thumb) {
        thumb.src = payload.thumbnail_url || (payload.download && payload.download.thumbnail) || '';
        thumb.alt = (payload.download && payload.download.title ? payload.download.title : 'Video') + ' thumbnail';
        thumb.addEventListener('error', function () {
          if (payload.download && payload.download.thumbnail && thumb.src !== payload.download.thumbnail) {
            thumb.src = payload.download.thumbnail;
          }
        }, { once: true });
      }

      updatePlayer(payload);
      renderSummary(payload);
      renderRaw(payload);
      renderMetadataState(payload);
    } catch (err) {
      var state = $('watch-state');
      if (state) state.textContent = err.message || String(err);
      var banner = $('watch-banner');
      if (banner) {
        banner.classList.remove('hidden');
        banner.textContent = 'Failed to load watch data.';
      }
      console.error(err);
    }
  }

  document.addEventListener('DOMContentLoaded', main);
})();

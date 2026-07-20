(function (root) {
  'use strict';

  function parseDataLine(line) {
    if (!line.startsWith('data:')) return null;
    let raw = line.slice(5);
    if (raw.startsWith(' ')) raw = raw.slice(1);
    raw = raw.trim();
    if (!raw || raw === '[DONE]') return null;
    try {
      return JSON.parse(raw);
    } catch (_) {
      return null;
    }
  }

  function createParser(onData) {
    let buffer = '';

    function push(chunk) {
      if (typeof chunk !== 'string' || chunk.length === 0) return 0;
      buffer += chunk;
      buffer = buffer.replace(/\r\n/g, '\n').replace(/\r/g, '\n');
      const events = buffer.split('\n\n');
      buffer = events.pop() || '';
      let emitted = 0;
      for (const event of events) {
        const lines = event.split('\n');
        for (const line of lines) {
          const data = parseDataLine(line);
          if (!data) continue;
          emitted += 1;
          onData(data);
        }
      }
      return emitted;
    }

    function pending() {
      return buffer;
    }

    return { push, pending };
  }

  const api = { createParser };
  root.YentEventStream = api;
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
})(typeof globalThis !== 'undefined' ? globalThis : this);

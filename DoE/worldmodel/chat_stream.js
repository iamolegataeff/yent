(function (root) {
  'use strict';

  const DEFAULT_ENDPOINT = '/chat/completions';

  function dependency(options) {
    const eventStream = (options && options.eventStream) || root.YentEventStream;
    if (!eventStream || typeof eventStream.createParser !== 'function') {
      throw new Error('YentEventStream helper missing');
    }
    return eventStream;
  }

  function fetchImpl(options) {
    if (options && typeof options.fetch === 'function') return options.fetch;
    if (typeof root.fetch === 'function') return root.fetch.bind(root);
    throw new Error('fetch unavailable');
  }

  function decoderImpl(options) {
    const Decoder = (options && options.TextDecoder) || root.TextDecoder;
    if (typeof Decoder !== 'function') throw new Error('TextDecoder unavailable');
    return new Decoder();
  }

  function requestBody(options) {
    const messages = Array.isArray(options && options.messages) ? options.messages : [];
    const temperature = Number.isFinite(options && options.temperature) ? options.temperature : 0.8;
    const maxTokens = Number.isFinite(options && options.maxTokens) ? Math.floor(options.maxTokens) : 512;
    return JSON.stringify({
      messages,
      temperature,
      max_tokens: maxTokens
    });
  }

  async function stream(options) {
    options = options || {};
    const eventStream = dependency(options);
    const response = await fetchImpl(options)(options.endpoint || DEFAULT_ENDPOINT, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      signal: options.signal,
      body: requestBody(options)
    });

    if (!response || !response.ok) {
      const status = response && Number.isFinite(response.status) ? response.status : 0;
      throw new Error(`HTTP ${status}`);
    }
    if (!response.body || typeof response.body.getReader !== 'function') {
      throw new Error('response body unavailable');
    }

    const reader = response.body.getReader();
    const decoder = decoderImpl(options);
    let doneSeen = false;
    let events = 0;
    let tokens = 0;
    const parser = eventStream.createParser(data => {
      events++;
      if (typeof options.onEvent === 'function') options.onEvent(data);
      if (data && data.done) {
        doneSeen = true;
        if (typeof options.onDone === 'function') options.onDone(data);
        return;
      }
      if (data && typeof data.token === 'string') {
        tokens++;
        if (typeof options.onToken === 'function') options.onToken(data.token, data);
      }
    });

    while (!doneSeen) {
      const next = await reader.read();
      if (!next || next.done) break;
      parser.push(decoder.decode(next.value, { stream: true }));
    }
    parser.push(decoder.decode());

    return {
      done: doneSeen,
      events,
      tokens,
      pending: parser.pending()
    };
  }

  const api = { DEFAULT_ENDPOINT, requestBody, stream };
  root.YentChatStream = api;
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
})(typeof globalThis !== 'undefined' ? globalThis : this);

const assert = require('node:assert/strict');
const chat = require('./chat_stream.js');
const eventStream = require('./event_stream.js');

function makeResponse(chunks, options = {}) {
  let index = 0;
  const encoder = new TextEncoder();
  return {
    ok: options.ok !== false,
    status: options.status || 200,
    body: options.noBody ? null : {
      getReader() {
        return {
          async read() {
            if (index >= chunks.length) return { done: true };
            return { done: false, value: encoder.encode(chunks[index++]) };
          }
        };
      }
    }
  };
}

async function main() {
{
  const body = JSON.parse(chat.requestBody({
    messages: [{ role: 'user', content: 'hello' }],
    temperature: 0.4,
    maxTokens: 33.8
  }));
  assert.deepEqual(body, {
    messages: [{ role: 'user', content: 'hello' }],
    temperature: 0.4,
    max_tokens: 33
  });
}

{
  let captured = null;
  const seen = [];
  const result = await chat.stream({
    eventStream,
    messages: [{ role: 'user', content: 'hi' }],
    temperature: 0.7,
    maxTokens: 12,
    fetch: async (url, init) => {
      captured = { url, init };
      return makeResponse([
        'data: {"token":"he',
        'llo","selected_prob":0.5}\n\n',
        'data: {"done":true}\n\n'
      ]);
    },
    onToken: (token, data) => seen.push({ token, prob: data.selected_prob })
  });
  assert.equal(captured.url, '/chat/completions');
  assert.equal(captured.init.method, 'POST');
  assert.equal(captured.init.headers['Content-Type'], 'application/json');
  assert.deepEqual(JSON.parse(captured.init.body), {
    messages: [{ role: 'user', content: 'hi' }],
    temperature: 0.7,
    max_tokens: 12
  });
  assert.deepEqual(seen, [{ token: 'hello', prob: 0.5 }]);
  assert.deepEqual(result, { done: true, events: 2, tokens: 1, pending: '' });
}

{
  await assert.rejects(
    () => chat.stream({
      eventStream,
      fetch: async () => makeResponse([], { ok: false, status: 503 })
    }),
    /HTTP 503/
  );
}

{
  await assert.rejects(
    () => chat.stream({
      eventStream,
      fetch: async () => makeResponse([], { noBody: true })
    }),
    /response body unavailable/
  );
}

}

main().catch(err => {
  console.error(err);
  process.exit(1);
});

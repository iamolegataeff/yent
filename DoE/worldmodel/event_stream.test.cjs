const assert = require('node:assert/strict');
const stream = require('./event_stream.js');

{
  const seen = [];
  const parser = stream.createParser(data => seen.push(data));
  assert.equal(parser.push('data: {"token":"he'), 0);
  assert.equal(parser.push('llo"}\n\n'), 1);
  assert.deepEqual(seen, [{ token: 'hello' }]);
  assert.equal(parser.pending(), '');
}

{
  const seen = [];
  const parser = stream.createParser(data => seen.push(data));
  assert.equal(parser.push('data: {"token":"a"}\r\n\r\ndata: {"done":true}\r\n\r\n'), 2);
  assert.deepEqual(seen, [{ token: 'a' }, { done: true }]);
}

{
  const seen = [];
  const parser = stream.createParser(data => seen.push(data));
  assert.equal(parser.push(': keepalive\n\ndata: [DONE]\n\ndata: not-json\n\ndata: {"ok":1}\n\n'), 1);
  assert.deepEqual(seen, [{ ok: 1 }]);
}

{
  const seen = [];
  const parser = stream.createParser(data => seen.push(data));
  assert.equal(parser.push('data:{"compact":true}\n\n'), 1);
  assert.deepEqual(seen, [{ compact: true }]);
}

{
  const seen = [];
  const parser = stream.createParser(data => seen.push(data));
  assert.equal(parser.push('data: {"split":'), 0);
  assert.equal(parser.push('true}\r'), 0);
  assert.equal(parser.push('\n\r\n'), 1);
  assert.deepEqual(seen, [{ split: true }]);
}

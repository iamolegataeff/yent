const assert = require('node:assert/strict');
const session = require('./interface_session.js');

function storage() {
  const data = new Map();
  return {
    getItem(key) {
      return data.has(key) ? data.get(key) : null;
    },
    setItem(key, value) {
      data.set(key, value);
    }
  };
}

{
  const source = [
    { role: 'system', content: 'ignore' },
    { role: 'user', content: 'first' },
    { role: 'assistant', content: '' },
    { role: 'assistant', content: 'second' },
    { role: 'user', content: 'third' }
  ];
  assert.deepEqual(session.normalize(source, { limit: 2 }), [
    { role: 'assistant', content: 'second' },
    { role: 'user', content: 'third' }
  ]);
}

{
  const long = 'x'.repeat(session.CONTENT_LIMIT + 9);
  const normalized = session.normalize([{ role: 'user', content: long }]);
  assert.equal(normalized.length, 1);
  assert.equal(normalized[0].content.length, session.CONTENT_LIMIT);
}

{
  const s = storage();
  assert.equal(session.save(s, [
    { role: 'user', content: 'visible prompt' },
    { role: 'assistant', content: 'visible answer' },
    { role: 'tool', content: 'not visible' }
  ]), true);
  assert.deepEqual(session.load(s), [
    { role: 'user', content: 'visible prompt' },
    { role: 'assistant', content: 'visible answer' }
  ]);
}

{
  const s = storage();
  s.setItem(session.KEY, '{not valid json');
  assert.deepEqual(session.load(s), []);
}

{
  const broken = {
    getItem() {
      throw new Error('read denied');
    },
    setItem() {
      throw new Error('write denied');
    }
  };
  assert.equal(session.save(broken, [{ role: 'user', content: 'x' }]), false);
  assert.deepEqual(session.load(broken), []);
}

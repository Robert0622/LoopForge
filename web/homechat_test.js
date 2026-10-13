'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('path');
const source = fs.readFileSync(path.join(__dirname, 'app/homechat.js'), 'utf8');
const css = fs.readFileSync(path.join(__dirname, 'onboarding.css'), 'utf8');
assert.match(source, /function renderHomeChat/);
assert.match(source, /\/api\/chat-clarify/);
assert.match(source, /\/api\/chat-clarify\/cancel/);
assert.match(source, /建单并开跑/);
assert.match(source, /function renderHomeRunsPanel/, 'run progress lives in a side panel, not in the thread');
assert.match(source, /homeRunSteps/);
assert.match(source, /data-q=.*data-opt=/, 'clarify questions render as clickable options');

const send = source.slice(source.indexOf('async function homeChatSend'), source.indexOf('async function homeChatClarify'));
assert.ok(send.length > 0);
assert.doesNotMatch(send, /homeChatRunDemo\(|startDemoRequirement\(/, 'sending a message never launches the demo by itself');

for (const cls of new Set(source.match(/hc-[a-z-]+/g))) {
  assert.ok(css.includes('.' + cls) || css.includes(cls), `missing style for ${cls}`);
}
console.log('homechat ok');

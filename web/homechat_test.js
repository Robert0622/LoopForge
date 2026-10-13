'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('path');
const source = fs.readFileSync(path.join(__dirname, 'app/homechat.js'), 'utf8');
const css = fs.readFileSync(path.join(__dirname, 'onboarding.css'), 'utf8');
assert.match(source, /function renderHomeChat/);
assert.match(source, /\/api\/assistant\/chat/, 'the home chat is the platform assistant, not a clarify-only entry');
assert.match(source, /scene: 'home'/);
assert.match(source, /\/api\/assistant\/files/, 'images and files can be attached');
assert.match(source, /onpaste/);
assert.match(source, /ondrop/);
assert.match(source, /homeMentionPick/, 'requirements can be @-mentioned');
assert.match(source, /homeChatMarkLaunchDone\(chat\.sessionId/, 'a launched card is written back so reopening the session cannot launch it twice');
assert.match(source, /method: 'PUT'/);
assert.match(source, /\[\^A-Za-z0-9_\.@\]\)@/, '@ works right after CJK punctuation but not inside emails');
assert.match(source, /refs:/);
assert.match(source, /建单并开跑/);
assert.match(source, /\/mcp/, 'write drafts are confirmed through the MCP endpoint');
assert.match(source, /mermaid/, 'mermaid diagrams in replies are rendered');
assert.match(source, /function renderHomeRunsPanel/, 'run progress lives in a side panel, not in the thread');
assert.match(source, /data-q=.*data-opt=/, 'follow-up questions render as clickable options');

const send = source.slice(source.indexOf('async function homeChatSend'), source.indexOf('function homeChatRetry'));
assert.ok(send.length > 0);
assert.doesNotMatch(send, /homeChatRunDemo\(|startDemoRequirement\(/, 'sending a message never launches the demo by itself');

for (const cls of new Set(source.match(/hc-[a-z-]+/g))) {
  assert.ok(css.includes('.' + cls) || css.includes(cls), `missing style for ${cls}`);
}
console.log('homechat ok');

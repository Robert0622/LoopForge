'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const source = fs.readFileSync(path.join(__dirname, 'app/quickstart.js'), 'utf8');
const context = { console, localStorage: { getItem() { return null; }, setItem() {}, removeItem() {} } };
vm.createContext(context);
vm.runInContext(source, context);

const fresh = context.quickstartSteps({ demo: { available: false } });
assert.equal(fresh.demoSteps.length, 0);
assert.equal(fresh.realSteps.map(step => step.id).join(','), 'model-key,git-token,repo,issue,result');
assert.ok(fresh.realSteps.every(step => !step.done && step.action));
assert.equal(fresh.complete, false);

const demo = context.quickstartSteps({ modelKey: true, demo: { available: true, started: true, waitingIssueId: 'I-1' } });
assert.equal(demo.demoSteps.length, 2);
assert.equal(demo.demoSteps[0].done, true);
assert.equal(demo.demoSteps[0].action.run, 'demo');
assert.equal(demo.demoSteps[1].done, false);
assert.equal(demo.demoSteps[1].action.issue, 'I-1');
assert.equal(demo.realSteps[0].done, true);

const notStarted = context.quickstartSteps({ demo: { available: true } });
assert.equal(notStarted.demoSteps[1].action, null);

const finished = context.quickstartSteps({ modelKey: true, gitToken: true, repoBound: true, realIssue: true, realFinished: true, demo: { available: true } });
assert.equal(finished.complete, true);

console.log('quickstart ok');

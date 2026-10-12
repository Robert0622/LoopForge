'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const source = fs.readFileSync(path.join(__dirname, 'app/onboarding.js'), 'utf8');
const context = {
  console,
  setTimeout,
  window: undefined,
  document: undefined,
  localStorage: {getItem() { return null; }, setItem() {}},
};
vm.createContext(context);
vm.runInContext(source, context);

const steps = context.onboardingSteps(false);
assert.equal(steps.length, 5);
assert.equal(steps[0].target, '#dashQuickstart');
assert.ok(steps.slice(1).every(step => step.target.startsWith('.nav[data-view=')));
assert.equal(steps.at(-1).finish, true);
assert.equal(steps.at(-1).view, 'settings');
assert.equal(steps.at(-1).next, '去填写');
assert.equal(steps.at(-1).target, '.nav[data-view="settings"]');

const demoSteps = context.onboardingSteps(true);
assert.equal(demoSteps.length, 5);
assert.match(demoSteps[0].body, /一键开跑/);
assert.equal(demoSteps.at(-1).view, 'dashboard');
assert.equal(demoSteps.at(-1).next, '开始体验');

const right = context.onboardingPlace(
  {left: 12, top: 180, right: 220, bottom: 220, width: 208, height: 40},
  {width: 320, height: 140},
  {width: 1280, height: 800},
);
assert.equal(right.placement, 'right');
assert.equal(right.left, 234);
assert.equal(right.top, 130);

const wide = context.onboardingPlace(
  {left: 188, top: 116, right: 1416, bottom: 466, width: 1228, height: 350},
  {width: 320, height: 150},
  {width: 1440, height: 900},
);
assert.equal(wide.placement, 'below');
assert.equal(wide.top, 480);
assert.equal(wide.left, 212);

const tall = context.onboardingPlace(
  {left: 188, top: 160, right: 1416, bottom: 760, width: 1228, height: 600},
  {width: 320, height: 200},
  {width: 1440, height: 900},
);
assert.equal(tall.placement, 'below');
assert.equal(tall.top, 684);

console.log('onboarding sidebar ok');

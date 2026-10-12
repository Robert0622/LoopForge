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

const steps = context.onboardingSteps();
assert.equal(steps.length, 4);
assert.ok(steps.every(step => step.target && step.target.startsWith('.nav[data-view=')));
assert.equal(steps.at(-1).finish, true);
assert.equal(steps.at(-1).view, 'settings');
assert.equal(steps.at(-1).target, '.nav[data-view="settings"]');
assert.ok(source.includes("finishOnboarding(step.view || 'settings')"));

const right = context.onboardingPlace(
  {left: 12, top: 180, right: 220, bottom: 220, width: 208, height: 40},
  {width: 320, height: 140},
  {width: 1280, height: 800},
);
assert.equal(right.placement, 'right');
assert.equal(right.left, 234);
assert.equal(right.top, 130);

console.log('onboarding sidebar ok');

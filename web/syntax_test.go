package web

import (
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestOnboardingStylesEmbedded(t *testing.T) {
	data, err := fs.ReadFile(Files, "onboarding.css")
	if err != nil {
		t.Fatalf("onboarding.css is linked from index.html but not embedded, so the tour stays in normal flow and breaks the page: %v", err)
	}
	css := string(data)
	for _, want := range []string{"#onboarding{position:fixed;inset:0", ".onboarding-card{position:fixed", ".onboarding-spot{position:fixed"} {
		if !strings.Contains(css, want) {
			t.Fatalf("onboarding.css is missing %q", want)
		}
	}
	if strings.Contains(css, "top:50%") {
		t.Fatal("onboarding card must sit beside the sidebar item")
	}
	page, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), `href="/onboarding.css`) {
		t.Fatal("index.html must load onboarding.css")
	}
}

func TestBootLoadsWorkspaceAfterSignIn(t *testing.T) {
	data, err := os.ReadFile("app.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	start := strings.Index(src, "async function boot(){")
	end := strings.Index(src, "\nfunction bind(){")
	if start < 0 || end < start {
		t.Fatal("boot() not found")
	}
	boot := src[start:end]
	for _, want := range []string{"/api/auth/current", "openLoginModal({forced:true})", "/api/bootstrap", "renderAll();", "switchView(view);", "maybeStartOnboarding"} {
		if !strings.Contains(boot, want) {
			t.Fatalf("boot() no longer contains %q; a signed-in user would see an empty guest shell", want)
		}
	}
}

func TestCredentialBrandAssetsEmbedded(t *testing.T) {
	for _, name := range []string{"github.svg", "codebuddy.svg", "codex.svg", "claude.svg"} {
		data, err := fs.ReadFile(Files, "images/credentials/"+name)
		if err != nil {
			t.Fatalf("credential logo %s is not embedded: %v", name, err)
		}
		if !strings.Contains(string(data), "<svg") {
			t.Fatalf("credential logo %s is not SVG", name)
		}
	}
	app, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"https://github.com/settings/tokens", "https://www.codebuddy.ai/", "credential-description", "git-token-verify"} {
		if !strings.Contains(string(app), want) {
			t.Fatalf("credential UI is missing %q", want)
		}
	}
}

// A parse error anywhere in the app bundle blanks the whole SPA, and `go test` alone
// cannot catch it, so gate the bundle on node's parser when node is available.
func TestClarifyModelPickerAndMCPPrompt(t *testing.T) {
	app, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"loopforge.clarifyModel",
		"selectedClarifyModel",
		"data-clarify-model",
		"model:selectedClarifyModel()",
	} {
		if !strings.Contains(string(app), want) {
			t.Fatalf("clarify model picker is missing %q", want)
		}
	}
}

func TestAppJSParses(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping app.js parse check")
	}
	for _, name := range appBundleFiles(t) {
		out, err := exec.Command(node, "--check", name).CombinedOutput()
		if err != nil {
			t.Fatalf("%s failed to parse: %v\n%s", name, err, out)
		}
	}
}

func TestIssueRunRecoveryActions(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
const state = {data:{workflows:[]}};
const escapeHTML = value => String(value);
const runContext = () => ({issue:{}});
const forgeIssueClosed = () => false;
const issueExecution = issue => ({run:issue.run});
const issueCurrentStage = issue => ({key:issue.stage});
const issueRunHealth = () => 'attention';
const issueManuallyClosed = () => false;
const issueTrackerClosed = () => false;
const deliveryLocked = () => false;
const issueClarifying = () => false;
const clarificationNeedsConfirm = () => false;
const requirementNeedsClarification = () => false;
const isScriptWorkflowNode = node => node?.type==='script'||node?.type==='command';
const cachedRun = () => null;
eval(code.slice(code.indexOf('function canRerunFromNode('), code.indexOf('function afRerunFromNode(')));
eval(code.slice(code.indexOf('function issueActionButtons('), code.indexOf('function setIssueRunFilter(')));
eval(code.slice(code.indexOf('function refreshRunSOPButtonHTML('), code.indexOf('function pausedRunPanelHTML(')));
const run = {status:'failed', workflowId:'wf', workflowSnapshot:{id:'wf',nodes:[{id:'a'}]}, nodes:{a:{status:'failed'}}};
for (const status of ['waiting','failed','cancelled','succeeded']) {
  assert.equal(canRerunFromNode({...run,status}), true, status);
}
assert.equal(canRerunFromNode({...run,status:'running'}), false);
assert.equal(canRerunFromNode({...run,status:'waiting',nodes:{a:{status:'running'}}}), false);
assert.equal(runNodeAllowsContext({id:'a',type:'script'}), false);
assert.equal(runNodeAllowsContext({id:'a',type:'command'}), false);
assert.equal(runNodeAllowsContext({id:'a',type:'agent'}), true);
assert.equal(runNodeAllowsContext({id:'a'}), false);
assert.equal(runNodeChoices({...run,workflowSnapshot:{id:'wf',nodes:[{id:'a',type:'command'}]}})[0].type, 'command');
const chained={...run,workflowSnapshot:{id:'wf',nodes:[{id:'first',type:'agent'},{id:'second',type:'agent'}],edges:[{source:'first',target:'second'}]},nodes:{first:{status:'failed'},second:{status:'pending'}}};
assert.deepEqual(runNodeChoices(chained).map(n=>n.id),['first']);
assert.equal(canRerunFromHere(chained,'first'),true);
assert.equal(canRerunFromHere(chained,'second'),false);
assert.deepEqual(runNodeChoices({...chained,status:'waiting',nodes:{first:{status:'waiting_approval'},second:{status:'pending'}}}).map(n=>n.id),['first']);
assert.equal(canRerunFromHere({...chained,status:'waiting',nodes:{first:{status:'waiting_approval'},second:{status:'pending'}}},'second'),false);
assert.deepEqual(runNodeChoices({...chained,nodes:{first:{status:'succeeded'},second:{status:'failed'}}}).map(n=>n.id),['first','second']);
assert.equal(canRerunFromHere({...chained,nodes:{first:{status:'succeeded'},second:{status:'failed'}}},'second'),true);
const failed = issueActionButtons({id:'issue-1',run,stage:'failed'});
assert.equal((failed.match(/class="issue-act /g)||[]).length, 1);
assert.ok(failed.includes('处理失败'));
assert.ok(!failed.includes('data-resume-run') && !failed.includes('data-pass-node'));
const waiting = issueActionButtons({id:'issue-1',run:{...run,status:'waiting'},stage:'waiting'});
assert.ok(waiting.includes('去确认'));
assert.equal((waiting.match(/class="issue-act /g)||[]).length, 1);
state.data.workflows = [{id:'wf',version:2,revision:1}];
assert.equal(refreshRunSOPButtonHTML({...run,workflowSnapshot:{}}), '');
assert.ok(refreshRunSOPButtonHTML({...run,workflowSnapshot:{id:'wf',version:1,revision:1}}).includes('更新为最新 SOP'));
assert.equal(refreshRunSOPButtonHTML({...run,workflowSnapshot:{id:'wf',version:2,revision:1}}), '');
assert.ok(!code.includes('function issueMoreActionsHTML('));
assert.ok(code.includes('data-rerun-from-here'));
assert.ok(!code.includes('data-rerun-from-node-issue'));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("issue recovery controls failed: %v\n%s", err, out)
	}
}

func TestMetricsPhaseAverageExcludesMissingSamples(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
eval(code.slice(code.indexOf('function metricsDeliveryPhases('), code.indexOf('function metricsDashboardModel(')));
const rows = [
  {phases:{testing:0,samples:{testing:0}}},
  {phases:{testing:60000,samples:{testing:1}}},
  {phases:{testing:0,samples:{testing:1}}},
];
assert.deepEqual(metricsPhaseAverage(rows, 'testing'), {value:30000,samples:2});
assert.deepEqual(metricsPhaseAverage(rows.slice(0,1), 'testing'), {value:null,samples:0});
const missingStage=metricsDeliveryPhases({clarificationDurationMs:3600000,developmentDurationMs:1200000,cycleDurationMs:9000000,phaseSamples:{clarification:1,development:1}});
assert.deepEqual(missingStage.samples, {clarification:1,development:1,testing:0,total:0});
assert.equal(missingStage.total, null);
const complete=metricsDeliveryPhases({clarificationDurationMs:3600000,developmentDurationMs:1200000,testDurationMs:600000,cycleDurationMs:9000000,phaseSamples:{clarification:1,development:1,testing:1}});
assert.equal(complete.total, complete.clarification+complete.development+complete.testing);
assert.equal(complete.total, 5400000);
assert.deepEqual(metricsPhaseAverage([{phases:missingStage},{phases:complete}], 'total'), {value:5400000,samples:1});
const observedZero=metricsDeliveryPhases({phaseSamples:{clarification:1,development:1,testing:1}});
assert.equal(observedZero.total, 0);
assert.equal(metricsPhaseAverage([{phases:observedZero}], 'total').value, 0);
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("phase sample behavior failed: %v\n%s", err, out)
	}
}

func TestMetricsDashboardTotalUsesCompletePhaseSamples(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
const METRICS_WINDOWS = {'7d':{days:7,label:'近 7 天',grain:'day'}};
const now = new Date().toISOString();
const issues = [1,2].map(n=>({id:'issue-'+n,projectId:'project',source:'local',kind:'requirement',status:'done',createdAt:now,updatedAt:now}));
const deliveries = [
  {issueId:'issue-1',lastRunAt:now,firstRunAt:now,clarificationDurationMs:600000,developmentDurationMs:1200000,testDurationMs:1800000,cycleDurationMs:7200000,phaseSamples:{clarification:1,development:1,testing:1}},
  {issueId:'issue-2',lastRunAt:now,firstRunAt:now,clarificationDurationMs:2400000,developmentDurationMs:3000000,cycleDurationMs:18000000,phaseSamples:{clarification:1,development:1,testing:0}},
];
const state = {metricsRange:'7d',metricsData:{deliveries,runTraces:[],nodes:[],summary:{}},metricsIssues:issues};
function currentProject(){return {id:'project'}}
eval(code.slice(code.indexOf('function metricsWindow('), code.indexOf('function metricsDisplayModel(')));
const model = metricsDashboardModel();
assert.equal(model.delivered, 2);
assert.deepEqual(model.phaseTotalValues, [3600000]);
assert.equal(model.averagePhaseTotalMs, 3600000);
assert.deepEqual(model.phaseAverages.total, {value:3600000,samples:1});
assert.equal(model.phaseAverages.total.value, model.phaseAverages.clarification.value+model.phaseAverages.development.value+model.phaseAverages.testing.value);
assert.equal(model.requirements.find(item=>item.issueId==='issue-2').phases.total, null);
assert.equal(model.phaseTrend.at(-1).total, 3600000);
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("dashboard phase total check failed: %v\n%s", err, out)
	}
}

func TestMetricsCachedDataStaysWithinSelectedProject(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
const state = {
  projectFilter:'beta', metricsRange:'30d', metricsData:{deliveries:[]},
  metricsDataKey:'alpha|30d||', metricsLoading:false, metricsError:'',
};
function metricsQueryKey(){return [state.projectFilter,state.metricsRange,'',''].join('|')}
function bindMetricsDateControls(){}
const body={innerHTML:''},live={innerHTML:''};
function $(selector){return selector==='#metricsBody'?body:selector==='#metricsLiveState'?live:null}
function metricsDashboardModel(){throw new Error('stale data must not be rendered')}
eval(code.slice(code.indexOf('function renderMetricsDashboard('),code.indexOf('let metricsRequestSeq=')));
renderMetricsDashboard();
assert.match(body.innerHTML,/正在切换到当前项目的数据/);
state.metricsLoading=true;
renderMetricsDashboard();
assert.match(body.innerHTML,/正在加载当前项目的研发效能数据/);
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("metrics project isolation failed: %v\n%s", err, out)
	}
}

func TestMetricsRefreshIgnoresPreviousProjectResponse(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
const state={projectFilter:'alpha',metricsRange:'30d',metricsDateFrom:'',metricsDateTo:'',metricsData:null,metricsIssues:[],metricsLoading:false,metricsError:''};
function currentProject(){return {id:state.projectFilter}}
function metricsWindow(){return {from:new Date('2026-09-01T00:00:00Z'),to:new Date('2026-10-01T00:00:00Z'),grain:'day'}}
function renderMetricsDashboard(){}
function metricsDemoEnabled(){return false}
const calls=[];let resolveOld;
function api(path){calls.push(path);if(path.startsWith('/api/analytics?')&&path.includes('projectId=alpha'))return new Promise(resolve=>{resolveOld=resolve});if(path.startsWith('/api/analytics?'))return Promise.resolve({deliveries:[{issueId:'beta-issue'}]});return Promise.resolve([])}
eval(code.slice(code.indexOf('let metricsRequestSeq='),code.indexOf('function metricHealth(')));
async function check(){
  const old=refreshMetrics();
  state.projectFilter='beta';invalidateMetricsForProjectChange();
  resolveOld({deliveries:[{issueId:'alpha-issue'}]});
  await old;
  assert.equal(state.metricsData,null);
  await refreshMetrics();
  assert.equal(state.metricsData.deliveries[0].issueId,'beta-issue');
  assert.equal(state.metricsDataKey,'beta|30d||');
  assert.ok(calls.some(path=>path.includes('projectId=beta')));
  state.projectFilter='alpha';invalidateMetricsForProjectChange();
  assert.equal(state.metricsData,null);
  assert.equal(state.metricsDataKey,'');
}
check().catch(error=>{console.error(error);process.exitCode=1});
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("metrics request isolation failed: %v\n%s", err, out)
	}
}

func TestPlatformMetricsRequirementIdentity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
eval(code.slice(code.indexOf('function metricsIssueSource('), code.indexOf('function metricsIssueDone(')));
assert.equal(metricsRequirementIssue({id:'gh-req',source:'github',kind:'requirement'}), true);
const unbound = {id:'local-1',projectId:'p',source:'local',kind:'requirement'};
const bound = {id:'local-2',projectId:'p',source:'local',kind:'requirement',trackingId:'1001'};
const synced = {id:'gh-1',projectId:'p',source:'github',kind:'requirement',externalRef:{source:'github',externalId:'1001'}};
assert.equal(metricsRequirementIssue(unbound), true);
assert.equal(metricsRequirementKey(unbound), 'issue:local-1');
assert.equal(metricsRequirementKey(bound), metricsRequirementKey(synced));
assert.equal(metricsRequirementIssue({id:'bug',source:'github',kind:'issue'}), false);
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("platform requirement metrics behavior failed: %v\n%s", err, out)
	}
	page, err := fs.ReadFile(Files, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`name="syncTarget" value="local"`, `name="trackingId"`} {
		if !strings.Contains(string(page), want) {
			t.Fatalf("platform requirement editor is missing %q", want)
		}
	}
}

func TestNodeShowsRoleValidation(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping unified validation behavior check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
eval(code.slice(code.indexOf('function effectiveNodeArtifacts('), code.indexOf('function nodeGatePane(')));
const agent = {
  artifactOutputs: [{name:'report',path:'role.md',required:true},{name:'evidence',path:'evidence.md'}],
  verification: [{id:'shared',name:'role shared'},{id:'legacy',name:'legacy submit',mode:'submit'},{id:'role-only',name:'role only'}]
};
const workflowNode = {artifactOutputs: [{name:'report',path:'node.md',required:true}]};
assert.deepEqual(effectiveNodeArtifacts(workflowNode, agent).map(x=>x.path), ['node.md','evidence.md']);
assert.deepEqual(roleGates(agent).map(x=>x.name), ['role shared','role only']);
assert.ok(code.includes('Agent 节点先提交结构化终态，随后校验产物，最后执行可选门禁命令'));
assert.ok(code.includes('pass | rework | fail | human'));
assert.ok(!code.includes('node?.verification'));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("unified validation behavior check failed: %v\n%s", err, out)
	}
}

func TestAssistantJSParses(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping assistant.js parse check")
	}
	out, err := exec.Command(node, "--check", "assistant.js").CombinedOutput()
	if err != nil {
		t.Fatalf("assistant.js failed to parse: %v\n%s", err, out)
	}
}

func TestAssistantSidebarResize(t *testing.T) {
	js, err := os.ReadFile("assistant.js")
	if err != nil {
		t.Fatal(err)
	}
	css, err := os.ReadFile("assistant.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`class="ai-resize"`,
		`loopforge.assistantWidth`,
		`setPointerCapture`,
		`--ai-panel-w`,
		`collapseThinking`,
		`pushThought`,
	} {
		if !strings.Contains(string(js), want) {
			t.Fatalf("assistant.js missing %q", want)
		}
	}
	for _, want := range []string{
		`.ai-resize`,
		`cursor:col-resize`,
		`body.ai-assistant-resizing`,
		`var(--ai-panel-w)`,
	} {
		if !strings.Contains(string(css), want) {
			t.Fatalf("assistant.css missing %q", want)
		}
	}
}

func TestRuleGlobalSOPPicker(t *testing.T) {
	app, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	css, err := os.ReadFile("platform.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"作为全局规则",
		"ruleGlobalFields",
		"bindRuleGlobalPicker",
		"selectedRuleWorkflowIds",
		"该 SOP 的每个节点执行时都会带上这条规则",
		"workflowIds:global?selectedRuleWorkflowIds():[]",
	} {
		if !strings.Contains(string(app), want) {
			t.Fatalf("rule global picker is missing %q", want)
		}
	}
	for _, want := range []string{".rule-global-block", ".rule-global-input:checked", ".rule-global-sops"} {
		if !strings.Contains(string(css), want) {
			t.Fatalf("rule global picker style is missing %q", want)
		}
	}
}

func TestAssetSaveKeepsSelectedIdentity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping asset save behavior check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
eval(code.slice(code.indexOf('function resolveAssetSaveIdentity('), code.indexOf('function renderAssetForm(')));
const items = [{id:'existing',name:'Existing'}];
const edit = resolveAssetSaveIdentity(items, 'existing', 'accidental-new-id');
assert.equal(edit.id, 'existing');
assert.equal(edit.existing, items[0]);
assert.equal(edit.created, false);
assert.match(resolveAssetSaveIdentity(items, '__new__', 'existing').error, /已存在/);
assert.deepEqual(resolveAssetSaveIdentity(items, '__new__', 'new-id'), {id:'new-id',existing:null,created:true});
assert.ok(code.includes('readonly aria-readonly="true"'));
assert.ok(code.includes("identity.created?'资产已创建':'资产修改已保存'"));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("asset save behavior check failed: %v\n%s", err, out)
	}
}

func TestMCPLogoHasBrokenImageFallback(t *testing.T) {
	app, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"mcp-logo-fallback", "bindMCPLogoFallbacks", "naturalWidth===0", "logo-load-failed"} {
		if !strings.Contains(string(app), want) {
			t.Fatalf("MCP logo fallback is missing %q", want)
		}
	}
	css, err := os.ReadFile("platform.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".mcp-card-logo .mcp-logo-fallback{display:inline", ".mcp-card-logo img+.mcp-logo-fallback{display:none"} {
		if !strings.Contains(string(css), want) {
			t.Fatalf("MCP initial avatar style is missing %q", want)
		}
	}
}

func TestMCPJSONImportParser(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping mcp.json import parser check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
eval(code.slice(code.indexOf('function mcpImportPlainObject('), code.indexOf('function mcpApplyImportedServer(')));
const stdio = mcpImportCandidates({mcpServers:{playwright:{command:'npx',args:['@playwright/mcp@latest'],env:{TOKEN:'secret'},cwd:'tools',pathEntries:['custom/bin']}}});
assert.equal(stdio.length, 1);
assert.deepEqual(stdio[0], {id:'playwright',name:'playwright',description:'',transport:'stdio',timeoutSec:60,url:'',authType:'',headers:{},command:'npx',args:['@playwright/mcp@latest'],installScript:'',workingDirectory:'tools',pathEntries:['custom/bin'],environment:{TOKEN:'secret'}});
const http = mcpImportCandidates({servers:{github:{type:'streamable-http',url:'https://api.githubcopilot.com/mcp/',headers:{Authorization:'Bearer token'},timeoutSec:30}}});
assert.equal(http[0].transport, 'http');
assert.equal(http[0].url, 'https://api.githubcopilot.com/mcp/');
assert.deepEqual(http[0].headers, {Authorization:'Bearer token'});
assert.equal(http[0].timeoutSec, 30);
const direct = mcpImportCandidates({id:'nested',name:'Nested',transport:'stdio',stdio:{command:'node',args:['server.js'],environment:{PORT:3000}}});
assert.equal(direct[0].command, 'node');
assert.deepEqual(direct[0].environment, {PORT:'3000'});
const fragment = '"remote": {\u00a0"url": "[http://mcp.example.com/open/mcp](http://mcp.example.com/open/mcp)", "headers": {"x-api-token": "secret"}}';
const pasted = mcpImportCandidates(mcpParseJSONText(fragment));
assert.equal(pasted[0].id, 'remote');
assert.equal(pasted[0].transport, 'http');
assert.equal(pasted[0].url, 'http://mcp.example.com/open/mcp');
assert.deepEqual(pasted[0].headers, {'x-api-token':'secret'});
const ticks = String.fromCharCode(96).repeat(3);
assert.equal(mcpImportCandidates(mcpParseJSONText(ticks+'json\n{"mcpServers":{"demo":{"command":"node"}}}\n'+ticks))[0].command, 'node');
assert.equal(mcpImportCandidates({mcpServers:{one:{command:'a'},two:{url:'https://example.com/mcp'}}}).length, 2);
assert.throws(()=>mcpImportCandidates({mcpServers:{bad:{command:'node',args:'server.js'}}}), /args 必须是数组/);
assert.throws(()=>mcpImportCandidates({mcpServers:{bad:{command:'node',pathEntries:'bin'}}}), /pathEntries 必须是 JSON 数组/);
assert.throws(()=>mcpImportCandidates({mcpServers:{bad:{type:'websocket',url:'wss:\/\/example.com'}}}), /不支持的 MCP transport/);
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("mcp.json import parser check failed: %v\n%s", err, out)
	}
}

func TestMCPJSONImportIsCollapsedByDefault(t *testing.T) {
	index, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(index)
	if !strings.Contains(html, `<details id="mcpJsonImport" class="mcp-json-import"><summary>从 mcp.json 粘贴导入</summary>`) {
		t.Fatal("mcp.json paste importer must use a collapsed details entry")
	}
	if strings.Contains(html, `<details id="mcpJsonImport" class="mcp-json-import" open>`) {
		t.Fatal("mcp.json paste importer must be collapsed by default")
	}
}

func TestMCPUploadPathHelpIsPrepared(t *testing.T) {
	app, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"prepareMCPFormHints", "文件路径相对于上传内容根目录", "上传文件时默认以内容根目录启动", "通常留空；例如 packages/my-mcp", "额外 PATH 目录", "常见目录已由平台自动加入 PATH"} {
		if !strings.Contains(string(app), want) {
			t.Fatalf("MCP upload path help is missing %q", want)
		}
	}
}

func TestMCPClientConfigsAreClientNative(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping MCP client config check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
eval(code.slice(code.indexOf('function accessMcpClientSpec('), code.indexOf('// 对外展示的接入地址')));
const base = 'http://loopforge.example.com';
const token = '<YOUR_PERSONAL_TOKEN>';
const claude = accessMcpClientSpec('claude', base, token);
assert.equal(claude.location, 'Claude CLI（用户级配置写入 ~/.claude.json）');
assert.ok(claude.code.includes('claude mcp add --transport http --scope user'));
assert.ok(claude.code.includes('--header "Authorization: Bearer <YOUR_PERSONAL_TOKEN>"'));
const codex = accessMcpClientSpec('codex', base, token);
assert.equal(codex.location, '~/.codex/config.toml');
assert.ok(codex.code.includes('[mcp_servers.loopforge]'));
assert.ok(codex.code.includes('http_headers = { Authorization ='));
assert.ok(!codex.code.includes('mcpServers'));
const cursor = accessMcpClientSpec('cursor', base, token);
assert.ok(cursor.code.includes('"mcpServers"'));
const prompt = accessMcpAgentPrompt(codex, base, token);
assert.equal(prompt, '');
const readyPrompt = accessMcpAgentPrompt(codex, base, 'afp_test_token');
assert.ok(readyPrompt.includes('使用 curl 下载 安装流程文件, 禁止使用 fetch：http://loopforge.example.com/mcp-setup.txt'));
assert.ok(!readyPrompt.includes('当前客户端'));
assert.ok(readyPrompt.includes('API_TOKEN = afp_test_token'));
assert.ok(readyPrompt.includes('ENDPOINT = http://loopforge.example.com/mcp'));
assert.ok(code.includes('async function createQuickAccessPrompt('));
assert.ok(code.includes('MCP 快速接入 · ${spec.label}'));
assert.ok(code.includes('expiresInDays:90'));
assert.ok(code.includes("data-create-token-first]')?.addEventListener('click',e=>createQuickAccessPrompt"));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("MCP client config check failed: %v\n%s", err, out)
	}
}

func TestMCPSetupInstructionsAreEmbedded(t *testing.T) {
	setup, err := os.ReadFile("mcp-setup.txt")
	if err != nil {
		t.Fatal(err)
	}
	text := string(setup)
	for _, want := range []string{"Cursor", "Claude Code", "CodeBuddy", "Codex", "API_TOKEN", "ENDPOINT", "tools/list", "失败排查", "汇报", "时间戳的备份"} {
		if !strings.Contains(text, want) {
			t.Fatalf("MCP setup instructions are missing %q", want)
		}
	}
	embedSource, err := os.ReadFile("embed.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(embedSource), "mcp-setup.txt") {
		t.Fatal("mcp-setup.txt is not embedded in the web bundle")
	}
}

func TestAccessCapabilitiesGroupingIsCallableAndFailuresAreVisible(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping capabilities UI check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
assert.ok(!code.includes('+function accessGroup('));
eval(code.slice(code.indexOf('function accessGroup('), code.indexOf('function accessCode(')));
assert.equal(accessGroup({name: 'runs_get'}), '运行与审批');
assert.equal(accessGroup({name: 'projects_list'}), '项目与仓库');
assert.ok(code.includes('能力清单加载失败'));
assert.ok(code.includes('data-retry-capabilities'));
assert.ok(code.includes('正在加载能力清单'));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("capabilities UI check failed: %v\n%s", err, out)
	}
}

func TestAccessGuideIsTheSingleOnboardingEntry(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	if strings.Contains(page, `id="accessView"`) || strings.Contains(page, `data-view="access"`) {
		t.Fatal("legacy access page or navigation entry is still present")
	}
	for _, want := range []string{`id="guideView"`, `data-access-tab="mcp">MCP 快速接入`, `data-access-tab="tokens"`, `data-access-tab="capabilities"`, `data-access-tab="api"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("unified guide is missing %q", want)
		}
	}
	if strings.Contains(page, `data-access-tab="stdio"`) {
		t.Fatal("stdio bridge must not be advertised in the onboarding UI")
	}
	js, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	code := string(js)
	if !strings.Contains(code, `if(hash==='access')return 'guide'`) || !strings.Contains(code, `if(name==='access')name='guide'`) {
		t.Fatal("legacy #/access links must redirect to #/guide")
	}
}

func TestRunnerBindCommandBootstrapsWithProtocolFallback(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	for _, want := range []string{"https://", "http://", "bindHttpsOrigin", "bindHttpOrigin", "--server", "bindServerHost"} {
		if !strings.Contains(page, want) {
			t.Fatalf("runner bind command is missing %q", want)
		}
	}
}

func TestRenderUserBadgeScope(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping check")
	}
	script := `
const fs = require("fs");
const appCode = fs.readFileSync("` + appBundlePath(t) + `", "utf8");
global.window = { location: { href: "http://localhost:8080/", pathname: "/", search: "", hash: "", hostname: "localhost" } };
global.sessionStorage = { _s: {}, getItem(k){return this._s[k]||null;}, setItem(k,v){this._s[k]=String(v);}, removeItem(k){delete this._s[k];} };
const makeEl = () => ({
  classList: { add(){}, remove(){} },
  title: "",
  textContent: "",
  innerHTML: "",
  onclick: null,
  querySelector(){ return makeEl(); },
  querySelectorAll(){ return []; }
});
const elements = {};
global.document = {
  querySelector(sel) {
    if (!elements[sel]) elements[sel] = makeEl();
    return elements[sel];
  },
  querySelectorAll() { return []; }
};
global.$ = s => global.document.querySelector(s);
global.escapeHTML = s => String(s);
global.state = { currentUser: { isGuest: true } };
global.isPlatformAdmin = () => false;
global.openLoginModal = () => {};
const fnStart = appCode.indexOf("function renderUserProfilePopover()");
const fnEnd = appCode.indexOf("async function verifyCodeBuddyKey(");
eval(appCode.slice(fnStart, fnEnd));

// Guest mode
renderUserBadge();

// Authenticated mode
state.currentUser = { isGuest: false, displayName: "Test User", loginName: "testuser" };
renderUserBadge();
`
	cmd := exec.Command(node, "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("renderUserBadge failed: %v\n%s", err, out)
	}
}

func TestCredentialClearButtons(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping credential UI check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
eval(code.slice(code.indexOf('function credentialClearAction('), code.indexOf('async function renderPersonalSettings(')));
for (const name of ['personalMcpToken', 'gitToken', 'codeBuddyApiKey', 'codexApiKey', 'claudeCodeApiKey']) {
  for (const configured of [false, true]) {
    const html = credentialClearAction(name, configured);
    assert.ok(html.includes('type="button"'));
    assert.ok(!html.includes('checkbox'));
    const input = {value: 'unsaved-draft', disabled: false};
    const status = {dataset: {configured: String(configured)}, textContent: ''};
    const button = {
      dataset: {clearCredential: name, clearLabel: 'API Key', pendingClear: 'false'}
    };
    const submit = {disabled: false};
    const form = {
      elements: {[name]: input},
      querySelectorAll() { return [button]; },
      querySelector(selector) {
        if (selector === 'button[type="submit"]') return submit;
        return selector.includes('data-credential-status') ? status : button;
      }
    };
    bindCredentialClearButtons(form);
    assert.equal(credentialClearPending(form, name), false);
    button.onclick();
    assert.equal(credentialClearPending(form, name), true);
    assert.equal(input.disabled, true);
    assert.equal(status.textContent, '待清除，保存后生效');
    assert.equal(button.textContent, '撤销清除');
    submit.disabled = true;
    button.onclick();
    assert.equal(credentialClearPending(form, name), true, 'save in progress must freeze clear state');
    submit.disabled = false;
    button.onclick();
    assert.equal(credentialClearPending(form, name), false);
    assert.equal(input.disabled, false);
    assert.equal(input.value, 'unsaved-draft', 'undo must preserve the draft');
    assert.equal(status.textContent, configured ? '当前已配置' : '尚未配置');
    assert.equal(button.textContent, '清除 API Key');
  }
}
assert.ok(!code.includes('通常包含 /v1'));
assert.ok(!code.includes('不含 /v1/messages'));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("credential clear buttons failed: %v\n%s", err, out)
	}
}

// Generic templates must tell users up front that the 发起 PR node needs a GitHub
// Git Token and a project-scoped GitHub MCP (+ personal MCP token).
func TestGenericTemplateRequirementPreflight(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping generic template preflight check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
global.escapeHTML = value => String(value);
global.state = {
  currentUser: {id: 7}, projectFilter: 'alpha',
  mcpMarket: [{id: 'github', aliases: ['forge'], url: 'https://api.githubcopilot.com/mcp/'}],
  data: {users: [{id: 7, gitTokenConfigured: true, personalMcpTokenConfigured: false}], mcpServers: []}
};
global.userById = id => state.data.users.find(u => Number(u.id) === Number(id));
eval(code.slice(code.indexOf('const GENERIC_REQUIREMENT_CHECKS='), code.indexOf('function genericRequirementChips(')));
const workflow = {requirements: [
  {key: 'git-token', nodeId: 'mr', label: 'GitHub Git Token', hint: '推送分支'},
  {key: 'personal-token', nodeId: 'mr', label: '个人 MCP 令牌', hint: 'MCP 认证'},
  {key: 'github-mcp', nodeId: 'mr', label: 'GitHub MCP', hint: '创建 PR'}
]};
let rows = genericRequirementRows(workflow);
assert.deepEqual(rows.map(r => r.ok), [true, true, false]);
state.data.users[0].gitTokenConfigured = false;
assert.deepEqual(genericRequirementRows(workflow).map(r => r.ok), [false, false, false]);
state.data.users[0].gitTokenConfigured = true;
assert.equal(rows[0].fix, 'settings');
assert.equal(rows[2].fix, 'mcp');
assert.equal(rows[2].nodeId, 'mr');
state.data.mcpServers = [{scope: 'project', projectId: 'alpha', marketId: 'github'}];
rows = genericRequirementRows(workflow);
assert.deepEqual(rows.map(r => r.ok), [true, true, true]);
// An install owned by another project must not satisfy the dependency.
state.data.mcpServers = [{scope: 'project', projectId: 'beta', marketId: 'github'}];
assert.equal(genericRequirementRows(workflow)[2].ok, false);
// Without market metadata the name/url fallback still recognises GitHub.
state.mcpMarket = [];
state.data.mcpServers = [{scope: 'project', projectId: 'alpha', name: 'GitHub MCP', url: 'https://api.githubcopilot.com/mcp/'}];
assert.equal(genericRequirementRows(workflow)[2].ok, true);
assert.ok(code.includes('通用模板为只读'));
assert.ok(code.includes('data-adopt-generic'));
assert.ok(code.includes('sop-generic'));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("generic template preflight check failed: %v\n%s", err, out)
	}
}

func TestLegacyExecutorIsNotSilentlyDisplayedAsCodeBuddy(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping executor UI check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
global.escapeHTML = value => String(value);
global.state = {data: {executors: [
  {id: 'codebuddy', name: 'CodeBuddy'},
  {id: 'codex', name: 'Codex'},
  {id: 'claude', name: 'Claude Code'}
]}};
eval(code.slice(code.indexOf('function executorOptions('), code.indexOf('function selectAgent(')));
const legacy = executorOptions('legacy-codex');
assert.ok(legacy.includes('历史执行器 legacy-codex'));
assert.ok(legacy.includes('value="legacy-codex" selected disabled'));
assert.ok(!legacy.includes('value="codebuddy" selected'));
assert.ok(executorOptions('codex').includes('value="codex" selected'));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("legacy executor UI check failed: %v\n%s", err, out)
	}
}

func TestScheduleIssueStatusPicker(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`name="issueStatus" value="backlog" checked`,
		`name="issueStatus" value="todo" checked`,
		`name="issueStatus" value="in_progress"`,
		`触发哪些状态`,
	} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("schedule form missing %q", want)
		}
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping schedule status helper check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
eval(code.slice(code.indexOf('function scheduleIssueStatusValues(job){'), code.indexOf('function scheduleModelAPI(')));
assert.deepEqual(scheduleIssueStatusValues({}), ['backlog','todo']);
assert.deepEqual(scheduleIssueStatusValues({id:'job-1'}), ['backlog','todo','in_progress','in_review','blocked']);
assert.deepEqual(scheduleIssueStatusValues({id:'job-1',skipDispatch:true}), ['backlog','todo']);
assert.deepEqual(scheduleIssueStatusValues({issueStatuses:['in_progress','blocked']}), ['in_progress','blocked']);
eval(code.slice(code.indexOf('function scheduleJobRunning(job){'), code.indexOf('function scheduleIntervalLabel(sec){')));
assert.equal(scheduleJobRunning({lastStatus:'running'}), true);
assert.equal(scheduleJobRunning({lastStatus:'success'}), false);
assert.equal(scheduleJobStatusText({lastStatus:'running'}), '正在运行… 关闭页面不会中断');
assert.equal(scheduleRunningCount([{projectId:'dp',lastStatus:'running'},{projectId:'platform',lastStatus:'success'}],'platform'), 0);
assert.equal(scheduleRunningCount([{projectId:'dp',lastStatus:'running'}],'dp'), 1);
assert.ok(!code.includes('if(!active)fillScheduleJobForm()'));
assert.ok(code.includes('已在服务端启动，关闭页面不会中断'));
assert.ok(code.includes('startScheduleJobPoll'));
assert.ok(code.includes("lastStatus:'running'"));
assert.ok(code.includes('请至少选择一个触发状态'));
const openModal=code.slice(code.indexOf('async function openScheduleJobModal'), code.indexOf('function findAlignRun'));
assert.ok(openModal.indexOf('resetScheduleJobForm()') < openModal.indexOf('await Promise.all'), 'opening the modal must reset the form before network, not after');
assert.ok(!/await Promise\.all\([^;]*\);\s*resetScheduleJobForm/.test(openModal), 'awaiting status must not wipe the AI tab');
const poller=code.slice(code.indexOf('function startScheduleJobPoll(){'), code.indexOf('async function runScheduleJob('));
assert.ok(poller.includes('scheduleRunningCount(state.data.scheduleJobs, state.projectFilter)'));
assert.ok(!poller.includes('.some(scheduleJobRunning)'));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("schedule status helper check failed: %v\n%s", err, out)
	}
}

func TestApprovalsAreProjectScoped(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping approvals project scope check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
global.state = {
  currentUser: {id: 7},
  projectFilter: 'shop',
  data: {
    issues: [{id:'iss-afp', projectId:'platform'}],
    tasks: [{runId:'run-afp', issueId:'iss-afp'}],
    repositories: [{id:'repo-widget', projectId:'platform'}],
    runs: [
      {id:'run-afp', status:'waiting', executionContext:{responsibleUserId:7}},
      {id:'run-align', status:'waiting', workflowId:'scheduled-baseline-align', executionContext:{projectId:'platform', responsibleUserId:7}, repository:{name:'Widget', repositoryId:'repo-widget'}},
      {id:'run-shop', status:'waiting', executionContext:{projectId:'shop', responsibleUserId:7}},
      {id:'run-other-user', status:'waiting', executionContext:{projectId:'shop', responsibleUserId:99}},
    ]
  }
};
eval(code.slice(code.indexOf('function runContext(r){'), code.indexOf('function taskTriggerInfo(')));
eval(code.slice(code.indexOf('function runProjectId(r){'), code.indexOf('function preferredWorkflow(')));
eval(code.slice(code.indexOf('function approvalItems(){'), code.indexOf('async function cancelRunFlow(')));
assert.deepEqual(approvalItems().map(r => r.id), ['run-shop']);
state.projectFilter = 'platform';
assert.deepEqual(approvalItems().map(r => r.id), ['run-afp']);
state.data.runs[1].executionContext = {responsibleUserId:7};
assert.equal(runProjectId(state.data.runs[1]), 'platform');
assert.ok(code.includes('renderApprovals(true);'));
assert.ok(code.includes('function updateApprovalBadge('));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("approvals project scope check failed: %v\n%s", err, out)
	}
}

func TestRepositoryFormUsesSingleURL(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`name="repoUrl"`,
		`集成分支（可选）`,
		`检查多个 SOP 执行后工作分支之间是否存在合并冲突`,
	} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("repository form missing %q", want)
		}
	}
	for _, not := range []string{
		`GitHub Web URL`,
		`name="integrateBranch" value="loopforge/integration" required`,
	} {
		if strings.Contains(string(html), not) {
			t.Fatalf("repository form still has %q", not)
		}
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping repository URL helper check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
eval(code.slice(code.indexOf('function parseForgeRepoURL(raw){'), code.indexOf('function fillRepositoryFromLink(')));
const web = parseForgeRepoURL('https://github.com/acme/loopforge-demo');
assert.equal(web.project, 'acme/loopforge-demo');
assert.equal(web.cloneUrl, 'https://github.com/acme/loopforge-demo.git');
const ssh = parseForgeRepoURL('git@github.com:acme/loopforge-demo.git');
assert.equal(ssh.project, 'acme/loopforge-demo');
assert.equal(ssh.cloneUrl, 'git@github.com:acme/loopforge-demo.git');
const cloned = parseForgeRepoURL('git clone git@github.com:acme/loopforge-demo.git');
assert.equal(cloned.project, 'acme/loopforge-demo');
assert.equal(cloned.cloneUrl, 'git@github.com:acme/loopforge-demo.git');
assert.equal(web.name, 'loopforge-demo');
const http = parseForgeRepoURL('http://github.com/acme/loopforge-demo.git');
assert.equal(http.project, 'acme/loopforge-demo');
assert.equal(http.name, 'loopforge-demo');
assert.equal(ssh.name, 'loopforge-demo');
assert.ok(code.includes("integrateBranch:''"));
assert.ok(code.includes('集成分支用于检查多个 SOP 执行后工作分支之间是否存在合并冲突'));
assert.ok(!code.includes("integrateBranch:'loopforge/integration'"));
assert.ok(code.includes('项目名称'));
assert.ok(code.includes('data-edit-repo'));
assert.ok(code.includes('repo-cell-link'));
assert.ok(code.includes('target="_blank"'));
assert.ok(code.includes('repoRowWebHref'));
assert.ok(!code.includes('data-edit-col'));
assert.ok(!code.includes('data-save-col'));
assert.ok(code.includes('repoAdminRowHTML'));
assert.ok(code.includes('savedRepoOptions=repoRows.filter(r=>r.id)'));
assert.ok(!code.includes('（待保存）'));
assert.ok(code.includes("if(!row.querySelector('[data-r]'))"));
eval(code.slice(code.indexOf('function repoDisplayURL(r){'), code.indexOf('function repoPayloadFromRow(')));
var state = {data:{repositories:[{id:'widget',name:'widget',repoPath:'acme/widget',webUrl:'https://github.com/acme/widget',cloneUrl:'git@github.com:acme/widget.git',defaultBranch:'master',integrateBranch:'loopforge/integration'}]}};
const displayRow = {
  dataset:{repoRow:'0',repoId:'widget'},
  querySelector(sel){return sel==='[data-r]'||sel==='[data-r="repoUrl"]'?null:null},
  querySelectorAll(){return []}
};
const draft = {};
captureRepoRows({querySelectorAll(sel){return sel==='[data-repo-row]'?[displayRow]:[]}}, draft);
assert.equal(draft.repoRows[0].id, 'widget');
assert.equal(draft.repoRows[0].repoPath, 'acme/widget');
assert.equal(draft.repoRows[0].webUrl, 'https://github.com/acme/widget');
assert.ok(String(draft.repoRows[0].repoUrl||'').includes('widget'));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("repository URL helper check failed: %v\n%s", err, out)
	}
}

func TestRequirementDirectCreateUI(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	for _, want := range []string{`id="chatCreateDirect"`, `直接创建`, `id="chatCreateLocal"`, `平台需求`, `id="chatSend"`, `id="reqWorkflow"`, `SOP 流程`} {
		if !strings.Contains(page, want) {
			t.Fatalf("requirement composer UI is missing %q", want)
		}
	}
	sendIdx := strings.Index(page, `id="chatSend"`)
	directIdx := strings.Index(page, `id="chatCreateDirect"`)
	if sendIdx < 0 || directIdx < 0 || sendIdx > directIdx {
		t.Fatal("直接创建 must sit to the right of 开始分析")
	}
	if !strings.Contains(page, `nav-label">项目配置`) {
		t.Fatal("sidebar must label the projects page 项目配置")
	}
	if !strings.Contains(page, `<script src="/app/composer.js`) {
		t.Fatal("index.html must load the requirement composer script")
	}
	if !strings.Contains(page, `data-add="sop"`) {
		t.Fatal("flow designer must offer SOP as a node type")
	}
	if !strings.Contains(page, `class="req-composer-actions"`) {
		t.Fatal("composer actions must be grouped so 直接创建 cannot wrap under the hint")
	}
	if strings.Contains(page, `id="chatCreateForge"`) {
		t.Fatal("requirement composer must not offer GitHub as a create destination")
	}
	reqStart := strings.Index(page, `id="requirementModal"`)
	reqEnd := strings.Index(page, `id="importIssueModal"`)
	if reqStart >= 0 && reqEnd > reqStart {
		composer := page[reqStart:reqEnd]
		if strings.Contains(composer, `需求套餐`) {
			t.Fatal("create requirement modal must not show 需求套餐")
		}
		if !strings.Contains(composer, `id="reqWorkflow"`) || !strings.Contains(composer, `SOP 流程`) {
			t.Fatal("create requirement must show a visible SOP 流程 select")
		}
		if strings.Contains(composer, `id="reqSopRow"`) || strings.Contains(composer, `id="reqPlaybooks"`) || strings.Contains(composer, `hidden></select>`) {
			t.Fatal("create requirement must not hide SOP behind playbook chips")
		}
	}
	if !strings.Contains(page, `问清再建单`) {
		t.Fatal("placeholder must say 问清再建单")
	}
	if strings.Contains(page, "再建模") {
		t.Fatal("placeholder must not say 再建模")
	}
	js, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	code := string(js)
	if strings.Contains(code, "直接创建会同步到 GitHub") {
		t.Fatal("直接创建 must not auto-sync GitHub; user chooses dest")
	}
	if !strings.Contains(code, "patchCachedRun(runId,{pauseRequested:false,status:'running'})") {
		t.Fatal("resume must clear pauseRequested in the cached run")
	}
	if !strings.Contains(code, "pauseRequested:Boolean(incoming.pauseRequested)") {
		t.Fatal("live run merge must treat omitted pauseRequested as false")
	}
	if !strings.Contains(code, "function runRetryNeedsContext(run)") {
		t.Fatal("script retry must skip the context prompt")
	}
	for _, want := range []string{`state.reqCreateMode='direct'`, `reqCreateDirect`, `不会立刻启动`, `persistRequirementComposer`, `未配置`, `if(modal.id==='requirementModal')return`, `resumeChatClarify`, `jobId:chatClarifyJobId`, `正在接上分析`, `从空白开始`, `stripClarifyReason`, `renderClarifyPanel(JSON.parse(lastAssistant.content))`, `assigneeUserId:Number(state.currentUser?.id)||0`, `fillReqWorkflow`, `selectedReqWorkflowId`, `workflowId:selectedReqWorkflowId()`, `composerProgressHTML`, `继续分析`, `先建单，详情里继续澄清`, `跳过分析直接建单`, `按当前结论建单`, `分析还没完成，仍要建单`, `没有固定轮数`, `composerCreateReady`, `setComposerRetryMode`, `composerClarifyRetry`} {
		if !strings.Contains(code, want) {
			t.Fatalf("direct-create dest picker is missing %q", want)
		}
	}
	if strings.Contains(code, `renderClarifyPanel(JSON.parse(lastAssistant.content), reconnect?'':interruptHint)`) {
		t.Fatal("restored analysis must not treat the interrupt hint as a render error")
	}
}

func TestInternalDashboardsAreRemoved(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	app, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	for file, raw := range map[string][]byte{"index.html": html, "app bundle": app} {
		for _, banned := range []string{"deployment-metrics", "deploymentsView", "renderObsTabs", "/api/auth/photo", "onerror=\""} {
			if strings.Contains(string(raw), banned) {
				t.Fatalf("%s still references %q", file, banned)
			}
		}
	}
}

func TestIssueImportUI(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	for _, want := range []string{`id="importIssueButton"`, `id="importIssueModal"`, `id="importIssueURL"`, `id="importIssueWorkflow"`, `id="importIssueRepository"`, `代码仓库（可多选）`, `repo-picker`} {
		if !strings.Contains(page, want) {
			t.Fatalf("UI is missing %q", want)
		}
	}
	daily := strings.Index(page, `>日常工作<`)
	runsNav := strings.Index(page, `data-view="runs"`)
	projectsNav := strings.Index(page, `data-view="projects"`)
	flowConfig := strings.Index(page, `>流程配置<`)
	insights := strings.Index(page, `>数据洞察<`)
	if daily < 0 || runsNav < 0 || insights < 0 || !(daily < runsNav && runsNav < insights) {
		t.Fatal("运行记录 must sit under 日常工作, not 数据洞察")
	}
	if projectsNav < 0 || flowConfig < 0 || !(daily < projectsNav && projectsNav < flowConfig) {
		t.Fatal("项目配置 must sit under 日常工作, before 流程配置")
	}
	js, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	code := string(js)
	for _, want := range []string{`/api/issues/import`, `openImportIssueModal`, `fillImportIssueWorkflow`, `fillImportIssueRepository`, `selectedImportIssueRepositoryIds`, `renderImportIssueRepositoryPicker`, `repositoryIds`, `请至少选择一个代码仓库`, `importIssueFromURL`, `requirementNeedsClarification`, `clarificationNeedsConfirm`, `data-dtab="clarify"`, `确认结论`, `clarification?.gated`, `autoStartClarify`, `issueClarifyJobs`, `attachIssueClarifyStream`, `关闭网页也不会中断`, `clarifyOptionLabel`, `clarifyQuestionOptions`, `createIssueDirect`, `projectGitHubReady`, `chatCreateDirect`, `chatCreateLocal`, `pushIssueToGitHubFromDetail`, `run-id-copy`, `data-export-logs`, `从这里重跑`, `重新触发`, `data-retrigger-issue`, `data-rerun-from-here`, `canRetriggerIssue`, `openRerunFromNode`, `executionContext?.issueId`, `run?.status==='succeeded'`, `/api/issues/${encodeURIComponent(issueId)}/requirement`, `inferSOPLane`, `req-lane-card`, `issueLaneDots`, `.filter(l=>!l.orphan)`, `lanes.length<2`, `collapseSingleSOPLane`, `issueSOPLabel`, `actionWorkflow`, `data-workflow`, `workflowSOPLane`, `需求开发`, `saveIssueSOPS`, `/sops`, `effectiveIssueSOPBindings`, `projectSOPBindings`, `fillReqWorkflow`, `plannedSOPNodes`, `sopWorkflowId`, `sopWorkflowChoices`, `syncReqComposerLayout`} {
		if !strings.Contains(code, want) {
			t.Fatalf("app.js is missing %q", want)
		}
	}
	if strings.Contains(code, `projectPlaybookHTML`) {
		t.Fatal("playbook editor must not live in 项目成员")
	}
	if strings.Contains(code, `在「项目成员」里配`) || strings.Contains(code, `可在项目成员里改`) {
		t.Fatal("requirement playbooks must not be configured from 项目成员")
	}
	if strings.Contains(code, `data-open-sops-playbook`) {
		t.Fatal("playbook config links must not send users to 流程模板")
	}
	if strings.Contains(code, `data-issue-playbook`) || strings.Contains(code, `assignIssuePlaybook`) || strings.Contains(code, `去改套餐`) {
		t.Fatal("执行进展 must not configure or switch playbooks")
	}
	if strings.Contains(code, `交接产物`) {
		t.Fatal("SOP pane must not ask users to type 交接产物")
	}
	if strings.Contains(code, `并指定每一步的责任人`) {
		t.Fatal("SOP pane must not ask users to configure an owner for every step")
	}
	if strings.Contains(code, `从节点重跑`) {
		t.Fatal("run header must not keep 从节点重跑; use node-local 从这里重跑")
	}
	if strings.Contains(code, `data-rerun-from-node`) {
		t.Fatal("run header must not keep data-rerun-from-node")
	}
	if strings.Contains(code, `data-copy-run>复制 Run ID`) {
		t.Fatal("run detail must not keep a header 复制 Run ID button")
	}
	if strings.Contains(code, `run-eval-header-btn`) {
		t.Fatal("run detail header must not keep 智能评测")
	}
	headerExport := strings.Index(code, `run-header-actions`)
	sectionExport := strings.Index(code, `run-section-label-actions`)
	logsExport := strings.Index(code, `data-export-logs title="下载本次运行全部节点的 Worker 日志">导出全部日志`)
	workerLogs := strings.Index(code, `worker-log-heading-actions`)
	if headerExport < 0 || sectionExport < 0 || logsExport < 0 {
		t.Fatal("run detail must still export all-node logs from 执行详情")
	}
	if logsExport < sectionExport || logsExport < headerExport {
		t.Fatal("导出全部日志 should sit with 执行详情, not the top header")
	}
	if workerLogs >= 0 && strings.Contains(code[workerLogs:workerLogs+180], `data-export-logs`) {
		t.Fatal("导出全部日志 must not sit inside Worker 日志")
	}
}

func TestPlaybooksPageRemoved(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	for _, leftover := range []string{`data-view="playbooks"`, `id="playbooksView"`, `id="sopPlaybooks"`, `需求套餐`} {
		if strings.Contains(page, leftover) {
			t.Fatalf("需求套餐 page must be removed, still has %q", leftover)
		}
	}
	js, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	code := string(js)
	for _, leftover := range []string{`function renderSOPPlaybooks`, `function addProjectPlaybook`, `需求套餐`} {
		if strings.Contains(code, leftover) {
			t.Fatalf("playbook editor must be removed from app.js, still has %q", leftover)
		}
	}
}

func TestUsageAnalysisUI(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	for _, want := range []string{`data-obs-tab="overview">用量`, `data-obs-tab="delivery">交付`, `id="exportObsReport"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("usage analysis UI missing %q", want)
		}
	}
	for _, not := range []string{`data-obs-tab="quality"`, `data-obs-tab="cost"`, `data-obs-tab="agents"`, `data-obs-tab="trace"`} {
		if strings.Contains(page, not) {
			t.Fatalf("usage analysis still has extra tab %q", not)
		}
	}
	js, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	code := string(js)
	for _, want := range []string{"ensureObsUserFilter", "obsRunMatchesUser", "exportObsReport", "buildObsReportHTML", "userDisplay(user)", "assigneeUserId", "各模型用量", "每人每日用量", "成员汇总", "data-obs-scope", "setObsScope", "需求类型分布", "SOP 版本对比", "节点质量明细", "setObsTrendGrain", "obsTrendChart", "该责任人在各项目", "data-obs-grain", "grain=", "全部", "obsApiGrain", "obsRangeCaption", "obsPeriodPicker", "setObsPeriod", "obsScopedView", "data-obs-period", "不含暂停/恢复", "交付指标", "workflow_task_id", "averageTokensPerRun", "toolSuccessRate", "runTraces", "obs-traces", "isPreviewHost", "showDirectRunButton"} {
		if !strings.Contains(code, want) {
			t.Fatalf("app.js is missing %q", want)
		}
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping usage analysis helper check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
global.state = {
  currentUser: {id: 7, loginName: 'alice', displayName: '爱丽丝'},
  obsUserFilter: '',
  obsUserFilterReady: false,
  obsScope: 'user',
  data: {
    users: [{id: 7, loginName: 'alice', displayName: '爱丽丝'}, {id: 8, loginName: 'bob', displayName: '鲍勃'}],
    issues: [{id: 'iss-1', assigneeUserId: 7, source: 'github'}],
    tasks: [{runId: 'run-assignee', issueId: 'iss-1'}],
  }
};
function userById(id){return (state.data.users||[]).find(u=>Number(u.id)===Number(id))}
function userDisplay(user){return user?(user.displayName&&user.loginName?user.loginName+'('+user.displayName+')':user.loginName||user.displayName||('用户 '+user.id)):''}
eval(code.slice(code.indexOf('function ensureObsUserFilter(){'), code.indexOf('async function switchObsTab')));
eval(code.slice(code.indexOf('function obsUserName(id){'), code.indexOf('function obsUserChoices(){')));
assert.equal(obsUserName(7), 'alice(爱丽丝)');
assert.equal(obsRunMatchesUser({executionContext:{responsibleUserId:7}}, 7), true);
assert.equal(obsRunMatchesUser({id:'run-assignee', executionContext:{}}, 7), true);
assert.equal(obsRunMatchesUser({id:'run-assignee', executionContext:{}}, 8), false);
ensureObsUserFilter();
assert.equal(state.obsUserFilter, '7');
state.obsUserFilterReady=false;state.obsUserFilter='';state.obsScope='project';
ensureObsUserFilter();
assert.equal(state.obsUserFilter, '', 'project dimension must not default to current user');
assert.ok(code.includes("api('/api/analytics'+analyticsQuery"));
assert.ok(code.includes("obsScope()==='user'&&state.obsUserFilter"));
assert.ok(code.includes('grain='));
assert.ok(code.includes('crossProject'));
assert.ok(!code.includes('function obsQuality('));
eval(code.slice(code.indexOf('function obsTrendGrain(){'), code.indexOf('function ensureObsUserFilter')));
function formatTokens(n){return String(n)}
function escapeHTML(s){return String(s)}
assert.equal(obsWeekLabel('2026-W37'), '9/7–9/13');
assert.equal(obsTrendLabel({date:'2026-09-16'}), '09/16');
assert.equal(obsTrendValue({tokens:10,runs:2,issues:3,costUsd:1.5},'issues'), 3);
const delta=obsPeriodDelta([{date:'2026-W36',tokens:200},{date:'2026-W37',tokens:100}],'tokens');
assert.equal(delta.delta, -100);
assert.ok(obsDeltaChip(delta,true).includes('good'));
assert.ok(obsTrendChart([{date:'2026-W36',tokens:20},{date:'2026-W37',tokens:10}],'tokens').includes('obs-chart-line'));
state.obsTrendGrain='all';
assert.equal(obsApiGrain(), 'month');
assert.equal(obsRangeCaption(), '累计');
assert.equal(obsTrendWindow([{date:'2020-01'},{date:'2026-09'}]).length, 2);
state.obsTrendGrain='day';
state.obsPeriodKey='2026-09-20';
assert.equal(obsTrendWindow(Array.from({length:40},()=>({date:'x'}))).length, 30);
assert.equal(obsStepPeriodKey('2026-09-20','day',1), '2026-09-21');
assert.equal(obsPeriodRange('2026-09-20','day').to.toISOString().slice(0,10), '2026-09-21');
assert.equal(obsDateToWeekKey(new Date(Date.UTC(2026,8,16))), '2026-W38');
assert.equal(obsPeriodRange('2026-W37','week').label, '9/7–9/13');
state.analytics={runTraces:[
  {status:'succeeded',tokens:100,humanInterventions:0,issueId:'a',createdAt:'2026-09-20T08:00:00Z',durationMs:1000,toolCallsSucceeded:2,toolCallsFailed:0},
  {status:'failed',tokens:50,humanInterventions:1,issueId:'b',createdAt:'2026-09-21T08:00:00Z',durationMs:2000,toolCallsSucceeded:0,toolCallsFailed:1}
]};
const scoped=obsScopedView();
assert.equal(scoped.total, 1);
assert.equal(scoped.tokens, 100);
assert.equal(scoped.demo.humanInterventions, 0);
assert.ok(!code.slice(code.indexOf('async function refreshObs(){'), code.indexOf('function runCost(')).includes("api('/api/runs"));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("usage analysis helper check failed: %v\n%s", err, out)
	}
}

func TestListPaginationUI(t *testing.T) {
	js, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	code := string(js)
	for _, want := range []string{
		"function paginateList(",
		"function pagerHTML(",
		"function goListPage(",
		"function bindPager(",
		"paginateList('issues'",
		"paginateList('runs'",
		"paginateList('mcp'",
		"obs-users",
		"obs-deliveries",
		"data-page-key",
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("app.js is missing pagination %q", want)
		}
	}
	for _, not := range []string{"paginateList('sops'", "paginateList('agents'", "paginateList('assets'", "paginateList('approvals'", "paginateList('sync'", "obs-sops", "obs-models"} {
		if strings.Contains(code, not) {
			t.Fatalf("app.js still paginates %q", not)
		}
	}
	css, err := os.ReadFile("platform.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), ".pager{") && !strings.Contains(string(css), ".pager {") {
		t.Fatal("platform.css is missing .pager styles")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping pagination helper check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
global.state = {listPages:{}};
function escapeHTML(v){return String(v??'')}
eval(code.slice(code.indexOf('const LIST_PAGE_SIZE='), code.indexOf('function goListPage')));
const items = Array.from({length:45}, (_,i)=>i+1);
const p1 = paginateList('issues', items);
assert.equal(p1.size, 20);
assert.equal(p1.page, 1);
assert.equal(p1.pages, 3);
assert.deepEqual(p1.items, items.slice(0,20));
assert.equal(pagerHTML('issues', p1).includes('data-page-key="issues"'), true);
assert.equal(pagerHTML('issues', p1).includes('第 1–20 条 / 共 45 条'), true);
state.listPages.issues = 3;
const p3 = paginateList('issues', items);
assert.equal(p3.page, 3);
assert.deepEqual(p3.items, items.slice(40,45));
assert.equal(pagerHTML('issues', {total:10,size:20}).trim(), '');
resetListPage('issues');
assert.equal(state.listPages.issues, 1);
const obs = paginateList('obs-users', items);
assert.equal(obs.size, 20);
`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("pagination helper check failed: %v\n%s", err, out)
	}
}

func TestIssueTitleFuzzySearch(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), "模糊搜索标题") {
		t.Fatal("issue list must advertise title fuzzy search")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping issue title fuzzy-search behavior check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
eval(code.slice(code.indexOf('function normalizeIssueTitleSearch('), code.indexOf('function filteredIssues(')));
const issue = {number: 42, title: '全量 PHPUnit-回归', description: '覆盖 Acme 主流程', externalRef: {externalId: 'GH-100'}};
const project = {key: 'DP'};
assert.equal(issueMatchesQuery(issue, project, 'php unit'), true);
assert.equal(issueMatchesQuery(issue, project, '回归 php'), true);
assert.equal(issueMatchesQuery(issue, project, 'PHPUNIT回归'), true);
assert.equal(issueMatchesQuery(issue, project, 'DP-42'), true);
assert.equal(issueMatchesQuery(issue, project, 'Acme'), true);
assert.equal(issueMatchesQuery(issue, project, '不存在的标题'), false);
`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("issue title fuzzy-search behavior check failed: %v\n%s", err, out)
	}
}

func TestWorkspaceBrowserDefaultsAndLanguageHighlighting(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping workspace language helper check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('vscode.js', 'utf8');
const app = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
assert.ok(code.includes("workbench.web.main"), 'workspace page must boot vscode-web');
assert.ok(code.includes("loopforge-workspace"), 'workspace page must load the run filesystem extension');
assert.ok(code.includes("scheme: 'loopforge'"), 'workspace page must open an loopforge virtual folder');
assert.ok(app.includes('data-vscode-frame'), 'run workspace must embed vscode iframe');
assert.ok(app.includes('vscode.html'), 'run workspace must load vscode workbench page');
assert.ok(app.includes('openVSCodeWindow'), 'run workspace must allow a detached vscode window');
assert.ok(app.includes('当前无法浏览工作区'), 'finished runs must show the deleted-workspace empty state');
assert.ok(app.includes('loadRunWorkspace'), 'workspace tab must probe availability before embedding vscode');
assert.ok(app.includes('runWorkspaceWasCleaned'), 'year-1 JSON timestamps must not be treated as a cleaned workspace');
eval(app.slice(app.indexOf('function runWorkspaceWasCleaned('), app.indexOf('function workspaceIsAvailable(')));
assert.equal(runWorkspaceWasCleaned({status:'running', workspaceCleanedAt:'0001-01-01T00:00:00Z'}), false);
assert.equal(runWorkspaceWasCleaned({status:'succeeded', workspaceCleanedAt:'0001-01-01T00:00:00Z'}), false);
assert.equal(runWorkspaceWasCleaned({status:'succeeded', workspaceCleanedAt:'1970-01-01T00:00:00Z'}), false);
assert.equal(runWorkspaceWasCleaned({status:'succeeded', workspaceCleanedAt:'2026-09-17T04:00:00Z'}), true);
assert.equal(runWorkspaceWasCleaned({status:'running'}), false);
assert.ok(code.includes("window.open(vscodeWorkbenchURL(id), vscodeWindowName(id)"), 'vscode workbench must open a named window');
assert.ok(code.includes('info.available'), 'standalone vscode must refuse to boot when the workspace is gone');
assert.ok(code.includes("'git.enabled': false"), 'vscode-web has no git CLI; keep builtin git off against the virtual folder');
assert.ok(code.includes("params.set('changes'"), 'vscode-web must pass workspace git changes into the workspace folder query');
assert.ok(code.includes('info.repositories'), 'vscode-web must use backend git roots instead of treating every top-level path as a repo');
assert.ok(!code.includes("entry.type === 'directory'"), 'vscode-web must not treat workspace folders as nested git repositories');
const extension = require('node:fs').readFileSync('extensions/loopforge-workspace/extension.js', 'utf8');
const pkg = require('node:fs').readFileSync('extensions/loopforge-workspace/package.json', 'utf8');
assert.ok(code.includes("'scm.defaultViewMode': 'list'"), 'vscode-web must keep native SCM list/tree toggle defaults');
assert.ok(code.includes("'scm.experimental.showHistoryGraph': false"), 'vscode-web must keep the experimental git graph off so commits expand to files');
assert.ok(code.includes("'scm.showOutgoingChanges': 'auto'"), 'vscode-web must show outgoing commits that can expand to a file list');
assert.ok(code.includes("'scm.showChangesSummary': false"), 'vscode-web must not insert an All Changes summary above commit files');
assert.ok(extension.includes('gitRefQuery'), 'commit file URIs must use JSON ref queries so native vscode.diff can open them');
assert.ok(extension.includes('createSourceControl'), 'workspace must use vscode native SCM providers for nested git');
assert.ok(extension.includes("createSourceControl(id, 'Git'"), 'SCM provider label must stay Git so the repo name is not duplicated');
assert.ok(extension.includes('createResourceGroup'), 'workspace SCM must use native staged/changes/untracked groups');
assert.ok(extension.includes('historyProvider'), 'workspace SCM must expose native git history');
assert.ok(extension.includes('loopforge.refresh'), 'workspace SCM must provide a native refresh command');
assert.ok(extension.includes('startStatusAutoRefresh'), 'workspace SCM must auto-refresh git status like vscode');
assert.ok(extension.includes('setTimeout'), 'workspace SCM must keep a git status poll loop');
assert.ok(extension.includes('onDidChangeWindowState'), 'workspace SCM must refresh git status when the window is focused');
assert.ok(extension.includes('ahead: commits.length'), 'history provider must resolve an ancestor so commits appear as an expandable list');
assert.ok(pkg.includes('scmHistoryProvider'), 'workspace extension must enable the SCM history proposal');
assert.ok(pkg.includes('view == workbench.scm'), 'refresh must sit on the SCM view title once, not per repository');
assert.ok(pkg.includes('"scm/title"'), 'refresh must use scm/title because the SCM pane does not use view/title');
assert.ok(pkg.includes('$(refresh)'), 'workspace extension must contribute a refresh icon on the SCM title');
assert.ok(!extension.includes('SOP 更改'), 'workspace must not invent a custom SOP source control');
assert.ok(!extension.includes('mountGitChanges'), 'workspace must not paint a custom git HTML overlay');
assert.ok(extension.includes('loopforge-base'), 'workspace extension must open diffs against HEAD');
assert.ok(extension.includes("command: 'loopforge.openChange'"), 'SCM resources must open a file preview on click');
assert.ok(extension.includes("'vscode.diff'"), 'modified SCM resources must open the native diff editor');
assert.ok(extension.includes("'vscode.open'"), 'untracked SCM resources must open a file preview');
eval(extension.slice(extension.indexOf('function changeCode('), extension.indexOf('function changeDecoration(')));
eval(extension.slice(extension.indexOf('function shouldDiffChange('), extension.indexOf('function changeResourceState(')));
eval(extension.slice(extension.indexOf('function workspaceRepoName('), extension.indexOf('class LoopForgeFS')));
{
  const vscodeStub = { EventEmitter: class { constructor() { this.event = () => ({ dispose() {} }); } fire() {} }, Disposable: class {} };
  const src = extension.slice(extension.indexOf('class LoopForgeFS'), extension.indexOf('function workspaceUri('));
  const providers = new Function('vscode', src + '; return [LoopForgeFS, LoopForgeBaseFS];')(vscodeStub);
  for (const Provider of providers) {
    assert.equal(typeof new Provider('run-1').onDidChangeFile, 'function', Provider.name + ' must expose onDidChangeFile or registerFileSystemProvider aborts activation');
  }
}
assert.equal(changeCode({index:'M', worktree:' '}), 'M');
assert.equal(changeCode({index:' ', worktree:'?'}), 'U');
assert.equal(shouldDiffChange({index:'M', worktree:' '}), true);
assert.equal(shouldDiffChange({index:' ', worktree:'?'}), false);
const grouped = repositoriesFromChanges([{path:'a/main.go', repository:'a'}, {path:'b/app.ts', repository:'b'}], [{path:'a', type:'directory'}, {path:'b', type:'directory'}]);
assert.equal(grouped.length, 2);
assert.equal(grouped[0].name, 'a');
assert.equal(grouped[1].name, 'b');
const rootGit = repositoriesFromChanges([{path:'e2e-cases.md'}, {path:'docs/readme.md'}], [{path:'docs', type:'directory'}, {path:'e2e-cases.md', type:'file'}]);
assert.equal(rootGit.length, 1);
assert.equal(rootGit[0].name, 'workspace');
assert.equal(sourceControlRootPath('workspace'), '/');
assert.equal(sourceControlRootPath('.'), '/');
assert.equal(sourceControlRootPath('acme-web'), '/acme-web');
eval(extension.slice(extension.indexOf('function changesForRepo('), extension.indexOf('function changesSignature(')));
assert.equal(changesForRepo([{path:'e2e-cases.md'}, {path:'a/main.go', repository:'a'}], 'workspace').length, 1);
assert.equal(changesForRepo([{path:'e2e-cases.md'}, {path:'a/main.go', repository:'a'}], 'a').length, 1);
eval(extension.slice(extension.indexOf('function changesSignature('), extension.indexOf('function applyRepoChanges(')));
assert.equal(changesSignature([{path:'b',index:'M',worktree:' '},{path:'a',index:' ',worktree:'M'}]), changesSignature([{path:'a',index:' ',worktree:'M'},{path:'b',index:'M',worktree:' '}]));
assert.notEqual(changesSignature([{path:'a',index:' ',worktree:'M'}]), changesSignature([]));
`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("workspace language helper check failed: %v\n%s", err, out)
	}
	css, err := os.ReadFile("observability.css")
	if err != nil {
		t.Fatal(err)
	}
	styles := string(css)
	for _, want := range []string{".run-workspace.vscode-embed{display:block", "height:clamp(520px,calc(100dvh - 230px),760px)", ".run-workspace.vscode-embed iframe{width:100%;height:100%", ".run-workspace.vscode-embed.maximized{position:fixed"} {
		if !strings.Contains(styles, want) {
			t.Fatalf("workspace independent scrolling CSS is missing %q", want)
		}
	}
}

func TestVSCodeJSParses(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping vscode.js parse check")
	}
	out, err := exec.Command(node, "--check", "vscode.js").CombinedOutput()
	if err != nil {
		t.Fatalf("vscode.js failed to parse: %v\n%s", err, out)
	}
}

func TestIssueFilterAndManualCloseUI(t *testing.T) {
	htmlBytes, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlBytes)
	for _, want := range []string{
		`id="issueStatusFilter"`,
		`id="issueAssigneeFilter"`,
		`<option value="backlog">待规划</option>`,
		`<option value="todo">待处理 / 待执行</option>`,
		`<option value="done">已完成</option>`,
		`<option value="closed">已关闭</option>`,
		`data-dtab="operations"`,
		`未指定责任人`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("index.html is missing %q", want)
		}
	}

	appBytes, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	app := string(appBytes)
	for _, want := range []string{
		"issueStatusFilter:''",
		"issueAssigneeFilter:''",
		"$('#issueStatusFilter')",
		"$('#issueAssigneeFilter')",
		"renderIssueAssigneeFilter",
		"closeIssueById",
		"data-close-issue",
		"/api/issues/${encodeURIComponent(issueId)}/close",
		"data-issue-menu",
		"showIssueActionMenu",
		"ensureIssueMenuPopover",
		"issue-action-popover",
		"编辑需求",
		"issueMatchesStatusFilter",
		"issueWaitingToExecute",
		"['closed','Closed']",
		"operationsPane",
		"function runRouteId()",
		"function flowRouteId()",
		"function restoreOpenWorkflow()",
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("app.js is missing %q", want)
		}
	}
	if strings.Contains(app, "submit.textContent=isNew?'同步并保存':'保存 Issue'") {
		t.Fatalf("app.js still contains '保存 Issue' in submit button")
	}

	node, err := exec.LookPath("node")
	if err == nil {
		nodeScript := `
const fs = require('node:fs');
const code = fs.readFileSync('` + appBundlePath(t) + `', 'utf8');
const mockEl = () => ({
  classList: { toggle: () => {}, contains: () => false, add: () => {}, remove: () => {} },
  style: {},
  dataset: {},
  innerHTML: '',
  appendChild: () => {},
  addEventListener: () => {},
  querySelector: () => mockEl(),
  querySelectorAll: () => []
});
global.window = global;
global.location = { origin: 'http://localhost' };
global.document = {
  hidden: false,
  documentElement: { dataset: {} },
  querySelector: () => mockEl(),
  querySelectorAll: () => [],
  getElementById: () => mockEl(),
  body: mockEl()
};
global.$ = () => mockEl();
global.$$ = () => [];
global.localStorage = { getItem: () => null, setItem: () => {} };
global.fetch = () => Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
eval(code + '; global.state = state; global.renderIssues = renderIssues; global.issueMatchesStatusFilter = issueMatchesStatusFilter; global.issueFinished = issueFinished; global.issueRunHealth = issueRunHealth; global.issueStatusBucket = issueStatusBucket; global.deliveryLockedLabel = deliveryLockedLabel; global.reopenDeliveredButton = reopenDeliveredButton; global.canRetriggerIssue = canRetriggerIssue; global.issueOutcome = issueOutcome; global.issueCurrentStage = issueCurrentStage; global.issueListProgress = issueListProgress;');
state.data = { issues: [], projects: [], repositories: [], workflows: [], runs: [], tasks: [] };
state.listPages = {};
renderIssues();
state.data.issues = [{ id: 'iss-1', projectId: 'p1', number: 1, title: 'test', status: 'backlog' }];
renderIssues();
const noRunCases = [
  ['in_progress', 'in_progress'],
  ['in_review', 'in_review'],
  ['blocked', 'blocked']
];
for (const [status, expected] of noRunCases) {
  const issue = { id: 'case-' + status, status };
  if (!issueMatchesStatusFilter(issue, expected) || issueMatchesStatusFilter(issue, 'todo')) {
    throw new Error(status + ' without a run was swallowed by the todo filter');
  }
}
if (!issueMatchesStatusFilter({ id: 'backlog-case', status: 'backlog' }, 'backlog') || issueMatchesStatusFilter({ id: 'backlog-case', status: 'backlog' }, 'todo')) {
  throw new Error('backlog must follow its persisted status instead of being folded into todo');
}
const namedNodeIssue = { id: 'named-node', status: 'in_progress', workflowId: 'wf' };
state.data.workflows = [{ id: 'wf', nodes: [{ id: 'node', name: '已完成' }] }];
state.data.runs = [{ id: 'named-node-run', status: 'running', executionContext: { issueId: namedNodeIssue.id }, nodes: { node: { status: 'running' } } }];
if (issueFinished(namedNodeIssue) || issueMatchesStatusFilter(namedNodeIssue, 'done')) {
  throw new Error('a running node named 已完成 must not mark its issue complete');
}
const trackerClosed = { id: 'gh-closed', source: 'github', kind: 'requirement', status: 'closed', closeSource: 'external', externalRef: { externalState: 'closed' } };
if (issueRunHealth(trackerClosed) !== 'closed' || issueStatusBucket(trackerClosed) !== 'closed' || !issueMatchesStatusFilter(trackerClosed, 'closed') || issueMatchesStatusFilter(trackerClosed, 'done')) {
  throw new Error('closed external work item without delivery must use the closed filter');
}
if (issueCurrentStage(trackerClosed).label !== '已关闭' || issueListProgress(trackerClosed, null, null).pct !== 0) {
  throw new Error('externally closed item without a run must display closed with zero SOP progress');
}
const forgeClosed = { id: 'gf-closed', source: 'github', status: 'closed', closeSource: 'external', externalRef: { externalState: 'closed' } };
if (issueStatusBucket(forgeClosed) !== 'closed' || issueCurrentStage(forgeClosed).label !== 'GitHub 已关闭') {
  throw new Error('closed forge issue without delivery must stay in the closed filter');
}
const forgeMerged = { id: 'gf-merged', source: 'github', status: 'closed', closeSource: 'external', externalRef: { externalState: 'closed' }, deliveryState: 'merged', deliveryMrState: 'merged', deliveryMrUrl: 'https://github.com/org/repo/pull/9' };
if (issueCurrentStage(forgeMerged).label !== 'PR 已合入' || issueOutcome(forgeMerged).title !== 'PR 已合入' || issueStatusBucket(forgeMerged) !== 'done' || issueMatchesStatusFilter(forgeMerged, 'closed')) {
  throw new Error('a merged PR must outrank a closed Forge issue');
}
const forgeIntegrated = { id: 'gf-integrated', source: 'github', status: 'closed', closeSource: 'external', externalRef: { externalState: 'closed' }, deliveryState: 'merged', deliveryMrState: 'integrated', deliveryMrUrl: 'https://github.com/org/repo/pull/10' };
if (issueCurrentStage(forgeIntegrated).label !== '已合入基线' || issueStatusBucket(forgeIntegrated) !== 'done') {
  throw new Error('baseline merge must outrank a closed Forge issue');
}
const delivered = { id: 'delivered', source: 'github', kind: 'requirement', status: 'closed', closeSource: 'external', deliveryState: 'merged', deliveryMrState: 'opened', externalRef: { externalState: 'closed' } };
if (issueStatusBucket(delivered) !== 'closed' || issueMatchesStatusFilter(delivered, 'done') || issueCurrentStage(delivered).label !== '已关闭' || reopenDeliveredButton(delivered)) {
  throw new Error('external closure must remain closed even after local delivery');
}
if (canRetriggerIssue(delivered, { status: 'succeeded' }) || issueOutcome(delivered).title !== '外部工作项已关闭') {
  throw new Error('externally closed tracker delivery must not offer a local retrigger');
}
const closedMR = { id: 'closed-mr', status: 'done', deliveryState: 'merged', deliveryMrState: 'closed' };
if (issueStatusBucket(closedMR) !== 'closed' || !issueMatchesStatusFilter(closedMR, 'closed') || issueMatchesStatusFilter(closedMR, 'done')) {
  throw new Error('a closed PR marker without a link must leave the completed filter');
}
if (issueCurrentStage(closedMR).label !== '已关闭' || issueListProgress(closedMR, null, null).pct !== 0) {
  throw new Error('manual close forged as a closed PR must display closed with zero progress');
}
const linkedClosedMR = { id: 'linked-closed-mr', status: 'done', deliveryState: 'merged', deliveryMrState: 'closed', deliveryMrUrl: 'https://github.com/org/repo/pull/2' };
if (issueCurrentStage(linkedClosedMR).label !== 'PR 已关闭' || issueStatusBucket(linkedClosedMR) !== 'closed' || issueListProgress(linkedClosedMR, null, null).pct !== 100) {
  throw new Error('a linked closed PR must stay PR closed in the single stage column');
}
const mergedMR = { id: 'merged-mr', status: 'done', deliveryState: 'merged', deliveryMrState: 'merged', deliveryMrUrl: 'https://github.com/org/repo/pull/1' };
if (issueStatusBucket(mergedMR) !== 'done' || issueCurrentStage(mergedMR).label !== 'PR 已合入' || issueMatchesStatusFilter(mergedMR, 'closed')) {
  throw new Error('merged PR must stay in the completed filter under the existing stage column');
}
if (issueOutcome(mergedMR).title !== 'PR 已合入' || !canRetriggerIssue(mergedMR, { status: 'succeeded' })) {
  throw new Error('a delivered but externally open item must keep preview retrigger behavior');
}
const manuallyClosed = { id: 'manually-closed', status: 'closed', deliveryState: 'merged', deliveryMrState: 'integrated' };
if (issueStatusBucket(manuallyClosed) !== 'closed' || issueMatchesStatusFilter(manuallyClosed, 'done')) {
  throw new Error('persisted closed status must be authoritative');
}
if (deliveryLockedLabel(manuallyClosed) !== '关闭' || !reopenDeliveredButton(manuallyClosed).includes('data-reopen-issue')) {
  throw new Error('manual close must display as closed and remain reopenable without fake delivery state');
}
const succeededIssue = { id: 'succeeded-issue', status: 'done' };
state.data.tasks.push({ id: 'succeeded-task', issueId: succeededIssue.id, runId: 'succeeded-run', status: 'completed', createdAt: '2026-09-23T00:00:00Z' });
state.data.runs.push({ id: 'succeeded-run', status: 'succeeded', nodes: {}, createdAt: '2026-09-23T00:00:00Z' });
if (issueCurrentStage(succeededIssue).label !== '交付闭环' || issueListProgress(succeededIssue, state.data.runs.at(-1), null).pct !== 100) {
  throw new Error('persisted completion with a successful run must display delivery closure');
}
const reopenedReview = { id: 'reopened-review', status: 'in_review', deliveryState: 'reopened' };
state.data.tasks.push({ id: 'reopened-task', issueId: reopenedReview.id, runId: 'reopened-run', status: 'completed', createdAt: '2026-09-23T00:00:00Z' });
state.data.runs.push({ id: 'reopened-run', status: 'succeeded', nodes: {}, createdAt: '2026-09-23T00:00:00Z' });
if (issueCurrentStage(reopenedReview).label !== '评审中' || issueStatusBucket(reopenedReview) !== 'in_review' || issueMatchesStatusFilter(reopenedReview, 'done')) {
  throw new Error('a reopened issue must follow its current status instead of the old succeeded run');
}
const statusMatrix = [
  ['backlog', 'backlog'],
  ['todo', 'todo'],
  ['in_progress', 'in_progress'],
  ['in_review', 'in_review'],
  ['done', 'done'],
  ['closed', 'closed'],
  ['blocked', 'blocked'],
  ['cancelled', 'cancelled']
];
for (const [stored, bucket] of statusMatrix) {
  const issue = { id: 'matrix-' + stored, status: stored };
  const matches = ['', 'backlog', 'todo', 'in_progress', 'in_review', 'done', 'closed', 'blocked', 'cancelled'].filter(filter => issueMatchesStatusFilter(issue, filter));
  if (issueStatusBucket(issue) !== bucket || matches.length !== 2 || !matches.includes('') || !matches.includes(bucket)) {
    throw new Error('status bucket is not exclusive for ' + stored + ': ' + matches.join(','));
  }
}
`
		if out, err := exec.Command(node, "-e", nodeScript).CombinedOutput(); err != nil {
			t.Fatalf("renderIssues node execution check failed: %v\n%s", err, out)
		}
	}
}

func TestClarificationConclusionAndWriteBackUX(t *testing.T) {
	appBytes, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	appCode := string(appBytes)

	for _, want := range []string{
		`needs-confirm`,
		`需要确认结论`,
		`确认写回原单（追加评论）`,
		`clarify-next-step-callout`,
		`去确认结论`,
		`data-dispatch-issue`,
	} {
		if !strings.Contains(appCode, want) {
			t.Fatalf("app.js missing %q", want)
		}
	}

	cssBytes, err := os.ReadFile("platform.css")
	if err != nil {
		t.Fatal(err)
	}
	cssCode := string(cssBytes)
	for _, want := range []string{
		`.issue-table-row.needs-confirm`,
		`.kanban-card.needs-confirm`,
		`.issue-progress-cell.confirm`,
	} {
		if !strings.Contains(cssCode, want) {
			t.Fatalf("platform.css missing %q", want)
		}
	}

	node, err := exec.LookPath("node")
	if err == nil {
		nodeScript := `
const fs = require('fs');
const code = fs.readFileSync('` + appBundlePath(t) + `', 'utf8');
const mockEl = () => ({
  dataset: {},
  classList: { add: () => {}, remove: () => {}, toggle: () => {}, contains: () => false },
  appendChild: () => {},
  addEventListener: () => {},
  setAttribute: () => {},
  style: {},
  querySelector: () => mockEl(),
  querySelectorAll: () => []
});
global.window = {
  addEventListener: () => {},
  removeEventListener: () => {},
  innerWidth: 1200,
  innerHeight: 800
};
global.document = {
  hidden: false,
  documentElement: { dataset: {} },
  querySelector: () => mockEl(),
  querySelectorAll: () => [],
  getElementById: () => mockEl(),
  body: mockEl()
};
global.$ = () => mockEl();
global.$$ = () => [];
global.localStorage = { getItem: () => null, setItem: () => {} };
eval(code + '; global.issueRow = issueRow; global.clarifyPane = clarifyPane; global.state = state;');

// 1. Issue with pending confirm
const pendingIssue = {
  id: 'iss-test-1',
  kind: 'requirement',
  source: 'github',
  kind: 'requirement',
  externalRef: { source: 'github', externalId: '1001' },
  clarification: { status: 'ready', writeBackStatus: 'pending', score: 84, summary: '测试结论' }
};
state.data = { projects: [], repositories: [], workflows: [], runs: [], tasks: [] };
const rowHtml = issueRow(pendingIssue);
if (!rowHtml.includes('needs-confirm')) {
  console.error('issueRow does not contain needs-confirm:', rowHtml);
  process.exit(1);
}
if (!rowHtml.includes('需要确认结论')) {
  console.error('issueRow does not contain 需要确认结论:', rowHtml);
  process.exit(1);
}

// 2. Clarify pane before write back
const pendingPaneHtml = clarifyPane(pendingIssue, null);
if (!pendingPaneHtml.includes('确认写回原单（追加评论）')) {
  console.error('clarifyPane pending does not contain 确认写回原单（追加评论）:', pendingPaneHtml);
  process.exit(1);
}

// 3. Clarify pane after write back
const succeededIssue = {
  id: 'iss-test-2',
  kind: 'requirement',
  source: 'github',
  kind: 'requirement',
  externalRef: { source: 'github', externalId: '1002' },
  clarification: { status: 'ready', writeBackStatus: 'succeeded', score: 84, summary: '测试结论' }
};
const readyPaneHtml = clarifyPane(succeededIssue, null);
if (!readyPaneHtml.includes('data-dispatch-issue="iss-test-2"')) {
  console.error('clarifyPane ready does not contain data-dispatch-issue:', readyPaneHtml);
  process.exit(1);
}
if (!readyPaneHtml.includes('触发需求')) {
  console.error('clarifyPane ready does not contain 触发需求:', readyPaneHtml);
  process.exit(1);
}
// 4. Dispatched task active state handling
const dispatchedIssue = {
  id: 'iss-test-3',
  kind: 'requirement',
  source: 'github',
  kind: 'requirement',
  externalRef: { source: 'github', externalId: '1003' },
  clarification: { status: 'ready', writeBackStatus: 'succeeded', score: 84, summary: '测试结论' }
};
const activeTask = { id: 'task-test-3', issueId: 'iss-test-3', status: 'running' };
state.data.tasks = [activeTask];
state.data.runs = [];

const health = issueRunHealth(dispatchedIssue);
if (health !== 'active') {
  console.error('issueRunHealth should be active for running task, got:', health);
  process.exit(1);
}
const stage = issueCurrentStage(dispatchedIssue);
if (stage.key !== 'active') {
  console.error('issueCurrentStage key should be active for running task, got:', stage);
  process.exit(1);
}
`
		if out, err := exec.Command(node, "-e", nodeScript).CombinedOutput(); err != nil {
			t.Fatalf("clarification UX node test failed: %v\n%s", err, out)
		}
	}
}

func TestGenericTemplateEditorUsesBundledRoles(t *testing.T) {
	appBytes, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	app := string(appBytes)
	for _, want := range []string{
		"genericTemplateAgents",
		"hydrateGenericWorkflowRoles",
		"refreshGenericWorkflowRoles",
		"/api/workflows/${encodeURIComponent(id)}/agents",
		"/api/workflows/${encodeURIComponent(state.workflow.id)}/agents",
		"这些角色打包在通用模板里",
		"该角色不属于当前通用模板",
		"通用模板角色",
		"agentWizardSubject",
		"agentWizardForeign",
		"genericTemplateMCPOptions",
		"cloneBtn.hidden=catalogGeneric(state.workflow)||!foreign",
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("app.js is missing generic template role editor wiring %q", want)
		}
	}
	if strings.Contains(app, "function workflowAgents(){const projectId=state.workflow?.projectId||state.projectFilter||'';return (state.data?.agents||[]).filter") {
		t.Fatal("workflowAgents still binds generic templates to the current project catalog")
	}
	if strings.Contains(app, "const foreign=catalogForeign((state.data.agents.find(x=>x.id===state.selectedAgent)||state.agentDraft||{}).projectId)") {
		t.Fatal("agent wizard still treats a same-id project clone as a foreign generic role")
	}
	for _, want := range []string{"renderPlatformAdminPanel", "platformAdminPanel", "谁可以改通用模板"} {
		if !strings.Contains(app, want) {
			t.Fatalf("app.js is missing platform admin panel wiring %q", want)
		}
	}
	if strings.Contains(app, `id="platformAdminCard"`) {
		t.Fatal("platform admin management is still embedded in personal settings")
	}
	htmlBytes, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(htmlBytes), `id="platformAdminPanel"`) {
		t.Fatal("index.html is missing the generic-template platform admin panel")
	}
}

// 管理员名单是平台级权限，既不属于某个项目，也不属于个人凭据，
// 所以它有自己的导航入口，而不是挂在流程模板页上。
func TestPlatformAdminLivesOnItsOwnView(t *testing.T) {
	app, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	source, markup := string(app), string(html)

	for _, want := range []string{
		`id="platformView"`,
		`id="platformNavGroup"`,
		`data-view="platform"`,
	} {
		if !strings.Contains(markup, want) {
			t.Fatalf("index.html is missing the platform admin view wiring %q", want)
		}
	}
	if strings.Contains(markup, `<div id="platformAdminPanel" class="platform-admin-panel" hidden></div>`) {
		t.Fatal("platform admin panel is still embedded in the SOP catalog page")
	}
	for _, want := range []string{
		"platform:'平台管理'",
		"if(name==='platform')renderPlatformAdminPanel()",
		"renderPlatformNav",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("app.js is missing the platform view wiring %q", want)
		}
	}
	// 入口只对平台或业务域管理员渲染，未授权的 #/platform 要被挡回去；
	// 后端的 403 是底线，但不能靠它来当第一道门。
	if !strings.Contains(source, "group.hidden=!canAccessPlatformManagement()") {
		t.Fatal("the platform nav entry is rendered for non-admins")
	}
	if !strings.Contains(source, "if(name==='platform'&&!isPlatformAdmin())name='chat'") {
		t.Fatal("a hand-typed #/platform is not blocked for non-admins")
	}
	if strings.Contains(source, "state.sopScope==='generic'&&isPlatformAdmin()") {
		t.Fatal("platform admin visibility is still tied to the generic SOP tab")
	}
}

// 加载失败时若把模板 id 记成「已加载」，那个标记会让后续请求全部短路，
// 门禁就会一直显示成没有配置——这正是它在界面上消失的原因。
func TestGenericRoleHydrationRetriesAfterFailure(t *testing.T) {
	app, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	source := string(app)

	start := strings.Index(source, "async function hydrateGenericWorkflowRoles(")
	if start < 0 {
		t.Fatal("hydrateGenericWorkflowRoles is gone")
	}
	body := source[start:]
	if end := strings.Index(body, "\nasync function refreshGenericWorkflowRoles("); end > 0 {
		body = body[:end]
	}
	failure := strings.Index(body, "}catch(")
	if failure < 0 {
		t.Fatal("hydrateGenericWorkflowRoles no longer handles load failures")
	}
	if strings.Contains(body[failure:], "state.genericTemplateAgentWorkflowId=id") {
		t.Fatal("a failed role load is cached as hydrated, so the gates never come back")
	}
	if !strings.Contains(body[failure:], "clearGenericTemplateAgents()") {
		t.Fatal("a failed role load must reset the hydration marker so it can retry")
	}
}

// 通用模板角色走乐观并发：提交的必须是这次编辑所基于的版本。
// 多加一个 +1 会让服务端每次都判成冲突，而且只有两个管理员同时改时才暴露。
func TestGenericRoleSaveSendsTheVersionItEdited(t *testing.T) {
	app, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	source := string(app)
	if strings.Contains(source, "a.version=(bundled.version||0)+1") {
		t.Fatal("generic role save sends a bumped version, which the store reads as a stale write")
	}
	if !strings.Contains(source, "a.version=bundled.version||0") {
		t.Fatal("generic role save no longer sends the version it based the edit on")
	}
}

func TestRunRetryContextSkipsScriptNodes(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping run retry context check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
const start = code.indexOf('function failedNodeId(run){');
const end = code.indexOf('function canManuallyPassFailedNode(');
assert.ok(start >= 0 && end > start);
eval(code.slice(start, end));
global.state = {data:{runs:[],workflows:[{id:'script-wf',nodes:[{id:'script',type:'command'}]}]}};
const scriptFail = {id:'run-1',status:'failed',workflowId:'script-wf',workflowSnapshot:{nodes:[{id:'script',type:'command'}]},nodes:{script:{status:'failed'}}};
const agentFail = {id:'run-2',status:'failed',workflowSnapshot:{nodes:[{id:'agent',type:'agent'}]},nodes:{agent:{status:'failed'}}};
const cancelled = {id:'run-3',status:'cancelled',workflowSnapshot:{nodes:[{id:'agent',type:'agent'}]},nodes:{agent:{status:'failed'}}};
assert.equal(runRetryNeedsContext(scriptFail), false);
assert.equal(runRetryNeedsContext(agentFail), true);
assert.equal(runRetryNeedsContext(cancelled), false);
assert.ok(code.includes("if(!context && !fromEl?.closest?.('[data-human-context-root]') && runRetryNeedsContext(run))"));
assert.ok(code.includes("r.status==='failed'&&!forgeIssueClosed(ctx.issue)"));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("run retry context check failed: %v\n%s", err, out)
	}
}

func TestPreviewIframeHttpsFallback(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping preview https fallback test")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
assert.ok(code.includes('function previewCandidateHttpsURL('), 'must declare previewCandidateHttpsURL');
assert.ok(code.includes('async function resolvePreviewSessionURL('), 'must declare resolvePreviewSessionURL');
assert.ok(code.includes('session.url=await resolvePreviewSessionURL(session.url)'), 'loadRunPreview must resolve session url');

// Test previewCandidateHttpsURL logic
const start = code.indexOf('function previewCandidateHttpsURL(');
const end = code.indexOf('async function resolvePreviewSessionURL(');
assert.ok(start >= 0 && end > start);
eval(code.slice(start, end));

global.window = { location: { href: 'https://loopforge-test.example.com/runs', protocol: 'https:' } };
assert.equal(previewCandidateHttpsURL('http://loopforge-test.example.com/preview/token/'), 'https://loopforge-test.example.com/preview/token/');
assert.equal(previewCandidateHttpsURL('http://loopforge-test.example.com:80/preview/token/'), 'https://loopforge-test.example.com/preview/token/');
assert.equal(previewCandidateHttpsURL('https://loopforge-test.example.com/preview/token/'), 'https://loopforge-test.example.com/preview/token/');
assert.equal(previewCandidateHttpsURL(''), '');
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("preview https fallback check failed: %v\n%s", err, out)
	}
}
func TestResearchEfficiencyDashboardUI(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	for _, want := range []string{`data-view="metrics"`, `id="metricsView"`, `id="metricsBody"`, `data-metrics-range="7d"`, `data-metrics-range="30d"`, `data-metrics-range="90d"`, `data-metrics-range="365d"`, `id="metricsDateFrom"`, `id="metricsDateTo"`, `metrics-dashboard.css`} {
		if !strings.Contains(page, want) {
			t.Fatalf("研发效能看板缺少 %q", want)
		}
	}
	js, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	code := string(js)
	for _, want := range []string{"renderMetricsDashboard", "refreshMetrics", "metricsDisplayModel", "metricsTrendSVG", "metricsPhaseTrendSVG", "metricsRequirementTable", "metricsDonut", "metricsDemoEnabled", "metricsDemoSnapshot", "演示数据", "需求交付数", "平均需求耗时", "总耗时", "每条需求耗时明细", "平均澄清耗时", "平均研发耗时", "平均测试耗时", "AI 需求覆盖率", "Run 成功率"} {
		if !strings.Contains(code, want) {
			t.Fatalf("研发效能看板脚本缺少 %q", want)
		}
	}
	if strings.Contains(code, "第一阶段指标口径") {
		t.Fatal("研发效能看板不应展示指标口径表")
	}
}

func TestClarifyOptionToggleLogic(t *testing.T) {
	app, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	source := string(app)

	// Verify issue clarify option unselect on second click
	if !strings.Contains(source, "const wasSelected=b.classList.contains('selected');") {
		t.Fatal("issue clarify option does not track wasSelected to toggle off on repeat click")
	}
	if !strings.Contains(source, "if(!wasSelected)b.classList.add('selected');") {
		t.Fatal("issue clarify option does not conditionally add selected")
	}

	// Verify req clarify panel option unselect on second click
	if !strings.Contains(source, "const wasSelected=btn.classList.contains('selected');") {
		t.Fatal("req clarify panel option does not track wasSelected to toggle off on repeat click")
	}
	if !strings.Contains(source, "if(!wasSelected)btn.classList.add('selected');") {
		t.Fatal("req clarify panel option does not conditionally add selected")
	}

	// Verify CSS distinguishes hover from selected so cancellation is visually obvious
	v4css, err := os.ReadFile("product-v4.css")
	if err != nil {
		t.Fatal(err)
	}
	v4style := string(v4css)
	if strings.Contains(v4style, ".issue-clarify-option:hover,.issue-clarify-option.selected") {
		t.Fatal("issue-clarify-option must not share identical hover and selected styles")
	}
	if !strings.Contains(v4style, ".issue-clarify-option:hover{") || !strings.Contains(v4style, ".issue-clarify-option.selected{") {
		t.Fatal("issue-clarify-option must define separate hover and selected rules")
	}
}

func TestPlatformAdminEnhancedUI(t *testing.T) {
	app, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	css, err := os.ReadFile("platform.css")
	if err != nil {
		t.Fatal(err)
	}
	source, style := string(app), string(css)

	// Key elements in app.js
	for _, want := range []string{
		"platform-metric-grid",
		"platformMetricTotal",
		"platformMetricLocked",
		"platformMetricGranted",
		"platform-admin-layout",
		"platformAdminSearch",
		"platformFilterTabs",
		"platform-admin-side",
		"userAvatarHTML(row.loginName",
		"data-platform-nav",
		"switchView(target)",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("app.js missing platform admin enhancement: %q", want)
		}
	}

	// Key styles in platform.css
	for _, want := range []string{
		".platform-metric-grid",
		".platform-metric-card",
		".platform-admin-layout",
		".platform-admin-avatar",
		".platform-source-badge",
		".platform-admin-side",
		".platform-locked-tag",
		".platform-revoke-btn",
	} {
		if !strings.Contains(style, want) {
			t.Fatalf("platform.css missing style: %q", want)
		}
	}

	// Verify widescreen full-width adaptive support
	if strings.Contains(style, ".platform-admin-panel{max-width:1380px") || strings.Contains(style, "max-width:1380px;") && strings.Contains(style, ".platform-admin-panel") {
		t.Fatal("platform-admin-panel must not be locked to 1380px fixed max width")
	}
	v4css, err := os.ReadFile("product-v4.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(v4css), ".platform-admin-panel") || !strings.Contains(string(v4css), ".platform-admin-layout") {
		t.Fatal("product-v4.css must include platform-admin-panel and platform-admin-layout in full-width adaptive rules")
	}
}

func TestCollabSurfacesParse(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping collab parse check")
	}
	for _, name := range []string{"rooms.js", "orchestrate.js"} {
		out, err := exec.Command(node, "--check", name).CombinedOutput()
		if err != nil {
			t.Fatalf("%s failed to parse: %v\n%s", name, err, out)
		}
	}
	app, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	rooms, err := os.ReadFile("rooms.js")
	if err != nil {
		t.Fatal(err)
	}
	orch, err := os.ReadFile("orchestrate.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`rooms:'协作群聊'`, `loadRooms`} {
		if !strings.Contains(string(app), want) {
			t.Fatalf("app.js missing %q", want)
		}
	}
	for _, want := range []string{"roomsView", "aiOrchestrator", "建单并派发", "playWorkflowOnCanvas", `id="roomMention"`, "function roomMentionItems", "function roomGuideHTML", "roomWorkflowLabel"} {
		if !strings.Contains(string(html), want) && !strings.Contains(string(rooms), want) && !strings.Contains(string(orch), want) {
			t.Fatalf("collab UI missing %q", want)
		}
	}
	if !strings.Contains(string(orch), "ai-just-built") || !strings.Contains(string(orch), "ai-just-wired") {
		t.Fatal("canvas playback does not mark nodes and edges as they are built")
	}
	script := `
const assert = require('node:assert/strict');
const fs = require('node:fs');
const code = fs.readFileSync('orchestrate.js','utf8');
const start = code.indexOf('function layoutAIGraph');
const end = code.indexOf('function normalizeAIWorkflow');
eval(code.slice(start, end));
const nodes = [{id:'a',name:'澄清'},{id:'b',name:'实现'},{id:'c',name:'评审'}];
const edges = [{source:'a',target:'b'},{source:'b',target:'c'}];
layoutAIGraph(nodes, edges);
assert.ok(nodes[0].position.x < nodes[1].position.x);
assert.ok(nodes[1].position.x < nodes[2].position.x);
assert.equal(nodes[0].position.y, nodes[1].position.y);
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("layout check failed: %v\n%s", err, out)
	}
	editScript := `
const assert = require('node:assert/strict');
const fs = require('node:fs');
const code = fs.readFileSync('orchestrate.js','utf8');
const cut = (from, to) => code.slice(code.indexOf(from), code.indexOf(to));
function uid(prefix){ return prefix + '-new'; }
eval(cut('function normalizeAINode', 'function aiDecodePartialString'));
eval(cut('function aiClone', 'function aiOrchestratePrompt'));
assert.equal(aiWantsFreshWorkflow('清空当前画布，重新搭一条'), true);
assert.equal(aiWantsFreshWorkflow('补上构建静态站点，其它不要动'), false);
assert.equal(aiWantsPreviewRemoved('去掉构建静态站点'), true);
assert.equal(aiWantsPreviewRemoved('怎么没有构建静态站点节点'), false);
assert.equal(aiWantsPreviewRemoved('补上构建静态站点，其它不要动'), false);
const base = {id:'sop-1', name:'产品交付', nodes:[
  {id:'design', name:'方案', type:'agent', script:'keep-me'},
  {id:'af-preview-build', name:'构建静态站点', type:'agent', agentId:'af-preview-builder'}
], edges:[{id:'e1', source:'design', target:'af-preview-build', when:'success'}], preview:{enabled:true}};
const merged = mergeAIEdit(base, {name:'产品交付', nodes:[{id:'design', name:'方案设计', type:'agent'}], edges:[]}, '把方案改个名字', []);
assert.equal(merged.nodes.find(n => n.id==='design').name, '方案设计');
assert.equal(merged.nodes.find(n => n.id==='design').script, 'keep-me');
assert.ok(merged.nodes.some(n => n.id==='af-preview-build'));
assert.ok(merged.edges.some(e => e.target==='af-preview-build'));
assert.equal(merged.preview.enabled, true);
`
	out, err = exec.Command(node, "-e", editScript).CombinedOutput()
	if err != nil {
		t.Fatalf("orchestrator edit check failed: %v\n%s", err, out)
	}
}

func TestCSSBracesBalanced(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".css") {
			continue
		}
		content, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("failed to read %s: %v", entry.Name(), err)
		}
		open := 0
		for _, b := range content {
			if b == '{' {
				open++
			} else if b == '}' {
				open--
			}
		}
		if open != 0 {
			t.Fatalf("unbalanced braces in %s: open count is %d", entry.Name(), open)
		}
	}
}

func TestLoginUsesLocalAccounts(t *testing.T) {
	app, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"function openLoginModal(", "/api/auth/login", "/api/auth/password"} {
		if !strings.Contains(string(app), want) {
			t.Fatalf("app.js is missing %q", want)
		}
	}
	for _, banned := range []string{"passport", "loginUrl", "/api/auth/exchange", "/api/auth/local"} {
		if strings.Contains(string(app), banned) {
			t.Fatalf("app.js still references %q", banned)
		}
	}
}

func TestHumanGateDisclosureOnDecisionSurfaces(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
const escapeHTML = value => String(value??'').replace(/[&<>'"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[c]));
const isScriptWorkflowNode = node => node?.type==='script'||node?.type==='command';
eval(code.slice(code.indexOf('function turnsExhausted('), code.indexOf('function failedNodeId(')));
eval(code.slice(code.indexOf('function approvalCalloutHTML('), code.indexOf('async function retryWaitingNode(')));
const node = {id:'summary', name:'交付汇总', type:'agent'};
const nr = {status:'waiting_approval', activity:'等待人工确认', gateResult:{
  decision:'human',
  actionRequired:'请打开流水线 <build> 判断失败原因',
  reason:'合流检查 block=true',
  summary:'MR 559 未合并',
  findings:[{title:'合流检查失败', evidence:'build 241981168'}]
}};
const html = humanDecisionPanelHTML('run-1','summary',nr,node,[node]);
assert.match(html, /human-gate-brief/);
assert.match(html, /需要确认/);
assert.match(html, /请打开流水线 &lt;build&gt; 判断失败原因/);
assert.match(html, /Agent 判断依据/);
assert.match(html, /合流检查 block=true/);
assert.match(html, /摘要/);
assert.match(html, /MR 559 未合并/);
assert.match(html, /待确认项与证据/);
assert.match(html, /合流检查失败/);
assert.match(html, /build 241981168/);
assert.match(html, /确认通过，继续下游/);
const callout = approvalCalloutHTML(node, nr, [node]);
assert.match(callout, /需要确认/);
assert.match(callout, /请打开流水线 &lt;build&gt; 判断失败原因/);
assert.match(callout, /等待人工确认/);
const bare = humanDecisionPanelHTML('run-1','summary',{status:'waiting_approval'},node,[node]);
assert.match(bare, /判断依据/);
assert.match(bare, /门禁未通过。请看产物和日志后决定。/);
const planned = humanDecisionPanelHTML('run-1','approve',{status:'waiting_approval'},{id:'approve',type:'approval'},[]);
assert.match(planned, /预设的人工审批节点/);
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("human gate disclosure failed: %v\n%s", err, out)
	}
}

func TestImportedGitHubRequirementUsesRequirementDimensions(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
const clarificationPolicy = () => ({dimensions:[{id:'goal',name:'目标与价值'},{id:'scope',name:'范围与边界'}]});
eval(code.slice(code.indexOf('function issueClarifyDimensions('), code.indexOf('function clarifyScoreLevel(')));
assert.equal(issueClarifyDimensions({source:'github',kind:'requirement'},{})[0].id, 'goal');
assert.equal(issueClarifyDimensions({source:'github',kind:'issue'},{})[0].id, 'phenomenon');
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("clarification dimensions: %v\n%s", err, out)
	}
}

func TestNewSOPDefaultsMatchWhatWillActuallyRun(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
const state = {data:{sandboxes:[{id:'local'},{id:'generic-docker'}], users:[{id:1, codeBuddyApiKeyConfigured:true}]}, currentUser:{id:1}};
const userById = id => state.data.users.find(u => u.id === id);
const runnerStart = code.indexOf('const runnerCLILabels=');
eval(code.slice(runnerStart, code.indexOf('}', code.indexOf('return false;', runnerStart))+1).replace('const runnerCLILabels', 'var runnerCLILabels'));
eval(code.slice(code.indexOf('function effectiveExecutorFallback('), code.indexOf('function newWorkflow(')));
assert.equal(defaultSandboxProfileId(), 'generic-docker');
assert.equal(effectiveExecutorFallback({kind:'claude-code'}), 'CodeBuddy');
assert.equal(effectiveExecutorFallback({kind:'codebuddy'}), '');
assert.equal(effectiveExecutorFallback({kind:'mock'}), '');
state.data.sandboxes = [{id:'local'}];
assert.equal(defaultSandboxProfileId(), 'local');
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("new SOP defaults: %v\n%s", err, out)
	}
}

func TestDefaultSOPPrefersRealSOPOnceAKeyIsSet(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
const workflows = [{id:'demo-delivery'},{id:'quick-fix'},{id:'generic-backend-delivery', generic:true}];
const state = {projectFilter:'platform', data:{projects:[{id:'platform', workflowId:'demo-delivery'}], workflows}};
const runnableWorkflowOptions = () => workflows;
let playbook = null;
const projectPlaybook = () => playbook;
let hasKey = false;
const homeChatHasModelKey = () => hasKey;
eval(code.slice(code.indexOf('function defaultSOPForIssue('), code.indexOf('function bindIssueTriggers(')));
assert.equal(defaultSOPForIssue({projectId:'platform', kind:'requirement'}), 'demo-delivery');
assert.equal(defaultSOPForIssue({projectId:'platform', source:'github', kind:'issue'}), 'quick-fix');
hasKey = true;
assert.equal(defaultSOPForIssue({projectId:'platform', kind:'requirement'}), 'quick-fix');
playbook = {bindings:[{workflowId:'demo-delivery'}]};
assert.equal(defaultSOPForIssue({projectId:'platform', kind:'requirement'}), 'quick-fix');
playbook = {bindings:[{workflowId:'generic-backend-delivery'}]};
assert.equal(defaultSOPForIssue({projectId:'platform', kind:'requirement'}), 'generic-backend-delivery');
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("default SOP: %v\n%s", err, out)
	}
}

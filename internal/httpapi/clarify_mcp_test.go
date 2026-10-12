package httpapi

import (
	"strings"
	"testing"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/repo"
)

func TestClarifyAllowedModelFallsBackToCatalog(t *testing.T) {
	if got := clarifyAllowedModel(""); got != clarifyDefaultModel {
		t.Fatalf("empty model = %q", got)
	}
	if got := clarifyAllowedModel("not-a-model"); got != clarifyDefaultModel {
		t.Fatalf("unknown model = %q", got)
	}
	if got := clarifyAllowedModel("kimi-k2.7"); got != "kimi-k2.7" {
		t.Fatalf("listed model = %q", got)
	}
}

func TestClarifyCommandKeepsReadGrepGlob(t *testing.T) {
	command, err := prepareClarifyCommand("codebuddy", clarifyDefaultModel, "prompt", "clarify-test", "af1", false)
	if err != nil {
		t.Fatal(err)
	}
	foundSession, foundResume, foundNoPersist := false, false, false
	for i, arg := range command.Argv {
		if arg == "--tools" && i+1 < len(command.Argv) {
			tools := command.Argv[i+1]
			if tools != "" && tools != "default" {
				t.Fatalf("--tools %q hides MCP", tools)
			}
		}
		if arg == "--session-id" && i+1 < len(command.Argv) && command.Argv[i+1] == "af1" {
			foundSession = true
		}
		if arg == "--resume" {
			foundResume = true
		}
		if arg == "--no-session-persistence" {
			foundNoPersist = true
		}
		if arg == "--model" && i+1 < len(command.Argv) && command.Argv[i+1] != clarifyDefaultModel {
			t.Fatalf("model = %q", command.Argv[i+1])
		}
	}
	if !foundSession {
		t.Fatalf("missing --session-id af1: %v", command.Argv)
	}
	if foundResume {
		t.Fatalf("first turn should not --resume: %v", command.Argv)
	}
	if foundNoPersist {
		t.Fatalf("session persistence should stay enabled: %v", command.Argv)
	}
}

func TestPrepareClarifyCommandSubmitOnlyDisallowsRead(t *testing.T) {
	command, err := prepareClarifyCommandWith(clarifyCommandOpts{
		Bin: "codebuddy", Model: clarifyDefaultModel, Prompt: "submit", RunID: "clarify-test", SessionID: "af1",
		Native: true, SubmitOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	denied, ok := argvFlagValue(command.Argv, "--disallowedTools")
	if !ok || !strings.Contains(denied, "Read") || !strings.Contains(denied, "Grep") {
		t.Fatalf("submit-only must deny Read/Grep: %q argv=%v", denied, command.Argv)
	}
}

func TestPrepareClarifyCommandResumeUsesResumeFlag(t *testing.T) {
	command, err := prepareClarifyCommand("codebuddy", clarifyDefaultModel, "继续", "clarify-test", "af1", true)
	if err != nil {
		t.Fatal(err)
	}
	foundResume, foundSession := false, false
	for i, arg := range command.Argv {
		if arg == "--resume" && i+1 < len(command.Argv) && command.Argv[i+1] == "af1" {
			foundResume = true
		}
		if arg == "--session-id" {
			foundSession = true
		}
		if arg == "--no-session-persistence" {
			t.Fatalf("resume turn still has --no-session-persistence: %v", command.Argv)
		}
	}
	if !foundResume {
		t.Fatalf("missing --resume af1: %v", command.Argv)
	}
	if foundSession {
		t.Fatalf("resume turn should not also pass --session-id: %v", command.Argv)
	}
}

func TestStripClarifyMCPCalls(t *testing.T) {
	parsed := map[string]any{
		"summary": "need comments",
		"mcpCalls": []any{
			map[string]any{"server": "github", "tool": "get_issue_comments", "arguments": map[string]any{"issue_number": 1}},
		},
	}
	stripped := stripClarifyMCPCalls(parsed).(map[string]any)
	if _, ok := stripped["mcpCalls"]; ok {
		t.Fatal("mcpCalls should be stripped")
	}
	if stripped["summary"] != "need comments" {
		t.Fatalf("summary=%v", stripped["summary"])
	}
}

func TestClarifyMCPRejectsWrites(t *testing.T) {
	if !clarifyMCPCallWrite("github", assistantMCPCall{Tool: "create_issue"}) {
		t.Fatal("create_issue must be blocked")
	}
	if clarifyMCPCallWrite("github", assistantMCPCall{Tool: "get_issue_comments"}) {
		t.Fatal("get_issue_comments must stay allowed")
	}
	if !clarifyMCPWriteName("saveDocument") || !clarifyMCPWriteName("update_issue") {
		t.Fatal("document/tracker writes must be blocked")
	}
}

func TestHydrateClarifyIssueFromDraftText(t *testing.T) {
	gh := domain.Issue{Title: "x", Description: "见 https://github.com/acme/web/issues/18"}
	hydrateClarifyIssueFromText(&gh)
	if gh.Source != "github" || gh.ExternalRef == nil || gh.ExternalRef.ExternalIID != 18 {
		t.Fatalf("github hydrate=%+v ref=%+v", gh.Source, gh.ExternalRef)
	}
	if path := forgeProjectPathFromClarifyURL(gh.ExternalRef.ExternalURL); path != "acme/web" {
		t.Fatalf("repo path=%q", path)
	}
	plain := domain.Issue{Title: "x", Description: "no links here"}
	hydrateClarifyIssueFromText(&plain)
	if plain.ExternalRef != nil {
		t.Fatalf("plain text must not bind a tracker: %+v", plain.ExternalRef)
	}
}

func TestClarifyMCPInstructionsRequireGitHubIssue(t *testing.T) {
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	user, ok := st.UserByLoginName("demo-owner")
	if !ok {
		t.Fatal("seed user")
	}
	if err := st.PutMCPServer(domain.MCPServerDefinition{ID: "mcp-github-project-platform", Name: "GitHub MCP", ProjectID: "platform", MarketID: "github", Scope: "project", Transport: "http", URL: "https://api.githubcopilot.com/mcp/"}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutRepository(domain.Repository{ID: "repo-gh", ProjectID: "platform", Name: "svc", RepoPath: "acme/svc"}); err != nil {
		t.Fatal(err)
	}
	h := &streamHandlers{deps: &deps{repo: st}}
	issue := domain.Issue{
		ProjectID: "platform", Source: "github", Kind: "issue", RepositoryID: "repo-gh",
		ExternalRef: &domain.ExternalRef{Source: "github", ExternalID: "88", ExternalIID: 12, ExternalType: "issue"},
	}
	prompt := h.clarifyMCPInstructions(issue, user, false)
	for _, want := range []string{"github__get_issue", "github__get_issue_comments", "owner=acme", "repo=svc", "issue_number=12"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

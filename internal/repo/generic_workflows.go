package repo

import "github.com/Tencent/LoopForge/internal/domain"

// GenericBackendDeliveryWorkflowID is the stable identifier of the platform
// backend delivery SOP stored in generic_templates.
const GenericBackendDeliveryWorkflowID = "generic-backend-delivery"

// Platform docker assets the generic backend SOP binds to. They are catalog
// rows (not part of the template blob), seeded by
// Bootstrap so new projects run in a container instead of the host-local
// driver.
const (
	GenericBackendDockerfileID = "generic-backend"
	GenericDockerSandboxID     = "generic-docker"
)

// Role identifiers used by the built-in backend delivery template. Cloning a
// generic template copies these roles into the target project.
const (
	GenericAgentRequirement = "backend-requirement"
	GenericAgentDesign      = "backend-design"
	GenericAgentImplement   = "backend-implement"
	GenericAgentReview      = "backend-review"
	GenericAgentTest        = "backend-test"
	GenericAgentMR          = "backend-mr"
	GenericAgentSummary     = "backend-summary"
)

// Requirement keys a generic template may declare. Studio maps them onto live
// checks (personal credentials, project MCP installs) so users learn about the
// dependency before the first run.
const (
	RequirementGitToken         = "git-token"
	RequirementGitHubMCP        = "github-mcp"
	RequirementPersonalMCPToken = "personal-token"
)

// Product and frontend harness templates share the same generic_templates
// table as the backend delivery SOP.
const (
	GenericProductDeliveryWorkflowID  = "generic-product-delivery"
	GenericFrontendDeliveryWorkflowID = "generic-frontend-delivery"
)

const (
	GenericAgentProductPRD        = "product-prd"
	GenericAgentProductDemo       = "product-demo"
	GenericAgentProductHandoff    = "product-handoff"
	GenericAgentProductAcceptance = "product-acceptance"
	GenericAgentProductIssue      = "product-issue"
	GenericAgentProductSummary    = "product-summary"

	GenericAgentFrontendShape   = "frontend-shape"
	GenericAgentFrontendDesign  = "frontend-design"
	GenericAgentFrontendBuild   = "frontend-build"
	GenericAgentFrontendReview  = "frontend-review"
	GenericAgentFrontendVerify  = "frontend-verify"
	GenericAgentFrontendMR      = "frontend-mr"
	GenericAgentFrontendArchive = "frontend-archive"
)

// Template-owned rule ids. These are cloned into the adopting project and
// referenced by the template roles.
const (
	GenericRuleCore           = "backend-core"
	GenericRuleBackendProfile = "backend-backend-profile"
)

// Template-owned skill ids, one per backend delivery stage capability.
const (
	GenericSkillClarifyRequirements = "backend-clarify-requirements"
	GenericSkillBrainstorming       = "backend-brainstorming"
	GenericSkillTechDesign          = "backend-tech-design"
	GenericSkillWritingPlans        = "backend-writing-plans"
	GenericSkillKnowledgeDistill    = "backend-knowledge-distillation"
)

// GenericTemplate bundles a project-independent SOP with the roles it needs.
// Rows live in generic_templates; the engine resolves them through the same
// Workflow/Agent catalog lookups as a custom SOP. The Generic flag on the
// workflow payload is only a marker for listing and clone/edit policy.
type GenericTemplate struct {
	Workflow domain.WorkflowDefinition
	Agents   []domain.AgentDefinition
	Rules    []domain.RuleDefinition
	Skills   []domain.SkillDefinition
	// VerdictGateSeedVersion is retained so stored blobs keep decoding.
	VerdictGateSeedVersion int
	// PlaybookSeedVersion is retained so stored blobs keep decoding.
	PlaybookSeedVersion int
}

package workflow

import _ "embed"

//go:embed defaults/implementer.md
var DefaultImplementerRole string

//go:embed defaults/reviewer.md
var DefaultReviewerRole string

//go:embed defaults/qa.md
var DefaultQARole string

//go:embed defaults/security.md
var DefaultSecurityRole string

// BuiltinRoles contains the default role definitions shipped with the binary.
var BuiltinRoles = map[string]RoleDefinition{
	"implementer": {Name: "Implementer", Description: "Expert software engineer, TDD/BDD", Instructions: DefaultImplementerRole},
	"reviewer":    {Name: "Adversarial Reviewer", Description: "Adversarial code reviewer", Instructions: DefaultReviewerRole},
	"qa":          {Name: "QA Reviewer", Description: "Quality and test coverage reviewer", Instructions: DefaultQARole},
	"security":    {Name: "Security Reviewer", Description: "Security vulnerability auditor", Instructions: DefaultSecurityRole},
}

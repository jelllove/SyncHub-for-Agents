package resource

type Spec struct {
	Key         string
	Provider    string
	ID          string
	Category    Category
	Strategy    Strategy
	Layout      Layout
	Root        string
	Targets     []string
	Include     []string
	Exclude     []string
	Transformer string
	Installer   string
	SharedAs    string
	KeyPatterns []string
}

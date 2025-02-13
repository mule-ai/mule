package types

type Repository struct {
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	CloneURL    string `json:"clone_url"`
	SSHURL      string `json:"ssh_url"`
}

type Issue struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	State     string     `json:"state"`
	HTMLURL   string     `json:"html_url"`
	SourceURL string     `json:"source_url"`
	CreatedAt string     `json:"created_at"`
	UpdatedAt string     `json:"updated_at"`
	Labels    []string   `json:"labels"`
	Comments  []*Comment `json:"comments"`
}

type IssueFilterOptions struct {
	State string `json:"state"`
	Label string `json:"label"`
}

type PullRequestInput struct {
	Title               string `json:"title"`
	Description         string `json:"description"`
	Branch              string `json:"branch"`
	Base                string `json:"base"`
	Draft               bool   `json:"draft"`
	MaintainerCanModify bool   `json:"maintainer_can_modify"`
}

type PullRequest struct {
	Number          int        `json:"number"`
	Title           string     `json:"title"`
	Body            string     `json:"body"`
	State           string     `json:"state"`
	HTMLURL         string     `json:"html_url"`
	Labels          []string   `json:"labels"`
	IssueURL        string     `json:"issue_url"`
	CreatedAt       string     `json:"created_at"`
	UpdatedAt       string     `json:"updated_at"`
	BaseBranch      string     `json:"base_branch"`
	LinkedIssueURLs []string   `json:"linked_issue_urls"`
	Diff            string     `json:"diff"`
	Comments        []*Comment `json:"comments"`
}

type Comment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	DiffHunk  string    `json:"diff_hunk,omitempty"`
	HTMLURL   string    `json:"html_url"`
	URL       string    `json:"url"`
	UserID    int64     `json:"user_id"`
	Reactions Reactions `json:"reactions,omitempty"`
}

type Reaction struct {
	ID      int64  `json:"id,omitempty"`
	Content string `json:"content,omitempty"`
}

type Reactions struct {
	TotalCount int `json:"total_count,omitempty"`
	PlusOne    int `json:"+1,omitempty"`
	MinusOne   int `json:"-1,omitempty"`
	Laugh      int `json:"laugh,omitempty"`
	Confused   int `json:"confused,omitempty"`
	Heart      int `json:"heart,omitempty"`
	Hooray     int `json:"hooray,omitempty"`
	Rocket     int `json:"rocket,omitempty"`
	Eyes       int `json:"eyes,omitempty"`
}

package shell

import (
	"os"
	"strings"

	"kiki-ai-shell/internal/config"
	"kiki-ai-shell/internal/pcp"
	"kiki-ai-shell/internal/rag"
	"kiki-ai-shell/internal/usage"
	"kiki-ai-shell/internal/ui"
)

type State struct {
	User       string
	Files      []string
	Profile    string
	Stream     bool
	LastAnswer string
	Ctx        map[string]string

	CtxSizeTarget   int
	CtxSizeObserved int

	UI    ui.Config
	RAG   *rag.Store
	Usage *usage.Logger
	PCP   *pcp.Client

	NoFence bool
}

// EnsureUsage lazily initializes the usage logger after we know the username.
func (st *State) EnsureUsage(cfg *config.Config) {
	if st == nil || cfg == nil {
		return
	}
	if !cfg.UsageEnabled {
		return
	}
	if st.Usage != nil {
		return
	}
	user := strings.TrimSpace(st.User)
	if user == "" {
		user = strings.TrimSpace(os.Getenv("USER"))
	}
	if user == "" {
		user = "unknown"
	}
	st.Usage = usage.NewLogger(cfg.UsageBaseDir, user)
}

func NewState(cfg *config.Config, uicfg ui.Config) *State {
	user := strings.TrimSpace(os.Getenv("USER"))
	if user == "" {
		user = "unknown"
	}
	st := &State{
		User:            user,
		Files:           []string{},
		Profile:         cfg.Profile,
		Stream:          cfg.Stream,
		Ctx:             map[string]string{},
		CtxSizeTarget:   cfg.CtxSizeTarget,
		CtxSizeObserved: cfg.CtxSizeObserved,
		UI:              uicfg,
		RAG:             rag.New(cfg.RAGEnabled),
		PCP:             pcp.New(cfg.PCPHost),
		NoFence:         cfg.NoFence,
	}
	st.EnsureUsage(cfg)
	return st
}

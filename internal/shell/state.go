package shell

import (
	"kiki-ai-shell/internal/config"
	"kiki-ai-shell/internal/rag"
	"kiki-ai-shell/internal/ui"
)

type State struct {
	Files      []string
	Profile    string
	Stream     bool
	LastAnswer string
	Ctx        map[string]string

	CtxSizeTarget   int
	CtxSizeObserved int

	UI  ui.Config
	RAG *rag.Store
}

func NewState(cfg *config.Config, uicfg ui.Config) *State {
	return &State{
		Files:           []string{},
		Profile:         cfg.Profile,
		Stream:          cfg.Stream,
		Ctx:             map[string]string{},
		CtxSizeTarget:   cfg.CtxSizeTarget,
		CtxSizeObserved: cfg.CtxSizeObserved,
		UI:              uicfg,
		RAG:             rag.New(cfg.RAGEnabled),
	}
}

package shell

import (
	"fmt"
	"os"
)

func promptLine(st *State) string {
	cwd, _ := os.Getwd()
	stream := "off"
	if st.Stream {
		stream = "on"
	}
	rag := "off"
	if st.RAG != nil && st.RAG.Enabled {
		rag = "on"
	}
	return fmt.Sprintf("kiki[%s|stream:%s|files:%d|rag:%s] %s> ", st.Profile, stream, len(st.Files), rag, cwd)
}

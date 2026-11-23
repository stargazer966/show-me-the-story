package story

import (
	"encoding/json"
	"fmt"
	"showmethestory/internal/config"
)

func RestoreWorkflow(path string, state *Progress, cfg *config.Config, settings *ProjectSettings, proof *PostProcessState) error {
	files := []progressFile{}
	for _, item := range []struct {
		id    int
		value any
	}{{progressConfigFile, cfg}, {-1, settings}, {-2, proof}} {
		data, err := json.MarshalIndent(item.value, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal history: %w", err)
		}
		files = append(files, progressFile{Num: item.id, Data: data})
	}
	return saveProgressFiles(path, state, files)
}

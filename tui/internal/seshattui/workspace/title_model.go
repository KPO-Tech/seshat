package workspace

import (
	"os"

	"github.com/KPO-Tech/seshat-runtime/pkg/runtimepath"
	"github.com/KPO-Tech/seshat-runtime/pkg/sdk"
	"github.com/KPO-Tech/seshat-runtime/tui/internal/titlemodel"
)

// localTitleGenerator is the local title model when the user installed it (`seshat title-model install`) and left it
// on, else nil: the engine then asks the chat provider for the title, as it does when the local model fails.
func localTitleGenerator() sdk.TitleGenerator {
	if g := titlemodel.Shared(os.Getenv(runtimepath.EnvRuntimeRoot)); g != nil {
		return g
	}
	return nil
}

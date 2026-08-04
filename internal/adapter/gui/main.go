// Command gui é o ponto de entrada da aplicação desktop do keyward (Wails
// v3). É o único pacote do repositório autorizado a importar
// github.com/wailsapp/wails (ver .golangci.yml, regra no-wails-outside-gui,
// e docs/specs/ssh-config-manager.md, seção 3.3).
//
// O tooling do wails3 (Taskfile.yml/build/) exige que main.go fique no
// mesmo diretório que essa configuração — por isso não há um cmd/gui/
// separado como em cmd/cli e cmd/tui.
package main

import (
	"embed"
	"log"
	"os"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/felipehrs/keyward/core"
	"github.com/felipehrs/keyward/internal/sshagent"
)

//go:embed all:frontend/dist
var assets embed.FS

// As variáveis de ambiente KEYWARD_CONFIG/KEYWARD_KEY_DIR/KEYWARD_METADATA
// seguem a mesma convenção de cmd/tui/main.go — permitem apontar a GUI para
// um ~/.ssh e metadata.json de teste durante o desenvolvimento, sem mexer
// no ambiente real do usuário.
func main() {
	configPath := os.Getenv("KEYWARD_CONFIG")
	keyDir := os.Getenv("KEYWARD_KEY_DIR")
	metadataPath := os.Getenv("KEYWARD_METADATA")

	keySvc := core.NewFileKeyService(keyDir, metadataPath)
	// golang.org/x/crypto/ssh/agent não fala diretamente com named pipe do
	// Windows — precisa do go-winio (internal/sshagent/dial_windows.go). Nas
	// demais plataformas o dial padrão do core (Unix domain socket via
	// SSH_AUTH_SOCK) já basta, sem depender de sshagent.
	if runtime.GOOS == "windows" {
		keySvc.AgentDial = sshagent.Dial
	}
	configSvc := core.NewFileConfigService(configPath)
	backupSvc := core.NewFileBackupService(configPath, keyDir, metadataPath)

	app := application.New(application.Options{
		Name:        "keyward",
		Description: "Gerenciador de configuração e chaves SSH",
		Services: []application.Service{
			application.NewService(newApp(configSvc, keySvc, backupSvc)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "keyward",
		Width:  1024,
		Height: 720,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(18, 18, 20),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

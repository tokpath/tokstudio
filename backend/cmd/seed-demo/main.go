// seed-demo is an explicit test-site administration command, never an API or
// startup hook. It refuses every public host except the designated test site.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/portal"
	"os"
)

func main() {
	confirm := flag.String("confirm", "", "must be TEST_DATA_ONLY")
	host := flag.String("host", "", "must exactly match the configured test host")
	model := flag.String("model-base-url", "", "optional independently deployed test-model URL")
	flag.Parse()
	cfg, err := portal.LoadConfig()
	if err != nil {
		fail(err)
	}
	options := portal.DemoOptions{Confirmation: *confirm, Host: *host, ModelBaseURL: *model, ModelKey: os.Getenv("TOKENHUB_DEMO_MODEL_KEY")}
	if err = portal.ValidateDemoTarget(cfg, options); err != nil {
		fail(err)
	}
	database, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		fail(err)
	}
	app, err := portal.New(context.Background(), cfg, database)
	if err != nil {
		fail(err)
	}
	defer app.Close()
	result, err := app.SeedDemo(context.Background(), options)
	if err != nil {
		fail(err)
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
}
func fail(err error) { fmt.Fprintln(os.Stderr, "Test fixture failed:", err); os.Exit(1) }

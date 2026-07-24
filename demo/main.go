// Package main runs a small local showcase for Iridium Logger.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	iridiumlogs "github.com/asosick/iridium-logger"
	iridiumconfig "github.com/iridiumgo/iridium/config"
	"github.com/iridiumgo/iridium/core/auth/user"
	"github.com/iridiumgo/iridium/core/context/ctxkeys"
	"github.com/iridiumgo/iridium/core/form"
	"github.com/iridiumgo/iridium/core/iridium"
	pagepanel "github.com/iridiumgo/iridium/core/page/panel"
	corepanel "github.com/iridiumgo/iridium/core/panel"
	"github.com/iridiumgo/iridium/network/middleware"
	"github.com/iridiumgo/iridium/network/wrapper"
)

type demoPage struct{}

func main() {
	logPath := seedLog()
	stream, err := iridiumlogs.NewHandler(iridiumlogs.Config{
		Sources: []iridiumlogs.Source{{ID: "demo", Path: logPath}},
	})
	if err != nil {
		log.Fatal(err)
	}
	go appendDemoLines(logPath)

	mux := http.NewServeMux()
	config := iridiumconfig.NewIridiumConfig()
	config.AppName = "Iridium Logger Demo"
	config.AppKey = "aXJpZGl1bS1sb2dnZXItZGVtby1hcHAta2V5"
	config.AppEncryptionKey = "aXJpZGl1bS1sb2dnZXItZGVtby1lbmNyeXB0aW9u"
	config.AppEnv = "development"

	page := pagepanel.NewPanelFormPage[demoPage]("Live logs", "logs", form.NewForm[demoPage](nil).
		HideFooter().
		Schema(
			iridiumlogs.New[demoPage]("DemoLogs", "/demo/logs/stream").
				Source("demo").
				Label("Application output").
				Description("A live stream from the Iridium Logger demo application.").
				Levels("DEBUG", "INFO", "WARN", "ERROR").
				InitialLines(14).
				MaxEntries(5000).
				Height("min(680px, calc(100vh - 250px))"),
		)).
		CustomRoutes(func(scoped wrapper.IMux) {
			stream.RegisterRoute(scoped, "/stream")
		})

	app := iridium.NewIridiumApp(config).
		RegisterPanels(corepanel.NewPanel(mux).
			ID("demo").
			Path("demo").
			PanelItems(page).
			Middleware(demoAuthentication).
			PanelMiddleware(middleware.Compression, middleware.Spa, middleware.ErrorPages))
	app.Initialize(mux)
	// Keep the preview endpoint explicit so EventSource can reach it through
	// the panel prefix when running this standalone showcase.
	mux.Handle("/demo/logs/stream", demoAuthentication(stream))

	fmt.Println("Iridium Logger demo running at http://localhost:8899/demo/logs")
	if err := http.ListenAndServe("localhost:8899", mux); err != nil {
		log.Fatal(err)
	}
}

func seedLog() string {
	file, err := os.CreateTemp("", "iridium-logger-demo-*.log")
	if err != nil {
		log.Fatal(err)
	}
	path := file.Name()
	entries := []string{
		"time=2026-07-24T15:04:00.000Z level=INFO msg=\"demo application started\" component=server",
		"time=2026-07-24T15:04:00.240Z level=DEBUG msg=\"loading configuration\" source=defaults",
		"time=2026-07-24T15:04:00.520Z level=INFO msg=\"connected to database\" latency=18ms",
		"time=2026-07-24T15:04:01.100Z level=WARN msg=\"slow request detected\" duration=842ms path=/reports",
		"time=2026-07-24T15:04:01.480Z level=INFO msg=\"request completed\" status=200",
		"plain text output from a custom logger",
		"time=2026-07-24T15:04:02.020Z level=ERROR msg=\"temporary upstream failure\" retry_in=2s",
		"time=2026-07-24T15:04:02.400Z level=INFO msg=\"retry succeeded\" status=200",
		"time=2026-07-24T15:04:02.900Z level=INFO msg=\"worker queue healthy\" pending=3",
		"time=2026-07-24T15:04:03.220Z level=DEBUG msg=\"cache refreshed\" entries=128",
		"time=2026-07-24T15:04:03.800Z level=INFO msg=\"heartbeat\" uptime=3s",
	}
	for _, entry := range entries {
		_, _ = fmt.Fprintln(file, entry)
	}
	_ = file.Close()
	return path
}

func appendDemoLines(path string) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		log.Printf("open demo log: %v", err)
		return
	}
	defer file.Close()

	levels := []string{"INFO", "INFO", "DEBUG", "INFO", "WARN", "INFO", "ERROR"}
	for index := 0; ; index++ {
		time.Sleep(900 * time.Millisecond)
		level := levels[index%len(levels)]
		message := map[string]string{
			"INFO":  "request completed",
			"DEBUG": "polling worker queue",
			"WARN":  "connection pool nearly full",
			"ERROR": "retrying upstream request",
		}[level]
		line := fmt.Sprintf("time=%s level=%s msg=\"%s\" request_id=req-%04d", time.Now().UTC().Format(time.RFC3339Nano), level, message, index+1)
		if index%5 == 3 {
			line += " \033[36mcolour output enabled\033[0m"
		}
		if _, err := fmt.Fprintln(file, line); err != nil {
			return
		}
		_ = file.Sync()
	}
}

type demoUser struct{}

func (demoUser) GetID() uint                                { return 1 }
func (demoUser) GetIdentifier() string                      { return "demo" }
func (demoUser) GetPasswordHash() string                    { return "" }
func (demoUser) GetByID(uint) (user.IUser, error)           { return demoUser{}, nil }
func (demoUser) GetByIdentifier(string) (user.IUser, error) { return demoUser{}, nil }

func demoAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), ctxkeys.User, user.IUser(demoUser{}))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

var _ user.IUser = demoUser{}

package example

import (
	iridiumlogs "github.com/asosick/iridium-logger"
	"github.com/iridiumgo/iridium/core/form"
	"github.com/iridiumgo/iridium/core/page/panel"
	"github.com/iridiumgo/iridium/network/wrapper"
)

type LogPage struct{}

func NewPage() (*panel.PanelFormPage[LogPage], error) {
	stream, err := iridiumlogs.NewHandler(iridiumlogs.Config{
		Sources: []iridiumlogs.Source{
			{ID: "application", Path: "./storage/logs/application.log"},
		},
	})
	if err != nil {
		return nil, err
	}

	definition := form.NewForm[LogPage](nil).
		HideFooter().
		Schema(
			iridiumlogs.New[LogPage]("ApplicationLogs", "/admin/system/logs/stream").
				Source("application").
				Label("Application logs").
				Description("Live output from the application process.").
				Height("40rem"),
		)

	return panel.NewPanelFormPage[LogPage]("Logs", "/system/logs", definition).
		CustomRoutes(func(mux wrapper.IMux) {
			stream.RegisterRoute(mux, "/stream")
		}), nil
}

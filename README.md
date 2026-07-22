# Iridium Logger

`iridium-logger` is a presentational form component for viewing and following server log files inside an Iridium form. It uses Iridium's form resolution, field wrapper, route middleware, and embedded asset registry.

The browser bundle wraps [TanStack Virtual](https://tanstack.com/virtual/latest) for row virtualization and [Fancy ANSI](https://github.com/kubetail-org/fancy-ansi) for safe ANSI rendering. It does not introduce React, an iframe, a separate application, or a CDN dependency.

## Features

- Live file tailing over Server-Sent Events
- Rotation and copy-truncate support
- Virtualized rows with bounded browser memory
- Logfmt-aware timestamps, levels, and messages
- Level toggles, text filtering, pause buffering, and tail-follow mode
- Safe ANSI colour rendering
- Iridium labels, descriptions, column spans, themes, and page middleware
- Server-owned source IDs; browser requests cannot provide filesystem paths

## Install

```sh
go get github.com/asosick/iridium-logs
```

The plugin embeds its compiled JavaScript and CSS. Consuming applications do not run npm. npm is only required when changing the plugin's frontend source.

## Add it to an Iridium form

Create one handler during application setup, then place the field in a form schema:

```go
stream, err := iridiumlogs.NewHandler(iridiumlogs.Config{
    Sources: []iridiumlogs.Source{
        {ID: "application", Path: "/var/log/my-app/application.log"},
        {ID: "worker", Path: "/var/log/my-app/worker.log"},
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
            Levels("DEBUG", "INFO", "WARN", "ERROR").
            InitialLines(250).
            MaxEntries(5000).
            Height("40rem"),
    )

page := panel.NewPanelFormPage[LogPage]("Logs", "/system/logs", definition).
    CustomRoutes(func(mux wrapper.IMux) {
        stream.RegisterRoute(mux, "/stream")
    })
```

In this example the panel is mounted at `/admin`, the page is `/system/logs`, and its custom route is `/stream`. Therefore the field's browser-visible endpoint is `/admin/system/logs/stream`.

`CustomRoutes` is important: Iridium applies the page and panel middleware to the stream. Logs commonly contain private operational data, so the route should never be mounted outside authenticated authorization middleware.

## Generated application helpers

An application can expose the plugin through the same generated-style helper used by its other Iridium fields:

```go
func FormLogs(name, endpoint string) *iridiumlogs.FormLogViewer[models.User] {
    return iridiumlogs.New[models.User](name, endpoint)
}
```

The resulting field can be used alongside ordinary inputs, cards, tabs, grids, and other form components.

## Field API

```go
iridiumlogs.New[Model](name, endpoint)
    .Source("application")
    .Label("Application logs")
    .Description("Current process output")
    .Height("32rem")
    .Levels("TRACE", "DEBUG", "INFO", "WARN", "ERROR")
    .InitialLines(250)
    .MaxEntries(5000)
    .ColumnSpan(2)
```

Optional behaviour:

- `SourceFn` and `EndpointFn` resolve values from `*ctxForm.Field[T]`.
- `WithoutSearch` removes the text filter.
- `WithoutFollow` starts with automatic tail-following disabled.
- `WithoutANSI` strips ANSI sequences and renders plain text.
- `HiddenFn` controls visibility through the normal form context.

The field is presentational and non-dehydrated. It does not submit log content or change the form's model.

For a live select of files within one server-owned directory, configure a directory source and resolve the filename from form state:

```go
stream, err := iridiumlogs.NewHandler(iridiumlogs.Config{
    Directories: []iridiumlogs.DirectorySource{{ID: "logs", Path: "./storage/logs"}},
})

iridiumlogs.New[Model]("RuntimeLogs", "/admin/logs/stream").
    Directory("logs").
    SourceFn(func(ctx *ctxForm.Field[Model]) string {
        filename, _ := Get[string](ctx, "File")
        return filename
    })
```

Directory sources accept regular direct children only. Traversal, nested paths, missing files, directories, and symbolic links are rejected.

## Stream limits

Defaults:

- 2,000 maximum initial lines
- 256 KiB maximum line size
- 50 simultaneous streams per handler
- 250 ms file polling interval
- 15 second SSE heartbeat
- 5,000 entries retained by each browser field

These are configurable through `Config` and the field builder. Missing files are tolerated, allowing the application to start before a log file is created.

Reverse proxies must pass SSE responses without buffering. The handler sets `X-Accel-Buffering: no`, although proxy-specific configuration may still be required.

## Frontend development

After changing `frontend/log-viewer.js` or `frontend/log-viewer.css`:

```sh
npm install
npm run build
templ generate
go test ./...
```

Commit the regenerated files in `dist`; they are embedded into the Go binary.

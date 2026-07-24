# Iridium Logger

Iridium Logger is an Iridium form field for displaying live server logs inside an Iridium application. It reads configured files on the server, streams new lines to the browser over Server-Sent Events (SSE), and provides a virtualized viewer for filtering and investigating output without loading the entire file into the page.

The plugin is designed for operational dashboards, administration panels, worker monitors, and other authenticated Iridium pages. It is presentational: the viewer does not dehydrate log content into the form model and does not modify the source files.

This plugin was developed by gpt5.6 sol with human oversight.

## Features

- Live log streaming with automatic reconnection.
- Initial tail loading, with a configurable number of recent lines.
- File rotation and copy-truncate detection.
- Partial-line buffering while a line is still being written.
- Maximum line-size protection and UTF-8 sanitization.
- Virtualized rendering through [TanStack Virtual](https://tanstack.com/virtual/latest), keeping browser work bounded for large streams.
- Logfmt-style parsing for `time`, `timestamp`, `level`, `msg`, and `message` fields.
- Readable rendering for structured and unstructured lines.
- ANSI colour rendering through [Fancy ANSI](https://github.com/kubetail-org/fancy-ansi), with an option to display plain text instead.
- Text search and configurable log-level toggles.
- Pause and resume controls. New lines continue buffering while the viewer is paused.
- Tail-follow mode, manual scrolling, and a button to return to the newest output.
- Browser-only clearing of displayed entries. Clearing never deletes or truncates a log file.
- Live line-rate and entry-count indicators.
- Configurable viewer height, initial line count, retained browser entries, and visible levels.
- Dynamic file selection from a server-owned directory without exposing filesystem paths to the browser.
- Embedded JavaScript and CSS assets with HTMX swap cleanup and reinitialization support.
- Iridium field labels, descriptions, visibility, responsive column spans, themes, and page middleware.

The browser bundle is framework-light. It does not require React, an iframe, a separate application, or a CDN dependency.

## Installation

```sh
go get github.com/asosick/iridium-logger
```

The compiled JavaScript and CSS are embedded in the Go package. Applications using the plugin do not need to run npm. npm is only required when developing the plugin's frontend assets.

## Basic usage

Create a handler with server-owned log sources, add the field to an Iridium form, and register the stream route on the page's scoped mux:

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

The route passed to `New` is the browser-visible stream endpoint. In this example, the panel prefix, page path, and custom route produce `/admin/system/logs/stream`.

Register the route through the page or panel mux so Iridium's authentication and authorization middleware protects the stream. Log files often contain credentials, request data, and other private operational information; the handler should not be mounted on an unprotected public route.

## Server configuration

### Fixed sources

`Source` maps a stable browser-visible ID to one server-side file:

```go
iridiumlogs.Source{
	ID:   "application",
	Path: "/var/log/my-app/application.log",
}
```

The browser sends the source ID, never a filesystem path. Source IDs may contain letters, numbers, `.`, `_`, and `-`, and must begin with a letter or number.

### Directory sources

`DirectorySource` allows a form value to select a file from a configured directory:

```go
stream, err := iridiumlogs.NewHandler(iridiumlogs.Config{
	Directories: []iridiumlogs.DirectorySource{
		{ID: "logs", Path: "./storage/logs"},
	},
})
```

Use `Directory` and `SourceFn` on the field to resolve the selected filename from the current form request:

```go
iridiumlogs.New[Model]("RuntimeLogs", "/admin/logs/stream").
	Directory("logs").
	SourceFn(func(ctx *ctxForm.Field[Model]) string {
		filename, _ := Get[string](ctx, "File")
		return filename
	})
```

Directory selection is intentionally narrow. Only regular direct children are accepted. Nested paths, `..`, missing files, directories, and symbolic links are rejected.

### Handler limits

`Config` supports the following operational limits:

| Option | Default | Purpose |
| --- | ---: | --- |
| `PollInterval` | `250ms` | How often an open file is checked for new data. |
| `Heartbeat` | `15s` | SSE keepalive interval. |
| `MaxInitialLines` | `2,000` | Maximum lines a connection may request initially. |
| `MaxLineBytes` | `256 KiB` | Maximum bytes retained from one line. Longer lines are marked as truncated. |
| `MaxConnections` | `50` | Maximum simultaneous live streams for one handler. |

Missing files are tolerated. This allows an application to start before a log file has been created; the viewer will continue polling and can begin displaying output when the file appears.

## Field API

```go
iridiumlogs.New[Model]("ApplicationLogs", "/admin/logs/stream").
	Source("application").
	Label("Application logs").
	Description("Current process output").
	Height("32rem").
	Levels("TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL").
	InitialLines(250).
	MaxEntries(5000).
	ColumnSpan(2)
```

Configuration methods:

- `Source(id)` selects a fixed source configured on the handler.
- `SourceFn(fn)` resolves the source ID from `*ctxForm.Field[T]`.
- `Directory(id)` selects a configured directory source.
- `Endpoint(url)` and `EndpointFn(fn)` set or resolve the stream endpoint.
- `Height(cssSize)` sets the viewer's CSS block size.
- `Levels(levels...)` defines the available level toggles. Values are normalized to uppercase.
- `InitialLines(count)` controls the initial tail size.
- `MaxEntries(count)` bounds the number of entries retained in browser memory. Values are constrained to 100–50,000.
- `WithoutSearch()` removes the text filter.
- `WithoutClear()` removes the browser-only clear control.
- `WithoutFollow()` starts without automatic tail-following.
- `WithoutANSI()` strips ANSI sequences and renders plain text.
- `Label(label)`, `Description(description)`, `ColumnSpan(columns)`, and `HiddenFn(fn)` use the normal Iridium field behaviour.

The viewer does not render a submitted input and never writes log entries to the form model.

## Run the demo

The repository includes a small showcase application in `demo/`. It creates a temporary log file, appends sample output, and serves the viewer at `http://localhost:8899/demo/logs`:

```sh
go run ./demo
```

The demo is intended for local evaluation and screenshots. Its authentication middleware is deliberately permissive and must not be used as an application security example.

## Log rendering

Each incoming line is retained as raw text and as a plain-text version with ANSI sequences removed. Lines containing key/value fields are inspected for common logging fields:

```text
time=2026-07-24T15:04:05Z level=INFO msg="worker started"
```

Recognized timestamps and levels are shown in dedicated columns, and the message is rendered separately. `WARNING` is normalized to `WARN`. Lines that do not match this structure remain readable as full-width unstructured output. Custom JSON, syslog, or other formats are displayed safely as text unless their fields happen to match the supported key/value pattern.

## Stream behaviour

The handler exposes a `GET` SSE endpoint. It sends:

- `line` events containing new log lines.
- `ready` after the initial connection has been established.
- `rotation` when the file is replaced or truncated.
- `problem` when the source is temporarily unavailable.
- SSE comment heartbeats at the configured interval.

The handler rejects unsupported methods, unknown source selections, and connections over `MaxConnections`. It also disables common proxy buffering with `X-Accel-Buffering: no`; reverse proxies may require additional SSE configuration.

## HTMX and embedded assets

The plugin registers its embedded assets when `New` is called. Its frontend initializes on page load and after HTMX swaps, and tears down EventSource connections, timers, event listeners, and virtualization state before swapped content is removed. This prevents stale streams when an Iridium form or page is replaced through HTMX or Alpine Morph.

## Frontend development

After changing `frontend/log-viewer.js` or `frontend/log-viewer.css`, rebuild the embedded assets and generated templ code:

```sh
npm install
npm run build
templ generate
go test ./...
```

The generated files in `dist` and `log_viewer_templ.go` are part of the package and must be committed with source changes.

## License

See [LICENSE.md](LICENSE.md).

package iridiumlogs

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/iridiumgo/iridium/core/asset"
	"github.com/iridiumgo/iridium/core/context/ctxForm"
)

type testModel struct{}

func TestFormLogViewerResolvesAsPresentationalField(t *testing.T) {
	t.Parallel()

	resolvable := New[testModel]("RuntimeLogs", "/admin/system/logs/stream").
		Source("worker").
		Directory("runtime").
		Label("Runtime logs").
		Description("Current worker output").
		Height("40rem").
		Levels("debug", "error").
		InitialLines(75).
		MaxEntries(1200)

	concrete, ok := resolvable.Resolve(&ctxForm.Field[testModel]{}).(*LogViewerConcrete)
	if !ok {
		t.Fatal("expected a LogViewerConcrete")
	}
	if concrete.Endpoint != "/admin/system/logs/stream" || concrete.Source != "worker" || concrete.Directory != "runtime" {
		t.Fatalf("resolved endpoint/source/directory = %q/%q/%q", concrete.Endpoint, concrete.Source, concrete.Directory)
	}
	if concrete.Height != "40rem" || concrete.Levels != "DEBUG,ERROR" {
		t.Fatalf("resolved height/levels = %q/%q", concrete.Height, concrete.Levels)
	}
	if concrete.RetainsState() {
		t.Fatal("presentational viewer must not retain a value in the form checkpoint")
	}
	if concrete.IsDehydrated() {
		t.Fatal("log viewer must not write presentation data to the form model")
	}
	if concrete.Download {
		t.Fatal("download should be disabled by default")
	}

	var rendered bytes.Buffer
	if err := concrete.Component().Render(context.Background(), &rendered); err != nil {
		t.Fatal(err)
	}
	html := rendered.String()
	for _, expected := range []string{
		`data-ir-log-viewer`,
		`data-endpoint="/admin/system/logs/stream"`,
		`data-source="worker"`,
		`data-directory="runtime"`,
		`data-levels="DEBUG,ERROR"`,
		`data-log-clear`,
		`Keep the view pinned to the newest log lines as they arrive`,
		`Runtime logs`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("rendered component does not contain %q", expected)
		}
	}
	if strings.Contains(html, `name="RuntimeLogs"`) {
		t.Fatal("presentational viewer unexpectedly rendered a submitted input")
	}
	if strings.Contains(html, `data-log-download`) {
		t.Fatal("download control was rendered without opt-in")
	}
}

func TestFormLogViewerDownloadIsOptIn(t *testing.T) {
	t.Parallel()

	concrete := New[testModel]("RuntimeLogs", "/logs/stream").
		Download().
		Resolve(&ctxForm.Field[testModel]{}).(*LogViewerConcrete)

	if !concrete.Download {
		t.Fatal("download should be enabled")
	}

	var rendered bytes.Buffer
	if err := concrete.Component().Render(context.Background(), &rendered); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.String(), `data-log-download`) {
		t.Fatal("download control was not rendered after opt-in")
	}
}

func TestFormLogViewerLevelsFnResolvesAndCanHideControls(t *testing.T) {
	t.Parallel()

	var called bool
	concrete := New[testModel]("RuntimeLogs", "/logs/stream").
		LevelsFn(func(*ctxForm.Field[testModel]) []string {
			called = true
			return []string{"debug", "error"}
		}).
		Resolve(&ctxForm.Field[testModel]{}).(*LogViewerConcrete)

	if !called {
		t.Fatal("levels resolver was not called")
	}
	if concrete.Levels != "DEBUG,ERROR" {
		t.Fatalf("resolved levels = %q, want %q", concrete.Levels, "DEBUG,ERROR")
	}

	hidden := New[testModel]("RuntimeLogs", "/logs/stream").
		LevelsFn(func(*ctxForm.Field[testModel]) []string { return nil }).
		Resolve(&ctxForm.Field[testModel]{}).(*LogViewerConcrete)
	if hidden.Levels != "" {
		t.Fatalf("resolved hidden levels = %q, want empty", hidden.Levels)
	}

	var rendered bytes.Buffer
	if err := hidden.Component().Render(context.Background(), &rendered); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.String(), `data-levels=""`) {
		t.Fatal("empty levels resolver result was not rendered")
	}
}

func TestFormLogViewerCanHideClearControl(t *testing.T) {
	t.Parallel()

	concrete := New[testModel]("RuntimeLogs", "/logs/stream").
		WithoutClear().
		Resolve(&ctxForm.Field[testModel]{}).(*LogViewerConcrete)

	if concrete.Clearable {
		t.Fatal("clear control should be disabled")
	}

	var rendered bytes.Buffer
	if err := concrete.Component().Render(context.Background(), &rendered); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered.String(), `data-log-clear`) {
		t.Fatal("clear control was rendered")
	}
}

func TestFormLogViewerRegistersEmbeddedAssets(t *testing.T) {
	New[testModel]("LogsAssets", "/logs/stream")
	if asset.ScriptSrc("viewer", assetPackage) == "" {
		t.Fatal("viewer JavaScript was not registered")
	}
	if asset.StyleHref("viewer", assetPackage) == "" {
		t.Fatal("viewer stylesheet was not registered")
	}
}

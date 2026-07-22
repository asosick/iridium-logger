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
	if concrete.IsDehydrated() {
		t.Fatal("log viewer must not write presentation data to the form model")
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
		`Runtime logs`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("rendered component does not contain %q", expected)
		}
	}
	if strings.Contains(html, `name="RuntimeLogs"`) {
		t.Fatal("presentational viewer unexpectedly rendered a submitted input")
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

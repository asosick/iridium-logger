package iridiumlogs

import (
	"reflect"
	"strings"
	"sync"

	"github.com/a-h/templ"
	"github.com/iridiumgo/iridium/bootstrap"
	"github.com/iridiumgo/iridium/core"
	iridiumcontext "github.com/iridiumgo/iridium/core/context"
	"github.com/iridiumgo/iridium/core/context/ctxForm"
	"github.com/iridiumgo/iridium/core/field"
	"github.com/iridiumgo/iridium/core/rules"
	"github.com/iridiumgo/iridium/core/tools"
)

const logViewerResolver = "iridium-logs::form-log-viewer"

var registeredFieldTypes sync.Map

// FormLogViewer is a presentational Iridium form field for a configured log source.
type FormLogViewer[T core.Model] struct {
	*field.BaseFormFieldResolvable[T]
	EndpointStr     tools.Resolvable[*ctxForm.Field[T], string]
	SourceStr       tools.Resolvable[*ctxForm.Field[T], string]
	DirectoryStr    tools.Resolvable[*ctxForm.Field[T], string]
	HeightStr       tools.Resolvable[*ctxForm.Field[T], string]
	LevelsStr       tools.Resolvable[*ctxForm.Field[T], string]
	InitialLinesInt tools.Resolvable[*ctxForm.Field[T], int]
	MaxEntriesInt   tools.Resolvable[*ctxForm.Field[T], int]
	SearchableBool  tools.Resolvable[*ctxForm.Field[T], bool]
	FollowBool      tools.Resolvable[*ctxForm.Field[T], bool]
	ANSIBool        tools.Resolvable[*ctxForm.Field[T], bool]
}

// LogViewerConcrete is the request-resolved form field rendered by Iridium.
type LogViewerConcrete struct {
	*field.BaseFormFieldConcrete
	Endpoint     string
	Source       string
	Directory    string
	Height       string
	Levels       string
	InitialLines int
	MaxEntries   int
	Searchable   bool
	Follow       bool
	ANSI         bool
}

// New creates an Iridium form field. endpoint is the authenticated Handler URL.
func New[T core.Model](name, endpoint string) *FormLogViewer[T] {
	registerAssets()
	registerFieldResolver[T]()
	return &FormLogViewer[T]{
		BaseFormFieldResolvable: field.NewBaseFormFieldResolvable[T](name),
		EndpointStr:             staticValue[*ctxForm.Field[T]](endpoint),
		SourceStr:               staticValue[*ctxForm.Field[T]]("application"),
		DirectoryStr:            staticValue[*ctxForm.Field[T]](""),
		HeightStr:               staticValue[*ctxForm.Field[T]]("32rem"),
		LevelsStr:               staticValue[*ctxForm.Field[T]]("TRACE,DEBUG,INFO,WARN,ERROR,FATAL,PANIC"),
		InitialLinesInt:         staticValue[*ctxForm.Field[T]](defaultInitialLines),
		MaxEntriesInt:           staticValue[*ctxForm.Field[T]](5_000),
		SearchableBool:          staticValue[*ctxForm.Field[T]](true),
		FollowBool:              staticValue[*ctxForm.Field[T]](true),
		ANSIBool:                staticValue[*ctxForm.Field[T]](true),
	}
}

// Source selects one of the IDs configured on Handler.
func (r *FormLogViewer[T]) Source(source string) *FormLogViewer[T] {
	tools.SetFieldValue(&r.SourceStr, source)
	return r
}

// SourceFn resolves the source from the current form request.
func (r *FormLogViewer[T]) SourceFn(fn func(*ctxForm.Field[T]) string) *FormLogViewer[T] {
	tools.SetFieldValue(&r.SourceStr, fn)
	return r
}

// Directory selects a configured server-owned directory for dynamic filenames.
func (r *FormLogViewer[T]) Directory(directory string) *FormLogViewer[T] {
	tools.SetFieldValue(&r.DirectoryStr, directory)
	return r
}

// Endpoint overrides the stream URL used by the browser.
func (r *FormLogViewer[T]) Endpoint(endpoint string) *FormLogViewer[T] {
	tools.SetFieldValue(&r.EndpointStr, endpoint)
	return r
}

// EndpointFn resolves the stream URL from the current form request.
func (r *FormLogViewer[T]) EndpointFn(fn func(*ctxForm.Field[T]) string) *FormLogViewer[T] {
	tools.SetFieldValue(&r.EndpointStr, fn)
	return r
}

// Height sets the viewer's CSS block size.
func (r *FormLogViewer[T]) Height(height string) *FormLogViewer[T] {
	tools.SetFieldValue(&r.HeightStr, height)
	return r
}

// Levels defines the visible level toggles. Values are normalized in the browser.
func (r *FormLogViewer[T]) Levels(levels ...string) *FormLogViewer[T] {
	normalized := make([]string, 0, len(levels))
	for _, level := range levels {
		level = strings.TrimSpace(strings.ToUpper(level))
		if level != "" {
			normalized = append(normalized, level)
		}
	}
	tools.SetFieldValue(&r.LevelsStr, strings.Join(normalized, ","))
	return r
}

// InitialLines controls how many existing lines are requested on connection.
func (r *FormLogViewer[T]) InitialLines(lines int) *FormLogViewer[T] {
	tools.SetFieldValue(&r.InitialLinesInt, max(lines, 0))
	return r
}

// MaxEntries bounds browser memory for this field.
func (r *FormLogViewer[T]) MaxEntries(entries int) *FormLogViewer[T] {
	tools.SetFieldValue(&r.MaxEntriesInt, min(max(entries, 100), 50_000))
	return r
}

// WithoutSearch hides the text filter.
func (r *FormLogViewer[T]) WithoutSearch() *FormLogViewer[T] {
	tools.SetFieldValue(&r.SearchableBool, false)
	return r
}

// WithoutFollow starts the viewer without automatically following new lines.
func (r *FormLogViewer[T]) WithoutFollow() *FormLogViewer[T] {
	tools.SetFieldValue(&r.FollowBool, false)
	return r
}

// WithoutANSI disables ANSI formatting and displays stripped plain text.
func (r *FormLogViewer[T]) WithoutANSI() *FormLogViewer[T] {
	tools.SetFieldValue(&r.ANSIBool, false)
	return r
}

// Label sets the standard Iridium field label.
func (r *FormLogViewer[T]) Label(label string) *FormLogViewer[T] {
	tools.SetFieldValue(&r.LabelStr, label)
	return r
}

// Description sets the standard Iridium field description.
func (r *FormLogViewer[T]) Description(description string) *FormLogViewer[T] {
	tools.SetFieldValue(&r.DescriptionStr, description)
	return r
}

// ColumnSpan sets the field width at the smallest breakpoint.
func (r *FormLogViewer[T]) ColumnSpan(columns int) *FormLogViewer[T] {
	tools.SetFieldValue(&r.ColumnSpanMap, map[string]int{"xs": columns})
	return r
}

// HiddenFn conditionally removes the viewer from the form.
func (r *FormLogViewer[T]) HiddenFn(fn func(*ctxForm.Field[T]) bool) *FormLogViewer[T] {
	tools.SetFieldValue(&r.HiddenBool, fn)
	return r
}

func (r *FormLogViewer[T]) Copy() field.IFormFieldResolvable[T] {
	return &FormLogViewer[T]{
		BaseFormFieldResolvable: r.BaseFormFieldResolvable.Copy(),
		EndpointStr:             r.EndpointStr,
		SourceStr:               r.SourceStr,
		DirectoryStr:            r.DirectoryStr,
		HeightStr:               r.HeightStr,
		LevelsStr:               r.LevelsStr,
		InitialLinesInt:         r.InitialLinesInt,
		MaxEntriesInt:           r.MaxEntriesInt,
		SearchableBool:          r.SearchableBool,
		FollowBool:              r.FollowBool,
		ANSIBool:                r.ANSIBool,
	}
}

func (r *FormLogViewer[T]) Resolve(ctx *ctxForm.Field[T]) field.IField {
	return bootstrap.MustResolveConcreteWithContext[*FormLogViewer[T], *LogViewerConcrete, *ctxForm.Field[T]](
		ctx, logViewerResolver, r,
	)
}

func (c *LogViewerConcrete) Component() templ.Component {
	if c.HiddenBool {
		return templ.NopComponent
	}
	return logViewer(c)
}

func (c *LogViewerConcrete) GenerateImplicitRules() []rules.IRule {
	return nil
}

func registerFieldResolver[T core.Model]() {
	typeKey := reflect.TypeFor[*FormLogViewer[T]]()
	if _, loaded := registeredFieldTypes.LoadOrStore(typeKey, struct{}{}); loaded {
		return
	}
	bootstrap.RegisterContextResolver[*FormLogViewer[T], *LogViewerConcrete, *ctxForm.Field[T]](
		logViewerResolver,
		func(ctx *ctxForm.Field[T], value *FormLogViewer[T]) (*LogViewerConcrete, error) {
			return &LogViewerConcrete{
				BaseFormFieldConcrete: value.BaseFormFieldResolvable.Resolve(ctx),
				Endpoint:              value.EndpointStr.Get(ctx),
				Source:                value.SourceStr.Get(ctx),
				Directory:             value.DirectoryStr.Get(ctx),
				Height:                value.HeightStr.Get(ctx),
				Levels:                value.LevelsStr.Get(ctx),
				InitialLines:          max(value.InitialLinesInt.Get(ctx), 0),
				MaxEntries:            min(max(value.MaxEntriesInt.Get(ctx), 100), 50_000),
				Searchable:            value.SearchableBool.Get(ctx),
				Follow:                value.FollowBool.Get(ctx),
				ANSI:                  value.ANSIBool.Get(ctx),
			}, nil
		},
	)
}

func staticValue[Context iridiumcontext.CustomContext, Value any](value Value) tools.Resolvable[Context, Value] {
	return tools.Resolvable[Context, Value]{IsStatic: true, StaticValue: value}
}

// Tideland Go Library - Etc
//
// Copyright (C) 2016-2017 Frank Mueller / Tideland / Oldenburg / Germany
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

package etc

//--------------------
// IMPORTS
//--------------------

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/tideland/golib/collections"
	"github.com/tideland/golib/errors"
	"github.com/tideland/golib/sml"
	"github.com/tideland/golib/stringex"
)

//--------------------
// GLOBAL
//--------------------

// key is to address a configuration inside a context.
type key int

var (
	etcKey    key = 1
	etcRoot       = []string{"etc"}
	defaulter     = stringex.NewDefaulter("etc", false)
)

//--------------------
// VALUE
//--------------------

// value helps to use the stringex.Defaulter.
type value struct {
	path    []string
	changer collections.KeyStringValueChanger
}

// Value retrieves the value or an error. It implements
// the Valuer interface.
func (v *value) Value() (string, error) {
	sv, err := v.changer.Value()
	if err != nil {
		return "", errors.New(ErrInvalidPath, errorMessages, pathToString(v.path))
	}
	return sv, nil
}

//--------------------
// ETC
//--------------------

// Application is used to apply values to a configurtation.
type Application map[string]string

// Etc contains the read etc configuration and provides access to
// it. ThetcRoot node "etc" is automatically preceded to the path.
// The node name have to consist out of 'a' to 'z', '0' to '9', and
// '-'. The nodes of a path are separated by '/'.
type Etc interface {
	fmt.Stringer

	// HasPath checks if the configurations has the defined path
	// regardles of the value or possible subconfigurations.
	HasPath(path string) bool

	// ValueAsString retrieves the string value at a given path. If it
	// doesn't exist the default value dv is returned.
	ValueAsString(path, dv string) string

	// ValueAsBool retrieves the bool value at a given path. If it
	// doesn't exist the default value dv is returned.
	ValueAsBool(path string, dv bool) bool

	// ValueAsInt retrieves the int value at a given path. If it
	// doesn't exist the default value dv is returned.
	ValueAsInt(path string, dv int) int

	// ValueAsFloat64 retrieves the float64 value at a given path. If it
	// doesn't exist the default value dv is returned.
	ValueAsFloat64(path string, dv float64) float64

	// ValueAsTime retrieves the string value at a given path and
	// interprets it as time with the passed format. If it
	// doesn't exist the default value dv is returned.
	ValueAsTime(path, layout string, dv time.Time) time.Time

	// ValueAsDuration retrieves the duration value at a given path.
	// If it doesn't exist the default value dv is returned.
	ValueAsDuration(path string, dv time.Duration) time.Duration

	// Spit produces a subconfiguration below the passed path.
	// The last path part will be the new root, all values below
	// that configuration node will be below the created root.
	// In case of an invalid path an empty configuration will
	// be returned as default.
	Split(path string) (Etc, error)

	// Dunp creates a map of paths and their values to apply
	// them into other configurations.
	Dump() (Application, error)

	// Apply creates a new configuration by adding of overwriting
	// the passed values. The keys of the map have to be slash
	// separated configuration paths without the leading "etc".
	Apply(appl Application) (Etc, error)

	// Write writes the configuration as SML to the passed target.
	// If prettyPrint is true the written SML is indented and has
	// linebreaks.
	Write(target io.Writer, prettyPrint bool) error
}

// etc implements the Etc interface.
type etc struct {
	values collections.KeyStringValueTree
}

// Read reads the SML source of the configuration from a
// reader, parses it, and returns the etc instance.
func Read(source io.Reader) (Etc, error) {
	builder := sml.NewKeyStringValueTreeBuilder()
	err := sml.ReadSML(source, builder)
	if err != nil {
		return nil, errors.Annotate(err, ErrIllegalSourceFormat, errorMessages)
	}
	values, err := builder.Tree()
	if err != nil {
		return nil, errors.Annotate(err, ErrIllegalSourceFormat, errorMessages)
	}
	if err = values.At("etc").Error(); err != nil {
		return nil, errors.Annotate(err, ErrIllegalSourceFormat, errorMessages)
	}
	cfg := &etc{
		values: values,
	}
	if err = cfg.postProcess(); err != nil {
		return nil, errors.Annotate(err, ErrCannotPostProcess, errorMessages)
	}
	return cfg, nil
}

// ReadString reads the SML source of the configuration from a
// string, parses it, and returns the etc instance.
func ReadString(source string) (Etc, error) {
	return Read(strings.NewReader(source))
}

// ReadFile reads the SML source of a configuration file,
// parses it, and returns the etc instance.
func ReadFile(filename string) (Etc, error) {
	source, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.Annotate(err, ErrCannotReadFile, errorMessages, filename)
	}
	return ReadString(string(source))
}

// HasPath implements the Etc interface.
func (e *etc) HasPath(path string) bool {
	fullPath := makeFullPath(path)
	changer := e.values.At(fullPath...)
	return changer.Error() == nil
}

// ValueAsString implements the Etc interface.
func (e *etc) ValueAsString(path, dv string) string {
	value := e.valueAt(path)
	return defaulter.AsString(value, dv)
}

// ValueAsBool implements the Etc interface.
func (e *etc) ValueAsBool(path string, dv bool) bool {
	value := e.valueAt(path)
	return defaulter.AsBool(value, dv)
}

// ValueAsInt implements the Etc interface.
func (e *etc) ValueAsInt(path string, dv int) int {
	value := e.valueAt(path)
	return defaulter.AsInt(value, dv)
}

// ValueAsFloat64 implements the Etc interface.
func (e *etc) ValueAsFloat64(path string, dv float64) float64 {
	value := e.valueAt(path)
	return defaulter.AsFloat64(value, dv)
}

// ValueAsTime implements the Etc interface.
func (e *etc) ValueAsTime(path, format string, dv time.Time) time.Time {
	value := e.valueAt(path)
	return defaulter.AsTime(value, format, dv)
}

// ValueAsDuration implements the Etc interface.
func (e *etc) ValueAsDuration(path string, dv time.Duration) time.Duration {
	value := e.valueAt(path)
	return defaulter.AsDuration(value, dv)
}

// Split implements the Etc interface.
func (e *etc) Split(path string) (Etc, error) {
	if !e.HasPath(path) {
		// Path not found, return empty configuration.
		return ReadString("{etc}")
	}
	fullPath := makeFullPath(path)
	values, err := e.values.CopyAt(fullPath...)
	if err != nil {
		return nil, errors.Annotate(err, ErrCannotSplit, errorMessages)
	}
	values.At(fullPath[len(fullPath)-1:]...).SetKey("etc")
	es := &etc{
		values: values,
	}
	return es, nil
}

// Dump implements the Etc interface.
func (e *etc) Dump() (Application, error) {
	appl := Application{}
	err := e.values.DoAllDeep(func(ks []string, v string) error {
		if len(ks) == 1 {
			// Continue on root element.
			return nil
		}
		path := strings.Join(ks[1:], "/")
		appl[path] = v
		return nil
	})
	if err != nil {
		return nil, err
	}
	return appl, nil
}

// Apply implements the Etc interface.
func (e *etc) Apply(appl Application) (Etc, error) {
	ec := &etc{
		values: e.values.Copy(),
	}
	for path, value := range appl {
		fullPath := makeFullPath(path)
		_, err := ec.values.Create(fullPath...).SetValue(value)
		if err != nil {
			return nil, errors.Annotate(err, ErrCannotApply, errorMessages)
		}
	}
	return ec, nil
}

// Write implements the Etc interface.
func (e *etc) Write(target io.Writer, prettyPrint bool) error {
	// Build the nodes tree.
	builder := sml.NewNodeBuilder()
	depth := 0
	err := e.values.DoAllDeep(func(ks []string, v string) error {
		doDepth := len(ks)
		tag := ks[doDepth-1]
		for i := depth; i > doDepth; i-- {
			builder.EndTagNode()
		}
		switch {
		case doDepth > depth:
			builder.BeginTagNode(tag)
			builder.TextNode(v)
			depth = doDepth
		case doDepth == depth:
			builder.EndTagNode()
			builder.BeginTagNode(tag)
			builder.TextNode(v)
		case doDepth < depth:
			builder.EndTagNode()
			builder.BeginTagNode(tag)
			builder.TextNode(v)
			depth = doDepth
		}
		return nil
	})
	if err != nil {
		return err
	}
	for i := depth; i > 0; i-- {
		builder.EndTagNode()
	}
	root, err := builder.Root()
	if err != nil {
		return err
	}
	// Now write the node structure.
	wp := sml.NewStandardSMLWriter()
	wctx := sml.NewWriterContext(wp, target, prettyPrint, "   ")
	return sml.WriteSML(root, wctx)
}

// Apply implements the Stringer interface.
func (e *etc) String() string {
	return fmt.Sprintf("%v", e.values)
}

// valueAt retrieves and encapsulates the value
// at a given path.
func (e *etc) valueAt(path string) *value {
	fullPath := makeFullPath(path)
	changer := e.values.At(fullPath...)
	return &value{fullPath, changer}
}

// postProcess replaces templates formated [path||default]
// with values found at that path or the default.
func (e *etc) postProcess() error {
	re := regexp.MustCompile("\\[.+(||.+)\\]")
	// Find all entries with template.
	changers := e.values.FindAll(func(k, v string) (bool, error) {
		return re.MatchString(v), nil
	})
	// Change the template.
	for _, changer := range changers {
		value, err := changer.Value()
		if err != nil {
			return err
		}
		found := re.FindString(value)
		// Look for default value.
		sourceDefault := strings.SplitN(found[1:len(found)-1], "||", 2)
		defaultValue := found
		if len(sourceDefault) > 1 {
			defaultValue = sourceDefault[1]
		}
		// Check if source is environment variable or path.
		substitute := ""
		if strings.HasPrefix(sourceDefault[0], "$") {
			if envValue, ok := os.LookupEnv(sourceDefault[0][1:]); ok {
				substitute = envValue
			} else {
				substitute = defaultValue
			}
		} else {
			substitute = e.ValueAsString(sourceDefault[0], defaultValue)
		}
		replaced := strings.Replace(value, found, substitute, -1)
		_, err = changer.SetValue(replaced)
		if err != nil {
			return err
		}
	}
	return nil
}

//--------------------
// CONTEXT
//--------------------

// NewContext returns a new context that carries a configuration.
func NewContext(ctx context.Context, cfg Etc) context.Context {
	return context.WithValue(ctx, etcKey, cfg)
}

// FromContext returns the configuration stored in ctx, if any.
func FromContext(ctx context.Context) (Etc, bool) {
	cfg, ok := ctx.Value(etcKey).(Etc)
	return cfg, ok
}

//--------------------
// HELPERS
//--------------------

// makeFullPath creates the full path out of a string.
func makeFullPath(path string) []string {
	parts := stringex.SplitMap(path, "/", func(p string) (string, bool) {
		if p == "" {
			return "", false
		}
		return strings.ToLower(p), true
	})
	return append(etcRoot, parts...)
}

// pathToString returns the path in a filesystem like notation.
func pathToString(path []string) string {
	return "/" + strings.Join(path, "/")
}

// EOF

package gen

import (
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	logging "github.com/ipfs/go-log"
	"golang.org/x/xerrors"
)

var log = logging.Logger("encoding.gen")

// TypeEncodingGenerator represents types that generate encoding/decoding impl for other types.
type TypeEncodingGenerator interface {
	// WriteImports outputs the imports.
	WriteImports(w io.Writer) error
	// WriteInit outputs the init for the file.
	WriteInit(w io.Writer, tis []TypeInfo) error
	// WriteEncodingForType outputs the encoding for the given type.
	WriteEncodingForType(w io.Writer, ti TypeInfo) error
}

// TypeInfo contains information about the a runtime type.
type TypeInfo struct {
	Name    string
	Type    reflect.Type
	Fields  []FieldInfo
	Options TypeOpt
}

// FieldInfo contains information about a field in a runtime type.
type FieldInfo struct {
	Name    string
	Pointer bool
	Type    reflect.Type
	Pkg     string

	IterLabel string
}

// Mode represents the encoding/decoding strategy mode.
//
// This can help understand things like "new types"
type Mode uint32

const (
	defaultMode Mode = iota
	// NewTypeStructMode tells the generator to treat the structure as a "new type".
	//
	// It will expect a `type Foo struct { anyName T }` and encode it as a type `T`.
	NewTypeStructMode
)

// TypeOpt allows the caller to configure how the encoding/decoding
// for a specific type will be generated.
type TypeOpt struct {
	Value          interface{}
	Mode           Mode
	GenerateEncode bool
	GenerateDecode bool
	RegisterType   bool
}

// WriteToFile creates a file and outputs all the generated encoding/decoding methods for the given types.
//
// The `types` argument can receive an instance to a type or a `TypeOpt`.
func WriteToFile(fname string, generator TypeEncodingGenerator, pkg string, types ...interface{}) error {
	log.Infof("Writing to file %s", fname)

	// create dir if needed
	err := os.MkdirAll(filepath.Dir(fname), os.ModePerm)
	if err != nil {
		return xerrors.Errorf("failed to create dir: %w", err)
	}

	// create file
	fi, err := os.Create(fname)
	if err != nil {
		return xerrors.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = fi.Close() }()

	if err := writePackageHeader(fi, pkg); err != nil {
		return xerrors.Errorf("failed to write header: %w", err)
	}

	if err := generator.WriteImports(fi); err != nil {
		return xerrors.Errorf("failed to write generator imports: %w", err)
	}

	tis := []TypeInfo{}

	for _, t := range types {
		ti, err := parseTypeInfo(pkg, t)
		if err != nil {
			return xerrors.Errorf("failed to parse type info: %w", err)
		}

		tis = append(tis, ti)
	}

	if err := generator.WriteInit(fi, tis); err != nil {
		return xerrors.Errorf("failed to write generator imports: %w", err)
	}

	for _, ti := range tis {
		if err := generator.WriteEncodingForType(fi, ti); err != nil {
			return xerrors.Errorf("failed to generate encoding methods: %w", err)
		}
	}

	return nil
}

func writePackageHeader(w io.Writer, pkg string) error {
	data := struct {
		Package string
	}{pkg}

	return doTemplate(w, data, `package {{ .Package }}
	
/* 
DO NOT EDIT THIS FILE BY HAND

This file was generated by "github.com/filecoin-project/go-filecoin/encoding/gen" 
*/
`)
}

func parseTypeInfo(pkg string, i interface{}) (TypeInfo, error) {
	var t reflect.Type
	var opt TypeOpt
	if opt, ok := i.(TypeOpt); ok {
		log.Debugf("Parsing type %T", opt.Value)
		t = reflect.TypeOf(opt.Value)
	} else {
		log.Debugf("Parsing type %T", t)
		t = reflect.TypeOf(i)
		opt = TypeOpt{Value: i}
	}

	out := TypeInfo{
		Name:    t.Name(),
		Options: opt,
	}

	switch t.Kind() {
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !nameIsExported(f.Name) {
				continue
			}

			ft := f.Type
			var pointer bool
			if ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
				pointer = true
			}

			out.Fields = append(out.Fields, FieldInfo{
				Name:    "t." + f.Name,
				Pointer: pointer,
				Type:    ft,
				Pkg:     pkg,
			})
		}
	default:
		return out, fmt.Errorf("unsupported type for generating encoding code: %T", opt.Value)
	}

	return out, nil
}

func doTemplate(w io.Writer, info interface{}, templ string) error {
	t := template.Must(template.New("").
		Funcs(template.FuncMap{}).Parse(templ))

	return t.Execute(w, info)
}

func nameIsExported(name string) bool {
	return strings.ToUpper(name[0:1]) == name[0:1]
}

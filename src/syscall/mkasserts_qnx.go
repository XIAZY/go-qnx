// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// mkasserts_qnx checks the struct layouts that cgo -godefs generated for QNX
// against the C compiler, and writes a C header of compile-time assertions so
// the check can be repeated by any C compiler for the target. (A header, not
// a .c file: the go command rejects .c files in a package that does not use
// cgo.)
//
//	go run mkasserts_qnx.go -types types_qnx.go -ztypes ztypes_qnx_386.go \
//		-o zasserts_qnx_386.h -- <C compiler flags>
//
// $CC must be a C compiler for i386 QNX 6.5 with its headers, such as
// the SDP's qcc.
//
// The C flags must include -D_FILE_OFFSET_BITS=64. It cannot be a #define
// in the types_qnx.go preamble: cgo's prolog includes <stddef.h> first, and
// the QNX headers fix the width of off_t, ino_t and blkcnt_t on first
// inclusion, so a later #define silently yields the 32-bit struct stat (60
// bytes in Go against 72 in C, which this program would reject).
//
// For every "type T C.x" in -types it compiles the cgo preamble with $CC -g,
// reads the layout of x from DWARF, and compares size, field offsets and field
// sizes with the Go type T as laid out by the gc compiler for GOARCH=386. Any
// difference is an error. It also finds 8-byte fields that Go only aligns to
// 4 bytes, and adds a comment to those fields in -ztypes, because they must
// never be passed to sync/atomic.
package main

import (
	"bytes"
	"debug/dwarf"
	"debug/elf"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

var (
	typesFile  = flag.String("types", "types_qnx.go", "cgo -godefs input")
	ztypesFile = flag.String("ztypes", "", "cgo -godefs output to check and annotate")
	outFile    = flag.String("o", "", "C assertion file to write")
	goarch     = flag.String("goarch", "386", "GOARCH whose layout rules apply")
)

// A mapping is one "type Name C.ctype" declaration.
type mapping struct {
	goName string
	ctype  string // C spelling: "struct stat", "sigset_t"
}

type member struct {
	name   string
	offset int64
	size   int64
}

type clayout struct {
	size     int64
	align    int64 // 0 if DWARF does not say
	members  []member
	isStruct bool
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("mkasserts_qnx: ")
	flag.Parse()
	if *ztypesFile == "" || *outFile == "" {
		log.Fatal("need -ztypes and -o")
	}
	cflags := flag.Args()

	preamble, maps := readTypes(*typesFile)
	golay := goLayouts(*ztypesFile)
	clay := cLayouts(preamble, maps, cflags)

	var asserts bytes.Buffer
	var errs []string
	annot := map[string]map[string]string{} // Go type -> field -> comment
	n := 0
	chk := func(cond, what string) {
		n++
		fmt.Fprintf(&asserts, "typedef char chk_%d[(%s) ? 1 : -1]; /* %s */\n", n, cond, what)
	}

	for _, m := range maps {
		g, ok := golay[m.goName]
		if !ok {
			continue // a constant or a type godefs did not emit
		}
		c := clay[m.goName]
		if g.size != c.size {
			errs = append(errs, fmt.Sprintf("%s: Go size %d, C sizeof(%s) %d", m.goName, g.size, m.ctype, c.size))
		}
		chk(fmt.Sprintf("sizeof(%s) == %d", m.ctype, g.size), m.goName)
		if !c.isStruct || g.st == nil {
			continue
		}
		cm := c.members
		for i := 0; i < g.st.NumFields(); i++ {
			f := g.st.Field(i)
			if strings.HasPrefix(f.Name(), "Pad_cgo_") || f.Name() == "_" {
				continue
			}
			if len(cm) == 0 {
				errs = append(errs, fmt.Sprintf("%s.%s: no matching C member", m.goName, f.Name()))
				break
			}
			mem := cm[0]
			cm = cm[1:]
			if !sameName(f.Name(), mem.name) {
				errs = append(errs, fmt.Sprintf("%s.%s: next C member is %s", m.goName, f.Name(), mem.name))
				break
			}
			goff, gsz := g.offsets[i], g.sizes[i]
			if goff != mem.offset || gsz != mem.size {
				errs = append(errs, fmt.Sprintf("%s.%s: Go offset %d size %d, C %s.%s offset %d size %d",
					m.goName, f.Name(), goff, gsz, m.ctype, mem.name, mem.offset, mem.size))
			}
			chk(fmt.Sprintf("offsetof(%s, %s) == %d", m.ctype, mem.name, goff), m.goName+"."+f.Name())
			chk(fmt.Sprintf("sizeof(((%s *)0)->%s) == %d", m.ctype, mem.name, gsz), m.goName+"."+f.Name())
			if gsz == 8 && is64(f.Type()) && goff%8 != 0 {
				setAnnot(annot, m.goName, f.Name(), "not 8-byte aligned: never use with sync/atomic")
			}
		}
		if len(cm) > 0 {
			var names []string
			for _, x := range cm {
				names = append(names, x.name)
			}
			errs = append(errs, fmt.Sprintf("%s: C members with no Go field: %s", m.goName, strings.Join(names, ", ")))
		}
		if c.align > g.align {
			// C places this struct on a stricter boundary than Go will.
			for i := 0; i < g.st.NumFields(); i++ {
				f := g.st.Field(i)
				if g.sizes[i] == 8 && is64(f.Type()) {
					setAnnot(annot, m.goName, f.Name(), fmt.Sprintf(
						"C aligns %s to %d bytes, Go to %d: never use with sync/atomic", m.ctype, c.align, g.align))
				}
			}
		}
	}
	if len(errs) > 0 {
		for _, e := range errs {
			log.Print(e)
		}
		log.Fatalf("%d layout mismatches between %s and $CC", len(errs), *ztypesFile)
	}

	var out bytes.Buffer
	fmt.Fprintf(&out, "// Code generated by mkasserts_qnx.go; DO NOT EDIT.\n\n")
	fmt.Fprintf(&out, "// Compile-time checks that the struct layouts in %s match the C\n", filepath.Base(*ztypesFile))
	fmt.Fprintf(&out, "// headers. Any C compiler for the target must accept this file:\n")
	fmt.Fprintf(&out, "//\t$CC %s -fsyntax-only %s\n\n", strings.Join(cflags, " "), filepath.Base(*outFile))
	out.WriteString("#include <stddef.h>\n")
	out.WriteString(preamble)
	out.WriteString("\n")
	out.Write(asserts.Bytes())
	if err := os.WriteFile(*outFile, out.Bytes(), 0o666); err != nil {
		log.Fatal(err)
	}
	// Prove the file itself compiles.
	run(append(append([]string{}, cflags...), "-fsyntax-only", *outFile)...)

	annotate(*ztypesFile, annot)
	fmt.Fprintf(os.Stderr, "mkasserts_qnx: %d types, %d assertions, %d annotated fields\n", len(maps), n, countAnnot(annot))
}

func setAnnot(a map[string]map[string]string, t, f, c string) {
	if a[t] == nil {
		a[t] = map[string]string{}
	}
	if _, ok := a[t][f]; !ok {
		a[t][f] = c
	}
}

func countAnnot(a map[string]map[string]string) int {
	n := 0
	for _, m := range a {
		n += len(m)
	}
	return n
}

// sameName reports whether goName is what cgo -godefs makes of C member cname:
// the common prefix up to the first '_' may be dropped ("st_ino" -> "Ino"),
// the first letter is upper-cased, and names starting with '_' get an "X"
// ("__bits" -> "X__bits").
func sameName(goName, cname string) bool {
	if strings.EqualFold(goName, cname) || strings.HasPrefix(cname, "_") && goName == "X"+cname {
		return true
	}
	if i := strings.Index(cname, "_"); i >= 0 && strings.EqualFold(goName, cname[i+1:]) {
		return true
	}
	return false
}

func is64(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	if !ok {
		return false
	}
	switch b.Kind() {
	case types.Int64, types.Uint64, types.Float64, types.Complex64:
		return true
	}
	return false
}

// readTypes returns the cgo preamble of the -types file and its C type mappings.
func readTypes(file string) (string, []mapping) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}
	var preamble string
	for _, imp := range f.Imports {
		if imp.Path.Value == `"C"` {
			d := findGenDecl(f, imp)
			if d != nil && d.Doc != nil {
				preamble = d.Doc.Text()
			}
		}
	}
	if preamble == "" {
		log.Fatalf("%s: no cgo preamble", file)
	}
	var maps []mapping
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, s := range gd.Specs {
			ts := s.(*ast.TypeSpec)
			sel, ok := ts.Type.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "C" {
				continue
			}
			maps = append(maps, mapping{ts.Name.Name, cSpelling(sel.Sel.Name)})
		}
	}
	return preamble, maps
}

func findGenDecl(f *ast.File, imp *ast.ImportSpec) *ast.GenDecl {
	for _, d := range f.Decls {
		if gd, ok := d.(*ast.GenDecl); ok {
			for _, s := range gd.Specs {
				if s == imp {
					return gd
				}
			}
		}
	}
	return nil
}

func cSpelling(sel string) string {
	for _, p := range []string{"struct_", "union_", "enum_"} {
		if strings.HasPrefix(sel, p) {
			return strings.TrimSuffix(p, "_") + " " + sel[len(p):]
		}
	}
	switch sel {
	case "longlong":
		return "long long"
	case "ulonglong":
		return "unsigned long long"
	case "uint":
		return "unsigned int"
	case "ulong":
		return "unsigned long"
	case "ushort":
		return "unsigned short"
	case "schar":
		return "signed char"
	case "uchar":
		return "unsigned char"
	}
	return sel
}

type golayout struct {
	size, align int64
	st          *types.Struct
	offsets     []int64
	sizes       []int64
}

// goLayouts type-checks the generated file on its own and lays out every
// named type the way gc does for -goarch.
func goLayouts(file string) map[string]golayout {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		log.Fatal(err)
	}
	conf := types.Config{Importer: importer.Default(), Error: func(error) {}}
	pkg, _ := conf.Check("syscall", fset, []*ast.File{f}, nil)
	sizes := types.SizesFor("gc", *goarch)
	out := map[string]golayout{}
	for _, name := range pkg.Scope().Names() {
		tn, ok := pkg.Scope().Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		t := tn.Type()
		l := golayout{size: sizes.Sizeof(t), align: sizes.Alignof(t)}
		if st, ok := t.Underlying().(*types.Struct); ok {
			l.st = st
			var fs []*types.Var
			for i := 0; i < st.NumFields(); i++ {
				fs = append(fs, st.Field(i))
				l.sizes = append(l.sizes, sizes.Sizeof(st.Field(i).Type()))
			}
			l.offsets = sizes.Offsetsof(fs)
		}
		out[name] = l
	}
	return out
}

// cLayouts compiles the preamble plus one variable per mapped type with $CC -g
// and reads the layouts back from DWARF.
func cLayouts(preamble string, maps []mapping, cflags []string) map[string]clayout {
	dir, err := os.MkdirTemp("", "mkasserts")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	var src bytes.Buffer
	src.WriteString(preamble)
	for _, m := range maps {
		fmt.Fprintf(&src, "%s __mkasserts_%s;\n", m.ctype, m.goName)
	}
	cfile := filepath.Join(dir, "layout.c")
	ofile := filepath.Join(dir, "layout.o")
	if err := os.WriteFile(cfile, src.Bytes(), 0o666); err != nil {
		log.Fatal(err)
	}
	run(append(append([]string{}, cflags...), "-g", "-c", "-o", ofile, cfile)...)

	ef, err := elf.Open(ofile)
	if err != nil {
		log.Fatal(err)
	}
	d, err := ef.DWARF()
	if err != nil {
		log.Fatal(err)
	}
	out := map[string]clayout{}
	r := d.Reader()
	for {
		e, err := r.Next()
		if err != nil {
			log.Fatal(err)
		}
		if e == nil {
			break
		}
		if e.Tag != dwarf.TagVariable {
			continue
		}
		name, _ := e.Val(dwarf.AttrName).(string)
		goName, ok := strings.CutPrefix(name, "__mkasserts_")
		if !ok {
			continue
		}
		off, _ := e.Val(dwarf.AttrType).(dwarf.Offset)
		t, err := d.Type(off)
		if err != nil {
			log.Fatal(err)
		}
		out[goName] = layoutOf(d, off, t)
	}
	for _, m := range maps {
		if _, ok := out[m.goName]; !ok {
			log.Fatalf("no DWARF for %s (%s)", m.goName, m.ctype)
		}
	}
	return out
}

func layoutOf(d *dwarf.Data, off dwarf.Offset, t dwarf.Type) clayout {
	for {
		td, ok := t.(*dwarf.TypedefType)
		if !ok {
			break
		}
		t = td.Type
	}
	l := clayout{size: t.Size(), align: dwarfAlign(d, off)}
	st, ok := t.(*dwarf.StructType)
	if !ok {
		return l
	}
	l.isStruct = true
	for _, f := range st.Field {
		if f.BitSize != 0 {
			log.Fatalf("%s.%s: bit fields are not supported", st.StructName, f.Name)
		}
		l.members = append(l.members, member{f.Name, f.ByteOffset, f.Type.Size()})
	}
	sort.SliceStable(l.members, func(i, j int) bool { return l.members[i].offset < l.members[j].offset })
	return l
}

// dwarfAlign returns the largest DW_AT_alignment on the type at off (after
// following typedefs) or on its members, or 0. QNX's _Int64t and _Uint64t
// carry aligned(8), which shows up here.
func dwarfAlign(d *dwarf.Data, off dwarf.Offset) int64 {
	r := d.Reader()
	var e *dwarf.Entry
	for {
		r.Seek(off)
		var err error
		if e, err = r.Next(); err != nil || e == nil {
			return 0
		}
		if e.Tag != dwarf.TagTypedef {
			break
		}
		next, ok := e.Val(dwarf.AttrType).(dwarf.Offset)
		if !ok {
			return 0
		}
		off = next
	}
	var max int64
	if a, ok := e.Val(dwarf.AttrAlignment).(int64); ok && a > max {
		max = a
	}
	if e.Tag != dwarf.TagStructType || !e.Children {
		return max
	}
	for {
		c, err := r.Next()
		if err != nil || c == nil || c.Tag == 0 {
			break
		}
		if a, ok := c.Val(dwarf.AttrAlignment).(int64); ok && a > max {
			max = a
		}
		if c.Children {
			r.SkipChildren()
		}
	}
	return max
}

// annotate adds a line comment to each listed field in the generated Go file.
func annotate(file string, annot map[string]map[string]string) {
	if len(annot) == 0 {
		return
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, s := range gd.Specs {
			ts := s.(*ast.TypeSpec)
			st, ok := ts.Type.(*ast.StructType)
			fields := annot[ts.Name.Name]
			if !ok || fields == nil {
				continue
			}
			for _, fl := range st.Fields.List {
				for _, n := range fl.Names {
					if c, ok := fields[n.Name]; ok && fl.Comment == nil {
						fl.Comment = &ast.CommentGroup{List: []*ast.Comment{{Slash: fl.End(), Text: "// " + c}}}
						f.Comments = append(f.Comments, fl.Comment)
					}
				}
			}
		}
	}
	sort.Slice(f.Comments, func(i, j int) bool { return f.Comments[i].Pos() < f.Comments[j].Pos() })
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(file, buf.Bytes(), 0o666); err != nil {
		log.Fatal(err)
	}
}

func run(args ...string) {
	cc := os.Getenv("CC")
	if cc == "" {
		log.Fatal("$CC must be the target C compiler")
	}
	cmd := exec.Command(cc, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("%s %s: %v", cc, strings.Join(args, " "), err)
	}
}

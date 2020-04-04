// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package source

import (
	"context"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strconv"
	"strings"

	"github.com/govim/govim/cmd/govim/internal/golang_org_x_tools/lsp/fuzzy"
	"github.com/govim/govim/cmd/govim/internal/golang_org_x_tools/lsp/protocol"
	"github.com/govim/govim/cmd/govim/internal/golang_org_x_tools/telemetry/event"
)

const maxSymbols = 100

type symbolOption string

const (
	optionSeparator                                   = "::"
	symbolOptionWorkspaceOnly            symbolOption = "workspaceOnly"
	symbolOptionNonWorkspaceExportedOnly symbolOption = "nonWorkspaceExportedOnly"
)

func WorkspaceSymbols(ctx context.Context, views []View, query string) ([]protocol.SymbolInformation, error) {
	ctx, done := event.StartSpan(ctx, "source.WorkspaceSymbols")
	defer done()

	var workspaceOnly bool
	var nonWorkspaceExportedOnly bool

	var queryParts []string
	for _, part := range strings.Fields(query) {
		if ok, v := tokenIsOption(part, symbolOptionWorkspaceOnly); ok {
			if b, err := strconv.ParseBool(v); err == nil {
				workspaceOnly = b
			}
			continue
		}
		if ok, v := tokenIsOption(part, symbolOptionNonWorkspaceExportedOnly); ok {
			if b, err := strconv.ParseBool(v); err == nil {
				nonWorkspaceExportedOnly = b
			}
			continue
		}
		queryParts = append(queryParts, part)
	}
	query = strings.Join(queryParts, " ")

	seen := make(map[string]struct{})
	var symbols []protocol.SymbolInformation
outer:
	for _, view := range views {
		knownPkgs, err := view.Snapshot().KnownPackages(ctx)
		if err != nil {
			return nil, err
		}
		workspacePkgs, err := view.Snapshot().WorkspacePackages(ctx)
		if err != nil {
			return nil, err
		}
		type knownPkg struct {
			PackageHandle
			isInWorkspace bool
		}
		var toSearch []knownPkg
		// Create a slice of packages to search. This slice is sorted such that
		// workspace packages appear first. That way if we exhaust the symbol
		// limit defined by maxSymbols we will have returned symbols "closest" to
		// the code the user is working on.
		for _, ph := range knownPkgs {
			pkgInWorkspace := false
			for _, p := range workspacePkgs {
				if p == ph {
					pkgInWorkspace = true
				}
			}
			if workspaceOnly && !pkgInWorkspace {
				continue
			}
			toSearch = append(toSearch, knownPkg{
				PackageHandle: ph,
				isInWorkspace: pkgInWorkspace,
			})
		}
		sort.Slice(toSearch, func(i, j int) bool {
			return toSearch[i].isInWorkspace
		})
		matcher := makeMatcher(view.Options().SymbolMatcher, query)
		for _, ph := range toSearch {
			pkg, err := ph.Check(ctx)
			if err != nil {
				return nil, err
			}
			if _, ok := seen[pkg.PkgPath()]; ok {
				continue
			}
			seen[pkg.PkgPath()] = struct{}{}
			for _, fh := range pkg.CompiledGoFiles() {
				file, _, _, _, err := fh.Cached()
				if err != nil {
					return nil, err
				}
				for _, si := range findSymbol(file.Decls, pkg.GetTypesInfo(), matcher, pkg.PkgPath(), nonWorkspaceExportedOnly && !ph.isInWorkspace) {
					mrng, err := posToMappedRange(view, pkg, si.node.Pos(), si.node.End())
					if err != nil {
						event.Error(ctx, "Error getting mapped range for node", err)
						continue
					}
					rng, err := mrng.Range()
					if err != nil {
						event.Error(ctx, "Error getting range from mapped range", err)
						continue
					}
					symbols = append(symbols, protocol.SymbolInformation{
						Name: si.name,
						Kind: si.kind,
						Location: protocol.Location{
							URI:   protocol.URIFromSpanURI(mrng.URI()),
							Range: rng,
						},
					})
					if len(symbols) > maxSymbols {
						break outer
					}
				}
			}
		}
	}
	return symbols, nil
}

func tokenIsOption(token string, option symbolOption) (bool, string) {
	pref := string(option) + optionSeparator
	match := strings.HasPrefix(token, pref)
	return match, strings.TrimPrefix(token, pref)
}

type symbolInformation struct {
	name string
	kind protocol.SymbolKind
	node ast.Node
}

type matcherFunc func(string) bool

func makeMatcher(m Matcher, query string) matcherFunc {
	switch m {
	case Fuzzy:
		fm := fuzzy.NewMatcher(query)
		return func(s string) bool {
			return fm.Score(s) > 0
		}
	case CaseSensitive:
		return func(s string) bool {
			return strings.Contains(s, query)
		}
	default:
		q := strings.ToLower(query)
		return func(s string) bool {
			return strings.Contains(strings.ToLower(s), q)
		}
	}
}

func findSymbol(decls []ast.Decl, info *types.Info, matcher matcherFunc, prefix string, exportedOnly bool) []symbolInformation {
	var result []symbolInformation
	for _, decl := range decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			target := prefix + "." + decl.Name.Name
			if (!exportedOnly || decl.Name.IsExported()) && matcher(target) {
				kind := protocol.Function
				if decl.Recv != nil {
					kind = protocol.Method
				}
				result = append(result, symbolInformation{
					name: target,
					kind: kind,
					node: decl.Name,
				})
			}
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					target := prefix + "." + spec.Name.Name
					if (!exportedOnly || spec.Name.IsExported()) && matcher(target) {
						result = append(result, symbolInformation{
							name: target,
							kind: typeToKind(info.TypeOf(spec.Type)),
							node: spec.Name,
						})
					}
					switch st := spec.Type.(type) {
					case *ast.StructType:
						for _, field := range st.Fields.List {
							result = append(result, findFieldSymbol(field, protocol.Field, matcher, target, exportedOnly)...)
						}
					case *ast.InterfaceType:
						for _, field := range st.Methods.List {
							kind := protocol.Method
							if len(field.Names) == 0 {
								kind = protocol.Interface
							}
							result = append(result, findFieldSymbol(field, kind, matcher, target, exportedOnly)...)
						}
					}
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						target := prefix + "." + name.Name
						if (!exportedOnly || name.IsExported()) && matcher(target) {
							kind := protocol.Variable
							if decl.Tok == token.CONST {
								kind = protocol.Constant
							}
							result = append(result, symbolInformation{
								name: target,
								kind: kind,
								node: name,
							})
						}
					}
				}
			}
		}
	}
	return result
}

func typeToKind(typ types.Type) protocol.SymbolKind {
	switch typ := typ.Underlying().(type) {
	case *types.Interface:
		return protocol.Interface
	case *types.Struct:
		return protocol.Struct
	case *types.Signature:
		if typ.Recv() != nil {
			return protocol.Method
		}
		return protocol.Function
	case *types.Named:
		return typeToKind(typ.Underlying())
	case *types.Basic:
		i := typ.Info()
		switch {
		case i&types.IsNumeric != 0:
			return protocol.Number
		case i&types.IsBoolean != 0:
			return protocol.Boolean
		case i&types.IsString != 0:
			return protocol.String
		}
	}
	return protocol.Variable
}

func findFieldSymbol(field *ast.Field, kind protocol.SymbolKind, matcher matcherFunc, prefix string, exportedOnly bool) []symbolInformation {
	var result []symbolInformation

	if len(field.Names) == 0 {
		name := types.ExprString(field.Type)
		target := prefix + "." + name
		if (!exportedOnly || token.IsExported(name)) && matcher(target) {
			result = append(result, symbolInformation{
				name: target,
				kind: kind,
				node: field,
			})
		}
		return result
	}

	for _, name := range field.Names {
		target := prefix + "." + name.Name
		if (!exportedOnly || name.IsExported()) && matcher(target) {
			result = append(result, symbolInformation{
				name: target,
				kind: kind,
				node: name,
			})
		}
	}

	return result
}

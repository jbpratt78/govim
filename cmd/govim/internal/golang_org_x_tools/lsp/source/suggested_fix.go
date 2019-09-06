package source

import (
	"go/token"

	"golang.org/x/tools/go/analysis"
	"github.com/govim/govim/cmd/govim/internal/golang_org_x_tools/lsp/diff"
	"github.com/govim/govim/cmd/govim/internal/golang_org_x_tools/span"
)

func getCodeActions(fset *token.FileSet, diag analysis.Diagnostic) ([]SuggestedFixes, error) {
	var cas []SuggestedFixes
	for _, fix := range diag.SuggestedFixes {
		var ca SuggestedFixes
		ca.Title = fix.Message
		for _, te := range fix.TextEdits {
			span, err := span.NewRange(fset, te.Pos, te.End).Span()
			if err != nil {
				return nil, err
			}
			ca.Edits = append(ca.Edits, diff.TextEdit{Span: span, NewText: string(te.NewText)})
		}
		cas = append(cas, ca)
	}
	return cas, nil
}
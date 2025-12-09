//go:build !remote

package main

import (
	"testing"

	errname "github.com/Antonboom/errname/pkg/analyzer"
	"golang.org/x/tools/go/analysis/passes/atomicalign"
	"golang.org/x/tools/go/analysis/passes/composite"
	"golang.org/x/tools/go/analysis/passes/copylock"
	"golang.org/x/tools/go/analysis/passes/deepequalerrors"
	"golang.org/x/tools/go/analysis/passes/gofix"
	"golang.org/x/tools/go/analysis/passes/hostport"
	"golang.org/x/tools/go/analysis/passes/httpmux"
	"golang.org/x/tools/go/analysis/passes/nilness"
	"golang.org/x/tools/go/analysis/passes/reflectvaluecompare"
	"golang.org/x/tools/go/analysis/passes/sortslice"
	"golang.org/x/tools/go/analysis/passes/unusedwrite"
	"golang.org/x/tools/go/analysis/passes/waitgroup"

	"lesiw.io/checker"
	"lesiw.io/errcheck/errcheck"
	"lesiw.io/linelen"
	"lesiw.io/plscheck/deprecated"
	"lesiw.io/plscheck/embeddirective"
	"lesiw.io/plscheck/fillreturns"
	"lesiw.io/plscheck/infertypeargs"
	"lesiw.io/plscheck/maprange"
	"lesiw.io/plscheck/modernize"
	"lesiw.io/plscheck/nonewvars"
	"lesiw.io/plscheck/noresultvalues"
	"lesiw.io/plscheck/recursiveiter"
	"lesiw.io/plscheck/simplifycompositelit"
	"lesiw.io/plscheck/simplifyrange"
	"lesiw.io/plscheck/simplifyslice"
	"lesiw.io/plscheck/unusedfunc"
	"lesiw.io/plscheck/unusedparams"
	"lesiw.io/plscheck/unusedvariable"
	"lesiw.io/plscheck/yield"
	"lesiw.io/tidytypes"
)

func TestCheck(t *testing.T) {
	checker.Run(t,
		atomicalign.Analyzer,
		composite.Analyzer,
		copylock.Analyzer,
		deepequalerrors.Analyzer,
		deprecated.Analyzer,
		embeddirective.Analyzer,
		errcheck.Analyzer,
		errname.New(),
		fillreturns.Analyzer,
		gofix.Analyzer,
		hostport.Analyzer,
		httpmux.Analyzer,
		infertypeargs.Analyzer,
		linelen.Analyzer,
		maprange.Analyzer,
		modernize.Analyzer,
		nilness.Analyzer,
		nonewvars.Analyzer,
		noresultvalues.Analyzer,
		recursiveiter.Analyzer,
		reflectvaluecompare.Analyzer,
		simplifycompositelit.Analyzer,
		simplifyrange.Analyzer,
		simplifyslice.Analyzer,
		sortslice.Analyzer,
		tidytypes.Analyzer,
		unusedfunc.Analyzer,
		unusedparams.Analyzer,
		unusedvariable.Analyzer,
		unusedwrite.Analyzer,
		waitgroup.Analyzer,
		yield.Analyzer,
	)
}

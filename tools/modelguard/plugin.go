// Package modelguard is a golangci-lint module plugin that enforces the
// project's domain-model discipline across every domain:
//
//   - a domain aggregate (a struct declared in a `.../internal/model` package
//     that has a New* constructor returning it) must be built through that
//     constructor, never via a raw struct literal, outside its own package;
//   - such an aggregate must be mutated through its methods, never by direct
//     field assignment, outside its own package.
//
// Value objects in a model package (structs without a New* constructor, e.g.
// data carriers) are unaffected. Boundary mappers (repositories, goverter
// output) are excluded via .golangci.yml exclusions, not here.
//
// The rule discovers aggregates by type information, so it covers all current
// and future domains with no per-type configuration.
package modelguard

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("modelguard", New)
}

// Settings is the optional configuration block, read from
// linters.settings.custom.modelguard.settings in .golangci.yml.
type Settings struct {
	// ModelPackagePattern overrides the regexp used to recognise domain model
	// packages. Default: (^|/)internal/[^/]+/internal/model$
	ModelPackagePattern string `json:"modelPackagePattern"`
}

// New is the plugin constructor required by the module-plugin contract.
func New(conf any) (register.LinterPlugin, error) {
	settings, err := register.DecodeSettings[Settings](conf)
	if err != nil {
		return nil, err
	}
	return &plugin{settings: settings}, nil
}

type plugin struct {
	settings Settings
}

func (p *plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	a, err := newAnalyzer(p.settings)
	if err != nil {
		return nil, err
	}
	return []*analysis.Analyzer{a}, nil
}

func (p *plugin) GetLoadMode() string {
	// We need full type information to identify aggregates and field writes.
	return register.LoadModeTypesInfo
}
